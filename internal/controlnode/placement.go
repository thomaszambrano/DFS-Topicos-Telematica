package controlnode

import (
	"fmt"
	"math/rand/v2"

	"github.com/thomaszambrano/DFS-Topicos-Telematica/internal/types"
)

// Placer decide en qué DataNodes se escribe cada chunk.
//
// La decisión es SIEMPRE del ControlNode; el cliente solo ejecuta lo que se le
// indica (RT-03). Si el cliente eligiera el destino, no habría forma de sostener
// las invariantes de colocación ni de balancear la ocupación del clúster.
type Placer struct {
	reg *Registry
}

// NewPlacer construye el colocador sobre el registro de membresía.
func NewPlacer(reg *Registry) *Placer {
	return &Placer{reg: reg}
}

// Select elige r DataNodes distintos para un chunk de `size` bytes.
//
// Heurística de dos candidatos aleatorios con puntuación (RN-10, RN-11):
//
//	score = 0.5·(1 − ocupación) + 0.3·(1 − carga) + 0.2·dispersión
//
// Elegir siempre el mejor nodo manda todas las escrituras concurrentes al mismo
// destino hasta el próximo heartbeat; elegir al azar puro reparte mal. Tomar dos al
// azar y quedarse con el mejor reparte casi como mirar todos, sin efecto manada.
//
// Invariantes: los r destinos son distintos (RN-08, dura) y se penalizan los nodos
// del chunk anterior `prev` para esparcir el archivo (RN-09, blanda).
func (p *Placer) Select(index uint32, r int, size int64, prev []types.NodeID) ([]*NodeState, error) {
	if r < 1 {
		return nil, fmt.Errorf("%w: factor de replicación %d", ErrBadRequest, r)
	}

	// Espacio para ESTE chunk, no solo "algo libre" (problema P3).
	var usable []*NodeState
	for _, n := range p.reg.Alive() {
		if n.Free() >= size {
			usable = append(usable, n)
		}
	}
	if len(usable) == 0 {
		return nil, fmt.Errorf("%w: ningún DataNode vivo con %d bytes libres", ErrNoNodes, size)
	}

	maxLoad := int64(1)
	for _, n := range usable {
		maxLoad = max(maxLoad, n.InFlight)
	}
	// Nodos del chunk anterior: elegirlos de nuevo apilaría el archivo (RN-09).
	avoid := make(map[types.NodeID]bool, len(prev))
	for _, id := range prev {
		avoid[id] = true
	}

	picked := make([]*NodeState, 0, r)
	for len(picked) < r && len(usable) > 0 {
		i := bestOfTwo(usable, maxLoad, avoid)
		picked = append(picked, usable[i])
		// Se saca de la lista: dos réplicas nunca en el mismo nodo (RN-08).
		usable = append(usable[:i], usable[i+1:]...)
	}
	for _, n := range picked {
		p.reg.Reserve(n.ID, size)
	}
	// Menos destinos que r: escritura degradada, no rechazada (RN-20).
	return picked, nil
}

func score(n *NodeState, maxLoad int64, avoid map[types.NodeID]bool) float64 {
	dispersion := 1.0
	if avoid[n.ID] {
		dispersion = 0
	}
	load := float64(n.InFlight) / float64(maxLoad)
	return 0.5*(1-n.Utilization()) + 0.3*(1-load) + 0.2*dispersion
}

// bestOfTwo devuelve el índice del mejor de dos candidatos distintos al azar.
func bestOfTwo(c []*NodeState, maxLoad int64, avoid map[types.NodeID]bool) int {
	if len(c) == 1 {
		return 0
	}
	a := rand.IntN(len(c))
	b := rand.IntN(len(c) - 1)
	if b >= a {
		b++ // garantiza b != a sin reintentar
	}
	if score(c[b], maxLoad, avoid) > score(c[a], maxLoad, avoid) {
		return b
	}
	return a
}

package controlnode

import (
	"fmt"
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

// Select elige los r DataNodes destino para el chunk que ocupa la posición index.
//
// Reglas que aplica, en orden:
//
//  1. Solo nodos vivos, no en drenado y con espacio libre.
//  2. INVARIANTE DURA (RN-08): los r destinos son nodos distintos. Dos réplicas del
//     mismo chunk en el mismo nodo no son dos réplicas.
//  3. INVARIANTE BLANDA (RN-09): chunks consecutivos empiezan en nodos distintos.
//     Se logra rotando el orden de los candidatos según el índice del chunk. Sin
//     esto, un archivo entero caería en el nodo más vacío y ni la escritura ni la
//     lectura se repartirían (RX-11, RX-12).
//  4. Dentro de esa rotación, los candidatos vienen ordenados por ocupación
//     ascendente, así que el reparto tiende a igualar el uso entre nodos.
//
// En el Hito 2 r = 1. La firma acepta r > 1 desde ahora para que el Hito 3 no la
// cambie: ahí entra la heurística de dos candidatos aleatorios con puntuación
// (RN-10, RN-11), que reemplaza el punto 4 sin tocar el 1, 2 ni 3.
func (p *Placer) Select(index uint32, r int) ([]*NodeState, error) {
	if r < 1 {
		return nil, fmt.Errorf("%w: factor de replicación %d", ErrBadRequest, r)
	}

	candidates := p.reg.Alive()
	usable := make([]*NodeState, 0, len(candidates))
	for _, n := range candidates {
		if n.Free() > 0 {
			usable = append(usable, n)
		}
	}
	if len(usable) == 0 {
		// 503: no hay dónde escribir. El error dice explícitamente la causa (RN-22).
		return nil, fmt.Errorf("%w: ningún DataNode vivo con espacio disponible", ErrNoNodes)
	}

	// La rotación es lo que separa chunks consecutivos. Con 4 nodos, el chunk 0
	// arranca en el primer candidato, el 1 en el segundo, y así: el archivo se
	// esparce en lugar de apilarse.
	offset := int(index) % len(usable)
	picked := make([]*NodeState, 0, r)
	for i := 0; i < len(usable) && len(picked) < r; i++ {
		picked = append(picked, usable[(offset+i)%len(usable)])
	}

	// Menos destinos que r: se acepta la escritura y se marca degradada en lugar de
	// rechazarla (RN-20). Rechazar dejaría el sistema inservible cada vez que el
	// clúster encoge por debajo de R, y RX-07 dice que eso va a pasar en vivo.
	return picked, nil
}

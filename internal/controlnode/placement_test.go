package controlnode

import (
	"testing"
	"time"

	"github.com/thomaszambrano/DFS-Topicos-Telematica/internal/types"
)

func TestSelectBalanceaOcupacion(t *testing.T) {
	reg := NewRegistry(9*time.Second, 30*time.Second)
	for _, id := range []string{"n1", "n2", "n3", "n4"} {
		reg.Update(report(id))
	}
	p := NewPlacer(reg)

	var prev []types.NodeID
	for i := range 100 {
		nodes, err := p.Select(uint32(i), 3, 64<<20, prev)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[types.NodeID]bool{}
		for _, n := range nodes {
			if seen[n.ID] {
				t.Fatalf("chunk %d: réplica repetida en %s (RN-08)", i, n.ID)
			}
			seen[n.ID] = true
		}
		prev = prev[:0]
		for _, n := range nodes {
			prev = append(prev, n.ID)
		}
	}

	lo, hi := 1.0, 0.0
	for _, n := range reg.All() {
		lo, hi = min(lo, n.Utilization()), max(hi, n.Utilization())
	}
	if hi-lo > 0.10 {
		t.Fatalf("diferencia de ocupación %.1f%% > 10%% (RN-11)", (hi-lo)*100)
	}
}

func TestSelectSinEspacioSuficiente(t *testing.T) {
	reg := NewRegistry(9*time.Second, 30*time.Second)
	reg.Update(report("n1"))
	if _, err := NewPlacer(reg).Select(0, 1, 20<<30, nil); err == nil {
		t.Fatal("se esperaba ErrNoNodes: el chunk no cabe en ningún nodo")
	}
}

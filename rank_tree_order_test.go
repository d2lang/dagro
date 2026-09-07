package dagro

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"testing"
)

func TestRankTreeOrderMatchesSeparateTraversals(t *testing.T) {
	ids := []string{"z", "10", "2", "0", "01", "a", "b", "c", "d", "\ue000", "\U00010000", "100", "5", "9", "22"}
	totalExchanges := 0
	for seed := int64(0); seed < 100; seed++ {
		build := func() *Graph {
			rng := rand.New(rand.NewSource(seed))
			g := NewGraph()
			for _, id := range ids {
				g.SetNode(id, Attrs{})
			}
			for i := 1; i < len(ids); i++ {
				g.SetEdge(ids[rng.Intn(i)], ids[i], Attrs{"weight": float64(rng.Intn(100)) / 7, "minlen": float64(1 + rng.Intn(4))})
			}
			for i, id := range ids {
				for j := i + 1; j < len(ids); j++ {
					if rng.Intn(6) == 0 {
						g.SetEdge(id, ids[j], Attrs{"weight": float64(rng.Intn(100)) / 7, "minlen": float64(1 + rng.Intn(4))})
					}
				}
			}
			return g
		}
		g, wantG := build(), build()
		longestPath(g)
		longestPath(wantG)
		tree, wantTree := feasibleTree(g), feasibleTree(wantG)
		order := newRankTreeOrder(tree)
		order.assignLowLim(tree)
		assignCutValues(tree, g, order.post)
		initLowLimValues(wantTree)
		initCutValues(wantTree, wantG)
		compare := func(a, b *Graph) {
			// JSON also distinguishes negative zero, unlike reflect.DeepEqual.
			got, err := json.Marshal([]any{a.Nodes(), a.Edges(), a.nodes, a.edgeLabels})
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal([]any{b.Nodes(), b.Edges(), b.nodes, b.edgeLabels})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("seed %d: graph changed\ngot %s\nwant %s", seed, got, want)
			}
		}
		for exchanges := 0; ; exchanges++ {
			compare(tree, wantTree)
			compare(g, wantG)
			e, ok := leaveEdge(wantTree)
			if !ok {
				break
			}
			if exchanges > 1000 {
				t.Fatalf("seed %d: excessive exchanges", seed)
			}
			f, ok := enterEdge(wantTree, wantG, e)
			if !ok {
				t.Fatalf("seed %d: no entering edge", seed)
			}
			totalExchanges++
			exchangeEdges(tree, g, e, f, order)
			exchangeEdges(wantTree, wantG, e, f)
		}
	}
	if totalExchanges < 20 {
		t.Fatalf("only %d exchanges exercised", totalExchanges)
	}
	t.Logf("compared %d tree exchanges across 100 graphs", totalExchanges)
}

func TestRankTreeOrderPreservesDifferentRankRoot(t *testing.T) {
	build := func() (*Graph, *Graph) {
		g := newRankTestGraph(true).
			SetNode("a", Attrs{"rank": 0.0, "parent": "input-root"}).
			SetNode("b", Attrs{"rank": 2.0}).
			SetNode("c", Attrs{"rank": 5.0}).
			SetPath([]string{"a", "b", "c"})
		tree := newRankTestTree().SetPath([]string{"a", "b", "c"})
		initLowLimValues(tree)
		return g, tree
	}
	g, tree := build()
	wantG, wantTree := build()
	order := newRankTreeOrder(tree)
	order.assignLowLim(tree)
	updateRanksWithOrder(tree, g, order.pre)
	updateRanks(wantTree, wantG)
	for _, id := range g.Nodes() {
		if got, want := num(asAttrs(g.Node(id)), "rank"), num(asAttrs(wantG.Node(id)), "rank"); math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("node %q rank = %v; want %v", id, got, want)
		}
	}
}

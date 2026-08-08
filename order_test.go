package dagro

import (
	"reflect"
	"testing"
)

func TestOrder(t *testing.T) {
	t.Run("layer matrix skips ranked nodes without order", func(t *testing.T) {
		g := newOrderTestGraph(false)
		g.SetNode("ordered", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("missing-order", Attrs{"rank": float64(0)})
		got := orderBuildLayerMatrix(g)
		want := [][]string{{"ordered"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("orderBuildLayerMatrix = %v, want %v", got, want)
		}
	})

	t.Run("does not add crossings to a tree", func(t *testing.T) {
		g := newOrderTestGraph(false)
		g.SetNode("a", Attrs{"rank": float64(1)})
		for _, v := range []string{"b", "e"} {
			g.SetNode(v, Attrs{"rank": float64(2)})
		}
		for _, v := range []string{"c", "d", "f"} {
			g.SetNode(v, Attrs{"rank": float64(3)})
		}
		g.SetPath([]string{"a", "b", "c"})
		g.SetEdge("b", "d")
		g.SetPath([]string{"a", "e", "f"})
		order(g)
		if got := crossCount(g, orderBuildLayerMatrix(g)); got != 0 {
			t.Fatalf("crossCount after order = %v, want 0", got)
		}
	})

	t.Run("orders an edgeless three-rank graph", func(t *testing.T) {
		g := newOrderTestGraph(false)
		for _, pair := range []struct {
			id   string
			rank float64
		}{
			{"a", 1}, {"d", 1},
			{"b", 2}, {"f", 2}, {"e", 2},
			{"c", 3}, {"g", 3},
		} {
			g.SetNode(pair.id, Attrs{"rank": pair.rank})
		}
		order(g)
		if got := crossCount(g, orderBuildLayerMatrix(g)); got != 0 {
			t.Fatalf("crossCount after order = %v, want 0", got)
		}
	})

	t.Run("orders an edgeless four-rank graph", func(t *testing.T) {
		g := newOrderTestGraph(false)
		for _, pair := range []struct {
			id   string
			rank float64
		}{
			{"a", 1},
			{"b", 2}, {"e", 2}, {"g", 2},
			{"c", 3}, {"f", 3}, {"h", 3},
			{"d", 4},
		} {
			g.SetNode(pair.id, Attrs{"rank": pair.rank})
		}
		order(g)
		if got := crossCount(g, orderBuildLayerMatrix(g)); got > 1 {
			t.Fatalf("crossCount after order = %v, want <= 1", got)
		}
	})

	t.Run("removes a simple crossing", func(t *testing.T) {
		g := newOrderTestGraph(false)
		g.SetNode("a", Attrs{"rank": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(0)})
		g.SetNode("c", Attrs{"rank": float64(1)})
		g.SetNode("d", Attrs{"rank": float64(1)})
		g.SetEdge("a", "d")
		g.SetEdge("b", "c")
		order(g)
		if got := crossCount(g, orderBuildLayerMatrix(g)); got != 0 {
			t.Fatalf("crossCount after order = %v, want 0; layering=%v", got, orderBuildLayerMatrix(g))
		}
	})

	t.Run("stable across repeated graphs", func(t *testing.T) {
		var want [][]string
		for iteration := 0; iteration < 50; iteration++ {
			g := newOrderTestGraph(false)
			for _, pair := range []struct {
				id   string
				rank float64
			}{{"0", 0}, {"1", 0}, {"2", 0}, {"3", 1}, {"4", 1}, {"5", 1}} {
				g.SetNode(pair.id, Attrs{"rank": pair.rank})
			}
			g.SetEdge("0", "4")
			g.SetEdge("0", "5")
			g.SetEdge("1", "3")
			g.SetEdge("2", "3")
			order(g)
			got := orderBuildLayerMatrix(g)
			if iteration == 0 {
				want = cloneLayering(got)
			} else if !reflect.DeepEqual(got, want) {
				t.Fatalf("iteration %d layering = %v, want %v", iteration, got, want)
			}
		}
	})
}

package dagro

import (
	"reflect"
	"testing"
)

func newSortSubgraphTestGraph(t *testing.T) (*Graph, *Graph) {
	t.Helper()
	g := newOrderTestGraph(true)
	for i, v := range []string{"0", "1", "2", "3", "4"} {
		g.SetNode(v, Attrs{"order": float64(i)})
	}
	return g, NewGraph()
}

func setOrderTestParents(t *testing.T, g *Graph, children []string, parent string) {
	t.Helper()
	for _, child := range children {
		if err := g.SetParent(child, parent); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSortSubgraph(t *testing.T) {
	t.Run("flat barycenter sort", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetEdge("3", "x")
		g.SetEdge("1", "y", Attrs{"weight": float64(2)})
		g.SetEdge("4", "y")
		setOrderTestParents(t, g, []string{"x", "y"}, "movable")
		if got, want := sortSubgraph(g, "movable", cg, false).VS, []string{"y", "x"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("sortSubgraph = %v, want %v", got, want)
		}
	})

	t.Run("preserves node without neighbors", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetEdge("3", "x")
		g.SetNode("y")
		g.SetEdge("1", "z", Attrs{"weight": float64(2)})
		g.SetEdge("4", "z")
		setOrderTestParents(t, g, []string{"x", "y", "z"}, "movable")
		if got, want := sortSubgraph(g, "movable", cg, false).VS, []string{"z", "y", "x"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("sortSubgraph = %v, want %v", got, want)
		}
	})

	t.Run("tie bias", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetEdge("1", "x")
		g.SetEdge("1", "y")
		setOrderTestParents(t, g, []string{"x", "y"}, "movable")
		if got, want := sortSubgraph(g, "movable", cg, false).VS, []string{"x", "y"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("left-biased sort = %v, want %v", got, want)
		}
		if got, want := sortSubgraph(g, "movable", cg, true).VS, []string{"y", "x"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("right-biased sort = %v, want %v", got, want)
		}
	})

	t.Run("aggregates subgraph statistics", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetEdge("3", "x")
		g.SetEdge("1", "y", Attrs{"weight": float64(2)})
		g.SetEdge("4", "y")
		setOrderTestParents(t, g, []string{"x", "y"}, "movable")
		got := sortSubgraph(g, "movable", cg, false)
		if !got.HasBarycenter || got.Barycenter != 2.25 || got.Weight != 4 {
			t.Fatalf("sortSubgraph stats = %#v, want barycenter=2.25 weight=4", got)
		}
	})

	t.Run("nested subgraph without barycenter", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetNodes([]string{"a", "b", "c"})
		setOrderTestParents(t, g, []string{"a", "b", "c"}, "y")
		g.SetEdge("0", "x")
		g.SetEdge("1", "z")
		g.SetEdge("2", "y")
		setOrderTestParents(t, g, []string{"x", "y", "z"}, "movable")
		want := []string{"x", "z", "a", "b", "c"}
		if got := sortSubgraph(g, "movable", cg, false).VS; !reflect.DeepEqual(got, want) {
			t.Fatalf("sortSubgraph = %v, want %v", got, want)
		}
	})

	t.Run("nested subgraph with barycenter", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetNodes([]string{"a", "b", "c"})
		setOrderTestParents(t, g, []string{"a", "b", "c"}, "y")
		g.SetEdge("0", "a", Attrs{"weight": float64(3)})
		g.SetEdge("0", "x")
		g.SetEdge("1", "z")
		g.SetEdge("2", "y")
		setOrderTestParents(t, g, []string{"x", "y", "z"}, "movable")
		want := []string{"x", "a", "b", "c", "z"}
		if got := sortSubgraph(g, "movable", cg, false).VS; !reflect.DeepEqual(got, want) {
			t.Fatalf("sortSubgraph = %v, want %v", got, want)
		}
	})

	t.Run("nested subgraph with leaf in-edges", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetNodes([]string{"a", "b", "c"})
		setOrderTestParents(t, g, []string{"a", "b", "c"}, "y")
		g.SetEdge("0", "a")
		g.SetEdge("1", "b")
		g.SetEdge("0", "x")
		g.SetEdge("1", "z")
		setOrderTestParents(t, g, []string{"x", "y", "z"}, "movable")
		want := []string{"x", "a", "b", "c", "z"}
		if got := sortSubgraph(g, "movable", cg, false).VS; !reflect.DeepEqual(got, want) {
			t.Fatalf("sortSubgraph = %v, want %v", got, want)
		}
	})

	t.Run("border nodes stay at extremes", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetEdge("0", "x")
		g.SetEdge("1", "y")
		g.SetEdge("2", "z")
		g.SetNode("sg1", Attrs{"borderLeft": "bl", "borderRight": "br"})
		setOrderTestParents(t, g, []string{"x", "y", "z", "bl", "br"}, "sg1")
		want := []string{"bl", "x", "y", "z", "br"}
		if got := sortSubgraph(g, "sg1", cg, false).VS; !reflect.DeepEqual(got, want) {
			t.Fatalf("sortSubgraph = %v, want %v", got, want)
		}
	})

	t.Run("border predecessors determine barycenter", func(t *testing.T) {
		g, cg := newSortSubgraphTestGraph(t)
		g.SetNode("bl1", Attrs{"order": float64(0)})
		g.SetNode("br1", Attrs{"order": float64(1)})
		g.SetEdge("bl1", "bl2")
		g.SetEdge("br1", "br2")
		setOrderTestParents(t, g, []string{"bl2", "br2"}, "sg")
		g.SetNode("sg", Attrs{"borderLeft": "bl2", "borderRight": "br2"})
		got := sortSubgraph(g, "sg", cg, false)
		want := orderResult{VS: []string{"bl2", "br2"}, Barycenter: 0.5, Weight: 2, HasBarycenter: true}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("sortSubgraph = %#v, want %#v", got, want)
		}
	})
}

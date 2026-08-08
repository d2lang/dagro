package dagro

import (
	"reflect"
	"testing"
)

func newOrderTestGraph(compound bool) *Graph {
	g := NewGraph(GraphOptions{Compound: compound, Multigraph: true})
	g.SetDefaultNodeLabel(func(string) any { return Attrs{} })
	g.SetDefaultEdgeLabel(func(string, string, *string) any { return Attrs{"weight": float64(1)} })
	return g
}

func TestBarycenter(t *testing.T) {
	t.Run("no predecessors", func(t *testing.T) {
		g := newOrderTestGraph(false)
		g.SetNode("x", Attrs{})
		got := barycenter(g, []string{"x"})
		want := []orderEntry{{V: "x"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("barycenter = %#v, want %#v", got, want)
		}
	})

	t.Run("sole predecessor", func(t *testing.T) {
		g := newOrderTestGraph(false)
		g.SetNode("a", Attrs{"order": float64(2)})
		g.SetEdge("a", "x")
		got := barycenter(g, []string{"x"})
		want := []orderEntry{{V: "x", Barycenter: 2, Weight: 1, HasBarycenter: true}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("barycenter = %#v, want %#v", got, want)
		}
	})

	t.Run("weighted predecessors and all movable nodes", func(t *testing.T) {
		g := newOrderTestGraph(false)
		g.SetNode("a", Attrs{"order": float64(1)})
		g.SetNode("b", Attrs{"order": float64(2)})
		g.SetNode("c", Attrs{"order": float64(4)})
		g.SetEdge("a", "x")
		g.SetEdge("b", "x")
		g.SetNode("y")
		g.SetEdge("a", "z", Attrs{"weight": float64(2)})
		g.SetEdge("c", "z")
		got := barycenter(g, []string{"x", "y", "z"})
		want := []orderEntry{
			{V: "x", Barycenter: 1.5, Weight: 2, HasBarycenter: true},
			{V: "y"},
			{V: "z", Barycenter: 2, Weight: 3, HasBarycenter: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("barycenter = %#v, want %#v", got, want)
		}
	})
}

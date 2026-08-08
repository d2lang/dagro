package dagro

import (
	"math"
	"reflect"
	"testing"
)

func TestFeasibleTree(t *testing.T) {
	t.Run("trivial graph", func(t *testing.T) {
		g := NewGraph().
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(1)}).
			SetEdge("a", "b", Attrs{"minlen": float64(1)})
		tree := feasibleTree(g)
		requireNumber(t,
			num(asAttrs(g.Node("b")), "rank"),
			num(asAttrs(g.Node("a")), "rank")+1)
		if got := tree.Neighbors("a"); !reflect.DeepEqual(got, []string{"b"}) {
			t.Fatalf("neighbors(a) = %v", got)
		}
	})

	t.Run("pulls a node up", func(t *testing.T) {
		g := NewGraph().
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(1)}).
			SetNode("c", Attrs{"rank": float64(2)}).
			SetNode("d", Attrs{"rank": float64(2)}).
			SetPath([]string{"a", "b", "c"}, Attrs{"minlen": float64(1)}).
			SetEdge("a", "d", Attrs{"minlen": float64(1)})
		tree := feasibleTree(g)
		requireRank(t, g, "b", num(asAttrs(g.Node("a")), "rank")+1)
		requireRank(t, g, "c", num(asAttrs(g.Node("b")), "rank")+1)
		requireRank(t, g, "d", num(asAttrs(g.Node("a")), "rank")+1)
		for node, want := range map[string][]string{
			"a": {"b", "d"}, "b": {"a", "c"}, "c": {"b"}, "d": {"a"},
		} {
			if got := sortedStrings(tree.Neighbors(node)); !reflect.DeepEqual(got, want) {
				t.Fatalf("neighbors(%s) = %v, want %v", node, got, want)
			}
		}
	})

	t.Run("pulls a node down", func(t *testing.T) {
		g := NewGraph().
			SetNode("a", Attrs{"rank": float64(2)}).
			SetNode("b", Attrs{"rank": float64(0)}).
			SetNode("c", Attrs{"rank": float64(2)}).
			SetEdge("b", "a", Attrs{"minlen": float64(1)}).
			SetEdge("b", "c", Attrs{"minlen": float64(1)})
		tree := feasibleTree(g)
		requireRank(t, g, "a", num(asAttrs(g.Node("b")), "rank")+1)
		requireRank(t, g, "c", num(asAttrs(g.Node("b")), "rank")+1)
		for node, want := range map[string][]string{
			"a": {"b"}, "b": {"a", "c"}, "c": {"b"},
		} {
			if got := sortedStrings(tree.Neighbors(node)); !reflect.DeepEqual(got, want) {
				t.Fatalf("neighbors(%s) = %v, want %v", node, got, want)
			}
		}
	})
}

func TestTightTreeTreatsNaNSlackAsFalsy(t *testing.T) {
	g := NewGraph().
		SetNode("a", Attrs{"rank": float64(0)}).
		SetNode("b", Attrs{"rank": float64(0)}).
		SetEdge("a", "b", Attrs{"minlen": math.NaN()})
	tree := NewGraph(GraphOptions{Undirected: true}).SetNode("a", Attrs{})
	if got := tightTree(tree, g); got != 2 || !tree.HasNode("b") {
		t.Fatalf("tightTree with NaN slack has %d nodes (b=%v), want 2", got, tree.HasNode("b"))
	}
}

func TestFindMinSlackEdgeKeepsFirstTie(t *testing.T) {
	g := NewGraph().
		SetNode("a", Attrs{"rank": float64(0)}).
		SetNode("b", Attrs{"rank": float64(2)}).
		SetNode("c", Attrs{"rank": float64(2)}).
		SetEdge("a", "b", Attrs{"minlen": float64(1)}).
		SetEdge("a", "c", Attrs{"minlen": float64(1)})
	tree := NewGraph(GraphOptions{Undirected: true}).SetNode("a", Attrs{})
	edge, ok := findMinSlackEdge(tree, g)
	if !ok || edge.V != "a" || edge.W != "b" {
		t.Fatalf("first tied edge = %#v, %v", edge, ok)
	}
}

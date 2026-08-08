package dagro

import (
	"reflect"
	"sort"
	"testing"
)

func graphHasDirectedCycle(g *Graph) bool {
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(v string) bool {
		if state[v] == 1 {
			return true
		}
		if state[v] == 2 {
			return false
		}
		state[v] = 1
		for _, w := range g.Successors(v) {
			if visit(w) {
				return true
			}
		}
		state[v] = 2
		return false
	}
	for _, v := range g.Nodes() {
		if visit(v) {
			return true
		}
	}
	return false
}

func newAcyclicTestGraph(acyclicer string) *Graph {
	return NewGraph(GraphOptions{Multigraph: true}).
		SetGraph(Attrs{"acyclicer": acyclicer}).
		SetDefaultEdgeLabel(func(string, string, *string) any {
			return Attrs{"minlen": float64(1), "weight": float64(1)}
		})
}

func TestAcyclicRunAndUndo(t *testing.T) {
	for _, acyclicer := range []string{"greedy", "dfs", "unknown-should-still-work"} {
		t.Run(acyclicer, func(t *testing.T) {
			t.Run("acyclic graph unchanged", func(t *testing.T) {
				g := newAcyclicTestGraph(acyclicer).
					SetPath([]string{"a", "b", "d"}).
					SetPath([]string{"a", "c", "d"})
				want := append([]Edge(nil), g.Edges()...)
				runAcyclic(g)
				if graphHasDirectedCycle(g) || !reflect.DeepEqual(g.Edges(), want) {
					t.Fatalf("acyclic graph changed: got %#v, want %#v", g.Edges(), want)
				}
			})

			t.Run("breaks cycle and preserves edge count", func(t *testing.T) {
				g := newAcyclicTestGraph(acyclicer).SetPath([]string{"a", "b", "c", "d", "a"})
				runAcyclic(g)
				if graphHasDirectedCycle(g) || g.EdgeCount() != 4 {
					t.Fatalf("cycle remains=%v edge count=%d edges=%#v",
						graphHasDirectedCycle(g), g.EdgeCount(), g.Edges())
				}
			})

			t.Run("creates reverse multiedge", func(t *testing.T) {
				g := newAcyclicTestGraph(acyclicer).SetPath([]string{"a", "b", "a"})
				runAcyclic(g)
				if graphHasDirectedCycle(g) || g.EdgeCount() != 2 {
					t.Fatalf("two-cycle not broken: %#v", g.Edges())
				}
				if len(g.OutEdges("a", "b")) != 2 && len(g.OutEdges("b", "a")) != 2 {
					t.Fatalf("reversal did not create parallel edges: %#v", g.Edges())
				}
			})

			t.Run("undo restores labels names and directions", func(t *testing.T) {
				g := newAcyclicTestGraph(acyclicer)
				g.SetEdge("a", "b", Attrs{"minlen": float64(2), "weight": float64(3)}, "ab")
				g.SetEdge("b", "a", Attrs{"minlen": float64(3), "weight": float64(4)}, "ba")
				runAcyclic(g)
				undoAcyclic(g)
				if g.EdgeCount() != 2 || !g.HasEdge("a", "b", "ab") || !g.HasEdge("b", "a", "ba") {
					t.Fatalf("undo identities = %#v", g.Edges())
				}
				ab := asAttrs(g.EdgeByArgs("a", "b", "ab"))
				ba := asAttrs(g.EdgeByArgs("b", "a", "ba"))
				if num(ab, "minlen") != 2 || num(ab, "weight") != 3 ||
					num(ba, "minlen") != 3 || num(ba, "weight") != 4 {
					t.Fatalf("undo labels: ab=%#v ba=%#v", ab, ba)
				}
				if has(ab, "reversed") || has(ba, "reversed") || has(ab, "forwardName") || has(ba, "forwardName") {
					t.Fatalf("undo left internal attrs: ab=%#v ba=%#v", ab, ba)
				}
			})
		})
	}
}

func TestGreedyAcyclicPrefersLowWeightEdge(t *testing.T) {
	g := newAcyclicTestGraph("greedy").
		SetDefaultEdgeLabel(func(string, string, *string) any {
			return Attrs{"minlen": float64(1), "weight": float64(2)}
		}).
		SetPath([]string{"a", "b", "c", "d", "a"})
	g.SetEdge("c", "d", Attrs{"minlen": float64(1), "weight": float64(1)})
	runAcyclic(g)
	if graphHasDirectedCycle(g) || g.HasEdge("c", "d") {
		t.Fatalf("greedy did not reverse low-weight edge: %#v", sortedEdgeStrings(g.Edges()))
	}
}

func sortedEdgeStrings(edges []Edge) []string {
	out := make([]string, len(edges))
	for i, e := range edges {
		out[i] = e.V + "->" + e.W + ":" + e.Name
	}
	sort.Strings(out)
	return out
}

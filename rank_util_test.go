package dagro

import (
	"math"
	"sort"
	"testing"
)

func newRankTestGraph(multigraph bool) *Graph {
	return NewGraph(GraphOptions{Multigraph: multigraph}).
		SetDefaultNodeLabel(func(string) any { return Attrs{} }).
		SetDefaultEdgeLabel(func(string, string, *string) any {
			return Attrs{"minlen": float64(1), "weight": float64(1)}
		})
}

func newRankTestTree() *Graph {
	return NewGraph(GraphOptions{Undirected: true}).
		SetDefaultNodeLabel(func(string) any { return Attrs{} }).
		SetDefaultEdgeLabel(func(string, string, *string) any { return Attrs{} })
}

func normalizeRanksForTest(g *Graph) {
	minimum := math.Inf(1)
	for _, v := range g.Nodes() {
		r := num(asAttrs(g.Node(v)), "rank")
		if r < minimum {
			minimum = r
		}
	}
	for _, v := range g.Nodes() {
		label := asAttrs(g.Node(v))
		label["rank"] = num(label, "rank") - minimum
	}
}

func requireNumber(t *testing.T, got, want float64) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func requireRank(t *testing.T, g *Graph, node string, want float64) {
	t.Helper()
	requireNumber(t, num(asAttrs(g.Node(node)), "rank"), want)
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func TestLongestPath(t *testing.T) {
	t.Run("single node", func(t *testing.T) {
		g := newRankTestGraph(false).SetNode("a")
		longestPath(g)
		normalizeRanksForTest(g)
		requireRank(t, g, "a", 0)
	})

	t.Run("unconnected nodes", func(t *testing.T) {
		g := newRankTestGraph(false).SetNodes([]string{"a", "b"})
		longestPath(g)
		normalizeRanksForTest(g)
		requireRank(t, g, "a", 0)
		requireRank(t, g, "b", 0)
	})

	t.Run("connected nodes", func(t *testing.T) {
		g := newRankTestGraph(false).SetEdge("a", "b")
		longestPath(g)
		normalizeRanksForTest(g)
		requireRank(t, g, "a", 0)
		requireRank(t, g, "b", 1)
	})

	t.Run("diamond", func(t *testing.T) {
		g := newRankTestGraph(false).
			SetPath([]string{"a", "b", "d"}).
			SetPath([]string{"a", "c", "d"})
		longestPath(g)
		normalizeRanksForTest(g)
		for node, want := range map[string]float64{"a": 0, "b": 1, "c": 1, "d": 2} {
			requireRank(t, g, node, want)
		}
	})

	t.Run("minlen", func(t *testing.T) {
		g := newRankTestGraph(false).
			SetPath([]string{"a", "b", "d"}).
			SetEdge("a", "c").
			SetEdge("c", "d", Attrs{"minlen": float64(2), "weight": float64(1)})
		longestPath(g)
		normalizeRanksForTest(g)
		for node, want := range map[string]float64{"a": 0, "b": 2, "c": 1, "d": 3} {
			requireRank(t, g, node, want)
		}
	})
}

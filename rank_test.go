package dagro

import "testing"

func TestRankersRespectMinlen(t *testing.T) {
	for _, ranker := range []string{
		"longest-path", "tight-tree", "network-simplex", "unknown-should-still-work",
	} {
		t.Run(ranker, func(t *testing.T) {
			g := newRankTestGraph(false).SetGraph(Attrs{"ranker": ranker}).
				SetPath([]string{"a", "b", "c", "d", "h"}).
				SetPath([]string{"a", "e", "g", "h"}).
				SetPath([]string{"a", "f", "g"})
			rank(g)
			for _, e := range g.Edges() {
				vRank := num(asAttrs(g.Node(e.V)), "rank")
				wRank := num(asAttrs(g.Node(e.W)), "rank")
				if got, want := wRank-vRank, num(asAttrs(g.Edge(e)), "minlen"); got < want {
					t.Fatalf("edge %s -> %s length %v < minlen %v", e.V, e.W, got, want)
				}
			}
		})
	}
}

func TestRankersHandleSingleNode(t *testing.T) {
	for _, ranker := range []string{
		"longest-path", "tight-tree", "network-simplex", "unknown-should-still-work",
	} {
		t.Run(ranker, func(t *testing.T) {
			g := NewGraph().SetGraph(Attrs{"ranker": ranker}).SetNode("a", Attrs{})
			rank(g)
			requireRank(t, g, "a", 0)
		})
	}
}

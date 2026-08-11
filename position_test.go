package dagro

import "testing"

func newPositionTestGraph() *Graph {
	return NewGraph(GraphOptions{Compound: true}).SetGraph(Attrs{
		"ranksep": float64(50),
		"nodesep": float64(50),
		"edgesep": float64(10),
	})
}

func TestPositionRespectsRankSep(t *testing.T) {
	g := newPositionTestGraph()
	asAttrs(g.Graph())["ranksep"] = float64(1000)
	g.SetNode("a", Attrs{"width": float64(50), "height": float64(100), "rank": float64(0), "order": float64(0)})
	g.SetNode("b", Attrs{"width": float64(50), "height": float64(80), "rank": float64(1), "order": float64(0)})
	g.SetEdge("a", "b")

	position(g)

	if got, want := num(asAttrs(g.Node("b")), "y"), float64(100+1000+80/2); got != want {
		t.Fatalf("b.y = %v, want %v", got, want)
	}
}

func TestPositionUsesLargestHeightInEachRankWithRankSep(t *testing.T) {
	g := newPositionTestGraph()
	asAttrs(g.Graph())["ranksep"] = float64(1000)
	g.SetNode("a", Attrs{"width": float64(50), "height": float64(100), "rank": float64(0), "order": float64(0)})
	g.SetNode("b", Attrs{"width": float64(50), "height": float64(80), "rank": float64(0), "order": float64(1)})
	g.SetNode("c", Attrs{"width": float64(50), "height": float64(90), "rank": float64(1), "order": float64(0)})
	g.SetEdge("a", "c")

	position(g)

	for _, tc := range []struct {
		v    string
		want float64
	}{
		{"a", 100 / 2},
		{"b", 100 / 2},
		{"c", 100 + 1000 + 90/2},
	} {
		if got := num(asAttrs(g.Node(tc.v)), "y"); got != tc.want {
			t.Errorf("%s.y = %v, want %v", tc.v, got, tc.want)
		}
	}
}

func TestPositionYAdvancesAcrossEmptyRanks(t *testing.T) {
	g := newPositionTestGraph()
	g.SetNode("a", Attrs{"height": float64(20), "rank": float64(0), "order": float64(0)})
	g.SetNode("b", Attrs{"height": float64(10), "rank": float64(2), "order": float64(0)})

	positionY(g)

	if got, want := num(asAttrs(g.Node("a")), "y"), float64(10); got != want {
		t.Fatalf("a.y = %v, want %v", got, want)
	}
	if got, want := num(asAttrs(g.Node("b")), "y"), float64(125); got != want {
		t.Fatalf("b.y = %v, want %v", got, want)
	}
}

func TestPositionRespectsNodeSep(t *testing.T) {
	g := newPositionTestGraph()
	asAttrs(g.Graph())["nodesep"] = float64(1000)
	g.SetNode("a", Attrs{"width": float64(50), "height": float64(100), "rank": float64(0), "order": float64(0)})
	g.SetNode("b", Attrs{"width": float64(70), "height": float64(80), "rank": float64(0), "order": float64(1)})

	position(g)

	aX := num(asAttrs(g.Node("a")), "x")
	if got, want := num(asAttrs(g.Node("b")), "x"), aX+50.0/2+1000+70.0/2; got != want {
		t.Fatalf("b.x = %v, want %v", got, want)
	}
}

func TestPositionDoesNotPositionSubgraphNode(t *testing.T) {
	g := newPositionTestGraph()
	g.SetNode("a", Attrs{"width": float64(50), "height": float64(50), "rank": float64(0), "order": float64(0)})
	g.SetNode("sg1", Attrs{})
	if err := g.SetParent("a", "sg1"); err != nil {
		t.Fatal(err)
	}

	position(g)

	if has(asAttrs(g.Node("sg1")), "x") || has(asAttrs(g.Node("sg1")), "y") {
		t.Fatalf("subgraph node was positioned: %#v", g.Node("sg1"))
	}
}

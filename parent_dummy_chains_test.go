package dagro

import "testing"

func newParentDummyTestGraph() *Graph {
	return NewGraph(GraphOptions{Compound: true}).SetGraph(Attrs{})
}

func setTestParent(t *testing.T, g *Graph, child, parent string) {
	t.Helper()
	if err := g.SetParent(child, parent); err != nil {
		t.Fatal(err)
	}
}

func requireTestParent(t *testing.T, g *Graph, child, want string) {
	t.Helper()
	got, ok := g.Parent(child)
	if want == "" {
		if ok {
			t.Fatalf("Parent(%s) = %q, want root", child, got)
		}
		return
	}
	if !ok || got != want {
		t.Fatalf("Parent(%s) = %q, %v, want %q", child, got, ok, want)
	}
}

func TestParentDummyChainsAtRootAndOneSubgraph(t *testing.T) {
	t.Run("both endpoints at root", func(t *testing.T) {
		g := newParentDummyTestGraph()
		g.SetNode("a").SetNode("b").SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "")
	})

	t.Run("tail parent", func(t *testing.T) {
		g := newParentDummyTestGraph()
		setTestParent(t, g, "a", "sg1")
		g.SetNode("sg1", Attrs{"minRank": float64(0), "maxRank": float64(2)})
		g.SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}, "rank": float64(2)})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "sg1")
	})

	t.Run("head parent", func(t *testing.T) {
		g := newParentDummyTestGraph()
		setTestParent(t, g, "b", "sg1")
		g.SetNode("sg1", Attrs{"minRank": float64(1), "maxRank": float64(3)})
		g.SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}, "rank": float64(1)})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "sg1")
	})
}

func TestParentDummyChainsLeavingAndEnteringSubgraph(t *testing.T) {
	t.Run("long chain starts in subgraph", func(t *testing.T) {
		g := newParentDummyTestGraph()
		setTestParent(t, g, "a", "sg1")
		g.SetNode("sg1", Attrs{"minRank": float64(0), "maxRank": float64(2)})
		g.SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}, "rank": float64(2)})
		g.SetNode("d2", Attrs{"rank": float64(3)}).SetNode("d3", Attrs{"rank": float64(4)})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "d2", "d3", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "sg1")
		requireTestParent(t, g, "d2", "")
		requireTestParent(t, g, "d3", "")
	})

	t.Run("long chain ends in subgraph", func(t *testing.T) {
		g := newParentDummyTestGraph()
		setTestParent(t, g, "b", "sg1")
		g.SetNode("sg1", Attrs{"minRank": float64(3), "maxRank": float64(5)})
		g.SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}, "rank": float64(1)})
		g.SetNode("d2", Attrs{"rank": float64(2)}).SetNode("d3", Attrs{"rank": float64(3)})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "d2", "d3", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "")
		requireTestParent(t, g, "d2", "")
		requireTestParent(t, g, "d3", "sg1")
	})
}

func TestParentDummyChainsNestedSubgraphs(t *testing.T) {
	g := newParentDummyTestGraph()
	setTestParent(t, g, "a", "sg2")
	setTestParent(t, g, "sg2", "sg1")
	g.SetNode("sg1", Attrs{"minRank": float64(0), "maxRank": float64(4)})
	g.SetNode("sg2", Attrs{"minRank": float64(1), "maxRank": float64(3)})
	setTestParent(t, g, "b", "sg4")
	setTestParent(t, g, "sg4", "sg3")
	g.SetNode("sg3", Attrs{"minRank": float64(6), "maxRank": float64(10)})
	g.SetNode("sg4", Attrs{"minRank": float64(7), "maxRank": float64(9)})
	for i := 0; i < 5; i++ {
		g.SetNode("d"+string(rune('1'+i)), Attrs{"rank": float64(i + 3)})
	}
	asAttrs(g.Node("d1"))["edgeObj"] = Edge{V: "a", W: "b"}
	asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
	g.SetPath([]string{"a", "d1", "d2", "d3", "d4", "d5", "b"})
	parentDummyChains(g)
	for child, want := range map[string]string{
		"d1": "sg2", "d2": "sg1", "d3": "", "d4": "sg3", "d5": "sg4",
	} {
		requireTestParent(t, g, child, want)
	}
}

func TestParentDummyChainsOverlappingAndNonRootLCA(t *testing.T) {
	t.Run("overlapping ranges", func(t *testing.T) {
		g := newParentDummyTestGraph()
		setTestParent(t, g, "a", "sg1")
		g.SetNode("sg1", Attrs{"minRank": float64(0), "maxRank": float64(3)})
		setTestParent(t, g, "b", "sg2")
		g.SetNode("sg2", Attrs{"minRank": float64(2), "maxRank": float64(6)})
		g.SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}, "rank": float64(2)})
		g.SetNode("d2", Attrs{"rank": float64(3)}).SetNode("d3", Attrs{"rank": float64(4)})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "d2", "d3", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "sg1")
		requireTestParent(t, g, "d2", "sg1")
		requireTestParent(t, g, "d3", "sg2")
	})

	t.Run("lowest common ancestor is subgraph", func(t *testing.T) {
		g := newParentDummyTestGraph()
		setTestParent(t, g, "a", "sg1")
		setTestParent(t, g, "sg2", "sg1")
		g.SetNode("sg1", Attrs{"minRank": float64(0), "maxRank": float64(6)})
		setTestParent(t, g, "b", "sg2")
		g.SetNode("sg2", Attrs{"minRank": float64(3), "maxRank": float64(5)})
		g.SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}, "rank": float64(2)})
		g.SetNode("d2", Attrs{"rank": float64(3)})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "d2", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "sg1")
		requireTestParent(t, g, "d2", "sg2")
	})

	t.Run("tail nested below non-root lowest common ancestor", func(t *testing.T) {
		g := newParentDummyTestGraph()
		setTestParent(t, g, "a", "sg2")
		setTestParent(t, g, "sg2", "sg1")
		g.SetNode("sg1", Attrs{"minRank": float64(0), "maxRank": float64(6)})
		setTestParent(t, g, "b", "sg1")
		g.SetNode("sg2", Attrs{"minRank": float64(1), "maxRank": float64(3)})
		g.SetNode("d1", Attrs{"edgeObj": Edge{V: "a", W: "b"}, "rank": float64(3)})
		g.SetNode("d2", Attrs{"rank": float64(4)})
		asAttrs(g.Graph())["dummyChains"] = []string{"d1"}
		g.SetPath([]string{"a", "d1", "d2", "b"})
		parentDummyChains(g)
		requireTestParent(t, g, "d1", "sg2")
		requireTestParent(t, g, "d2", "sg1")
	})
}

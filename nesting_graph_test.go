package dagro

import (
	"reflect"
	"testing"
)

func newNestingTestGraph() *Graph {
	return NewGraph(GraphOptions{Compound: true}).
		SetGraph(Attrs{}).
		SetDefaultNodeLabel(func(string) any { return Attrs{} })
}

func undirectedComponentCount(g *Graph) int {
	visited := map[string]bool{}
	count := 0
	var visit func(string)
	visit = func(v string) {
		if visited[v] {
			return
		}
		visited[v] = true
		for _, w := range g.Neighbors(v) {
			visit(w)
		}
	}
	for _, v := range g.Nodes() {
		if !visited[v] {
			count++
			visit(v)
		}
	}
	return count
}

func TestRunNestingGraphConnectsAndAddsBorders(t *testing.T) {
	g := newNestingTestGraph().SetNode("a").SetNode("b")
	if got := undirectedComponentCount(g); got != 2 {
		t.Fatalf("initial component count = %d", got)
	}
	runNestingGraph(g)
	if got := undirectedComponentCount(g); got != 1 || !g.HasNode("a") || !g.HasNode("b") {
		t.Fatalf("nested component count = %d nodes=%v", got, g.Nodes())
	}

	g = newNestingTestGraph()
	if err := g.SetParent("a", "sg1"); err != nil {
		t.Fatal(err)
	}
	runNestingGraph(g)
	sg := asAttrs(g.Node("sg1"))
	top, bottom := stringValue(sg, "borderTop"), stringValue(sg, "borderBottom")
	if top == "" || bottom == "" {
		t.Fatalf("missing borders: %#v", sg)
	}
	for _, border := range []string{top, bottom} {
		if parent, ok := g.Parent(border); !ok || parent != "sg1" {
			t.Fatalf("Parent(%s) = %q, %v", border, parent, ok)
		}
		want := Attrs{"width": float64(0), "height": float64(0), "dummy": "border"}
		if got := asAttrs(g.Node(border)); !reflect.DeepEqual(got, want) {
			t.Fatalf("border %s = %#v, want %#v", border, got, want)
		}
	}
	if len(g.OutEdges(top, "a")) != 1 || num(asAttrs(g.EdgeByArgs(top, "a")), "minlen") != 1 ||
		len(g.OutEdges("a", bottom)) != 1 || num(asAttrs(g.EdgeByArgs("a", bottom)), "minlen") != 1 {
		t.Fatalf("border edges: top=%#v bottom=%#v", g.OutEdges(top, "a"), g.OutEdges("a", bottom))
	}
}

func TestRunNestingGraphNestedBordersAndWeights(t *testing.T) {
	g := newNestingTestGraph()
	_ = g.SetParent("sg2", "sg1")
	_ = g.SetParent("a", "sg2")
	g.SetEdge("x", "a", Attrs{"weight": float64(100), "minlen": float64(1)})
	g.SetEdge("a", "y", Attrs{"weight": float64(200), "minlen": float64(1)})
	runNestingGraph(g)

	sg1, sg2 := asAttrs(g.Node("sg1")), asAttrs(g.Node("sg2"))
	sg1Top, sg1Bottom := stringValue(sg1, "borderTop"), stringValue(sg1, "borderBottom")
	sg2Top, sg2Bottom := stringValue(sg2, "borderTop"), stringValue(sg2, "borderBottom")
	if num(asAttrs(g.EdgeByArgs(sg1Top, sg2Top)), "minlen") != 1 ||
		num(asAttrs(g.EdgeByArgs(sg2Bottom, sg1Bottom)), "minlen") != 1 {
		t.Fatalf("nested border edges missing or wrong")
	}
	if num(asAttrs(g.EdgeByArgs(sg2Top, "a")), "weight") <= 300 ||
		num(asAttrs(g.EdgeByArgs("a", sg2Bottom)), "weight") <= 300 {
		t.Fatalf("border weights do not dominate: top=%#v bottom=%#v",
			g.EdgeByArgs(sg2Top, "a"), g.EdgeByArgs("a", sg2Bottom))
	}
}

func TestRunNestingGraphConnectsRootToTopLevelBorder(t *testing.T) {
	g := newNestingTestGraph()
	if err := g.SetParent("a", "sg1"); err != nil {
		t.Fatal(err)
	}
	runNestingGraph(g)

	root := stringValue(asAttrs(g.Graph()), "nestingRoot")
	top := stringValue(asAttrs(g.Node("sg1")), "borderTop")
	if root == "" || top == "" {
		t.Fatalf("missing nesting root or top border: graph=%#v sg1=%#v", g.Graph(), g.Node("sg1"))
	}
	if edges := g.OutEdges(root, top); len(edges) != 1 || !g.HasEdgeObject(edges[0]) {
		t.Fatalf("root -> top-level border edge = %#v, want one live edge", edges)
	}
}

func TestRunNestingGraphNestedBorderMinlen(t *testing.T) {
	g := newNestingTestGraph()
	if err := g.SetParent("a", "sg1"); err != nil {
		t.Fatal(err)
	}
	if err := g.SetParent("sg2", "sg1"); err != nil {
		t.Fatal(err)
	}
	if err := g.SetParent("b", "sg2"); err != nil {
		t.Fatal(err)
	}
	runNestingGraph(g)

	root := stringValue(asAttrs(g.Graph()), "nestingRoot")
	sg1 := asAttrs(g.Node("sg1"))
	sg2 := asAttrs(g.Node("sg2"))
	sg1Top, sg1Bottom := stringValue(sg1, "borderTop"), stringValue(sg1, "borderBottom")
	sg2Top, sg2Bottom := stringValue(sg2, "borderTop"), stringValue(sg2, "borderBottom")
	for _, edge := range []struct {
		v, w string
		want float64
	}{
		{root, sg1Top, 3},
		{sg1Top, sg2Top, 1},
		{sg1Top, "a", 2},
		{"a", sg1Bottom, 2},
		{sg2Top, "b", 1},
		{"b", sg2Bottom, 1},
		{sg2Bottom, sg1Bottom, 1},
	} {
		label, ok := g.EdgeByArgs(edge.v, edge.w).(Attrs)
		if !ok || num(label, "minlen") != edge.want {
			t.Errorf("edge %q -> %q = %#v, want minlen %g", edge.v, edge.w, label, edge.want)
		}
	}
}

func TestRunNestingGraphMinlenByDepth(t *testing.T) {
	for _, tt := range []struct {
		name       string
		configure  func(*Graph)
		wantMinlen float64
	}{
		{
			name: "root node", wantMinlen: 1,
			configure: func(g *Graph) { g.SetNode("a") },
		},
		{
			name: "one container", wantMinlen: 3,
			configure: func(g *Graph) { _ = g.SetParent("a", "sg1") },
		},
		{
			name: "two containers", wantMinlen: 5,
			configure: func(g *Graph) {
				_ = g.SetParent("sg2", "sg1")
				_ = g.SetParent("a", "sg2")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := newNestingTestGraph()
			tt.configure(g)
			runNestingGraph(g)
			root := stringValue(asAttrs(g.Graph()), "nestingRoot")
			label := asAttrs(g.EdgeByArgs(root, "a"))
			if num(label, "weight") != 0 || num(label, "minlen") != tt.wantMinlen {
				t.Fatalf("root -> a label = %#v", label)
			}
			if g.HasEdge(root, root) {
				t.Fatal("nesting graph added root self-edge")
			}
		})
	}

	for _, tt := range []struct {
		depth int
		want  float64
	}{
		{depth: 0, want: 1}, {depth: 1, want: 3}, {depth: 2, want: 5},
	} {
		g := newNestingTestGraph()
		if tt.depth >= 1 {
			_ = g.SetParent("a", "sg1")
		}
		if tt.depth >= 2 {
			_ = g.SetParent("sg1", "sg0")
		}
		g.SetEdge("a", "b", Attrs{"weight": float64(1), "minlen": float64(1)})
		runNestingGraph(g)
		if got := num(asAttrs(g.EdgeByArgs("a", "b")), "minlen"); got != tt.want {
			t.Fatalf("depth %d expanded minlen = %v, want %v", tt.depth, got, tt.want)
		}
	}
}

func TestCleanupNestingGraph(t *testing.T) {
	g := newNestingTestGraph()
	_ = g.SetParent("a", "sg1")
	g.SetEdge("a", "b", Attrs{"weight": float64(1), "minlen": float64(1)})
	runNestingGraph(g)
	root := stringValue(asAttrs(g.Graph()), "nestingRoot")
	cleanupNestingGraph(g)
	if g.HasNode(root) || has(asAttrs(g.Graph()), "nestingRoot") {
		t.Fatalf("nesting root not removed: root=%q nodes=%v graph=%#v", root, g.Nodes(), g.Graph())
	}
	if got := g.Successors("a"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("cleanup left nesting edges: successors(a)=%v", got)
	}
	for _, e := range g.Edges() {
		if boolValue(asAttrs(g.Edge(e)), "nestingEdge") {
			t.Fatalf("cleanup left nesting edge %#v", e)
		}
	}
}

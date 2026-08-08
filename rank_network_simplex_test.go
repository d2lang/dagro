package dagro

import (
	"reflect"
	"sort"
	"testing"
)

func gansnerRankGraph() *Graph {
	return newRankTestGraph(false).
		SetPath([]string{"a", "b", "c", "d", "h"}).
		SetPath([]string{"a", "e", "g", "h"}).
		SetPath([]string{"a", "f", "g"})
}

func gansnerRankTree() *Graph {
	return newRankTestTree().
		SetPath([]string{"a", "b", "c", "d", "h", "g", "e"}).
		SetEdge("g", "f")
}

func runNormalizedNetworkSimplex(g *Graph) {
	networkSimplex(g)
	normalizeRanksForTest(g)
}

func TestNetworkSimplexRanks(t *testing.T) {
	tests := []struct {
		name  string
		build func() *Graph
		want  map[string]float64
	}{
		{
			name:  "single node",
			build: func() *Graph { return newRankTestGraph(true).SetNode("a") },
			want:  map[string]float64{"a": 0},
		},
		{
			name:  "connected pair",
			build: func() *Graph { return newRankTestGraph(true).SetEdge("a", "b") },
			want:  map[string]float64{"a": 0, "b": 1},
		},
		{
			name: "diamond",
			build: func() *Graph {
				return newRankTestGraph(true).
					SetPath([]string{"a", "b", "d"}).
					SetPath([]string{"a", "c", "d"})
			},
			want: map[string]float64{"a": 0, "b": 1, "c": 1, "d": 2},
		},
		{
			name: "minlen",
			build: func() *Graph {
				return newRankTestGraph(true).
					SetPath([]string{"a", "b", "d"}).
					SetEdge("a", "c").
					SetEdge("c", "d", Attrs{"minlen": float64(2), "weight": float64(1)})
			},
			want: map[string]float64{"a": 0, "b": 2, "c": 1, "d": 3},
		},
		{
			name:  "gansner graph",
			build: gansnerRankGraph,
			want: map[string]float64{
				"a": 0, "b": 1, "c": 2, "d": 3, "h": 4, "e": 1, "f": 1, "g": 2,
			},
		},
		{
			name: "multiedges",
			build: func() *Graph {
				return newRankTestGraph(true).
					SetPath([]string{"a", "b", "c", "d"}).
					SetEdge("a", "e", Attrs{"weight": float64(2), "minlen": float64(1)}).
					SetEdge("e", "d").
					SetEdge("b", "c", Attrs{"weight": float64(1), "minlen": float64(2)}, "multi")
			},
			want: map[string]float64{"a": 0, "b": 1, "c": 3, "d": 4, "e": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := tt.build()
			runNormalizedNetworkSimplex(g)
			for node, want := range tt.want {
				requireRank(t, g, node, want)
			}
		})
	}
}

func TestLeaveEdge(t *testing.T) {
	tree := NewGraph(GraphOptions{Undirected: true}).
		SetEdge("a", "b", Attrs{"cutvalue": float64(1)}).
		SetEdge("b", "c", Attrs{"cutvalue": float64(1)})
	if _, ok := leaveEdge(tree); ok {
		t.Fatal("leaveEdge returned an edge without a negative cut value")
	}
	tree.SetEdge("b", "c", Attrs{"cutvalue": float64(-1)})
	if edge, ok := leaveEdge(tree); !ok || edge.V != "b" || edge.W != "c" {
		t.Fatalf("leaveEdge = %#v, %v", edge, ok)
	}
}

func TestEnterEdge(t *testing.T) {
	t.Run("head to tail component", func(t *testing.T) {
		g := newRankTestGraph(true).
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(2)}).
			SetNode("c", Attrs{"rank": float64(3)}).
			SetPath([]string{"a", "b", "c"}).
			SetEdge("a", "c")
		tree := newRankTestTree().SetPath([]string{"b", "c", "a"})
		initLowLimValues(tree, "c")
		edge, ok := enterEdge(tree, g, Edge{V: "b", W: "c"})
		if !ok || undirectedEdgeForTest(edge) != (Edge{V: "a", W: "b"}) {
			t.Fatalf("enterEdge = %#v, %v", edge, ok)
		}
	})

	t.Run("tree root is in tail component", func(t *testing.T) {
		g := newRankTestGraph(true).
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(2)}).
			SetNode("c", Attrs{"rank": float64(3)}).
			SetPath([]string{"a", "b", "c"}).
			SetEdge("a", "c")
		tree := newRankTestTree().SetPath([]string{"b", "c", "a"})
		initLowLimValues(tree, "b")
		edge, ok := enterEdge(tree, g, Edge{V: "b", W: "c"})
		if !ok || undirectedEdgeForTest(edge) != (Edge{V: "a", W: "b"}) {
			t.Fatalf("enterEdge = %#v, %v", edge, ok)
		}
	})

	t.Run("least slack", func(t *testing.T) {
		g := newRankTestGraph(true).
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(1)}).
			SetNode("c", Attrs{"rank": float64(3)}).
			SetNode("d", Attrs{"rank": float64(4)}).
			SetEdge("a", "d").
			SetPath([]string{"a", "c", "d"}).
			SetEdge("b", "c")
		tree := newRankTestTree().SetPath([]string{"c", "d", "a", "b"})
		initLowLimValues(tree, "a")
		edge, ok := enterEdge(tree, g, Edge{V: "c", W: "d"})
		if !ok || undirectedEdgeForTest(edge) != (Edge{V: "b", W: "c"}) {
			t.Fatalf("enterEdge = %#v, %v", edge, ok)
		}
	})

	t.Run("gansner orientations", func(t *testing.T) {
		for _, root := range []string{"a", "e"} {
			for _, leaving := range []Edge{{V: "g", W: "h"}, {V: "h", W: "g"}} {
				g := gansnerRankGraph()
				tree := gansnerRankTree()
				longestPath(g)
				initLowLimValues(tree, root)
				edge, ok := enterEdge(tree, g, leaving)
				undirected := undirectedEdgeForTest(edge)
				if !ok || undirected.V != "a" || (undirected.W != "e" && undirected.W != "f") {
					t.Fatalf("root %s leaving %#v: enterEdge = %#v, %v", root, leaving, edge, ok)
				}
			}
		}
	})
}

func TestInitLowLimValues(t *testing.T) {
	tree := NewGraph().
		SetDefaultNodeLabel(func(string) any { return Attrs{} }).
		SetNodes([]string{"a", "b", "c", "d", "e"}).
		SetPath([]string{"a", "b", "a", "c", "d", "c", "e"})
	initLowLimValues(tree, "a")

	var lims []float64
	for _, v := range tree.Nodes() {
		lims = append(lims, num(asAttrs(tree.Node(v)), "lim"))
	}
	sort.Float64s(lims)
	if !reflect.DeepEqual(lims, []float64{1, 2, 3, 4, 5}) {
		t.Fatalf("lims = %v", lims)
	}
	a := asAttrs(tree.Node("a"))
	if num(a, "low") != 1 || num(a, "lim") != 5 || has(a, "parent") {
		t.Fatalf("root attrs = %#v", a)
	}
	for node, parent := range map[string]string{"b": "a", "c": "a", "d": "c", "e": "c"} {
		if got := stringValue(asAttrs(tree.Node(node)), "parent"); got != parent {
			t.Fatalf("parent(%s) = %s, want %s", node, got, parent)
		}
	}
}

func TestExchangeEdges(t *testing.T) {
	g := gansnerRankGraph()
	tree := gansnerRankTree()
	longestPath(g)
	initLowLimValues(tree)
	exchangeEdges(tree, g, Edge{V: "g", W: "h"}, Edge{V: "a", W: "e"})

	for edge, want := range map[Edge]float64{
		{V: "a", W: "b"}: 2,
		{V: "b", W: "c"}: 2,
		{V: "c", W: "d"}: 2,
		{V: "d", W: "h"}: 2,
		{V: "a", W: "e"}: 1,
		{V: "e", W: "g"}: 1,
		{V: "f", W: "g"}: 0,
	} {
		requireNumber(t, num(asAttrs(tree.Edge(edge)), "cutvalue"), want)
	}
	lims := make([]float64, 0, tree.NodeCount())
	for _, v := range tree.Nodes() {
		lims = append(lims, num(asAttrs(tree.Node(v)), "lim"))
	}
	sort.Float64s(lims)
	if want := []float64{1, 2, 3, 4, 5, 6, 7, 8}; !reflect.DeepEqual(lims, want) {
		t.Fatalf("post-exchange lim values = %v, want %v", lims, want)
	}

	normalizeRanksForTest(g)
	for node, want := range map[string]float64{
		"a": 0, "b": 1, "c": 2, "d": 3, "e": 1, "f": 1, "g": 2, "h": 4,
	} {
		requireRank(t, g, node, want)
	}
}

func TestCalcCutValue(t *testing.T) {
	tests := []struct {
		name  string
		build func(g, tree *Graph)
		want  float64
	}{
		{"c to p", func(g, tree *Graph) { g.SetPath([]string{"c", "p"}); tree.SetPath([]string{"p", "c"}) }, 1},
		{"p to c", func(g, tree *Graph) { g.SetPath([]string{"p", "c"}); tree.SetPath([]string{"p", "c"}) }, 1},
		{"gc to c to p", func(g, tree *Graph) {
			g.SetPath([]string{"gc", "c", "p"})
			tree.SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).SetEdge("p", "c")
		}, 3},
		{"gc to c from p", func(g, tree *Graph) {
			g.SetEdge("p", "c").SetEdge("gc", "c")
			tree.SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).SetEdge("p", "c")
		}, -1},
		{"gc from c to p", func(g, tree *Graph) {
			g.SetEdge("c", "p").SetEdge("c", "gc")
			tree.SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).SetEdge("p", "c")
		}, -1},
		{"gc from c from p", func(g, tree *Graph) {
			g.SetPath([]string{"p", "c", "gc"})
			tree.SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).SetEdge("p", "c")
		}, 3},
		{"outside to child", func(g, tree *Graph) {
			g.SetEdge("o", "c", Attrs{"weight": float64(7), "minlen": float64(1)}).
				SetPath([]string{"gc", "c", "p", "o"})
			tree.SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).SetPath([]string{"c", "p", "o"})
		}, -4},
		{"child to outside", func(g, tree *Graph) {
			g.SetEdge("c", "o", Attrs{"weight": float64(7), "minlen": float64(1)}).
				SetPath([]string{"gc", "c", "p", "o"})
			tree.SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).SetPath([]string{"c", "p", "o"})
		}, 10},
		{"outside precedes grandchild and points to child", func(g, tree *Graph) {
			g.SetEdge("o", "c", Attrs{"weight": float64(7), "minlen": float64(1)}).
				SetPath([]string{"o", "gc", "c", "p"})
			tree.SetEdge("o", "gc").
				SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).
				SetEdge("c", "p")
		}, -4},
		{"outside precedes grandchild and child points to outside", func(g, tree *Graph) {
			g.SetEdge("c", "o", Attrs{"weight": float64(7), "minlen": float64(1)}).
				SetPath([]string{"o", "gc", "c", "p"})
			tree.SetEdge("o", "gc").
				SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).
				SetEdge("c", "p")
		}, 10},
		{"parent points to child and outside points to child", func(g, tree *Graph) {
			g.SetEdge("gc", "c").
				SetEdge("p", "c").
				SetEdge("p", "o").
				SetEdge("o", "c", Attrs{"weight": float64(7), "minlen": float64(1)})
			tree.SetEdge("o", "gc").
				SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).
				SetEdge("c", "p")
		}, 6},
		{"parent points to child and child points to outside", func(g, tree *Graph) {
			g.SetEdge("gc", "c").
				SetEdge("p", "c").
				SetEdge("p", "o").
				SetEdge("c", "o", Attrs{"weight": float64(7), "minlen": float64(1)})
			tree.SetEdge("o", "gc").
				SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).
				SetEdge("c", "p")
		}, -8},
		{"outside precedes grandchild with parent pointing to child", func(g, tree *Graph) {
			g.SetEdge("o", "c", Attrs{"weight": float64(7), "minlen": float64(1)}).
				SetPath([]string{"o", "gc", "c"}).
				SetEdge("p", "c")
			tree.SetEdge("o", "gc").
				SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).
				SetEdge("c", "p")
		}, 6},
		{"outside precedes grandchild with child pointing to outside and parent to child", func(g, tree *Graph) {
			g.SetEdge("c", "o", Attrs{"weight": float64(7), "minlen": float64(1)}).
				SetPath([]string{"o", "gc", "c"}).
				SetEdge("p", "c")
			tree.SetEdge("o", "gc").
				SetEdge("gc", "c", Attrs{"cutvalue": float64(3)}).
				SetEdge("c", "p")
		}, -8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newRankTestGraph(true)
			tree := newRankTestTree()
			tt.build(g, tree)
			initLowLimValues(tree, "p")
			requireNumber(t, calcCutValue(tree, g, "c"), tt.want)
		})
	}
}

func TestUpdateRanksAllowsEmptyStringRoot(t *testing.T) {
	rootLabel := Attrs{"rank": float64(0)}
	childLabel := Attrs{"rank": float64(99), "parent": ""}
	g := newRankTestGraph(false).
		SetNode("", rootLabel).
		SetNode("child", childLabel).
		SetEdge("", "child", Attrs{"minlen": float64(2), "weight": float64(1)})
	tree := newRankTestTree().
		SetNode("", rootLabel).
		SetNode("child", childLabel).
		SetEdge("", "child")

	updateRanks(tree, g)
	requireRank(t, g, "child", 2)
}

func TestInitCutValues(t *testing.T) {
	tests := []struct {
		name   string
		update func(*Graph)
		want   map[Edge]float64
	}{
		{
			name:   "gansner graph",
			update: func(*Graph) {},
			want: map[Edge]float64{
				{V: "a", W: "b"}: 3, {V: "b", W: "c"}: 3,
				{V: "c", W: "d"}: 3, {V: "d", W: "h"}: 3,
				{V: "g", W: "h"}: -1, {V: "e", W: "g"}: 0,
				{V: "f", W: "g"}: 0,
			},
		},
		{
			name: "updated gansner graph",
			update: func(tree *Graph) {
				tree.RemoveEdgeByArgs("g", "h")
				tree.SetEdge("a", "e")
			},
			want: map[Edge]float64{
				{V: "a", W: "b"}: 2, {V: "b", W: "c"}: 2,
				{V: "c", W: "d"}: 2, {V: "d", W: "h"}: 2,
				{V: "a", W: "e"}: 1, {V: "e", W: "g"}: 1,
				{V: "f", W: "g"}: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gansnerRankGraph()
			tree := gansnerRankTree()
			tt.update(tree)
			initLowLimValues(tree)
			initCutValues(tree, g)
			for edge, want := range tt.want {
				requireNumber(t, num(asAttrs(tree.Edge(edge)), "cutvalue"), want)
			}
		})
	}
}

func undirectedEdgeForTest(e Edge) Edge {
	if e.V > e.W {
		e.V, e.W = e.W, e.V
	}
	e.Name = ""
	e.HasName = false
	return e
}

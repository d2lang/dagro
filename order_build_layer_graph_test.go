package dagro

import (
	"reflect"
	"sort"
	"testing"
)

func TestBuildLayerGraph(t *testing.T) {
	newGraph := func() *Graph {
		return NewGraph(GraphOptions{Compound: true, Multigraph: true})
	}

	t.Run("movable roots and flat nodes", func(t *testing.T) {
		g := newGraph()
		g.SetNode("a", Attrs{"rank": float64(1)})
		g.SetNode("b", Attrs{"rank": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(2)})
		g.SetNode("d", Attrs{"rank": float64(3)})
		lg := buildLayerGraph(g, 1, "inEdges")
		root := stringValue(asAttrs(lg.Graph()), "root")
		if !lg.HasNode(root) {
			t.Fatalf("layer graph does not contain root %q", root)
		}
		if got, want := lg.Children(), []string{root}; !reflect.DeepEqual(got, want) {
			t.Fatalf("root children = %#v, want %#v", got, want)
		}
		if got, want := lg.Children(root), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("movable children = %#v, want %#v", got, want)
		}
		for rank, id := range map[int]string{1: "a", 2: "c", 3: "d"} {
			if !buildLayerGraph(g, rank, "inEdges").HasNode(id) {
				t.Fatalf("rank %d layer graph does not contain %q", rank, id)
			}
		}
	})

	t.Run("shares original labels", func(t *testing.T) {
		g := newGraph()
		a := Attrs{"foo": float64(1), "rank": float64(1)}
		b := Attrs{"foo": float64(2), "rank": float64(2)}
		g.SetNode("a", a)
		g.SetNode("b", b)
		g.SetEdge("a", "b", Attrs{"weight": float64(1)})
		lg := buildLayerGraph(g, 2, "inEdges")
		a["foo"] = "updated"
		b["foo"] = "also-updated"
		if asAttrs(lg.Node("a"))["foo"] != "updated" || asAttrs(lg.Node("b"))["foo"] != "also-updated" {
			t.Fatalf("layer graph did not retain shared labels: a=%#v b=%#v", lg.Node("a"), lg.Node("b"))
		}
	})

	t.Run("copies and reverses incident edges", func(t *testing.T) {
		g := newGraph()
		g.SetNode("a", Attrs{"rank": float64(1)})
		g.SetNode("b", Attrs{"rank": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(2)})
		g.SetNode("d", Attrs{"rank": float64(3)})
		g.SetEdge("a", "c", Attrs{"weight": float64(2)})
		g.SetEdge("b", "c", Attrs{"weight": float64(3)})
		g.SetEdge("c", "d", Attrs{"weight": float64(4)})

		if got := buildLayerGraph(g, 1, "inEdges").EdgeCount(); got != 0 {
			t.Fatalf("rank 1 in-edge count = %d, want 0", got)
		}
		lg := buildLayerGraph(g, 2, "inEdges")
		if got := num(asAttrs(lg.EdgeByArgs("a", "c")), "weight"); got != 2 {
			t.Fatalf("a->c weight = %v, want 2", got)
		}
		if got := num(asAttrs(lg.EdgeByArgs("b", "c")), "weight"); got != 3 {
			t.Fatalf("b->c weight = %v, want 3", got)
		}
		lg = buildLayerGraph(g, 3, "inEdges")
		if got := lg.EdgeCount(); got != 1 {
			t.Fatalf("rank 3 in-edge count = %d, want 1", got)
		}
		if got := num(asAttrs(lg.EdgeByArgs("c", "d")), "weight"); got != 4 {
			t.Fatalf("c->d weight = %v, want 4", got)
		}
		lg = buildLayerGraph(g, 1, "outEdges")
		if got := num(asAttrs(lg.EdgeByArgs("c", "a")), "weight"); got != 2 {
			t.Fatalf("reversed c->a weight = %v, want 2", got)
		}
		if got := num(asAttrs(lg.EdgeByArgs("c", "b")), "weight"); got != 3 {
			t.Fatalf("reversed c->b weight = %v, want 3", got)
		}
		lg = buildLayerGraph(g, 2, "outEdges")
		if got := lg.EdgeCount(); got != 1 {
			t.Fatalf("rank 2 out-edge count = %d, want 1", got)
		}
		if got := num(asAttrs(lg.EdgeByArgs("d", "c")), "weight"); got != 4 {
			t.Fatalf("reversed d->c weight = %v, want 4", got)
		}
		if got := buildLayerGraph(g, 3, "outEdges").EdgeCount(); got != 0 {
			t.Fatalf("rank 3 out-edge count = %d, want 0", got)
		}
	})

	t.Run("collapses multiedges", func(t *testing.T) {
		g := newGraph()
		g.SetNode("a", Attrs{"rank": float64(1)})
		g.SetNode("b", Attrs{"rank": float64(2)})
		g.SetEdge("a", "b", Attrs{"weight": float64(2)})
		g.SetEdge("a", "b", Attrs{"weight": float64(3)}, "multi")
		lg := buildLayerGraph(g, 2, "inEdges")
		if got := num(asAttrs(lg.EdgeByArgs("a", "b")), "weight"); got != 5 {
			t.Fatalf("collapsed edge weight = %v, want 5", got)
		}
	})

	t.Run("preserves hierarchy and sparse borders", func(t *testing.T) {
		g := newGraph()
		for _, v := range []string{"a", "b", "c"} {
			g.SetNode(v, Attrs{"rank": float64(2)})
		}
		g.SetNode("sg", Attrs{
			"minRank":     float64(2),
			"maxRank":     float64(2),
			"borderLeft":  map[int]string{2: "bl"},
			"borderRight": map[int]string{2: "br"},
		})
		for _, v := range []string{"a", "b"} {
			if err := g.SetParent(v, "sg"); err != nil {
				t.Fatal(err)
			}
		}
		lg := buildLayerGraph(g, 2, "inEdges")
		root := stringValue(asAttrs(lg.Graph()), "root")
		children := lg.Children(root)
		sort.Strings(children)
		if want := []string{"c", "sg"}; !reflect.DeepEqual(children, want) {
			t.Fatalf("root children = %#v, want %#v", children, want)
		}
		if parent, ok := lg.Parent("a"); !ok || parent != "sg" {
			t.Fatalf("parent(a) = %q, %v; want sg, true", parent, ok)
		}
		label := asAttrs(lg.Node("sg"))
		if stringValue(label, "borderLeft") != "bl" || stringValue(label, "borderRight") != "br" {
			t.Fatalf("sparse borders were not selected: %#v", label)
		}
	})
}

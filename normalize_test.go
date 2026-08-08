package dagro

import (
	"reflect"
	"sort"
	"testing"
)

func newNormalizeTestGraph() *Graph {
	return NewGraph(GraphOptions{Multigraph: true, Compound: true}).SetGraph(Attrs{})
}

func TestNormalizeShortAndLongEdges(t *testing.T) {
	t.Run("short edge unchanged", func(t *testing.T) {
		g := newNormalizeTestGraph().
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(1)}).
			SetEdge("a", "b", Attrs{})
		runNormalize(g)
		if got := g.Edges(); !reflect.DeepEqual(got, []Edge{{V: "a", W: "b"}}) {
			t.Fatalf("short edge changed: %#v", got)
		}
		if chains := asAttrs(g.Graph())["dummyChains"].([]string); len(chains) != 0 {
			t.Fatalf("short edge created chains: %v", chains)
		}
	})

	t.Run("splits long edge and records chain", func(t *testing.T) {
		g := newNormalizeTestGraph().
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(2)}).
			SetEdge("a", "b", Attrs{"weight": float64(2), "width": float64(10), "height": float64(10)})
		runNormalize(g)
		if len(g.Successors("a")) != 1 {
			t.Fatalf("successors(a) = %v", g.Successors("a"))
		}
		dummy := g.Successors("a")[0]
		label := asAttrs(g.Node(dummy))
		if stringValue(label, "dummy") != "edge" || num(label, "rank") != 1 ||
			num(label, "width") != 0 || num(label, "height") != 0 {
			t.Fatalf("dummy label = %#v", label)
		}
		if got := g.Successors(dummy); !reflect.DeepEqual(got, []string{"b"}) {
			t.Fatalf("dummy successors = %v", got)
		}
		if num(asAttrs(g.EdgeByArgs("a", dummy)), "weight") != 2 {
			t.Fatalf("segment weight not preserved: %#v", g.EdgeByArgs("a", dummy))
		}
		if chains := asAttrs(g.Graph())["dummyChains"].([]string); !reflect.DeepEqual(chains, []string{dummy}) {
			t.Fatalf("dummyChains = %v", chains)
		}
	})

	t.Run("missing label rank does not create an edge-label dummy", func(t *testing.T) {
		g := newNormalizeTestGraph().
			SetNode("a", Attrs{"rank": float64(-1)}).
			SetNode("b", Attrs{"rank": float64(1)}).
			SetEdge("a", "b", Attrs{"weight": float64(1), "width": float64(20), "height": float64(10)})
		runNormalize(g)
		dummy := g.Successors("a")[0]
		if got := stringValue(asAttrs(g.Node(dummy)), "dummy"); got != "edge" {
			t.Fatalf("dummy type = %q, want edge", got)
		}
	})

	t.Run("edge label rank gets dimensions", func(t *testing.T) {
		g := newNormalizeTestGraph().
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("b", Attrs{"rank": float64(4)}).
			SetEdge("a", "b", Attrs{
				"width": float64(20), "height": float64(10), "labelRank": float64(2),
			})
		runNormalize(g)
		labelV := g.Successors(g.Successors("a")[0])[0]
		label := asAttrs(g.Node(labelV))
		if stringValue(label, "dummy") != "edge-label" ||
			num(label, "width") != 20 || num(label, "height") != 10 {
			t.Fatalf("edge-label dummy = %#v", label)
		}
	})
}

func TestUndoNormalizeRestoresLabelsAndPoints(t *testing.T) {
	g := newNormalizeTestGraph().
		SetNode("a", Attrs{"rank": float64(0)}).
		SetNode("b", Attrs{"rank": float64(4)}).
		SetEdge("a", "b", Attrs{
			"foo": "bar", "width": float64(10), "height": float64(20), "labelRank": float64(2),
		})
	runNormalize(g)

	v := g.Successors("a")[0]
	for i, point := range []Point{{X: 5, Y: 10}, {X: 20, Y: 25}, {X: 100, Y: 200}} {
		label := asAttrs(g.Node(v))
		label["x"], label["y"] = point.X, point.Y
		if i == 1 {
			label["width"], label["height"] = float64(20), float64(10)
		}
		v = g.Successors(v)[0]
	}
	undoNormalize(g)

	if got := g.Edges(); !reflect.DeepEqual(got, []Edge{{V: "a", W: "b"}}) {
		t.Fatalf("restored edges = %#v", got)
	}
	label := asAttrs(g.EdgeByArgs("a", "b"))
	if stringValue(label, "foo") != "bar" || num(label, "x") != 20 || num(label, "y") != 25 ||
		num(label, "width") != 20 || num(label, "height") != 10 {
		t.Fatalf("restored edge label = %#v", label)
	}
	wantPoints := []Point{{X: 5, Y: 10}, {X: 20, Y: 25}, {X: 100, Y: 200}}
	if got := label["points"].([]Point); !reflect.DeepEqual(got, wantPoints) {
		t.Fatalf("points = %+v, want %+v", got, wantPoints)
	}
}

func TestUndoNormalizeRestoresNamedMultiedges(t *testing.T) {
	g := newNormalizeTestGraph().
		SetNode("a", Attrs{"rank": float64(0)}).
		SetNode("b", Attrs{"rank": float64(2)})
	g.SetEdge("a", "b", Attrs{}, "bar")
	g.SetEdge("a", "b", Attrs{}, "foo")
	runNormalize(g)

	edges := g.OutEdges("a")
	sort.Slice(edges, func(i, j int) bool { return edges[i].Name < edges[j].Name })
	if len(edges) != 2 {
		t.Fatalf("normalized out edges = %#v", edges)
	}
	bar := asAttrs(g.Node(edges[0].W))
	bar["x"], bar["y"] = float64(5), float64(10)
	foo := asAttrs(g.Node(edges[1].W))
	foo["x"], foo["y"] = float64(15), float64(20)
	undoNormalize(g)

	if g.HasEdge("a", "b") || !g.HasEdge("a", "b", "bar") || !g.HasEdge("a", "b", "foo") {
		t.Fatalf("restored edge identities = %#v", g.Edges())
	}
	if got := asAttrs(g.EdgeByArgs("a", "b", "bar"))["points"].([]Point); !reflect.DeepEqual(got, []Point{{X: 5, Y: 10}}) {
		t.Fatalf("bar points = %+v", got)
	}
	if got := asAttrs(g.EdgeByArgs("a", "b", "foo"))["points"].([]Point); !reflect.DeepEqual(got, []Point{{X: 15, Y: 20}}) {
		t.Fatalf("foo points = %+v", got)
	}
}

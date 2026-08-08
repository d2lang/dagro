package dagro

import "testing"

func TestDebugOrderingUpstreamStructure(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true}).SetGraph(Attrs{})
	g.SetNode("a", Attrs{"rank": 0.0, "order": 0.0})
	g.SetNode("b", Attrs{"rank": 0.0, "order": 1.0})
	g.SetNode("c", Attrs{"rank": 1.0, "order": 0.0})
	g.SetEdge("a", "c", Attrs{"weight": 7.0}, "named")
	g.SetEdge("b", "c", Attrs{"weight": 3.0})

	h := DebugOrdering(g)
	if !h.IsCompound() || !h.IsMultigraph() {
		t.Fatalf("debug graph options: compound=%v multigraph=%v", h.IsCompound(), h.IsMultigraph())
	}
	if _, ok := h.Graph().(Attrs); !ok {
		t.Fatalf("debug graph label has type %T, want Attrs", h.Graph())
	}
	for _, v := range []string{"a", "b", "c"} {
		if got := stringValue(asAttrs(h.Node(v)), "label"); got != v {
			t.Errorf("node %q label = %q, want %q", v, got, v)
		}
		wantParent := "layer0"
		if v == "c" {
			wantParent = "layer1"
		}
		if got, ok := h.Parent(v); !ok || got != wantParent {
			t.Errorf("parent(%q) = (%q, %v), want (%q, true)", v, got, ok, wantParent)
		}
	}
	for _, layer := range []string{"layer0", "layer1"} {
		if got := stringValue(asAttrs(h.Node(layer)), "rank"); got != "same" {
			t.Errorf("%s rank = %q, want same", layer, got)
		}
	}
	if !h.HasEdge("a", "c", "named") {
		t.Fatal("named source edge was not preserved")
	}
	if got := asAttrs(h.EdgeByArgs("a", "c", "named")); len(got) != 0 {
		t.Errorf("source edge label = %#v, want empty Attrs", got)
	}
	if got := stringValue(asAttrs(h.EdgeByArgs("a", "b")), "style"); got != "invis" {
		t.Errorf("ordering edge style = %q, want invis", got)
	}
	if !h.HasEdge("b", "c") {
		t.Fatal("unnamed source edge was not preserved")
	}
}

func TestDebugOrderingEmptyGraph(t *testing.T) {
	h := DebugOrdering(NewGraph().SetGraph(Attrs{}))
	if h.NodeCount() != 0 || h.EdgeCount() != 0 {
		t.Fatalf("empty debug graph has %d nodes and %d edges", h.NodeCount(), h.EdgeCount())
	}
}

func TestDebugOrderingPreservesUndefinedRankString(t *testing.T) {
	g := NewGraph().SetGraph(Attrs{})
	g.SetNode("missing", Attrs{})
	h := DebugOrdering(g)
	if got, ok := h.Parent("missing"); !ok || got != "layerundefined" {
		t.Errorf("parent(missing) = (%q, %v), want (layerundefined, true)", got, ok)
	}
}

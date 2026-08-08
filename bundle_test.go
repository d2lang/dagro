package dagro

import "testing"

// TestBundle ports Dagre's browser-bundle smoke test to the Go package API.
func TestBundle(t *testing.T) {
	t.Run("exports dagro", func(t *testing.T) {
		if Version == "" {
			t.Fatal("Version is empty")
		}
		if NewGraph().IsDirected() != true {
			t.Fatal("NewGraph is unavailable or has invalid defaults")
		}
	})

	t.Run("can do trivial layout", func(t *testing.T) {
		g := NewGraph().SetGraph(Attrs{})
		g.SetNode("a", Attrs{"label": "a", "width": float64(50), "height": float64(100)})
		g.SetNode("b", Attrs{"label": "b", "width": float64(50), "height": float64(100)})
		g.SetEdge("a", "b", Attrs{"label": "ab", "width": float64(50), "height": float64(100)})

		if err := Layout(g); err != nil {
			t.Fatal(err)
		}
		for _, v := range []string{"a", "b"} {
			node := asAttrs(g.Node(v))
			if !has(node, "x") || !has(node, "y") || num(node, "x") < 0 || num(node, "y") < 0 {
				t.Fatalf("node %q lacks non-negative coordinates: %#v", v, node)
			}
		}
		edge := asAttrs(g.EdgeByArgs("a", "b"))
		if !has(edge, "x") || !has(edge, "y") || num(edge, "x") < 0 || num(edge, "y") < 0 {
			t.Fatalf("edge lacks non-negative label coordinates: %#v", edge)
		}
	})
}

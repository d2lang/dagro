package dagro

import (
	"reflect"
	"sort"
	"testing"
)

func TestAddSubgraphConstraints(t *testing.T) {
	t.Run("flat and contiguous nodes add no constraints", func(t *testing.T) {
		g := NewGraph(GraphOptions{Compound: true})
		cg := NewGraph()
		g.SetNodes([]string{"a", "b", "c", "d"})
		addSubgraphConstraints(g, cg, []string{"a", "b", "c", "d"})
		if cg.NodeCount() != 0 || cg.EdgeCount() != 0 {
			t.Fatalf("flat constraint graph has %d nodes and %d edges", cg.NodeCount(), cg.EdgeCount())
		}
		for _, v := range []string{"a", "b", "c"} {
			if err := g.SetParent(v, "sg"); err != nil {
				t.Fatal(err)
			}
		}
		addSubgraphConstraints(g, cg, []string{"a", "b", "c"})
		if cg.NodeCount() != 0 || cg.EdgeCount() != 0 {
			t.Fatalf("contiguous constraint graph has %d nodes and %d edges", cg.NodeCount(), cg.EdgeCount())
		}
	})

	t.Run("adjacent parents differ", func(t *testing.T) {
		g := NewGraph(GraphOptions{Compound: true})
		cg := NewGraph()
		if err := g.SetParent("a", "sg1"); err != nil {
			t.Fatal(err)
		}
		if err := g.SetParent("b", "sg2"); err != nil {
			t.Fatal(err)
		}
		addSubgraphConstraints(g, cg, []string{"a", "b"})
		want := []Edge{{V: "sg1", W: "sg2"}}
		if !reflect.DeepEqual(cg.Edges(), want) {
			t.Fatalf("edges = %#v, want %#v", cg.Edges(), want)
		}
	})

	t.Run("multiple levels", func(t *testing.T) {
		g := NewGraph(GraphOptions{Compound: true})
		cg := NewGraph()
		vs := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
		g.SetNodes(vs)
		parents := [][2]string{
			{"b", "sg2"}, {"sg2", "sg1"}, {"c", "sg1"},
			{"d", "sg3"}, {"sg3", "sg1"}, {"f", "sg4"},
			{"g", "sg5"}, {"sg5", "sg4"},
		}
		for _, pair := range parents {
			if err := g.SetParent(pair[0], pair[1]); err != nil {
				t.Fatal(err)
			}
		}
		addSubgraphConstraints(g, cg, vs)
		got := cg.Edges()
		sort.Slice(got, func(i, j int) bool { return got[i].V < got[j].V })
		want := []Edge{{V: "sg1", W: "sg4"}, {V: "sg2", W: "sg3"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("edges = %#v, want %#v", got, want)
		}
	})
}

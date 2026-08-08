package dagro

import (
	"reflect"
	"sort"
	"testing"
)

func TestInitOrder(t *testing.T) {
	t.Run("tree", func(t *testing.T) {
		g := newOrderTestGraph(true)
		for v, rank := range map[string]float64{"a": 0, "b": 1, "c": 2, "d": 2, "e": 1} {
			g.SetNode(v, Attrs{"rank": rank})
		}
		g.SetPath([]string{"a", "b", "c"})
		g.SetEdge("b", "d")
		g.SetEdge("a", "e")
		got := initOrder(g)
		for _, layer := range got {
			sort.Strings(layer)
		}
		want := [][]string{{"a"}, {"b", "e"}, {"c", "d"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("initOrder = %#v, want %#v", got, want)
		}
	})

	t.Run("DAG", func(t *testing.T) {
		g := newOrderTestGraph(true)
		// Set explicitly to avoid making this test depend on Go map iteration.
		g.SetNode("a", Attrs{"rank": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(1)})
		g.SetNode("d", Attrs{"rank": float64(2)})
		g.SetPath([]string{"a", "b", "d"})
		g.SetPath([]string{"a", "c", "d"})
		got := initOrder(g)
		for _, layer := range got {
			sort.Strings(layer)
		}
		want := [][]string{{"a"}, {"b", "c"}, {"d"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("initOrder = %#v, want %#v", got, want)
		}
	})

	t.Run("subgraph nodes are omitted", func(t *testing.T) {
		g := newOrderTestGraph(true)
		g.SetNode("a", Attrs{"rank": float64(0)})
		g.SetNode("sg1", Attrs{})
		if err := g.SetParent("a", "sg1"); err != nil {
			t.Fatal(err)
		}
		got := initOrder(g)
		want := [][]string{{"a"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("initOrder = %#v, want %#v", got, want)
		}
	})
}

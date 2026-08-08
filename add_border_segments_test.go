package dagro

import (
	"reflect"
	"testing"
)

func TestAddBorderSegmentsNoClusters(t *testing.T) {
	for _, g := range []*Graph{
		NewGraph().SetNode("a", Attrs{"rank": float64(0)}),
		NewGraph(GraphOptions{Compound: true}).SetNode("a", Attrs{"rank": float64(0)}),
	} {
		addBorderSegments(g)
		if g.NodeCount() != 1 || !reflect.DeepEqual(g.Node("a"), Attrs{"rank": float64(0)}) {
			t.Fatalf("border nodes added without cluster: nodes=%v a=%#v", g.Nodes(), g.Node("a"))
		}
	}
}

func checkSubgraphBorder(t *testing.T, g *Graph, subgraph, id, borderType string, rank float64) {
	t.Helper()
	want := Attrs{
		"dummy": "border", "borderType": borderType,
		"rank": rank, "width": float64(0), "height": float64(0),
	}
	if got := asAttrs(g.Node(id)); !reflect.DeepEqual(got, want) {
		t.Fatalf("border %s = %#v, want %#v", id, got, want)
	}
	if parent, ok := g.Parent(id); !ok || parent != subgraph {
		t.Fatalf("Parent(%s) = %q, %v, want %q", id, parent, ok, subgraph)
	}
}

func TestAddBorderSegmentsSingleAndMultipleRanks(t *testing.T) {
	t.Run("single rank", func(t *testing.T) {
		g := NewGraph(GraphOptions{Compound: true}).
			SetNode("sg", Attrs{"minRank": float64(1), "maxRank": float64(1)})
		addBorderSegments(g)
		sg := asAttrs(g.Node("sg"))
		left := sg["borderLeft"].(map[int]string)[1]
		right := sg["borderRight"].(map[int]string)[1]
		checkSubgraphBorder(t, g, "sg", left, "borderLeft", 1)
		checkSubgraphBorder(t, g, "sg", right, "borderRight", 1)
	})

	t.Run("multiple ranks", func(t *testing.T) {
		g := NewGraph(GraphOptions{Compound: true}).
			SetNode("sg", Attrs{"minRank": float64(1), "maxRank": float64(2)})
		addBorderSegments(g)
		sg := asAttrs(g.Node("sg"))
		left := sg["borderLeft"].(map[int]string)
		right := sg["borderRight"].(map[int]string)
		for rank := 1; rank <= 2; rank++ {
			checkSubgraphBorder(t, g, "sg", left[rank], "borderLeft", float64(rank))
			checkSubgraphBorder(t, g, "sg", right[rank], "borderRight", float64(rank))
		}
		if !g.HasEdge(left[1], left[2]) || !g.HasEdge(right[1], right[2]) {
			t.Fatalf("border chains missing: edges=%#v", g.Edges())
		}
		if num(asAttrs(g.EdgeByArgs(left[1], left[2])), "weight") != 1 ||
			num(asAttrs(g.EdgeByArgs(right[1], right[2])), "weight") != 1 {
			t.Fatalf("border chain weights wrong")
		}
	})
}

func TestAddBorderSegmentsNestedSubgraphs(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true}).
		SetNode("sg1", Attrs{"minRank": float64(1), "maxRank": float64(1)}).
		SetNode("sg2", Attrs{"minRank": float64(1), "maxRank": float64(1)})
	if err := g.SetParent("sg2", "sg1"); err != nil {
		t.Fatal(err)
	}
	addBorderSegments(g)
	for _, sgID := range []string{"sg1", "sg2"} {
		sg := asAttrs(g.Node(sgID))
		left := sg["borderLeft"].(map[int]string)[1]
		right := sg["borderRight"].(map[int]string)[1]
		checkSubgraphBorder(t, g, sgID, left, "borderLeft", 1)
		checkSubgraphBorder(t, g, sgID, right, "borderRight", 1)
	}
}

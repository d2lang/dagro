package dagro

import (
	"reflect"
	"testing"
)

func TestGraphDefaultsAndOptions(t *testing.T) {
	g := NewGraph()
	if !g.IsDirected() || g.IsMultigraph() || g.IsCompound() {
		t.Fatalf("default options: directed=%v multigraph=%v compound=%v",
			g.IsDirected(), g.IsMultigraph(), g.IsCompound())
	}
	if g.NodeCount() != 0 || g.EdgeCount() != 0 || g.Graph() != nil {
		t.Fatalf("non-empty initial state: nodes=%d edges=%d label=%#v",
			g.NodeCount(), g.EdgeCount(), g.Graph())
	}

	u := NewGraph(GraphOptions{Undirected: true, Multigraph: true, Compound: true})
	if u.IsDirected() || !u.IsMultigraph() || !u.IsCompound() {
		t.Fatalf("configured options: directed=%v multigraph=%v compound=%v",
			u.IsDirected(), u.IsMultigraph(), u.IsCompound())
	}
	label := Attrs{"name": "graph"}
	if got := u.SetGraph(label); got != u || !reflect.DeepEqual(u.Graph(), label) {
		t.Fatalf("SetGraph chain/value mismatch: %#v", u.Graph())
	}
}

func TestGraphNodeOrderMatchesJavaScriptObjectKeys(t *testing.T) {
	g := NewGraph()
	for _, v := range []string{"10", "2", "alpha", "1", "beta", "01"} {
		g.SetNode(v, Attrs{"id": v})
	}
	want := []string{"1", "2", "10", "alpha", "beta", "01"}
	if got := g.Nodes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Nodes() = %v, want %v", got, want)
	}

	// Updating a node does not move it. Removing and re-adding a non-index key
	// moves it to the end, as deleting/recreating an object property does in JS.
	g.SetNode("alpha", Attrs{"updated": true})
	if got := g.Nodes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("updated Nodes() = %v, want %v", got, want)
	}
	g.RemoveNode("alpha").SetNode("alpha")
	want = []string{"1", "2", "10", "beta", "01", "alpha"}
	if got := g.Nodes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reinserted Nodes() = %v, want %v", got, want)
	}
}

func TestGraphNodeDefaultsAndRemoval(t *testing.T) {
	g := NewGraph().SetDefaultNodeLabel(func(v string) any { return Attrs{"id": v} })
	g.SetNode("a").SetNode("b", Attrs{"explicit": true}).SetEdge("a", "b", Attrs{})
	if got := stringValue(asAttrs(g.Node("a")), "id"); got != "a" {
		t.Fatalf("default node label id = %q", got)
	}
	if !boolValue(asAttrs(g.Node("b")), "explicit") {
		t.Fatalf("explicit node label overwritten: %#v", g.Node("b"))
	}
	g.SetNode("a")
	if got := stringValue(asAttrs(g.Node("a")), "id"); got != "a" {
		t.Fatalf("one-arg SetNode changed label: %#v", g.Node("a"))
	}
	g.RemoveNode("a")
	if g.HasNode("a") || g.EdgeCount() != 0 || len(g.Predecessors("b")) != 0 {
		t.Fatalf("RemoveNode left state: has=%v edges=%d preds=%v",
			g.HasNode("a"), g.EdgeCount(), g.Predecessors("b"))
	}
}

func TestGraphConstantDefaultLabels(t *testing.T) {
	nodeDefault := Attrs{"kind": "node"}
	edgeDefault := Attrs{"kind": "edge"}
	g := NewGraph(GraphOptions{Multigraph: true}).
		SetDefaultNodeLabel(nodeDefault).
		SetDefaultEdgeLabel(edgeDefault)
	g.SetNode("a").SetNode("b").SetEdge("a", "b")
	if !reflect.DeepEqual(g.Node("a"), nodeDefault) || !reflect.DeepEqual(g.Node("b"), nodeDefault) {
		t.Fatalf("constant node default not applied: a=%#v b=%#v", g.Node("a"), g.Node("b"))
	}
	if !reflect.DeepEqual(g.EdgeByArgs("a", "b"), edgeDefault) {
		t.Fatalf("constant edge default not applied: %#v", g.EdgeByArgs("a", "b"))
	}
}

func TestGraphDefaultLabelCallbacksAcceptNaturalGoSignatures(t *testing.T) {
	type nodeLabeler func(string) Attrs
	g := NewGraph(GraphOptions{Multigraph: true}).
		SetDefaultNodeLabel(nodeLabeler(func(v string) Attrs { return Attrs{"id": v} })).
		SetDefaultEdgeLabel(func(v, w, name string) Attrs {
			return Attrs{"id": v + "-" + w + "-" + name}
		})
	g.SetNode("a").SetEdgeObject(Edge{V: "a", W: "b", Name: "named", HasName: true})
	if got := stringValue(asAttrs(g.Node("a")), "id"); got != "a" {
		t.Fatalf("named node callback label = %q, want a", got)
	}
	if got := stringValue(asAttrs(g.EdgeByArgs("a", "b", "named")), "id"); got != "a-b-named" {
		t.Fatalf("natural edge callback label = %q, want a-b-named", got)
	}

	zeroArg := NewGraph().SetDefaultNodeLabel(func() Attrs { return Attrs{"zero": true} })
	zeroArg.SetNode("x")
	if !boolValue(asAttrs(zeroArg.Node("x")), "zero") {
		t.Fatalf("zero-argument callback label = %#v", zeroArg.Node("x"))
	}

	var nilCallback func(string) Attrs
	nilGraph := NewGraph().SetDefaultNodeLabel(nilCallback).SetNode("x")
	if nilGraph.Node("x") != nil {
		t.Fatalf("typed-nil callback label = %#v, want nil", nilGraph.Node("x"))
	}
}

func TestGraphCompoundRelationships(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true})
	if err := g.SetParent("child", "parent"); err != nil {
		t.Fatal(err)
	}
	if err := g.SetParent("grandchild", "child"); err != nil {
		t.Fatal(err)
	}
	if got, ok := g.Parent("child"); !ok || got != "parent" {
		t.Fatalf("Parent(child) = %q, %v", got, ok)
	}
	if got := g.Children("parent"); !reflect.DeepEqual(got, []string{"child"}) {
		t.Fatalf("Children(parent) = %v", got)
	}
	if err := g.SetParent("parent", "grandchild"); err == nil {
		t.Fatal("SetParent allowed a compound cycle")
	}

	if err := g.SetParent("child"); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Parent("child"); ok {
		t.Fatal("child still has a parent after SetParent(child)")
	}
	if got := g.Children(); !reflect.DeepEqual(got, []string{"parent", "child"}) {
		t.Fatalf("root children = %v", got)
	}

	if err := g.SetParent("child", "parent"); err != nil {
		t.Fatal(err)
	}
	g.RemoveNode("child")
	if got, ok := g.Parent("grandchild"); ok || got != "" {
		t.Fatalf("removed parent's child was not promoted: %q, %v", got, ok)
	}
	if got := g.Children(); !reflect.DeepEqual(got, []string{"parent", "grandchild"}) {
		t.Fatalf("root children after removal = %v", got)
	}

	if err := NewGraph().SetParent("a", "b"); err == nil {
		t.Fatal("SetParent on a non-compound graph succeeded")
	}
}

func TestGraphSetParentDetectsCycleThroughEmptyStringNode(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true})
	if err := g.SetParent("a", ""); err != nil {
		t.Fatal(err)
	}
	if err := g.SetParent("", "a"); err == nil {
		t.Fatal("SetParent allowed a cycle through the empty-string node")
	}
}

func TestGraphFilterNodesPreservesEdgesAndNearestCompoundAncestor(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"name": "g"})
	g.SetNode("outer", Attrs{"kind": "outer"})
	g.SetNode("inner", Attrs{"kind": "inner"})
	g.SetNode("a", Attrs{"kind": "leaf"})
	g.SetNode("b", Attrs{"kind": "leaf"})
	if err := g.SetParent("inner", "outer"); err != nil {
		t.Fatal(err)
	}
	if err := g.SetParent("a", "inner"); err != nil {
		t.Fatal(err)
	}
	g.SetEdge("a", "b", Attrs{"kind": "kept"}, "named")
	g.SetEdge("a", "inner", Attrs{"kind": "removed"})

	filtered := g.FilterNodes(func(v string) bool { return v != "inner" })
	if filtered.IsDirected() != g.IsDirected() || !filtered.IsCompound() || !filtered.IsMultigraph() {
		t.Fatalf("filtered options differ: directed=%v compound=%v multigraph=%v",
			filtered.IsDirected(), filtered.IsCompound(), filtered.IsMultigraph())
	}
	if parent, ok := filtered.Parent("a"); !ok || parent != "outer" {
		t.Fatalf("filtered parent(a) = %q, %v; want outer", parent, ok)
	}
	if !filtered.HasEdge("a", "b", "named") || filtered.EdgeCount() != 1 {
		t.Fatalf("filtered edges = %#v", filtered.Edges())
	}
	if !reflect.DeepEqual(filtered.Graph(), g.Graph()) {
		t.Fatalf("filtered graph label = %#v, want %#v", filtered.Graph(), g.Graph())
	}
}

func TestGraphSourcesSinksAndNeighborCounts(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true}).
		SetEdge("a", "b", Attrs{}, "one").
		SetEdge("a", "b", Attrs{}, "two").
		SetEdge("b", "c", Attrs{})
	if got := g.Sources(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("Sources() = %v", got)
	}
	if got := g.Sinks(); !reflect.DeepEqual(got, []string{"c"}) {
		t.Fatalf("Sinks() = %v", got)
	}
	if got := g.Successors("a"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("Successors(a) = %v", got)
	}
	g.RemoveEdgeByArgs("a", "b", "one")
	if got := g.Successors("a"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("removing one multiedge removed neighbor: %v", got)
	}
	g.RemoveEdgeByArgs("a", "b", "two")
	if len(g.Successors("a")) != 0 || len(g.Predecessors("b")) != 0 {
		t.Fatalf("last multiedge left neighbor counts: sucs=%v preds=%v",
			g.Successors("a"), g.Predecessors("b"))
	}
}

func TestGraphNamedMultiedgesAndOrder(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true}).
		SetDefaultEdgeLabel(func(v, w string, name *string) any {
			label := Attrs{"v": v, "w": w}
			if name != nil {
				label["name"] = *name
			}
			return label
		})

	g.SetEdge("a", "b", Attrs{"kind": "unnamed"})
	g.SetEdge("a", "b", Attrs{"kind": "foo"}, "foo")
	g.SetEdge("a", "b", Attrs{"kind": "empty"}, "")
	g.SetEdge("b", "a", Attrs{"kind": "reverse"}, "back")
	if g.EdgeCount() != 4 || len(g.OutEdges("a", "b")) != 3 {
		t.Fatalf("multiedge counts: edges=%d a->b=%d", g.EdgeCount(), len(g.OutEdges("a", "b")))
	}
	if got := stringValue(asAttrs(g.EdgeByArgs("a", "b", "")), "kind"); got != "empty" {
		t.Fatalf("empty named edge label = %q", got)
	}
	if got := stringValue(asAttrs(g.EdgeByArgs("a", "b")), "kind"); got != "unnamed" {
		t.Fatalf("unnamed edge label = %q", got)
	}

	want := []Edge{
		{V: "a", W: "b"},
		{V: "a", W: "b", Name: "foo", HasName: true},
		{V: "a", W: "b", Name: "", HasName: true},
		{V: "b", W: "a", Name: "back", HasName: true},
	}
	if got := g.Edges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Edges() = %#v, want %#v", got, want)
	}
	g.SetEdge("a", "b", Attrs{"kind": "updated"}, "foo")
	if got := g.Edges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("updating edge changed order: %#v", got)
	}
	g.RemoveEdgeByArgs("a", "b", "foo")
	g.SetEdge("a", "b", Attrs{"kind": "readded"}, "foo")
	want = []Edge{want[0], want[2], want[3], want[1]}
	if got := g.Edges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("re-added edge order = %#v, want %#v", got, want)
	}
}

func TestGraphSetEdgeObjectUsesNameAndDefault(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true}).
		SetDefaultEdgeLabel(func(v, w string, name *string) any {
			return Attrs{"name": *name}
		})
	e := Edge{V: "a", W: "b", Name: "named", HasName: true}
	g.SetEdgeObject(e)
	if !g.HasEdge("a", "b", "named") || g.HasEdge("a", "b") {
		t.Fatalf("SetEdgeObject created wrong identity: %#v", g.Edges())
	}
	if got := stringValue(asAttrs(g.Edge(e)), "name"); got != "named" {
		t.Fatalf("default edge label name = %q", got)
	}
}

func TestGraphEdgeNameUsesJavaScriptStringCoercion(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true})
	g.SetEdge("a", "b", Attrs{}, float64(1))
	if !g.HasEdge("a", "b", "1") {
		t.Fatalf("numeric edge name was not coerced like JavaScript: %#v", g.Edges())
	}
}

func TestGraphEmptyEndpointFilterMatchesGraphlibTruthiness(t *testing.T) {
	g := NewGraph().SetEdge("", "v", Attrs{"id": "empty"}).SetEdge("x", "v", Attrs{"id": "x"})
	if got := g.InEdges("v", ""); len(got) != 2 {
		t.Fatalf("InEdges(v, empty) = %#v, want all incoming edges", got)
	}
}

func TestGraphUndirectedEdges(t *testing.T) {
	g := NewGraph(GraphOptions{Undirected: true}).SetEdge("z", "a", Attrs{"ok": true})
	want := []Edge{{V: "a", W: "z"}}
	if got := g.Edges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("undirected edge identity = %#v", got)
	}
	if !g.HasEdge("z", "a") || !g.HasEdge("a", "z") || !boolValue(asAttrs(g.EdgeByArgs("z", "a")), "ok") {
		t.Fatalf("undirected lookup failed")
	}
	if got := g.Neighbors("a"); !reflect.DeepEqual(got, []string{"z"}) {
		t.Fatalf("Neighbors(a) = %v", got)
	}
}

func TestGraphUndirectedOrderingUsesJavaScriptUTF16(t *testing.T) {
	astral, privateUse := "😀", "\ue000"
	g := NewGraph(GraphOptions{Undirected: true}).SetEdge(privateUse, astral, Attrs{})
	want := []Edge{{V: astral, W: privateUse}}
	if got := g.Edges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("UTF-16 undirected identity = %#v, want %#v", got, want)
	}
}

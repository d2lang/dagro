package dagro

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// A label holding two spellings of an
// attribute key build reads makes the original's result depend on one draw of
// Go's map order (canonicalize keeps the spelling its range visits last). The
// typed build declines at such a label before it coerces any of its values,
// so that buildLayoutGraph makes the only draw.

// Spellings that strings.ToLower maps to a key build reads: ASCII upper case,
// U+0130 (capital I with dot above, lower-cases to "i") and U+212A (Kelvin
// sign, lower-cases to "k").
const (
	dottedI = "İ"
	kelvin  = "K"
)

func TestSpellingsLowerCaseToReadKeys(t *testing.T) {
	for spelling, want := range map[string]string{
		"W" + dottedI + "DTH":   "width",
		"marg" + dottedI + "nx": "marginx",
		"m" + dottedI + "nlen":  "minlen",
		"RAN" + kelvin + "DIR":  "rankdir",
	} {
		if got := strings.ToLower(spelling); got != want {
			t.Fatalf("strings.ToLower(%q) = %q, want %q", spelling, got, want)
		}
	}
}

// buildOutcome runs build on g in a fresh state and reports whether it
// accepted g and the value it panicked with, if any.
func buildOutcome(g *Graph) (accepted bool, panicked any) {
	defer func() { panicked = recover() }()
	return new(lstate).build(g), nil
}

// build must decline (return false) without coercing a label that holds two
// spellings of a key it reads: here every spelling holds a value number()
// rejects, so a coercion would panic. Before the fix build coerced the
// spelling its own range visited last and panicked.
func TestDuplicateSpellingDeclinesBeforeCoercion(t *testing.T) {
	bad := func() []int { return []int{1} }
	cases := []struct {
		name  string
		graph Attrs
		node  Attrs
		edge  Attrs
	}{
		{name: "graph ascii", graph: Attrs{"nodesep": bad(), "NodeSep": bad()}},
		{name: "graph dotted I", graph: Attrs{"marginx": bad(), "MARG" + dottedI + "NX": bad()}},
		{name: "node ascii", node: Attrs{"width": bad(), "WIDTH": bad(), "height": 3.0}},
		{name: "node dotted I", node: Attrs{"W" + dottedI + "DTH": bad(), "Width": bad()}},
		{name: "node three spellings", node: Attrs{"height": bad(), "Height": bad(), "HEIGHT": bad()}},
		{name: "edge ascii", edge: Attrs{"weight": bad(), "Weight": bad()}},
		{name: "edge dotted I", edge: Attrs{"minlen": bad(), "M" + dottedI + "NLEN": bad()}},
	}
	for _, c := range cases {
		for trial := 0; trial < 50; trial++ {
			graph := c.graph
			if graph == nil {
				graph = Attrs{}
			}
			g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(graph)
			g.SetNode("a", Attrs{"width": 10.0, "height": 10.0})
			if c.node != nil {
				g.SetNode("b", c.node)
			}
			edge := c.edge
			if edge == nil {
				edge = Attrs{}
			}
			g.SetEdge("a", "b", edge)
			accepted, panicked := buildOutcome(g)
			if panicked != nil || accepted {
				t.Fatalf("%s: build accepted=%v panicked=%v; want a decline without coercion", c.name, accepted, panicked)
			}
		}
	}
}

// A single non-ASCII spelling of a key is no draw at all: the typed engine
// keeps the graph and lays it out as the original does.
func TestSingleNonASCIISpellingStaysTyped(t *testing.T) {
	build := func() (*Graph, *Graph) {
		g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"RAN" + kelvin + "DIR": "LR", "marg" + dottedI + "nx": 7.0})
		o := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"RAN" + kelvin + "DIR": "LR", "marg" + dottedI + "nx": 7.0})
		g.SetNode("a", Attrs{"W" + dottedI + "DTH": 30.0, "height": 12.0})
		o.SetNode("a", Attrs{"W" + dottedI + "DTH": 30.0, "height": 12.0})
		g.SetNode("b", Attrs{"width": 8.0, "height": 9.0})
		o.SetNode("b", Attrs{"width": 8.0, "height": 9.0})
		g.SetEdge("a", "b", Attrs{"M" + dottedI + "NLEN": 3.0, "w" + dottedI + "dth": 5.0, "height": 4.0})
		o.SetEdge("a", "b", Attrs{"M" + dottedI + "NLEN": 3.0, "w" + dottedI + "dth": 5.0, "height": 4.0})
		return g, o
	}
	g, o := build()
	if accepted, panicked := buildOutcome(g); !accepted || panicked != nil {
		t.Fatalf("build accepted=%v panicked=%v; want the typed engine", accepted, panicked)
	}
	if err := Layout(g); err != nil {
		t.Fatal(err)
	}
	if err := referenceLayoutOpts(o); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(g.Graph(), g.Node("a"), g.Node("b"), g.Edge(Edge{V: "a", W: "b"}))
	want := fmt.Sprint(o.Graph(), o.Node("a"), o.Node("b"), o.Edge(Edge{V: "a", W: "b"}))
	if got != want {
		t.Fatalf("layout differs:\nnew  %s\norig %s", got, want)
	}
}

// The graph label with two spellings of marginx, one of which number()
// rejects, and an edge with an infinite minlen, on which the typed engine
// declines during run (its order phase does not take such ranks). The
// original draws the spelling once: "expected number" with the frequency q
// of the bad spelling winning, otherwise its makeslice panic in order. Before
// the fix the decline drew a second time: q + (1-q)q.
func TestGraphLabelSpellingDrawnOnce(t *testing.T) {
	const trials = 4000
	label := func() map[string]any {
		m := make(map[string]any, 2)
		m["marginx"] = 10.0
		m["MARG"+dottedI+"NX"] = []int{1}
		return m
	}
	classify := func(p any) string {
		if p == nil {
			return "no panic"
		}
		if strings.Contains(fmt.Sprint(p), "expected number") {
			return "coercion panic"
		}
		return "other panic"
	}
	count := func(layout func() any) map[string]int {
		c := map[string]int{}
		for i := 0; i < trials; i++ {
			var p any
			func() {
				defer func() { p = recover() }()
				p = layout()
			}()
			c[classify(p)]++
		}
		return c
	}
	want := count(func() any {
		g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs(label()))
		g.SetEdge("a", "b", Attrs{"minlen": math.Inf(1)})
		return referenceLayoutOpts(g)
	})
	got := count(func() any {
		g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs(label()))
		g.SetEdge("a", "b", Attrs{"minlen": math.Inf(1)})
		return Layout(g)
	})
	t.Logf("original %v", want)
	t.Logf("new      %v", got)
	if want["no panic"] != 0 || got["no panic"] != 0 {
		t.Fatalf("a layout succeeded: original %v, new %v", want, got)
	}
	pw := float64(want["coercion panic"]) / trials
	pg := float64(got["coercion panic"]) / trials
	if pw < 0.05 || pw > 0.95 {
		t.Fatalf("the original's draw is too one-sided to compare: %.3f", pw)
	}
	if math.Abs(pw-pg) > 0.05 {
		t.Errorf("coercion panic frequency differs: original %.3f, new %.3f (two draws: %.3f)", pw, pg, pw+(1-pw)*pw)
	}
}

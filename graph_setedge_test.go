package dagro

import (
	"fmt"
	"math/rand"
	"testing"
)

// SetEdge, SetEdgeObject and LayoutInput.Graph share one implementation,
// setEdge. Before that, SetEdge held the logic, SetEdgeObject called it with
// the edge's name as its fourth argument, and an internal setEdge repeated
// the body for a named edge without a label. oldSetEdge, oldSetEdgeObject
// and oldSetEdgeInternal are those three functions as they were (they use
// only Graph's state), and LayoutInput.Graph's edge loop is oldGraphEdges.

func oldSetEdge(g *Graph, v, w string, args ...any) *Graph {
	var value any
	valueSpecified := len(args) > 0
	if valueSpecified {
		value = args[0]
	}
	var name *string
	if len(args) > 1 {
		n := jsConcatString(args[1])
		name = &n
	}
	id := edgeArgsToID(g.directed, v, w, name)
	if _, ok := g.edgeLabels[id]; ok {
		if valueSpecified {
			g.edgeLabels[id] = value
		}
		return g
	}
	if name != nil && !g.multigraph {
		panic("dagro: cannot set a named edge when Multigraph is false")
	}
	g.SetNode(v)
	g.SetNode(w)
	if valueSpecified {
		g.edgeLabels[id] = value
	} else {
		g.edgeLabels[id] = g.defaultEdgeLabel(v, w, name)
	}
	e := edgeArgsToObj(g.directed, v, w, name)
	g.edgeObjs.set(id, e)
	g.ensureEdgeEndpointMaps(e.V, e.W)
	g.preds[e.W].inc(e.V)
	g.sucs[e.V].inc(e.W)
	g.in[e.W].set(id, e)
	g.out[e.V].set(id, e)
	return g
}

func oldSetEdgeObject(g *Graph, e Edge, value ...any) *Graph {
	if len(value) > 0 {
		if e.HasName {
			return oldSetEdge(g, e.V, e.W, value[0], e.Name)
		}
		return oldSetEdge(g, e.V, e.W, value[0])
	}
	if e.HasName {
		return oldSetEdgeInternal(g, e.V, e.W, nil, false, &e.Name)
	}
	return oldSetEdge(g, e.V, e.W)
}

func oldSetEdgeInternal(g *Graph, v, w string, value any, valueSpecified bool, name *string) *Graph {
	args := []any{}
	if valueSpecified {
		args = append(args, value)
	}
	if name != nil {
		if !valueSpecified {
			id := edgeArgsToID(g.directed, v, w, name)
			if _, ok := g.edgeLabels[id]; ok {
				return g
			}
			if !g.multigraph {
				panic("dagro: cannot set a named edge when Multigraph is false")
			}
			g.SetNode(v)
			g.SetNode(w)
			g.edgeLabels[id] = g.defaultEdgeLabel(v, w, name)
			e := edgeArgsToObj(g.directed, v, w, name)
			g.edgeObjs.set(id, e)
			g.ensureEdgeEndpointMaps(e.V, e.W)
			g.preds[e.W].inc(e.V)
			g.sucs[e.V].inc(e.W)
			g.in[e.W].set(id, e)
			g.out[e.V].set(id, e)
			return g
		}
		args = append(args, *name)
	}
	return oldSetEdge(g, v, w, args...)
}

// graphState is everything observable about g through its edge and node
// operations, in order.
func graphState(g *Graph) string {
	s := dumpGraph(g)
	for _, v := range g.Nodes() {
		s += fmt.Sprintf("%q: in %+q out %+q preds %q sucs %q neighbors %q nodeEdges %+q\n",
			v, fmt.Sprintf("%+v", g.InEdges(v)), fmt.Sprintf("%+v", g.OutEdges(v)), g.Predecessors(v), g.Successors(v),
			g.Neighbors(v), fmt.Sprintf("%+v", g.NodeEdges(v)))
	}
	return s + fmt.Sprintf("counts %d %d sources %q sinks %q\n", g.NodeCount(), g.EdgeCount(), g.Sources(), g.Sinks())
}

// The same random calls through the old functions and the new ones build the
// same graph, step by step, with the same panics and the same default edge
// labels (the callback sees the same endpoints and name).
func TestSetEdgeMatchesOldImplementation(t *testing.T) {
	rng := rand.New(rand.NewSource(20261122))
	names := []any{"", "n", "\x00", "a\x01b", nil, 7, 2.5, true, int64(-3), uint8(4), "1e21", 1e21, float32(0.5)}
	nodes := []string{"a", "b", "", "0", "10", "2", "é", "a\x01b", "_d1"}
	for c := 0; c < 400; c++ {
		opts := GraphOptions{Multigraph: rng.Intn(4) != 0, Compound: rng.Intn(2) == 0, Undirected: rng.Intn(3) == 0}
		gOld, gNew := NewGraph(opts), NewGraph(opts)
		for _, g := range []*Graph{gOld, gNew} {
			if rng.Intn(2) == 0 {
				g.SetDefaultEdgeLabel(func(v, w string, name *string) any {
					if name == nil {
						return v + ">" + w
					}
					return v + ">" + w + ":" + *name
				})
			}
		}
		// The two graphs must take the same default: redo the choice.
		gNew.defaultEdgeLabel = gOld.defaultEdgeLabel
		for op := 0; op < 30; op++ {
			v, w := nodes[rng.Intn(len(nodes))], nodes[rng.Intn(len(nodes))]
			label := any(Attrs{"k": float64(op)})
			name := names[rng.Intn(len(names))]
			var callOld, callNew func()
			what := ""
			switch rng.Intn(7) {
			case 0:
				what = fmt.Sprintf("SetEdge(%q, %q)", v, w)
				callOld, callNew = func() { oldSetEdge(gOld, v, w) }, func() { gNew.SetEdge(v, w) }
			case 1:
				what = fmt.Sprintf("SetEdge(%q, %q, label)", v, w)
				callOld, callNew = func() { oldSetEdge(gOld, v, w, label) }, func() { gNew.SetEdge(v, w, label) }
			case 2:
				what = fmt.Sprintf("SetEdge(%q, %q, label, %#v)", v, w, name)
				callOld, callNew = func() { oldSetEdge(gOld, v, w, label, name) }, func() { gNew.SetEdge(v, w, label, name) }
			case 3:
				what = fmt.Sprintf("SetEdge(%q, %q, nil, %#v, extra)", v, w, name)
				callOld, callNew = func() { oldSetEdge(gOld, v, w, nil, name, "extra") }, func() { gNew.SetEdge(v, w, nil, name, "extra") }
			default:
				e := Edge{V: v, W: w}
				if s, ok := name.(string); ok {
					e.Name, e.HasName = s, true
				}
				if rng.Intn(2) == 0 {
					what = fmt.Sprintf("SetEdgeObject(%+v)", e)
					callOld, callNew = func() { oldSetEdgeObject(gOld, e) }, func() { gNew.SetEdgeObject(e) }
				} else {
					what = fmt.Sprintf("SetEdgeObject(%+v, label)", e)
					callOld, callNew = func() { oldSetEdgeObject(gOld, e, label) }, func() { gNew.SetEdgeObject(e, label) }
				}
			}
			pOld, pNew := panicValue(callOld), panicValue(callNew)
			if fmt.Sprint(pOld) != fmt.Sprint(pNew) {
				t.Fatalf("case %d op %d %s (%+v): old panic %v, new panic %v", c, op, what, opts, pOld, pNew)
			}
			if a, b := graphState(gOld), graphState(gNew); a != b {
				t.Fatalf("case %d op %d %s (%+v): graphs differ\nold:\n%s\nnew:\n%s", c, op, what, opts, a, b)
			}
			if rng.Intn(12) == 0 && gOld.EdgeCount() > 0 {
				e := gOld.Edges()[rng.Intn(gOld.EdgeCount())]
				gOld.RemoveEdge(e)
				gNew.RemoveEdge(e)
			}
		}
	}
}

// oldGraphEdges is the edge loop of LayoutInput.Graph as it was: SetEdge
// with the label and, when HasName, the name.
func oldGraphEdges(in *LayoutInput, g *Graph) error {
	for i, e := range in.Edges {
		if e.V < 0 || e.V >= len(in.Nodes) || e.W < 0 || e.W >= len(in.Nodes) {
			return fmt.Errorf("dagro: edge %d endpoint out of range", i)
		}
		label := Attrs{
			"minlen": e.Minlen, "weight": e.Weight, "width": e.Width, "height": e.Height,
			"labeloffset": e.LabelOffset, "labelpos": e.LabelPos,
		}
		if e.HasName {
			oldSetEdge(g, in.Nodes[e.V].ID, in.Nodes[e.W].ID, label, e.Name)
		} else {
			oldSetEdge(g, in.Nodes[e.V].ID, in.Nodes[e.W].ID, label)
		}
	}
	return nil
}

// LayoutInput.Graph builds the graph it built when its edges went through
// SetEdge, for regular inputs and for those with repeated ids, repeated
// edges, empty and odd names, and invalid indices.
func TestLayoutInputGraphMatchesOldImplementation(t *testing.T) {
	z := inputGenerator{newSpecGenerator(4711, "weirdlabels,weirdnums")}
	z.weirdLabels = true
	built, failed := 0, 0
	for c := 0; c < 3000; c++ {
		in := z.input(12)
		got, gotErr := in.Graph()
		// The old function: the same node loop, then the old edge loop.
		nodesOnly := in
		nodesOnly.Edges = nil
		want, wantErr := nodesOnly.Graph()
		if wantErr == nil {
			wantErr = oldGraphEdges(&in, want)
		}
		if (gotErr == nil) != (wantErr == nil) || gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Fatalf("case %d: error %v, old %v\ninput %+v", c, gotErr, wantErr, in)
		}
		if gotErr != nil {
			if got != nil {
				t.Fatalf("case %d: a graph came with the error %v", c, gotErr)
			}
			failed++
			continue
		}
		built++
		if a, b := graphState(want), graphState(got); a != b {
			t.Fatalf("case %d: graphs differ\nold:\n%s\nnew:\n%s\ninput %+v", c, a, b, in)
		}
	}
	if built < 1500 || failed < 20 {
		t.Fatalf("%d graphs built, %d inputs rejected: the generator no longer covers both", built, failed)
	}
}

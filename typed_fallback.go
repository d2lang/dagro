//go:build !dagro_typed_only

package dagro

// This file is the half of LayoutTyped that needs the Graph engine: what it
// does with an input the typed engine does not accept or gives up on.
// typed_only.go takes its place in a build that sets the dagro_typed_only tag
// (see LayoutTyped). It is the only code through which
// the typed API reaches Layout, the Attrs engine or anything else that works
// on a Graph beyond building one.

// graphFallback reports whether LayoutTyped falls back to the Graph engine.
const graphFallback = true

// typedGreedy reports whether the typed engine takes graphs with acyclicer
// "greedy": its greedyFAS, below, runs the Graph implementation.
const typedGreedy = true

// layoutNotTyped handles inputs the typed engine does not take (and invalid
// ones) through the equivalent Graph.
func layoutNotTyped(in *LayoutInput, out *LayoutOutput) error {
	return layoutTypedWith(in, out, func(g *Graph) error { return Layout(g) })
}

// layoutBuilt runs the typed engine on the state buildTyped made of in.
func layoutBuilt(s *lstate, in *LayoutInput, out *LayoutOutput) error {
	err, ok := s.runGuarded(s.run)
	if !ok {
		// The typed engine panicked or declined (see runGuarded): s is
		// dropped. Layout(in.Graph()) would build the same typed state, run
		// it into the same panic and hand the graph to the Attrs engine, so
		// that engine lays out in.Graph() directly, replaying the counter
		// values s drew.
		return layoutTypedViaAttrs(in, out, s.drawn)
	}
	return s.finishTyped(in, out, err)
}

// layoutTypedViaAttrs lays out the equivalent Graph with the Attrs engine,
// which takes its first counter values from drawn (see drawReplay).
func layoutTypedViaAttrs(in *LayoutInput, out *LayoutOutput, drawn []uint64) error {
	return layoutTypedWith(in, out, func(g *Graph) error {
		layoutGraph := buildLayoutGraph(g)
		layoutGraph.replay = &drawReplay{vals: drawn}
		return layoutAttrsBuilt(g, layoutGraph, func(_ string, fn func() error) error { return fn() })
	})
}

// layoutTypedWith lays out the Graph equivalent to in with layout and stores
// the result in out.
func layoutTypedWith(in *LayoutInput, out *LayoutOutput, layout func(*Graph) error) error {
	g, err := in.Graph()
	if err != nil {
		return err
	}
	if err := layout(g); err != nil {
		return err
	}
	ga := asAttrs(g.Graph())
	out.Width, out.Height = num(ga, "width"), num(ga, "height")
	out.Nodes = resize(out.Nodes, len(in.Nodes))
	for i, n := range in.Nodes {
		a := asAttrs(g.Node(n.ID))
		out.Nodes[i] = NodeLayout{X: num(a, "x"), Y: num(a, "y"), Width: num(a, "width"), Height: num(a, "height")}
	}
	out.Edges = resize(out.Edges, len(in.Edges))
	for i, e := range in.Edges {
		var a Attrs
		if e.HasName {
			a = asAttrs(g.EdgeByArgs(in.Nodes[e.V].ID, in.Nodes[e.W].ID, e.Name))
		} else {
			a = asAttrs(g.EdgeByArgs(in.Nodes[e.V].ID, in.Nodes[e.W].ID))
		}
		points, _ := a["points"].([]Point)
		el := EdgeLayout{Points: append([]Point(nil), points...)}
		if has(a, "x") {
			el.X, el.Y, el.HasXY = num(a, "x"), num(a, "y"), true
		}
		out.Edges[i] = el
	}
	return nil
}

// greedyFAS runs the Attrs implementation on a copy of the graph that has the
// same node order, edge order and weights; it is only used with
// acyclicer "greedy", which D2 does not set.
func (s *lstate) greedyFAS() []edgeRef {
	c := NewGraph(GraphOptions{Multigraph: true})
	names := make([]string, len(s.enames)+1)
	names[0] = defaultEdgeName
	for name, k := range s.enames {
		names[k] = name
	}
	for _, v := range s.nodes() {
		c.SetNode(s.names.name(v))
	}
	for _, e := range s.edges() {
		ed := s.g.edges[e]
		edge := Edge{V: s.names.name(ed.v), W: s.names.name(ed.w)}
		if ed.hasName {
			edge.Name, edge.HasName = names[ed.name], true
		}
		c.SetEdgeObject(edge, s.el[ed.label].weight)
	}
	found := greedyFAS(c, func(e Edge) float64 { return c.Edge(e).(float64) })
	out := make([]edgeRef, len(found))
	for i, e := range found {
		v, _ := s.names.lookup(e.V)
		w, _ := s.names.lookup(e.W)
		r := edgeRef{v: v, w: w, hasName: e.HasName}
		if e.HasName {
			r.name = s.edgeNameKey(e.Name)
		}
		out[i] = r
	}
	return out
}

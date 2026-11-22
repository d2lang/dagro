package dagro

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// LayoutTyped against Layout, the Attrs engine, on the graph that
// LayoutInput.Graph documents (built with this package's Graph API).

func referenceGraphFromInput(in *LayoutInput) (*Graph, error) {
	label := Attrs{"rankdir": in.RankDir, "ranksep": in.RankSep, "edgesep": in.EdgeSep, "nodesep": in.NodeSep}
	if in.MarginX != 0 {
		label["marginx"] = in.MarginX
	}
	if in.MarginY != 0 {
		label["marginy"] = in.MarginY
	}
	if in.Acyclicer != "" {
		label["acyclicer"] = in.Acyclicer
	}
	if in.Ranker != "" {
		label["ranker"] = in.Ranker
	}
	if in.Align != "" {
		label["align"] = in.Align
	}
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(label)
	for i, n := range in.Nodes {
		g.SetNode(n.ID, Attrs{"width": n.Width, "height": n.Height})
		if n.Parent >= 0 {
			if n.Parent >= len(in.Nodes) {
				return nil, fmt.Errorf("dagro: node %d has parent %d out of range", i, n.Parent)
			}
			if err := g.SetParent(n.ID, in.Nodes[n.Parent].ID); err != nil {
				return nil, err
			}
		}
	}
	for i, e := range in.Edges {
		if e.V < 0 || e.V >= len(in.Nodes) || e.W < 0 || e.W >= len(in.Nodes) {
			return nil, fmt.Errorf("dagro: edge %d endpoint out of range", i)
		}
		l := Attrs{
			"minlen": e.Minlen, "weight": e.Weight, "width": e.Width, "height": e.Height,
			"labeloffset": e.LabelOffset, "labelpos": e.LabelPos,
		}
		if e.HasName {
			g.SetEdge(in.Nodes[e.V].ID, in.Nodes[e.W].ID, l, e.Name)
		} else {
			g.SetEdge(in.Nodes[e.V].ID, in.Nodes[e.W].ID, l)
		}
	}
	return g, nil
}

func outputNumber(a Attrs, k string) float64 {
	v, ok := a[k]
	if !ok {
		return math.NaN()
	}
	switch x := v.(type) {
	case float64:
		return x
	case nil:
		return 0
	}
	panic(fmt.Sprintf("unexpected %T", v))
}

// referenceTypedLayout computes what LayoutTyped must return, from the Attrs engine's result.
func referenceTypedLayout(in *LayoutInput) (out LayoutOutput, err error) {
	g, err := referenceGraphFromInput(in)
	if err != nil {
		return out, err
	}
	if err := referenceLayoutOpts(g); err != nil {
		return out, err
	}
	ga := g.Graph().(Attrs)
	out.Width, out.Height = outputNumber(ga, "width"), outputNumber(ga, "height")
	for _, n := range in.Nodes {
		a := g.Node(n.ID).(Attrs)
		out.Nodes = append(out.Nodes, NodeLayout{X: outputNumber(a, "x"), Y: outputNumber(a, "y"), Width: outputNumber(a, "width"), Height: outputNumber(a, "height")})
	}
	for _, e := range in.Edges {
		var a Attrs
		if e.HasName {
			a = g.EdgeByArgs(in.Nodes[e.V].ID, in.Nodes[e.W].ID, e.Name).(Attrs)
		} else {
			a = g.EdgeByArgs(in.Nodes[e.V].ID, in.Nodes[e.W].ID).(Attrs)
		}
		points, _ := a["points"].([]Point)
		el := EdgeLayout{}
		for _, p := range points {
			el.Points = append(el.Points, Point{X: p.X, Y: p.Y})
		}
		if _, ok := a["x"]; ok {
			el.X, el.Y, el.HasXY = outputNumber(a, "x"), outputNumber(a, "y"), true
		}
		out.Edges = append(out.Edges, el)
	}
	return out, nil
}

func dumpTypedOutput(out *LayoutOutput) string {
	var b strings.Builder
	b.WriteString("graph ")
	writeFloatBits(&b, out.Width)
	writeFloatBits(&b, out.Height)
	b.WriteString("\n")
	for i, n := range out.Nodes {
		fmt.Fprintf(&b, "node %d ", i)
		writeFloatBits(&b, n.X)
		writeFloatBits(&b, n.Y)
		writeFloatBits(&b, n.Width)
		writeFloatBits(&b, n.Height)
		b.WriteString("\n")
	}
	for i, e := range out.Edges {
		fmt.Fprintf(&b, "edge %d nil=%v ", i, e.Points == nil)
		writeCanonical(&b, e.Points)
		if e.HasXY {
			b.WriteString(" xy ")
			writeFloatBits(&b, e.X)
			writeFloatBits(&b, e.Y)
		}
		b.WriteString("\n")
	}
	return b.String()
}

type inputGenerator struct {
	*specGenerator
}

func (z inputGenerator) num(weird bool, def float64, normal func() float64) float64 {
	if weird && z.weirdNums && z.p(8) {
		return z.pickA(math.Copysign(0, -1), -3.0, math.NaN(), math.Inf(1), math.Inf(-1), 0.1, 1e300).(float64)
	}
	if z.p(6) {
		return def
	}
	return normal()
}

func (z inputGenerator) input(maxNodes int) LayoutInput {
	in := DefaultLayoutInput()
	in.RankDir = z.pickS("TB", "BT", "LR", "RL", "TB", "tb", "lr", "", "x", "Rl")
	sep := func() float64 { return float64(z.rng.Intn(120)) + z.pickA(0.0, 0.0, 0.5, 0.25).(float64) }
	in.RankSep = z.num(true, 50, sep)
	in.EdgeSep = z.num(true, 20, sep)
	in.NodeSep = z.num(true, 50, sep)
	if z.p(4) {
		in.MarginX = z.num(true, 0, sep)
		in.MarginY = z.num(true, 0, sep)
	}
	if z.p(6) {
		in.Acyclicer = z.pickS("greedy", "x")
	}
	if z.p(5) {
		in.Ranker = z.pickS("network-simplex", "tight-tree", "longest-path", "other")
	}
	if z.p(5) {
		in.Align = z.pickS("UL", "ur", "dl", "DR", "x")
	}
	n := 1 + z.rng.Intn(maxNodes)
	names := z.names(n)
	if z.p(3) {
		names = names[:0]
		for i := 0; i < n; i++ {
			names = append(names, strconv.Itoa(i))
		}
	}
	if z.weirdLabels && z.p(40) && len(names) > 1 {
		names[1] = names[0] // duplicate id
	}
	if z.weirdLabels && z.p(60) {
		names[0] = z.pickS("", "a\x01b")
	}
	size := func() float64 { return float64(z.rng.Intn(150)) + z.pickA(0.0, 0.0, 0.0, 0.5).(float64) }
	clusters := z.rng.Intn(4)
	if clusters >= len(names) {
		clusters = 0
	}
	for i, id := range names {
		node := LayoutNode{ID: id, Parent: -1, Width: z.num(true, 0, size), Height: z.num(true, 0, size)}
		if i >= clusters && clusters > 0 && z.rng.Intn(3) != 0 {
			node.Parent = z.rng.Intn(clusters)
		} else if i > 0 && i < clusters && z.p(2) {
			node.Parent = z.rng.Intn(i)
		}
		if z.weirdLabels && z.p(80) {
			node.Parent = z.pickA(i, len(names)+3, -2).(int) // self-parent, out of range, other negative
		}
		in.Nodes = append(in.Nodes, node)
	}
	leaves := []int{}
	for i := clusters; i < len(names); i++ {
		leaves = append(leaves, i)
	}
	if len(leaves) == 0 {
		return in
	}
	m := z.rng.Intn(3*len(leaves) + 1)
	for k := 0; k < m; k++ {
		v, w := leaves[z.rng.Intn(len(leaves))], leaves[z.rng.Intn(len(leaves))]
		if z.p(25) && clusters > 0 {
			w = z.rng.Intn(clusters)
		}
		if z.p(10) {
			w = v
		}
		e := NewLayoutEdge(v, w)
		if z.p(2) {
			e.Name, e.HasName = z.pickS(fmt.Sprintf("(%d -> %d)[%d]", v, w, k), "", "rev1", defaultEdgeName, "e"), true
		}
		if z.p(4) {
			e.Width, e.Height = z.num(true, 0, size), z.num(true, 0, size)
			e.LabelPos = z.pickS("c", "l", "r", "C", "", "x")
		}
		if z.p(6) {
			if z.fracMinlen && z.p(2) {
				e.Minlen = z.pickA(0.5, 1.5, 0.25, 2.5).(float64)
			} else if z.minlen0 && z.p(2) {
				e.Minlen = z.pickA(0.0, -1.0, math.NaN()).(float64)
			} else {
				e.Minlen = float64(1 + z.rng.Intn(3))
			}
		}
		if z.p(6) {
			if z.fracWeight && z.p(2) {
				e.Weight = z.pickA(0.5, 1.5, 0.25).(float64)
			} else {
				e.Weight = float64(1 + z.rng.Intn(4))
			}
		}
		if z.p(8) {
			e.LabelOffset = z.pickA(0.0, 5.0, 2.5, -3.0).(float64)
		}
		if z.weirdLabels && z.p(80) {
			e.V = len(names) + 2
		}
		in.Edges = append(in.Edges, e)
	}
	return in
}

func runTypedPair(in LayoutInput, start uint64) (want, got layoutOutcome) {
	want = runWithWatchdog(func(o *layoutOutcome) {
		uniqueIDCounter.Store(start)
		defer func() { o.counter = uniqueIDCounter.Load() - start }()
		out, err := referenceTypedLayout(&in)
		if err != nil {
			o.err = err.Error()
			return
		}
		o.dump = dumpTypedOutput(&out)
	})
	if want.hung {
		return want, got
	}
	got = runWithWatchdog(func(o *layoutOutcome) {
		uniqueIDCounter.Store(start)
		defer func() { o.counter = uniqueIDCounter.Load() - start }()
		var out LayoutOutput
		if err := LayoutTyped(&in, &out); err != nil {
			o.err = err.Error()
			return
		}
		o.dump = dumpTypedOutput(&out)
	})
	return want, got
}

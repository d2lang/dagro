package dagro

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// Phase-by-phase comparison of the Attrs engine and the typed engine (both in
// this tree), to locate where they diverge.

func attrsPhaseState(g *Graph) string {
	var lines []string
	for _, v := range g.Nodes() {
		n := asAttrs(g.Node(v))
		s := fmt.Sprintf("%s:", v)
		for _, k := range []string{"rank", "order", "x", "y", "minRank", "maxRank"} {
			if has(n, k) {
				s += fmt.Sprintf(" %s=%v", k, num(n, k))
			}
		}
		if p, ok := g.Parent(v); ok {
			s += " parent=" + p
		}
		lines = append(lines, s)
	}
	sort.Strings(lines)
	var e []string
	for _, ed := range g.Edges() {
		l := asAttrs(g.Edge(ed))
		e = append(e, fmt.Sprintf("%s->%s minlen=%v weight=%v", ed.V, ed.W, num(l, "minlen"), num(l, "weight")))
	}
	sort.Strings(e)
	return strings.Join(lines, "\n") + "\n" + strings.Join(e, "\n")
}

func typedPhaseState(s *lstate) string {
	var lines []string
	for _, id := range s.g.nodeList(s.key(), nil) {
		n := &s.nl[id]
		str := fmt.Sprintf("%s:", s.names.name(id))
		put := func(k string, v float64, ok bool) {
			if ok {
				str += fmt.Sprintf(" %s=%v", k, v)
			}
		}
		put("rank", n.rank, n.hasRank)
		put("order", n.order, n.hasOrder)
		put("x", n.x, n.hasX || !math.IsNaN(n.x))
		put("y", n.y, n.hasY || !math.IsNaN(n.y))
		put("minRank", n.minRank, n.hasMinMax)
		put("maxRank", n.maxRank, n.hasMinMax)
		if p := s.g.parentOf(id); p != inone {
			str += " parent=" + s.names.name(p)
		}
		lines = append(lines, str)
	}
	sort.Strings(lines)
	var e []string
	for _, ei := range s.g.edgeList(nil) {
		ed := s.g.edges[ei]
		l := &s.el[ed.label]
		e = append(e, fmt.Sprintf("%s->%s minlen=%v weight=%v", s.names.name(ed.v), s.names.name(ed.w), l.minlen, l.weight))
	}
	sort.Strings(e)
	return strings.Join(lines, "\n") + "\n" + strings.Join(e, "\n")
}

func comparePhases(t *testing.T, build func() *Graph, start uint64) {
	g1 := build()
	g2 := build()
	uniqueIDCounter.Store(start)
	lg := buildLayoutGraph(g1)
	attrsPhases := []struct {
		name string
		fn   func()
	}{
		{"makeSpaceForEdgeLabels", func() { makeSpaceForEdgeLabels(lg) }},
		{"removeSelfEdges", func() { removeSelfEdges(lg) }},
		{"runAcyclic", func() { runAcyclic(lg) }},
		{"runNestingGraph", func() { runNestingGraph(lg) }},
		{"rank", func() { rank(asNonCompoundGraph(lg)) }},
		{"injectEdgeLabelProxies", func() { injectEdgeLabelProxies(lg) }},
		{"removeEmptyRanks", func() { removeEmptyRanks(lg) }},
		{"cleanupNestingGraph", func() { cleanupNestingGraph(lg) }},
		{"normalizeRanks", func() { normalizeRanks(lg) }},
		{"assignRankMinMax", func() { assignRankMinMax(lg) }},
		{"removeEdgeLabelProxies", func() { removeEdgeLabelProxies(lg) }},
		{"runNormalize", func() { runNormalize(lg) }},
		{"parentDummyChains", func() { parentDummyChains(lg) }},
		{"addBorderSegments", func() { addBorderSegments(lg) }},
		{"order", func() { order(lg) }},
		{"insertSelfEdges", func() { insertSelfEdges(lg) }},
		{"adjustCoordinateSystem", func() { adjustCoordinateSystem(lg) }},
		{"position", func() { position(lg) }},
	}
	var attrsStates []string
	for _, p := range attrsPhases {
		p.fn()
		attrsStates = append(attrsStates, attrsPhaseState(lg))
	}
	uniqueIDCounter.Store(start)
	s := getLState()
	if !s.build(g2) {
		t.Fatalf("typed engine declined")
	}
	typedPhases := []func(){
		s.makeSpaceForEdgeLabels, s.removeSelfEdges, s.runAcyclic, s.runNestingGraph, s.rank,
		s.injectEdgeLabelProxies, s.removeEmptyRanks, s.cleanupNestingGraph, s.normalizeRanks,
		s.assignRankMinMax, s.removeEdgeLabelProxies, s.runNormalize, s.parentDummyChains,
		s.addBorderSegments, s.order, s.insertSelfEdges, s.adjustCoordinateSystem, s.position,
	}
	for i, fn := range typedPhases {
		fn()
		got := typedPhaseState(s)
		if got != attrsStates[i] {
			t.Logf("first divergence after phase %s:\nATTRS:\n%s\nTYPED:\n%s", attrsPhases[i].name, attrsStates[i], got)
			return
		}
	}
	t.Logf("no divergence through position")
}

func TestPhaseFracMinlen(t *testing.T) {
	build := func() *Graph {
		g := NewGraph(GraphOptions{Compound: true}).SetGraph(Attrs{})
		_ = g.SetParent("_7", "c2")
		g.SetEdge("4", "_10", Attrs{"minlen": 0.25})
		g.SetEdge("6", "_10", Attrs{})
		g.SetEdge("1", "4", Attrs{})
		return g
	}
	comparePhases(t, build, 40175)
}

func TestBKFracMinlen(t *testing.T) {
	build := func() *Graph {
		g := NewGraph(GraphOptions{Compound: true}).SetGraph(Attrs{})
		_ = g.SetParent("_7", "c2")
		g.SetEdge("4", "_10", Attrs{"minlen": 0.25})
		g.SetEdge("6", "_10", Attrs{})
		g.SetEdge("1", "4", Attrs{})
		return g
	}
	// Attrs engine up to position, then BK step by step.
	uniqueIDCounter.Store(40175)
	lg := buildLayoutGraph(build())
	for _, f := range []func(*Graph){makeSpaceForEdgeLabels, removeSelfEdges, runAcyclic, runNestingGraph} {
		f(lg)
	}
	rank(asNonCompoundGraph(lg))
	for _, f := range []func(*Graph){injectEdgeLabelProxies, removeEmptyRanks, cleanupNestingGraph, normalizeRanks,
		assignRankMinMax, removeEdgeLabelProxies, runNormalize, parentDummyChains, addBorderSegments, order,
		insertSelfEdges, adjustCoordinateSystem} {
		f(lg)
	}
	g := asNonCompoundGraph(lg)
	layering := buildLayerMatrix(g)
	t.Logf("ATTRS layering %v", layering)
	conflicts := shallowMergePositionConflicts(findType1Conflicts(g, layering), findType2Conflicts(g, layering))
	t.Logf("ATTRS conflicts %v", conflicts)
	al := verticalAlignment(g, layering, conflicts, g.Predecessors)
	t.Logf("ATTRS ul root %v align %v", al.root, al.align)
	bg := buildBlockGraph(g, layering, al.root, false)
	for _, e := range bg.Edges() {
		t.Logf("ATTRS block edge %s -> %s %v", e.V, e.W, bg.Edge(e))
	}
	xs := horizontalCompaction(g, layering, al.root, al.align, false)
	t.Logf("ATTRS ul xs %v", xs)

	uniqueIDCounter.Store(40175)
	s := getLState()
	if !s.build(build()) {
		t.Fatal("declined")
	}
	for _, f := range []func(){s.makeSpaceForEdgeLabels, s.removeSelfEdges, s.runAcyclic, s.runNestingGraph, s.rank,
		s.injectEdgeLabelProxies, s.removeEmptyRanks, s.cleanupNestingGraph, s.normalizeRanks,
		s.assignRankMinMax, s.removeEdgeLabelProxies, s.runNormalize, s.parentDummyChains,
		s.addBorderSegments, s.order, s.insertSelfEdges, s.adjustCoordinateSystem} {
		f()
	}
	p := &s.p
	p.s = s
	leaves := p.leaves[:0]
	for _, v := range s.nodes() {
		if !s.g.hasChildren(v) {
			leaves = append(leaves, v)
		}
	}
	p.leaves = leaves
	p.buildNeighbors()
	p.positionY(leaves)
	p.positionX(leaves)
	tl := s.buildLayerMatrix(leaves)
	var names [][]string
	for _, l := range tl {
		var row []string
		for _, v := range l {
			row = append(row, s.names.name(v))
		}
		names = append(names, row)
	}
	t.Logf("TYPED layering %v", names)
	xsT := map[string]float64{}
	for _, v := range p.nodes {
		xsT[s.names.name(v)] = p.xs[0][v]
	}
	t.Logf("TYPED ul xs %v", xsT)
	// redo ul alignment to print root/align and block graph
	adjusted := copyIDLayering(nil, tl, new(int))
	p.verticalAlignment(adjusted, true)
	ra := map[string]string{}
	for _, v := range p.nodes {
		ra[s.names.name(v)] = s.names.name(p.root[v])
	}
	t.Logf("TYPED ul root %v", ra)
	xs2 := make([]float64, len(s.nl))
	p.horizontalCompaction(adjusted, xs2, false)
	for e := range p.bk.ev {
		t.Logf("TYPED block edge %s -> %s %v", s.names.name(p.bk.ev[e]), s.names.name(p.bk.ew[e]), p.bk.weight[e])
	}
}

func TestBK4FracMinlen(t *testing.T) {
	build := func() *Graph {
		g := NewGraph(GraphOptions{Compound: true}).SetGraph(Attrs{})
		_ = g.SetParent("_7", "c2")
		g.SetEdge("4", "_10", Attrs{"minlen": 0.25})
		g.SetEdge("6", "_10", Attrs{})
		g.SetEdge("1", "4", Attrs{})
		return g
	}
	uniqueIDCounter.Store(40175)
	lg := buildLayoutGraph(build())
	for _, f := range []func(*Graph){makeSpaceForEdgeLabels, removeSelfEdges, runAcyclic, runNestingGraph} {
		f(lg)
	}
	rank(asNonCompoundGraph(lg))
	for _, f := range []func(*Graph){injectEdgeLabelProxies, removeEmptyRanks, cleanupNestingGraph, normalizeRanks,
		assignRankMinMax, removeEdgeLabelProxies, runNormalize, parentDummyChains, addBorderSegments, order,
		insertSelfEdges, adjustCoordinateSystem} {
		f(lg)
	}
	g := asNonCompoundGraph(lg)
	layering := buildLayerMatrix(g)
	conflicts := shallowMergePositionConflicts(findType1Conflicts(g, layering), findType2Conflicts(g, layering))
	for _, vert := range []string{"u", "d"} {
		adjustedLayering := clonePositionLayering(layering)
		if vert == "d" {
			reverseStrings2DOuter(adjustedLayering)
		}
		for _, horiz := range []string{"l", "r"} {
			if horiz == "r" {
				for i := range adjustedLayering {
					reverseStrings(adjustedLayering[i])
				}
			}
			neighborFn := g.Predecessors
			if vert == "d" {
				neighborFn = g.Successors
			}
			alignment := verticalAlignment(g, adjustedLayering, conflicts, neighborFn)
			xs := horizontalCompaction(g, adjustedLayering, alignment.root, alignment.align, horiz == "r")
			keys := []string{}
			for k := range xs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var b strings.Builder
			for _, k := range keys {
				fmt.Fprintf(&b, "%s=%v ", k, xs[k])
			}
			t.Logf("ATTRS %s%s: %s", vert, horiz, b.String())
		}
	}

	uniqueIDCounter.Store(40175)
	s := getLState()
	if !s.build(build()) {
		t.Fatal("declined")
	}
	for _, f := range []func(){s.makeSpaceForEdgeLabels, s.removeSelfEdges, s.runAcyclic, s.runNestingGraph, s.rank,
		s.injectEdgeLabelProxies, s.removeEmptyRanks, s.cleanupNestingGraph, s.normalizeRanks,
		s.assignRankMinMax, s.removeEdgeLabelProxies, s.runNormalize, s.parentDummyChains,
		s.addBorderSegments, s.order, s.insertSelfEdges, s.adjustCoordinateSystem} {
		f()
	}
	p := &s.p
	p.s = s
	leaves := p.leaves[:0]
	for _, v := range s.nodes() {
		if !s.g.hasChildren(v) {
			leaves = append(leaves, v)
		}
	}
	p.leaves = leaves
	p.buildNeighbors()
	p.positionY(leaves)
	// positionX without alignCoordinates/balance
	tl := s.buildLayerMatrix(leaves)
	layering2 := copyIDLayering(nil, tl, new(int))
	n := len(s.nl)
	p.chead = make([]int32, n)
	for i := range p.chead {
		p.chead[i] = inone
	}
	p.cents = p.cents[:0]
	p.type2Key = make([]bool, n)
	p.findType1Conflicts(layering2)
	p.findType2Conflicts(layering2)
	p.pos = make([]int32, n)
	p.posStamp = make([]int32, n)
	p.root = make([]int32, n)
	p.align = make([]int32, n)
	p.nodes = p.nodes[:0]
	for _, layer := range layering2 {
		p.nodes = append(p.nodes, layer...)
	}
	for _, vert := range [2]byte{'u', 'd'} {
		adjusted := copyIDLayering(nil, layering2, new(int))
		if vert == 'd' {
			for i, j := 0, len(adjusted)-1; i < j; i, j = i+1, j-1 {
				adjusted[i], adjusted[j] = adjusted[j], adjusted[i]
			}
		}
		for _, horiz := range [2]byte{'l', 'r'} {
			if horiz == 'r' {
				for _, layer := range adjusted {
					for i, j := 0, len(layer)-1; i < j; i, j = i+1, j-1 {
						layer[i], layer[j] = layer[j], layer[i]
					}
				}
			}
			p.verticalAlignment(adjusted, vert == 'u')
			xs := make([]float64, n)
			p.horizontalCompaction(adjusted, xs, horiz == 'r')
			keys := []string{}
			m := map[string]float64{}
			for _, v := range p.nodes {
				m[s.names.name(v)] = xs[v]
				keys = append(keys, s.names.name(v))
			}
			sort.Strings(keys)
			var b strings.Builder
			for _, k := range keys {
				fmt.Fprintf(&b, "%s=%v ", k, m[k])
			}
			t.Logf("TYPED %c%c: %s", vert, horiz, b.String())
		}
	}
}

func TestPhaseNaNMinlen(t *testing.T) {
	build := func() *Graph {
		g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"rankdir": "LR"})
		_ = g.SetParent("0", "_bt3")
		_ = g.SetParent("4", "_bt3")
		_ = g.SetParent("7", "_bt3")
		g.SetEdge("6", "7", Attrs{}, "(6 -> 7)[1]")
		g.SetEdge("6", "3", Attrs{"minlen": 7.0}, "(6 -> 3)[3]")
		g.SetEdge("7", "4", Attrs{"minlen": math.NaN()}, "(7 -> 4)[4]")
		g.SetEdge("3", "4", Attrs{}, "(3 -> 4)[7]")
		g.SetEdge("0", "_5", Attrs{}, "(0 -> _5)[9]")
		g.SetEdge("_5", "6", Attrs{}, "(_5 -> 6)[10]")
		return g
	}
	comparePhases(t, build, 177229)
}

func attrsLateState(g *Graph) string {
	var lines []string
	for _, v := range g.Nodes() {
		n := asAttrs(g.Node(v))
		s := fmt.Sprintf("%s:", v)
		for _, k := range []string{"rank", "order", "x", "y", "width", "height"} {
			if has(n, k) {
				s += fmt.Sprintf(" %s=%v", k, num(n, k))
			}
		}
		lines = append(lines, s)
	}
	sort.Strings(lines)
	var e []string
	for _, ed := range g.Edges() {
		l := asAttrs(g.Edge(ed))
		pts, _ := l["points"].([]Point)
		s := fmt.Sprintf("%s->%s points=%v", ed.V, ed.W, pts)
		if has(l, "x") {
			s += fmt.Sprintf(" x=%v y=%v", num(l, "x"), num(l, "y"))
		}
		e = append(e, s)
	}
	sort.Strings(e)
	gl := asAttrs(g.Graph())
	return strings.Join(lines, "\n") + "\n" + strings.Join(e, "\n") + fmt.Sprintf("\ngraph w=%v h=%v", num(gl, "width"), num(gl, "height"))
}

func typedLateState(s *lstate) string {
	var lines []string
	for _, id := range s.g.nodeList(s.key(), nil) {
		n := &s.nl[id]
		str := fmt.Sprintf("%s:", s.names.name(id))
		put := func(k string, v float64, ok bool) {
			if ok {
				str += fmt.Sprintf(" %s=%v", k, v)
			}
		}
		put("rank", n.rank, n.hasRank)
		put("order", n.order, n.hasOrder)
		put("x", n.x, n.hasX || !math.IsNaN(n.x))
		put("y", n.y, n.hasY || !math.IsNaN(n.y))
		put("width", n.width, true)
		put("height", n.height, true)
		lines = append(lines, str)
	}
	sort.Strings(lines)
	var e []string
	for _, ei := range s.g.edgeList(nil) {
		ed := s.g.edges[ei]
		l := &s.el[ed.label]
		var pts []Point
		if l.hasPoints {
			pts = l.points
		}
		str := fmt.Sprintf("%s->%s points=%v", s.names.name(ed.v), s.names.name(ed.w), pts)
		if l.hasX {
			str += fmt.Sprintf(" x=%v y=%v", l.x, l.y)
		}
		e = append(e, str)
	}
	sort.Strings(e)
	return strings.Join(lines, "\n") + "\n" + strings.Join(e, "\n") + fmt.Sprintf("\ngraph w=%v h=%v", s.width, s.height)
}

func compareLatePhases(t *testing.T, build func() *Graph, start uint64) {
	uniqueIDCounter.Store(start)
	lg := buildLayoutGraph(build())
	steps := []struct {
		name string
		fn   func()
	}{
		{"makeSpaceForEdgeLabels", func() { makeSpaceForEdgeLabels(lg) }},
		{"removeSelfEdges", func() { removeSelfEdges(lg) }},
		{"runAcyclic", func() { runAcyclic(lg) }},
		{"runNestingGraph", func() { runNestingGraph(lg) }},
		{"rank", func() { rank(asNonCompoundGraph(lg)) }},
		{"injectEdgeLabelProxies", func() { injectEdgeLabelProxies(lg) }},
		{"removeEmptyRanks", func() { removeEmptyRanks(lg) }},
		{"cleanupNestingGraph", func() { cleanupNestingGraph(lg) }},
		{"normalizeRanks", func() { normalizeRanks(lg) }},
		{"assignRankMinMax", func() { assignRankMinMax(lg) }},
		{"removeEdgeLabelProxies", func() { removeEdgeLabelProxies(lg) }},
		{"runNormalize", func() { runNormalize(lg) }},
		{"parentDummyChains", func() { parentDummyChains(lg) }},
		{"addBorderSegments", func() { addBorderSegments(lg) }},
		{"order", func() { order(lg) }},
		{"insertSelfEdges", func() { insertSelfEdges(lg) }},
		{"adjustCoordinateSystem", func() { adjustCoordinateSystem(lg) }},
		{"position", func() { position(lg) }},
		{"positionSelfEdges", func() { positionSelfEdges(lg) }},
		{"removeBorderNodes", func() { removeBorderNodes(lg) }},
		{"undoNormalize", func() { undoNormalize(lg) }},
		{"fixupEdgeLabelCoords", func() { fixupEdgeLabelCoords(lg) }},
		{"undoCoordinateSystem", func() { undoCoordinateSystem(lg) }},
		{"translateGraph", func() { translateGraph(lg) }},
		{"assignNodeIntersects", func() { _ = assignNodeIntersects(lg) }},
	}
	var states []string
	for _, st := range steps {
		st.fn()
		states = append(states, attrsLateState(lg))
	}
	uniqueIDCounter.Store(start)
	s := getLState()
	if !s.build(build()) {
		t.Fatal("declined")
	}
	typed := []func(){s.makeSpaceForEdgeLabels, s.removeSelfEdges, s.runAcyclic, s.runNestingGraph, s.rank,
		s.injectEdgeLabelProxies, s.removeEmptyRanks, s.cleanupNestingGraph, s.normalizeRanks,
		s.assignRankMinMax, s.removeEdgeLabelProxies, s.runNormalize, s.parentDummyChains,
		s.addBorderSegments, s.order, s.insertSelfEdges, s.adjustCoordinateSystem, s.position,
		s.positionSelfEdges, s.removeBorderNodes, s.undoNormalize, s.fixupEdgeLabelCoords,
		s.undoCoordinateSystem, s.translateGraph, func() { _ = s.assignNodeIntersects() }}
	for i, fn := range typed {
		fn()
		got := typedLateState(s)
		want := states[i]
		if i < 23 {
			got = got[:strings.LastIndex(got, "\ngraph")]
			want = want[:strings.LastIndex(want, "\ngraph")]
		}
		if i >= 17 && got != want {
			t.Logf("first divergence after %s:\nATTRS:\n%s\nTYPED:\n%s", steps[i].name, states[i], got)
			return
		}
	}
	t.Logf("no divergence")
}

func TestPhaseLateNaNMinlen(t *testing.T) {
	build := func() *Graph {
		g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"rankdir": "LR"})
		_ = g.SetParent("0", "_bt3")
		_ = g.SetParent("4", "_bt3")
		_ = g.SetParent("7", "_bt3")
		g.SetEdge("6", "7", Attrs{}, "(6 -> 7)[1]")
		g.SetEdge("6", "3", Attrs{"minlen": 7.0}, "(6 -> 3)[3]")
		g.SetEdge("7", "4", Attrs{"minlen": math.NaN()}, "(7 -> 4)[4]")
		g.SetEdge("3", "4", Attrs{}, "(3 -> 4)[7]")
		g.SetEdge("0", "_5", Attrs{}, "(0 -> _5)[9]")
		g.SetEdge("_5", "6", Attrs{}, "(_5 -> 6)[10]")
		return g
	}
	compareLatePhases(t, build, 177229)
}

func negZeroWeightGreedyGraph() *Graph {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"acyclicer": "greedy"})
	_ = g.SetParent("2", "103")
	_ = g.SetParent("1", "_bt1")
	g.SetEdge("1", "2", Attrs{"weight": math.Copysign(0, -1)}, "(1 -> 2)[0]")
	g.SetEdge("2", "0", Attrs{}, "(2 -> 0)[3]")
	g.SetEdge("2", "1", Attrs{}, "(2 -> 1)[4]")
	return g
}

func TestPhaseNegZeroWeightGreedy(t *testing.T) {
	skipWithoutTypedGreedy(t)
	comparePhases(t, negZeroWeightGreedyGraph, 42312)
}

func nanWeightLRGraph() *Graph {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"rankdir": "LR"})
	for _, p := range [][2]string{{"757", "c3"}, {"_bt1", "102"}, {"339", "_bt1"}, {"583", "102"}, {"n222", "102"}, {"n336", "_bt1"}, {"69", "c3"}} {
		_ = g.SetParent(p[0], p[1])
	}
	g.SetEdge("n222", "69", Attrs{}, "(n222 -> 69)[0]")
	g.SetEdge("969", "n930", Attrs{}, "(969 -> n930)[3]")
	g.SetEdge("n336", "n222", Attrs{}, "\x00")
	g.SetEdge("n336", "n225", Attrs{"weight": math.NaN()}, "(n336 -> n225)[10]")
	g.SetEdge("0", "339", Attrs{}, "(0 -> 339)[11]")
	g.SetEdge("757", "969", Attrs{}, "x")
	return g
}

func infWeightLRGraph() *Graph {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"rankdir": "LR"})
	for _, p := range [][2]string{{"_c3", "c4"}, {"_c2", "_c3"}, {"8", "c4"}, {"1", "_bt1"}, {"6", "_bt1"}, {"7", "_c2"}} {
		_ = g.SetParent(p[0], p[1])
	}
	g.SetEdge("4", "1", Attrs{}, "(4 -> 1)[2]")
	g.SetEdge("8", "4", Attrs{}, "rev01")
	g.SetEdge("6", "0", Attrs{}, "(6 -> 0)[11]")
	g.SetEdge("0", "3", Attrs{}, "(0 -> 3)[14]")
	g.SetEdge("7", "2", Attrs{"weight": math.Inf(1)}, "")
	g.SetEdge("1", "7", Attrs{}, "\x00")
	g.SetEdge("2", "4", Attrs{}, "(2 -> 4)[27]")
	return g
}

func TestPhaseLateNaNWeightLR(t *testing.T) {
	compareLatePhases(t, nanWeightLRGraph, 70703)
}
func TestPhaseLateInfWeightLR(t *testing.T) {
	compareLatePhases(t, infWeightLRGraph, 130928)
}
func TestPhaseNaNWeightLR(t *testing.T) { comparePhases(t, nanWeightLRGraph, 70703) }
func TestPhaseInfWeightLR(t *testing.T) { comparePhases(t, infWeightLRGraph, 130928) }

// threeWayCompare runs the original, this tree's Attrs engine and this tree's
// Layout (typed engine) on the same input.
func threeWayCompare(t *testing.T, build func() *Graph, buildReference func() *Graph, start uint64) {
	run := func(f func() (string, error)) (dump string, errText string, panicked string) {
		defer func() {
			if r := recover(); r != nil {
				panicked = fmt.Sprint(r)
			}
		}()
		d, err := f()
		if err != nil {
			errText = err.Error()
		}
		return d, errText, ""
	}
	od, oe, op := run(func() (string, error) {
		g := buildReference()
		uniqueIDCounter.Store(start)
		err := referenceLayoutOpts(g)
		return dumpReferenceGraph(g), err
	})
	ad, ae, ap := run(func() (string, error) {
		g := build()
		uniqueIDCounter.Store(start)
		err := referenceLayout(g)
		return dumpGraph(g), err
	})
	td, te, tp := run(func() (string, error) {
		g := build()
		uniqueIDCounter.Store(start)
		err := Layout(g)
		return dumpGraph(g), err
	})
	t.Logf("orig err %q panic %q\nattrs err %q panic %q\ntyped err %q panic %q\norig==attrs %v attrs==typed %v orig==typed %v",
		oe, op, ae, ap, te, tp, od == ad, ad == td, od == td)
}

func copyingBuild(build func() *Graph) func() *Graph {
	return func() *Graph {
		g := build()
		o := NewGraph(GraphOptions{Compound: g.IsCompound(), Multigraph: g.IsMultigraph(), Undirected: !g.IsDirected()})
		o.SetGraph(Attrs(copyAttrsMap(g.Graph().(Attrs))))
		for _, v := range g.Nodes() {
			if a, ok := g.Node(v).(Attrs); ok && a != nil {
				o.SetNode(v, Attrs(copyAttrsMap(a)))
			} else {
				o.SetNode(v, g.Node(v))
			}
		}
		for _, v := range g.Nodes() {
			if p, ok := g.Parent(v); ok {
				_ = o.SetParent(v, p)
			}
		}
		for _, e := range g.Edges() {
			l := Attrs(copyAttrsMap(g.Edge(e).(Attrs)))
			if e.HasName {
				o.SetEdge(e.V, e.W, l, e.Name)
			} else {
				o.SetEdge(e.V, e.W, l)
			}
		}
		return o
	}
}

func TestThreeWayNaNWeightLR(t *testing.T) {
	threeWayCompare(t, nanWeightLRGraph, copyingBuild(nanWeightLRGraph), 70703)
}
func TestThreeWayInfWeightLR(t *testing.T) {
	threeWayCompare(t, infWeightLRGraph, copyingBuild(infWeightLRGraph), 130928)
}
func TestThreeWayNegZeroGreedy(t *testing.T) {
	threeWayCompare(t, negZeroWeightGreedyGraph, copyingBuild(negZeroWeightGreedyGraph), 42312)
}

func nanWeightLRSpec() graphSpec {
	s := graphSpec{compound: true, multigraph: true, graph: attrsLabel("rankdir", "LR")}
	for _, p := range [][2]string{{"757", "c3"}, {"_bt1", "102"}, {"339", "_bt1"}, {"583", "102"}, {"n222", "102"}, {"n336", "_bt1"}, {"69", "c3"}} {
		s.ops = append(s.ops, specOp{kind: specOpSetParent, v: p[0], w: p[1]})
	}
	e := func(v, w string, l specLabel, name string) {
		s.ops = append(s.ops, specOp{kind: specOpSetEdge, v: v, w: w, label: l, name: name, hasName: true})
	}
	e("n222", "69", attrsLabel(), "(n222 -> 69)[0]")
	e("969", "n930", attrsLabel(), "(969 -> n930)[3]")
	e("n336", "n222", attrsLabel(), "\x00")
	e("n336", "n225", attrsLabel("weight", math.NaN()), "(n336 -> n225)[10]")
	e("0", "339", attrsLabel(), "(0 -> 339)[11]")
	e("757", "969", attrsLabel(), "x")
	return s
}

func TestSpecNaNWeightLR(t *testing.T) {
	s := nanWeightLRSpec()
	want, got := runSpecReference(s, 70703), runSpecLayout(s, 70703)
	t.Logf("differs: %q", diffOutcomes(want, got))
	build := func() *Graph { g, _ := buildFromSpec(s); return g }
	ref := runWithWatchdog(func(o *layoutOutcome) {
		g := build()
		uniqueIDCounter.Store(70703)
		defer func() { o.counter = uniqueIDCounter.Load() - 70703; o.dump = dumpGraph(g) }()
		if err := referenceLayout(g); err != nil {
			o.err = err.Error()
		}
	})
	t.Logf("in-tree Attrs engine vs original: %q", diffOutcomes(want, ref))
	comparePhases(t, build, 70703)
	compareLatePhases(t, build, 70703)
}

func nanWeightLRSpec2() graphSpec {
	s := nanWeightLRSpec()
	s.ops = append(s.ops, specOp{kind: specOpSetEdge, v: "n930", w: "n336", label: attrsLabel(), name: "(n930 -> n336)[13]", hasName: true})
	return s
}

func TestSpecNaNWeightLR2(t *testing.T) {
	s := nanWeightLRSpec2()
	want, got := runSpecReference(s, 70703), runSpecLayout(s, 70703)
	t.Logf("orig vs new differs: %v", diffOutcomes(want, got) != "")
	build := func() *Graph { g, _ := buildFromSpec(s); return g }
	ref := runWithWatchdog(func(o *layoutOutcome) {
		g := build()
		uniqueIDCounter.Store(70703)
		defer func() { o.counter = uniqueIDCounter.Load() - 70703; o.dump = dumpGraph(g) }()
		if err := referenceLayout(g); err != nil {
			o.err = err.Error()
		}
	})
	t.Logf("in-tree Attrs engine vs original differs: %v", diffOutcomes(want, ref) != "")
	compareLatePhases(t, build, 70703)
}

func nanWeightLRSpec3() graphSpec {
	s := nanWeightLRSpec2()
	s.ops = append(s.ops,
		specOp{kind: specOpSetEdge, v: "69", w: "583", label: attrsLabel(), name: "(69 -> 583)[18]", hasName: true},
		specOp{kind: specOpSetEdge, v: "n222", w: "969", label: attrsLabel("weight", -1.0), name: "rev2", hasName: true})
	return s
}

func TestSpecNaNWeightLR3(t *testing.T) {
	s := nanWeightLRSpec3()
	want, got := runSpecReference(s, 70703), runSpecLayout(s, 70703)
	t.Logf("orig vs new differs: %v", diffOutcomes(want, got) != "")
	build := func() *Graph { g, _ := buildFromSpec(s); return g }
	ref := runWithWatchdog(func(o *layoutOutcome) {
		g := build()
		uniqueIDCounter.Store(70703)
		defer func() { o.counter = uniqueIDCounter.Load() - 70703; o.dump = dumpGraph(g) }()
		if err := referenceLayout(g); err != nil {
			o.err = err.Error()
		}
	})
	t.Logf("in-tree Attrs engine vs original differs: %v", diffOutcomes(want, ref) != "")
	comparePhases(t, build, 70703)
	compareLatePhases(t, build, 70703)
}

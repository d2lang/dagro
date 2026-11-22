package dagro

// Helpers of the differential tests: graph specs applied through the Graph
// API (graphSpec), a generator of awkward inputs (specGenerator) and outcome
// comparison (diffOutcomes). The reference side (runSpecReference) lays a
// graph out with the Attrs engine, layoutAttrs, which runs the original
// algorithm; Layout, which takes the typed engine whenever it accepts the
// graph, must give the same result, error or panic value and advance the
// process-wide counter by as much. The generator's inputs (fractional and
// non-finite numbers, odd names, removals) reach the typed engine's declines
// and known failure sites, which random d2-shaped graphs rarely do.

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Canonical dumps (float bits included), identical format for both packages.

func writeFloatBits(b *strings.Builder, f float64) {
	fmt.Fprintf(b, "%v[%016x]", f, math.Float64bits(f))
}

func writeAttrsMap(b *strings.Builder, m map[string]any, isNil bool) {
	if isNil {
		b.WriteString("Attrs(nil)")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("{")
	for _, k := range keys {
		b.WriteString(strconv.Quote(k))
		b.WriteString(":")
		writeCanonical(b, m[k])
		b.WriteString(",")
	}
	b.WriteString("}")
}

func writeCanonical(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("nil")
	case float64:
		writeFloatBits(b, x)
	case string:
		b.WriteString(strconv.Quote(x))
	case Attrs:
		writeAttrsMap(b, map[string]any(x), x == nil)
	case []Point:
		if x == nil {
			b.WriteString("[]Point(nil)")
			return
		}
		b.WriteString("[")
		for _, p := range x {
			writeFloatBits(b, p.X)
			b.WriteString(",")
			writeFloatBits(b, p.Y)
			b.WriteString(";")
		}
		b.WriteString("]")
	case Edge:
		fmt.Fprintf(b, "Edge{%q,%q,%q,%v}", x.V, x.W, x.Name, x.HasName)
	default:
		s := fmt.Sprintf("%T:%#v", v, v)
		b.WriteString(s)
	}
}

func canonicalString(v any) string {
	var b strings.Builder
	writeCanonical(&b, v)
	return b.String()
}

func dumpGraph(g *Graph) string {
	var b strings.Builder
	b.WriteString("graph ")
	writeCanonical(&b, g.Graph())
	b.WriteString("\n")
	for _, v := range g.Nodes() {
		fmt.Fprintf(&b, "node %q ", v)
		writeCanonical(&b, g.Node(v))
		if g.IsCompound() {
			p, ok := g.Parent(v)
			fmt.Fprintf(&b, " parent=%q/%v children=%q", p, ok, g.Children(v))
		}
		b.WriteString("\n")
	}
	for _, e := range g.Edges() {
		fmt.Fprintf(&b, "edge %q %q %q %v ", e.V, e.W, e.Name, e.HasName)
		writeCanonical(&b, g.Edge(e))
		b.WriteString("\n")
	}
	return b.String()
}

func dumpReferenceGraph(g *Graph) string {
	var b strings.Builder
	b.WriteString("graph ")
	writeCanonical(&b, g.Graph())
	b.WriteString("\n")
	for _, v := range g.Nodes() {
		fmt.Fprintf(&b, "node %q ", v)
		writeCanonical(&b, g.Node(v))
		if g.IsCompound() {
			p, ok := g.Parent(v)
			fmt.Fprintf(&b, " parent=%q/%v children=%q", p, ok, g.Children(v))
		}
		b.WriteString("\n")
	}
	for _, e := range g.Edges() {
		fmt.Fprintf(&b, "edge %q %q %q %v ", e.V, e.W, e.Name, e.HasName)
		writeCanonical(&b, g.Edge(e))
		b.WriteString("\n")
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Package-neutral graph specs.

type specLabelKind uint8

const (
	specLabelAttrs  specLabelKind = iota
	specLabelNone                 // SetNode(v) / SetEdge without label
	specLabelNil                  // explicit nil label
	specLabelString               // a non-Attrs label
)

type specLabel struct {
	kind  specLabelKind
	attrs map[string]any
}

const (
	specOpSetNode = iota
	specOpSetParent
	specOpSetEdge
	specOpRemoveNode
	specOpRemoveEdge
)

type specOp struct {
	kind    int
	v, w    string
	name    string
	hasName bool
	label   specLabel
}

type graphSpec struct {
	compound, multigraph, undirected bool
	graph                            specLabel
	ops                              []specOp
}

func (s graphSpec) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "compound=%v multigraph=%v undirected=%v graph=%s\n", s.compound, s.multigraph, s.undirected, specLabelText(s.graph))
	for _, op := range s.ops {
		switch op.kind {
		case specOpSetNode:
			fmt.Fprintf(&b, "  SetNode(%q, %s)\n", op.v, specLabelText(op.label))
		case specOpSetParent:
			fmt.Fprintf(&b, "  SetParent(%q, %q)\n", op.v, op.w)
		case specOpSetEdge:
			fmt.Fprintf(&b, "  SetEdge(%q, %q, %s, name=%q/%v)\n", op.v, op.w, specLabelText(op.label), op.name, op.hasName)
		case specOpRemoveNode:
			fmt.Fprintf(&b, "  RemoveNode(%q)\n", op.v)
		case specOpRemoveEdge:
			fmt.Fprintf(&b, "  RemoveEdgeByArgs(%q, %q, name=%q/%v)\n", op.v, op.w, op.name, op.hasName)
		}
	}
	return b.String()
}

func specLabelText(l specLabel) string {
	switch l.kind {
	case specLabelNone:
		return "<none>"
	case specLabelNil:
		return "<nil>"
	case specLabelString:
		return "<string>"
	}
	return canonicalString(Attrs(l.attrs))
}

func copyAttrsMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (l specLabel) forNew() (any, bool) {
	switch l.kind {
	case specLabelNone:
		return nil, false
	case specLabelNil:
		return nil, true
	case specLabelString:
		return "label", true
	}
	return Attrs(copyAttrsMap(l.attrs)), true
}

func (l specLabel) forReference() (any, bool) {
	switch l.kind {
	case specLabelNone:
		return nil, false
	case specLabelNil:
		return nil, true
	case specLabelString:
		return "label", true
	}
	return Attrs(copyAttrsMap(l.attrs)), true
}

// buildFromSpec/buildReferenceFromSpec apply the spec (both through this package's Graph API); the transcript records errors and
// panics of the build calls so that they are compared too.
func buildFromSpec(s graphSpec) (g *Graph, transcript string) {
	var tr strings.Builder
	g = NewGraph(GraphOptions{Undirected: s.undirected, Compound: s.compound, Multigraph: s.multigraph})
	if lbl, ok := s.graph.forNew(); ok {
		g.SetGraph(lbl)
	}
	for i, op := range s.ops {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(&tr, "op %d panic: %v\n", i, r)
				}
			}()
			switch op.kind {
			case specOpSetNode:
				if lbl, ok := op.label.forNew(); ok {
					g.SetNode(op.v, lbl)
				} else {
					g.SetNode(op.v)
				}
			case specOpSetParent:
				if err := g.SetParent(op.v, op.w); err != nil {
					fmt.Fprintf(&tr, "op %d error: %v\n", i, err)
				}
			case specOpSetEdge:
				lbl, ok := op.label.forNew()
				switch {
				case op.hasName:
					g.SetEdge(op.v, op.w, lbl, op.name)
				case ok:
					g.SetEdge(op.v, op.w, lbl)
				default:
					g.SetEdge(op.v, op.w)
				}
			case specOpRemoveNode:
				g.RemoveNode(op.v)
			case specOpRemoveEdge:
				if op.hasName {
					g.RemoveEdgeByArgs(op.v, op.w, op.name)
				} else {
					g.RemoveEdgeByArgs(op.v, op.w)
				}
			}
		}()
	}
	return g, tr.String()
}

func buildReferenceFromSpec(s graphSpec) (g *Graph, transcript string) {
	var tr strings.Builder
	g = NewGraph(GraphOptions{Undirected: s.undirected, Compound: s.compound, Multigraph: s.multigraph})
	if lbl, ok := s.graph.forReference(); ok {
		g.SetGraph(lbl)
	}
	for i, op := range s.ops {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(&tr, "op %d panic: %v\n", i, r)
				}
			}()
			switch op.kind {
			case specOpSetNode:
				if lbl, ok := op.label.forReference(); ok {
					g.SetNode(op.v, lbl)
				} else {
					g.SetNode(op.v)
				}
			case specOpSetParent:
				if err := g.SetParent(op.v, op.w); err != nil {
					fmt.Fprintf(&tr, "op %d error: %v\n", i, err)
				}
			case specOpSetEdge:
				lbl, ok := op.label.forReference()
				switch {
				case op.hasName:
					g.SetEdge(op.v, op.w, lbl, op.name)
				case ok:
					g.SetEdge(op.v, op.w, lbl)
				default:
					g.SetEdge(op.v, op.w)
				}
			case specOpRemoveNode:
				g.RemoveNode(op.v)
			case specOpRemoveEdge:
				if op.hasName {
					g.RemoveEdgeByArgs(op.v, op.w, op.name)
				} else {
					g.RemoveEdgeByArgs(op.v, op.w)
				}
			}
		}()
	}
	return g, tr.String()
}

// ---------------------------------------------------------------------------
// Running with a watchdog.

type layoutOutcome struct {
	build    string
	dump     string
	err      string
	panicked string
	counter  uint64
	hung     bool
}

var outcomeTimeout = 20 * time.Second

var panicTextDiffs = map[string]int{}

func runWithWatchdog(f func(*layoutOutcome)) layoutOutcome {
	done := make(chan layoutOutcome, 1)
	go func() {
		var o layoutOutcome
		defer func() {
			if r := recover(); r != nil {
				o.panicked = fmt.Sprint(r)
			}
			done <- o
		}()
		f(&o)
	}()
	select {
	case o := <-done:
		return o
	case <-time.After(outcomeTimeout):
		return layoutOutcome{hung: true}
	}
}

// referenceLayoutOpts lays g out with the Attrs engine (layoutAttrs), which
// runs the original algorithm, and prints the DebugTiming lines as Layout
// does.
func referenceLayoutOpts(g *Graph, options ...LayoutOptions) error {
	debug := len(options) > 0 && options[0].DebugTiming
	return layoutAttrs(g, func(name string, fn func() error) error {
		if !debug {
			return fn()
		}
		start := time.Now()
		err := fn()
		fmt.Printf("%s time: %dms\n", name, time.Since(start).Milliseconds())
		return err
	})
}

func runSpecLayout(s graphSpec, start uint64) layoutOutcome {
	return runWithWatchdog(func(o *layoutOutcome) {
		g, tr := buildFromSpec(s)
		o.build = tr
		uniqueIDCounter.Store(start)
		defer func() {
			o.counter = uniqueIDCounter.Load() - start
			o.dump = dumpGraph(g)
		}()
		if err := Layout(g); err != nil {
			o.err = err.Error()
		}
	})
}

func runSpecReference(s graphSpec, start uint64) layoutOutcome {
	return runWithWatchdog(func(o *layoutOutcome) {
		g, tr := buildReferenceFromSpec(s)
		o.build = tr
		uniqueIDCounter.Store(start)
		defer func() {
			o.counter = uniqueIDCounter.Load() - start
			o.dump = dumpReferenceGraph(g)
		}()
		if err := referenceLayoutOpts(g); err != nil {
			o.err = err.Error()
		}
	})
}

func diffOutcomes(a, b layoutOutcome) string {
	var d []string
	if a.hung != b.hung {
		d = append(d, fmt.Sprintf("hang: orig=%v new=%v", a.hung, b.hung))
	}
	if a.build != b.build {
		d = append(d, fmt.Sprintf("build transcript:\norig %q\nnew  %q", a.build, b.build))
	}
	if a.panicked != "" && b.panicked != "" && a.panicked != b.panicked && os.Getenv("DAGRO_DIFF_PANICTEXT") == "0" {
		panicTextDiffs[a.panicked+" | "+b.panicked]++
		return ""
	}
	if a.panicked != b.panicked {
		d = append(d, fmt.Sprintf("panic:\norig %q\nnew  %q", a.panicked, b.panicked))
	}
	if a.err != b.err {
		d = append(d, fmt.Sprintf("error:\norig %q\nnew  %q", a.err, b.err))
	}
	if a.counter != b.counter {
		d = append(d, fmt.Sprintf("counter draws: orig=%d new=%d", a.counter, b.counter))
	}
	if a.dump != b.dump {
		al, bl := strings.Split(a.dump, "\n"), strings.Split(b.dump, "\n")
		n := 0
		for i := 0; i < len(al) || i < len(bl); i++ {
			var x, y string
			if i < len(al) {
				x = al[i]
			}
			if i < len(bl) {
				y = bl[i]
			}
			if x != y {
				d = append(d, fmt.Sprintf("dump line %d:\norig %s\nnew  %s", i, x, y))
				n++
				if n >= 4 {
					break
				}
			}
		}
	}
	return strings.Join(d, "\n")
}

// ---------------------------------------------------------------------------
// Generator.

type specGenerator struct {
	rng *rand.Rand
	// feature switches (per run)
	fracMinlen   bool
	fracWeight   bool
	zeroWeight   bool
	minlen0      bool
	noClusters   bool
	nonDyadic    bool
	oddNames     bool
	weirdWeights bool
	greedy0      bool
	weirdNums    bool
	weirdLabels  bool
	removals     bool
	undirected   bool
	nonCompound  bool
}

func (z *specGenerator) pickS(xs ...string) string { return xs[z.rng.Intn(len(xs))] }
func (z *specGenerator) pickA(xs ...any) any       { return xs[z.rng.Intn(len(xs))] }
func (z *specGenerator) p(n int) bool              { return z.rng.Intn(n) == 0 }

func (z *specGenerator) sizeValue() any {
	if z.weirdNums && z.p(6) {
		return z.pickA(math.Copysign(0, -1), -5.0, math.NaN(), math.Inf(1), "12", "0x10", "", nil, true, 3, int64(7), float32(2.5), " 8 ", "abc", "1e2")
	}
	switch z.rng.Intn(6) {
	case 0:
		return 0.0
	case 1:
		return float64(z.rng.Intn(10)) + 0.5
	default:
		return float64(1 + z.rng.Intn(150))
	}
}

func (z *specGenerator) sepValue() any {
	if z.weirdNums && z.p(5) {
		return z.pickA(math.Copysign(0, -1), -7.0, math.NaN(), math.Inf(1), "30", "", nil, false, 25, 0.1)
	}
	switch z.rng.Intn(5) {
	case 0:
		return 0.0
	case 1:
		return float64(z.rng.Intn(40)) + 0.25
	default:
		return float64(5 + z.rng.Intn(100))
	}
}

func (z *specGenerator) graphLabel() specLabel {
	if z.weirdLabels && z.p(15) {
		return specLabel{kind: specLabelKind(1 + z.rng.Intn(3))}
	}
	a := map[string]any{}
	switch z.rng.Intn(12) {
	case 0:
	case 1:
		a["rankdir"] = z.pickS("tb", "bt", "lr", "rl", "Lr", "rL", "x", "", " TB", "T\u0130")
	case 2:
		a["rankDir"] = z.pickS("LR", "BT", "RL", "TB")
	case 3:
		a["rankdir"] = z.pickA(7.0, nil, true)
	default:
		a["rankdir"] = z.pickS("TB", "BT", "LR", "RL")
	}
	for _, k := range []string{"nodesep", "edgesep", "ranksep"} {
		if z.p(4) {
			continue
		}
		a[k] = z.sepValue()
	}
	if z.p(4) {
		a["marginx"] = z.sepValue()
	}
	if z.p(4) {
		a["marginy"] = z.sepValue()
	}
	if z.p(4) {
		a["ranker"] = z.pickA("network-simplex", "tight-tree", "longest-path", "other", "", nil, 3.0)
	}
	if z.p(6) {
		a["acyclicer"] = z.pickA("greedy", "dfs", "Greedy", nil)
	}
	if z.p(5) {
		a["align"] = z.pickA("UL", "ur", "Dl", "dr", "x", "", nil, 1.0)
	}
	if z.p(8) {
		a["unrelated"] = "keep me"
	}
	return specLabel{attrs: a}
}

func (z *specGenerator) nodeLabel() specLabel {
	if z.weirdLabels && z.p(12) {
		return specLabel{kind: specLabelKind(1 + z.rng.Intn(3))}
	}
	a := map[string]any{}
	switch z.rng.Intn(14) {
	case 0:
	case 1:
		a["WIDTH"], a["Height"] = z.sizeValue(), z.sizeValue()
	case 2:
		a["width"] = z.sizeValue()
	default:
		a["width"], a["height"] = z.sizeValue(), z.sizeValue()
	}
	if z.p(5) {
		a["x"] = 123.0 // pre-existing coordinates are overwritten
	}
	if z.p(6) {
		a["label"] = "text"
	}
	return specLabel{attrs: a}
}

func (z *specGenerator) edgeLabel() specLabel {
	if z.weirdLabels && z.p(30) {
		return specLabel{kind: specLabelKind(1 + z.rng.Intn(3))}
	}
	a := map[string]any{}
	switch z.rng.Intn(6) {
	case 0:
	case 1:
		a["width"], a["height"] = z.sizeValue(), z.sizeValue()
	default:
		a["width"], a["height"] = z.sizeValue(), z.sizeValue()
		a["labelpos"] = z.pickA("c", "l", "r", "L", "R", "C", "c", "c", "", "x")
		if z.weirdNums && z.p(10) {
			a["labelpos"] = z.pickA(3.0, nil, true)
		}
	}
	if z.minlen0 && z.p(4) {
		a["minlen"] = z.pickA(0.0, -1.0, -2.0, math.NaN(), 0.0, 7.0, -0.0, math.Copysign(0, -1))
	} else if z.p(5) {
		if z.fracMinlen && z.p(2) {
			a["minlen"] = z.pickA(0.5, 1.5, 2.5, 0.25, 3.75, 1.25)
		} else {
			a["minlen"] = z.pickA(1.0, 2.0, 3.0, "2", true, 1)
		}
	}
	if z.p(5) {
		switch {
		case z.greedy0 && z.p(3):
			a["weight"] = z.pickA(0.0, math.Copysign(0, -1), math.NaN())
		case z.weirdWeights && z.p(2):
			a["weight"] = z.pickA(math.NaN(), math.Inf(1), -1.0, -0.5, 1e300, math.Copysign(0, -1), 4.5e15, "x")
		case z.nonDyadic && z.p(2):
			a["weight"] = z.pickA(0.1, 0.3, 0.7, 1.1, 2.3, 1.0/3, 0.2)
		case z.zeroWeight && z.p(4):
			a["weight"] = 0.0
		case z.fracWeight && z.p(2):
			a["weight"] = z.pickA(0.5, 1.5, 0.25, 2.5, 0.75)
		default:
			a["weight"] = z.pickA(1.0, 2.0, 3.0, 5.0, "4", 2)
		}
	}
	if z.p(6) {
		a["labeloffset"] = z.pickA(0.0, 5.0, 12.0, 2.5, -3.0)
	}
	if z.p(15) {
		if _, ok := a["labelpos"]; !ok {
			a["LabelPos"] = z.pickS("l", "c", "r")
		}
	}
	if z.p(10) {
		a["x"] = 5.0
	}
	return specLabel{attrs: a}
}

func (z *specGenerator) names(n int) []string {
	mode := z.rng.Intn(8)
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for i := 0; len(out) < n && i < 4*n; i++ {
		var s string
		switch mode {
		case 0, 1:
			s = strconv.Itoa(i)
		case 2:
			s = strconv.Itoa(z.rng.Intn(3 * n))
		case 3:
			s = string(rune('a'+i%26)) + strconv.Itoa(i/26)
		case 4:
			s = z.pickS("_d", "_d1", "_d7", "_d12", "_root", "_root1", "_root2", "_bt", "_bt1", "_bb", "_bl", "_br",
				"_se", "_ep", "_ep3", "_root3", "01", "4294967295", "4294967294", "-1", "1.5", "z", strconv.Itoa(i),
				"\u00e9", "Z", "rev1", "_", "__d1")
		case 5:
			s = z.pickS("\uff61", "\U0001F600", "a", "\u00e9", "\u65e5\u672c", "A", "b", "0", "10", "9", "\ud7ff", "\U00010000", "x\U0001F600", "x\uff61")
			if seen[s] {
				s = s + strconv.Itoa(i)
			}
		case 6:
			if z.p(2) {
				s = strconv.Itoa(z.rng.Intn(1000))
			} else {
				s = fmt.Sprintf("n%d", z.rng.Intn(1000))
			}
		default:
			s = strconv.Itoa(i)
			if z.p(3) {
				s = "_" + s
			}
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if mode == 1 {
		z.rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	if z.oddNames && len(out) > 1 {
		out[z.rng.Intn(len(out))] = z.pickS("", "a\x01b", "\x01", "x\x01")
	}
	return out
}

func (z *specGenerator) spec(maxNodes int) graphSpec {
	s := graphSpec{compound: true, multigraph: true}
	if z.nonCompound && z.p(8) {
		s.compound = false
	}
	if z.undirected && z.p(10) {
		s.undirected = true
	}
	if z.p(15) {
		s.multigraph = false
	}
	s.graph = z.graphLabel()
	if z.greedy0 && s.graph.kind == specLabelAttrs {
		s.graph.attrs["acyclicer"] = "greedy"
	}
	if z.nonDyadic && s.graph.kind == specLabelAttrs {
		s.graph.attrs["ranker"] = z.pickS("longest-path", "tight-tree")
	}
	nodeCount := 1 + z.rng.Intn(maxNodes)
	ids := z.names(nodeCount)
	var clusters []string
	if s.compound && !z.noClusters {
		for c := z.rng.Intn(5); c > 0; c-- {
			name := z.pickS(fmt.Sprintf("c%d", c), strconv.Itoa(100+c), fmt.Sprintf("_c%d", c), fmt.Sprintf("_bt%d", c))
			if !contains(clusters, name) && !contains(ids, name) {
				clusters = append(clusters, name)
			}
		}
	}
	order := append(append([]string(nil), clusters...), ids...)
	if z.p(3) {
		z.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	}
	for _, v := range order {
		s.ops = append(s.ops, specOp{kind: specOpSetNode, v: v, label: z.nodeLabel()})
	}
	parentOf := map[string]string{}
	for i, c := range clusters {
		if i > 0 && z.p(2) {
			parentOf[c] = clusters[z.rng.Intn(i)]
		}
	}
	for _, v := range order {
		if p, ok := parentOf[v]; ok {
			s.ops = append(s.ops, specOp{kind: specOpSetParent, v: v, w: p})
		} else if len(clusters) > 0 && !contains(clusters, v) && z.rng.Intn(3) != 0 {
			s.ops = append(s.ops, specOp{kind: specOpSetParent, v: v, w: clusters[z.rng.Intn(len(clusters))]})
		}
	}
	if s.compound && z.p(10) && len(order) > 1 {
		// re-parenting, including to the same parent and back to the root
		v := order[z.rng.Intn(len(order))]
		if !contains(clusters, v) {
			s.ops = append(s.ops, specOp{kind: specOpSetParent, v: v, w: z.pickS(append([]string{""}, clusters...)...)})
		}
	}
	leaves := append([]string(nil), ids...)
	if z.p(30) && len(clusters) > 0 {
		leaves = append(leaves, clusters[0])
	}
	edgeCount := z.rng.Intn(3*len(ids) + 1)
	for i := 0; i < edgeCount; i++ {
		v, w := leaves[z.rng.Intn(len(leaves))], leaves[z.rng.Intn(len(leaves))]
		if z.p(10) {
			w = v
		}
		if z.greedy0 && z.p(2) && len(s.ops) > 0 {
			if last := s.ops[len(s.ops)-1]; last.kind == specOpSetEdge {
				v, w = last.w, last.v
			}
		}
		op := specOp{kind: specOpSetEdge, v: v, w: w, label: z.edgeLabel()}
		if s.multigraph {
			switch z.rng.Intn(10) {
			case 0:
			case 1:
				op.name, op.hasName = z.pickS("rev1", "rev2", "rev3", "", "rev01", "x"), true
			case 2:
				op.name, op.hasName = defaultEdgeName, true
			default:
				op.name, op.hasName = fmt.Sprintf("(%s -> %s)[%d]", v, w, i), true
			}
		}
		s.ops = append(s.ops, op)
		if z.removals && z.p(12) {
			s.ops = append(s.ops, specOp{kind: specOpRemoveEdge, v: op.v, w: op.w, name: op.name, hasName: op.hasName})
			if z.p(2) {
				s.ops = append(s.ops, op)
			}
		}
	}
	if z.removals && z.p(6) && len(ids) > 2 {
		v := ids[z.rng.Intn(len(ids))]
		s.ops = append(s.ops, specOp{kind: specOpRemoveNode, v: v})
		if z.p(2) {
			s.ops = append(s.ops, specOp{kind: specOpSetNode, v: v, label: z.nodeLabel()})
		}
	}
	return s
}

func envInt(name string, def int) int {
	if s := os.Getenv(name); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

func newSpecGenerator(seed int64, features string) *specGenerator {
	z := &specGenerator{rng: rand.New(rand.NewSource(seed))}
	all := features == "all"
	z.fracMinlen = all || strings.Contains(features, "fracminlen")
	z.fracWeight = all || strings.Contains(features, "fracweight")
	z.zeroWeight = strings.Contains(features, "zeroweight")
	z.minlen0 = strings.Contains(features, "minlen0")
	z.noClusters = strings.Contains(features, "noclusters")
	z.nonDyadic = strings.Contains(features, "nondyadic")
	z.oddNames = strings.Contains(features, "oddnames")
	z.weirdWeights = strings.Contains(features, "weirdweights")
	z.greedy0 = strings.Contains(features, "greedy0")
	z.weirdNums = all || strings.Contains(features, "weirdnums")
	z.weirdLabels = all || strings.Contains(features, "weirdlabels")
	z.removals = all || strings.Contains(features, "removals")
	z.undirected = all || strings.Contains(features, "undirected")
	z.nonCompound = all || strings.Contains(features, "noncompound")
	return z
}

func attrsLabel(kv ...any) specLabel {
	m := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return specLabel{attrs: m}
}

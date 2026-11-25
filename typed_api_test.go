package dagro

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// typedAPIInput converts a diffInput into the LayoutInput that
// buildLayoutGraph would read from it (lower-cased keys, number coercion,
// defaults), so that LayoutTyped(in) and Layout(in.Graph()) can be compared
// on loosely typed inputs too.
func typedAPIInput(d diffInput) (*LayoutInput, bool) {
	in := DefaultLayoutInput()
	var vals [9]any
	var found [9]bool
	if d.Graph != nil {
		if !attrLookup(d.Graph, typedGraphKeys, vals[:], found[:]) {
			return nil, false // the result depends on map order
		}
		nums := [5]*float64{&in.NodeSep, &in.EdgeSep, &in.RankSep, &in.MarginX, &in.MarginY}
		for i, p := range nums {
			if found[i] {
				*p = number(vals[i])
			}
		}
		strs := [4]*string{&in.Acyclicer, &in.Ranker, &in.RankDir, &in.Align}
		for i, p := range strs {
			if found[5+i] {
				*p = anyString(vals[5+i])
			}
		}
	}
	index := map[string]int{}
	for i, n := range d.Nodes {
		if _, dup := index[n.ID]; dup {
			return nil, false
		}
		index[n.ID] = i
	}
	for _, n := range d.Nodes {
		node := LayoutNode{ID: n.ID, Parent: -1}
		if !attrLookup(n.Attrs, typedNodeKeys, vals[:2], found[:2]) {
			return nil, false
		}
		if found[0] {
			node.Width = number(vals[0])
		}
		if found[1] {
			node.Height = number(vals[1])
		}
		if n.Parent != nil {
			p, ok := index[*n.Parent]
			if !ok {
				return nil, false
			}
			node.Parent = p
		}
		in.Nodes = append(in.Nodes, node)
	}
	for _, e := range d.Edges {
		v, okV := index[e.V]
		w, okW := index[e.W]
		if !okV || !okW {
			return nil, false
		}
		edge := NewLayoutEdge(v, w)
		if !attrLookup(e.Attrs, typedEdgeKeys, vals[:6], found[:6]) {
			return nil, false
		}
		nums := [5]*float64{&edge.Minlen, &edge.Weight, &edge.Width, &edge.Height, &edge.LabelOffset}
		for j, p := range nums {
			if found[j] {
				*p = number(vals[j])
			}
		}
		if found[5] {
			edge.LabelPos = anyString(vals[5])
		}
		if e.Name != nil {
			edge.Name, edge.HasName = *e.Name, true
		}
		in.Edges = append(in.Edges, edge)
	}
	return &in, true
}

func sameBits(a, b float64) bool { return math.Float64bits(a) == math.Float64bits(b) }

// typedAPIRead reads the result of Layout(g) the way LayoutTyped reports it.
func typedAPIRead(in *LayoutInput, g *Graph) LayoutOutput {
	var out LayoutOutput
	ga := asAttrs(g.Graph())
	out.Width, out.Height = num(ga, "width"), num(ga, "height")
	for _, n := range in.Nodes {
		a := asAttrs(g.Node(n.ID))
		out.Nodes = append(out.Nodes, NodeLayout{X: num(a, "x"), Y: num(a, "y"), Width: num(a, "width"), Height: num(a, "height")})
	}
	for _, e := range in.Edges {
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
		out.Edges = append(out.Edges, el)
	}
	return out
}

// layoutViaGraph lays out in.Graph() with Layout and reads the result the way
// LayoutTyped reports it: what LayoutTyped(in, out) is defined to equal. It
// is the same in every build (LayoutTyped's own fallback is not, see
// typed_fallback.go).
func layoutViaGraph(in *LayoutInput, out *LayoutOutput) error {
	g, err := in.Graph()
	if err != nil {
		return err
	}
	if err := Layout(g); err != nil {
		return err
	}
	*out = typedAPIRead(in, g)
	return nil
}

func typedAPICompare(t *testing.T, name string, want, got LayoutOutput) bool {
	t.Helper()
	if !sameBits(want.Width, got.Width) || !sameBits(want.Height, got.Height) {
		t.Errorf("%s: graph size %v x %v, typed %v x %v", name, want.Width, want.Height, got.Width, got.Height)
		return false
	}
	if len(want.Nodes) != len(got.Nodes) || len(want.Edges) != len(got.Edges) {
		t.Errorf("%s: result lengths differ", name)
		return false
	}
	for i := range want.Nodes {
		a, b := want.Nodes[i], got.Nodes[i]
		if !sameBits(a.X, b.X) || !sameBits(a.Y, b.Y) || !sameBits(a.Width, b.Width) || !sameBits(a.Height, b.Height) {
			t.Errorf("%s: node %d: %+v, typed %+v", name, i, a, b)
			return false
		}
	}
	for i := range want.Edges {
		a, b := want.Edges[i], got.Edges[i]
		if a.HasXY != b.HasXY || !sameBits(a.X, b.X) || !sameBits(a.Y, b.Y) ||
			len(a.Points) != len(b.Points) || (a.Points == nil) != (b.Points == nil) {
			t.Errorf("%s: edge %d: %+v, typed %+v", name, i, a, b)
			return false
		}
		for j := range a.Points {
			if !sameBits(a.Points[j].X, b.Points[j].X) || !sameBits(a.Points[j].Y, b.Points[j].Y) {
				t.Errorf("%s: edge %d point %d: %v, typed %v", name, i, j, a.Points[j], b.Points[j])
				return false
			}
		}
	}
	return true
}

// typedAPICheck runs Layout(in.Graph()) and LayoutTyped(in) from the same
// unique-id counter value and compares results, counter use and panics.
func typedAPICheck(t *testing.T, name string, in *LayoutInput) bool {
	t.Helper()
	start := uint64(1000 + rand.Intn(100000))
	var want, got LayoutOutput
	var wantErr, gotErr error
	var wantPanic, gotPanic any
	var wantCounter, gotCounter uint64
	func() {
		defer func() { wantPanic = recover(); wantCounter = uniqueIDCounter.Load() }()
		uniqueIDCounter.Store(start)
		g, err := in.Graph()
		if err != nil {
			wantErr = err
			return
		}
		if wantErr = Layout(g); wantErr == nil {
			want = typedAPIRead(in, g)
		}
	}()
	func() {
		defer func() { gotPanic = recover(); gotCounter = uniqueIDCounter.Load() }()
		uniqueIDCounter.Store(start)
		gotErr = LayoutTyped(in, &got)
	}()
	if !graphFallback {
		// Without the Graph fallback LayoutTyped equals Layout(in.Graph())
		// only where the typed engine lays the input out.
		switch typedOnlyKind(in, start) {
		case kindNotTyped:
			if wantErr := notTypedError(in); gotPanic != nil || gotErr == nil || gotErr.Error() != wantErr.Error() || gotCounter != start {
				t.Errorf("%s: an input the typed engine does not accept: error %v, panic %v, %d draws; want error %v", name, gotErr, gotPanic, gotCounter-start, wantErr)
				return false
			}
			return true
		case kindGaveUp:
			if gotPanic == nil {
				t.Errorf("%s: the typed engine gives up on the input, but LayoutTyped returned %v", name, gotErr)
				return false
			}
			return true
		}
	}
	if (wantPanic != nil) != (gotPanic != nil) {
		t.Errorf("%s: panic mismatch: Graph %v, typed %v", name, wantPanic, gotPanic)
		return false
	}
	if wantPanic != nil {
		return true
	}
	if (wantErr == nil) != (gotErr == nil) || wantErr != nil && wantErr.Error() != gotErr.Error() {
		t.Errorf("%s: error mismatch: Graph %v, typed %v", name, wantErr, gotErr)
		return false
	}
	if wantErr != nil {
		return true
	}
	if wantCounter != gotCounter {
		t.Errorf("%s: unique-id counter %d (Graph) vs %d (typed)", name, wantCounter, gotCounter)
		return false
	}
	return typedAPICompare(t, name, want, got)
}

func TestLayoutTypedMatchesGraphRandom(t *testing.T) {
	cases := 500
	if s := os.Getenv("DAGRO_TYPED_DIFF_CASES"); s != "" {
		cases, _ = strconv.Atoi(s)
	}
	seed := int64(77)
	if s := os.Getenv("DAGRO_TYPED_DIFF_SEED"); s != "" {
		seed, _ = strconv.ParseInt(s, 10, 64)
	}
	rng := rand.New(rand.NewSource(seed))
	checked, failures := 0, 0
	for i := 0; i < cases; i++ {
		in, ok := typedAPIInput(typedRandomInput(rng))
		if !ok {
			continue
		}
		checked++
		if !typedAPICheck(t, fmt.Sprintf("seed %d case %d", seed, i), in) {
			failures++
			if failures >= 5 {
				t.Fatalf("stopping after %d failures", failures)
			}
		}
	}
	t.Logf("%d of %d random inputs checked", checked, cases)
}

func TestLayoutTypedFixtures(t *testing.T) {
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{
		{ID: "0", Parent: -1, Width: 120, Height: 80},
		{ID: "1", Parent: 0, Width: 40, Height: 30},
		{ID: "2", Parent: 0, Width: 50, Height: 30},
		{ID: "3", Parent: -1, Width: 60, Height: 40},
	}
	e := NewLayoutEdge(1, 2)
	e.Name, e.HasName, e.Width, e.Height, e.LabelPos = "(1 -> 2)[0]", true, 20, 10, "c"
	in.Edges = []LayoutEdge{e, NewLayoutEdge(2, 3), NewLayoutEdge(3, 1), NewLayoutEdge(3, 3)}
	typedAPICheck(t, "small", &in)

	// Inputs that take the Graph path.
	dup := in
	dup.Nodes = append([]LayoutNode(nil), in.Nodes...)
	dup.Nodes[3].ID = "1"
	typedAPICheck(t, "duplicate-id", &dup)
	cycle := in
	cycle.Nodes = append([]LayoutNode(nil), in.Nodes...)
	cycle.Nodes[0].Parent = 1
	typedAPICheck(t, "parent-cycle", &cycle)
	twice := in
	twice.Edges = append(append([]LayoutEdge(nil), in.Edges...), in.Edges[1])
	typedAPICheck(t, "duplicate-edge", &twice)
	empty := in
	empty.Nodes = append([]LayoutNode(nil), in.Nodes...)
	empty.Nodes[3].ID = ""
	typedAPICheck(t, "empty-id", &empty)
	bad := in
	bad.Edges = []LayoutEdge{NewLayoutEdge(0, 9)}
	var out LayoutOutput
	if err := LayoutTyped(&bad, &out); err == nil {
		t.Errorf("out-of-range endpoint: no error")
	}
}

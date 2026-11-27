package dagro

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// Tests of order's repair of the layerings that dagre's constraint cycles
// leave inconsistent (see order and takeIn).

// loadKnownSite reads a LayoutInput fixture of testdata/knownsites.
func loadKnownSite(t testing.TB, name string) LayoutInput {
	t.Helper()
	data, err := os.ReadFile("testdata/knownsites/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var in LayoutInput
	if err := json.Unmarshal(data, &in); err != nil {
		t.Fatal(err)
	}
	return in
}

// tinyConstraintCycle is the smallest input known on which dagre's ordering
// closes a constraint cycle: the layout D2 asks for the diagram
//
//	a.x
//	b.p -> c.q
//	a.x -> b.r
//	b.p -> d.s
//	c.q -> b.p
//	b.p -> a.y
//
// In the first sweep down the ranks, d's entry merges into b's at rank 4 and
// the merged entry sorts before c, against the constraint c -> d of rank 3;
// the constraints then hold the cycle c -> d -> b -> c, and b, c and d are
// left out of rank 5.
func tinyConstraintCycle() LayoutInput {
	in := DefaultLayoutInput()
	in.RankSep, in.NodeSep = 100, 60
	for i, p := range []int{-1, 0, -1, 2, -1, 4, 2, -1, 7, 0} {
		in.Nodes = append(in.Nodes, LayoutNode{ID: string(rune('0' + i)), Parent: p, Width: 60, Height: 66})
	}
	for _, e := range [][2]int{{3, 5}, {1, 6}, {3, 8}, {5, 3}, {3, 9}} {
		le := NewLayoutEdge(e[0], e[1])
		le.LabelPos = "c"
		in.Edges = append(in.Edges, le)
	}
	return in
}

type constraintCycleInput struct {
	name string
	in   LayoutInput
	gap  int    // what is wrong with the layering of order's first run
	was  string // the error of dagro v0.4.0
	// The layout's goldenHash, and the values it takes from the counter: as
	// many as v0.4.0's failed attempt took, since these inputs have no
	// self-loops.
	hash  string
	draws uint64
}

// Errors of dagro v0.4.0 on the inputs of constraintCycleInputs.
const (
	errOrderGap = `dagro: layout failed: dagre fails on this input: it sets "order" on a node missing from a layer`
	errRankGap  = `dagro: layout failed: dagre fails on this input: it sets "rank" on a node missing from a layer`
)

// constraintCycleInputs are layouts of D2 diagrams on which dagre's ordering
// closes a cycle among sibling clusters in a sweep's constraint graph, and on
// which dagro v0.4.0 failed. The first run of order leaves each kind of
// inconsistent layering: an empty position in o.best, a node missing from
// o.best that leaves one in the orders, two nodes on one position. The
// takeover needs its walk over the entries an absorbed entry absorbed before
// on takeover_absorbed.json, and absorbs entries below the first level on
// takeover_nested.json.
func constraintCycleInputs(t testing.TB) []constraintCycleInput {
	return []constraintCycleInput{
		{"tiny", tinyConstraintCycle(), gapEmpty, errOrderGap, "6dfb2fa9894a34a2", 60},
		{"assignorder_layer_gap.json", loadKnownSite(t, "assignorder_layer_gap.json"), gapEmpty, errOrderGap,
			"04faf7e2f0d11aae", 395},
		// A D2 diagram of 25 objects and 18 connections.
		{"assignorder_layer_gap_min.json", loadKnownSite(t, "assignorder_layer_gap_min.json"), gapEmpty, errOrderGap,
			"7f118d0f6d63514d", 395},
		{"insertselfedges_layer_gap.json", loadKnownSite(t, "insertselfedges_layer_gap.json"), gapMissing, errRankGap,
			"8bfe08aed3a58d06", 578},
		// A D2 diagram of 33 objects and 37 connections.
		{"intersect_lost_node.json", loadKnownSite(t, "intersect_lost_node.json"), gapLost, errIntersectInside.Error(),
			"5b87ca33545a870f", 592},
		// Generated D2 diagrams of 83 objects and 78 connections, and of 103
		// objects and 62 connections.
		{"takeover_absorbed.json", loadKnownSite(t, "takeover_absorbed.json"), gapEmpty, errOrderGap,
			"e4dd1f93f16c3a41", 1149},
		{"takeover_nested.json", loadKnownSite(t, "takeover_nested.json"), gapEmpty, errOrderGap,
			"29834c634bfc8325", 610},
	}
}

// childrenOutside lists the nodes of in whose box in out is not inside their
// parent's.
func childrenOutside(in *LayoutInput, out *LayoutOutput) []string {
	var bad []string
	for i, n := range in.Nodes {
		if n.Parent < 0 {
			continue
		}
		c, p := out.Nodes[i], out.Nodes[n.Parent]
		if !(c.X-c.Width/2 >= p.X-p.Width/2 && c.X+c.Width/2 <= p.X+p.Width/2 &&
			c.Y-c.Height/2 >= p.Y-p.Height/2 && c.Y+c.Height/2 <= p.Y+p.Height/2) {
			bad = append(bad, n.ID+" in "+in.Nodes[n.Parent].ID)
		}
	}
	return bad
}

// finiteLayout reports whether every number of out is finite.
func finiteLayout(out *LayoutOutput) bool {
	finite := func(xs ...float64) bool {
		for _, x := range xs {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return false
			}
		}
		return true
	}
	if !finite(out.Width, out.Height) {
		return false
	}
	for _, n := range out.Nodes {
		if !finite(n.X, n.Y, n.Width, n.Height) {
			return false
		}
	}
	for _, e := range out.Edges {
		for _, p := range e.Points {
			if !finite(p.X, p.Y) {
				return false
			}
		}
		if e.HasXY && !finite(e.X, e.Y) {
			return false
		}
	}
	return true
}

// TestConstraintCycleInputsLayOut checks that the inputs on which dagre's
// ordering leaves clusters out of a layer lay out, with finite numbers, every
// child inside its parent and the pinned results; that order's first run
// detects how its layering is inconsistent; and that the layout LayoutTyped
// repeats after a lost node takes no values from the process-wide counter.
func TestConstraintCycleInputsLayOut(t *testing.T) {
	for _, c := range constraintCycleInputs(t) {
		in := c.in
		// One layout, without LayoutTyped's repeat: order repairs the
		// layerings with an empty position by itself.
		s := new(lstate)
		if err := s.buildTyped(&in); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var once LayoutOutput
		uniqueIDCounter.Store(goldenCounterStart)
		err := s.runGuarded(&in, &once)
		draws := uniqueIDCounter.Load() - goldenCounterStart
		if s.o.gap != c.gap {
			t.Errorf("%s: the first run's layering has gap %d, want %d", c.name, s.o.gap, c.gap)
		}
		if c.gap == gapLost {
			// The layout fails as before; LayoutTyped repeats it.
			if err == nil || err.Error() != c.was {
				t.Errorf("%s: one layout gives error %v, want %q", c.name, err, c.was)
			}
		} else if err != nil {
			t.Errorf("%s: %v (dagro v0.4.0: %s)", c.name, err, c.was)
		}

		var out LayoutOutput
		uniqueIDCounter.Store(goldenCounterStart)
		if err := LayoutTyped(&in, &out); err != nil {
			t.Errorf("%s: %v (dagro v0.4.0: %s)", c.name, err, c.was)
			continue
		}
		if got := uniqueIDCounter.Load() - goldenCounterStart; got != draws {
			t.Errorf("%s: LayoutTyped took %d values from the counter, one layout %d", c.name, got, draws)
		}
		if hash := goldenHash(goldenDump(&out)); hash != c.hash || draws != c.draws {
			t.Errorf("%s: layout %s with %d draws, pinned %s with %d", c.name, hash, draws, c.hash, c.draws)
		}
		if !finiteLayout(&out) {
			t.Errorf("%s: the layout has non-finite numbers", c.name)
		}
		if bad := childrenOutside(&in, &out); len(bad) > 0 {
			t.Errorf("%s: nodes outside their parents: %v", c.name, bad)
		}
		if c.gap != gapLost && goldenDump(&out) != goldenDump(&once) {
			t.Errorf("%s: LayoutTyped's layout differs from one run's", c.name)
		}
	}
}

// TestLostNodeLayoutUnchanged checks an input on which order's first run
// puts two nodes on one position twice, and the layout goes on without the
// left border nodes of a cluster at two ranks. It lays out (with a child
// outside its cluster's box), so it keeps the layout of dagro v0.4.0 bit for
// bit, and begin resets the gap for the next layout.
func TestLostNodeLayoutUnchanged(t *testing.T) {
	in := loadKnownSite(t, "lost_inner_border.json")
	s := new(lstate)
	if err := s.buildTyped(&in); err != nil {
		t.Fatal(err)
	}
	var once LayoutOutput
	if err := s.runGuarded(&in, &once); err != nil {
		t.Fatal(err)
	}
	if s.o.gap != gapLost {
		t.Fatalf("the first run's layering has gap %d, want gapLost", s.o.gap)
	}
	// The next layout of the state starts without it.
	ok := tinyConstraintCycle()
	if err := s.buildTyped(&ok); err != nil || s.o.gap != gapNone {
		t.Fatalf("after buildTyped: gap %d, error %v", s.o.gap, err)
	}

	uniqueIDCounter.Store(goldenCounterStart)
	var out LayoutOutput
	if err := LayoutTyped(&in, &out); err != nil {
		t.Fatal(err)
	}
	draws := uniqueIDCounter.Load() - goldenCounterStart
	if hash := goldenHash(goldenDump(&out)); hash != "1dc47f5fc936c991" || draws != 237 {
		t.Fatalf("layout %s with %d draws, want dagro v0.4.0's 1dc47f5fc936c991 with 237", hash, draws)
	}
}

// TestOrderGap checks orderGap's verdict on the layerings a run of the
// sweeps can leave: nodes 0 and 1 on rank 0, nodes 2 and 3 on rank 1, with
// the orders the sweeps gave them, and o.best, the layer matrix of the orders
// of one sweep, in which a node the sweep left out has its stale order.
func TestOrderGap(t *testing.T) {
	cases := []struct {
		name       string
		orders     [4]float64 // after the last sweep
		best       [][]int32  // nil: no layering kept
		want       int
		wantOrders [4]float64
	}{
		{"consistent", [4]float64{1, 0, 0, 1}, [][]int32{{0, 1}, {2, 3}}, gapNone, [4]float64{0, 1, 0, 1}},
		// Node 1's stale order equalled node 0's: position 1 stayed empty.
		{"empty position", [4]float64{0, 0, 0, 1}, [][]int32{{0, inone}, {2, 3}}, gapEmpty, [4]float64{0, 0, 0, 1}},
		// Node 1 is missing from o.best, and its stale order is past the
		// end of rank 0: position 1 stays empty.
		{"missing node", [4]float64{0, 2, 0, 1}, [][]int32{{0}, {2, 3}}, gapMissing, [4]float64{0, 2, 0, 1}},
		// Node 1's stale order is node 0's position.
		{"lost node", [4]float64{0, 0, 0, 1}, [][]int32{{0}, {2, 3}}, gapLost, [4]float64{0, 0, 0, 1}},
		// Node 1's stale order is the free position at the end of rank 0.
		{"missing node, consistent", [4]float64{0, 1, 0, 1}, [][]int32{{0}, {2, 3}}, gapNone, [4]float64{0, 1, 0, 1}},
		{"no best, consistent", [4]float64{1, 0, 0, 1}, nil, gapNone, [4]float64{1, 0, 0, 1}},
		{"no best, empty position", [4]float64{0, 2, 0, 1}, nil, gapMissing, [4]float64{0, 2, 0, 1}},
		{"no best, shared position", [4]float64{0, 0, 1, 0}, nil, gapLost, [4]float64{0, 0, 1, 0}},
	}
	for _, c := range cases {
		s := new(lstate)
		s.o.s = s
		for v, order := range c.orders {
			l := newNLabel()
			l.rank, l.hasRank = float64(v/2), true
			l.order, l.hasOrder = order, true
			s.nl = append(s.nl, l)
		}
		s.o.best = c.best
		if got := s.o.orderGap([]int32{0, 1, 2, 3}, 1, c.best != nil); got != c.want {
			t.Errorf("%s: orderGap %d, want %d", c.name, got, c.want)
		}
		for v, want := range c.wantOrders {
			if s.nl[v].order != want {
				t.Errorf("%s: node %d has order %v, want %v", c.name, v, s.nl[v].order, want)
			}
		}
	}
}

// TestTakeoverInconsistentFailsAsBefore checks a generated D2 diagram of 152
// objects and 40 connections, with ordinary numbers, on which the layering
// of the takeover has an empty position too (as the repeated layout, which
// orders with the takeover from the first sweep on, finds): order then runs
// the sweeps once more as the original does, and the layout fails as with
// dagro v0.4.0, with the same error and counter draws.
func TestTakeoverInconsistentFailsAsBefore(t *testing.T) {
	in := loadKnownSite(t, "takeover_inconsistent.json")
	s := new(lstate)
	s.o.repeat = true
	if err := s.buildTyped(&in); err != nil {
		t.Fatal(err)
	}
	var out LayoutOutput
	if err := s.runGuarded(&in, &out); err != errRepeatInconsistent || s.o.gap != gapEmpty {
		t.Fatalf("with the takeover from the first sweep: gap %d, error %v", s.o.gap, err)
	}
	s.o.repeat = false
	if err := s.buildTyped(&in); err != nil {
		t.Fatal(err)
	}
	if err := s.runGuarded(&in, &out); err == nil || err.Error() != errOrderGap || s.o.gap != gapEmpty {
		t.Fatalf("one layout: gap %d, error %v", s.o.gap, err)
	}
	uniqueIDCounter.Store(goldenCounterStart)
	err := LayoutTyped(&in, &out)
	if draws := uniqueIDCounter.Load() - goldenCounterStart; err == nil || err.Error() != errOrderGap || draws != 346 {
		t.Fatalf("LayoutTyped: error %v with %d draws, want dagro v0.4.0's %q with 346", err, draws, errOrderGap)
	}
}

// TestRepeatedLayoutKeepsFirstError checks generated inputs on which order's
// first run puts two nodes on one position and the layout fails (with a panic
// or with dagre's error), and so does its repeat, which gives up on its
// inconsistent layering: LayoutTyped returns the first layout's error, and
// the process-wide counter moves only by the first layout's draws. The inputs
// have fractional minlens and weights, but no NaN or infinite numbers, whose
// conversion to int differs between platforms: they fail the same way on all.
func TestRepeatedLayoutKeepsFirstError(t *testing.T) {
	z := newInputGen(21, "accepted,fracminlen,fracweight")
	panics, errs := 0, 0
	for i := 0; i < 1500; i++ {
		in := z.input(16)
		s := new(lstate)
		if s.buildTyped(&in) != nil {
			continue
		}
		var out LayoutOutput
		uniqueIDCounter.Store(goldenCounterStart)
		err := s.runGuarded(&in, &out)
		draws := uniqueIDCounter.Load() - goldenCounterStart
		if err == nil || s.o.gap != gapLost {
			continue
		}
		uniqueIDCounter.Store(goldenCounterStart)
		err2 := LayoutTyped(&in, &out)
		if err2 == nil {
			continue // the repeat laid it out
		}
		if _, ok := err.(*PanicError); ok {
			panics++
		} else {
			errs++
		}
		if err2.Error() != err.Error() {
			t.Errorf("input %d: LayoutTyped gives %q, one layout %q", i, err2, err)
		}
		if got := uniqueIDCounter.Load() - goldenCounterStart; got != draws {
			t.Errorf("input %d: LayoutTyped took %d values from the counter, one layout %d", i, got, draws)
		}
	}
	if panics == 0 || errs == 0 {
		t.Fatalf("generated inputs that fail after their layering lost a node: %d with a panic, %d with an error", panics, errs)
	}
}

// TestRepairAllocations checks what the repairs cost on a warm state. The
// takeover keeps its buffers in the state, so that a repaired layout
// allocates little, as any other does: 2 to 43 objects per call for these
// inputs, where a fresh state allocates over a thousand and buffers made
// anew in every call cost 80 to 620. And a layout that fails while its
// layering is consistent is not repeated: LayoutTyped allocates what one
// layout allocates (a repeat would double it).
func TestRepairAllocations(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector's sync.Pool drops Puts at random")
	}
	for _, c := range constraintCycleInputs(t) {
		in := c.in
		var out LayoutOutput
		if err := LayoutTyped(&in, &out); err != nil {
			t.Fatal(err)
		}
		allocs := testing.AllocsPerRun(10, func() {
			if err := LayoutTyped(&in, &out); err != nil {
				t.Fatal(err)
			}
		})
		if allocs > 64 {
			t.Errorf("%s: %.0f allocations per layout on a warm state", c.name, allocs)
		}
	}

	z := newInputGen(21, "accepted,fracminlen,fracweight")
	for i := 0; i < 1000; i++ {
		in := z.input(16)
		s := new(lstate)
		var out LayoutOutput
		if s.buildTyped(&in) != nil {
			continue
		}
		err := s.runGuarded(&in, &out)
		if _, panicked := err.(*PanicError); err == nil || panicked || s.o.gap != gapNone {
			continue
		}
		// What LayoutTyped does when it does not repeat the layout.
		one := testing.AllocsPerRun(10, func() {
			s := getLState()
			if s.buildTyped(&in) == nil && s.runGuarded(&in, &out) == nil {
				t.Fatalf("input %d laid out", i)
			}
			putLState(s)
		})
		all := testing.AllocsPerRun(10, func() {
			if LayoutTyped(&in, &out) == nil {
				t.Fatalf("input %d laid out", i)
			}
		})
		if all > one*3/2 {
			t.Fatalf("input %d (%v): LayoutTyped allocates %.0f objects per call, one layout %.0f", i, err, all, one)
		}
		return
	}
	t.Fatal("no generated input fails while its layering is consistent")
}

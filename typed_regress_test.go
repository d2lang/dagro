package dagro

import (
	"math"
	"testing"
)

// Regression tests. Each compares Layout with the Attrs engine (the
// original algorithm) on one minimized input; an earlier version of the
// typed engine failed them.

func checkSpec(t *testing.T, s graphSpec, start uint64) {
	t.Helper()
	want := runSpecReference(s, start)
	got := runSpecLayout(s, start)
	if d := diffOutcomes(want, got); d != "" {
		t.Errorf("Layout differs from the original:\n%s\nspec:\n%s", d, s)
	}
}

// Brandes-Koepf: with a fractional minlen a node's neighbour can sit in its
// own layer; the original then leaves some nodes out of an alignment's xs map
// (it assigns xs only to the VALUES of align), and findSmallestWidthAlignment,
// alignCoordinates and balance only see the map's entries. The typed engine
// gives every layering node xs[root[v]] in every alignment.
func TestFracMinlenMatchesOriginal(t *testing.T) {
	s := graphSpec{compound: true, graph: attrsLabel(), ops: []specOp{
		{kind: specOpSetParent, v: "_7", w: "c2"},
		{kind: specOpSetEdge, v: "4", w: "_10", label: attrsLabel("minlen", 0.25)},
		{kind: specOpSetEdge, v: "6", w: "_10", label: attrsLabel()},
		{kind: specOpSetEdge, v: "1", w: "4", label: attrsLabel()},
	}}
	checkSpec(t, s, 40175)
}

// Same mechanism without compound nodes: seven unlabeled nodes with ASCII
// names, one edge with minlen 1.25.
func TestFracMinlenPlainMatchesOriginal(t *testing.T) {
	s := graphSpec{compound: true, multigraph: true, graph: attrsLabel(), ops: []specOp{
		{kind: specOpSetEdge, v: "a", w: "A", label: attrsLabel()},
		{kind: specOpSetEdge, v: "0", w: "010", label: attrsLabel()},
		{kind: specOpSetEdge, v: "b", w: "a", label: attrsLabel()},
		{kind: specOpSetEdge, v: "c", w: "d", label: attrsLabel()},
		{kind: specOpSetEdge, v: "010", w: "c", label: attrsLabel("minlen", 1.25)},
		{kind: specOpSetEdge, v: "0", w: "a", label: attrsLabel()},
		{kind: specOpSetEdge, v: "d", w: "A", label: attrsLabel()},
	}}
	checkSpec(t, s, 61686)
}

// swapXY: the original swaps the raw values, so a node without x (or y)
// before undoCoordinateSystem gets x = nil (read as 0) for rankdir LR/RL; the
// typed engine keeps it absent (NaN), and every coordinate becomes NaN.
func TestNaNMinlenLRMatchesOriginal(t *testing.T) {
	s := graphSpec{compound: true, multigraph: true, graph: attrsLabel("rankdir", "LR"), ops: []specOp{
		{kind: specOpSetParent, v: "0", w: "_bt3"},
		{kind: specOpSetParent, v: "4", w: "_bt3"},
		{kind: specOpSetParent, v: "7", w: "_bt3"},
		{kind: specOpSetEdge, v: "6", w: "7", label: attrsLabel(), name: "(6 -> 7)[1]", hasName: true},
		{kind: specOpSetEdge, v: "6", w: "3", label: attrsLabel("minlen", 7.0), name: "(6 -> 3)[3]", hasName: true},
		{kind: specOpSetEdge, v: "7", w: "4", label: attrsLabel("minlen", math.NaN()), name: "(7 -> 4)[4]", hasName: true},
		{kind: specOpSetEdge, v: "3", w: "4", label: attrsLabel(), name: "(3 -> 4)[7]", hasName: true},
		{kind: specOpSetEdge, v: "0", w: "_5", label: attrsLabel(), name: "(0 -> _5)[9]", hasName: true},
		{kind: specOpSetEdge, v: "_5", w: "6", label: attrsLabel(), name: "(_5 -> 6)[10]", hasName: true},
	}}
	checkSpec(t, s, 177229)
}

// Both engines panic, with different panic values (runtime error vs string).
func TestNaNMinlenPanicMatchesOriginal(t *testing.T) {
	s := graphSpec{compound: true, multigraph: true, graph: attrsLabel(), ops: []specOp{
		{kind: specOpSetParent, v: "7", w: "101"},
		{kind: specOpSetParent, v: "2", w: "101"},
		{kind: specOpSetParent, v: "24", w: "101"},
		{kind: specOpSetEdge, v: "2", w: "21", label: attrsLabel(), name: "(2 -> 21)[2]", hasName: true},
		{kind: specOpSetEdge, v: "2", w: "24", label: attrsLabel("minlen", math.NaN())},
		{kind: specOpSetEdge, v: "21", w: "8", label: attrsLabel(), name: "(21 -> 8)[11]", hasName: true},
		{kind: specOpSetEdge, v: "16", w: "2", label: attrsLabel(), name: "(16 -> 2)[12]", hasName: true},
		{kind: specOpSetEdge, v: "16", w: "10", label: attrsLabel()},
		{kind: specOpSetEdge, v: "10", w: "7", label: attrsLabel()},
	}}
	checkSpec(t, s, 29561)
}

// layoutSingleNode returns -0 for the graph size and the centre when the
// node's width or height is -0; Layout(in.Graph()) (original and this tree)
// returns +0.
func TestSingleNodeNegativeZero(t *testing.T) {
	negZero := math.Copysign(0, -1)
	for _, in := range []LayoutInput{
		{RankDir: "TB", RankSep: 50, EdgeSep: 20, NodeSep: 50, Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: negZero, Height: 10}}},
		{RankDir: "TB", RankSep: 50, EdgeSep: 20, NodeSep: 50, Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: 10, Height: negZero}}},
	} {
		want, got := runTypedPair(in, 1)
		if d := diffTypedOutcomes(in, 1, want, got); d != "" {
			t.Errorf("LayoutTyped differs from the original Layout(in.Graph()):\n%s\ninput %+v", d, in)
		}
		var viaGraph LayoutOutput
		if err := layoutViaGraph(&in, &viaGraph); err != nil {
			t.Fatal(err)
		}
		var fast LayoutOutput
		_ = LayoutTyped(&in, &fast)
		if a, b := dumpTypedOutput(&viaGraph), dumpTypedOutput(&fast); a != b {
			t.Errorf("LayoutTyped differs from this tree's Layout(in.Graph()):\nvia graph %s\nLayoutTyped %s", a, b)
		}
	}
}

// swapXY again, reached through NaN and negative edge weights (rankdir LR):
// node "0" ends up without coordinates in both engines; the original then
// reads it as (0, 0), the typed engine as NaN, and the whole layout is NaN.
func TestNaNWeightLRMatchesOriginal(t *testing.T) {
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
	e("n930", "n336", attrsLabel(), "(n930 -> n336)[13]")
	e("69", "583", attrsLabel(), "(69 -> 583)[18]")
	e("n222", "969", attrsLabel("weight", -1.0), "rev2")
	checkSpec(t, s, 70703)
}

// acyclicer "greedy" with a zero-weight edge leaves the cycle 2 -> 0 -> 2 in
// place (greedyFAS files a node whose out-weight sum is 0 as a sink), so the
// ranker sees edges in both directions between 0 and 2. The original's
// calcCutValue/enterEdge/updateRanks pick the edge with g.HasEdge(child,
// parent); the typed network simplex uses the tree edge's own graph edge.
// Both engines panic later, but after different ranks: the process-wide
// dummy-name counter advances by 9 in the original and by 5 here, which
// changes the dummy names (and possibly the layouts) of every later call.
// Other inputs of this kind make the original pivot forever while the typed
// engine panics (seed 152 case 39 and seed 154 case 2 of the generator's
// features "greedy0,noclusters").
func TestGreedyTwoCycleMatchesOriginal(t *testing.T) {
	s := graphSpec{compound: true, multigraph: true, graph: attrsLabel("acyclicer", "greedy"), ops: []specOp{
		{kind: specOpSetEdge, v: "2", w: "0", label: attrsLabel("weight", 0.0), name: "(2 -> 0)[1]", hasName: true},
		{kind: specOpSetEdge, v: "0", w: "2", label: attrsLabel()},
		{kind: specOpSetEdge, v: "5", w: "2", label: attrsLabel(), name: "\x00", hasName: true},
	}}
	checkSpec(t, s, 128240)
}

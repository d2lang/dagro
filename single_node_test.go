package dagro

import (
	"math"
	"math/rand"
	"testing"
)

// TestSingleNodeMatchesEngine compares layoutSingleNode with the
// typed engine, bit for bit, on one-node graphs with
// every rankdir spelling, ranker, acyclicer and alignment, random separations
// (NaN and infinities included) and random sizes, and checks that the fast
// path declines the inputs it does not cover; the process-wide dummy name
// counter must not move.
func TestSingleNodeMatchesEngine(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	sizes := []float64{0, 1, 2, 3, 53, 66, 100, 101, 1e6, 1<<52 - 1, 1 << 52}
	seps := []float64{0, 1, 20, 50, 60, 100, -3, 0.5, math.NaN(), math.Inf(1), math.Inf(-1)}
	dirs := []string{"TB", "BT", "LR", "RL", "tb", "bt", "lr", "rl", "", "xx"}
	rankers := []string{"", "network-simplex", "tight-tree", "longest-path"}
	aligns := []string{"", "UL", "UR", "DL", "DR", "ul"}
	ids := []string{"0", "a", "17", "x.y", "root"}
	bits := math.Float64bits
	for c := 0; c < 20000; c++ {
		pick := func(s []float64) float64 {
			if rng.Intn(3) == 0 {
				return float64(rng.Intn(500))
			}
			return s[rng.Intn(len(s))]
		}
		in := LayoutInput{
			RankDir: dirs[rng.Intn(len(dirs))], RankSep: pick(seps), EdgeSep: pick(seps), NodeSep: pick(seps),
			Ranker: rankers[rng.Intn(len(rankers))], Align: aligns[rng.Intn(len(aligns))],
			Nodes: []LayoutNode{{ID: ids[rng.Intn(len(ids))], Parent: -1, Width: pick(sizes), Height: pick(sizes)}},
		}
		if rng.Intn(2) == 0 {
			in.Acyclicer = "greedy"
		}
		var fast LayoutOutput
		if !layoutSingleNode(&in, &fast) {
			t.Fatalf("case %d: fast path declined %+v", c, in)
		}
		counter := uniqueIDCounter.Load()
		typedFastPaths = false
		var engine LayoutOutput
		err := LayoutTyped(&in, &engine)
		typedFastPaths = true
		if err != nil {
			t.Fatalf("case %d: %v", c, err)
		}
		if uniqueIDCounter.Load() != counter {
			t.Fatalf("case %d: the engine drew %d generated names", c, uniqueIDCounter.Load()-counter)
		}
		for _, ref := range []*LayoutOutput{&engine} {
			same := bits(ref.Width) == bits(fast.Width) && bits(ref.Height) == bits(fast.Height) &&
				len(ref.Nodes) == 1 && len(ref.Edges) == 0 && len(fast.Edges) == 0 &&
				bits(ref.Nodes[0].X) == bits(fast.Nodes[0].X) && bits(ref.Nodes[0].Y) == bits(fast.Nodes[0].Y) &&
				bits(ref.Nodes[0].Width) == bits(fast.Nodes[0].Width) && bits(ref.Nodes[0].Height) == bits(fast.Nodes[0].Height)
			if !same {
				t.Fatalf("case %d: input %+v\nfast %+v\nref  %+v", c, in, fast, *ref)
			}
		}
	}
	// Inputs the fast path must leave to the engine.
	declined := []LayoutInput{
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: 1.5, Height: 2}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: -2, Height: 2}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: math.NaN(), Height: 2}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: 1 << 53, Height: 2}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "_root", Parent: -1, Width: 2, Height: 2}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "", Parent: -1, Width: 2, Height: 2}}},
		{RankDir: "TB", MarginX: 3, Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: 2, Height: 2}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "a", Parent: 0, Width: 2, Height: 2}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: 2, Height: 2}, {ID: "b", Parent: -1}}},
		{RankDir: "TB", Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: 2, Height: 2}}, Edges: []LayoutEdge{NewLayoutEdge(0, 0)}},
	}
	for i := range declined {
		var out LayoutOutput
		if layoutSingleNode(&declined[i], &out) {
			t.Fatalf("declined input %d taken: %+v", i, declined[i])
		}
	}
}

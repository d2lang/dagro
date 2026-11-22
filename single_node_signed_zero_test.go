package dagro

import (
	"math"
	"testing"
)

// The single-node closed form against the original's Layout(in.Graph()) and
// this tree's engine, for signed zero sizes in every rank direction.
func TestSingleNodeSignedZeroMatchesOriginal(t *testing.T) {
	negZero := math.Copysign(0, -1)
	sizes := []float64{negZero, 0, 7, 10}
	for _, dir := range []string{"TB", "BT", "LR", "RL", "", "x"} {
		for _, w := range sizes {
			for _, h := range sizes {
				in := LayoutInput{RankDir: dir, RankSep: 50, EdgeSep: 20, NodeSep: 50,
					Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: w, Height: h}}}
				want, got := runTypedPair(in, 3)
				if d := diffOutcomes(want, got); d != "" {
					t.Errorf("rankdir %q w %v h %v (signbits %v %v): LayoutTyped differs from the original:\n%s",
						dir, w, h, math.Signbit(w), math.Signbit(h), d)
				}
				var engine LayoutOutput
				typedFastPaths = false
				err := LayoutTyped(&in, &engine)
				typedFastPaths = true
				if err != nil {
					t.Fatal(err)
				}
				var fast LayoutOutput
				if err := LayoutTyped(&in, &fast); err != nil {
					t.Fatal(err)
				}
				if a, b := dumpTypedOutput(&engine), dumpTypedOutput(&fast); a != b {
					t.Errorf("rankdir %q w %v h %v: closed form differs from the engine:\nengine %s\nfast   %s", dir, w, h, a, b)
				}
			}
		}
	}
}

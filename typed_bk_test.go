package dagro

import (
	"fmt"
	"testing"
)

// Brandes-Koepf membership and the other position phases on
// generated graphs with fractional and zero minlen, against the original.
func TestFracAndZeroMinlenMatchOriginal(t *testing.T) {
	for _, c := range []struct {
		features string
		seed     int64
		cases    int
	}{{"fracminlen", 801, 150}, {"minlen0", 802, 150}, {"fracminlen,noclusters", 803, 100}} {
		z := newSpecGenerator(c.seed, c.features)
		differ := 0
		for i := 0; i < c.cases; i++ {
			s := z.spec(14)
			start := uint64(1 + z.rng.Intn(200000))
			want := runSpecReference(s, start)
			if want.hung {
				t.Fatalf("%s case %d: the original hangs", c.features, i)
			}
			got := runSpecLayout(s, start)
			if d := diffOutcomes(want, got); d != "" {
				differ++
				if differ <= 2 {
					t.Errorf("%s case %d (counter start %d) differs from the original:\n%s\nspec:\n%s", c.features, i, start, d, s)
				}
			}
		}
		if differ > 0 {
			t.Errorf("%s: %d of %d cases differ", c.features, differ, c.cases)
		}
	}
}

// BK alignments with a neighbor in the same layer, built directly: a node
// that drops out of align's values gets no x in "ul" or reads 0 elsewhere.
func TestSameLayerNeighborsMatchOriginal(t *testing.T) {
	for _, minlen := range []float64{0, 0.25, 0.5, 1.25, 2.75} {
		for _, align := range []string{"", "UL", "ur", "dl", "DR", "xx"} {
			for _, rankdir := range []string{"TB", "RL"} {
				s := graphSpec{compound: true, multigraph: true, graph: attrsLabel("align", align, "rankdir", rankdir)}
				for i := 0; i < 6; i++ {
					s.ops = append(s.ops, specOp{kind: specOpSetEdge, v: fmt.Sprint("n", i), w: fmt.Sprint("n", i+1), label: attrsLabel("minlen", minlen)})
					s.ops = append(s.ops, specOp{kind: specOpSetEdge, v: fmt.Sprint("n", i), w: fmt.Sprint("m", i), label: attrsLabel()})
					s.ops = append(s.ops, specOp{kind: specOpSetEdge, v: fmt.Sprint("m", i), w: fmt.Sprint("n", i+1), label: attrsLabel("minlen", 1.0-minlen/4)})
				}
				want := runSpecReference(s, 31)
				if want.hung {
					t.Fatalf("minlen %v align %q: the original hangs", minlen, align)
				}
				got := runSpecLayout(s, 31)
				if d := diffOutcomes(want, got); d != "" {
					t.Errorf("minlen %v align %q rankdir %s differs from the original:\n%s", minlen, align, rankdir, d)
				}
			}
		}
	}
}

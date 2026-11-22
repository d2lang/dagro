package dagro

import (
	"math"
	"testing"
)

// swapXY with nodes that position left without coordinates (NaN minlen, NaN
// and negative weights), in every rank direction: the original's swapXYOne
// turns a missing coordinate into a present nil (read as 0).
func TestUnplacedNodesAllRankdirsMatchOriginal(t *testing.T) {
	nanMinlen := func(rankdir string) graphSpec {
		return graphSpec{compound: true, multigraph: true, graph: attrsLabel("rankdir", rankdir), ops: []specOp{
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
	}
	nanWeight := func(rankdir string) graphSpec {
		s := graphSpec{compound: true, multigraph: true, graph: attrsLabel("rankdir", rankdir)}
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
		return s
	}
	for _, rankdir := range []string{"LR", "RL", "lr", "Rl", "BT", "TB", ""} {
		for name, s := range map[string]graphSpec{"nanMinlen": nanMinlen(rankdir), "nanWeight": nanWeight(rankdir)} {
			want := runSpecReference(s, 177229)
			got := runSpecLayout(s, 177229)
			if d := diffOutcomes(want, got); d != "" {
				t.Errorf("%s rankdir %q differs from the original:\n%s", name, rankdir, d)
			}
		}
	}
}

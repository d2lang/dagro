package dagro

import (
	"fmt"
	"math"
	"testing"
)

// Network simplex with edges in both directions between two nodes
// in the ranking graph: acyclicer "greedy" leaves a cycle when a weight is
// 0, -0 or NaN, and the original orients each tree edge with g.HasEdge.
// (Inputs on which the original pivots forever cannot be compared in-process:
// the hung run keeps drawing the counter. Two such cases were
// checked in fresh processes: both engines hang in the exchange loop.)
func TestGreedyTwoCyclesMatchOriginal(t *testing.T) {
	weights := []any{0.0, math.Copysign(0, -1), math.NaN(), 1.0, 2.5}
	type e struct {
		v, w string
		l    specLabel
	}
	variants := func(w0 any) [][]e {
		return [][]e{
			{{"a", "b", attrsLabel("weight", w0)}, {"b", "a", attrsLabel()}},
			{{"a", "b", attrsLabel("weight", w0)}, {"b", "a", attrsLabel()}, {"b", "c", attrsLabel()}},
			{{"a", "b", attrsLabel("weight", w0)}, {"b", "a", attrsLabel("minlen", 2.0)}, {"c", "a", attrsLabel()}, {"b", "d", attrsLabel()}},
			{{"b", "a", attrsLabel("weight", w0)}, {"a", "b", attrsLabel()}, {"c", "b", attrsLabel()}, {"a", "d", attrsLabel()}},
			{{"a", "b", attrsLabel("weight", w0)}, {"b", "c", attrsLabel()}, {"c", "a", attrsLabel()}, {"c", "b", attrsLabel()}},
			{{"d", "a", attrsLabel()}, {"a", "b", attrsLabel("weight", w0, "minlen", 2.0)}, {"b", "a", attrsLabel()}, {"b", "c", attrsLabel()}, {"c", "d", attrsLabel("weight", 2.0)}},
			{{"2", "0", attrsLabel("weight", w0)}, {"0", "2", attrsLabel()}, {"5", "2", attrsLabel()}},
			{{"x", "y", attrsLabel("weight", w0)}, {"y", "x", attrsLabel("weight", w0)}, {"y", "z", attrsLabel()}, {"z", "x", attrsLabel()}},
		}
	}
	n := 0
	for _, w0 := range weights {
		for vi, edges := range variants(w0) {
			for _, nodes := range []bool{false, true} {
				for _, rankdir := range []string{"TB", "LR"} {
					s := graphSpec{compound: true, multigraph: true, graph: attrsLabel("acyclicer", "greedy", "rankdir", rankdir)}
					if nodes {
						for _, v := range []string{"d", "c", "b", "a", "z", "y", "x", "0", "1", "2", "5"} {
							s.ops = append(s.ops, specOp{kind: specOpSetNode, v: v, label: attrsLabel("width", 20.0, "height", 10.0)})
						}
					}
					for _, ed := range edges {
						s.ops = append(s.ops, specOp{kind: specOpSetEdge, v: ed.v, w: ed.w, label: ed.l})
					}
					want := runSpecReference(s, 1000)
					if want.hung {
						t.Fatalf("w0 %v variant %d: the original hangs", w0, vi)
					}
					got := runSpecLayout(s, 1000)
					if d := diffOutcomes(want, got); d != "" {
						t.Errorf("w0 %v variant %d nodes %v rankdir %s differs from the original:\n%s", w0, vi, nodes, rankdir, d)
					}
					n++
				}
			}
		}
	}
	t.Logf("%d graphs", n)
}

// The opposite-edge table is computed only for acyclicer "greedy": with any
// other acyclicer the simplified ranking graph never has edges in both
// directions between two nodes. Checked on the generator's graphs by
// inspecting the state after the typed ranking.
func TestNoOppositeRankEdgesWithoutGreedy(t *testing.T) {
	for _, features := range []string{"", "weirdweights", "minlen0", "fracminlen", "removals,weirdlabels"} {
		z := newSpecGenerator(99, features)
		checked := 0
		for i := 0; i < 400; i++ {
			sp := z.spec(14)
			g, _ := buildFromSpec(sp)
			s := new(lstate)
			if !buildNoPanic(s, g) {
				continue
			}
			if s.acyclicer == "greedy" || s.ranker == "longest-path" || s.ranker == "tight-tree" {
				continue
			}
			func() {
				// The original's runtime errors after ranking go through
				// runGuarded; only the ranking graph matters.
				defer func() { _ = recover() }()
				s.runGuarded(s.run)
			}()
			sg := &s.r.simple
			if len(sg.ev) == 0 {
				continue // ranking was not reached
			}
			checked++
			pairs := map[[2]int32]bool{}
			for e := range sg.ev {
				pairs[[2]int32{sg.ev[e], sg.ew[e]}] = true
			}
			for e := range sg.ev {
				if pairs[[2]int32{sg.ew[e], sg.ev[e]}] {
					t.Fatalf("features %q case %d: opposite ranking edges without greedy:\n%s", features, i, sp)
				}
			}
		}
		t.Logf("features %q: %d ranking graphs checked", features, checked)
	}
}

func buildNoPanic(s *lstate, g *Graph) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return s.build(g)
}

// rankStates runs the phases through rank in the Attrs engine and
// in the typed engine (from the same counter value) and returns both states.
func rankStates(build func() *Graph, start uint64) (attrs, typed string, ok bool) {
	uniqueIDCounter.Store(start)
	lg := buildLayoutGraph(build())
	makeSpaceForEdgeLabels(lg)
	removeSelfEdges(lg)
	runAcyclic(lg)
	runNestingGraph(lg)
	rank(asNonCompoundGraph(lg))
	attrs = attrsPhaseState(lg)
	uniqueIDCounter.Store(start)
	s := new(lstate)
	if !s.build(build()) {
		return "", "", false
	}
	s.makeSpaceForEdgeLabels()
	s.removeSelfEdges()
	s.runAcyclic()
	s.runNestingGraph()
	s.rank()
	return attrs, typedPhaseState(s), true
}

// The ranks themselves (not only the final outcome, which the decline path
// also repairs where the typed engine panics later) equal the Attrs engine's
// on greedy two-cycles.
func TestGreedyTwoCycleRanksMatchAttrsEngine(t *testing.T) {
	skipWithoutTypedGreedy(t)
	builds := map[string]func() *Graph{
		"negzero-weight": negZeroWeightGreedyGraph,
		"three-edges": func() *Graph {
			g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"acyclicer": "greedy"})
			g.SetEdge("2", "0", Attrs{"weight": 0.0}, "(2 -> 0)[1]")
			g.SetEdge("0", "2", Attrs{})
			g.SetEdge("5", "2", Attrs{}, "\x00")
			return g
		},
	}
	for _, w0 := range []float64{0, math.Copysign(0, -1), math.NaN()} {
		w0 := w0
		if math.IsNaN(w0) && !nanConvertsToZero() {
			continue // the Attrs engine's greedyFAS panics in makeslice here
		}
		builds[fmt.Sprint("pair-", w0)] = func() *Graph {
			g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"acyclicer": "greedy"})
			g.SetEdge("a", "b", Attrs{"weight": w0})
			g.SetEdge("b", "a", Attrs{"minlen": 2.0})
			g.SetEdge("c", "a", Attrs{})
			g.SetEdge("b", "d", Attrs{})
			return g
		}
		builds[fmt.Sprint("chain-", w0)] = func() *Graph {
			g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"acyclicer": "greedy"})
			g.SetEdge("d", "a", Attrs{})
			g.SetEdge("a", "b", Attrs{"weight": w0, "minlen": 2.0})
			g.SetEdge("b", "a", Attrs{})
			g.SetEdge("b", "c", Attrs{})
			g.SetEdge("c", "d", Attrs{"weight": 2.0})
			return g
		}
	}
	for name, build := range builds {
		attrs, typed, ok := rankStates(build, 42312)
		if !ok {
			t.Fatalf("%s: declined", name)
		}
		if attrs != typed {
			t.Errorf("%s: ranks differ:\nATTRS:\n%s\nTYPED:\n%s", name, attrs, typed)
		}
	}
}

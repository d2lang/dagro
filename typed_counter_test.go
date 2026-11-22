package dagro

import (
	"sort"
	"strconv"
	"sync"
	"testing"
)

// The typed engine draws from the process-wide
// counter as the original does (one atomic increment per value, visible at
// once) and records the values; a decline replays them to the Attrs engine.

// counterGraph has compound nodes (nesting and border dummies), long
// edges (edge dummies) and a cycle (reversed edge names): many draws.
func counterGraph() *Graph {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"rankdir": "LR"})
	for i := 0; i < 24; i++ {
		v := "n" + strconv.Itoa(i)
		g.SetNode(v, Attrs{"width": float64(10 + i%5), "height": 12.0})
		if i%6 != 0 {
			_ = g.SetParent(v, "c"+strconv.Itoa(i/6))
		}
	}
	for i := 0; i+1 < 24; i++ {
		g.SetEdge("n"+strconv.Itoa(i), "n"+strconv.Itoa(i+1), Attrs{"minlen": float64(1 + i%3), "width": 8.0, "height": 6.0})
	}
	g.SetEdge("n23", "n2", Attrs{})
	g.SetEdge("n17", "n4", Attrs{"minlen": 2.0}, "back")
	return g
}

// The phases up to addBorderSegments, called directly (outside runGuarded),
// have advanced the process-wide counter by as many values as the Attrs
// engine's same phases, and those values are the ones recorded in drawn.
// (An earlier version drew on a private view that only runGuarded
// published, so the counter had not moved.)
func TestTypedDrawsVisibleImmediately(t *testing.T) {
	const start = 7000
	uniqueIDCounter.Store(start)
	lg := buildLayoutGraph(counterGraph())
	makeSpaceForEdgeLabels(lg)
	removeSelfEdges(lg)
	runAcyclic(lg)
	runNestingGraph(lg)
	rank(asNonCompoundGraph(lg))
	injectEdgeLabelProxies(lg)
	removeEmptyRanks(lg)
	cleanupNestingGraph(lg)
	normalizeRanks(lg)
	assignRankMinMax(lg)
	removeEdgeLabelProxies(lg)
	runNormalize(lg)
	parentDummyChains(lg)
	addBorderSegments(lg)
	want := uniqueIDCounter.Load() - start

	uniqueIDCounter.Store(start)
	s := new(lstate)
	if !s.build(counterGraph()) {
		t.Fatal("declined")
	}
	s.makeSpaceForEdgeLabels()
	s.removeSelfEdges()
	s.runAcyclic()
	s.runNestingGraph()
	s.rank()
	s.injectEdgeLabelProxies()
	s.removeEmptyRanks()
	s.cleanupNestingGraph()
	s.normalizeRanks()
	s.assignRankMinMax()
	s.removeEdgeLabelProxies()
	s.runNormalize()
	s.parentDummyChains()
	s.addBorderSegments()
	got := uniqueIDCounter.Load() - start
	if want < 20 || got != want {
		t.Fatalf("counter advanced by %d during the typed phases, by %d during the Attrs phases", got, want)
	}
	if uint64(len(s.drawn)) != got {
		t.Fatalf("recorded %d draws, counter advanced by %d", len(s.drawn), got)
	}
	for i, v := range s.drawn {
		if v != start+uint64(i)+1 {
			t.Fatalf("draw %d is %d, want %d", i, v, start+uint64(i)+1)
		}
	}
}

// Concurrent typed layouts draw distinct values whose union is exactly the
// range the counter advanced over, as the original's atomic draws. (An
// earlier version drew from the base each call loaded, so two calls
// running at the same time drew the same values.)
func TestConcurrentDrawsDistinct(t *testing.T) {
	const workers, rounds = 6, 20
	start := uniqueIDCounter.Load()
	var mu sync.Mutex
	var all []uint64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			in := bigInput(40+10*w, 3)
			for r := 0; r < rounds; r++ {
				s := new(lstate)
				if !s.buildTyped(&in) {
					t.Error("declined")
					return
				}
				if err, ok := s.runGuarded(s.run); !ok || err != nil {
					t.Errorf("run: ok=%v err=%v", ok, err)
					return
				}
				mu.Lock()
				all = append(all, s.drawn...)
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	end := uniqueIDCounter.Load()
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	if uint64(len(all)) != end-start {
		t.Fatalf("%d values drawn, counter advanced by %d", len(all), end-start)
	}
	for i, v := range all {
		if v != start+uint64(i)+1 {
			t.Fatalf("drawn values are not the counter's range: position %d holds %d, want %d", i, v, start+uint64(i)+1)
		}
	}
}

// The Attrs engine's draws on a layout graph with a replay return the
// replayed values in order, then new values from the counter.
func TestReplaySource(t *testing.T) {
	const start = 9000
	uniqueIDCounter.Store(start)
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{})
	g.SetNode("_d", Attrs{})
	g.SetNode("_d42", Attrs{})
	g.replay = &drawReplay{vals: []uint64{42, 17, 5}}
	// addDummyNode skips "_d" and "_d42" (taken), then takes "_d17".
	if v := addDummyNode(g, "edge", Attrs{}, "_d"); v != "_d17" {
		t.Fatalf("addDummyNode = %q, want _d17", v)
	}
	if v := createOrderRootNode(g); v != "_root5" {
		t.Fatalf("createOrderRootNode = %q, want _root5", v)
	}
	if v := nextReversedEdgeName(g, "a", "b"); v != "rev"+strconv.Itoa(start+1) {
		t.Fatalf("nextReversedEdgeName = %q, want rev%d", v, start+1)
	}
	if c := uniqueIDCounter.Load(); c != start+1 {
		t.Fatalf("counter %d, want %d", c, start+1)
	}
	// Without a replay the graph draws from the counter, as before.
	h := NewGraph()
	if v := h.uniqueID("x"); v != "x"+strconv.Itoa(start+2) {
		t.Fatalf("uniqueID = %q", v)
	}
}

// Over generated graphs on which the typed engine declines after it has
// drawn (an unexpected panic or errDecline: the Attrs engine then reruns the
// original algorithm with the draws replayed), Layout and LayoutTyped give
// the original's outcome and leave the counter where the original leaves it.
// Without the replay the counter would end ahead by the typed engine's draws.
func TestDeclineReplaysDraws(t *testing.T) {
	replayed := 0
	z := newSpecGenerator(20261121, "minlen0,weirdnums,weirdlabels,removals")
	for i := 0; i < 700; i++ {
		sp := z.spec(14)
		start := uint64(1 + z.rng.Intn(200000))
		g, _ := buildFromSpec(sp)
		uniqueIDCounter.Store(start)
		if n := declinedDraws(func(s *lstate) bool { return s.build(g) }); n == 0 {
			continue
		}
		replayed++
		want := runSpecReference(sp, start)
		got := runSpecLayout(sp, start)
		if want.hung || got.hung {
			t.Fatalf("case %d hangs (original %v, new %v)", i, want.hung, got.hung)
		}
		if d := diffOutcomes(want, got); d != "" {
			t.Fatalf("case %d (counter start %d) differs:\n%s\nspec:\n%s", i, start, d, sp)
		}
	}
	zt := inputGenerator{newSpecGenerator(20261122, "minlen0,weirdnums")}
	typedReplayed := 0
	for i := 0; i < 700; i++ {
		in := zt.input(14)
		start := uint64(1 + zt.rng.Intn(200000))
		uniqueIDCounter.Store(start)
		if n := declinedDraws(func(s *lstate) bool { return s.buildTyped(&in) }); n == 0 {
			continue
		}
		typedReplayed++
		want, got := runTypedPair(in, start)
		if want.hung || got.hung {
			t.Fatalf("typed case %d hangs (original %v, new %v)", i, want.hung, got.hung)
		}
		if d := diffOutcomes(want, got); d != "" {
			t.Fatalf("typed case %d (counter start %d) differs:\n%s\ninput: %+v", i, start, d, in)
		}
	}
	t.Logf("declines after drawing: %d Graph specs, %d LayoutInputs", replayed, typedReplayed)
	if replayed < 20 || typedReplayed < 20 {
		t.Fatalf("too few declines after drawing to check the replay: %d, %d", replayed, typedReplayed)
	}
}

// declinedDraws builds with build into a fresh state and runs it
// guarded; it returns how many values the typed engine drew before it
// declined, or 0 when it did not decline (or drew nothing first).
func declinedDraws(build func(*lstate) bool) (n int) {
	s := new(lstate)
	defer func() {
		if r := recover(); r != nil {
			n = 0 // build panicked (a coercion), or a known runtime error
		}
	}()
	if !build(s) {
		return 0
	}
	if _, ok := s.runGuarded(s.run); ok {
		return 0
	}
	return len(s.drawn)
}

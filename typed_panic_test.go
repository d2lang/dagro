package dagro

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Tests of the typed engine's decline path (lstate.runGuarded): where the
// typed engine panics or declines, Layout and LayoutTyped must behave exactly
// like the Attrs engine (the original algorithm), including the panic value, the counter
// and the DebugTiming lines.

// An empty graph: the original panics in order (make with cap -1). The typed
// engine used to return nil.
func TestEmptyGraphMatchesOriginal(t *testing.T) {
	for _, s := range []graphSpec{
		{compound: true, multigraph: true, graph: attrsLabel()},
		{graph: attrsLabel()},
		{compound: true, graph: attrsLabel("rankdir", "LR", "ranker", "longest-path")},
		{compound: true, multigraph: true, graph: attrsLabel("acyclicer", "greedy", "ranker", "tight-tree")},
		{compound: true, multigraph: true, graph: specLabel{kind: specLabelNone}},
	} {
		want := runSpecReference(s, 500)
		got := runSpecLayout(s, 500)
		if want.panicked == "" {
			t.Fatalf("the original no longer panics on %s", s)
		}
		if d := diffOutcomes(want, got); d != "" {
			t.Errorf("empty graph differs from the original:\n%s\nspec:\n%s", d, s)
		}
	}
}

// LayoutTyped on an input without nodes must fail like the original Layout on
// in.Graph().
func TestEmptyInputLayoutTypedMatchesOriginal(t *testing.T) {
	for _, in := range []LayoutInput{DefaultLayoutInput(), {}, {RankDir: "LR", MarginX: 3}} {
		want, got := runTypedPair(in, 77)
		if want.panicked == "" {
			t.Fatalf("the original no longer panics on %+v", in)
		}
		if d := diffOutcomes(want, got); d != "" {
			t.Errorf("LayoutTyped differs from the original Layout(in.Graph()):\n%s\ninput %+v", d, in)
		}
	}
}

// An infinite minlen gives a node the rank +Inf; the original's order then
// fails in make([]int, 0, maxRank) ("cap out of range"), while the typed order
// used to fail with an index out of range. (A huge finite minlen makes both
// create dummy nodes without end.)
func TestInfiniteRankMatchesOriginal(t *testing.T) {
	for _, minlen := range []float64{math.Inf(1)} {
		s := graphSpec{compound: true, multigraph: true, graph: attrsLabel(), ops: []specOp{
			{kind: specOpSetEdge, v: "a", w: "b", label: attrsLabel("minlen", minlen)},
			{kind: specOpSetEdge, v: "b", w: "c", label: attrsLabel()},
		}}
		want := runSpecReference(s, 900)
		got := runSpecLayout(s, 900)
		if want.hung {
			t.Fatalf("the original hangs on minlen %v", minlen)
		}
		if d := diffOutcomes(want, got); d != "" {
			t.Errorf("minlen %v differs from the original:\n%s", minlen, d)
		}
		t.Logf("minlen %v: original panic %q err %q", minlen, want.panicked, want.err)
	}
}

// captureStdout runs fn and returns what it printed to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Skipf("cannot capture stdout here: %v", err) // js/wasm has no pipes
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	func() {
		defer func() { os.Stdout = saved }()
		fn()
	}()
	_ = w.Close()
	return <-done
}

var debugTimes = regexp.MustCompile(`time: [0-9]+ms`)

// The DebugTiming lines of a typed run that declines are the original's: one
// buildLayoutGraph line, then the lines of the phases that finish.
func TestDebugTimingOnDeclineMatchesOriginal(t *testing.T) {
	specs := []graphSpec{
		// Panics in the original (assignOrder): one buildLayoutGraph line.
		{compound: true, multigraph: true, graph: attrsLabel(), ops: []specOp{
			{kind: specOpSetParent, v: "7", w: "101"},
			{kind: specOpSetParent, v: "2", w: "101"},
			{kind: specOpSetParent, v: "24", w: "101"},
			{kind: specOpSetEdge, v: "2", w: "21", label: attrsLabel(), name: "(2 -> 21)[2]", hasName: true},
			{kind: specOpSetEdge, v: "2", w: "24", label: attrsLabel("minlen", math.NaN())},
			{kind: specOpSetEdge, v: "21", w: "8", label: attrsLabel(), name: "(21 -> 8)[11]", hasName: true},
			{kind: specOpSetEdge, v: "16", w: "2", label: attrsLabel(), name: "(16 -> 2)[12]", hasName: true},
			{kind: specOpSetEdge, v: "16", w: "10", label: attrsLabel()},
			{kind: specOpSetEdge, v: "10", w: "7", label: attrsLabel()},
		}},
		// The empty graph.
		{compound: true, multigraph: true, graph: attrsLabel()},
		// A regular graph (no decline): all three lines.
		{compound: true, multigraph: true, graph: attrsLabel(), ops: []specOp{
			{kind: specOpSetEdge, v: "a", w: "b", label: attrsLabel()},
		}},
	}
	for i, s := range specs {
		var wantPanic, gotPanic any
		want := captureStdout(t, func() {
			g, _ := buildReferenceFromSpec(s)
			defer func() { wantPanic = recover() }()
			_ = referenceLayoutOpts(g, LayoutOptions{DebugTiming: true})
		})
		got := captureStdout(t, func() {
			g, _ := buildFromSpec(s)
			defer func() { gotPanic = recover() }()
			_ = Layout(g, LayoutOptions{DebugTiming: true})
		})
		want = debugTimes.ReplaceAllString(want, "time: Nms")
		got = debugTimes.ReplaceAllString(got, "time: Nms")
		if want != got {
			t.Errorf("spec %d: DebugTiming output differs:\noriginal %q\nnew      %q", i, want, got)
		}
		if fmt.Sprint(wantPanic) != fmt.Sprint(gotPanic) {
			t.Errorf("spec %d: panic differs: original %v, new %v", i, wantPanic, gotPanic)
		}
	}
}

// inputEdgeLabel raises updateInputGraph's runtime error for an input edge the
// layout lost or left without points.
func TestUpdateMissingPointsRuntimeError(t *testing.T) {
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{{ID: "a", Parent: -1, Width: 10, Height: 10}, {ID: "b", Parent: -1, Width: 10, Height: 10}}
	in.Edges = []LayoutEdge{NewLayoutEdge(0, 1)}
	for _, corrupt := range []string{"points", "edge"} {
		s := new(lstate)
		if !s.buildTyped(&in) {
			t.Fatal("not accepted")
		}
		if err := s.run(); err != nil {
			t.Fatal(err)
		}
		k := s.inputEdges[0]
		e := s.g.findEdge(k.v, k.w, k.n)
		if corrupt == "points" {
			s.el[s.g.edges[e].label].hasPoints = false
		} else {
			s.g.removeEdge(e)
		}
		var got any
		out := LayoutOutput{Width: -1}
		func() {
			defer func() { got = recover() }()
			s.updateTyped(&in, &out)
		}()
		re, ok := got.(runtime.Error)
		if !ok {
			t.Fatalf("%s: panic %T %v, want a runtime.Error", corrupt, got, got)
		}
		if want := "interface conversion: interface {} is nil, not []dagro.Point"; re.Error() != want {
			t.Errorf("%s: panic %q, want %q", corrupt, re.Error(), want)
		}
		if out.Width != -1 || out.Nodes != nil {
			t.Errorf("%s: out was written before the failure: %+v", corrupt, out)
		}
	}
}

// Concurrent typed layouts publish their draws: the counter advances by the
// sum of the draws of each call, as with the original's per-draw increments.
func TestConcurrentCounterDraws(t *testing.T) {
	build := func() *Graph {
		g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{})
		g.SetEdge("a", "b", Attrs{"minlen": 3.0})
		g.SetEdge("b", "c", Attrs{})
		g.SetEdge("c", "a", Attrs{})
		g.SetNode("p")
		_ = g.SetParent("c", "p")
		return g
	}
	start := uniqueIDCounter.Load()
	if err := Layout(build()); err != nil {
		t.Fatal(err)
	}
	one := uniqueIDCounter.Load() - start
	if one == 0 {
		t.Fatal("the graph draws no counter values")
	}
	const workers, rounds = 8, 50
	start = uniqueIDCounter.Load()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				if err := Layout(build()); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if got, want := uniqueIDCounter.Load()-start, one*workers*rounds; got != want {
		t.Errorf("counter advanced by %d, want %d", got, want)
	}
}

// The original's panic messages that the typed engine's own panics stand for
// never reach a caller of Layout.
func TestNoTypedPanicTextEscapes(t *testing.T) {
	slowUnderRace(t)
	z := newSpecGenerator(4242, "minlen0,weirdnums")
	for i := 0; i < 3000; i++ {
		s := z.spec(14)
		got := runSpecLayout(s, 1000)
		if strings.Contains(got.panicked, "typed engine") || strings.Contains(got.panicked, "missing node in") ||
			strings.Contains(got.panicked, "without border segments") {
			t.Fatalf("case %d: typed panic escaped: %q\n%s", i, got.panicked, s)
		}
	}
}

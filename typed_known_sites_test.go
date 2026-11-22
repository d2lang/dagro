package dagro

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// Where the original fails with a Go runtime
// error at a known site, the typed engine performs the same failing
// operation (failNilMapWrite, makeRankList) instead of declining to the
// Attrs engine, so such inputs cost the typed engine's time.

// knownSite runs the typed engine on g in a fresh state (build, then
// runGuarded) and reports the known site a propagated panic came from ("" when
// run finished or declined), with the panic value and the draws made.
func knownSite(build func(*lstate) bool) (site string, value any, draws int, declined bool) {
	s := new(lstate)
	defer func() {
		if r := recover(); r != nil {
			value = r
			draws = len(s.drawn)
			st := string(debug.Stack())
			for _, fn := range []string{"assignOrder", "insertSelfEdges", "positionY", "makeRankList"} {
				if strings.Contains(st, "(*lstate)."+fn+"(") || strings.Contains(st, "dagro."+fn+"(") {
					site = fn
					return
				}
			}
			site = "other: " + fmt.Sprint(r)
		}
	}()
	if !build(s) {
		return "", nil, 0, false
	}
	_, ok := s.runGuarded(s.run)
	return "", nil, len(s.drawn), !ok
}

// loadKnownSiteInput reads a LayoutInput fixture of testdata/knownsites.
func loadKnownSiteInput(t *testing.T, name string) LayoutInput {
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

// panicValue returns the value fn panics with.
func panicValue(fn func()) (p any) {
	defer func() { p = recover() }()
	fn()
	return nil
}

// Every known site raises, from the typed engine itself (no decline to the
// Attrs engine), the original's runtime error: the same panic value, the
// counter where the original leaves it, the input untouched, and through the
// Graph API the same DebugTiming lines. The LayoutInputs are d2 requests
// captured from generated diagrams and minimized against the original.
func TestKnownSitesRaiseOriginalPanic(t *testing.T) {
	typedCases := []struct {
		fixture, site string
	}{
		{"insertselfedges_layer_gap.json", "insertSelfEdges"},
		{"assignorder_layer_gap.json", "assignOrder"},
	}
	for _, c := range typedCases {
		in := loadKnownSiteInput(t, c.fixture)
		uniqueIDCounter.Store(3000)
		site, value, draws, _ := knownSite(func(s *lstate) bool { return s.buildTyped(&in) })
		if site != c.site {
			t.Fatalf("%s: typed engine stopped at %q (%v), want the %s site", c.fixture, site, value, c.site)
		}
		if _, ok := value.(runtime.Error); !ok || fmt.Sprint(value) != "assignment to entry in nil map" {
			t.Fatalf("%s: panic value %T %v", c.fixture, value, value)
		}
		for _, start := range []uint64{0, 3000, 99} {
			want, got := runTypedPair(in, start)
			if want.panicked != "assignment to entry in nil map" {
				t.Fatalf("%s: the original gives %+v", c.fixture, want)
			}
			if d := diffOutcomes(want, got); d != "" {
				t.Fatalf("%s (counter start %d): LayoutTyped differs from the original:\n%s", c.fixture, start, d)
			}
		}
		// The same input through the Graph API, with DebugTiming.
		var wantPanic, gotPanic any
		wantOut, captured := captureStdoutIfPossible(func() {
			g, err := referenceGraphFromInput(&in)
			if err != nil {
				t.Fatal(err)
			}
			uniqueIDCounter.Store(500)
			wantPanic = panicValue(func() { _ = referenceLayoutOpts(g, LayoutOptions{DebugTiming: true}) })
		})
		wantCounter := uniqueIDCounter.Load()
		g, err := in.Graph()
		if err != nil {
			t.Fatal(err)
		}
		before := dumpGraph(g)
		gotOut, _ := captureStdoutIfPossible(func() {
			uniqueIDCounter.Store(500)
			gotPanic = panicValue(func() { _ = Layout(g, LayoutOptions{DebugTiming: true}) })
		})
		if gotPanic != wantPanic {
			t.Errorf("%s: Layout panics with %T %v, the original with %T %v", c.fixture, gotPanic, gotPanic, wantPanic, wantPanic)
		}
		if n := uniqueIDCounter.Load(); n != wantCounter {
			t.Errorf("%s: counter %d after Layout, the original's %d", c.fixture, n, wantCounter)
		}
		if after := dumpGraph(g); after != before {
			t.Errorf("%s: Layout changed its input before panicking", c.fixture)
		}
		wantOut = debugTimes.ReplaceAllString(wantOut, "time: Nms")
		gotOut = debugTimes.ReplaceAllString(gotOut, "time: Nms")
		if captured && wantOut != gotOut {
			t.Errorf("%s: DebugTiming output %q, the original's %q", c.fixture, gotOut, wantOut)
		}
		t.Logf("%s: %d values drawn before the %s site", c.fixture, draws, c.site)
	}

	// A Graph-API spec on which the original's assignOrder fails.
	g, _ := buildFromSpec(declineSpec())
	if site, value, _, _ := knownSite(func(s *lstate) bool { return s.build(g) }); site != "assignOrder" {
		t.Fatalf("decline spec: typed engine stopped at %q (%v)", site, value)
	}

	// The empty graph: the original's order fails on make([]int, 0, -1).
	empty := DefaultLayoutInput()
	if site, value, _, _ := knownSite(func(s *lstate) bool { return s.buildTyped(&empty) }); site != "makeRankList" ||
		fmt.Sprint(value) != "runtime error: makeslice: cap out of range" {
		t.Fatalf("empty graph: typed engine stopped at %q (%v)", site, value)
	}
	wantPanic := panicValue(func() {
		_ = referenceLayoutOpts(NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{}))
	})
	gotPanic := panicValue(func() {
		_ = Layout(NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{}))
	})
	typedPanic := panicValue(func() { _ = LayoutTyped(&empty, new(LayoutOutput)) })
	if gotPanic != wantPanic || typedPanic != wantPanic {
		t.Fatalf("empty graph: Layout %v, LayoutTyped %v, the original %v", gotPanic, typedPanic, wantPanic)
	}
}

// captureStdoutIfPossible is captureStdout where os.Pipe works; elsewhere (js/wasm)
// it runs fn without capturing and reports false.
func captureStdoutIfPossible(fn func()) (string, bool) {
	r, w, err := os.Pipe()
	if err != nil {
		fn()
		return "", false
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()
	func() {
		defer func() { os.Stdout = saved }()
		fn()
	}()
	_ = w.Close()
	return <-done, true
}

// declineSpec panics in the original (assignOrder writes into a nil
// map) and makes the typed engine decline (the DebugTiming spec of
// typed_panic_test.go).
func declineSpec() graphSpec {
	return graphSpec{compound: true, multigraph: true, graph: attrsLabel(), ops: []specOp{
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
}

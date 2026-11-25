package dagro

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// LayoutTyped in a build without the Graph engine fallback (typed_only.go,
// the dagro_typed_only tag):
// what it does there, and that such a build does not link the Graph engine.
// The tests that compare LayoutTyped with Layout(in.Graph()) run in both
// kinds of build; where the fallback is missing they hold LayoutTyped to what
// it documents for that build (typedOnlyKind).
//
//	go test -tags dagro_typed_only ./...

// The kinds of typedOnlyKind.
const (
	kindTyped    = "typed"     // the typed engine lays the input out, or fails with the original's own panic
	kindNotTyped = "not typed" // buildTyped does not accept the input
	kindGaveUp   = "gave up"   // the typed engine panics or declines: runGuarded reports false
)

// typedOnlyKind says what LayoutTyped does with in when the counter is at
// start, as far as a build without the Graph fallback can tell the cases
// apart: there kindNotTyped gives an error and kindGaveUp a panic of the
// typed engine, while the fallback hands both to the Graph engine.
func typedOnlyKind(in *LayoutInput, start uint64) (kind string) {
	var probe LayoutOutput
	if typedFastPaths && layoutSingleNode(in, &probe) {
		return kindTyped
	}
	s := new(lstate)
	if !s.buildTyped(in) {
		return kindNotTyped
	}
	saved := uniqueIDCounter.Load()
	uniqueIDCounter.Store(start)
	defer func() {
		uniqueIDCounter.Store(saved)
		if recover() != nil {
			kind = kindTyped // a known site: the panic is the original's
		}
	}()
	if _, ok := s.runGuarded(s.run); !ok {
		return kindGaveUp
	}
	return kindTyped
}

// notTypedError is the error of LayoutTyped, without the Graph fallback, for
// an input buildTyped does not accept.
func notTypedError(in *LayoutInput) error {
	if _, err := in.Graph(); err != nil {
		return err
	}
	return ErrGraphEngine
}

// diffTypedOutcomes is diffOutcomes for the pair runTypedPair(in, start)
// returns. Without the Graph fallback it holds got, LayoutTyped's outcome,
// to the reference only where the typed engine lays the input out, and to
// the documented error or panic elsewhere.
func diffTypedOutcomes(in LayoutInput, start uint64, want, got layoutOutcome) string {
	if graphFallback {
		return diffOutcomes(want, got)
	}
	switch typedOnlyKind(&in, start) {
	case kindNotTyped:
		if wantErr := notTypedError(&in).Error(); got.err != wantErr || got.panicked != "" || got.counter != 0 || got.hung {
			return fmt.Sprintf("an input the typed engine does not accept: want error %q and no draws, got %+v", wantErr, got)
		}
		return ""
	case kindGaveUp:
		if got.panicked == "" {
			return fmt.Sprintf("the typed engine gives up on the input, but LayoutTyped did not panic: %+v", got)
		}
		return ""
	}
	return diffOutcomes(want, got)
}

// skipWithoutTypedGreedy skips a test of the typed engine's greedy acyclicer
// in a build that leaves it out (typedGreedy).
func skipWithoutTypedGreedy(t *testing.T) {
	t.Helper()
	if !typedGreedy {
		t.Skip("the typed engine has no greedy acyclicer in this build")
	}
}

// layoutAny is LayoutTyped for tests that are not about its fallback: in a
// build without it, an input that LayoutTyped leaves to the Graph engine is
// laid out through the Graph here.
func layoutAny(in *LayoutInput, out *LayoutOutput) error {
	err := LayoutTyped(in, out)
	if !graphFallback && errors.Is(err, ErrGraphEngine) {
		return layoutViaGraph(in, out)
	}
	return err
}

// typedOnlyFixture is a graph the typed engine lays out.
func typedOnlyFixture() LayoutInput {
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{
		{ID: "0", Parent: -1, Width: 120, Height: 80},
		{ID: "1", Parent: 0, Width: 40, Height: 30},
		{ID: "2", Parent: 0, Width: 50, Height: 30},
		{ID: "3", Parent: -1, Width: 60, Height: 40},
	}
	e := NewLayoutEdge(1, 2)
	e.Name, e.HasName, e.Width, e.Height, e.LabelPos = "(1 -> 2)[0]", true, 20, 10, "c"
	in.Edges = []LayoutEdge{e, NewLayoutEdge(2, 3), NewLayoutEdge(3, 1), NewLayoutEdge(3, 3)}
	return in
}

// Without the Graph fallback, LayoutTyped lays out what the typed engine
// accepts exactly as Layout lays out in.Graph(), returns ErrGraphEngine for
// every other valid input, leaving out untouched, and in.Graph()'s error for
// an invalid one.
func TestLayoutTypedWithoutGraphFallback(t *testing.T) {
	if graphFallback {
		t.Skip("this build has the Graph fallback: go test -tags dagro_typed_only")
	}
	base := typedOnlyFixture()
	var want, got LayoutOutput
	if err := layoutViaGraph(&base, &want); err != nil {
		t.Fatal(err)
	}
	if err := LayoutTyped(&base, &got); err != nil {
		t.Fatalf("typed input: %v", err)
	}
	typedAPICompare(t, "typed input", want, got)

	with := func(change func(in *LayoutInput)) LayoutInput {
		in := base
		in.Nodes = append([]LayoutNode(nil), base.Nodes...)
		in.Edges = append([]LayoutEdge(nil), base.Edges...)
		change(&in)
		return in
	}
	notTyped := map[string]LayoutInput{
		"duplicate id":         with(func(in *LayoutInput) { in.Nodes[3].ID = "1" }),
		"duplicate edge":       with(func(in *LayoutInput) { in.Edges = append(in.Edges, in.Edges[1]) }),
		"empty id":             with(func(in *LayoutInput) { in.Nodes[3].ID = "" }),
		"edge-id delimiter":    with(func(in *LayoutInput) { in.Nodes[3].ID = "a\x01b" }),
		"edge to a compound":   with(func(in *LayoutInput) { in.Edges = append(in.Edges, NewLayoutEdge(3, 0)) }),
		"greedy acyclicer":     with(func(in *LayoutInput) { in.Acyclicer = "greedy" }),
		"greedy, one node":     {RankDir: "TB", Acyclicer: "greedy", MarginX: 1, Nodes: []LayoutNode{{ID: "a", Parent: -1, Width: 2, Height: 2}}},
		"greedy, invalid edge": {RankDir: "TB", Acyclicer: "greedy", Nodes: []LayoutNode{{ID: "a", Parent: -1}}, Edges: []LayoutEdge{NewLayoutEdge(0, 7)}},
	}
	for name, in := range notTyped {
		wantErr := ErrGraphEngine
		switch name {
		case "greedy, invalid edge":
			_, wantErr = in.Graph()
			if wantErr == nil {
				t.Fatalf("%s: in.Graph() accepts the input", name)
			}
		case "edge to a compound":
			// The Graph engine fails on it while ranking; LayoutTyped does
			// not get that far.
			if p := panicValue(func() { _ = layoutViaGraph(&in, new(LayoutOutput)) }); p == nil {
				t.Fatalf("%s: the Graph engine no longer panics", name)
			}
		default:
			if err := layoutViaGraph(&in, new(LayoutOutput)); err != nil {
				t.Fatalf("%s: the Graph engine does not lay the input out: %v", name, err)
			}
		}
		out := LayoutOutput{Width: -1}
		counter := uniqueIDCounter.Load()
		err := LayoutTyped(&in, &out)
		if err == nil || err.Error() != wantErr.Error() || (wantErr == ErrGraphEngine) != errors.Is(err, ErrGraphEngine) {
			t.Errorf("%s: LayoutTyped returned %v, want %v", name, err, wantErr)
		}
		if out.Width != -1 || out.Nodes != nil || out.Edges != nil {
			t.Errorf("%s: LayoutTyped wrote its output: %+v", name, out)
		}
		if n := uniqueIDCounter.Load() - counter; n != 0 {
			t.Errorf("%s: LayoutTyped drew %d generated names", name, n)
		}
	}

	invalid := map[string]LayoutInput{
		"parent cycle":          with(func(in *LayoutInput) { in.Nodes[0].Parent = 1 }),
		"parent out of range":   with(func(in *LayoutInput) { in.Nodes[2].Parent = 9 }),
		"endpoint out of range": with(func(in *LayoutInput) { in.Edges[1].W = 9 }),
		"duplicate id, cycle":   with(func(in *LayoutInput) { in.Nodes[3].ID, in.Nodes[3].Parent = "1", 2; in.Nodes[2].Parent = 1 }),
	}
	for name, in := range invalid {
		_, wantErr := in.Graph()
		if wantErr == nil {
			t.Fatalf("%s: in.Graph() accepts the input", name)
		}
		err := LayoutTyped(&in, new(LayoutOutput))
		if err == nil || err.Error() != wantErr.Error() || errors.Is(err, ErrGraphEngine) {
			t.Errorf("%s: LayoutTyped returned %v, want in.Graph()'s %v", name, err, wantErr)
		}
	}
}

// Without the Graph fallback a panic of the typed engine reaches the caller
// of LayoutTyped, whose state is dropped; with it, the Graph engine takes
// over and the caller sees what Layout(in.Graph()) does (TestCounterReplay,
// TestNoTypedPanicTextEscapes). The inputs are the generated ones on which
// the typed engine gives up (odd minlens and sizes: no D2 diagram).
func TestLayoutTypedPanicWithoutGraphFallback(t *testing.T) {
	if graphFallback {
		t.Skip("this build has the Graph fallback: go test -tags dagro_typed_only")
	}
	z := inputGenerator{newSpecGenerator(20261122, "minlen0,weirdnums")}
	fixture := typedOnlyFixture()
	var want LayoutOutput
	if err := layoutViaGraph(&fixture, &want); err != nil {
		t.Fatal(err)
	}
	gaveUp := 0
	for i := 0; i < 700 && gaveUp < 10; i++ {
		in := z.input(14)
		start := uint64(1 + z.rng.Intn(200000))
		if typedOnlyKind(&in, start) != kindGaveUp {
			continue
		}
		gaveUp++
		lstateCache.Store(nil)
		uniqueIDCounter.Store(start)
		out := LayoutOutput{Width: -1}
		if p := panicValue(func() { _ = LayoutTyped(&in, &out) }); p == nil {
			t.Fatalf("input %d: LayoutTyped did not panic; out %+v", i, out)
		}
		if out.Width != -1 || out.Nodes != nil || out.Edges != nil {
			t.Errorf("input %d: LayoutTyped wrote its output before it panicked: %+v", i, out)
		}
		if lstateCache.Load() != nil {
			t.Errorf("input %d: the state of a call that panicked was kept", i)
		}
		// The next call starts from a fresh state.
		var got LayoutOutput
		if err := LayoutTyped(&fixture, &got); err != nil {
			t.Fatal(err)
		}
		typedAPICompare(t, fmt.Sprintf("after the panic of input %d", i), want, got)
	}
	if gaveUp == 0 {
		t.Fatal("the typed engine gave up on none of the generated inputs")
	}
	t.Logf("%d inputs on which the typed engine gives up", gaveUp)
}

// graphEngineSymbols are functions of the Graph engine and of the Graph
// operations a layout needs beyond building a Graph, one or more from each
// phase and file: none may be in a program that only uses the typed API.
var graphEngineSymbols = []string{
	"dagro.Layout", "dagro.layoutAttrs", "dagro.layoutAttrsBuilt", "dagro.runLayout", "dagro.buildLayoutGraph",
	"dagro.updateInputGraph", "dagro.canonicalize", "dagro.number", "dagro.asAttrs", "dagro.num",
	"dagro.runAcyclic", "dagro.dfsFAS", "dagro.greedyFAS", "dagro.runNestingGraph", "dagro.rank",
	"dagro.networkSimplex", "dagro.longestPath", "dagro.feasibleTree", "dagro.simplify", "dagro.asNonCompoundGraph",
	"dagro.runNormalize", "dagro.parentDummyChains", "dagro.addBorderSegments", "dagro.order", "dagro.sortSubgraph",
	"dagro.resolveConflicts", "dagro.buildLayerGraphNodes", "dagro.initOrderNodes", "dagro.position",
	"dagro.positionX", "dagro.horizontalCompaction", "dagro.translateGraph", "dagro.assignNodeIntersects",
	"dagro.addDummyNode", "dagro.buildLayerMatrix", "dagro.intersectRect",
	"dagro.(*lstate).build", "dagro.(*lstate).update", "dagro.(*lstate).greedyFAS.func", "dagro.(*lstate).runGuarded",
	"dagro.(*Graph).RemoveNode", "dagro.(*Graph).RemoveEdge", "dagro.(*Graph).Nodes", "dagro.(*Graph).Edges",
	"dagro.(*Graph).OutEdges", "dagro.(*Graph).InEdges", "dagro.(*Graph).Successors", "dagro.(*Graph).Predecessors",
	"dagro.(*Graph).Children", "dagro.(*Graph).Neighbors", "dagro.(*Graph).NodeEdges",
	"dagro.jsObjectKeyOrder", "dagro.jsConcatString",
}

// typedEngineSymbols must be in that program: the check reads the right
// symbol table.
var typedEngineSymbols = []string{
	"dagro.LayoutTyped", "dagro.(*lstate).buildTyped", "dagro.(*lstate).run", "dagro.(*lstate).order",
	"dagro.(*LayoutInput).Graph", "dagro.(*Graph).SetNode",
}

// A program that only uses the typed API, the way D2's dagre adapter does,
// links none of the Graph engine when it is built without the Graph
// fallback, and all of it otherwise. The program is testdata/typedonly; it
// is built with the gc toolchain, whose linker, like TinyGo's, keeps only
// what main reaches, with inlining off so that every function that is
// reached keeps its symbol.
func TestTypedOnlyLinksNoGraphEngine(t *testing.T) {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" || runtime.Compiler != "gc" {
		t.Skip("needs the go command")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("needs the go command: %v", err)
	}
	symbols := func(tags string) map[string]bool {
		t.Helper()
		bin := filepath.Join(t.TempDir(), "typedonly")
		build := exec.Command(goTool, "build", "-tags", tags, "-gcflags=all=-l", "-o", bin, "./testdata/typedonly")
		build.Env = append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "GOFLAGS=")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build -tags %q: %v\n%s", tags, err, out)
		}
		out, err := exec.Command(goTool, "tool", "nm", bin).Output()
		if err != nil {
			t.Fatalf("go tool nm: %v", err)
		}
		set := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if name := fields[len(fields)-1]; strings.HasPrefix(name, "github.com/d2lang/dagro.") {
				set[strings.TrimPrefix(name, "github.com/d2lang/")] = true
			}
		}
		return set
	}
	has := func(set map[string]bool, name string) bool {
		if strings.HasSuffix(name, ".func") { // any closure of the function
			for s := range set {
				if strings.HasPrefix(s, name) {
					return true
				}
			}
			return false
		}
		return set[name]
	}

	typedOnly := symbols("dagro_typed_only")
	for _, name := range typedEngineSymbols {
		if !has(typedOnly, name) {
			t.Errorf("typed-only build: %s is not linked (did it get another name?)", name)
		}
	}
	for _, name := range graphEngineSymbols {
		if has(typedOnly, name) {
			t.Errorf("typed-only build links %s", name)
		}
	}
	// No function of the Graph engine's files is linked at all, whatever its
	// name: everything dagro contributes is on the list below or belongs to
	// the typed engine's types.
	for name := range typedOnly {
		if !typedOnlyAllowed(name) {
			t.Errorf("typed-only build links %s, which is neither the typed engine's nor needed to build a Graph", name)
		}
	}

	// With the fallback the same program links the Graph engine: the list
	// names functions that exist.
	full := symbols("")
	for _, name := range append(append([]string(nil), graphEngineSymbols...), typedEngineSymbols...) {
		if name == "dagro.jsConcatString" || name == "dagro.(*Graph).Neighbors" || name == "dagro.(*Graph).InEdges" {
			// Inlined away or only used by tests and other public API.
			continue
		}
		if !has(full, name) {
			t.Errorf("build with the Graph fallback: %s is not linked (did it get another name?)", name)
		}
	}
	t.Logf("dagro symbols linked: %d typed-only, %d with the Graph fallback", len(typedOnly), len(full))
}

// typedOnlyAllowed reports whether a dagro symbol may be in a typed-only
// program: the typed engine (its types' methods and the functions of its
// files) and what LayoutInput.Graph needs to build a Graph.
func typedOnlyAllowed(name string) bool {
	name = strings.TrimPrefix(name, "dagro.")
	name = strings.TrimPrefix(name, ".dict.") // a generic instance's dictionary
	for _, typ := range []string{
		"(*lstate).", "(*igraph).", "(*lnames).", "(*orderState).", "(*posState).", "(*rankState).", "(*rtree).",
		"(*rgraph).", "(*incidence).", "(*blockGraph).", "(*nlabel).", "(*sortLevel).", "(*layerGraph).",
		"(*rankSorter).", "(*sortableSorter).", "(*unsortableSorter).", "(*LayoutInput).",
		// Building a Graph.
		"(*orderedSet).", "(*orderedCounter).", "(*edgeMap).",
	} {
		if strings.HasPrefix(name, typ) {
			return true
		}
	}
	if method, ok := strings.CutPrefix(name, "(*Graph)."); ok {
		switch method {
		case "SetGraph", "SetNode", "SetParent", "setParent", "Parent", "removeFromParentsChildList", "setEdge",
			"ensureEdgeEndpointMaps":
			return true
		}
		return false
	}
	if i := strings.IndexAny(name, ".["); i >= 0 {
		name = name[:i] // a closure, a generic instance, or "" for the package's .inittask
	}
	switch name {
	case "LayoutTyped", "layoutSingleNode", "layoutNotTyped", "layoutBuilt", "NewLayoutEdge", "DefaultLayoutInput",
		"getLState", "putLState", "newIGraph", "newNLabel", "newELabel", "typedEligibleName", "generatedLike",
		"jsOrderIDs", "jsArrayIndex", "cmpBytes", "cmpBytesString", "lowerRankdir", "finiteOr", "intersectRectXY",
		"appendCenteredSelfEdgePoints", "copyIDLayering", "makeRankList", "capsLayers", "firstTypedNode",
		"hasEqualTruthyBarycenters", "mergeTypedEntries", "sortTypedOrderEntries",
		"resize", "grow", "appendGrow", "reserve",
		"jsTruthyNumber", "maxJS2", "minJS2", "lodashMin2", "lodashMax2", "jsStringGreater",
		// Package state.
		"init", "", "ErrGraphEngine", "errDecline", "errIntersectInside", "uniqueIDCounter", "lstateCache", "lstatePool",
		"nlabelNone", "typedFastPaths", "dummyPrefixes", "rtreeCompactAlways",
		// Building a Graph.
		"NewGraph", "newGraphWithCapacity", "newEdgeMap", "newEdgeMapWithCapacity", "newOrderedSet", "newOrderedCounter",
		"edgeArgsToID", "edgeArgsToObj":
		return true
	}
	return strings.HasPrefix(name, "sz") // element sizes (typed_retain.go)
}

package dagro

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"
)

// The typed engine must reproduce the Attrs engine exactly. These tests run
// both on the same random inputs, starting the process-wide unique-id counter
// at the same value, and require identical labels (float bits included),
// identical counter consumption, and identical panics.

func typedDiffBuild(in diffInput) *Graph {
	g := NewGraph(in.Options).SetGraph(cloneAttrs(in.Graph))
	for _, node := range in.Nodes {
		g.SetNode(node.ID, cloneAttrs(node.Attrs))
	}
	for _, node := range in.Nodes {
		if node.Parent != nil {
			if err := g.SetParent(node.ID, *node.Parent); err != nil {
				panic(err)
			}
		}
	}
	for _, edge := range in.Edges {
		if edge.Name != nil {
			g.SetEdge(edge.V, edge.W, cloneAttrs(edge.Attrs), *edge.Name)
		} else {
			g.SetEdge(edge.V, edge.W, cloneAttrs(edge.Attrs))
		}
	}
	return g
}

type typedDiffOutcome struct {
	g        *Graph
	err      error
	panicked any
	counter  uint64
}

func typedDiffRun(in diffInput, start uint64, layout func(*Graph) error) (out typedDiffOutcome) {
	out.g = typedDiffBuild(in)
	uniqueIDCounter.Store(start)
	defer func() {
		out.counter = uniqueIDCounter.Load()
		if r := recover(); r != nil {
			out.panicked = r
		}
	}()
	out.err = layout(out.g)
	return out
}

func referenceLayout(g *Graph) error {
	return layoutAttrs(g, func(_ string, fn func() error) error { return fn() })
}

func typedDiffEqualValue(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && math.Float64bits(x) == math.Float64bits(y)
	case []Point:
		y, ok := b.([]Point)
		if !ok || len(x) != len(y) || (x == nil) != (y == nil) {
			return false
		}
		for i := range x {
			if math.Float64bits(x[i].X) != math.Float64bits(y[i].X) || math.Float64bits(x[i].Y) != math.Float64bits(y[i].Y) {
				return false
			}
		}
		return true
	case Attrs:
		y, ok := b.(Attrs)
		if !ok || len(x) != len(y) || (x == nil) != (y == nil) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !typedDiffEqualValue(v, w) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

func typedDiffCompare(t *testing.T, name string, want, got typedDiffOutcome) bool {
	t.Helper()
	if (want.panicked != nil) != (got.panicked != nil) {
		t.Errorf("%s: panic mismatch: reference %v, typed %v", name, want.panicked, got.panicked)
		return false
	}
	if want.panicked != nil {
		return true
	}
	if (want.err == nil) != (got.err == nil) || want.err != nil && want.err.Error() != got.err.Error() {
		t.Errorf("%s: error mismatch: reference %v, typed %v", name, want.err, got.err)
		return false
	}
	if want.counter != got.counter {
		t.Errorf("%s: unique-id counter %d (reference) vs %d (typed)", name, want.counter, got.counter)
		return false
	}
	if !typedDiffEqualValue(want.g.Graph(), got.g.Graph()) {
		t.Errorf("%s: graph label differs:\nreference %v\ntyped     %v", name, want.g.Graph(), got.g.Graph())
		return false
	}
	wn, gn := want.g.Nodes(), got.g.Nodes()
	if !reflect.DeepEqual(wn, gn) {
		t.Errorf("%s: node lists differ", name)
		return false
	}
	for _, v := range wn {
		if !typedDiffEqualValue(want.g.Node(v), got.g.Node(v)) {
			t.Errorf("%s: node %q differs:\nreference %v\ntyped     %v", name, v, want.g.Node(v), got.g.Node(v))
			return false
		}
	}
	we, ge := want.g.Edges(), got.g.Edges()
	if !reflect.DeepEqual(we, ge) {
		t.Errorf("%s: edge lists differ", name)
		return false
	}
	for _, e := range we {
		if !typedDiffEqualValue(want.g.Edge(e), got.g.Edge(e)) {
			t.Errorf("%s: edge %v differs:\nreference %v\ntyped     %v", name, e, want.g.Edge(e), got.g.Edge(e))
			return false
		}
	}
	return true
}

func typedDiffCheck(t *testing.T, name string, in diffInput) (bool, bool) {
	t.Helper()
	start := uint64(1000 + rand.Intn(100000))
	want := typedDiffRun(in, start, referenceLayout)
	got := typedDiffRun(in, start, func(g *Graph) error { return Layout(g) })
	return typedDiffCompare(t, name, want, got), want.panicked != nil
}

func pick[T any](rng *rand.Rand, xs ...T) T { return xs[rng.Intn(len(xs))] }

// typedRandomInput draws graphs from the D2 profile and well beyond it:
// array-index and ordinary names, names that collide with generated dummy
// names, deep compound trees, cycles, self-loops, parallel and reversed
// edges, every rankdir/ranker/acyclicer/align, and loosely typed attributes.
func typedRandomInput(rng *rand.Rand) diffInput {
	in := diffInput{Options: GraphOptions{Compound: true, Multigraph: true}, Graph: Attrs{}}
	if rng.Intn(10) == 0 {
		in.Options.Compound = false
	}
	gl := in.Graph
	switch rng.Intn(10) {
	case 0:
	case 1:
		gl["rankdir"] = pick(rng, "tb", "bt", "lr", "rl", "Lr", "x")
	case 2:
		gl["rankDir"] = pick(rng, "LR", "BT")
	case 3:
		gl["rankdir"] = 7.0
	default:
		gl["rankdir"] = pick(rng, "TB", "BT", "LR", "RL")
	}
	for _, k := range []string{"nodesep", "edgesep", "ranksep"} {
		switch rng.Intn(8) {
		case 0:
		case 1:
			gl[k] = strconv.Itoa(rng.Intn(80))
		case 2:
			gl[k] = 0.0
		default:
			gl[k] = float64(5 + rng.Intn(120))
		}
	}
	if rng.Intn(4) == 0 {
		gl["marginx"] = float64(rng.Intn(20))
		gl["marginy"] = pick(rng, 0.0, 3.0, 11.5)
	}
	if rng.Intn(5) == 0 {
		gl["ranker"] = pick(rng, "network-simplex", "tight-tree", "longest-path", "other")
	}
	if rng.Intn(6) == 0 {
		gl["acyclicer"] = "greedy"
	}
	if rng.Intn(6) == 0 {
		gl["align"] = pick(rng, "UL", "ur", "Dl", "dr", "x")
	}

	nodeCount := 1 + rng.Intn(14)
	ids := make([]string, nodeCount)
	mode := rng.Intn(6)
	for i := range ids {
		switch mode {
		case 0, 1:
			ids[i] = strconv.Itoa(i)
		case 2:
			ids[i] = strconv.Itoa(rng.Intn(40))
		case 3:
			ids[i] = string(rune('a'+i%26)) + strconv.Itoa(i/26)
		case 4:
			ids[i] = pick(rng, "_d", "_d1", "_d7", "_root", "_bt", "_bb", "_bl", "_br", "_se", "_ep", "_root3", "01", "4294967295", "-1", "1.5", "z", strconv.Itoa(i), "é", "Z")
		default:
			if rng.Intn(2) == 0 {
				ids[i] = strconv.Itoa(rng.Intn(1000))
			} else {
				ids[i] = fmt.Sprintf("n%d", rng.Intn(1000))
			}
		}
	}
	// distinct ids
	seen := map[string]bool{}
	uniq := ids[:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	ids = uniq
	if mode == 1 {
		rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	}

	clusterCount := 0
	if in.Options.Compound {
		clusterCount = rng.Intn(5)
	}
	var clusters []string
	for c := 0; c < clusterCount; c++ {
		name := pick(rng, fmt.Sprintf("c%d", c), strconv.Itoa(100+c), fmt.Sprintf("_c%d", c))
		if seen[name] {
			continue
		}
		seen[name] = true
		clusters = append(clusters, name)
	}
	parentOf := map[string]string{}
	for i, c := range clusters {
		if i > 0 && rng.Intn(2) == 0 {
			parentOf[c] = clusters[rng.Intn(i)]
		}
	}
	nodeAttrs := func() Attrs {
		a := Attrs{}
		switch rng.Intn(12) {
		case 0:
		case 1:
			a["WIDTH"], a["Height"] = float64(rng.Intn(90)), float64(rng.Intn(90))
		case 2:
			a["width"], a["height"] = strconv.Itoa(10+rng.Intn(90)), "0x20"
		case 3:
			a["width"], a["height"] = 0.0, float64(rng.Intn(50))
		default:
			a["width"], a["height"] = float64(1+rng.Intn(150)), float64(1+rng.Intn(120))
		}
		a["id"] = "x"
		return a
	}
	order := append([]string(nil), clusters...)
	order = append(order, ids...)
	if rng.Intn(3) == 0 {
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	}
	for _, id := range order {
		in.Nodes = append(in.Nodes, diffNodeInput{ID: id, Attrs: nodeAttrs()})
	}
	for i := range in.Nodes {
		id := in.Nodes[i].ID
		if p, ok := parentOf[id]; ok {
			in.Nodes[i].Parent = strptr(p)
		} else if len(clusters) > 0 && !contains(clusters, id) && rng.Intn(3) != 0 {
			in.Nodes[i].Parent = strptr(clusters[rng.Intn(len(clusters))])
		}
	}
	// Leaves only (edges touching compound nodes take the Attrs engine).
	leaves := append([]string(nil), ids...)
	hasChild := map[string]bool{}
	for _, n := range in.Nodes {
		if n.Parent != nil {
			hasChild[*n.Parent] = true
		}
	}
	if rng.Intn(25) == 0 && len(clusters) > 0 {
		leaves = append(leaves, clusters[0])
	}
	edgeCount := rng.Intn(3*len(ids) + 1)
	for i := 0; i < edgeCount && len(leaves) > 0; i++ {
		v, w := pick(rng, leaves...), pick(rng, leaves...)
		if rng.Intn(10) == 0 {
			w = v
		}
		e := diffEdgeInput{V: v, W: w, Attrs: Attrs{}}
		switch rng.Intn(10) {
		case 0:
		case 1:
			e.Name = strptr(pick(rng, "rev1", "rev2", "rev3", ""))
		case 2:
			e.Name = strptr(defaultEdgeName)
		default:
			e.Name = strptr(fmt.Sprintf("(%s -> %s)[%d]", v, w, i))
		}
		a := e.Attrs
		switch rng.Intn(6) {
		case 0:
		case 1:
			a["width"], a["height"] = float64(rng.Intn(60)), float64(rng.Intn(30))
		default:
			a["width"], a["height"] = float64(rng.Intn(60)), float64(rng.Intn(30))
			a["labelpos"] = pick[any](rng, "c", "l", "r", "L", "R", "c", "c", 3.0)
		}
		if rng.Intn(6) == 0 {
			a["minlen"] = pick[any](rng, 1.0, 2.0, 3.0, "2", true)
		}
		if rng.Intn(6) == 0 {
			// No zero weights: the Attrs engine's network simplex (like
			// dagre.js) can pivot forever on zero-weight edges.
			a["weight"] = pick[any](rng, 1.0, 2.0, 5.0, "3")
		}
		if rng.Intn(8) == 0 {
			a["labeloffset"] = pick[any](rng, 0.0, 5.0, 12.0)
		}
		if _, ok := a["labelpos"]; !ok && rng.Intn(15) == 0 {
			// Differently cased keys only without the lowercase key: with both,
			// the Attrs engine picks one in map iteration order.
			a["LabelPos"] = "l"
		}
		in.Edges = append(in.Edges, e)
	}
	_ = hasChild
	return in
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestTypedEngineMatchesAttrsEngineRandom(t *testing.T) {
	cases := 1000
	if s := os.Getenv("DAGRO_TYPED_DIFF_CASES"); s != "" {
		cases, _ = strconv.Atoi(s)
	}
	seed := int64(20261120)
	if s := os.Getenv("DAGRO_TYPED_DIFF_SEED"); s != "" {
		seed, _ = strconv.ParseInt(s, 10, 64)
	}
	rng := rand.New(rand.NewSource(seed))
	failures, panics := 0, 0
	for i := 0; i < cases; i++ {
		in := typedRandomInput(rng)
		ok, panicked := typedDiffCheck(t, fmt.Sprintf("seed %d case %d", seed, i), in)
		if panicked {
			panics++
		}
		if !ok {
			failures++
			if failures >= 5 {
				t.Fatalf("stopping after %d failures", failures)
			}
		}
	}
	t.Logf("%d cases, %d panicked in both engines", cases, panics)
}

func TestTypedEngineMatchesAttrsEngineFixtures(t *testing.T) {
	for _, f := range differentialFixtures() {
		typedDiffCheck(t, f.name, f.input)
	}
	typedDiffCheck(t, "concurrent-dummy-ids", concurrentDummyIDFixture())
	rng := rand.New(rand.NewSource(3112026))
	for i := 0; i < 300; i++ {
		typedDiffCheck(t, fmt.Sprintf("d2-profile %d", i), d2ProfileRandomInput(rng, i))
	}
}

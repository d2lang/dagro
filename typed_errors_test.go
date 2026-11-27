package dagro

import (
	"errors"
	"math"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestLayoutTypedUnsupportedInputs checks the error LayoutTyped returns for
// each kind of input the engine does not lay out, and that it leaves out
// untouched.
func TestLayoutTypedUnsupportedInputs(t *testing.T) {
	base := func() LayoutInput {
		in := DefaultLayoutInput()
		in.Nodes = []LayoutNode{{ID: "a", Parent: -1, Width: 10, Height: 10}, {ID: "b", Parent: -1, Width: 20, Height: 20}, {ID: "c", Parent: -1, Width: 5, Height: 5}}
		in.Edges = []LayoutEdge{NewLayoutEdge(0, 1), NewLayoutEdge(1, 2)}
		return in
	}
	cases := []struct {
		name, want string
		change     func(*LayoutInput)
	}{
		{"no nodes", "no nodes", func(in *LayoutInput) { in.Nodes, in.Edges = nil, nil }},
		{"empty id", `node 1 has the id ""`, func(in *LayoutInput) { in.Nodes[1].ID = "" }},
		{"id with \\x01", `node 2 has the id "c\x01"`, func(in *LayoutInput) { in.Nodes[2].ID = "c\x01" }},
		{"repeated id", `nodes 0 and 2 have the same id "a"`, func(in *LayoutInput) { in.Nodes[2].ID = "a" }},
		{"parent out of range", "node 1 has parent 3 out of range", func(in *LayoutInput) { in.Nodes[1].Parent = 3 }},
		{"parent cycle", "making node 0 the parent of node 1 would create a cycle", func(in *LayoutInput) {
			in.Nodes[1].Parent, in.Nodes[0].Parent = 0, 1
			in.Edges = nil
		}},
		{"self-parent", "making node 2 the parent of node 2 would create a cycle", func(in *LayoutInput) { in.Nodes[2].Parent = 2 }},
		{"edge endpoint out of range", "edge 1 has an endpoint out of range", func(in *LayoutInput) { in.Edges[1].W = 3 }},
		{"negative edge endpoint", "edge 0 has an endpoint out of range", func(in *LayoutInput) { in.Edges[0].V = -1 }},
		{"edge endpoint with children", "edge 0 has an endpoint with children", func(in *LayoutInput) { in.Nodes[2].Parent = 0 }},
		{"repeated edge", "edge 2 repeats the endpoints and name of an earlier edge", func(in *LayoutInput) {
			in.Edges = append(in.Edges, in.Edges[0])
		}},
		{"repeated named edge", "edge 1 repeats the endpoints and name of an earlier edge", func(in *LayoutInput) {
			in.Edges[0].Name, in.Edges[0].HasName = "x", true
			in.Edges[1] = in.Edges[0]
		}},
	}
	for _, c := range cases {
		in := base()
		c.change(&in)
		out := LayoutOutput{Width: 7, Nodes: []NodeLayout{{X: 1}}}
		err := LayoutTyped(&in, &out)
		if !errors.Is(err, ErrUnsupportedInput) {
			t.Errorf("%s: error %v, want one wrapping ErrUnsupportedInput", c.name, err)
			continue
		}
		if want := "dagro: unsupported input: " + c.want; !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%s: error %q, want %q", c.name, err, want)
		}
		if !reflect.DeepEqual(out, LayoutOutput{Width: 7, Nodes: []NodeLayout{{X: 1}}}) {
			t.Errorf("%s: out changed to %+v", c.name, out)
		}
	}

	// The same edges with distinct names, or with the name "" and no name,
	// are laid out.
	in := base()
	in.Edges = append(in.Edges, in.Edges[0], in.Edges[0])
	in.Edges[2].Name, in.Edges[2].HasName = "", true
	in.Edges[3].Name, in.Edges[3].HasName = "x", true
	var out LayoutOutput
	if err := LayoutTyped(&in, &out); err != nil {
		t.Fatal(err)
	}
}

// TestLayoutTypedDeclinesHugeRanks checks that an infinite minlen gives an
// error instead of exhausting memory: the ranks convert to the largest int on
// arm64, which the engine declines (more than 2^26 ranks), and to a negative
// int on amd64, on which the layout fails.
func TestLayoutTypedDeclinesHugeRanks(t *testing.T) {
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{{ID: "a", Parent: -1, Width: 1, Height: 1}, {ID: "b", Parent: -1, Width: 1, Height: 1}}
	e := NewLayoutEdge(0, 1)
	e.Minlen = math.Inf(1)
	in.Edges = []LayoutEdge{e}
	var out LayoutOutput
	err := LayoutTyped(&in, &out)
	var perr *PanicError
	declined := errors.Is(err, ErrUnsupportedInput) && strings.Contains(err.Error(), "more than 67108864 ranks")
	if !declined && !errors.As(err, &perr) {
		t.Fatalf("error %v", err)
	}
}

// TestLayoutTypedPanicBecomesError checks that inputs on which the layout
// writes into the label of an entry missing from a layer, and inputs on which
// the engine raises a Go runtime error, give a *PanicError, and that the next
// layout is unaffected.
func TestLayoutTypedPanicBecomesError(t *testing.T) {
	ok := d2ProfileInput(rand.New(rand.NewSource(1)), 0)
	var want LayoutOutput
	if err := LayoutTyped(&ok, &want); err != nil {
		t.Fatal(err)
	}
	check := func(what string, in LayoutInput, wantText string, runtimeError bool) {
		t.Helper()
		var out LayoutOutput
		err := LayoutTyped(&in, &out)
		var perr *PanicError
		if !errors.As(err, &perr) {
			t.Fatalf("%s: error %v, want a *PanicError", what, err)
		}
		if !strings.Contains(err.Error(), wantText) || !strings.HasPrefix(err.Error(), "dagro: layout failed: ") {
			t.Fatalf("%s: error %q, want it to contain %q", what, err, wantText)
		}
		var rerr runtime.Error
		if errors.As(err, &rerr) != runtimeError {
			t.Fatalf("%s: error %v: runtime error %v, want %v", what, err, !runtimeError, runtimeError)
		}
		var got LayoutOutput
		if err := LayoutTyped(&ok, &got); err != nil || goldenDump(&got) != goldenDump(&want) {
			t.Fatalf("%s: the next layout differs (%v)", what, err)
		}
	}
	// A NaN minlen makes the engine index out of range. A few inputs of this
	// set leave an empty position in a layer instead, a panic with dagre's
	// message (the next check covers those), and are skipped: where NaN
	// converts to int differently (amd64), one could come first.
	z := newInputGen(6, "minlen0")
	found := 0
	for i := 0; i < 1000 && found < 3; i++ {
		in := z.input(12)
		var out LayoutOutput
		var perr *PanicError
		if err := LayoutTyped(&in, &out); errors.As(err, &perr) && !strings.Contains(err.Error(), "missing from a layer") {
			found++
			check("minlen0", in, "runtime error: index out of range", true)
		}
	}
	if found == 0 {
		t.Fatal("no generated input panics")
	}

	// Fractional minlens can rank a node outside its cluster's ranks, which
	// leaves an empty position in a layer that no ordering fills (input 411,
	// whose numbers are all finite, so that it fails the same way on every
	// platform).
	z = newInputGen(5, "fracminlen,fracweight")
	gaps := 0
	for i := 0; i < 1000 && gaps == 0; i++ {
		in := z.input(12)
		var out LayoutOutput
		if err := LayoutTyped(&in, &out); err != nil && strings.Contains(err.Error(), "missing from a layer") {
			gaps++
			check("layer gap", in, `dagre fails on this input: it sets "order" on a node missing from a layer`, false)
		}
	}
	if gaps == 0 {
		t.Fatal("no generated input leaves an empty position in a layer")
	}
}

// TestConcurrentLayouts lays out D2-shaped inputs, and the inputs whose
// ordering order repairs (one of which LayoutTyped lays out twice), from
// several goroutines and compares the results with serial ones (run it with
// -race).
func TestConcurrentLayouts(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	inputs := make([]LayoutInput, 200)
	for i := range inputs {
		inputs[i] = d2ProfileInput(rng, i)
	}
	for _, c := range constraintCycleInputs(t) {
		inputs = append(inputs, c.in)
	}
	want := make([]string, len(inputs))
	for i := range inputs {
		var out LayoutOutput
		if err := LayoutTyped(&inputs[i], &out); err != nil {
			t.Fatal(err)
		}
		want[i] = goldenDump(&out)
	}
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			var out LayoutOutput // reused across calls
			for k := range inputs {
				i := (k + 25*g) % len(inputs)
				if err := LayoutTyped(&inputs[i], &out); err != nil || goldenDump(&out) != want[i] {
					errs <- "input " + inputs[i].Nodes[0].ID
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error("concurrent layout differs from the serial one:", e)
	}
}

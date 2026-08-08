package dagro

import (
	"io"
	"math"
	"os"
	"reflect"
	"regexp"
	"testing"
)

func captureTestStdout(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = original
		_ = writer.Close()
		_ = reader.Close()
	}()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = original
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestTimeAndNoTime(t *testing.T) {
	var timedResult any
	output := captureTestStdout(t, func() {
		timedResult = Time("foo", func() string { return "bar" })
	})
	if timedResult != "bar" {
		t.Fatalf("Time return value = %q, want bar", timedResult)
	}
	if !regexp.MustCompile(`^foo time: [0-9]+ms\n$`).MatchString(output) {
		t.Fatalf("Time output = %q, want upstream timing format", output)
	}

	var untimedResult any
	output = captureTestStdout(t, func() {
		untimedResult = NoTime("foo", func() string { return "bar" })
	})
	if untimedResult != "bar" || output != "" {
		t.Fatalf("NoTime = (%q, %q), want (bar, no output)", untimedResult, output)
	}

	output = captureTestStdout(t, func() {
		if got := Time("void", func() {}); got != nil {
			t.Fatalf("void Time result = %#v, want nil", got)
		}
	})
	if !regexp.MustCompile(`^void time: [0-9]+ms\n$`).MatchString(output) {
		t.Fatalf("void Time output = %q, want upstream timing format", output)
	}
}

func TestSimplifyPreservesSingleEdgeLabel(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true}).
		SetEdge("a", "b", Attrs{"weight": float64(1), "minlen": float64(1)})
	simple := simplify(g)
	want := Attrs{"weight": float64(1), "minlen": float64(1)}
	if simple.EdgeCount() != 1 || !reflect.DeepEqual(simple.EdgeByArgs("a", "b"), want) {
		t.Fatalf("simplified single edge = %#v (count %d), want %#v", simple.EdgeByArgs("a", "b"), simple.EdgeCount(), want)
	}
}

func TestSimplify(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true}).SetGraph(Attrs{"name": "g"})
	g.SetEdge("a", "b", Attrs{"weight": float64(1), "minlen": float64(1)})
	g.SetEdge("a", "b", Attrs{"weight": float64(2), "minlen": float64(2)}, "multi")
	g.SetEdge("b", "c", Attrs{"weight": float64(4), "minlen": float64(1)})

	simple := simplify(g)
	if simple.IsMultigraph() || simple.EdgeCount() != 2 {
		t.Fatalf("simplified graph options/count: multigraph=%v edges=%d",
			simple.IsMultigraph(), simple.EdgeCount())
	}
	if got := asAttrs(simple.EdgeByArgs("a", "b")); num(got, "weight") != 3 || num(got, "minlen") != 2 {
		t.Fatalf("collapsed label = %#v", got)
	}
	if !reflect.DeepEqual(simple.Graph(), g.Graph()) || !reflect.DeepEqual(simple.Nodes(), g.Nodes()) {
		t.Fatalf("simplify did not preserve graph/nodes: graph=%#v nodes=%v", simple.Graph(), simple.Nodes())
	}
}

func TestAsNonCompoundGraph(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{"name": "g"})
	g.SetNode("a", Attrs{"kind": "leaf"})
	_ = g.SetParent("a", "sg")
	g.SetNode("b", Attrs{"kind": "leaf"})
	g.SetEdge("a", "b", Attrs{"kind": "plain"})
	g.SetEdge("a", "b", Attrs{"kind": "multi"}, "multi")

	simple := asNonCompoundGraph(g)
	if simple.IsCompound() || !simple.IsMultigraph() || simple.HasNode("sg") {
		t.Fatalf("non-compound options/nodes: compound=%v multi=%v nodes=%v",
			simple.IsCompound(), simple.IsMultigraph(), simple.Nodes())
	}
	if got := stringValue(asAttrs(simple.Node("a")), "kind"); got != "leaf" {
		t.Fatalf("node label = %q", got)
	}
	if got := stringValue(asAttrs(simple.EdgeByArgs("a", "b")), "kind"); got != "plain" {
		t.Fatalf("unnamed edge label = %q", got)
	}
	if got := stringValue(asAttrs(simple.EdgeByArgs("a", "b", "multi")), "kind"); got != "multi" {
		t.Fatalf("named edge label = %q", got)
	}
	if !reflect.DeepEqual(simple.Graph(), g.Graph()) {
		t.Fatalf("graph label = %#v, want %#v", simple.Graph(), g.Graph())
	}
}

func TestSuccessorAndPredecessorWeights(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true})
	g.SetEdge("a", "b", Attrs{"weight": float64(2)})
	g.SetEdge("b", "c", Attrs{"weight": float64(1)})
	g.SetEdge("b", "c", Attrs{"weight": float64(2)}, "multi")
	g.SetEdge("b", "d", Attrs{"weight": float64(1)}, "multi")

	wantSuccessors := map[string]map[string]float64{
		"a": {"b": 2}, "b": {"c": 3, "d": 1}, "c": {}, "d": {},
	}
	if got := successorWeights(g); !reflect.DeepEqual(got, wantSuccessors) {
		t.Fatalf("successorWeights = %#v, want %#v", got, wantSuccessors)
	}
	wantPredecessors := map[string]map[string]float64{
		"a": {}, "b": {"a": 2}, "c": {"b": 3}, "d": {"b": 1},
	}
	if got := predecessorWeights(g); !reflect.DeepEqual(got, wantPredecessors) {
		t.Fatalf("predecessorWeights = %#v, want %#v", got, wantPredecessors)
	}
}

func TestIntersectRect(t *testing.T) {
	rect := Attrs{"x": float64(0), "y": float64(0), "width": float64(2), "height": float64(2)}
	for _, point := range []Point{{X: 2, Y: 6}, {X: 2, Y: -6}, {X: 6, Y: 2}, {X: -6, Y: 2}, {X: 5}, {Y: 5}} {
		cross, err := intersectRect(rect, point)
		if err != nil {
			t.Fatalf("intersectRect(%+v): %v", point, err)
		}
		if math.Abs(cross.X) != 1 && math.Abs(cross.Y) != 1 {
			t.Fatalf("intersection %+v does not touch border", cross)
		}
		// Cross, center, and point must be collinear.
		if math.Abs(cross.X*point.Y-cross.Y*point.X) > 1e-12 {
			t.Fatalf("intersection %+v is not on center-to-point slope %+v", cross, point)
		}
	}
	if _, err := intersectRect(rect, Point{}); err == nil {
		t.Fatal("intersectRect accepted a point at the rectangle center")
	}
}

func TestBuildLayerMatrix(t *testing.T) {
	g := NewGraph().
		SetNode("a", Attrs{"rank": float64(0), "order": float64(0)}).
		SetNode("b", Attrs{"rank": float64(0), "order": float64(1)}).
		SetNode("c", Attrs{"rank": float64(1), "order": float64(0)}).
		SetNode("d", Attrs{"rank": float64(1), "order": float64(1)}).
		SetNode("e", Attrs{"rank": float64(2), "order": float64(0)})
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}
	if got := buildLayerMatrix(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("buildLayerMatrix = %v, want %v", got, want)
	}
}

func TestNormalizeRanks(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true}).
		SetNode("a", Attrs{"rank": float64(-3)}).
		SetNode("b", Attrs{"rank": float64(-2)}).
		SetNode("sg", Attrs{})
	_ = g.SetParent("a", "sg")
	normalizeRanks(g)
	requireRank(t, g, "a", 0)
	requireRank(t, g, "b", 1)
	if has(asAttrs(g.Node("sg")), "rank") {
		t.Fatalf("normalizeRanks assigned compound rank: %#v", g.Node("sg"))
	}
}

func TestRemoveEmptyRanks(t *testing.T) {
	for _, tt := range []struct {
		name     string
		bottom   float64
		wantRank float64
	}{
		{name: "border ranks", bottom: 4, wantRank: 1},
		{name: "non-border ranks", bottom: 8, wantRank: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGraph().SetGraph(Attrs{"nodeRankFactor": float64(4)}).
				SetNode("a", Attrs{"rank": float64(0)}).
				SetNode("b", Attrs{"rank": tt.bottom})
			removeEmptyRanks(g)
			requireRank(t, g, "a", 0)
			requireRank(t, g, "b", tt.wantRank)
		})
	}

	t.Run("non-array-index ranks", func(t *testing.T) {
		g := NewGraph().SetGraph(Attrs{"nodeRankFactor": float64(4)}).
			SetNode("a", Attrs{"rank": float64(0)}).
			SetNode("infinite", Attrs{"rank": math.Inf(1)}).
			SetNode("too-large", Attrs{"rank": float64(4294967295)})
		removeEmptyRanks(g)
		requireRank(t, g, "a", 0)
		if got := num(asAttrs(g.Node("infinite")), "rank"); !math.IsInf(got, 1) {
			t.Fatalf("infinite rank = %v, want +Inf", got)
		}
		requireRank(t, g, "too-large", 4294967295)
	})
}

func TestIntersectRectTreatsNaNAsFalsy(t *testing.T) {
	_, err := intersectRect(
		Attrs{"x": float64(0), "y": float64(0), "width": float64(10), "height": float64(10)},
		Point{X: math.NaN(), Y: math.NaN()},
	)
	if err == nil {
		t.Fatal("intersectRect with two NaN deltas did not report the center-point error")
	}
}

func TestDummyIDsAreGraphLocalAndAvoidCollisions(t *testing.T) {
	first := NewGraph().SetNode("_d1", Attrs{})
	id := addDummyNode(first, "edge", Attrs{}, "_d")
	if id != "_d2" {
		t.Fatalf("collision-skipping dummy id = %q, want _d2", id)
	}
	second := NewGraph()
	if got := addDummyNode(second, "edge", Attrs{}, "_d"); got != "_d1" {
		t.Fatalf("second graph did not reset ids: %q", got)
	}
}

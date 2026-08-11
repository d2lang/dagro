package dagro

import (
	"math"
	"reflect"
	"testing"
)

func newBKTestGraph() *Graph {
	return NewGraph().SetGraph(Attrs{})
}

func newBKConflictGraph() (*Graph, [][]string) {
	g := newBKTestGraph().SetDefaultEdgeLabel(func(string, string, *string) any { return Attrs{} })
	g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
	g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1)})
	g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0)})
	g.SetNode("d", Attrs{"rank": float64(1), "order": float64(1)})
	g.SetEdge("a", "d")
	g.SetEdge("b", "c")
	return g, buildLayerMatrix(g)
}

func requirePositionStringMap(t *testing.T, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func requirePositionFloatMap(t *testing.T, got, want map[string]float64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestFindType1ConflictsDoesNotMarkEdgesWithoutConflict(t *testing.T) {
	g, layering := newBKConflictGraph()
	g.RemoveEdgeByArgs("a", "d")
	g.RemoveEdgeByArgs("b", "c")
	g.SetEdge("a", "c")
	g.SetEdge("b", "d")

	conflicts := findType1Conflicts(g, layering)
	if hasConflict(conflicts, "a", "c") || hasConflict(conflicts, "b", "d") {
		t.Fatalf("unexpected conflict: %#v", conflicts)
	}
}

func TestFindType1ConflictsDoesNotMarkType0Conflicts(t *testing.T) {
	for _, dummy := range []string{"", "a", "b", "c", "d"} {
		t.Run(dummy, func(t *testing.T) {
			g, layering := newBKConflictGraph()
			if dummy != "" {
				asAttrs(g.Node(dummy))["dummy"] = true
			}
			conflicts := findType1Conflicts(g, layering)
			if hasConflict(conflicts, "a", "d") || hasConflict(conflicts, "b", "c") {
				t.Fatalf("unexpected conflict: %#v", conflicts)
			}
		})
	}
}

func TestFindType1ConflictsMarksType1Conflicts(t *testing.T) {
	for _, nonDummy := range []string{"a", "b", "c", "d"} {
		t.Run(nonDummy, func(t *testing.T) {
			g, layering := newBKConflictGraph()
			for _, v := range []string{"a", "b", "c", "d"} {
				if v != nonDummy {
					asAttrs(g.Node(v))["dummy"] = true
				}
			}

			conflicts := findType1Conflicts(g, layering)
			ad, bc := hasConflict(conflicts, "a", "d"), hasConflict(conflicts, "b", "c")
			wantAD := nonDummy == "a" || nonDummy == "d"
			if ad != wantAD || bc == wantAD {
				t.Fatalf("conflicts (a,d)=%v (b,c)=%v, non-dummy=%q", ad, bc, nonDummy)
			}
		})
	}
}

func TestFindType1ConflictsDoesNotMarkType2Conflicts(t *testing.T) {
	g, layering := newBKConflictGraph()
	for _, v := range []string{"a", "b", "c", "d"} {
		asAttrs(g.Node(v))["dummy"] = true
	}
	conflicts := findType1Conflicts(g, layering)
	if hasConflict(conflicts, "a", "d") || hasConflict(conflicts, "b", "c") {
		t.Fatalf("unexpected conflict: %#v", conflicts)
	}
}

func TestFindType2ConflictsFavorsBorderSegments(t *testing.T) {
	t.Run("first crossing", func(t *testing.T) {
		g, layering := newBKConflictGraph()
		for _, v := range []string{"a", "d"} {
			asAttrs(g.Node(v))["dummy"] = true
		}
		for _, v := range []string{"b", "c"} {
			asAttrs(g.Node(v))["dummy"] = "border"
		}
		conflicts := findType2Conflicts(g, layering)
		if !hasConflict(conflicts, "a", "d") || hasConflict(conflicts, "b", "c") {
			t.Fatalf("unexpected conflicts: %#v", conflicts)
		}
	})

	t.Run("second crossing", func(t *testing.T) {
		g, layering := newBKConflictGraph()
		for _, v := range []string{"b", "c"} {
			asAttrs(g.Node(v))["dummy"] = true
		}
		for _, v := range []string{"a", "d"} {
			asAttrs(g.Node(v))["dummy"] = "border"
		}
		conflicts := findType2Conflicts(g, layering)
		if hasConflict(conflicts, "a", "d") || !hasConflict(conflicts, "b", "c") {
			t.Fatalf("unexpected conflicts: %#v", conflicts)
		}
	})
}

func TestFindType2ConflictsStartsNorthBoundaryAtMinusOne(t *testing.T) {
	g := newBKTestGraph()
	g.SetNode("north", Attrs{"dummy": true, "order": float64(-2)})
	g.SetNode("south", Attrs{"dummy": true, "order": float64(0)})
	g.SetEdge("north", "south")

	conflicts := findType2Conflicts(g, [][]string{{"north"}, {"south"}})
	if !hasConflict(conflicts, "north", "south") {
		t.Fatalf("missing conflict with modern -1 north boundary: %#v", conflicts)
	}
}

func TestPositionConflictsAreOrientationIndependentAndComposable(t *testing.T) {
	conflicts := positionConflicts{}
	addConflict(conflicts, "b", "a")
	if !hasConflict(conflicts, "a", "b") || !hasConflict(conflicts, "b", "a") {
		t.Fatal("conflict was orientation-dependent")
	}
	addConflict(conflicts, "a", "c")
	if !hasConflict(conflicts, "a", "b") || !hasConflict(conflicts, "a", "c") {
		t.Fatal("multiple conflicts with one node were not retained")
	}
}

func TestPositionConflictMergeUsesModernShallowOverwrite(t *testing.T) {
	got := shallowMergePositionConflicts(
		positionConflicts{
			"a": {"b": true, "c": true},
			"e": {"f": true},
		},
		positionConflicts{"a": {"d": true}},
	)
	if hasConflict(got, "a", "b") || hasConflict(got, "a", "c") || !hasConflict(got, "a", "d") {
		t.Fatalf("colliding conflict key was not replaced: %#v", got)
	}
	if !hasConflict(got, "e", "f") {
		t.Fatalf("non-colliding conflict key was lost: %#v", got)
	}
}

func TestVerticalAlignment(t *testing.T) {
	t.Run("self without adjacencies", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(0)})
		got := verticalAlignment(g, buildLayerMatrix(g), positionConflicts{}, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"a": "a", "b": "b"})
		requirePositionStringMap(t, got.align, map[string]string{"a": "a", "b": "b"})
	})

	t.Run("sole adjacency", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(0)})
		g.SetEdge("a", "b")
		got := verticalAlignment(g, buildLayerMatrix(g), positionConflicts{}, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"a": "a", "b": "a"})
		requirePositionStringMap(t, got.align, map[string]string{"a": "b", "b": "a"})
	})

	t.Run("left median", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0)})
		g.SetEdge("a", "c")
		g.SetEdge("b", "c")
		got := verticalAlignment(g, buildLayerMatrix(g), positionConflicts{}, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"a": "a", "b": "b", "c": "a"})
		requirePositionStringMap(t, got.align, map[string]string{"a": "c", "b": "b", "c": "a"})
	})

	t.Run("node name and insertion order", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0)})
		g.SetNode("z", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetEdge("z", "c")
		g.SetEdge("b", "c")
		got := verticalAlignment(g, buildLayerMatrix(g), positionConflicts{}, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"z": "z", "b": "b", "c": "z"})
		requirePositionStringMap(t, got.align, map[string]string{"z": "c", "b": "b", "c": "z"})
	})

	t.Run("right median when left unavailable", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0)})
		g.SetEdge("a", "c")
		g.SetEdge("b", "c")
		conflicts := positionConflicts{}
		addConflict(conflicts, "a", "c")
		got := verticalAlignment(g, buildLayerMatrix(g), conflicts, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"a": "a", "b": "b", "c": "b"})
		requirePositionStringMap(t, got.align, map[string]string{"a": "a", "b": "c", "c": "b"})
	})

	t.Run("neither median available", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0)})
		g.SetNode("d", Attrs{"rank": float64(1), "order": float64(1)})
		g.SetEdge("a", "d")
		g.SetEdge("b", "c")
		g.SetEdge("b", "d")
		got := verticalAlignment(g, buildLayerMatrix(g), positionConflicts{}, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"a": "a", "b": "b", "c": "b", "d": "d"})
		requirePositionStringMap(t, got.align, map[string]string{"a": "a", "b": "c", "c": "b", "d": "d"})
	})

	t.Run("odd number of adjacencies", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1)})
		g.SetNode("c", Attrs{"rank": float64(0), "order": float64(2)})
		g.SetNode("d", Attrs{"rank": float64(1), "order": float64(0)})
		g.SetEdge("a", "d")
		g.SetEdge("b", "d")
		g.SetEdge("c", "d")
		got := verticalAlignment(g, buildLayerMatrix(g), positionConflicts{}, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"a": "a", "b": "b", "c": "c", "d": "b"})
		requirePositionStringMap(t, got.align, map[string]string{"a": "a", "b": "d", "c": "c", "d": "b"})
	})

	t.Run("blocks across layers", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(0)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(1)})
		g.SetNode("d", Attrs{"rank": float64(2), "order": float64(0)})
		g.SetPath([]string{"a", "b", "d"})
		g.SetPath([]string{"a", "c", "d"})
		got := verticalAlignment(g, buildLayerMatrix(g), positionConflicts{}, g.Predecessors)
		requirePositionStringMap(t, got.root, map[string]string{"a": "a", "b": "a", "c": "c", "d": "a"})
		requirePositionStringMap(t, got.align, map[string]string{"a": "b", "b": "d", "c": "c", "d": "a"})
	})
}

func TestHorizontalCompaction(t *testing.T) {
	t.Run("single node at origin", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0)})
		xs := horizontalCompaction(g, buildLayerMatrix(g), map[string]string{"a": "a"}, map[string]string{"a": "a"})
		requirePositionFloatMap(t, xs, map[string]float64{"a": 0})
	})

	t.Run("node separation", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["nodesep"] = float64(100)
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100)})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1), "width": float64(200)})
		xs := horizontalCompaction(g, buildLayerMatrix(g), map[string]string{"a": "a", "b": "b"}, map[string]string{"a": "a", "b": "b"})
		requirePositionFloatMap(t, xs, map[string]float64{"a": 0, "b": 100.0/2 + 100 + 200.0/2})
	})

	t.Run("edge separation", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["edgesep"] = float64(20)
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100), "dummy": true})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1), "width": float64(200), "dummy": true})
		xs := horizontalCompaction(g, buildLayerMatrix(g), map[string]string{"a": "a", "b": "b"}, map[string]string{"a": "a", "b": "b"})
		requirePositionFloatMap(t, xs, map[string]float64{"a": 0, "b": 100.0/2 + 20 + 200.0/2})
	})

	t.Run("same block centers", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(0), "width": float64(200)})
		xs := horizontalCompaction(g, buildLayerMatrix(g), map[string]string{"a": "a", "b": "a"}, map[string]string{"a": "b", "b": "a"})
		requirePositionFloatMap(t, xs, map[string]float64{"a": 0, "b": 0})
	})

	t.Run("block separation", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["nodesep"] = float64(75)
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(1), "width": float64(200)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0), "width": float64(50)})
		xs := horizontalCompaction(g, buildLayerMatrix(g), map[string]string{"a": "a", "b": "a", "c": "c"}, map[string]string{"a": "b", "b": "a", "c": "c"})
		want := 50.0/2 + 75 + 200.0/2
		requirePositionFloatMap(t, xs, map[string]float64{"a": want, "b": want, "c": 0})
	})

	t.Run("class separation", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["nodesep"] = float64(75)
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100)})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1), "width": float64(200)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0), "width": float64(50)})
		g.SetNode("d", Attrs{"rank": float64(1), "order": float64(1), "width": float64(80)})
		xs := horizontalCompaction(g, buildLayerMatrix(g),
			map[string]string{"a": "a", "b": "b", "c": "c", "d": "b"},
			map[string]string{"a": "a", "b": "d", "c": "c", "d": "b"})
		b := 100.0/2 + 75 + 200.0/2
		requirePositionFloatMap(t, xs, map[string]float64{
			"a": 0, "b": b, "c": b - 80.0/2 - 75 - 50.0/2, "d": b,
		})
	})

	for _, tc := range []struct {
		name   string
		bWidth float64
		dWidth float64
		wantB  float64
	}{
		{"maximum separation from first layer", 150, 70, 50.0/2 + 75 + 150.0/2},
		{"maximum separation from second layer", 70, 150, 60.0/2 + 75 + 150.0/2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newBKTestGraph()
			asAttrs(g.Graph())["nodesep"] = float64(75)
			g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(50)})
			g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1), "width": tc.bWidth})
			g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0), "width": float64(60)})
			g.SetNode("d", Attrs{"rank": float64(1), "order": float64(1), "width": tc.dWidth})
			xs := horizontalCompaction(g, buildLayerMatrix(g),
				map[string]string{"a": "a", "b": "b", "c": "a", "d": "b"},
				map[string]string{"a": "c", "b": "d", "c": "a", "d": "b"})
			requirePositionFloatMap(t, xs, map[string]float64{"a": 0, "b": tc.wantB, "c": 0, "d": tc.wantB})
		})
	}

	t.Run("cascaded class shift", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["nodesep"] = float64(75)
		for _, node := range []struct {
			v           string
			rank, order float64
		}{
			{"a", 0, 0}, {"b", 0, 1}, {"c", 1, 0}, {"d", 1, 1},
			{"e", 1, 2}, {"f", 2, 0}, {"g", 2, 1},
		} {
			g.SetNode(node.v, Attrs{"rank": node.rank, "order": node.order, "width": float64(50)})
		}
		xs := horizontalCompaction(g, buildLayerMatrix(g),
			map[string]string{"a": "a", "b": "b", "c": "c", "d": "d", "e": "b", "f": "f", "g": "d"},
			map[string]string{"a": "a", "b": "e", "c": "c", "d": "g", "e": "b", "f": "f", "g": "d"})
		sep := 50.0/2 + 75 + 50.0/2
		if xs["a"] != xs["b"]-sep || xs["b"] != xs["e"] || xs["c"] != xs["f"] ||
			xs["d"] != xs["c"]+sep || xs["e"] != xs["d"]+sep || xs["g"] != xs["f"]+sep {
			t.Fatalf("unexpected cascade: %#v", xs)
		}
	})

	for _, tc := range []struct {
		labelPos string
		wantB    float64
		wantCInc float64
	}{
		{"l", 100.0/2 + 50 + 200, 0 + 50 + 300.0/2},
		{"c", 100.0/2 + 50 + 200.0/2, 200.0/2 + 50 + 300.0/2},
		{"r", 100.0/2 + 50 + 0, 200 + 50 + 300.0/2},
	} {
		t.Run("labelpos "+tc.labelPos, func(t *testing.T) {
			g := newBKTestGraph()
			asAttrs(g.Graph())["edgesep"] = float64(50)
			g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100), "dummy": "edge"})
			g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1), "width": float64(200), "dummy": "edge-label", "labelpos": tc.labelPos})
			g.SetNode("c", Attrs{"rank": float64(0), "order": float64(2), "width": float64(300), "dummy": "edge"})
			xs := horizontalCompaction(g, buildLayerMatrix(g),
				map[string]string{"a": "a", "b": "b", "c": "c"},
				map[string]string{"a": "a", "b": "b", "c": "c"})
			requirePositionFloatMap(t, xs, map[string]float64{"a": 0, "b": tc.wantB, "c": tc.wantB + tc.wantCInc})
		})
	}
}

func TestBuildBlockGraphUsesJavaScriptFallbackForNaNPreviousMaximum(t *testing.T) {
	g := newBKTestGraph()
	asAttrs(g.Graph())["nodesep"] = float64(0)
	g.SetNode("a", Attrs{"width": math.NaN()})
	g.SetNode("b", Attrs{"width": float64(0)})
	g.SetNode("c", Attrs{"width": float64(0)})
	g.SetNode("d", Attrs{"width": float64(0)})
	root := map[string]string{"a": "left", "b": "right", "c": "left", "d": "right"}
	blockGraph := buildBlockGraph(g, [][]string{{"a", "b"}, {"c", "d"}}, root, false)
	if got := number(blockGraph.EdgeByArgs("left", "right")); got != 0 {
		t.Fatalf("replaced NaN maximum = %v, want 0", got)
	}
}

func TestAlignCoordinates(t *testing.T) {
	t.Run("single node", func(t *testing.T) {
		xss := positionAlignments{
			"ul": {"a": 50}, "ur": {"a": 100}, "dl": {"a": 50}, "dr": {"a": 200},
		}
		alignCoordinates(xss, xss["ul"])
		for alignment, xs := range xss {
			requirePositionFloatMap(t, xs, map[string]float64{"a": 50})
			_ = alignment
		}
	})

	t.Run("multiple nodes", func(t *testing.T) {
		xss := positionAlignments{
			"ul": {"a": 50, "b": 1000}, "ur": {"a": 100, "b": 900},
			"dl": {"a": 150, "b": 800}, "dr": {"a": 200, "b": 700},
		}
		alignCoordinates(xss, xss["ul"])
		requirePositionFloatMap(t, xss["ul"], map[string]float64{"a": 50, "b": 1000})
		requirePositionFloatMap(t, xss["ur"], map[string]float64{"a": 200, "b": 1000})
		requirePositionFloatMap(t, xss["dl"], map[string]float64{"a": 50, "b": 700})
		requirePositionFloatMap(t, xss["dr"], map[string]float64{"a": 500, "b": 1000})
	})
}

func TestFindSmallestWidthAlignment(t *testing.T) {
	t.Run("smallest width", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"width": float64(50)})
		g.SetNode("b", Attrs{"width": float64(50)})
		xss := positionAlignments{
			"ul": {"a": 0, "b": 1000}, "ur": {"a": -5, "b": 1000},
			"dl": {"a": 5, "b": 2000}, "dr": {"a": 0, "b": 200},
		}
		requirePositionFloatMap(t, findSmallestWidthAlignment(g, xss), xss["dr"])
	})

	t.Run("node width", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"width": float64(50)})
		g.SetNode("b", Attrs{"width": float64(50)})
		g.SetNode("c", Attrs{"width": float64(200)})
		xss := positionAlignments{
			"ul": {"a": 0, "b": 100, "c": 75}, "ur": {"a": 0, "b": 100, "c": 80},
			"dl": {"a": 0, "b": 100, "c": 85}, "dr": {"a": 0, "b": 100, "c": 90},
		}
		requirePositionFloatMap(t, findSmallestWidthAlignment(g, xss), xss["ul"])
	})
}

func TestBalance(t *testing.T) {
	for _, tc := range []struct {
		name string
		xss  positionAlignments
		want map[string]float64
	}{
		{
			"shared median",
			positionAlignments{"ul": {"a": 0}, "ur": {"a": 100}, "dl": {"a": 100}, "dr": {"a": 200}},
			map[string]float64{"a": 100},
		},
		{
			"average different medians",
			positionAlignments{"ul": {"a": 0}, "ur": {"a": 75}, "dl": {"a": 125}, "dr": {"a": 200}},
			map[string]float64{"a": 100},
		},
		{
			"multiple nodes",
			positionAlignments{
				"ul": {"a": 0, "b": 50}, "ur": {"a": 75, "b": 0},
				"dl": {"a": 125, "b": 60}, "dr": {"a": 200, "b": 75},
			},
			map[string]float64{"a": 100, "b": 55},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requirePositionFloatMap(t, balance(tc.xss, ""), tc.want)
		})
	}
}

func TestPositionX(t *testing.T) {
	t.Run("single node", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100)})
		requirePositionFloatMap(t, positionX(g), map[string]float64{"a": 0})
	})

	t.Run("single node block", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(100)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(0), "width": float64(100)})
		g.SetEdge("a", "b")
		requirePositionFloatMap(t, positionX(g), map[string]float64{"a": 0, "b": 0})
	})

	t.Run("single block with differing sizes", func(t *testing.T) {
		g := newBKTestGraph()
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(40)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(0), "width": float64(500)})
		g.SetNode("c", Attrs{"rank": float64(2), "order": float64(0), "width": float64(20)})
		g.SetPath([]string{"a", "b", "c"})
		requirePositionFloatMap(t, positionX(g), map[string]float64{"a": 0, "b": 0, "c": 0})
	})

	t.Run("predecessor centered over same-sized nodes", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["nodesep"] = float64(10)
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(20)})
		g.SetNode("b", Attrs{"rank": float64(1), "order": float64(0), "width": float64(50)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(1), "width": float64(50)})
		g.SetEdge("a", "b")
		g.SetEdge("a", "c")
		got := positionX(g)
		a := got["a"]
		requirePositionFloatMap(t, got, map[string]float64{"a": a, "b": a - 30, "c": a + 30})
	})

	t.Run("blocks on both sides", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["nodesep"] = float64(10)
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(50)})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1), "width": float64(60)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0), "width": float64(70)})
		g.SetNode("d", Attrs{"rank": float64(1), "order": float64(1), "width": float64(80)})
		g.SetEdge("b", "c")
		got := positionX(g)
		b := got["b"]
		requirePositionFloatMap(t, got, map[string]float64{
			"a": b - 60.0/2 - 10 - 50.0/2, "b": b, "c": b, "d": b + 70.0/2 + 10 + 80.0/2,
		})
	})

	t.Run("inner segments", func(t *testing.T) {
		g := newBKTestGraph()
		asAttrs(g.Graph())["nodesep"] = float64(10)
		// Layout supplies both separations before calling positionX. Making the
		// edge separation explicit also avoids the upstream test's accidental
		// undefined/NaN arithmetic while preserving its intended assertions.
		asAttrs(g.Graph())["edgesep"] = float64(10)
		g.SetNode("a", Attrs{"rank": float64(0), "order": float64(0), "width": float64(50), "dummy": true})
		g.SetNode("b", Attrs{"rank": float64(0), "order": float64(1), "width": float64(60)})
		g.SetNode("c", Attrs{"rank": float64(1), "order": float64(0), "width": float64(70)})
		g.SetNode("d", Attrs{"rank": float64(1), "order": float64(1), "width": float64(80), "dummy": true})
		g.SetEdge("b", "c")
		g.SetEdge("a", "d")
		got := positionX(g)
		a := got["a"]
		requirePositionFloatMap(t, got, map[string]float64{
			"a": a, "b": a + 50.0/2 + 10 + 60.0/2,
			"c": a - 70.0/2 - 10 - 80.0/2, "d": a,
		})
	})
}

package dagro

import (
	"math"
	"reflect"
	"testing"
)

func TestLayoutUpstreamNodesOnSameRank(t *testing.T) {
	g := newLayoutTestGraph()
	asAttrs(g.Graph())["nodesep"] = 200.0
	g.SetNode("a", Attrs{"width": 50.0, "height": 100.0})
	g.SetNode("b", Attrs{"width": 75.0, "height": 200.0})
	layoutMustSucceed(t, g)

	assertLayoutPoint(t, asAttrs(g.Node("a")), 25, 100)
	assertLayoutPoint(t, asAttrs(g.Node("b")), 287.5, 100)
}

func TestLayoutUpstreamEdgeLabels(t *testing.T) {
	t.Run("centered label", func(t *testing.T) {
		g := newLayoutTestGraph()
		asAttrs(g.Graph())["ranksep"] = 300.0
		g.SetNode("a", Attrs{"width": 50.0, "height": 100.0})
		g.SetNode("b", Attrs{"width": 75.0, "height": 200.0})
		g.SetEdge("a", "b", Attrs{"width": 60.0, "height": 70.0, "labelpos": "c"})
		layoutMustSucceed(t, g)

		assertLayoutPoint(t, asAttrs(g.Node("a")), 37.5, 50)
		assertLayoutPoint(t, asAttrs(g.Node("b")), 37.5, 570)
		assertLayoutPoint(t, asAttrs(g.EdgeByArgs("a", "b")), 37.5, 285)
	})

	t.Run("long edge label lies between endpoints", func(t *testing.T) {
		g := newLayoutTestGraph()
		asAttrs(g.Graph())["ranksep"] = 300.0
		g.SetNode("a", Attrs{"width": 50.0, "height": 100.0})
		g.SetNode("b", Attrs{"width": 75.0, "height": 200.0})
		g.SetEdge("a", "b", Attrs{
			"width": 60.0, "height": 70.0, "minlen": 2.0, "labelpos": "c",
		})
		layoutMustSucceed(t, g)

		a, b := asAttrs(g.Node("a")), asAttrs(g.Node("b"))
		edge := asAttrs(g.EdgeByArgs("a", "b"))
		assertNear(t, num(edge, "x"), 37.5)
		if y := num(edge, "y"); y <= num(a, "y") || y >= num(b, "y") {
			t.Fatalf("edge label y = %g, want between endpoint centers %g and %g", y, num(a, "y"), num(b, "y"))
		}
	})
}

func TestLayoutUpstreamLongLabelsSeparateRoutes(t *testing.T) {
	for _, rankdir := range []string{"TB", "BT", "LR", "RL"} {
		t.Run(rankdir, func(t *testing.T) {
			g := newLayoutTestGraph()
			graph := asAttrs(g.Graph())
			graph["nodesep"], graph["edgesep"], graph["rankdir"] = 10.0, 10.0, rankdir
			for _, v := range []string{"a", "b", "c", "d"} {
				g.SetNode(v, Attrs{"width": 10.0, "height": 10.0})
			}
			g.SetEdge("a", "c", Attrs{"width": 2000.0, "height": 10.0, "labelpos": "c"})
			g.SetEdge("b", "d", Attrs{"width": 1.0, "height": 1.0})
			layoutMustSucceed(t, g)

			var x1, x2 float64
			if rankdir == "TB" || rankdir == "BT" {
				x1 = num(asAttrs(g.EdgeByArgs("a", "c")), "x")
				x2 = num(asAttrs(g.EdgeByArgs("b", "d")), "x")
			} else {
				x1 = num(asAttrs(g.Node("a")), "x")
				x2 = num(asAttrs(g.Node("c")), "x")
			}
			if separation := math.Abs(x1 - x2); separation <= 1000 {
				t.Fatalf("horizontal separation = %g, want > 1000", separation)
			}
		})
	}
}

func TestLayoutUpstreamLabelOffsets(t *testing.T) {
	for _, rankdir := range []string{"TB", "BT", "LR", "RL"} {
		t.Run(rankdir, func(t *testing.T) {
			g := newLayoutTestGraph()
			graph := asAttrs(g.Graph())
			graph["nodesep"], graph["edgesep"], graph["rankdir"] = 10.0, 10.0, rankdir
			for _, v := range []string{"a", "b", "c", "d"} {
				g.SetNode(v, Attrs{"width": 10.0, "height": 10.0})
			}
			g.SetEdge("a", "b", Attrs{
				"width": 10.0, "height": 10.0, "labelpos": "l", "labeloffset": 1000.0,
			})
			g.SetEdge("c", "d", Attrs{
				"width": 10.0, "height": 10.0, "labelpos": "r", "labeloffset": 1000.0,
			})
			layoutMustSucceed(t, g)

			left, right := asAttrs(g.EdgeByArgs("a", "b")), asAttrs(g.EdgeByArgs("c", "d"))
			leftPoints, rightPoints := layoutEdgePoints(t, left), layoutEdgePoints(t, right)
			if rankdir == "TB" || rankdir == "BT" {
				assertNear(t, num(left, "x")-leftPoints[0].X, -1005)
				assertNear(t, num(right, "x")-rightPoints[0].X, 1005)
			} else {
				assertNear(t, num(left, "y")-leftPoints[0].Y, -1005)
				assertNear(t, num(right, "y")-rightPoints[0].Y, 1005)
			}
		})
	}
}

func TestLayoutUpstreamShortCycle(t *testing.T) {
	g := newLayoutTestGraph()
	asAttrs(g.Graph())["ranksep"] = 200.0
	g.SetNode("a", Attrs{"width": 100.0, "height": 100.0})
	g.SetNode("b", Attrs{"width": 100.0, "height": 100.0})
	g.SetEdge("a", "b", Attrs{"weight": 2.0})
	g.SetEdge("b", "a")
	layoutMustSucceed(t, g)

	assertLayoutPoint(t, asAttrs(g.Node("a")), 50, 50)
	assertLayoutPoint(t, asAttrs(g.Node("b")), 50, 350)
	down := layoutEdgePoints(t, asAttrs(g.EdgeByArgs("a", "b")))
	up := layoutEdgePoints(t, asAttrs(g.EdgeByArgs("b", "a")))
	if len(down) < 2 || down[1].Y <= down[0].Y {
		t.Fatalf("a -> b points do not point down: %#v", down)
	}
	if len(up) < 2 || up[0].Y <= up[1].Y {
		t.Fatalf("b -> a points do not point up: %#v", up)
	}
}

func TestLayoutUpstreamLongEdgeIntersections(t *testing.T) {
	g := newLayoutTestGraph()
	asAttrs(g.Graph())["ranksep"] = 200.0
	g.SetNode("a", Attrs{"width": 100.0, "height": 100.0})
	g.SetNode("b", Attrs{"width": 100.0, "height": 100.0})
	g.SetEdge("a", "b", Attrs{"minlen": 2.0})
	layoutMustSucceed(t, g)

	edge := asAttrs(g.EdgeByArgs("a", "b"))
	points := layoutEdgePoints(t, edge)
	want := []Point{
		{X: 50, Y: 100},
		{X: 50, Y: 200},
		{X: 50, Y: 300},
		{X: 50, Y: 400},
		{X: 50, Y: 500},
	}
	assertLayoutPoints(t, points, want)
	if _, ok := edge["x"]; ok {
		t.Fatalf("unlabeled edge unexpectedly has x coordinate: %#v", edge)
	}
	if _, ok := edge["y"]; ok {
		t.Fatalf("unlabeled edge unexpectedly has y coordinate: %#v", edge)
	}
}

func TestLayoutUpstreamSelfLoops(t *testing.T) {
	for _, rankdir := range []string{"TB", "BT", "LR", "RL"} {
		t.Run(rankdir, func(t *testing.T) {
			g := newLayoutTestGraph()
			graph := asAttrs(g.Graph())
			graph["edgesep"], graph["rankdir"] = 75.0, rankdir
			g.SetNode("a", Attrs{"width": 100.0, "height": 100.0})
			g.SetEdge("a", "a", Attrs{"width": 50.0, "height": 50.0})
			layoutMustSucceed(t, g)

			node := asAttrs(g.Node("a"))
			points := layoutEdgePoints(t, asAttrs(g.EdgeByArgs("a", "a")))
			if len(points) != 7 {
				t.Fatalf("self-loop points = %d, want 7: %#v", len(points), points)
			}
			for _, point := range points {
				if rankdir == "TB" || rankdir == "BT" {
					if point.X <= num(node, "x") || math.Abs(point.Y-num(node, "y")) > num(node, "height")/2+1e-9 {
						t.Errorf("point %#v is outside the expected right side of node %#v", point, node)
					}
				} else if point.Y <= num(node, "y") || math.Abs(point.X-num(node, "x")) > num(node, "width")/2+1e-9 {
					t.Errorf("point %#v is outside the expected lower side of node %#v", point, node)
				}
			}
			if rankdir == "RL" {
				assertLayoutPoints(t, points, []Point{
					{X: 100, Y: 242.5},
					{X: 100, Y: 242.5},
					{X: 50, Y: 192.5},
					{X: 0, Y: 142.5},
					{X: 0, Y: 142.5},
					{X: 50, Y: 192.5},
					{X: 50, Y: 192.5},
				})
			}
		})
	}
}

func TestInsertSelfEdgesSeedsModernSpline(t *testing.T) {
	g := newOrderTestGraph(false)
	edge := Edge{V: "a", W: "a", Name: "loop", HasName: true}
	label := Attrs{"width": 20.0, "height": 10.0}
	g.SetNode("a", Attrs{
		"rank": float64(0), "order": float64(0),
		"selfEdges": []selfEdgeRecord{{e: edge, label: label}},
	})

	insertSelfEdges(g)

	dummy := asAttrs(g.Node("_se"))
	dummyLabel, ok := dummy["edgeLabel"].(Attrs)
	if stringValue(dummy, "dummy") != "selfedge" || !ok || num(dummyLabel, "width") != 20 || has(dummy, "label") {
		t.Fatalf("self-edge dummy = %#v", dummy)
	}
	points, ok := label["points"].([]Point)
	if !ok || len(points) != 7 {
		t.Fatalf("seeded self-edge points = %#v, want seven points", label["points"])
	}
}

func TestPositionSelfEdgesUsesFiniteFallbacks(t *testing.T) {
	g := NewGraph(GraphOptions{Multigraph: true})
	edge := Edge{V: "a", W: "a", Name: "loop", HasName: true}
	label := Attrs{}
	g.SetNode("a", Attrs{
		"x": math.NaN(), "y": math.Inf(1), "width": math.Inf(-1), "height": math.NaN(),
	})
	g.SetNode("_se", Attrs{
		"dummy": "selfedge", "e": edge, "edgeLabel": label,
		"x": math.Inf(1), "y": math.NaN(),
	})

	positionSelfEdges(g)

	points := layoutEdgePoints(t, asAttrs(g.Edge(edge)))
	assertLayoutPoints(t, points, []Point{{}, {}, {}, {}, {}, {}, {}})
	if num(label, "x") != 0 || num(label, "y") != 0 {
		t.Fatalf("self-edge label fallback = (%v,%v), want (0,0)", num(label, "x"), num(label, "y"))
	}
}

func TestLayoutUpstreamCompoundGraphs(t *testing.T) {
	t.Run("single child", func(t *testing.T) {
		g := newLayoutTestGraph()
		g.SetNode("a", Attrs{"width": 50.0, "height": 50.0})
		if err := g.SetParent("a", "sg1"); err != nil {
			t.Fatal(err)
		}
		layoutMustSucceed(t, g)
	})

	t.Run("minimizes subgraph height", func(t *testing.T) {
		g := newLayoutTestGraph()
		for _, v := range []string{"a", "b", "c", "d", "x", "y"} {
			g.SetNode(v, Attrs{"width": 50.0, "height": 50.0})
		}
		g.SetPath([]string{"a", "b", "c", "d"})
		g.SetEdge("a", "x", Attrs{"weight": 100.0})
		g.SetEdge("y", "d", Attrs{"weight": 100.0})
		if err := g.SetParent("x", "sg"); err != nil {
			t.Fatal(err)
		}
		if err := g.SetParent("y", "sg"); err != nil {
			t.Fatal(err)
		}
		layoutMustSucceed(t, g)
		assertNear(t, num(asAttrs(g.Node("x")), "y"), num(asAttrs(g.Node("y")), "y"))
	})

	t.Run("bounds child in every lowercase direction", func(t *testing.T) {
		g := newLayoutTestGraph()
		g.SetNode("a", Attrs{"width": 50.0, "height": 50.0})
		g.SetNode("sg", Attrs{})
		if err := g.SetParent("a", "sg"); err != nil {
			t.Fatal(err)
		}
		for _, rankdir := range []string{"tb", "bt", "lr", "rl"} {
			asAttrs(g.Graph())["rankdir"] = rankdir
			layoutMustSucceed(t, g)
			sg := asAttrs(g.Node("sg"))
			if num(sg, "width") <= 50 || num(sg, "height") <= 50 || num(sg, "x") <= 25 || num(sg, "y") <= 25 {
				t.Errorf("rankdir %s produced invalid subgraph bounds: %#v", rankdir, sg)
			}
		}
	})
}

func TestLayoutUpstreamGraphBounds(t *testing.T) {
	t.Run("dimensions and margins", func(t *testing.T) {
		g := newLayoutTestGraph()
		graph := asAttrs(g.Graph())
		graph["marginx"], graph["marginy"] = 10.0, 20.0
		g.SetNode("a", Attrs{"width": 100.0, "height": 50.0})
		layoutMustSucceed(t, g)

		assertLayoutPoint(t, asAttrs(g.Node("a")), 60, 45)
		assertNear(t, num(graph, "width"), 120)
		assertNear(t, num(graph, "height"), 90)
	})

	// TB is covered by TestLayoutUpstreamExamples. These are the remaining
	// directions from upstream's standalone-node bounding-box matrix.
	for _, rankdir := range []string{"BT", "LR", "RL"} {
		t.Run("standalone node "+rankdir, func(t *testing.T) {
			g := newLayoutTestGraph()
			asAttrs(g.Graph())["rankdir"] = rankdir
			g.SetNode("a", Attrs{"width": 100.0, "height": 200.0})
			layoutMustSucceed(t, g)
			assertLayoutPoint(t, asAttrs(g.Node("a")), 50, 100)
		})
	}

	for _, rankdir := range []string{"TB", "BT", "LR", "RL"} {
		t.Run("left edge label "+rankdir, func(t *testing.T) {
			g := newLayoutTestGraph()
			asAttrs(g.Graph())["rankdir"] = rankdir
			g.SetNode("a", Attrs{"width": 100.0, "height": 100.0})
			g.SetNode("b", Attrs{"width": 100.0, "height": 100.0})
			g.SetEdge("a", "b", Attrs{
				"width": 1000.0, "height": 2000.0, "labelpos": "l", "labeloffset": 0.0,
			})
			layoutMustSucceed(t, g)

			edge := asAttrs(g.EdgeByArgs("a", "b"))
			if rankdir == "TB" || rankdir == "BT" {
				assertNear(t, num(edge, "x"), 500)
			} else {
				assertNear(t, num(edge, "y"), 1000)
			}
		})
	}
}

func TestLayoutPreservesMultipleReversedParallelPartners(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{
		"edgesep": float64(32), "nodesep": float64(17), "rankdir": "LR", "ranksep": float64(84),
	})
	g.SetNode("cluster", Attrs{"width": float64(149), "height": float64(80)})
	g.SetNode("1", Attrs{"width": float64(113), "height": float64(50)})
	g.SetNode("7", Attrs{"width": float64(125), "height": float64(71)})
	if err := g.SetParent("1", "cluster"); err != nil {
		t.Fatal(err)
	}
	g.SetEdge("1", "7", Attrs{"width": float64(1), "height": float64(29), "labelpos": "c"}, "forward")
	g.SetEdge("7", "1", Attrs{"width": float64(50), "height": float64(34), "labelpos": "c"}, "reverse-1")
	g.SetEdge("7", "1", Attrs{"width": float64(55), "height": float64(9), "labelpos": "c"}, "reverse-2")

	layoutMustSucceed(t, g)
	graph := asAttrs(g.Graph())
	assertNear(t, num(graph, "width"), 461)
	assertNear(t, num(graph, "height"), 200.5)
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		for _, key := range []string{"x", "y", "width", "height"} {
			if value := num(node, key); math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("node %s %s is not finite: %v", v, key, value)
			}
		}
	}
	for _, edge := range g.Edges() {
		edgeAttrs := asAttrs(g.Edge(edge))
		points := layoutEdgePoints(t, edgeAttrs)
		if len(points) < 2 {
			t.Fatalf("edge %#v has %d points", edge, len(points))
		}
		for i, point := range points {
			if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
				t.Fatalf("edge %#v point %d is not finite: %#v", edge, i, point)
			}
		}
		for _, key := range []string{"x", "y"} {
			if !has(edgeAttrs, key) {
				continue
			}
			if value := num(edgeAttrs, key); math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("edge %#v %s is not finite: %v", edge, key, value)
			}
		}
		if !pointOnNodeBorder(points[0], asAttrs(g.Node(edge.V))) {
			t.Fatalf("edge %#v does not start on its source: %#v", edge, points[0])
		}
		if !pointOnNodeBorder(points[len(points)-1], asAttrs(g.Node(edge.W))) {
			t.Fatalf("edge %#v does not end on its target: %#v", edge, points[len(points)-1])
		}
	}
}

func TestLayoutPreservesModernRootChildInsertionOrder(t *testing.T) {
	input := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{
		"edgesep": float64(12), "nodesep": float64(45), "rankdir": "BT", "ranksep": float64(190),
	})
	input.SetNode("cluster0", Attrs{"width": float64(79), "height": float64(87)})
	input.SetNode("cluster1", Attrs{"width": float64(97), "height": float64(113)})
	input.SetNode("1", Attrs{"width": float64(48), "height": float64(37)})
	input.SetNode("5", Attrs{"width": float64(129), "height": float64(77)})
	input.SetNode("0", Attrs{"width": float64(95), "height": float64(105)})
	input.SetNode("4", Attrs{"width": float64(42), "height": float64(101)})
	for _, pair := range []struct {
		child  string
		parent string
	}{
		{child: "1", parent: "cluster0"},
		{child: "5", parent: "cluster1"},
		{child: "0", parent: "cluster1"},
		{child: "4", parent: "cluster0"},
	} {
		if err := input.SetParent(pair.child, pair.parent); err != nil {
			t.Fatal(err)
		}
	}
	input.SetEdge("0", "4", Attrs{"width": float64(5), "height": float64(24), "labelpos": "c"}, "0-to-4")
	input.SetEdge("1", "5", Attrs{"width": float64(48), "height": float64(13), "labelpos": "c"}, "1-to-5")

	layoutGraph := buildLayoutGraph(input)
	if got, want := layoutGraph.Children(), []string{"cluster1", "cluster0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want JavaScript insertion order %v", got, want)
	}
	layoutMustSucceed(t, input)
	assertNear(t, num(asAttrs(input.Graph()), "width"), 303)
	assertNear(t, num(asAttrs(input.Graph()), "height"), 610)
	assertNear(t, num(asAttrs(input.Node("0")), "x"), 93)
	assertNear(t, num(asAttrs(input.Node("cluster0")), "x"), 250.5)
}

func TestLayoutUpstreamCaseInsensitiveAttributeNames(t *testing.T) {
	t.Run("graph", func(t *testing.T) {
		g := newLayoutTestGraph()
		asAttrs(g.Graph())["nodeSep"] = 200.0
		g.SetNode("a", Attrs{"width": 50.0, "height": 100.0})
		g.SetNode("b", Attrs{"width": 75.0, "height": 200.0})
		layoutMustSucceed(t, g)

		assertLayoutPoint(t, asAttrs(g.Node("a")), 25, 100)
		assertLayoutPoint(t, asAttrs(g.Node("b")), 287.5, 100)
	})

	t.Run("node and edge", func(t *testing.T) {
		g := newLayoutTestGraph()
		asAttrs(g.Graph())["RankSep"] = 300.0
		g.SetNode("a", Attrs{"Width": 50.0, "HEIGHT": 100.0})
		g.SetNode("b", Attrs{"WIDTH": 75.0, "Height": 200.0})
		g.SetEdge("a", "b", Attrs{"Width": 60.0, "HEIGHT": 70.0, "LabelPos": "c"})
		layoutMustSucceed(t, g)

		assertLayoutPoint(t, asAttrs(g.Node("a")), 37.5, 50)
		assertLayoutPoint(t, asAttrs(g.Node("b")), 37.5, 570)
		assertLayoutPoint(t, asAttrs(g.EdgeByArgs("a", "b")), 37.5, 285)
	})
}

func TestTranslateGraphDefaultsMissingMarginsToZero(t *testing.T) {
	g := NewGraph().SetGraph(Attrs{}).
		SetNode("a", Attrs{"x": float64(5), "y": float64(5), "width": float64(10), "height": float64(10)})
	translateGraph(g)
	graph := asAttrs(g.Graph())
	assertNear(t, num(graph, "width"), 10)
	assertNear(t, num(graph, "height"), 10)
	assertLayoutPoint(t, asAttrs(g.Node("a")), 5, 5)
}

func TestInjectEdgeLabelProxiesUsesJavaScriptTruthiness(t *testing.T) {
	g := NewGraph().
		SetNode("a", Attrs{"rank": float64(0)}).
		SetNode("b", Attrs{"rank": float64(2)}).
		SetEdge("a", "b", Attrs{"width": math.NaN(), "height": float64(10)})
	injectEdgeLabelProxies(g)
	if got := g.NodeCount(); got != 2 {
		t.Fatalf("node count = %d, want 2; NaN is falsy in JavaScript", got)
	}
}

func layoutMustSucceed(t *testing.T, g *Graph) {
	t.Helper()
	if err := Layout(g); err != nil {
		t.Fatal(err)
	}
}

func assertLayoutPoint(t *testing.T, attrs Attrs, wantX, wantY float64) {
	t.Helper()
	assertNear(t, num(attrs, "x"), wantX)
	assertNear(t, num(attrs, "y"), wantY)
}

func layoutEdgePoints(t *testing.T, edge Attrs) []Point {
	t.Helper()
	points, ok := edge["points"].([]Point)
	if !ok {
		t.Fatalf("edge points have type %T, want []Point: %#v", edge["points"], edge)
	}
	return points
}

func pointOnNodeBorder(point Point, node Attrs) bool {
	const epsilon = 1e-9
	dx := math.Abs(point.X - num(node, "x"))
	dy := math.Abs(point.Y - num(node, "y"))
	halfWidth := num(node, "width") / 2
	halfHeight := num(node, "height") / 2
	return math.Abs(dx-halfWidth) <= epsilon && dy <= halfHeight+epsilon ||
		math.Abs(dy-halfHeight) <= epsilon && dx <= halfWidth+epsilon
}

func assertLayoutPoints(t *testing.T, got, want []Point) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("points length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if math.Abs(got[i].X-want[i].X) > 1e-9 || math.Abs(got[i].Y-want[i].Y) > 1e-9 {
			t.Errorf("point %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

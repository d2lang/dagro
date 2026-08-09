package dagro

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// LayoutOptions controls optional diagnostic behavior.
type LayoutOptions struct {
	DebugTiming bool
}

// Layout runs the Dagre 0.8.5 layout algorithm and writes coordinates and
// routes back to g. The input graph's topology is not modified.
func Layout(g *Graph, options ...LayoutOptions) error {
	opts := LayoutOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	run := func(name string, fn func() error) error {
		if !opts.DebugTiming {
			return fn()
		}
		start := time.Now()
		err := fn()
		fmt.Printf("%s time: %dms\n", name, time.Since(start).Milliseconds())
		return err
	}
	var layoutGraph *Graph
	if err := run("  buildLayoutGraph", func() error {
		layoutGraph = buildLayoutGraph(g)
		return nil
	}); err != nil {
		return err
	}
	if err := run("  runLayout", func() error { return runLayout(layoutGraph) }); err != nil {
		return err
	}
	return run("  updateInputGraph", func() error {
		updateInputGraph(g, layoutGraph)
		return nil
	})
}

func runLayout(g *Graph) error {
	makeSpaceForEdgeLabels(g)
	removeSelfEdges(g)
	runAcyclic(g)
	runNestingGraph(g)
	rank(asNonCompoundGraph(g))
	injectEdgeLabelProxies(g)
	removeEmptyRanks(g)
	cleanupNestingGraph(g)
	normalizeRanks(g)
	assignRankMinMax(g)
	removeEdgeLabelProxies(g)
	runNormalize(g)
	parentDummyChains(g)
	addBorderSegments(g)
	order(g)
	insertSelfEdges(g)
	adjustCoordinateSystem(g)
	position(g)
	positionSelfEdges(g)
	removeBorderNodes(g)
	undoNormalize(g)
	fixupEdgeLabelCoords(g)
	undoCoordinateSystem(g)
	translateGraph(g)
	if err := assignNodeIntersects(g); err != nil {
		return err
	}
	reversePointsForReversedEdges(g)
	undoAcyclic(g)
	return nil
}

func updateInputGraph(inputGraph, layoutGraph *Graph) {
	for _, v := range inputGraph.Nodes() {
		inputLabel, ok := inputGraph.Node(v).(Attrs)
		if !ok || inputLabel == nil {
			continue
		}
		layoutLabel := asAttrs(layoutGraph.Node(v))
		inputLabel["x"], inputLabel["y"] = num(layoutLabel, "x"), num(layoutLabel, "y")
		if len(layoutGraph.Children(v)) > 0 {
			inputLabel["width"], inputLabel["height"] = num(layoutLabel, "width"), num(layoutLabel, "height")
		}
	}
	for _, e := range inputGraph.Edges() {
		inputLabel := asAttrs(inputGraph.Edge(e))
		layoutLabel := asAttrs(layoutGraph.Edge(e))
		inputLabel["points"] = append([]Point(nil), layoutLabel["points"].([]Point)...)
		if has(layoutLabel, "x") {
			inputLabel["x"], inputLabel["y"] = num(layoutLabel, "x"), num(layoutLabel, "y")
		}
	}
	inputAttrs, layoutAttrs := asAttrs(inputGraph.Graph()), asAttrs(layoutGraph.Graph())
	inputAttrs["width"], inputAttrs["height"] = num(layoutAttrs, "width"), num(layoutAttrs, "height")
}

var graphNumAttrs = []string{"nodesep", "edgesep", "ranksep", "marginx", "marginy"}
var graphAttrs = []string{"acyclicer", "ranker", "rankdir", "align"}
var nodeNumAttrs = []string{"width", "height"}
var edgeNumAttrs = []string{"minlen", "weight", "width", "height", "labeloffset"}
var edgeAttrs = []string{"labelpos"}

func buildLayoutGraph(inputGraph *Graph) *Graph {
	g := NewGraph(GraphOptions{Multigraph: true, Compound: true})
	graph := canonicalize(inputGraph.Graph())
	graphLabel := Attrs{"ranksep": float64(50), "edgesep": float64(20), "nodesep": float64(50), "rankdir": "tb"}
	mergeNumberAttrs(graphLabel, graph, graphNumAttrs)
	mergeAttrs(graphLabel, graph, graphAttrs)
	g.SetGraph(graphLabel)
	for _, v := range inputGraph.Nodes() {
		node := canonicalize(inputGraph.Node(v))
		label := Attrs{}
		mergeNumberAttrs(label, node, nodeNumAttrs)
		if !has(label, "width") {
			label["width"] = float64(0)
		}
		if !has(label, "height") {
			label["height"] = float64(0)
		}
		g.SetNode(v, label)
		if parent, ok := inputGraph.Parent(v); ok {
			_ = g.setParentKnownAcyclic(v, parent)
		} else {
			_ = g.setParentKnownAcyclic(v)
		}
	}
	for _, e := range inputGraph.Edges() {
		edge := canonicalize(inputGraph.Edge(e))
		label := Attrs{
			"minlen": float64(1), "weight": float64(1), "width": float64(0), "height": float64(0),
			"labeloffset": float64(10), "labelpos": "r",
		}
		mergeNumberAttrs(label, edge, edgeNumAttrs)
		mergeAttrs(label, edge, edgeAttrs)
		g.SetEdgeObject(e, label)
	}
	return g
}

func canonicalize(value any) Attrs {
	a, ok := value.(Attrs)
	if !ok || a == nil {
		return Attrs{}
	}
	out := make(Attrs, len(a))
	for k, v := range a {
		out[strings.ToLower(k)] = v
	}
	return out
}

func mergeNumberAttrs(dst, src Attrs, keys []string) {
	for _, k := range keys {
		if v, ok := src[k]; ok {
			dst[k] = number(v)
		}
	}
}

func mergeAttrs(dst, src Attrs, keys []string) {
	for _, k := range keys {
		if v, ok := src[k]; ok {
			dst[k] = v
		}
	}
}

func makeSpaceForEdgeLabels(g *Graph) {
	graph := asAttrs(g.Graph())
	graph["ranksep"] = num(graph, "ranksep") / 2
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		edge["minlen"] = num(edge, "minlen") * 2
		if strings.ToLower(stringValue(edge, "labelpos")) != "c" {
			if stringValue(graph, "rankdir") == "TB" || stringValue(graph, "rankdir") == "BT" {
				edge["width"] = num(edge, "width") + num(edge, "labeloffset")
			} else {
				edge["height"] = num(edge, "height") + num(edge, "labeloffset")
			}
		}
	}
}

func injectEdgeLabelProxies(g *Graph) {
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		if jsTruthyNumber(num(edge, "width")) && jsTruthyNumber(num(edge, "height")) {
			v, w := asAttrs(g.Node(e.V)), asAttrs(g.Node(e.W))
			addDummyNode(g, "edge-proxy", Attrs{
				"rank": (num(w, "rank")-num(v, "rank"))/2 + num(v, "rank"), "e": e,
			}, "_ep")
		}
	}
}

func assignRankMinMax(g *Graph) {
	max := 0.0
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if has(node, "borderTop") {
			node["minRank"] = num(asAttrs(g.Node(stringValue(node, "borderTop"))), "rank")
			node["maxRank"] = num(asAttrs(g.Node(stringValue(node, "borderBottom"))), "rank")
			max = lodashMaxJS(max, num(node, "maxRank"))
		}
	}
	asAttrs(g.Graph())["maxRank"] = max
}

func removeEdgeLabelProxies(g *Graph) {
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if stringValue(node, "dummy") == "edge-proxy" {
			e := node["e"].(Edge)
			asAttrs(g.Edge(e))["labelRank"] = num(node, "rank")
			g.RemoveNode(v)
		}
	}
}

func translateGraph(g *Graph) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := 0.0, 0.0
	graph := asAttrs(g.Graph())
	marginX, marginY := num(graph, "marginx"), num(graph, "marginy")
	if !jsTruthyNumber(marginX) {
		marginX = 0
	}
	if !jsTruthyNumber(marginY) {
		marginY = 0
	}
	getExtremes := func(attrs Attrs) {
		x, y, w, h := num(attrs, "x"), num(attrs, "y"), num(attrs, "width"), num(attrs, "height")
		minX, maxX = mathMinJS(minX, x-w/2), mathMaxJS(maxX, x+w/2)
		minY, maxY = mathMinJS(minY, y-h/2), mathMaxJS(maxY, y+h/2)
	}
	for _, v := range g.Nodes() {
		getExtremes(asAttrs(g.Node(v)))
	}
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		if has(edge, "x") {
			getExtremes(edge)
		}
	}
	minX, minY = minX-marginX, minY-marginY
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		node["x"], node["y"] = num(node, "x")-minX, num(node, "y")-minY
	}
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		points, _ := edge["points"].([]Point)
		for i := range points {
			points[i].X -= minX
			points[i].Y -= minY
		}
		edge["points"] = points
		if has(edge, "x") {
			edge["x"] = num(edge, "x") - minX
		}
		if has(edge, "y") {
			edge["y"] = num(edge, "y") - minY
		}
	}
	graph["width"], graph["height"] = maxX-minX+marginX, maxY-minY+marginY
}

func assignNodeIntersects(g *Graph) error {
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		nodeV, nodeW := asAttrs(g.Node(e.V)), asAttrs(g.Node(e.W))
		points, exists := edge["points"].([]Point)
		var p1, p2 Point
		if !exists {
			points = []Point{}
			p1 = Point{X: num(nodeW, "x"), Y: num(nodeW, "y")}
			p2 = Point{X: num(nodeV, "x"), Y: num(nodeV, "y")}
		} else {
			p1, p2 = points[0], points[len(points)-1]
		}
		start, err := intersectRect(nodeV, p1)
		if err != nil {
			return err
		}
		end, err := intersectRect(nodeW, p2)
		if err != nil {
			return err
		}
		points = append([]Point{start}, points...)
		points = append(points, end)
		edge["points"] = points
	}
	return nil
}

func fixupEdgeLabelCoords(g *Graph) {
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		if !has(edge, "x") {
			continue
		}
		pos := stringValue(edge, "labelpos")
		if pos == "l" || pos == "r" {
			edge["width"] = num(edge, "width") - num(edge, "labeloffset")
		}
		if pos == "l" {
			edge["x"] = num(edge, "x") - num(edge, "width")/2 - num(edge, "labeloffset")
		} else if pos == "r" {
			edge["x"] = num(edge, "x") + num(edge, "width")/2 + num(edge, "labeloffset")
		}
	}
}

func reversePointsForReversedEdges(g *Graph) {
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		if !boolValue(edge, "reversed") {
			continue
		}
		points := edge["points"].([]Point)
		for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
			points[i], points[j] = points[j], points[i]
		}
	}
}

func removeBorderNodes(g *Graph) {
	for _, v := range g.Nodes() {
		if len(g.Children(v)) == 0 {
			continue
		}
		node := asAttrs(g.Node(v))
		t := asAttrs(g.Node(stringValue(node, "borderTop")))
		b := asAttrs(g.Node(stringValue(node, "borderBottom")))
		left, right := node["borderLeft"].(map[int]string), node["borderRight"].(map[int]string)
		maxLeft, maxRight := math.MinInt, math.MinInt
		for rank := range left {
			if rank > maxLeft {
				maxLeft = rank
			}
		}
		for rank := range right {
			if rank > maxRight {
				maxRight = rank
			}
		}
		l, r := asAttrs(g.Node(left[maxLeft])), asAttrs(g.Node(right[maxRight]))
		node["width"] = math.Abs(num(r, "x") - num(l, "x"))
		node["height"] = math.Abs(num(b, "y") - num(t, "y"))
		node["x"] = num(l, "x") + num(node, "width")/2
		node["y"] = num(t, "y") + num(node, "height")/2
	}
	for _, v := range g.Nodes() {
		if stringValue(asAttrs(g.Node(v)), "dummy") == "border" {
			g.RemoveNode(v)
		}
	}
}

type selfEdgeRecord struct {
	e     Edge
	label Attrs
}

func removeSelfEdges(g *Graph) {
	for _, e := range g.Edges() {
		if e.V != e.W {
			continue
		}
		node := asAttrs(g.Node(e.V))
		selfEdges, _ := node["selfEdges"].([]selfEdgeRecord)
		node["selfEdges"] = append(selfEdges, selfEdgeRecord{e: e, label: asAttrs(g.Edge(e))})
		g.RemoveEdge(e)
	}
}

func insertSelfEdges(g *Graph) {
	for _, layer := range buildLayerMatrix(g) {
		orderShift := 0
		for i, v := range layer {
			node := asAttrs(g.Node(v))
			node["order"] = float64(i + orderShift)
			selfEdges, _ := node["selfEdges"].([]selfEdgeRecord)
			for _, selfEdge := range selfEdges {
				orderShift++
				addDummyNode(g, "selfedge", Attrs{
					"width": num(selfEdge.label, "width"), "height": num(selfEdge.label, "height"),
					"rank": num(node, "rank"), "order": float64(i + orderShift),
					"e": selfEdge.e, "label": selfEdge.label,
				}, "_se")
			}
			delete(node, "selfEdges")
		}
	}
}

func positionSelfEdges(g *Graph) {
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if stringValue(node, "dummy") != "selfedge" {
			continue
		}
		e := node["e"].(Edge)
		label := node["label"].(Attrs)
		selfNode := asAttrs(g.Node(e.V))
		x, y := num(selfNode, "x")+num(selfNode, "width")/2, num(selfNode, "y")
		dx, dy := num(node, "x")-x, num(selfNode, "height")/2
		g.SetEdgeObject(e, label)
		g.RemoveNode(v)
		label["points"] = []Point{
			{X: x + 2*dx/3, Y: y - dy},
			{X: x + 5*dx/6, Y: y - dy},
			{X: x + dx, Y: y},
			{X: x + 5*dx/6, Y: y + dy},
			{X: x + 2*dx/3, Y: y + dy},
		}
		label["x"], label["y"] = num(node, "x"), num(node, "y")
	}
}

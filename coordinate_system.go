package dagro

import "strings"

func adjustCoordinateSystem(g *Graph) {
	rankDir := strings.ToLower(stringValue(asAttrs(g.Graph()), "rankdir"))
	if rankDir == "lr" || rankDir == "rl" {
		swapWidthHeight(g)
	}
}

func undoCoordinateSystem(g *Graph) {
	rankDir := strings.ToLower(stringValue(asAttrs(g.Graph()), "rankdir"))
	if rankDir == "bt" || rankDir == "rl" {
		reverseY(g)
	}
	if rankDir == "lr" || rankDir == "rl" {
		swapXY(g)
		swapWidthHeight(g)
	}
}

func swapWidthHeight(g *Graph) {
	for _, v := range g.Nodes() {
		swapWidthHeightOne(asAttrs(g.Node(v)))
	}
	for _, e := range g.Edges() {
		swapWidthHeightOne(asAttrs(g.Edge(e)))
	}
}

func swapWidthHeightOne(attrs Attrs) {
	attrs["width"], attrs["height"] = attrs["height"], attrs["width"]
}

func reverseY(g *Graph) {
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		node["y"] = -num(node, "y")
	}
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		points, _ := edge["points"].([]Point)
		for i := range points {
			points[i].Y = -points[i].Y
		}
		edge["points"] = points
		if has(edge, "y") {
			edge["y"] = -num(edge, "y")
		}
	}
}

func swapXY(g *Graph) {
	for _, v := range g.Nodes() {
		swapXYOne(asAttrs(g.Node(v)))
	}
	for _, e := range g.Edges() {
		edge := asAttrs(g.Edge(e))
		points, _ := edge["points"].([]Point)
		for i := range points {
			points[i].X, points[i].Y = points[i].Y, points[i].X
		}
		edge["points"] = points
		if has(edge, "x") {
			edge["x"], edge["y"] = edge["y"], edge["x"]
		}
	}
}

func swapXYOne(attrs Attrs) { attrs["x"], attrs["y"] = attrs["y"], attrs["x"] }

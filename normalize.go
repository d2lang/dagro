package dagro

func runNormalize(g *Graph) {
	graphAttrs := asAttrs(g.Graph())
	graphAttrs["dummyChains"] = []string{}
	for _, e := range g.Edges() {
		normalizeEdge(g, e)
	}
}

func normalizeEdge(g *Graph, e Edge) {
	v, w := e.V, e.W
	vRank, wRank := num(asAttrs(g.Node(v)), "rank"), num(asAttrs(g.Node(w)), "rank")
	edgeLabel := asAttrs(g.Edge(e))
	labelRank := num(edgeLabel, "labelRank")
	if wRank == vRank+1 {
		return
	}
	g.RemoveEdge(e)
	for i := 0; vRank+1 < wRank; i++ {
		vRank++
		edgeLabel["points"] = []Point{}
		attrs := Attrs{
			"width": float64(0), "height": float64(0),
			"edgeLabel": edgeLabel, "edgeObj": e, "rank": vRank,
		}
		dummy := addDummyNode(g, "edge", attrs, "_d")
		if vRank == labelRank {
			attrs["width"], attrs["height"] = num(edgeLabel, "width"), num(edgeLabel, "height")
			attrs["dummy"] = "edge-label"
			attrs["labelpos"] = edgeLabel["labelpos"]
		}
		setEdgePreservingName(g, v, dummy, Attrs{"weight": num(edgeLabel, "weight")}, e)
		if i == 0 {
			chains := asAttrs(g.Graph())["dummyChains"].([]string)
			asAttrs(g.Graph())["dummyChains"] = append(chains, dummy)
		}
		v = dummy
	}
	setEdgePreservingName(g, v, w, Attrs{"weight": num(edgeLabel, "weight")}, e)
}

func undoNormalize(g *Graph) {
	chains, _ := asAttrs(g.Graph())["dummyChains"].([]string)
	for _, start := range chains {
		v := start
		node := asAttrs(g.Node(v))
		origLabel := node["edgeLabel"].(Attrs)
		origEdge := node["edgeObj"].(Edge)
		g.SetEdgeObject(origEdge, origLabel)
		for stringValue(node, "dummy") != "" {
			w := g.Successors(v)[0]
			g.RemoveNode(v)
			points, _ := origLabel["points"].([]Point)
			origLabel["points"] = append(points, Point{X: num(node, "x"), Y: num(node, "y")})
			if stringValue(node, "dummy") == "edge-label" {
				origLabel["x"], origLabel["y"] = num(node, "x"), num(node, "y")
				origLabel["width"], origLabel["height"] = num(node, "width"), num(node, "height")
			}
			v = w
			node = asAttrs(g.Node(v))
		}
	}
}

package dagro

func addBorderSegments(g *Graph) {
	var dfs func(string)
	dfs = func(v string) {
		children := g.Children(v)
		node := asAttrs(g.Node(v))
		for _, child := range children {
			dfs(child)
		}
		if !has(node, "minRank") {
			return
		}
		node["borderLeft"] = map[int]string{}
		node["borderRight"] = map[int]string{}
		for rank, max := integer(node, "minRank"), integer(node, "maxRank")+1; rank < max; rank++ {
			addSubgraphBorderNode(g, "borderLeft", "_bl", v, node, rank)
			addSubgraphBorderNode(g, "borderRight", "_br", v, node, rank)
		}
	}
	for _, v := range g.Children() {
		dfs(v)
	}
}

func addSubgraphBorderNode(g *Graph, prop, prefix, subgraph string, subgraphNode Attrs, rank int) {
	label := Attrs{
		"width": float64(0), "height": float64(0), "rank": float64(rank), "borderType": prop,
	}
	borders := subgraphNode[prop].(map[int]string)
	prev := borders[rank-1]
	curr := addDummyNode(g, "border", label, prefix)
	borders[rank] = curr
	_ = g.setParentKnownAcyclic(curr, subgraph)
	if prev != "" {
		g.SetEdge(prev, curr, Attrs{"weight": float64(1)})
	}
}

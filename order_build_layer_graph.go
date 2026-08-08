package dagro

func buildLayerGraph(g *Graph, rank int, relationship string) *Graph {
	root := createOrderRootNode(g)
	result := NewGraph(GraphOptions{Compound: true}).SetGraph(Attrs{"root": root})
	result.SetDefaultNodeLabel(func(v string) any { return g.Node(v) })

	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		onRank := has(node, "rank") && integer(node, "rank") == rank
		spansRank := has(node, "minRank") && has(node, "maxRank") &&
			integer(node, "minRank") <= rank && rank <= integer(node, "maxRank")
		if !onRank && !spansRank {
			continue
		}

		result.SetNode(v)
		parent, ok := g.Parent(v)
		if !ok || parent == "" {
			parent = root
		}
		if err := result.SetParent(v, parent); err != nil {
			panic(err)
		}

		var incident []Edge
		switch relationship {
		case "inEdges":
			incident = g.InEdges(v)
		case "outEdges":
			incident = g.OutEdges(v)
		default:
			panic("dagro: unsupported layer-graph relationship: " + relationship)
		}
		for _, e := range incident {
			u := e.V
			if u == v {
				u = e.W
			}
			weight := 0.0
			if result.HasEdge(u, v) {
				weight = num(asAttrs(result.EdgeByArgs(u, v)), "weight")
			}
			weight += num(asAttrs(g.Edge(e)), "weight")
			result.SetEdge(u, v, Attrs{"weight": weight})
		}

		if has(node, "minRank") {
			result.SetNode(v, Attrs{
				"borderLeft":  orderBorderAt(node["borderLeft"], rank),
				"borderRight": orderBorderAt(node["borderRight"], rank),
			})
		}
	}

	return result
}

func createOrderRootNode(g *Graph) string {
	for {
		v := g.uniqueID("_root")
		if !g.HasNode(v) {
			return v
		}
	}
}

func orderBorderAt(value any, rank int) string {
	switch borders := value.(type) {
	case []string:
		if rank >= 0 && rank < len(borders) {
			return borders[rank]
		}
	case []any:
		if rank >= 0 && rank < len(borders) {
			if border, ok := borders[rank].(string); ok {
				return border
			}
		}
	case map[int]string:
		return borders[rank]
	}
	return ""
}

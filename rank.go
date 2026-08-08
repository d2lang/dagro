package dagro

// rank selects one of Dagre 0.8.5's three rankers. Unknown and absent values
// intentionally fall back to network simplex.
func rank(g *Graph) {
	ranker := ""
	if graphLabel := asAttrs(g.Graph()); graphLabel != nil {
		ranker = stringValue(graphLabel, "ranker")
	}

	switch ranker {
	case "longest-path":
		longestPath(g)
	case "tight-tree":
		longestPath(g)
		feasibleTree(g)
	case "network-simplex":
		networkSimplex(g)
	default:
		networkSimplex(g)
	}
}

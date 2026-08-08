package dagro

// feasibleTree constructs a spanning tree of tight edges, shifting ranks until
// every node can be added. It preserves graphlib edge order for all ties.
func feasibleTree(g *Graph) *Graph {
	t := NewGraph(GraphOptions{Undirected: true})
	start := g.Nodes()[0]
	size := g.NodeCount()
	t.SetNode(start, Attrs{})

	for tightTree(t, g) < size {
		edge, ok := findMinSlackEdge(t, g)
		if !ok {
			panic("dagro: feasibleTree could not find an incident edge")
		}
		delta := slack(g, edge)
		if !t.HasNode(edge.V) {
			delta = -delta
		}
		shiftRanks(t, g, delta)
	}

	return t
}

// tightTree grows t along zero-slack edges and returns its node count.
func tightTree(t, g *Graph) int {
	var dfs func(string)
	dfs = func(v string) {
		for _, e := range g.NodeEdges(v) {
			w := e.V
			if v == e.V {
				w = e.W
			}
			if !t.HasNode(w) && !jsTruthyNumber(slack(g, e)) {
				t.SetNode(w, Attrs{})
				t.SetEdge(v, w, Attrs{})
				dfs(w)
			}
		}
	}

	for _, v := range t.Nodes() {
		dfs(v)
	}
	return t.NodeCount()
}

// findMinSlackEdge returns the first minimum-slack edge incident on t. Lodash
// minBy is stable for ties, so replacement is deliberately strict.
func findMinSlackEdge(t, g *Graph) (Edge, bool) {
	var best Edge
	bestSlack := 0.0
	found := false
	for _, e := range g.Edges() {
		if t.HasNode(e.V) == t.HasNode(e.W) {
			continue
		}
		s := slack(g, e)
		if !found || s < bestSlack {
			best, bestSlack, found = e, s, true
		}
	}
	return best, found
}

func shiftRanks(t, g *Graph, delta float64) {
	for _, v := range t.Nodes() {
		label := asAttrs(g.Node(v))
		label["rank"] = num(label, "rank") + delta
	}
}

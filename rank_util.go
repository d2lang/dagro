package dagro

import "math"

// longestPath initializes ranks by pushing every node as low as its outgoing
// minlen constraints permit. This is a direct port of lib/rank/util.js.
func longestPath(g *Graph) {
	visited := make(map[string]bool, g.NodeCount())

	var dfs func(string) float64
	dfs = func(v string) float64 {
		label := asAttrs(g.Node(v))
		if visited[v] {
			return num(label, "rank")
		}
		visited[v] = true

		rank := math.Inf(1)
		for _, e := range g.OutEdges(v) {
			candidate := dfs(e.W) - num(asAttrs(g.Edge(e)), "minlen")
			if candidate < rank {
				rank = candidate
			}
		}
		if math.IsInf(rank, 1) {
			rank = 0
		}

		label["rank"] = rank
		return rank
	}

	for _, v := range g.Sources() {
		dfs(v)
	}
}

// slack is the difference between an edge's current length and minlen.
func slack(g *Graph, e Edge) float64 {
	vRank := num(asAttrs(g.Node(e.V)), "rank")
	wRank := num(asAttrs(g.Node(e.W)), "rank")
	return wRank - vRank - num(asAttrs(g.Edge(e)), "minlen")
}

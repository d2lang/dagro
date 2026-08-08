package dagro

import gosort "sort"

func initOrder(g *Graph) [][]string {
	visited := map[string]bool{}
	simpleNodes := make([]string, 0, g.NodeCount())
	for _, v := range g.Nodes() {
		if len(g.Children(v)) == 0 {
			simpleNodes = append(simpleNodes, v)
		}
	}
	if len(simpleNodes) == 0 {
		return nil
	}

	maxRank := -1
	for _, v := range simpleNodes {
		rank := integer(asAttrs(g.Node(v)), "rank")
		if rank > maxRank {
			maxRank = rank
		}
	}
	layers := make([][]string, maxRank+1)

	var dfs func(string)
	dfs = func(v string) {
		if visited[v] {
			return
		}
		visited[v] = true
		node := asAttrs(g.Node(v))
		rank := integer(node, "rank")
		layers[rank] = append(layers[rank], v)
		for _, successor := range g.Successors(v) {
			dfs(successor)
		}
	}

	orderedVs := append([]string(nil), simpleNodes...)
	gosort.SliceStable(orderedVs, func(i, j int) bool {
		return num(asAttrs(g.Node(orderedVs[i])), "rank") < num(asAttrs(g.Node(orderedVs[j])), "rank")
	})
	for _, v := range orderedVs {
		dfs(v)
	}

	return layers
}

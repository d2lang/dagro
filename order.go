package dagro

import "math"

// orderEntry is the Go representation of the small JavaScript records passed
// between Dagre's barycenter, conflict-resolution, and sorting phases.
// HasBarycenter preserves the difference between an absent barycenter and 0.
type orderEntry struct {
	V             string
	VS            []string
	I             int
	Barycenter    float64
	Weight        float64
	HasBarycenter bool
}

type orderResult struct {
	VS            []string
	Barycenter    float64
	Weight        float64
	HasBarycenter bool
}

// order applies Dagre 0.8.5's four-way sweeping heuristic and writes the best
// order found back to the node labels.
func order(g *Graph) {
	maxRank := orderMaxRank(g)
	downRanks := make([]int, 0, maxRank)
	for rank := 1; rank <= maxRank; rank++ {
		downRanks = append(downRanks, rank)
	}
	upRanks := make([]int, 0, maxRank)
	for rank := maxRank - 1; rank >= 0; rank-- {
		upRanks = append(upRanks, rank)
	}

	downLayerGraphs := buildLayerGraphs(g, downRanks, "inEdges")
	upLayerGraphs := buildLayerGraphs(g, upRanks, "outEdges")

	layering := initOrder(g)
	assignOrder(g, layering)

	bestCC := math.Inf(1)
	var best [][]string
	for i, lastBest := 0, 0; lastBest < 4; i, lastBest = i+1, lastBest+1 {
		if i%2 != 0 {
			sweepLayerGraphs(downLayerGraphs, i%4 >= 2)
		} else {
			sweepLayerGraphs(upLayerGraphs, i%4 >= 2)
		}

		layering = orderBuildLayerMatrix(g)
		cc := crossCount(g, layering)
		if cc < bestCC {
			lastBest = 0
			best = cloneLayering(layering)
			bestCC = cc
		}
	}

	assignOrder(g, best)
}

func buildLayerGraphs(g *Graph, ranks []int, relationship string) []*Graph {
	result := make([]*Graph, 0, len(ranks))
	for _, rank := range ranks {
		result = append(result, buildLayerGraph(g, rank, relationship))
	}
	return result
}

func sweepLayerGraphs(layerGraphs []*Graph, biasRight bool) {
	cg := NewGraph()
	for _, lg := range layerGraphs {
		root := stringValue(asAttrs(lg.Graph()), "root")
		sorted := sortSubgraph(lg, root, cg, biasRight)
		for i, v := range sorted.VS {
			asAttrs(lg.Node(v))["order"] = float64(i)
		}
		addSubgraphConstraints(lg, cg, sorted.VS)
	}
}

func assignOrder(g *Graph, layering [][]string) {
	for _, layer := range layering {
		for i, v := range layer {
			asAttrs(g.Node(v))["order"] = float64(i)
		}
	}
}

func orderMaxRank(g *Graph) int {
	maxRank := -1
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if has(node, "rank") {
			rank := integer(node, "rank")
			if rank > maxRank {
				maxRank = rank
			}
		}
	}
	return maxRank
}

func orderBuildLayerMatrix(g *Graph) [][]string {
	maxRank := orderMaxRank(g)
	if maxRank < 0 {
		return nil
	}
	layering := make([][]string, maxRank+1)
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if !has(node, "rank") || !has(node, "order") {
			continue
		}
		rank := integer(node, "rank")
		position := integer(node, "order")
		if rank < 0 || rank >= len(layering) || position < 0 {
			continue
		}
		if position >= len(layering[rank]) {
			grown := make([]string, position+1)
			copy(grown, layering[rank])
			layering[rank] = grown
		}
		layering[rank][position] = v
	}
	return layering
}

func cloneLayering(layering [][]string) [][]string {
	result := make([][]string, len(layering))
	for i := range layering {
		result[i] = append([]string(nil), layering[i]...)
	}
	return result
}

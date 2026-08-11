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
	UsedBias      bool
}

// order applies Dagre 0.8.5's four-way sweeping heuristic and writes the best
// order found back to the node labels.
func order(g *Graph) {
	// The topology does not change during ordering. Capture JavaScript node
	// enumeration once, then use it for rank bucketing and layer snapshots.
	nodes := g.Nodes()
	maxRank := orderMaxRankNodes(g, nodes)
	downRanks := make([]int, 0, maxRank)
	for rank := 1; rank <= maxRank; rank++ {
		downRanks = append(downRanks, rank)
	}
	upRanks := make([]int, 0, maxRank)
	for rank := maxRank - 1; rank >= 0; rank-- {
		upRanks = append(upRanks, rank)
	}

	nodesByRank := buildLayerNodeBuckets(g, nodes, maxRank)
	downLayerGraphs := buildLayerGraphsFromBuckets(g, downRanks, "inEdges", nodesByRank)
	upLayerGraphs := buildLayerGraphsFromBuckets(g, upRanks, "outEdges", nodesByRank)

	layering := initOrderNodes(g, nodes)
	assignOrder(g, layering)

	bestCC := math.Inf(1)
	var best [][]string
	for i, lastBest := 0, 0; lastBest < 4; i, lastBest = i+1, lastBest+1 {
		if i%2 != 0 {
			sweepLayerGraphs(downLayerGraphs, i%4 >= 2)
		} else {
			sweepLayerGraphs(upLayerGraphs, i%4 >= 2)
		}

		layering = orderBuildLayerMatrixNodes(g, nodes, maxRank)
		cc := crossCount(g, layering)
		if cc < bestCC {
			lastBest = 0
			best = cloneLayering(layering)
			bestCC = cc
		} else if cc == bestCC {
			// Modern Dagre intentionally keeps the last equally good
			// ordering found by the sweep heuristic.
			best = cloneLayering(layering)
		}
	}

	assignOrder(g, best)
}

func buildLayerGraphs(g *Graph, ranks []int, relationship string) []*Graph {
	maxRank := -1
	for _, rank := range ranks {
		if rank > maxRank {
			maxRank = rank
		}
	}
	nodes := g.Nodes()
	nodesByRank := buildLayerNodeBuckets(g, nodes, maxRank)
	return buildLayerGraphsFromBuckets(g, ranks, relationship, nodesByRank)
}

func buildLayerGraphsFromBuckets(g *Graph, ranks []int, relationship string, nodesByRank [][]string) []*Graph {
	result := make([]*Graph, 0, len(ranks))
	for _, rank := range ranks {
		var nodes []string
		if 0 <= rank && rank < len(nodesByRank) {
			nodes = nodesByRank[rank]
		}
		result = append(result, buildLayerGraphNodes(g, rank, relationship, nodes))
	}
	return result
}

// buildLayerNodeBuckets performs the same global JavaScript-order filtering as
// buildLayerGraph did rank by rank. Nodes are the outer loop, so every bucket
// retains the exact order of g.nodes(). Compound nodes are added to each rank
// in their inclusive minRank/maxRank interval.
func buildLayerNodeBuckets(g *Graph, nodes []string, maxRank int) [][]string {
	if maxRank < 0 {
		return nil
	}
	buckets := make([][]string, maxRank+1)
	for _, v := range nodes {
		node := asAttrs(g.Node(v))
		nodeRank, hasRank := 0, has(node, "rank")
		if hasRank {
			nodeRank = integer(node, "rank")
			if 0 <= nodeRank && nodeRank <= maxRank {
				buckets[nodeRank] = append(buckets[nodeRank], v)
			}
		}
		if !has(node, "minRank") || !has(node, "maxRank") {
			continue
		}
		minRank, maxNodeRank := integer(node, "minRank"), integer(node, "maxRank")
		if minRank < 0 {
			minRank = 0
		}
		if maxNodeRank > maxRank {
			maxNodeRank = maxRank
		}
		for rank := minRank; rank <= maxNodeRank; rank++ {
			if hasRank && rank == nodeRank {
				continue
			}
			buckets[rank] = append(buckets[rank], v)
		}
	}
	return buckets
}

func sweepLayerGraphs(layerGraphs []*Graph, switchBias bool) {
	biasRight := true
	cg := NewGraph()
	for _, lg := range layerGraphs {
		root := stringValue(asAttrs(lg.Graph()), "root")
		sorted := sortSubgraph(lg, root, cg, biasRight)
		if switchBias && sorted.UsedBias {
			biasRight = !biasRight
		}
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
	return orderMaxRankNodes(g, g.Nodes())
}

func orderMaxRankNodes(g *Graph, nodes []string) int {
	maxRank := -1
	for _, v := range nodes {
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
	nodes := g.Nodes()
	return orderBuildLayerMatrixNodes(g, nodes, orderMaxRankNodes(g, nodes))
}

func orderBuildLayerMatrixNodes(g *Graph, nodes []string, maxRank int) [][]string {
	if maxRank < 0 {
		return nil
	}
	layering := make([][]string, maxRank+1)
	layerSizes := make([]int, maxRank+1)
	for _, v := range nodes {
		node := asAttrs(g.Node(v))
		if !has(node, "rank") || !has(node, "order") {
			continue
		}
		rank := integer(node, "rank")
		position := integer(node, "order")
		if rank < 0 || rank >= len(layering) || position < 0 {
			continue
		}
		if position >= layerSizes[rank] {
			layerSizes[rank] = position + 1
		}
	}
	for rank, size := range layerSizes {
		layering[rank] = make([]string, size)
	}
	for _, v := range nodes {
		node := asAttrs(g.Node(v))
		if !has(node, "rank") || !has(node, "order") {
			continue
		}
		rank := integer(node, "rank")
		position := integer(node, "order")
		if rank < 0 || rank >= len(layering) || position < 0 {
			continue
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

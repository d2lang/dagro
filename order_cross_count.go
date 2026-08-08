package dagro

import gosort "sort"

func crossCount(g *Graph, layering [][]string) float64 {
	cc := 0.0
	for i := 1; i < len(layering); i++ {
		cc += twoLayerCrossCount(g, layering[i-1], layering[i])
	}
	return cc
}

type southEntry struct {
	pos    int
	weight float64
}

func twoLayerCrossCount(g *Graph, northLayer, southLayer []string) float64 {
	southPos := make(map[string]int, len(southLayer))
	for i, v := range southLayer {
		southPos[v] = i
	}

	southEntries := make([]southEntry, 0)
	for _, v := range northLayer {
		entries := make([]southEntry, 0, len(g.OutEdges(v)))
		for _, e := range g.OutEdges(v) {
			entries = append(entries, southEntry{
				pos:    southPos[e.W],
				weight: num(asAttrs(g.Edge(e)), "weight"),
			})
		}
		gosort.SliceStable(entries, func(i, j int) bool { return entries[i].pos < entries[j].pos })
		southEntries = append(southEntries, entries...)
	}

	firstIndex := 1
	for firstIndex < len(southLayer) {
		firstIndex <<= 1
	}
	treeSize := 2*firstIndex - 1
	firstIndex--
	tree := make([]float64, treeSize)

	cc := 0.0
	for _, entry := range southEntries {
		index := entry.pos + firstIndex
		tree[index] += entry.weight
		weightSum := 0.0
		for index > 0 {
			if index%2 != 0 {
				weightSum += tree[index+1]
			}
			index = (index - 1) >> 1
			tree[index] += entry.weight
		}
		cc += entry.weight * weightSum
	}

	return cc
}

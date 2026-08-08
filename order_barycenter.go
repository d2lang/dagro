package dagro

func barycenter(g *Graph, movable []string) []orderEntry {
	result := make([]orderEntry, 0, len(movable))
	for _, v := range movable {
		in := g.InEdges(v)
		entry := orderEntry{V: v}
		if len(in) != 0 {
			sum := 0.0
			weight := 0.0
			for _, e := range in {
				edge := asAttrs(g.Edge(e))
				nodeU := asAttrs(g.Node(e.V))
				edgeWeight := num(edge, "weight")
				sum += edgeWeight * num(nodeU, "order")
				weight += edgeWeight
			}
			entry.Barycenter = sum / weight
			entry.Weight = weight
			entry.HasBarycenter = true
		}
		result = append(result, entry)
	}
	return result
}

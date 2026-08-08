package dagro

func sortSubgraph(g *Graph, v string, cg *Graph, biasRight bool) orderResult {
	movable := g.Children(v)
	node := asAttrs(g.Node(v))
	bl := ""
	br := ""
	if node != nil {
		bl = stringValue(node, "borderLeft")
		br = stringValue(node, "borderRight")
	}
	subgraphs := map[string]orderResult{}

	if bl != "" {
		filtered := make([]string, 0, len(movable))
		for _, w := range movable {
			if w != bl && w != br {
				filtered = append(filtered, w)
			}
		}
		movable = filtered
	}

	barycenters := barycenter(g, movable)
	for i := range barycenters {
		entry := &barycenters[i]
		if len(g.Children(entry.V)) != 0 {
			subgraphResult := sortSubgraph(g, entry.V, cg, biasRight)
			subgraphs[entry.V] = subgraphResult
			if subgraphResult.HasBarycenter {
				mergeOrderBarycenters(entry, subgraphResult)
			}
		}
	}

	entries := resolveConflicts(barycenters, cg)
	expandSubgraphs(entries, subgraphs)
	result := sortOrderEntries(entries, biasRight)

	if bl != "" {
		withBorders := make([]string, 0, len(result.VS)+2)
		withBorders = append(withBorders, bl)
		withBorders = append(withBorders, result.VS...)
		withBorders = append(withBorders, br)
		result.VS = withBorders

		blPreds := g.Predecessors(bl)
		if len(blPreds) != 0 {
			brPreds := g.Predecessors(br)
			blPred := asAttrs(g.Node(blPreds[0]))
			brPred := asAttrs(g.Node(brPreds[0]))
			if !result.HasBarycenter {
				result.Barycenter = 0
				result.Weight = 0
				result.HasBarycenter = true
			}
			result.Barycenter = (result.Barycenter*result.Weight +
				num(blPred, "order") + num(brPred, "order")) / (result.Weight + 2)
			result.Weight += 2
		}
	}

	return result
}

func expandSubgraphs(entries []orderEntry, subgraphs map[string]orderResult) {
	for i := range entries {
		expanded := make([]string, 0, len(entries[i].VS))
		for _, v := range entries[i].VS {
			if subgraph, ok := subgraphs[v]; ok {
				expanded = append(expanded, subgraph.VS...)
			} else {
				expanded = append(expanded, v)
			}
		}
		entries[i].VS = expanded
	}
}

func mergeOrderBarycenters(target *orderEntry, other orderResult) {
	if target.HasBarycenter {
		target.Barycenter = (target.Barycenter*target.Weight +
			other.Barycenter*other.Weight) / (target.Weight + other.Weight)
		target.Weight += other.Weight
	} else {
		target.Barycenter = other.Barycenter
		target.Weight = other.Weight
		target.HasBarycenter = true
	}
}

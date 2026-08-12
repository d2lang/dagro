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

	reversedPairs := make([]reversedOrderPair, 0)
	usedBias := false
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if !entries[i].HasBarycenter || !entries[j].HasBarycenter ||
				!jsTruthyNumber(entries[i].Barycenter) || !jsTruthyNumber(entries[j].Barycenter) ||
				entries[i].Barycenter != entries[j].Barycenter {
				continue
			}

			nameI, nameJ := firstOrderNode(entries[i]), firstOrderNode(entries[j])
			nodeI, nodeJ := asAttrs(g.Node(nameI)), asAttrs(g.Node(nameJ))
			edgeI, edgeIOK := nodeI["edgeObj"].(Edge)
			edgeJ, edgeJOK := nodeJ["edgeObj"].(Edge)
			reversedI := boolValue(asAttrs(nodeI["edgeLabel"]), "reversed")
			reversedJ := boolValue(asAttrs(nodeJ["edgeLabel"]), "reversed")
			isReversedPair := stringValue(nodeI, "dummy") == "edge" &&
				stringValue(nodeJ, "dummy") == "edge" && edgeIOK && edgeJOK &&
				edgeI.V == edgeJ.V && edgeI.W == edgeJ.W && reversedI != reversedJ
			if !isReversedPair {
				usedBias = true
				continue
			}

			if reversedI {
				setReversedOrderPair(&reversedPairs, nameJ, entries[i])
				entries = append(entries[:i], entries[i+1:]...)
				i--
				break
			}
			setReversedOrderPair(&reversedPairs, nameI, entries[j])
			entries = append(entries[:j], entries[j+1:]...)
			j--
		}
	}

	result := sortOrderEntriesWithReversedPairs(entries, reversedPairs, biasRight)
	result.UsedBias = usedBias

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

func setReversedOrderPair(pairs *[]reversedOrderPair, key string, entry orderEntry) {
	// Dagre's object-valued reversedPairs table loses an earlier reversed
	// parallel dummy when more than one partner maps to the same forward dummy.
	// Keep every partner; sortOrderEntriesWithReversedPairs restores them in
	// encounter order immediately after the forward dummy.
	*pairs = append(*pairs, reversedOrderPair{key: key, entry: entry})
}

func firstOrderNode(entry orderEntry) string {
	if len(entry.VS) == 0 {
		return ""
	}
	return entry.VS[0]
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

package dagro

// addSubgraphConstraints records ordering constraints between non-contiguous
// sibling subgraphs. The traversal and early return mirror Dagre 0.8.5.
func addSubgraphConstraints(g, cg *Graph, vs []string) {
	prev := map[string]string{}
	rootPrev := ""

	for _, v := range vs {
		child, ok := g.Parent(v)
		for ok && child != "" {
			parent, hasParent := g.Parent(child)
			prevChild := ""
			if hasParent && parent != "" {
				prevChild = prev[parent]
				prev[parent] = child
			} else {
				prevChild = rootPrev
				rootPrev = child
			}
			if prevChild != "" && prevChild != child {
				cg.SetEdge(prevChild, child)
				break
			}
			child, ok = parent, hasParent
		}
	}
}

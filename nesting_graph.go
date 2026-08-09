package dagro

func runNestingGraph(g *Graph) {
	root := addDummyNode(g, "root", Attrs{}, "_root")
	depths := treeDepths(g)
	height := 0
	for _, depth := range depths {
		if depth-1 > height {
			height = depth - 1
		}
	}
	nodeSep := 2*height + 1
	graphLabel := asAttrs(g.Graph())
	graphLabel["nestingRoot"] = root
	for _, e := range g.Edges() {
		label := asAttrs(g.Edge(e))
		label["minlen"] = num(label, "minlen") * float64(nodeSep)
	}
	weight := sumWeights(g) + 1
	for _, child := range g.Children() {
		nestingDFS(g, root, nodeSep, weight, height, depths, child)
	}
	graphLabel["nodeRankFactor"] = float64(nodeSep)
}

func nestingDFS(g *Graph, root string, nodeSep int, weight float64, height int, depths map[string]int, v string) {
	children := g.Children(v)
	if len(children) == 0 {
		if v != root {
			g.SetEdge(root, v, Attrs{"weight": float64(0), "minlen": float64(nodeSep)})
		}
		return
	}
	top := addBorderNode(g, "_bt")
	bottom := addBorderNode(g, "_bb")
	label := asAttrs(g.Node(v))
	_ = g.setParentKnownAcyclic(top, v)
	label["borderTop"] = top
	_ = g.setParentKnownAcyclic(bottom, v)
	label["borderBottom"] = bottom
	for _, child := range children {
		nestingDFS(g, root, nodeSep, weight, height, depths, child)
		childNode := asAttrs(g.Node(child))
		childTop, childBottom := child, child
		thisWeight := 2 * weight
		if has(childNode, "borderTop") {
			childTop = stringValue(childNode, "borderTop")
			childBottom = stringValue(childNode, "borderBottom")
			thisWeight = weight
		}
		minlen := 1
		if childTop == childBottom {
			minlen = height - depths[v] + 1
		}
		g.SetEdge(top, childTop, Attrs{"weight": thisWeight, "minlen": float64(minlen), "nestingEdge": true})
		g.SetEdge(childBottom, bottom, Attrs{"weight": thisWeight, "minlen": float64(minlen), "nestingEdge": true})
	}
	if _, ok := g.Parent(v); !ok {
		g.SetEdge(root, top, Attrs{"weight": float64(0), "minlen": float64(height + depths[v])})
	}
}

func treeDepths(g *Graph) map[string]int {
	depths := map[string]int{}
	var dfs func(string, int)
	dfs = func(v string, depth int) {
		for _, child := range g.Children(v) {
			dfs(child, depth+1)
		}
		depths[v] = depth
	}
	for _, v := range g.Children() {
		dfs(v, 1)
	}
	return depths
}

func sumWeights(g *Graph) float64 {
	total := 0.0
	for _, e := range g.Edges() {
		total += num(asAttrs(g.Edge(e)), "weight")
	}
	return total
}

func cleanupNestingGraph(g *Graph) {
	graphLabel := asAttrs(g.Graph())
	root := stringValue(graphLabel, "nestingRoot")
	g.RemoveNode(root)
	delete(graphLabel, "nestingRoot")
	for _, e := range g.Edges() {
		if boolValue(asAttrs(g.Edge(e)), "nestingEdge") {
			g.RemoveEdge(e)
		}
	}
}

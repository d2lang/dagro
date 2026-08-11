package dagro

// networkSimplex assigns ranks and iteratively exchanges negative-cut tree
// edges to reduce weighted edge length. The structure follows
// lib/rank/network-simplex.ts from Dagre 3.1.1.
func networkSimplex(input *Graph) {
	g := simplify(input)
	longestPath(g)
	t := feasibleTree(g)
	initLowLimValues(t)
	initCutValues(t, g)

	for {
		e, ok := leaveEdge(t)
		if !ok {
			break
		}
		f, ok := enterEdge(t, g, e)
		if !ok {
			panic("dagro: networkSimplex could not find an entering edge")
		}
		exchangeEdges(t, g, e, f)
	}
}

func initCutValues(t, g *Graph) {
	vs := postorder(t, t.Nodes())
	if len(vs) > 0 {
		vs = vs[:len(vs)-1]
	}
	for _, v := range vs {
		assignCutValue(t, g, v)
	}
}

func assignCutValue(t, g *Graph, child string) {
	childLabel := asAttrs(t.Node(child))
	parent := stringValue(childLabel, "parent")
	edge := treeEdge(t, child, parent)
	label := asAttrs(t.Edge(edge))
	label["cutvalue"] = calcCutValue(t, g, child)
}

func calcCutValue(t, g *Graph, child string) float64 {
	childLabel := asAttrs(t.Node(child))
	parent := stringValue(childLabel, "parent")
	childIsTail := true

	var graphEdge Edge
	if g.HasEdge(child, parent) {
		graphEdge = Edge{V: child, W: parent}
	} else {
		childIsTail = false
		graphEdge = Edge{V: parent, W: child}
	}
	cutValue := num(asAttrs(g.Edge(graphEdge)), "weight")

	for _, e := range g.NodeEdges(child) {
		isOutEdge := e.V == child
		other := e.V
		if isOutEdge {
			other = e.W
		}
		if other == parent {
			continue
		}

		pointsToHead := isOutEdge == childIsTail
		otherWeight := num(asAttrs(g.Edge(e)), "weight")
		if pointsToHead {
			cutValue += otherWeight
		} else {
			cutValue -= otherWeight
		}
		if isTreeEdge(t, child, other) {
			otherCutValue := num(asAttrs(t.Edge(treeEdge(t, child, other))), "cutvalue")
			if pointsToHead {
				cutValue -= otherCutValue
			} else {
				cutValue += otherCutValue
			}
		}
	}

	return cutValue
}

// initLowLimValues accepts an optional root to mirror the JS test hook.
func initLowLimValues(tree *Graph, roots ...string) {
	root := tree.Nodes()[0]
	if len(roots) > 0 {
		root = roots[0]
	}
	dfsAssignLowLim(tree, map[string]bool{}, 1, root, "")
}

func dfsAssignLowLim(tree *Graph, visited map[string]bool, nextLim float64, v, parent string) float64 {
	low := nextLim
	label := asAttrs(tree.Node(v))
	visited[v] = true
	for _, w := range tree.Neighbors(v) {
		if !visited[w] {
			nextLim = dfsAssignLowLim(tree, visited, nextLim, w, v)
		}
	}

	label["low"] = low
	label["lim"] = nextLim
	nextLim++
	if parent != "" {
		label["parent"] = parent
	} else {
		delete(label, "parent")
	}
	return nextLim
}

func leaveEdge(tree *Graph) (Edge, bool) {
	for _, e := range tree.Edges() {
		if num(asAttrs(tree.Edge(e)), "cutvalue") < 0 {
			return e, true
		}
	}
	return Edge{}, false
}

func enterEdge(t, g *Graph, edge Edge) (Edge, bool) {
	v, w := edge.V, edge.W
	if !g.HasEdge(v, w) {
		v, w = w, v
	}

	vLabel := asAttrs(t.Node(v))
	wLabel := asAttrs(t.Node(w))
	tailLabel := vLabel
	flip := false
	if num(vLabel, "lim") > num(wLabel, "lim") {
		tailLabel = wLabel
		flip = true
	}

	var best Edge
	bestSlack := 0.0
	found := false
	for _, candidate := range g.Edges() {
		vDescendant := isDescendant(asAttrs(t.Node(candidate.V)), tailLabel)
		wDescendant := isDescendant(asAttrs(t.Node(candidate.W)), tailLabel)
		if flip != vDescendant || flip == wDescendant {
			continue
		}
		s := slack(g, candidate)
		if !found || s < bestSlack {
			best, bestSlack, found = candidate, s, true
		}
	}
	return best, found
}

func exchangeEdges(t, g *Graph, e, f Edge) {
	t.RemoveEdge(e)
	t.SetEdge(f.V, f.W, Attrs{})
	initLowLimValues(t)
	initCutValues(t, g)
	updateRanks(t, g)
}

func updateRanks(t, g *Graph) {
	root, found := "", false
	for _, v := range t.Nodes() {
		label := asAttrs(g.Node(v))
		if !rankTruthy(label["parent"]) {
			root = v
			found = true
			break
		}
	}
	if !found {
		return
	}

	vs := preorder(t, []string{root})
	if len(vs) > 0 {
		vs = vs[1:]
	}
	for _, v := range vs {
		parent := stringValue(asAttrs(t.Node(v)), "parent")
		flipped := false
		var edge Edge
		if g.HasEdge(v, parent) {
			edge = Edge{V: v, W: parent}
		} else {
			edge = Edge{V: parent, W: v}
			flipped = true
		}
		minlen := num(asAttrs(g.Edge(edge)), "minlen")
		parentRank := num(asAttrs(g.Node(parent)), "rank")
		if flipped {
			asAttrs(g.Node(v))["rank"] = parentRank + minlen
		} else {
			asAttrs(g.Node(v))["rank"] = parentRank - minlen
		}
	}
}

func isTreeEdge(tree *Graph, u, v string) bool {
	return tree.HasEdge(u, v)
}

func isDescendant(vLabel, rootLabel Attrs) bool {
	return num(rootLabel, "low") <= num(vLabel, "lim") &&
		num(vLabel, "lim") <= num(rootLabel, "lim")
}

func treeEdge(tree *Graph, u, v string) Edge {
	for _, e := range tree.NodeEdges(u, v) {
		return e
	}
	panic("dagro: expected tree edge")
}

func rankTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && x == x
	case float32:
		return x != 0 && x == x
	case int:
		return x != 0
	case int8:
		return x != 0
	case int16:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case uint:
		return x != 0
	case uint8:
		return x != 0
	case uint16:
		return x != 0
	case uint32:
		return x != 0
	case uint64:
		return x != 0
	default:
		return true
	}
}

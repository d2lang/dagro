package dagro

// networkSimplex assigns ranks and iteratively exchanges negative-cut tree
// edges to reduce weighted edge length. The structure follows
// lib/rank/network-simplex.ts from Dagre 3.1.1.
func networkSimplex(input *Graph) {
	g := simplify(input)
	longestPath(g)
	t := feasibleTree(g)
	order := newRankTreeOrder(t)
	order.assignLowLim(t)
	assignCutValues(t, g, order.post)

	for {
		e, ok := leaveEdge(t)
		if !ok {
			break
		}
		f, ok := enterEdge(t, g, e)
		if !ok {
			panic("dagro: networkSimplex could not find an entering edge")
		}
		exchangeEdges(t, g, e, f, order)
	}
}

func initCutValues(t, g *Graph) {
	assignCutValues(t, g, postorder(t, t.Nodes()))
}

func assignCutValues(t, g *Graph, vs []string) {
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

	// The input graph is not mutated while computing cuts. Walk its incident
	// edge maps directly, in NodeEdges order, without copying both lists on
	// every tree-edge exchange.
	for _, incident := range [2]*edgeMap{g.in[child], g.out[child]} {
		if incident == nil {
			continue
		}
		for _, id := range incident.order {
			e := incident.items[id]
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

// A feasible tree stays connected and keeps the same nodes through all edge
// exchanges. Its low/lim traversal is also the traversal used to calculate
// cuts and update ranks. Retain both orders instead of traversing it three
// times, and reuse the traversal storage across exchanges.
type rankTreeOrder struct {
	root      string
	visited   map[string]bool
	pre, post []string
}

func newRankTreeOrder(tree *Graph) *rankTreeOrder {
	return &rankTreeOrder{
		root: tree.Nodes()[0], visited: make(map[string]bool, tree.NodeCount()),
		pre: make([]string, 0, tree.NodeCount()), post: make([]string, 0, tree.NodeCount()),
	}
}

func (order *rankTreeOrder) assignLowLim(tree *Graph) {
	clear(order.visited)
	order.pre, order.post = order.pre[:0], order.post[:0]
	dfsAssignLowLim(tree, order.visited, 1, order.root, "", order)
}

func dfsAssignLowLim(tree *Graph, visited map[string]bool, nextLim float64, v, parent string, orders ...*rankTreeOrder) float64 {
	low := nextLim
	label := asAttrs(tree.Node(v))
	visited[v] = true
	if len(orders) > 0 {
		orders[0].pre = append(orders[0].pre, v)
	}
	for _, w := range tree.Neighbors(v) {
		if !visited[w] {
			nextLim = dfsAssignLowLim(tree, visited, nextLim, w, v, orders...)
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
	if len(orders) > 0 {
		orders[0].post = append(orders[0].post, v)
	}
	return nextLim
}

func leaveEdge(tree *Graph) (Edge, bool) {
	for _, id := range tree.edgeObjs.order {
		e := tree.edgeObjs.items[id]
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
	for _, id := range g.edgeObjs.order {
		candidate := g.edgeObjs.items[id]
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

func exchangeEdges(t, g *Graph, e, f Edge, orders ...*rankTreeOrder) {
	t.RemoveEdge(e)
	t.SetEdge(f.V, f.W, Attrs{})
	if len(orders) > 0 {
		order := orders[0]
		order.assignLowLim(t)
		assignCutValues(t, g, order.post)
		updateRanksWithOrder(t, g, order.pre)
		return
	}
	initLowLimValues(t)
	initCutValues(t, g)
	updateRanks(t, g)
}

func updateRanks(t, g *Graph) {
	updateRanksWithOrder(t, g, nil)
}

func updateRanksWithOrder(t, g *Graph, vs []string) {
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

	// The JS hook permits a root selected from input labels to differ from
	// the low/lim root. Preserve that traversal when the roots differ.
	if len(vs) == 0 || vs[0] != root {
		vs = preorder(t, []string{root})
	}
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
	// Feasible trees are simple and undirected. Locate their canonical edge
	// directly instead of allocating and scanning both adjacency lists.
	// Keep NodeEdges' empty-string filter behavior for the JS test hook.
	if !tree.directed && !tree.multigraph && v != "" {
		if e, ok := tree.edgeObjs.items[edgeArgsToID(false, u, v, nil)]; ok &&
			(e.V == u && e.W == v || e.V == v && e.W == u) {
			return e
		}
		panic("dagro: expected tree edge")
	}
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

package dagro

type edgeNameState struct {
	name    string
	present bool
}

func runAcyclic(g *Graph) {
	var fas []Edge
	if stringValue(asAttrs(g.Graph()), "acyclicer") == "greedy" {
		fas = greedyFAS(g, func(e Edge) float64 { return num(asAttrs(g.Edge(e)), "weight") })
	} else {
		fas = dfsFAS(g)
	}
	for _, e := range fas {
		label := asAttrs(g.Edge(e))
		g.RemoveEdge(e)
		label["forwardName"] = edgeNameState{name: e.Name, present: e.HasName}
		label["reversed"] = true
		g.SetEdge(e.W, e.V, label, nextReversedEdgeName(g, e.W, e.V))
	}
}

func nextReversedEdgeName(g *Graph, v, w string) string {
	for {
		name := g.uniqueID("rev")
		if !g.HasEdge(v, w, name) {
			return name
		}
	}
}

func dfsFAS(g *Graph) []Edge {
	var fas []Edge
	stack, visited := map[string]bool{}, map[string]bool{}
	var dfs func(string)
	dfs = func(v string) {
		if visited[v] {
			return
		}
		visited[v], stack[v] = true, true
		for _, e := range g.OutEdges(v) {
			if stack[e.W] {
				fas = append(fas, e)
			} else {
				dfs(e.W)
			}
		}
		delete(stack, v)
	}
	// Modern Dagre starts DFS from sources before visiting any remaining
	// components. This makes cycle breaking deterministic when a source feeds
	// into a strongly connected component whose nodes were inserted earlier.
	for _, v := range g.Sources() {
		dfs(v)
	}
	for _, v := range g.Nodes() {
		dfs(v)
	}
	return fas
}

func undoAcyclic(g *Graph) {
	for _, e := range g.Edges() {
		label := asAttrs(g.Edge(e))
		if !boolValue(label, "reversed") {
			continue
		}
		g.RemoveEdge(e)
		state, _ := label["forwardName"].(edgeNameState)
		delete(label, "reversed")
		delete(label, "forwardName")
		if state.present {
			g.SetEdge(e.W, e.V, label, state.name)
		} else {
			g.SetEdge(e.W, e.V, label)
		}
	}
}

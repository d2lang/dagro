package dagro

type postorderRange struct{ low, lim int }

func parentDummyChains(g *Graph) {
	postorderNums := compoundPostorder(g)
	chains, _ := asAttrs(g.Graph())["dummyChains"].([]string)
	for _, start := range chains {
		v := start
		node := asAttrs(g.Node(v))
		edgeObj := node["edgeObj"].(Edge)
		path, lca := findCompoundPath(g, postorderNums, edgeObj.V, edgeObj.W)
		pathIdx := 0
		pathV := path[pathIdx]
		ascending := true
		for v != edgeObj.W {
			node = asAttrs(g.Node(v))
			if ascending {
				for {
					pathV = path[pathIdx]
					if pathV == lca || num(asAttrs(g.Node(pathV)), "maxRank") >= num(node, "rank") {
						break
					}
					pathIdx++
				}
				if pathV == lca {
					ascending = false
				}
			}
			if !ascending {
				for pathIdx < len(path)-1 {
					next := path[pathIdx+1]
					if num(asAttrs(g.Node(next)), "minRank") > num(node, "rank") {
						break
					}
					pathIdx++
					pathV = next
				}
				pathV = path[pathIdx]
			}
			if pathV == "" {
				_ = g.setParentKnownAcyclic(v)
			} else {
				_ = g.setParentKnownAcyclic(v, pathV)
			}
			v = g.Successors(v)[0]
		}
	}
}

func findCompoundPath(g *Graph, nums map[string]postorderRange, v, w string) ([]string, string) {
	var vPath, wPath []string
	low := nums[v].low
	if nums[w].low < low {
		low = nums[w].low
	}
	lim := nums[v].lim
	if nums[w].lim > lim {
		lim = nums[w].lim
	}
	parent := v
	for {
		p, ok := g.Parent(parent)
		if ok {
			parent = p
		} else {
			parent = ""
		}
		vPath = append(vPath, parent)
		if parent == "" || !(nums[parent].low > low || lim > nums[parent].lim) {
			break
		}
	}
	lca := parent
	parent = w
	for {
		p, ok := g.Parent(parent)
		if ok {
			parent = p
		} else {
			parent = ""
		}
		if parent == lca {
			break
		}
		wPath = append(wPath, parent)
	}
	for i, j := 0, len(wPath)-1; i < j; i, j = i+1, j-1 {
		wPath[i], wPath[j] = wPath[j], wPath[i]
	}
	return append(vPath, wPath...), lca
}

func compoundPostorder(g *Graph) map[string]postorderRange {
	result := map[string]postorderRange{}
	lim := 0
	var dfs func(string)
	dfs = func(v string) {
		low := lim
		for _, child := range g.Children(v) {
			dfs(child)
		}
		result[v] = postorderRange{low: low, lim: lim}
		lim++
	}
	for _, v := range g.Children() {
		dfs(v)
	}
	return result
}

package dagro

func greedyFAS(g *Graph, weightFn func(Edge) float64) []Edge {
	if g.NodeCount() <= 1 {
		return nil
	}
	stateGraph, buckets, zeroIdx := buildFASState(g, weightFn)
	results := doGreedyFAS(stateGraph, buckets, zeroIdx)
	var expanded []Edge
	for _, e := range results {
		expanded = append(expanded, g.OutEdges(e.V, e.W)...)
	}
	return expanded
}

func doGreedyFAS(g *Graph, buckets []*fasList, zeroIdx int) []Edge {
	var results []Edge
	sources, sinks := buckets[len(buckets)-1], buckets[0]
	for g.NodeCount() > 0 {
		for entry := sinks.dequeue(); entry != nil; entry = sinks.dequeue() {
			removeFASNode(g, buckets, zeroIdx, entry, false)
		}
		for entry := sources.dequeue(); entry != nil; entry = sources.dequeue() {
			removeFASNode(g, buckets, zeroIdx, entry, false)
		}
		if g.NodeCount() > 0 {
			for i := len(buckets) - 2; i > 0; i-- {
				if entry := buckets[i].dequeue(); entry != nil {
					results = append(results, removeFASNode(g, buckets, zeroIdx, entry, true)...)
					break
				}
			}
		}
	}
	return results
}

func removeFASNode(g *Graph, buckets []*fasList, zeroIdx int, entry *fasEntry, collect bool) []Edge {
	var results []Edge
	for _, e := range g.InEdges(entry.V) {
		weight := number(g.Edge(e))
		uEntry := g.Node(e.V).(*fasEntry)
		if collect {
			results = append(results, Edge{V: e.V, W: e.W})
		}
		uEntry.Out -= weight
		assignFASBucket(buckets, zeroIdx, uEntry)
	}
	for _, e := range g.OutEdges(entry.V) {
		weight := number(g.Edge(e))
		wEntry := g.Node(e.W).(*fasEntry)
		wEntry.In -= weight
		assignFASBucket(buckets, zeroIdx, wEntry)
	}
	g.RemoveNode(entry.V)
	return results
}

func buildFASState(g *Graph, weightFn func(Edge) float64) (*Graph, []*fasList, int) {
	fasGraph := NewGraph()
	maxIn, maxOut := 0.0, 0.0
	for _, v := range g.Nodes() {
		fasGraph.SetNode(v, &fasEntry{V: v})
	}
	for _, e := range g.Edges() {
		prevWeight := 0.0
		if existing := fasGraph.EdgeByArgs(e.V, e.W); existing != nil {
			prevWeight = number(existing)
		}
		weight := weightFn(e)
		fasGraph.SetEdge(e.V, e.W, prevWeight+weight)
		vEntry, wEntry := fasGraph.Node(e.V).(*fasEntry), fasGraph.Node(e.W).(*fasEntry)
		vEntry.Out += weight
		wEntry.In += weight
		maxOut, maxIn = mathMaxJS(maxOut, vEntry.Out), mathMaxJS(maxIn, wEntry.In)
	}
	buckets := make([]*fasList, int(maxOut+maxIn)+3)
	for i := range buckets {
		buckets[i] = &fasList{}
	}
	zeroIdx := int(maxIn) + 1
	for _, v := range fasGraph.Nodes() {
		assignFASBucket(buckets, zeroIdx, fasGraph.Node(v).(*fasEntry))
	}
	return fasGraph, buckets, zeroIdx
}

func assignFASBucket(buckets []*fasList, zeroIdx int, entry *fasEntry) {
	index := 0
	if !jsTruthyNumber(entry.Out) {
		index = 0
	} else if !jsTruthyNumber(entry.In) {
		index = len(buckets) - 1
	} else {
		index = int(entry.Out-entry.In) + zeroIdx
	}
	buckets[index].enqueue(entry)
}

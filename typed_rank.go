package dagro

import (
	"math"
	"sort"
)

// resize returns s with length n and zeroed contents, reusing its storage.
// It is not inlined: it is called from about thirty places, none of them in
// a hot loop. (grow, below, is called in hot loops and stays inlinable.)
//
//go:noinline
func resize[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	s = s[:n]
	clear(s)
	return s
}

// grow returns s with length n, reusing its storage without clearing it: for
// buffers every element of which is written before it is read.
func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	return s[:n]
}

// appendGrow is append(s, v), except that a full s doubles its capacity.
// append grows a large slice by about 1.25 times, so a slice built one element
// at a time allocates about five times its final size in all; doubling
// allocates at most twice. The per-node and per-edge arrays grow this way
// while a large graph gains its dummy nodes and edges: a state that is not
// reused (see putLState) then costs less than half the allocation.
func appendGrow[T any](s []T, v T) []T {
	if len(s) == cap(s) {
		n := make([]T, len(s), max(2*len(s), 16))
		copy(n, s)
		s = n
	}
	return append(s, v)
}

// reserve returns s with room for n more elements, in one allocation when s
// has less.
func reserve[T any](s []T, n int) []T {
	if n <= cap(s)-len(s) {
		return s
	}
	x := make([]T, len(s), len(s)+n)
	copy(x, s)
	return x
}

// rgraph is the graph a ranker works on: asNonCompoundGraph(g), or its
// simplification for network simplex. Nodes are local indices in the order of
// the Graph the Attrs engine builds (JavaScript order of g's leaves); edges
// are in insertion order. Ranks live in rank/hasRank and are written back to
// the shared node labels at the end, exactly where the Attrs engine writes
// them through shared label maps.
type rgraph struct {
	ids            []int32 // local -> global node id
	ev, ew         []int32
	weight, minlen []float64
	incidence
	rank    []float64
	hasRank []bool
}

// incidence lists the edges ev[i] -> ew[i] of a graph by node, each list in
// edge order: the edges into node v are inList[inStart[v]:inStart[v+1]], the
// edges out of it outList[outStart[v]:outStart[v+1]].
type incidence struct {
	inStart, inList   []int32
	outStart, outList []int32
	fill              []int32 // build's scratch
}

// build makes the lists of nodes 0 to n-1. local, when not nil, maps the
// endpoints in ev and ew to their node numbers.
func (c *incidence) build(n int, ev, ew, local []int32) {
	c.inStart = resize(c.inStart, n+1)
	c.outStart = resize(c.outStart, n+1)
	for i := range ev {
		v, w := ev[i], ew[i]
		if local != nil {
			v, w = local[v], local[w]
		}
		c.outStart[v+1]++
		c.inStart[w+1]++
	}
	for v := 0; v < n; v++ {
		c.outStart[v+1] += c.outStart[v]
		c.inStart[v+1] += c.inStart[v]
	}
	c.inList = grow(c.inList, len(ev))
	c.outList = grow(c.outList, len(ev))
	c.fill = grow(c.fill, 2*n)
	inFill, outFill := c.fill[:n], c.fill[n:]
	copy(inFill, c.inStart[:n])
	copy(outFill, c.outStart[:n])
	for i := range ev {
		v, w := ev[i], ew[i]
		if local != nil {
			v, w = local[v], local[w]
		}
		c.outList[outFill[v]] = int32(i)
		outFill[v]++
		c.inList[inFill[w]] = int32(i)
		inFill[w]++
	}
}

func (rg *rgraph) n() int { return len(rg.ids) }

func (rg *rgraph) clearEdges() {
	rg.ev, rg.ew, rg.weight, rg.minlen = rg.ev[:0], rg.ew[:0], rg.weight[:0], rg.minlen[:0]
}

// finish builds the incidence lists; each keeps edge insertion order.
func (rg *rgraph) finish() {
	n := rg.n()
	rg.incidence.build(n, rg.ev, rg.ew, nil)
	rg.rank = grow(rg.rank, n)
	rg.hasRank = resize(rg.hasRank, n)
	for i := range rg.rank {
		rg.rank[i] = math.NaN()
	}
}

func (rg *rgraph) slack(e int32) float64 {
	return rg.rank[rg.ew[e]] - rg.rank[rg.ev[e]] - rg.minlen[e]
}

type lpFrame struct {
	v    int32
	pos  int32
	best float64
}

type ftFrame struct {
	v   int32
	pos int32 // position in in-list ++ out-list
}

type lowLimFrame struct {
	v        int32
	from, to int32 // neighbor range in nbr
}

// rankState keeps the ranking storage between layouts.
type rankState struct {
	full, simple rgraph
	tree         rtree
	local        []int32
	firstOut     []int32 // simplify: per node, its first merged out edge
	nextOut      []int32 // simplify: per merged edge, the next one from its source
	visited      []bool
	lp           []lpFrame
	ft           []ftFrame
	ll           []lowLimFrame
	snapshot     []int32
	nbr          []int32
	idx, rest    []int32
	opp          []int32 // simplify: per merged edge, the merged edge in the opposite direction, or inone
	hasOpp       bool    // some merged edge has an opposite one
}

// nonCompoundRankGraph is asNonCompoundGraph(g) restricted to what the
// rankers read: leaves in JavaScript order and every edge with its weight and
// minlen. The typed engine only runs when no edge touches a compound node, so
// every endpoint is a leaf.
func (s *lstate) nonCompoundRankGraph() *rgraph {
	r := &s.r
	all := s.nodes()
	r.local = resize(r.local, len(s.g.nodes))
	rg := &r.full
	rg.ids = rg.ids[:0]
	rg.clearEdges()
	for _, v := range all {
		if s.g.hasChildren(v) {
			r.local[v] = inone
			continue
		}
		r.local[v] = int32(len(rg.ids))
		rg.ids = append(rg.ids, v)
	}
	for _, e := range s.edges() {
		ed := &s.g.edges[e]
		l := &s.el[ed.label]
		rg.ev = append(rg.ev, r.local[ed.v])
		rg.ew = append(rg.ew, r.local[ed.w])
		rg.weight = append(rg.weight, l.weight)
		rg.minlen = append(rg.minlen, l.minlen)
	}
	return rg
}

// simplify is the port of simplify: parallel edges merge into the first one,
// weights add up from 0 and minlen is Math.max(1, minlen...). With
// findOpposite it also records, for every merged edge, the merged edge in the
// opposite direction (see rtree.graphEdge).
func (r *rankState) simplify(rg *rgraph, findOpposite bool) *rgraph {
	out := &r.simple
	out.ids = append(out.ids[:0], rg.ids...)
	out.clearEdges()
	// The merged edge of (v, w) is found on v's chain of merged edges.
	r.firstOut = grow(r.firstOut, rg.n())
	for i := range r.firstOut {
		r.firstOut[i] = inone
	}
	r.nextOut = r.nextOut[:0]
edges:
	for i := range rg.ev {
		v, w := rg.ev[i], rg.ew[i]
		for j := r.firstOut[v]; j != inone; j = r.nextOut[j] {
			if out.ew[j] == w {
				out.weight[j] = out.weight[j] + rg.weight[i]
				out.minlen[j] = maxJS2(out.minlen[j], rg.minlen[i])
				continue edges
			}
		}
		r.nextOut = append(r.nextOut, r.firstOut[v])
		r.firstOut[v] = int32(len(out.ev))
		out.ev = append(out.ev, v)
		out.ew = append(out.ew, w)
		out.weight = append(out.weight, 0+rg.weight[i])
		out.minlen = append(out.minlen, maxJS2(1, rg.minlen[i]))
	}
	out.finish()
	r.hasOpp = false
	if findOpposite {
		r.opp = grow(r.opp, len(out.ev))
		for e := range out.ev {
			r.opp[e] = inone
			v, w := out.ev[e], out.ew[e]
			for j := r.firstOut[w]; j != inone; j = r.nextOut[j] {
				if out.ew[j] == v {
					r.opp[e], r.hasOpp = j, true
					break
				}
			}
		}
	}
	return out
}

func (s *lstate) rank() {
	s.rankGraph(s.nonCompoundRankGraph())
}

func (s *lstate) rankGraph(rg *rgraph) {
	rg.finish()
	switch s.ranker {
	case "longest-path":
		s.r.longestPath(rg)
		s.writeRanks(rg)
	case "tight-tree":
		s.r.longestPath(rg)
		s.r.feasibleTree(rg, s)
		s.writeRanks(rg)
	default:
		// The ranking graph has edges in both directions between two nodes
		// only when acyclicer "greedy" left a cycle (greedyFAS files a node
		// whose out-weights sum to 0, -0 or NaN as a sink): the DFS acyclicer
		// leaves the input edges acyclic, and every nesting edge touches the
		// nesting root or a border node, which no input edge does.
		sg := s.r.simplify(rg, s.acyclicer == "greedy")
		s.r.networkSimplex(sg, s)
		s.writeRanks(sg)
	}
}

func (s *lstate) writeRanks(rg *rgraph) {
	for i, id := range rg.ids {
		if rg.hasRank[i] {
			n := &s.nl[id]
			n.rank, n.hasRank = rg.rank[i], true
		}
	}
}

// longestPath ports lib/rank/util longestPath with an explicit stack. Out
// edges are visited in order and the minimum keeps the first of equal values,
// as the recursive version does.
func (r *rankState) longestPath(rg *rgraph) {
	n := rg.n()
	r.visited = resize(r.visited, n)
	visited := r.visited
	stack := r.lp[:0]
	for v := 0; v < n; v++ {
		if rg.inStart[v] != rg.inStart[v+1] {
			continue
		}
		if visited[v] {
			continue
		}
		visited[v] = true
		stack = append(stack[:0], lpFrame{v: int32(v), pos: rg.outStart[v], best: math.Inf(1)})
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.pos == rg.outStart[top.v+1] {
				rk := top.best
				if math.IsInf(rk, 1) {
					rk = 0
				}
				rg.rank[top.v], rg.hasRank[top.v] = rk, true
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					parent := &stack[len(stack)-1]
					e := rg.outList[parent.pos]
					parent.pos++
					if candidate := rk - rg.minlen[e]; candidate < parent.best {
						parent.best = candidate
					}
				}
				continue
			}
			e := rg.outList[top.pos]
			w := rg.ew[e]
			if visited[w] {
				top.pos++
				if candidate := rg.rank[w] - rg.minlen[e]; candidate < top.best {
					top.best = candidate
				}
				continue
			}
			visited[w] = true
			stack = append(stack, lpFrame{v: w, pos: rg.outStart[w], best: math.Inf(1)})
		}
	}
	r.lp = stack[:0]
}

// rtree is the undirected feasible tree. Graphlib stores an undirected edge
// with its lexicographically smaller endpoint first; that endpoint lists the
// other as a successor and the other lists it as a predecessor, which decides
// the neighbor order of the low/lim traversal.
type rtree struct {
	rg       *rgraph
	s        *lstate
	in       []bool
	idxNodes []int32 // tree nodes with array-index names, by numeric value
	others   []int32 // other tree nodes in insertion order
	count    int

	ea, eb, eg []int32
	opp        []int32 // rankState.opp when the graph has opposite edges, else nil
	cut        []float64
	alive      []bool
	entA, entB []int32
	live       int

	pHead, pTail, sHead, sTail []int32
	ent                        []tentry
	free                       int32

	low, lim           []int32
	parent, parentEdge []int32
	pre, post          []int32
	preIdx             []int32 // position of each node in pre
	visited            []bool
	remap              []int32 // compact: old edge index -> new, inone when dropped
}

type tentry struct{ node, edge, prev, next int32 }

func (t *rtree) init(rg *rgraph, s *lstate) {
	n := rg.n()
	t.rg, t.s = rg, s
	t.opp = nil
	t.in = resize(t.in, n)
	t.idxNodes, t.others, t.count = t.idxNodes[:0], t.others[:0], 0
	t.ea, t.eb, t.eg, t.cut = t.ea[:0], t.eb[:0], t.eg[:0], t.cut[:0]
	t.alive, t.entA, t.entB, t.live = t.alive[:0], t.entA[:0], t.entB[:0], 0
	t.pHead = grow(t.pHead, n)
	t.pTail = grow(t.pTail, n)
	t.sHead = grow(t.sHead, n)
	t.sTail = grow(t.sTail, n)
	for i := 0; i < n; i++ {
		t.pHead[i], t.pTail[i], t.sHead[i], t.sTail[i] = inone, inone, inone, inone
	}
	t.ent, t.free = t.ent[:0], inone
	t.low = grow(t.low, n)
	t.lim = grow(t.lim, n)
	t.parent = grow(t.parent, n)
	t.parentEdge = grow(t.parentEdge, n)
	t.visited = grow(t.visited, n)
	t.preIdx = grow(t.preIdx, n)
	t.pre, t.post = t.pre[:0], t.post[:0]
}

func (t *rtree) key(v int32) int64 { return t.s.names.key[t.rg.ids[v]] }

func (t *rtree) addNode(v int32) {
	t.in[v] = true
	t.count++
	if k := t.key(v); k >= 0 {
		i := sort.Search(len(t.idxNodes), func(i int) bool { return t.key(t.idxNodes[i]) > k })
		t.idxNodes = append(t.idxNodes, 0)
		copy(t.idxNodes[i+1:], t.idxNodes[i:])
		t.idxNodes[i] = v
		return
	}
	t.others = append(t.others, v)
}

// nodes returns t.Nodes() in JavaScript order.
func (t *rtree) nodes(buf []int32) []int32 {
	buf = append(buf[:0], t.idxNodes...)
	return append(buf, t.others...)
}

func (t *rtree) newEntry(node, edge int32) int32 {
	if t.free != inone {
		i := t.free
		t.free = t.ent[i].next
		t.ent[i] = tentry{node: node, edge: edge, prev: inone, next: inone}
		return i
	}
	t.ent = append(t.ent, tentry{node: node, edge: edge, prev: inone, next: inone})
	return int32(len(t.ent) - 1)
}

func (t *rtree) link(head, tail *int32, i int32) {
	t.ent[i].prev = *tail
	if *tail != inone {
		t.ent[*tail].next = i
	} else {
		*head = i
	}
	*tail = i
}

func (t *rtree) unlink(head, tail *int32, i int32) {
	en := &t.ent[i]
	if en.prev != inone {
		t.ent[en.prev].next = en.next
	} else {
		*head = en.next
	}
	if en.next != inone {
		t.ent[en.next].prev = en.prev
	} else {
		*tail = en.prev
	}
	en.next = t.free
	t.free = i
}

// addEdge is t.SetEdge(v, w) for a new edge backed by graph edge ge.
func (t *rtree) addEdge(v, w, ge int32) {
	a, b := v, w
	if t.s.names.jsGreater(t.rg.ids[a], t.rg.ids[b]) {
		a, b = b, a
	}
	idx := int32(len(t.ea))
	t.ea = append(t.ea, a)
	t.eb = append(t.eb, b)
	t.eg = append(t.eg, ge)
	t.cut = append(t.cut, math.NaN())
	t.alive = append(t.alive, true)
	eb := t.newEntry(a, idx) // b's predecessor a
	t.link(&t.pHead[b], &t.pTail[b], eb)
	ea := t.newEntry(b, idx) // a's successor b
	t.link(&t.sHead[a], &t.sTail[a], ea)
	t.entA = append(t.entA, ea)
	t.entB = append(t.entB, eb)
	t.live++
}

func (t *rtree) removeEdge(idx int32) {
	t.alive[idx] = false
	a, b := t.ea[idx], t.eb[idx]
	t.unlink(&t.sHead[a], &t.sTail[a], t.entA[idx])
	t.unlink(&t.pHead[b], &t.pTail[b], t.entB[idx])
	t.live--
}

// compact drops removed edges, keeping insertion order. Parent edges are
// renumbered with them (a removed edge becomes inone), so nodes whose low/lim
// traversal is not redone keep valid parent edges.
func (t *rtree) compact() {
	t.remap = grow(t.remap, len(t.ea))
	j := int32(0)
	for i := range t.ea {
		if !t.alive[i] {
			t.remap[i] = inone
			continue
		}
		t.remap[i] = j
		if int32(i) != j {
			t.ea[j], t.eb[j], t.eg[j], t.cut[j] = t.ea[i], t.eb[i], t.eg[i], t.cut[i]
			t.alive[j], t.entA[j], t.entB[j] = true, t.entA[i], t.entB[i]
			t.ent[t.entA[j]].edge = j
			t.ent[t.entB[j]].edge = j
		}
		j++
	}
	t.ea, t.eb, t.eg, t.cut = t.ea[:j], t.eb[:j], t.eg[:j], t.cut[:j]
	t.alive, t.entA, t.entB = t.alive[:j], t.entA[:j], t.entB[:j]
	for v, e := range t.parentEdge {
		if e != inone {
			t.parentEdge[v] = t.remap[e]
		}
	}
}

// feasibleTree ports feasibleTree; ranks are shifted in rg.
func (r *rankState) feasibleTree(rg *rgraph, s *lstate) *rtree {
	n := rg.n()
	t := &r.tree
	t.init(rg, s)
	t.addNode(0)
	stack := r.ft[:0]
	tightTree := func() int {
		r.snapshot = t.nodes(r.snapshot)
		for _, root := range r.snapshot {
			stack = append(stack[:0], ftFrame{v: root})
			for len(stack) > 0 {
				top := &stack[len(stack)-1]
				v := top.v
				inCount := rg.inStart[v+1] - rg.inStart[v]
				total := inCount + rg.outStart[v+1] - rg.outStart[v]
				if top.pos == total {
					stack = stack[:len(stack)-1]
					continue
				}
				var e int32
				if top.pos < inCount {
					e = rg.inList[rg.inStart[v]+top.pos]
				} else {
					e = rg.outList[rg.outStart[v]+top.pos-inCount]
				}
				// w := e.V; if v == e.V { w = e.W }
				w := rg.ev[e]
				if v == w {
					w = rg.ew[e]
				}
				top.pos++
				if !t.in[w] && !jsTruthyNumber(rg.slack(e)) {
					t.addNode(w)
					t.addEdge(v, w, e)
					stack = append(stack, ftFrame{v: w})
				}
			}
		}
		return t.count
	}
	for tightTree() < n {
		best, bestSlack, found := int32(0), 0.0, false
		for e := range rg.ev {
			if t.in[rg.ev[e]] == t.in[rg.ew[e]] {
				continue
			}
			sl := rg.slack(int32(e))
			if !found || sl < bestSlack {
				best, bestSlack, found = int32(e), sl, true
			}
		}
		if !found {
			panic("dagro: feasibleTree could not find an incident edge")
		}
		delta := rg.slack(best)
		if !t.in[rg.ev[best]] {
			delta = -delta
		}
		r.snapshot = t.nodes(r.snapshot)
		for _, v := range r.snapshot {
			rg.rank[v], rg.hasRank[v] = rg.rank[v]+delta, true
		}
	}
	r.ft = stack[:0]
	return t
}

// assignLowLim is dfsAssignLowLim from the tree root, recording parents,
// parent edges and the pre/post orders.
func (r *rankState) assignLowLim(t *rtree, root int32) {
	clear(t.visited)
	n := len(t.low)
	if cap(t.pre) < n {
		t.pre = make([]int32, n)
	}
	if cap(t.post) < n {
		t.post = make([]int32, n)
	}
	t.pre, t.post = t.pre[:n], t.post[:n]
	visited := r.lowLimDFS(t, root, inone, inone, 1, 0)
	t.pre, t.post = t.pre[:visited], t.post[:visited]
}

// assignLowLimSubtree redoes the traversal for the subtree of l only. It
// equals assignLowLim from the root when l's subtree has the same node set
// and l the same parent and parent edge as at the last traversal, and every
// tree change since lies inside it: the traversal outside the subtree then
// reads the same neighbor lists, so it enters l at the same point with the
// same counter (low[l]), the subtree takes the same span of the pre and post
// orders and of the low/lim numbers, and everything after it is unchanged.
// Nodes outside the subtree are reachable only through l's parent, which is
// marked visited, as it is when the full traversal enters l.
func (r *rankState) assignLowLimSubtree(t *rtree, l int32) {
	size := t.lim[l] - t.low[l] + 1
	preAt := t.preIdx[l]
	for _, v := range t.pre[preAt : preAt+size] {
		t.visited[v] = false
	}
	if p := t.parent[l]; p != inone {
		t.visited[p] = true
	}
	r.lowLimDFS(t, l, t.parent[l], t.parentEdge[l], t.low[l], preAt)
}

// lowLimDFS runs the traversal from start (whose parent and parent edge are
// given) with the counter at next, writing the pre order from position
// preAt and the post order from position next-1 (the post position of a node
// is its lim minus 1). It returns the number of nodes visited.
func (r *rankState) lowLimDFS(t *rtree, start, startParent, startEdge, next, preAt int32) int32 {
	stack := r.ll[:0]
	nbr := r.nbr[:0]
	first := preAt
	enter := func(v, parent, edge int32) {
		t.low[v] = next
		t.visited[v] = true
		t.pre[preAt] = v
		t.preIdx[v] = preAt
		preAt++
		t.parent[v], t.parentEdge[v] = parent, edge
		from := int32(len(nbr))
		for i := t.pHead[v]; i != inone; i = t.ent[i].next {
			nbr = append(nbr, i)
		}
		r.jsOrderEntries(t, nbr[from:])
		mid := int32(len(nbr))
		for i := t.sHead[v]; i != inone; i = t.ent[i].next {
			nbr = append(nbr, i)
		}
		r.jsOrderEntries(t, nbr[mid:])
		stack = append(stack, lowLimFrame{v: v, from: from, to: int32(len(nbr))})
	}
	enter(start, startParent, startEdge)
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.from == top.to {
			v := top.v
			t.lim[v] = next
			t.post[next-1] = v
			next++
			stack = stack[:len(stack)-1]
			if len(stack) > 0 {
				nbr = nbr[:stack[len(stack)-1].to]
			}
			continue
		}
		en := nbr[top.from]
		top.from++
		w := t.ent[en].node
		if !t.visited[w] {
			enter(w, top.v, t.ent[en].edge)
		}
	}
	r.ll, r.nbr = stack[:0], nbr[:0]
	return preAt - first
}

// jsOrderEntries orders neighbor entries by their node names' JavaScript
// property order, keeping insertion order among ordinary names.
func (r *rankState) jsOrderEntries(t *rtree, list []int32) {
	if len(list) < 2 {
		return
	}
	inPlace := true
	sawNonIndex := false
	count := 0
	var previous int64
	for _, i := range list {
		k := t.key(t.ent[i].node)
		if k < 0 {
			sawNonIndex = true
			continue
		}
		if sawNonIndex || count > 0 && k < previous {
			inPlace = false
		}
		previous = k
		count++
	}
	if inPlace {
		return
	}
	idx, rest := r.idx[:0], r.rest[:0]
	for _, i := range list {
		if t.key(t.ent[i].node) >= 0 {
			idx = append(idx, i)
		} else {
			rest = append(rest, i)
		}
	}
	// Array-index names are distinct: insertion sort by value is exact.
	for i := 1; i < len(idx); i++ {
		x := idx[i]
		kx := t.key(t.ent[x].node)
		j := i
		for j > 0 && t.key(t.ent[idx[j-1]].node) > kx {
			idx[j] = idx[j-1]
			j--
		}
		idx[j] = x
	}
	copy(list, idx)
	copy(list[len(idx):], rest)
	r.idx, r.rest = idx[:0], rest[:0]
}

// graphEdge returns the graph edge behind tree edge te as the Attrs engine
// finds it from the tree edge's endpoint v: the edge from v to the other
// endpoint when the graph has it (g.HasEdge(child, parent) in calcCutValue
// and updateRanks, g.HasEdge(edge.V, edge.W) in enterEdge), else the edge
// toward v. That is the edge te was created from, unless the graph has edges
// in both directions between the two nodes.
func (t *rtree) graphEdge(v, te int32) int32 {
	ge := t.eg[te]
	if t.opp != nil && t.rg.ev[ge] != v && t.opp[ge] != inone {
		return t.opp[ge]
	}
	return ge
}

func (t *rtree) calcCutValue(child int32) float64 {
	rg := t.rg
	parent := t.parent[child]
	ge := t.graphEdge(child, t.parentEdge[child])
	childIsTail := rg.ev[ge] == child
	cutValue := rg.weight[ge]
	visit := func(e int32) {
		isOutEdge := rg.ev[e] == child
		other := rg.ev[e]
		if isOutEdge {
			other = rg.ew[e]
		}
		if other == parent {
			return
		}
		pointsToHead := isOutEdge == childIsTail
		otherWeight := rg.weight[e]
		if pointsToHead {
			cutValue += otherWeight
		} else {
			cutValue -= otherWeight
		}
		if t.parent[other] == child {
			otherCutValue := t.cut[t.parentEdge[other]]
			if pointsToHead {
				cutValue -= otherCutValue
			} else {
				cutValue += otherCutValue
			}
		}
	}
	for i := rg.inStart[child]; i < rg.inStart[child+1]; i++ {
		visit(rg.inList[i])
	}
	for i := rg.outStart[child]; i < rg.outStart[child+1]; i++ {
		visit(rg.outList[i])
	}
	return cutValue
}

func (t *rtree) assignCutValues() {
	vs := t.post
	if len(vs) > 0 {
		vs = vs[:len(vs)-1]
	}
	for _, v := range vs {
		t.cut[t.parentEdge[v]] = t.calcCutValue(v)
	}
}

// assignCutValuesAfter is assignCutValues after an exchange whose changes lie
// inside the subtree of l (see assignLowLimSubtree). calcCutValue(v) reads
// v's graph edges, the parents of v and of its graph neighbors, and the cut
// values of v's children. Outside the subtree and l's ancestors none of these
// changed: a neighbor inside the subtree has its parent inside it or is l with
// parent l's parent, an ancestor. So those cut values are recomputed from the
// same inputs to the same bits and are kept; the subtree (in post order) and
// then l's ancestors, children before parents as in the post order, are
// recomputed by the same calls.
func (t *rtree) assignCutValuesAfter(l, root int32) {
	for _, v := range t.post[t.low[l]-1 : t.lim[l]] {
		if v != root {
			t.cut[t.parentEdge[v]] = t.calcCutValue(v)
		}
	}
	for v := t.parent[l]; v != inone && v != root; v = t.parent[v] {
		t.cut[t.parentEdge[v]] = t.calcCutValue(v)
	}
}

// rtreeCompactAlways makes every exchange compact the tree's edge list
// (tests only: compaction is storage management and must not change results).
var rtreeCompactAlways = false

// networkSimplex ports networkSimplex on the simplified graph rg.
func (r *rankState) networkSimplex(rg *rgraph, s *lstate) {
	r.longestPath(rg)
	t := r.feasibleTree(rg, s)
	if r.hasOpp {
		t.opp = r.opp
	}
	r.snapshot = t.nodes(r.snapshot)
	root := r.snapshot[0]
	r.assignLowLim(t, root)
	t.assignCutValues()
	ranksFromTree := false
	for {
		// leaveEdge: first tree edge with a negative cut value.
		leave := inone
		for i := range t.ea {
			if t.alive[i] && t.cut[i] < 0 {
				leave = int32(i)
				break
			}
		}
		if leave == inone {
			break
		}
		// enterEdge: the tree stores an edge with its JavaScript-smaller
		// endpoint first (ea), as Graph does for undirected edges.
		ge := t.graphEdge(t.ea[leave], leave)
		v, w := rg.ev[ge], rg.ew[ge]
		tail := v
		flip := false
		if t.lim[v] > t.lim[w] {
			tail = w
			flip = true
		}
		tailLow, tailLim := t.low[tail], t.lim[tail]
		best, bestSlack, found := int32(0), 0.0, false
		for e := range rg.ev {
			cv, cw := rg.ev[e], rg.ew[e]
			vDescendant := tailLow <= t.lim[cv] && t.lim[cv] <= tailLim
			wDescendant := tailLow <= t.lim[cw] && t.lim[cw] <= tailLim
			if flip != vDescendant || flip == wDescendant {
				continue
			}
			sl := rg.slack(int32(e))
			if !found || sl < bestSlack {
				best, bestSlack, found = int32(e), sl, true
			}
		}
		if !found {
			panic("dagro: networkSimplex could not find an entering edge")
		}
		// Every tree change of this exchange lies inside the subtree of l,
		// the lowest common ancestor of the leaving edge's upper endpoint
		// and the entering edge's endpoint outside tail's subtree: the
		// subtree of tail moves under that endpoint, so l keeps its node
		// set, its parent and its parent edge.
		outside := rg.ev[best]
		if tailLow <= t.lim[outside] && t.lim[outside] <= tailLim {
			outside = rg.ew[best]
		}
		l := t.parent[tail]
		for !(t.low[l] <= t.lim[outside] && t.lim[outside] <= t.lim[l]) {
			l = t.parent[l]
		}
		// exchangeEdges
		t.removeEdge(leave)
		t.addEdge(rg.ev[best], rg.ew[best], best)
		if len(t.ea) > 2*t.live+16 || rtreeCompactAlways && len(t.ea) > t.live {
			t.compact()
		}
		// = assignLowLim(t, root) and assignCutValues(), see there.
		r.assignLowLimSubtree(t, l)
		t.assignCutValuesAfter(l, root)
		// The ranks of the nodes outside l's subtree follow the same tree
		// path from the root as at the previous exchange, so recomputing
		// them gives the values already set; the first exchange recomputes
		// all ranks, which until then come from the feasible tree.
		pre := t.pre[1:]
		if ranksFromTree {
			pre = t.pre[t.preIdx[l]:][:t.lim[l]-t.low[l]+1]
			if l == root {
				pre = pre[1:]
			}
		}
		ranksFromTree = true
		for _, v := range pre {
			parent := t.parent[v]
			ge := t.graphEdge(v, t.parentEdge[v])
			minlen := rg.minlen[ge]
			parentRank := rg.rank[parent]
			if rg.ev[ge] != v {
				rg.rank[v] = parentRank + minlen
			} else {
				rg.rank[v] = parentRank - minlen
			}
			rg.hasRank[v] = true
		}
	}
}

package dagro

import (
	"math"
	"slices"
	"sort"
)

// layerGraph is the result of buildLayerGraphNodes in the form sortSubgraph
// reads it. Local index 0 is the generated root. The other local nodes are
// the bucket nodes, the parents the construction added and the sources of the
// merged edges. Labels are shared with the main graph except for bucket nodes
// with minRank, whose label the construction replaces by {borderLeft,
// borderRight} at this rank, and the root, whose label is nil.
type layerGraph struct {
	ids        []int32 // local -> global
	parent     []int32 // local parent, inone at the top level
	childStart []int32 // children per local node, JavaScript order, local indices
	childList  []int32
	replaced   []bool
	bl, br     []int32 // global border ids for replaced labels, inone when missing
	inStart    []int32 // incoming edges per local node, merged by source
	inU        []int32 // global source ids
	inW        []float64
	bytes      int // storageBytes as of the last buildLayerGraph
}

type typedOrderEntry struct {
	v          int32
	vs         []int32
	i          int
	barycenter float64
	weight     float64
	has        bool
}

type typedOrderResult struct {
	vs         []int32
	barycenter float64
	weight     float64
	has        bool
	usedBias   bool
}

type childRec struct{ parent, child int32 }

type inRec struct {
	target int32
	u      int32
	w      float64
}

type subRec struct {
	v      int32
	off, n int
}

type typedReversedPair struct {
	key   int32
	entry typedOrderEntry
}

// cnode is a conflict entry of resolveConflicts. Its vs list and its
// incoming and outgoing lists are array-linked: merging prepends the source's
// vs list in O(1), and incoming entries are pushed at the front, which visits
// them in the reverse of their arrival order exactly like the reversed slice
// of the Attrs engine.
type cnode struct {
	e                typedOrderEntry
	indegree         int
	merged           bool
	vsHead, vsTail   int32
	inHead           int32
	outHead, outTail int32
}

type poolEnt struct{ node, next int32 }

// sortLevel is the scratch storage of one sortSubgraph recursion depth.
type sortLevel struct {
	movable    []int32
	bary       []typedOrderEntry
	subs       []subRec
	subVS      []int32
	cn         []cnode
	vsNext     []int32
	inPool     []poolEnt
	outPool    []poolEnt
	order      []int32
	sourceSet  []int32
	processed  []int32
	resVS      []int32
	offs       []int
	resolved   []typedOrderEntry
	expVS      []int32
	pairs      []typedReversedPair
	sortable   []typedOrderEntry
	unsortable []typedOrderEntry
	insKeys    []int32
	insCounts  []int
	out        []int32
	final      []int32
	ss         sortableSorter
	us         unsortableSorter
	baryVals   []float64
	bytes      int // storageBytes as of the end of the last order that used the level
}

type ioFrame struct{ start, end, pos int }

type orderState struct {
	s       *lstate
	layers  []layerGraph
	nodes   []int32
	buckets [][]int32
	local   []int32 // global -> local index of the active layer graph
	stamp   []int32
	gen     int32
	cgEdges [][2]int32
	cgSeen  map[uint64]bool
	mapped  []int32 // global -> conflict entry index (valid while stamped)
	mstamp  []int32
	mgen    int32
	subIdx  []int32
	sstamp  []int32
	sgen    int32
	prevOf  []int32
	pstamp  []int32
	pgen    int32
	posOf   []int32
	posGen  []int32
	posg    int32
	tree    []float64
	sents   []southEntry
	levels  []*sortLevel

	layerBytes int // the sum of layers[:cap(layers)].bytes
	levelBytes int // the sum of levels[i].bytes
	depthUsed  int // the deepest level the current order used

	childRecs []childRec
	parents   []childRec
	inRecs    []inRec
	fill      []int32
	globals   []int32

	simple     []int32
	ordered    []int32
	visited    []bool
	frames     []ioFrame
	succStack  []int32
	init       [][]int32
	layering   [][]int32
	best       [][]int32
	sizes      []int
	rs         rankSorter
	rankStart  []int32
	rankSorted []int32

	skipWeightlessBarycenters bool
}

func (o *orderState) ensure(n int) {
	grow := func(p *[]int32, fill int32) {
		for len(*p) < n {
			*p = append(*p, fill)
		}
	}
	grow(&o.local, inone)
	grow(&o.stamp, 0)
	grow(&o.mapped, inone)
	grow(&o.mstamp, 0)
	grow(&o.subIdx, inone)
	grow(&o.sstamp, 0)
	grow(&o.prevOf, inone)
	grow(&o.pstamp, 0)
	grow(&o.posOf, 0)
	grow(&o.posGen, 0)
}

// reset prepares o for a new layout. Generation counters keep increasing
// across layouts, so stale stamps never match; they restart (with cleared
// arrays) only before they could overflow.
func (o *orderState) reset(s *lstate) {
	o.s = s
	if o.gen > 1<<30 || o.mgen > 1<<30 || o.sgen > 1<<30 || o.pgen > 1<<30 || o.posg > 1<<30 {
		clear(o.stamp)
		clear(o.mstamp)
		clear(o.sstamp)
		clear(o.pstamp)
		clear(o.posGen)
		o.gen, o.mgen, o.sgen, o.pgen, o.posg = 0, 0, 0, 0, 0
	}
	if o.cgSeen == nil {
		o.cgSeen = map[uint64]bool{}
	}
	o.layers = o.layers[:0]
}

func (o *orderState) level(depth int) *sortLevel {
	for len(o.levels) <= depth {
		o.levels = append(o.levels, &sortLevel{})
	}
	if depth > o.depthUsed {
		o.depthUsed = depth
	}
	return o.levels[depth]
}

func (s *lstate) createOrderRootNode() int32 {
	if s.lazyNodeNames {
		id := s.names.internLazy("_root", s.draw())
		s.ensureNode(id)
		return id
	}
	for {
		name := s.uniqueID("_root")
		if !s.nameInUse(name) {
			id := s.names.intern(name)
			s.ensureNode(id)
			return id
		}
	}
}

func (o *orderState) newLayer() *layerGraph {
	if len(o.layers) < cap(o.layers) {
		o.layers = o.layers[:len(o.layers)+1]
	} else {
		o.layers = append(o.layers, layerGraph{})
	}
	lg := &o.layers[len(o.layers)-1]
	lg.ids = lg.ids[:0]
	return lg
}

// buildLayerGraph ports buildLayerGraphNodes into o.layers[index].
func (o *orderState) buildLayerGraph(index, rank int, outEdges bool, bucket []int32) {
	s := o.s
	root := s.createOrderRootNode()
	o.ensure(len(s.nl))
	o.gen++
	lg := &o.layers[index]
	localOf := func(v int32) int32 {
		if o.stamp[v] == o.gen {
			return o.local[v]
		}
		o.stamp[v] = o.gen
		o.local[v] = int32(len(lg.ids))
		lg.ids = append(lg.ids, v)
		return o.local[v]
	}
	localOf(root)
	childRecs, parents, inRecs := o.childRecs[:0], o.parents[:0], o.inRecs[:0]
	for _, v := range bucket {
		lv := localOf(v)
		p := s.g.parentOf(v)
		if p == inone {
			p = root
		}
		lp := localOf(p)
		childRecs = append(childRecs, childRec{lp, lv})
		parents = append(parents, childRec{lp, lv})
		start := len(inRecs)
		var e int32
		if outEdges {
			e = s.g.nodes[v].outHead
		} else {
			e = s.g.nodes[v].inHead
		}
		for e != inone {
			ed := &s.g.edges[e]
			u := ed.v
			if u == v {
				u = ed.w
			}
			weight := s.el[ed.label].weight
			merged := false
			for i := start; i < len(inRecs); i++ {
				if inRecs[i].u == u {
					inRecs[i].w = inRecs[i].w + weight
					merged = true
					break
				}
			}
			if !merged {
				localOf(u)
				inRecs = append(inRecs, inRec{target: lv, u: u, w: 0 + weight})
			}
			if outEdges {
				e = ed.outNext
			} else {
				e = ed.inNext
			}
		}
	}
	o.childRecs, o.parents, o.inRecs = childRecs, parents, inRecs
	n := len(lg.ids)
	lg.parent = grow(lg.parent, n)
	for i := range lg.parent {
		lg.parent[i] = inone
	}
	for _, r := range parents {
		lg.parent[r.child] = r.parent
	}
	lg.replaced = resize(lg.replaced, n)
	lg.bl = grow(lg.bl, n)
	lg.br = grow(lg.br, n)
	for i := range lg.bl {
		lg.bl[i], lg.br[i] = inone, inone
	}
	for _, v := range bucket {
		lv := o.local[v]
		if n := &s.nl[v]; n.hasMinMax {
			lg.replaced[lv] = true
			lg.bl[lv] = n.borderAt(btLeft, rank)
			lg.br[lv] = n.borderAt(btRight, rank)
		}
	}
	lg.childStart = resize(lg.childStart, n+1)
	for _, r := range childRecs {
		lg.childStart[r.parent+1]++
	}
	for i := 0; i < n; i++ {
		lg.childStart[i+1] += lg.childStart[i]
	}
	lg.childList = grow(lg.childList, len(childRecs))
	o.fill = append(o.fill[:0], lg.childStart[:n]...)
	for _, r := range childRecs {
		lg.childList[o.fill[r.parent]] = r.child
		o.fill[r.parent]++
	}
	key := s.key()
	for i := 0; i < n; i++ {
		list := lg.childList[lg.childStart[i]:lg.childStart[i+1]]
		o.jsOrderLocal(lg, list, key)
	}
	lg.inStart = resize(lg.inStart, n+1)
	for _, r := range inRecs {
		lg.inStart[r.target+1]++
	}
	for i := 0; i < n; i++ {
		lg.inStart[i+1] += lg.inStart[i]
	}
	lg.inU = grow(lg.inU, len(inRecs))
	lg.inW = grow(lg.inW, len(inRecs))
	o.fill = append(o.fill[:0], lg.inStart[:n]...)
	for _, r := range inRecs {
		lg.inU[o.fill[r.target]] = r.u
		lg.inW[o.fill[r.target]] = r.w
		o.fill[r.target]++
	}
	b := lg.storageBytes()
	o.layerBytes += b - lg.bytes
	lg.bytes = b
}

// jsOrderLocal orders local indices by the JavaScript order of their names.
func (o *orderState) jsOrderLocal(lg *layerGraph, list []int32, key []int64) {
	if len(list) < 2 {
		return
	}
	globals := o.globals[:0]
	for _, l := range list {
		globals = append(globals, lg.ids[l])
	}
	jsOrderIDs(globals, key)
	for i, g := range globals {
		list[i] = o.local[g]
	}
	o.globals = globals[:0]
}

// firstPred is g.Predecessors(v)[0] in lg: the first predecessor in
// JavaScript order (the smallest array-index name, otherwise the first one
// inserted).
func (o *orderState) firstPred(lg *layerGraph, v int32, key []int64) (int32, bool) {
	l := o.localIn(lg, v)
	if l == inone {
		return inone, false
	}
	list := lg.inU[lg.inStart[l]:lg.inStart[l+1]]
	if len(list) == 0 {
		return inone, false
	}
	best, bestKey := inone, int64(-1)
	for _, u := range list {
		if k := key[u]; k >= 0 && (bestKey < 0 || k < bestKey) {
			best, bestKey = u, k
		}
	}
	if best != inone {
		return best, true
	}
	return list[0], true
}

// activate makes localIn answer for lg.
func (o *orderState) activate(lg *layerGraph) {
	o.gen++
	for i, id := range lg.ids {
		o.local[id], o.stamp[id] = int32(i), o.gen
	}
}

// localIn is the local index of global v in the active layer graph, or inone
// when v is not a node of it.
func (o *orderState) localIn(lg *layerGraph, v int32) int32 {
	if v >= 0 && int(v) < len(o.stamp) && o.stamp[v] == o.gen {
		return o.local[v]
	}
	return inone
}

// maxTypedRank bounds the ranks the typed order accepts (see order).
const maxTypedRank = 1 << 26

func (s *lstate) order() {
	s.orderWithRankConstraints(false)
}

func (s *lstate) orderWithRankConstraints(skipWeightlessBarycenters bool) {
	o := &s.o
	o.reset(s)
	o.skipWeightlessBarycenters = skipWeightlessBarycenters
	o.nodes = append(o.nodes[:0], s.nodes()...)
	nodes := o.nodes
	maxRank := -1
	for _, v := range nodes {
		if n := &s.nl[v]; n.hasRank {
			if r := int(n.rank); r > maxRank {
				maxRank = r
			}
		}
	}
	// The Attrs engine's order starts with make([]int, 0, maxRank) (the same
	// maxRank: the ranks of the same nodes, converted as integer does). When
	// no node has a rank (an empty graph) that panics with Go's "makeslice:
	// cap out of range", and the typed engine makes the same call. For huge
	// values it fails or runs out of memory (an infinite rank converts to the
	// largest int): the typed engine declines, so that the Attrs engine fails
	// as the original does; no layout has that many ranks.
	if maxRank < 0 {
		s.origPanic = true
		_ = makeRankList(maxRank)
	}
	if maxRank > maxTypedRank {
		panic(errDecline)
	}
	o.depthUsed = -1
	before := 0
	for i := range o.buckets {
		if i <= maxRank {
			before += cap(o.buckets[i])
		}
		o.buckets[i] = o.buckets[i][:0]
	}
	if maxRank >= 0 {
		for len(o.buckets) < maxRank+1 {
			o.buckets = append(o.buckets, nil)
		}
		buckets := o.buckets
		for _, v := range nodes {
			n := &s.nl[v]
			nodeRank, hasRank := 0, n.hasRank
			if hasRank {
				nodeRank = int(n.rank)
				if 0 <= nodeRank && nodeRank <= maxRank {
					buckets[nodeRank] = append(buckets[nodeRank], v)
				}
			}
			if !n.hasMinMax {
				continue
			}
			minRank, maxNodeRank := int(n.minRank), int(n.maxRank)
			if minRank < 0 {
				minRank = 0
			}
			if maxNodeRank > maxRank {
				maxNodeRank = maxRank
			}
			for rank := minRank; rank <= maxNodeRank; rank++ {
				if hasRank && rank == nodeRank {
					continue
				}
				buckets[rank] = append(buckets[rank], v)
			}
		}
		s.rankBytes += 4 * (capsLayers(buckets[:maxRank+1]) - before)
	}
	bucketAt := func(rank int) []int32 {
		if 0 <= rank && rank <= maxRank {
			return o.buckets[rank]
		}
		return nil
	}
	downCount := 0
	for rank := 1; rank <= maxRank; rank++ {
		o.newLayer()
		o.buildLayerGraph(downCount, rank, false, bucketAt(rank))
		downCount++
	}
	for rank := maxRank - 1; rank >= 0; rank-- {
		o.newLayer()
		o.buildLayerGraph(len(o.layers)-1, rank, true, bucketAt(rank))
	}
	o.ensure(len(s.nl))

	s.assignOrder(o.initOrder(nodes))

	bestCC := math.Inf(1)
	haveBest := false
	for i, lastBest := 0, 0; lastBest < 4; i, lastBest = i+1, lastBest+1 {
		if i%2 != 0 {
			o.sweep(0, downCount, i%4 >= 2)
		} else {
			o.sweep(downCount, len(o.layers), i%4 >= 2)
		}
		layering := o.layerMatrix(nodes, maxRank)
		cc := o.crossCount(layering)
		if cc < bestCC {
			lastBest = 0
			o.best = copyIDLayering(o.best, layering, &s.rankBytes)
			haveBest = true
			bestCC = cc
		} else if cc == bestCC {
			o.best = copyIDLayering(o.best, layering, &s.rankBytes)
			haveBest = true
		}
	}
	if haveBest {
		s.assignOrder(o.best)
	}
	for _, L := range o.levels[:o.depthUsed+1] {
		b := L.storageBytes()
		o.levelBytes += b - L.bytes
		L.bytes = b
	}
}

// copyIDLayering copies src into dst's storage (cloneLayering) and adds the
// growth of dst's inner storage, in bytes, to *bytes.
func copyIDLayering(dst, src [][]int32, bytes *int) [][]int32 {
	// Extend within the capacity first: the layers past len(dst) keep the
	// storage of an earlier, longer layering.
	if len(src) <= cap(dst) {
		dst = dst[:len(src)]
	}
	for len(dst) < len(src) {
		dst = append(dst, nil)
	}
	dst = dst[:len(src)]
	for i := range src {
		old := cap(dst[i])
		dst[i] = append(dst[i][:0], src[i]...)
		*bytes += 4 * (cap(dst[i]) - old)
	}
	return dst
}

// assignOrder is assignOrder: asAttrs(g.Node(v))["order"] = i for every entry
// of the layering, in order. An entry the layer matrix left empty ("" in the
// original) has no node: the original writes into its nil label there.
func (s *lstate) assignOrder(layering [][]int32) {
	for _, layer := range layering {
		for i, v := range layer {
			if v == inone {
				s.failNilMapWrite("order", float64(i))
			}
			s.nl[v].order, s.nl[v].hasOrder = float64(i), true
		}
	}
}

// makeRankList is the original order's first allocation, make([]int, 0,
// maxRank); order calls it only for a negative maxRank, where it fails with
// the original's runtime error.
//
//go:noinline
func makeRankList(maxRank int) []int {
	return make([]int, 0, maxRank)
}

type rankSorter struct {
	vs []int32
	nl []nlabel
}

func (r *rankSorter) Len() int           { return len(r.vs) }
func (r *rankSorter) Less(i, j int) bool { return r.nl[r.vs[i]].rank < r.nl[r.vs[j]].rank }
func (r *rankSorter) Swap(i, j int)      { r.vs[i], r.vs[j] = r.vs[j], r.vs[i] }

// countingSortByRank stably sorts vs by rank when every rank is an integer
// in [0, 2*len(vs)+64) and reports whether it did. With no NaN among the
// ranks, < is a strict weak order, whose stable sort has a single result:
// the one sort.Stable(&rankSorter{...}) computes.
func (o *orderState) countingSortByRank(vs []int32) bool {
	nl := o.s.nl
	limit := 2*len(vs) + 64
	maxRank := 0
	for _, v := range vs {
		r := nl[v].rank
		if !(r >= 0 && r < float64(limit)) || r != math.Trunc(r) {
			return false
		}
		maxRank = max(maxRank, int(r))
	}
	start := resize(o.rankStart, maxRank+2)
	for _, v := range vs {
		start[int(nl[v].rank)+1]++
	}
	for r := 1; r < len(start); r++ {
		start[r] += start[r-1]
	}
	sorted := grow(o.rankSorted, len(vs))
	for _, v := range vs {
		r := int(nl[v].rank)
		sorted[start[r]] = v
		start[r]++
	}
	copy(vs, sorted)
	o.rankStart, o.rankSorted = start, sorted
	return true
}

// initOrder ports initOrderNodes.
func (o *orderState) initOrder(nodes []int32) [][]int32 {
	s := o.s
	simple := o.simple[:0]
	for _, v := range nodes {
		if !s.g.hasChildren(v) {
			simple = append(simple, v)
		}
	}
	o.simple = simple
	if len(simple) == 0 {
		return nil
	}
	maxRank := -1
	for _, v := range simple {
		if r := int(s.nl[v].rank); r > maxRank {
			maxRank = r
		}
	}
	for len(o.init) < maxRank+1 {
		o.init = append(o.init, nil)
	}
	layers := o.init[:maxRank+1]
	before := 0
	for i := range layers {
		before += cap(layers[i])
		layers[i] = layers[i][:0]
	}
	o.visited = resize(o.visited, len(s.g.nodes))
	visited := o.visited
	ordered := append(o.ordered[:0], simple...)
	o.ordered = ordered
	if !o.countingSortByRank(ordered) {
		o.rs.vs, o.rs.nl = ordered, s.nl
		sort.Stable(&o.rs)
		o.rs.vs, o.rs.nl = nil, nil
	}
	key := s.key()
	stack := o.frames[:0]
	succ := o.succStack[:0]
	visit := func(v int32) {
		visited[v] = true
		r := int(s.nl[v].rank)
		layers[r] = append(layers[r], v)
		start := len(succ)
		succ = s.g.successors(v, key, succ)
		stack = append(stack, ioFrame{start: start, end: len(succ), pos: start})
	}
	for _, v := range ordered {
		if visited[v] {
			continue
		}
		visit(v)
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.pos == top.end {
				succ = succ[:top.start]
				stack = stack[:len(stack)-1]
				continue
			}
			w := succ[top.pos]
			top.pos++
			if !visited[w] {
				visit(w)
			}
		}
	}
	o.frames, o.succStack = stack[:0], succ[:0]
	s.rankBytes += 4 * (capsLayers(layers) - before)
	return layers
}

// layerMatrix ports orderBuildLayerMatrixNodes; gaps stay inone.
func (o *orderState) layerMatrix(nodes []int32, maxRank int) [][]int32 {
	s := o.s
	if maxRank < 0 {
		return nil
	}
	for len(o.layering) < maxRank+1 {
		o.layering = append(o.layering, nil)
	}
	layering := o.layering[:maxRank+1]
	o.sizes = resize(o.sizes, maxRank+1)
	sizes := o.sizes
	for _, v := range nodes {
		n := &s.nl[v]
		if !n.hasRank || !n.hasOrder {
			continue
		}
		rank, position := int(n.rank), int(n.order)
		if rank < 0 || rank >= len(layering) || position < 0 {
			continue
		}
		if position >= sizes[rank] {
			sizes[rank] = position + 1
		}
	}
	grown := 0
	for rank, size := range sizes {
		old := cap(layering[rank])
		layer := grow(layering[rank], size)
		for i := range layer {
			layer[i] = inone
		}
		layering[rank] = layer
		grown += cap(layer) - old
	}
	s.rankBytes += 4 * grown
	for _, v := range nodes {
		n := &s.nl[v]
		if !n.hasRank || !n.hasOrder {
			continue
		}
		rank, position := int(n.rank), int(n.order)
		if rank < 0 || rank >= len(layering) || position < 0 {
			continue
		}
		layering[rank][position] = v
	}
	return layering
}

func (o *orderState) crossCount(layering [][]int32) float64 {
	cc := 0.0
	for i := 1; i < len(layering); i++ {
		cc += o.twoLayerCrossCount(layering[i-1], layering[i])
	}
	return cc
}

func (o *orderState) twoLayerCrossCount(north, south []int32) float64 {
	s := o.s
	o.posg++
	for i, v := range south {
		if v != inone {
			o.posOf[v], o.posGen[v] = int32(i), o.posg
		}
	}
	ents := o.sents[:0]
	for _, v := range north {
		if v == inone {
			continue
		}
		start := len(ents)
		for e := s.g.nodes[v].outHead; e != inone; e = s.g.edges[e].outNext {
			ed := &s.g.edges[e]
			pos := 0
			if o.posGen[ed.w] == o.posg {
				pos = int(o.posOf[ed.w])
			}
			ents = append(ents, southEntry{pos: pos, weight: s.el[ed.label].weight})
		}
		// stable insertion sort by position (a total order on ints)
		part := ents[start:]
		for i := 1; i < len(part); i++ {
			for j := i; j > 0 && part[j].pos < part[j-1].pos; j-- {
				part[j], part[j-1] = part[j-1], part[j]
			}
		}
	}
	o.sents = ents
	firstIndex := 1
	for firstIndex < len(south) {
		firstIndex <<= 1
	}
	treeSize := 2*firstIndex - 1
	firstIndex--
	if cap(o.tree) < treeSize {
		o.tree = make([]float64, treeSize)
	}
	tree := o.tree[:treeSize]
	clear(tree)
	cc := 0.0
	for _, entry := range ents {
		index := entry.pos + firstIndex
		tree[index] += entry.weight
		weightSum := 0.0
		for index > 0 {
			if index%2 != 0 {
				weightSum += tree[index+1]
			}
			index = (index - 1) >> 1
			tree[index] += entry.weight
		}
		cc += entry.weight * weightSum
	}
	return cc
}

// sweep is sweepLayerGraphs over o.layers[from:to].
func (o *orderState) sweep(from, to int, switchBias bool) {
	s := o.s
	biasRight := true
	o.cgEdges = o.cgEdges[:0]
	clear(o.cgSeen)
	for li := from; li < to; li++ {
		lg := &o.layers[li]
		o.activate(lg)
		sorted := o.sortSubgraph(lg, 0, biasRight, 0)
		if switchBias && sorted.usedBias {
			biasRight = !biasRight
		}
		for i, v := range sorted.vs {
			if v == inone {
				panic("dagro: missing node in sorted layer")
			}
			l := o.localIn(lg, v)
			if l == inone || l == 0 {
				// The Attrs engine writes into a nil label here and panics.
				panic("dagro: order assigned to a node without a layer label")
			}
			if lg.replaced[l] {
				continue
			}
			s.nl[v].order, s.nl[v].hasOrder = float64(i), true
		}
		o.addSubgraphConstraints(lg, sorted.vs)
	}
}

// parentIn is lg.Parent(v): the main parent or the root for bucket nodes,
// none for the root and for parents outside the bucket.
func (o *orderState) parentIn(lg *layerGraph, v int32) (int32, bool) {
	l := o.localIn(lg, v)
	if l == inone || lg.parent[l] == inone {
		return inone, false
	}
	return lg.ids[lg.parent[l]], true
}

func (o *orderState) addSubgraphConstraints(lg *layerGraph, vs []int32) {
	o.pgen++
	rootPrev := inone
	prevGet := func(p int32) int32 {
		if o.pstamp[p] == o.pgen {
			return o.prevOf[p]
		}
		return inone
	}
	for _, v := range vs {
		child, ok := o.parentIn(lg, v)
		for ok && child != inone {
			parent, hasParent := o.parentIn(lg, child)
			prevChild := inone
			if hasParent && parent != inone {
				prevChild = prevGet(parent)
				o.prevOf[parent], o.pstamp[parent] = child, o.pgen
			} else {
				prevChild = rootPrev
				rootPrev = child
			}
			if prevChild != inone && prevChild != child {
				k := uint64(uint32(prevChild))<<32 | uint64(uint32(child))
				if !o.cgSeen[k] {
					o.cgSeen[k] = true
					o.cgEdges = append(o.cgEdges, [2]int32{prevChild, child})
				}
				break
			}
			child, ok = parent, hasParent
		}
	}
}

// sortSubgraph ports sortSubgraph. The result's vs lives in the scratch of
// this recursion depth and is valid until the next call at the same depth.
func (o *orderState) sortSubgraph(lg *layerGraph, lv int32, biasRight bool, depth int) typedOrderResult {
	s := o.s
	key := s.key()
	L := o.level(depth)
	o.sgen++
	myGen := o.sgen

	movable := L.movable[:0]
	for _, c := range lg.childList[lg.childStart[lv]:lg.childStart[lv+1]] {
		movable = append(movable, lg.ids[c])
	}
	bl, br := inone, inone
	if lg.replaced[lv] {
		bl, br = lg.bl[lv], lg.br[lv]
	}
	if bl != inone {
		filtered := movable[:0]
		for _, w := range movable {
			if w != bl && w != br {
				filtered = append(filtered, w)
			}
		}
		movable = filtered
	}
	L.movable = movable

	bary := L.bary[:0]
	for _, v := range movable {
		entry := typedOrderEntry{v: v}
		l := o.localIn(lg, v)
		if l != inone && lg.inStart[l] != lg.inStart[l+1] {
			sum, weight := 0.0, 0.0
			for i := lg.inStart[l]; i < lg.inStart[l+1]; i++ {
				edgeWeight := lg.inW[i]
				sum += edgeWeight * o.labelOrder(lg, lg.inU[i])
				weight += edgeWeight
			}
			o.setBarycenter(&entry, sum, weight)
		}
		bary = append(bary, entry)
	}
	L.bary = bary

	L.subs, L.subVS = L.subs[:0], L.subVS[:0]
	for i := range L.bary {
		entry := &L.bary[i]
		l := o.localIn(lg, entry.v)
		if l != inone && lg.childStart[l] != lg.childStart[l+1] {
			sub := o.sortSubgraph(lg, l, biasRight, depth+1)
			off := len(L.subVS)
			L.subVS = append(L.subVS, sub.vs...)
			L.subs = append(L.subs, subRec{v: entry.v, off: off, n: len(sub.vs)})
			o.subIdx[entry.v], o.sstamp[entry.v] = int32(len(L.subs)-1), myGen
			if sub.has {
				if entry.has {
					entry.barycenter = (entry.barycenter*entry.weight + sub.barycenter*sub.weight) / (entry.weight + sub.weight)
					entry.weight += sub.weight
				} else {
					entry.barycenter = sub.barycenter
					entry.weight = sub.weight
					entry.has = true
				}
			}
		}
	}

	entries := o.resolveConflicts(L, L.bary)
	// expandSubgraphs
	L.expVS = L.expVS[:0]
	L.offs = L.offs[:0]
	for i := range entries {
		off := len(L.expVS)
		for _, v := range entries[i].vs {
			if o.sstamp[v] == myGen {
				sr := L.subs[o.subIdx[v]]
				L.expVS = append(L.expVS, L.subVS[sr.off:sr.off+sr.n]...)
			} else {
				L.expVS = append(L.expVS, v)
			}
		}
		L.offs = append(L.offs, off, len(L.expVS)-off)
	}
	for i := range entries {
		off, n := L.offs[2*i], L.offs[2*i+1]
		entries[i].vs = L.expVS[off : off+n : off+n]
	}

	reversedPairs := L.pairs[:0]
	usedBias := false
	// The loop below acts only on two entries with equal truthy
	// barycenters; without such a pair it does nothing, so long lists, whose
	// pairwise scan is quadratic, are checked first in n log n.
	if len(entries) <= equalBaryScanMax || hasEqualTruthyBarycenters(L, entries) {
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if !entries[i].has || !entries[j].has ||
					!jsTruthyNumber(entries[i].barycenter) || !jsTruthyNumber(entries[j].barycenter) ||
					entries[i].barycenter != entries[j].barycenter {
					continue
				}
				nameI, nameJ := firstTypedNode(entries[i]), firstTypedNode(entries[j])
				edgeI, okI, dummyI, reversedI := o.pairInfo(lg, nameI)
				edgeJ, okJ, dummyJ, reversedJ := o.pairInfo(lg, nameJ)
				isReversedPair := dummyI == dkEdge && dummyJ == dkEdge && okI && okJ &&
					edgeI.v == edgeJ.v && edgeI.w == edgeJ.w && reversedI != reversedJ
				if !isReversedPair {
					usedBias = true
					continue
				}
				if reversedI {
					reversedPairs = append(reversedPairs, typedReversedPair{key: nameJ, entry: entries[i]})
					entries = append(entries[:i], entries[i+1:]...)
					i--
					break
				}
				reversedPairs = append(reversedPairs, typedReversedPair{key: nameI, entry: entries[j]})
				entries = append(entries[:j], entries[j+1:]...)
				j--
			}
		}
	}
	L.pairs = reversedPairs

	result := sortTypedOrderEntries(L, entries, reversedPairs, biasRight)
	result.usedBias = usedBias

	if bl != inone {
		final := append(L.final[:0], bl)
		final = append(final, result.vs...)
		final = append(final, br)
		L.final = final
		result.vs = final
		if blPred, ok := o.firstPred(lg, bl, key); ok {
			brPred, ok := o.firstPred(lg, br, key)
			if !ok {
				// g.Predecessors(br)[0] on an empty list: the Attrs engine panics.
				panic("dagro: border node without predecessor")
			}
			blOrder := o.labelOrder(lg, blPred)
			brOrder := o.labelOrder(lg, brPred)
			if !result.has {
				result.barycenter = 0
				result.weight = 0
				result.has = true
			}
			result.barycenter = (result.barycenter*result.weight + blOrder + brOrder) / (result.weight + 2)
			result.weight += 2
		}
	}
	return result
}

// equalBaryScanMax is the longest entry list sortSubgraph scans pairwise
// without checking first for equal barycenters.
const equalBaryScanMax = 12

// hasEqualTruthyBarycenters reports whether two of the entries that have a
// barycenter have the same truthy (non-zero, non-NaN) one, which is when
// sortSubgraph's pairwise scan acts.
func hasEqualTruthyBarycenters(L *sortLevel, entries []typedOrderEntry) bool {
	vals := L.baryVals[:0]
	for i := range entries {
		if entries[i].has && jsTruthyNumber(entries[i].barycenter) {
			vals = append(vals, entries[i].barycenter)
		}
	}
	L.baryVals = vals
	if len(vals) < 2 {
		return false
	}
	// No NaN is left, so the order is total and equal values end up next to
	// each other.
	slices.Sort(vals)
	for k := 1; k < len(vals); k++ {
		if vals[k] == vals[k-1] {
			return true
		}
	}
	return false
}

// labelOrder is num(lg.Node(v), "order"): NaN for a node outside lg, for
// the root (nil label) and for replaced labels, which never get an order.
func (o *orderState) labelOrder(lg *layerGraph, v int32) float64 {
	l := o.localIn(lg, v)
	if l == inone || l == 0 || lg.replaced[l] {
		return math.NaN()
	}
	return o.s.nl[v].order
}

// pairInfo reads edgeObj, dummy and edgeLabel.reversed of v's label in lg.
func (o *orderState) pairInfo(lg *layerGraph, v int32) (edgeRef, bool, dummyKind, bool) {
	l := o.localIn(lg, v)
	if l == inone || l == 0 || lg.replaced[l] {
		return edgeRef{}, false, dkNone, false
	}
	n := &o.s.nl[v]
	reversed := false
	if n.edgeLabel != inone {
		reversed = o.s.el[n.edgeLabel].reversed
	}
	return n.edgeObj, n.hasEdgeObj, n.dummy, reversed
}

func firstTypedNode(e typedOrderEntry) int32 {
	if len(e.vs) == 0 {
		return inone
	}
	return e.vs[0]
}

func (o *orderState) resolveConflicts(L *sortLevel, entries []typedOrderEntry) []typedOrderEntry {
	key := o.s.key()
	o.mgen++
	L.cn = grow(L.cn, len(entries))
	L.vsNext = grow(L.vsNext, len(entries))
	L.inPool, L.outPool = L.inPool[:0], L.outPool[:0]
	order := L.order[:0]
	for i, entry := range entries {
		L.cn[i] = cnode{
			e:      typedOrderEntry{v: entry.v, i: i, barycenter: entry.barycenter, weight: entry.weight, has: entry.has},
			vsHead: int32(i), vsTail: int32(i), inHead: inone, outHead: inone, outTail: inone,
		}
		L.vsNext[i] = inone
		o.mapped[entry.v], o.mstamp[entry.v] = int32(i), o.mgen
		order = append(order, entry.v)
	}
	L.order = order
	lookup := func(v int32) int32 {
		if v >= 0 && int(v) < len(o.mstamp) && o.mstamp[v] == o.mgen {
			return o.mapped[v]
		}
		return inone
	}
	for _, e := range o.cgEdges {
		iv, iw := lookup(e[0]), lookup(e[1])
		if iv != inone && iw != inone {
			L.cn[iw].indegree++
			k := int32(len(L.outPool))
			L.outPool = append(L.outPool, poolEnt{node: iw, next: inone})
			if L.cn[iv].outTail != inone {
				L.outPool[L.cn[iv].outTail].next = k
			} else {
				L.cn[iv].outHead = k
			}
			L.cn[iv].outTail = k
		}
	}
	jsOrderIDs(order, key)
	sourceSet := L.sourceSet[:0]
	for _, v := range order {
		i := lookup(v)
		if L.cn[i].indegree == 0 {
			sourceSet = append(sourceSet, i)
		}
	}
	processed := L.processed[:0]
	for len(sourceSet) > 0 {
		i := sourceSet[len(sourceSet)-1]
		sourceSet = sourceSet[:len(sourceSet)-1]
		processed = append(processed, i)
		entry := &L.cn[i]
		for k := entry.inHead; k != inone; k = L.inPool[k].next {
			incoming := &L.cn[L.inPool[k].node]
			if incoming.merged {
				continue
			}
			if !incoming.e.has || !entry.e.has || incoming.e.barycenter >= entry.e.barycenter {
				o.mergeEntries(L, entry, incoming)
			}
		}
		for k := entry.outHead; k != inone; k = L.outPool[k].next {
			j := L.outPool[k].node
			outgoing := &L.cn[j]
			p := int32(len(L.inPool))
			L.inPool = append(L.inPool, poolEnt{node: i, next: outgoing.inHead})
			outgoing.inHead = p
			outgoing.indegree--
			if outgoing.indegree == 0 {
				sourceSet = append(sourceSet, j)
			}
		}
	}
	L.sourceSet, L.processed = sourceSet, processed
	L.resVS = L.resVS[:0]
	L.offs = L.offs[:0]
	for _, i := range processed {
		entry := &L.cn[i]
		if entry.merged {
			continue
		}
		off := len(L.resVS)
		for el := entry.vsHead; el != inone; el = L.vsNext[el] {
			L.resVS = append(L.resVS, L.cn[el].e.v)
		}
		L.offs = append(L.offs, off, len(L.resVS)-off)
	}
	result := L.resolved[:0]
	k := 0
	for _, i := range processed {
		entry := &L.cn[i]
		if entry.merged {
			continue
		}
		off, n := L.offs[2*k], L.offs[2*k+1]
		k++
		result = append(result, typedOrderEntry{
			v: inone, vs: L.resVS[off : off+n : off+n], i: entry.e.i,
			barycenter: entry.e.barycenter, weight: entry.e.weight, has: entry.e.has,
		})
	}
	L.resolved = result
	return result
}

// With rank constraints, a zero-weight neighborhood expresses no ordering
// preference. Treat it like a node with no incident edges. A NaN here can
// invalidate the ordering of compound siblings in later sweeps.
func (o *orderState) setBarycenter(entry *typedOrderEntry, sum, weight float64) {
	entry.weight = weight
	entry.has = weight != 0 || !o.skipWeightlessBarycenters
	if entry.has {
		entry.barycenter = sum / weight
	}
}

func (o *orderState) mergeEntries(L *sortLevel, target, source *cnode) {
	sum := 0.0
	weight := 0.0
	if jsTruthyNumber(target.e.weight) {
		sum += target.e.barycenter * target.e.weight
		weight += target.e.weight
	}
	if jsTruthyNumber(source.e.weight) {
		sum += source.e.barycenter * source.e.weight
		weight += source.e.weight
	}
	// target.vs = source.vs ++ target.vs
	L.vsNext[source.vsTail] = target.vsHead
	target.vsHead = source.vsHead
	o.setBarycenter(&target.e, sum, weight)
	if source.e.i < target.e.i {
		target.e.i = source.e.i
	}
	source.merged = true
}

// smallEntrySort is the length up to which sort.Stable is one insertion sort.
const smallEntrySort = 20

type unsortableSorter struct{ entries []typedOrderEntry }

func (u *unsortableSorter) Len() int           { return len(u.entries) }
func (u *unsortableSorter) Less(i, j int) bool { return -u.entries[i].i < -u.entries[j].i }
func (u *unsortableSorter) Swap(i, j int)      { u.entries[i], u.entries[j] = u.entries[j], u.entries[i] }

type sortableSorter struct {
	entries   []typedOrderEntry
	biasRight bool
}

func (s *sortableSorter) Len() int { return len(s.entries) }
func (s *sortableSorter) Less(i, j int) bool {
	entryV, entryW := &s.entries[i], &s.entries[j]
	if entryV.barycenter < entryW.barycenter {
		return true
	}
	if entryV.barycenter > entryW.barycenter {
		return false
	}
	if !s.biasRight {
		return entryV.i < entryW.i
	}
	return entryV.i > entryW.i
}
func (s *sortableSorter) Swap(i, j int) { s.entries[i], s.entries[j] = s.entries[j], s.entries[i] }

// sortTypedOrderEntries ports sortOrderEntriesWithReversedPairs; it uses the
// same stable sort algorithm as sort.SliceStable with the same comparators.
func sortTypedOrderEntries(L *sortLevel, entries []typedOrderEntry, reversedPairs []typedReversedPair, biasRight bool) typedOrderResult {
	sortable := L.sortable[:0]
	unsortable := L.unsortable[:0]
	for _, entry := range entries {
		if entry.has {
			sortable = append(sortable, entry)
		} else {
			unsortable = append(unsortable, entry)
		}
	}
	// sort.Stable sorts up to smallEntrySort elements with exactly this
	// insertion sort (insertionSort(data, 0, n): no block merge follows), so
	// for short lists (most layers) the same comparisons and swaps run here
	// with the comparators inlined, NaN barycenters included.
	if len(unsortable) <= smallEntrySort {
		for i := 1; i < len(unsortable); i++ {
			// unsortableSorter.Less(j, j-1): -i_j < -i_{j-1}.
			for j := i; j > 0 && -unsortable[j].i < -unsortable[j-1].i; j-- {
				unsortable[j], unsortable[j-1] = unsortable[j-1], unsortable[j]
			}
		}
	} else {
		L.us.entries = unsortable
		sort.Stable(&L.us)
		L.us.entries = nil
	}
	if len(sortable) > smallEntrySort {
		L.ss.entries, L.ss.biasRight = sortable, biasRight
		sort.Stable(&L.ss)
		L.ss.entries = nil
	} else {
		for i := 1; i < len(sortable); i++ {
			for j := i; j > 0; j-- {
				// sortableSorter.Less(j, j-1)
				a, b := &sortable[j], &sortable[j-1]
				less := a.barycenter < b.barycenter
				if !less && !(a.barycenter > b.barycenter) {
					if biasRight {
						less = a.i > b.i
					} else {
						less = a.i < b.i
					}
				}
				if !less {
					break
				}
				sortable[j], sortable[j-1] = sortable[j-1], sortable[j]
			}
		}
	}

	insKeys, insCounts := L.insKeys[:0], L.insCounts[:0]
	for _, pair := range reversedPairs {
		keyIndex := -1
		for i := range sortable {
			if len(sortable[i].vs) != 0 && sortable[i].vs[0] == pair.key {
				keyIndex = i
				break
			}
		}
		count := 0
		slot := -1
		for k, v := range insKeys {
			if v == pair.key {
				count, slot = insCounts[k], k
				break
			}
		}
		insertAt := keyIndex + 1 + count
		sortable = append(sortable, typedOrderEntry{})
		copy(sortable[insertAt+1:], sortable[insertAt:])
		sortable[insertAt] = pair.entry
		if slot < 0 {
			insKeys = append(insKeys, pair.key)
			insCounts = append(insCounts, 1)
		} else {
			insCounts[slot]++
		}
	}
	L.insKeys, L.insCounts = insKeys, insCounts

	vs := L.out[:0]
	sum := 0.0
	weight := 0.0
	vsIndex := 0
	consume := func(index int) int {
		for len(unsortable) != 0 {
			last := unsortable[len(unsortable)-1]
			if last.i > index {
				break
			}
			unsortable = unsortable[:len(unsortable)-1]
			vs = append(vs, last.vs...)
			index++
		}
		return index
	}
	vsIndex = consume(vsIndex)
	for _, entry := range sortable {
		vsIndex += len(entry.vs)
		vs = append(vs, entry.vs...)
		sum += entry.barycenter * entry.weight
		weight += entry.weight
		vsIndex = consume(vsIndex)
	}
	L.sortable, L.unsortable, L.out = sortable[:0], unsortable[:0], vs
	result := typedOrderResult{vs: vs}
	if jsTruthyNumber(weight) {
		result.barycenter = sum / weight
		result.weight = weight
		result.has = true
	}
	return result
}

package dagro

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// The typed engine runs the same phases as dagre's runLayout on interned
// integer node ids and typed labels. Every phase below reproduces dagre's
// function of the same name (dagro's earlier Graph engine ported them line by
// line); comments note where a JavaScript semantic is reproduced explicitly.

type dummyKind uint8

const (
	dkNone dummyKind = iota
	dkEdge
	dkEdgeLabel
	dkBorder
	dkRoot
	dkSelfEdge
	dkEdgeProxy
)

type borderSide uint8

const (
	btNone borderSide = iota
	btLeft
	btRight
)

// edgeRef is an Edge value captured by a phase: endpoints, name key and
// whether the edge was named.
type edgeRef struct {
	v, w, name int32
	hasName    bool
}

type selfRec struct {
	e     edgeRef
	label int32
}

// nlabel is a node label. Numeric attributes that may be absent carry a
// presence flag; an absent attribute reads as NaN, as in dagre's labels.
type nlabel struct {
	width, height    float64
	x, y             float64
	rank, order      float64
	minRank, maxRank float64
	hasX, hasY       bool
	hasRank          bool
	hasOrder         bool
	hasMinMax        bool
	hasBorderTop     bool
	hasEdgeObj       bool
	hasLabelpos      bool
	hasBorders       bool
	hasSelfEdges     bool
	dummy            dummyKind
	borderType       borderSide
	labelpos         string
	borderTop        int32
	borderBottom     int32
	edgeLabel        int32
	edgeObj          edgeRef // "edgeObj" of edge dummies, "e" of edge-proxy and selfedge dummies
	borderBase       int
	borderLeft       []int32
	borderRight      []int32
	selfEdges        []selfRec
}

// elabel is an edge label. minlen, weight, labeloffset and labelRank are NaN
// when absent; width and height keep presence flags because swapping them
// makes an absent attribute present (nil, which num reads as 0).
type elabel struct {
	minlen, weight, labeloffset float64
	width, height               float64
	labelRank                   float64
	x, y                        float64
	points                      []Point
	labelpos                    string
	forwardName                 int32
	forwardHasName              bool
	hasWidth, hasHeight         bool
	hasX, hasY                  bool
	hasPoints                   bool
	reversed                    bool
	nestingEdge                 bool
}

// newNLabel returns a node label without attributes: a copy of nlabelNone,
// which is smaller code than building the label at each inlined call.
func newNLabel() nlabel { return nlabelNone }

// nlabelNone is the label of a node without attributes. It is only ever
// copied, never written.
var nlabelNone = nlabel{
	x: math.NaN(), y: math.NaN(), rank: math.NaN(), order: math.NaN(), minRank: math.NaN(), maxRank: math.NaN(),
	borderTop: inone, borderBottom: inone, edgeLabel: inone,
}

func newELabel() elabel {
	nan := math.NaN()
	return elabel{
		minlen: nan, weight: nan, labeloffset: nan, width: nan, height: nan,
		labelRank: nan, x: nan, y: nan,
	}
}

type lstate struct {
	names  lnames
	enames map[string]int32
	g      *igraph
	nl     []nlabel
	el     []elabel

	ranksep, edgesep, nodesep, marginx, marginy float64
	acyclicer, ranker, rankdir, align           string
	nestingRoot                                 int32
	nodeRankFactor                              float64
	maxRank                                     float64
	dummyChains                                 []int32
	width, height                               float64

	inputNodes []int32
	inputEdges []ekey

	buf       []int32
	nodesBuf  []int32
	edgesBuf  []int32
	stack     []int32   // children lists of recursive traversals
	ptbuf     [][]Point // reusable point storage per edge label index
	ptbufCap  int       // total capacity of the ptbuf entries, in points
	rankBytes int       // inner storage of lm, o.buckets, o.init, o.layering, o.best and p.adjusted
	vals      [9]any
	found     [9]bool
	depths    []int
	layerOf   []int
	nonEmpty  []bool
	deltas    []float64
	nums      []postorderNum
	vPath     []int32
	wPath     []int32
	parentBuf []int32
	seenBuf   []bool
	borderBuf []int32 // storage of the nodes' borderLeft and borderRight lists

	nextNameKey   int32
	lazyNodeNames bool
	lazyRevNames  bool
	lm            [][]int32

	o orderState
	p posState
	r rankState
}

var lstatePool = sync.Pool{New: func() any { return new(lstate) }}

// lstateCache keeps one engine state across garbage collections, which empty
// lstatePool: callers that lay out one graph at a time between allocation-
// heavy work (like D2) then always start with warm storage. It never keeps a
// state larger than lstateKeepMaxBytes (see typed_retain.go).
var lstateCache atomic.Pointer[lstate]

func getLState() *lstate {
	if s := lstateCache.Swap(nil); s != nil {
		return s
	}
	return lstatePool.Get().(*lstate)
}

// putLState releases s and keeps it for the next call. A state that holds
// more than lstateKeepMaxBytes never goes to lstateCache, which survives
// garbage collections: under the gc toolchain it goes to lstatePool, which the
// garbage collector empties, and under TinyGo, whose sync.Pool never frees,
// it is dropped (poolOversized). Only states that finished a call without
// panicking may be passed; a call that panics drops its state.
//
// The engine's functions store grown buffers back into the state on their
// normal returns, not from deferred calls: a state whose call panicked is
// dropped anyway, and under TinyGo (d2's WebAssembly build) extra deferred
// calls in the functions a panic unwinds can make a recover (runGuarded's or
// a caller's) fail or return a different value.
func putLState(s *lstate) {
	s.release()
	if s.retainedBytes() > lstateKeepMaxBytes {
		if poolOversized {
			lstatePool.Put(s)
		}
		return
	}
	if lstateCache.CompareAndSwap(nil, s) {
		return
	}
	lstatePool.Put(s)
}

// release drops references to the caller's data before s is kept.
func (s *lstate) release() {
	s.names.reset()
	clear(s.enames)
	clear(s.vals[:])
	clear(s.found[:])
	clear(s.el)
	clear(s.nl)
	s.nl = s.nl[:0]
	s.el = s.el[:0]
	s.acyclicer, s.ranker, s.rankdir, s.align = "", "", "", ""
}

func (s *lstate) key() []int64 { return s.names.key }

func (s *lstate) newEdgeLabel(l elabel) int32 {
	s.el = appendGrow(s.el, l)
	return int32(len(s.el) - 1)
}

// pointBuf returns empty, non-nil point storage owned by edge label i. Labels
// never share storage, and update copies points out, so reuse is safe.
func (s *lstate) pointBuf(i int32) []Point {
	for int32(len(s.ptbuf)) <= i {
		s.ptbuf = append(s.ptbuf, make([]Point, 0, 4))
		s.ptbufCap += 4
	}
	return s.ptbuf[i][:0]
}

// setPoints stores pts as label i's points and keeps the grown storage.
func (s *lstate) setPoints(i int32, pts []Point) {
	s.el[i].points, s.el[i].hasPoints = pts, true
	if int32(len(s.ptbuf)) > i && cap(pts) > cap(s.ptbuf[i]) {
		s.ptbufCap += cap(pts) - cap(s.ptbuf[i])
		s.ptbuf[i] = pts[:0]
	}
}

func (s *lstate) edgeNameKey(name string) int32 {
	if name == defaultEdgeName {
		return 0
	}
	if k, ok := s.enames[name]; ok {
		return k
	}
	s.nextNameKey++
	s.enames[name] = s.nextNameKey
	return s.nextNameKey
}

// dummyPrefixes are the prefixes addDummyNode and createOrderRootNode use.
var dummyPrefixes = [...]string{"_root", "_bt", "_bb", "_ep", "_d", "_bl", "_br", "_se"}

// generatedLike reports whether name is prefix followed by one or more ASCII
// digits, i.e. whether a counter-numbered name with that prefix could equal it.
func generatedLike(name, prefix string) bool {
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return false
	}
	for i := len(prefix); i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return true
}

// noteInputName clears the lazy-name flags when an input node name could
// equal a counter-numbered dummy name.
func (s *lstate) noteInputName(name string) {
	if len(name) < 2 || name[0] != '_' {
		return
	}
	for _, prefix := range dummyPrefixes {
		if generatedLike(name, prefix) {
			s.lazyNodeNames = false
		}
	}
}

// noteInputEdgeName clears the lazy reversed-name flag when an input edge
// could carry a generated "rev" name.
func (s *lstate) noteInputEdgeName(name string) {
	if generatedLike(name, "rev") {
		s.lazyRevNames = false
	}
}

// ensureNode extends s.nl with labels without attributes up to node id. It
// is not inlined: it runs once per new node, and inlining its label append at
// every call site would add kilobytes of code.
//
//go:noinline
func (s *lstate) ensureNode(id int32) {
	for int32(len(s.nl)) <= id {
		s.nl = appendGrow(s.nl, newNLabel())
	}
}

func (s *lstate) nameInUse(name string) bool {
	id, ok := s.names.lookup(name)
	return ok && s.g.hasNode(id)
}

// uniqueIDCounter numbers the dummy nodes whose plain names are taken, as
// dagre's uniqueId does: process-wide, so that concurrent layouts never draw
// the same value.
var uniqueIDCounter atomic.Uint64

// draw is dagre's uniqueId counter increment: each value is visible to the
// whole process as soon as it is drawn, and concurrent layouts never draw the
// same value.
func (s *lstate) draw() uint64 {
	return uniqueIDCounter.Add(1)
}

// uniqueID is uniqueID(prefix), drawn through draw.
func (s *lstate) uniqueID(prefix string) string {
	return prefix + strconv.FormatUint(s.draw(), 10)
}

// errDecline is the panic value with which the typed engine gives up on an
// input it does not lay out (see order); LayoutTyped returns it.
var errDecline = fmt.Errorf("%w: more than %d ranks", ErrUnsupportedInput, maxTypedRank)

// runGuarded runs s.run and, when it succeeds, stores the result in out
// (updateTyped). It turns a panic into an error: errDecline is returned as it
// is, any other value p as &PanicError{Value: p}. The caller drops s after a
// panic.
func (s *lstate) runGuarded(in *LayoutInput, out *LayoutOutput) (err error) {
	defer func() {
		if p := recover(); p != nil {
			if p == errDecline {
				err = errDecline
			} else {
				err = &PanicError{Value: p}
			}
		}
	}()
	if err := s.run(); err != nil {
		return err
	}
	s.updateTyped(in, out)
	return nil
}

// failNilMapWrite stops the layout where dagre writes key into the label of
// a node that is not in its graph (an entry missing from a layer of the
// layer matrix), which fails in dagre too ("assignment to entry in nil map").
// The sites are assignOrder, insertSelfEdges and positionY.
func (s *lstate) failNilMapWrite(key string) {
	panic(fmt.Sprintf("dagre fails on this input: it sets %q on a node missing from a layer", key))
}

// addDummy is addDummyNode: the plain prefix when free, otherwise the next
// free uniqueID(prefix). It consumes the process-wide counter exactly as
// dagre does.
func (s *lstate) addDummy(kind dummyKind, prefix string, label nlabel) int32 {
	var id int32
	if !s.nameInUse(prefix) {
		id = s.names.intern(prefix)
	} else if s.lazyNodeNames {
		// No input name has the form prefix+digits and counter values never
		// repeat, so the first draw is free, as uniqueID's loop would find.
		id = s.names.internLazy(prefix, s.draw())
	} else {
		name := prefix
		for s.nameInUse(name) {
			name = s.uniqueID(prefix)
		}
		id = s.names.intern(name)
	}
	s.ensureNode(id)
	label.dummy = kind
	s.nl[id] = label
	s.g.setNode(id)
	return id
}

// nodes returns g.Nodes() in a buffer that the next call overwrites.
func (s *lstate) nodes() []int32 {
	s.nodesBuf = s.g.nodeList(s.key(), s.nodesBuf[:0])
	return s.nodesBuf
}

// edges returns g.Edges() in a buffer that the next call overwrites. It is
// not inlined: the phases call it about twenty times, once per pass over the
// edges, which inlining would not speed up.
//
//go:noinline
func (s *lstate) edges() []int32 {
	s.edgesBuf = s.g.edgeList(s.edgesBuf[:0])
	return s.edgesBuf
}

// pushChildren appends v's children to the traversal stack; the caller
// truncates the stack back to the returned mark when done.
func (s *lstate) pushChildren(v int32) (children []int32, mark int) {
	mark = len(s.stack)
	s.stack = s.g.children(v, s.key(), s.stack)
	return s.stack[mark:], mark
}

func (s *lstate) refOf(e int32) edgeRef {
	ed := &s.g.edges[e]
	return edgeRef{v: ed.v, w: ed.w, name: ed.name, hasName: ed.hasName}
}

// edgeByRef returns the live edge with r's key, or inone.
func (s *lstate) edgeByRef(r edgeRef) int32 { return s.g.findEdge(r.v, r.w, r.name) }

// setEdgeRef is SetEdgeObject(e, label).
func (s *lstate) setEdgeRef(r edgeRef, label int32) int32 {
	return s.g.setEdge(r.v, r.w, r.name, r.hasName, label, true)
}

// typedEligibleName reports whether the typed engine reproduces graphlib's string
// edge ids and node-id truthiness for g: no empty node name and no node name
// containing the edge-id delimiter.
func typedEligibleName(v string) bool {
	return v != "" && !strings.Contains(v, edgeKeyDelim)
}

// begin resets s for a graph with the given numbers of input nodes and edges
// and sets the graph attributes to buildLayoutGraph's defaults.
func (s *lstate) begin(nodes, edges int) {
	s.names.reset()
	if s.enames == nil {
		s.enames = make(map[string]int32, edges)
	} else {
		clear(s.enames)
	}
	s.nextNameKey = 0
	s.lazyNodeNames, s.lazyRevNames = true, true
	// Dummy nodes and internal edges typically outnumber the input; size the
	// label storage once instead of growing it repeatedly.
	if want := 2*(nodes+edges) + 32; cap(s.nl) < want {
		s.nl = make([]nlabel, 0, want)
	}
	if want := 3*edges + nodes + 32; cap(s.el) < want {
		s.el = make([]elabel, 0, want)
	}
	s.nl = s.nl[:0]
	s.el = s.el[:0]
	if s.g == nil {
		s.g = newIGraph(true, true, nodes*4+16, edges*4+16)
	} else {
		s.g.reset(true, true)
	}
	s.g.grow(int32(nodes - 1))
	s.dummyChains = s.dummyChains[:0]
	s.stack = s.stack[:0]
	s.borderBuf = s.borderBuf[:0]
	s.ranksep, s.edgesep, s.nodesep, s.rankdir = 50, 20, 50, "TB"
	s.marginx, s.marginy = math.NaN(), math.NaN()
	s.acyclicer, s.ranker, s.align = "", "", ""
	s.nestingRoot, s.nodeRankFactor, s.maxRank, s.width, s.height = inone, 0, 0, 0, 0
	s.inputNodes = s.inputNodes[:0]
	s.inputEdges = s.inputEdges[:0]
}

func (s *lstate) run() error {
	s.makeSpaceForEdgeLabels()
	s.removeSelfEdges()
	s.runAcyclic()
	s.runNestingGraph()
	s.rank()
	s.injectEdgeLabelProxies()
	s.removeEmptyRanks()
	s.cleanupNestingGraph()
	s.normalizeRanks()
	s.assignRankMinMax()
	s.removeEdgeLabelProxies()
	s.runNormalize()
	s.parentDummyChains()
	s.addBorderSegments()
	s.order()
	s.insertSelfEdges()
	s.adjustCoordinateSystem()
	s.position()
	s.positionSelfEdges()
	s.removeBorderNodes()
	s.undoNormalize()
	s.fixupEdgeLabelCoords()
	s.undoCoordinateSystem()
	s.translateGraph()
	if err := s.assignNodeIntersects(); err != nil {
		return err
	}
	s.reversePointsForReversedEdges()
	s.undoAcyclic()
	return nil
}

func (s *lstate) makeSpaceForEdgeLabels() {
	s.ranksep = s.ranksep / 2
	horizontalLabels := s.rankdir == "TB" || s.rankdir == "BT"
	for _, e := range s.edges() {
		l := &s.el[s.g.edges[e].label]
		l.minlen = l.minlen * 2
		if strings.ToLower(l.labelpos) != "c" {
			if horizontalLabels {
				l.width = l.width + l.labeloffset
			} else {
				l.height = l.height + l.labeloffset
			}
		}
	}
}

func (s *lstate) removeSelfEdges() {
	for _, e := range s.edges() {
		ed := s.g.edges[e]
		if ed.v != ed.w {
			continue
		}
		n := &s.nl[ed.v]
		n.selfEdges = append(n.selfEdges, selfRec{e: s.refOf(e), label: ed.label})
		n.hasSelfEdges = true
		s.g.removeEdge(e)
	}
}

func (s *lstate) runAcyclic() {
	var fas []edgeRef
	if s.acyclicer == "greedy" {
		fas = s.greedyFAS()
	} else {
		fas = s.dfsFAS()
	}
	for _, r := range fas {
		e := s.edgeByRef(r)
		label := inone
		if e != inone {
			label = s.g.edges[e].label
			s.g.removeEdge(e)
		}
		l := &s.el[label]
		l.forwardName, l.forwardHasName = r.name, r.hasName
		l.reversed = true
		name := s.nextReversedEdgeName(r.w, r.v)
		s.g.setEdge(r.w, r.v, name, true, label, true)
	}
}

func (s *lstate) nextReversedEdgeName(v, w int32) int32 {
	if s.lazyRevNames {
		// No input edge name has the form rev+digits: the first candidate is
		// free. Its string is never needed, only a fresh name key.
		s.draw()
		s.nextNameKey++
		return s.nextNameKey
	}
	for {
		name := s.uniqueID("rev")
		k, ok := s.enames[name]
		if !ok || s.g.findEdge(v, w, k) == inone {
			return s.edgeNameKey(name)
		}
	}
}

func (s *lstate) dfsFAS() []edgeRef {
	var fas []edgeRef
	n := len(s.g.nodes)
	visited := make([]bool, n)
	onStack := make([]bool, n)
	type frame struct{ v, e int32 }
	var stack []frame
	dfs := func(start int32) {
		if visited[start] {
			return
		}
		visited[start], onStack[start] = true, true
		stack = append(stack[:0], frame{start, s.g.nodes[start].outHead})
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.e == inone {
				onStack[top.v] = false
				stack = stack[:len(stack)-1]
				continue
			}
			e := top.e
			top.e = s.g.edges[e].outNext
			w := s.g.edges[e].w
			if onStack[w] {
				fas = append(fas, s.refOf(e))
			} else if !visited[w] {
				visited[w], onStack[w] = true, true
				stack = append(stack, frame{w, s.g.nodes[w].outHead})
			}
		}
	}
	all := s.nodes()
	for _, v := range all {
		if s.g.nodes[v].inHead == inone {
			dfs(v)
		}
	}
	for _, v := range all {
		dfs(v)
	}
	return fas
}

// greedyFAS is dagre's greedyFAS with the edge weights, run on the layout
// graph (acyclicer "greedy", which D2 does not set): it returns the edges to
// reverse, in dagre's order. As graphlib's state graph does, it merges the
// edges of each ordered node pair into one weighted with their sum, keeps the
// nodes and the in- and out-edges of each node in the layout graph's order,
// and removes a node's edges with it.
func (s *lstate) greedyFAS() []edgeRef {
	nodes := s.nodes()
	if len(nodes) <= 1 {
		return nil
	}
	index := make(map[int32]int, len(nodes))
	entries := make([]fasEntry, len(nodes))
	for i, v := range nodes {
		index[v] = i
		entries[i].v = i
	}
	type pairKey struct{ v, w int }
	type pair struct {
		v, w   int
		weight float64
		edges  []int32 // the layout graph's edges from v to w, in order
	}
	var pairs []pair
	pairOf := map[pairKey]int{}
	in, out := make([][]int, len(nodes)), make([][]int, len(nodes))
	maxIn, maxOut := 0.0, 0.0
	for _, e := range s.edges() {
		ed := &s.g.edges[e]
		v, w := index[ed.v], index[ed.w]
		p, ok := pairOf[pairKey{v, w}]
		if !ok {
			p = len(pairs)
			pairOf[pairKey{v, w}] = p
			pairs = append(pairs, pair{v: v, w: w})
			out[v] = append(out[v], p)
			in[w] = append(in[w], p)
		}
		weight := s.el[ed.label].weight
		pairs[p].weight += weight
		pairs[p].edges = append(pairs[p].edges, e)
		entries[v].out += weight
		entries[w].in += weight
		maxOut, maxIn = maxJS2(maxOut, entries[v].out), maxJS2(maxIn, entries[w].in)
	}
	buckets := make([]fasList, int(maxOut+maxIn)+3)
	zeroIdx := int(maxIn) + 1
	assign := func(entry *fasEntry) {
		index := 0
		if !jsTruthyNumber(entry.out) {
			index = 0
		} else if !jsTruthyNumber(entry.in) {
			index = len(buckets) - 1
		} else {
			index = int(entry.out-entry.in) + zeroIdx
		}
		buckets[index].enqueue(entry)
	}
	for i := range entries {
		assign(&entries[i])
	}
	removed := make([]bool, len(nodes))
	remaining := len(nodes)
	var results []int
	remove := func(entry *fasEntry, collect bool) {
		v := entry.v
		if removed[v] {
			return
		}
		for _, p := range in[v] {
			if removed[pairs[p].v] {
				continue
			}
			u := &entries[pairs[p].v]
			if collect {
				results = append(results, p)
			}
			u.out -= pairs[p].weight
			assign(u)
		}
		for _, p := range out[v] {
			if removed[pairs[p].w] {
				continue
			}
			w := &entries[pairs[p].w]
			w.in -= pairs[p].weight
			assign(w)
		}
		removed[v] = true
		remaining--
	}
	sources, sinks := &buckets[len(buckets)-1], &buckets[0]
	for remaining > 0 {
		for entry := sinks.dequeue(); entry != nil; entry = sinks.dequeue() {
			remove(entry, false)
		}
		for entry := sources.dequeue(); entry != nil; entry = sources.dequeue() {
			remove(entry, false)
		}
		if remaining > 0 {
			for i := len(buckets) - 2; i > 0; i-- {
				if entry := buckets[i].dequeue(); entry != nil {
					remove(entry, true)
					break
				}
			}
		}
	}
	var fas []edgeRef
	for _, p := range results {
		for _, e := range pairs[p].edges {
			ed := &s.g.edges[e]
			r := edgeRef{v: ed.v, w: ed.w, hasName: ed.hasName}
			if ed.hasName {
				r.name = ed.name
			}
			fas = append(fas, r)
		}
	}
	return fas
}

func (s *lstate) runNestingGraph() {
	l := newNLabel()
	root := s.addDummy(dkRoot, "_root", l)
	s.depths = append(s.depths[:0], make([]int, len(s.g.nodes))...)
	depths := s.depths
	roots, mark := s.pushChildren(inone)
	for _, v := range roots {
		s.treeDepths(v, 1)
	}
	s.stack = s.stack[:mark]
	height := 0
	for _, depth := range depths {
		if depth-1 > height {
			height = depth - 1
		}
	}
	nodeSep := 2*height + 1
	s.nestingRoot = root
	for _, e := range s.edges() {
		l := &s.el[s.g.edges[e].label]
		l.minlen = l.minlen * float64(nodeSep)
	}
	weight := s.sumWeights() + 1
	top, mark := s.pushChildren(inone)
	for _, child := range top {
		s.nestingDFS(root, nodeSep, weight, height, depths, child)
	}
	s.stack = s.stack[:mark]
	s.nodeRankFactor = float64(nodeSep)
}

func (s *lstate) treeDepths(v int32, depth int) {
	children, mark := s.pushChildren(v)
	for _, child := range children {
		s.treeDepths(child, depth+1)
	}
	s.stack = s.stack[:mark]
	s.depths[v] = depth
}

func (s *lstate) sumWeights() float64 {
	total := 0.0
	for _, e := range s.edges() {
		total += s.el[s.g.edges[e].label].weight
	}
	return total
}

func (s *lstate) nestingEdgeLabel(weight, minlen float64, nesting bool) int32 {
	l := newELabel()
	l.weight, l.minlen, l.nestingEdge = weight, minlen, nesting
	return s.newEdgeLabel(l)
}

func (s *lstate) addBorderNode(prefix string) int32 {
	l := newNLabel()
	l.width, l.height = 0, 0
	return s.addDummy(dkBorder, prefix, l)
}

func (s *lstate) nestingDFS(root int32, nodeSep int, weight float64, height int, depths []int, v int32) {
	children, mark := s.pushChildren(v)
	if len(children) == 0 {
		if v != root {
			s.g.setEdge(root, v, 0, false, s.nestingEdgeLabel(0, float64(nodeSep), false), true)
		}
		s.stack = s.stack[:mark]
		return
	}
	top := s.addBorderNode("_bt")
	bottom := s.addBorderNode("_bb")
	s.g.setParent(top, v)
	s.nl[v].borderTop, s.nl[v].hasBorderTop = top, true
	s.g.setParent(bottom, v)
	s.nl[v].borderBottom = bottom
	for _, child := range children {
		s.nestingDFS(root, nodeSep, weight, height, depths, child)
		childNode := &s.nl[child]
		childTop, childBottom := child, child
		thisWeight := 2 * weight
		if childNode.hasBorderTop {
			childTop = childNode.borderTop
			childBottom = childNode.borderBottom
			thisWeight = weight
		}
		minlen := 1
		if childTop == childBottom {
			minlen = height - depths[v] + 1
		}
		s.g.setEdge(top, childTop, 0, false, s.nestingEdgeLabel(thisWeight, float64(minlen), true), true)
		s.g.setEdge(childBottom, bottom, 0, false, s.nestingEdgeLabel(thisWeight, float64(minlen), true), true)
	}
	if s.g.parentOf(v) == inone {
		s.g.setEdge(root, top, 0, false, s.nestingEdgeLabel(0, float64(height+depths[v]), false), true)
	}
	s.stack = s.stack[:mark]
}

func (s *lstate) injectEdgeLabelProxies() {
	for _, e := range s.edges() {
		ed := s.g.edges[e]
		l := &s.el[ed.label]
		if jsTruthyNumber(l.width) && jsTruthyNumber(l.height) {
			v, w := &s.nl[ed.v], &s.nl[ed.w]
			p := newNLabel()
			p.rank, p.hasRank = (w.rank-v.rank)/2+v.rank, true
			p.edgeObj = s.refOf(e)
			s.addDummy(dkEdgeProxy, "_ep", p)
		}
	}
}

func (s *lstate) removeEmptyRanks() {
	all := s.nodes()
	offset := math.Inf(1)
	for _, v := range all {
		if n := &s.nl[v]; n.hasRank {
			offset = lodashMin2(offset, n.rank)
		}
	}
	s.layerOf = append(s.layerOf[:0], make([]int, len(all))...)
	layerOf := s.layerOf
	nonEmpty := s.nonEmpty[:0]
	for i, v := range all {
		layerOf[i] = -1
		n := &s.nl[v]
		if !n.hasRank {
			continue
		}
		rankValue := n.rank - offset
		if math.IsNaN(rankValue) || math.IsInf(rankValue, 0) ||
			rankValue < 0 || rankValue != math.Trunc(rankValue) || rankValue >= 4294967295 {
			continue
		}
		r := int(rankValue)
		for len(nonEmpty) <= r {
			nonEmpty = append(nonEmpty, false)
		}
		nonEmpty[r] = true
		layerOf[i] = r
	}
	s.nonEmpty = nonEmpty
	s.deltas = append(s.deltas[:0], make([]float64, len(nonEmpty))...)
	deltas := s.deltas
	delta := 0.0
	factor := s.nodeRankFactor
	for i, has := range nonEmpty {
		if !has && math.Mod(float64(i), factor) != 0 {
			delta--
		}
		deltas[i] = delta
	}
	for i, v := range all {
		if r := layerOf[i]; r >= 0 && deltas[r] != 0 {
			n := &s.nl[v]
			n.rank = n.rank + deltas[r]
		}
	}
}

func (s *lstate) cleanupNestingGraph() {
	s.buf = s.g.removeNode(s.nestingRoot, s.key(), s.buf)
	for _, e := range s.edges() {
		if s.el[s.g.edges[e].label].nestingEdge {
			s.g.removeEdge(e)
		}
	}
}

func (s *lstate) normalizeRanks() {
	all := s.nodes()
	min := math.Inf(1)
	for _, v := range all {
		if n := &s.nl[v]; n.hasRank {
			min = lodashMin2(min, n.rank)
		}
	}
	for _, v := range all {
		if n := &s.nl[v]; n.hasRank {
			n.rank = n.rank - min
		}
	}
}

func (s *lstate) assignRankMinMax() {
	max := 0.0
	for _, v := range s.nodes() {
		n := &s.nl[v]
		if n.hasBorderTop {
			n.minRank = s.nl[n.borderTop].rank
			n.maxRank = s.nl[n.borderBottom].rank
			n.hasMinMax = true
			max = lodashMax2(max, n.maxRank)
		}
	}
	s.maxRank = max
}

func (s *lstate) removeEdgeLabelProxies() {
	for _, v := range s.nodes() {
		n := &s.nl[v]
		if n.dummy == dkEdgeProxy {
			e := s.edgeByRef(n.edgeObj)
			s.el[s.g.edges[e].label].labelRank = n.rank
			s.buf = s.g.removeNode(v, s.key(), s.buf)
		}
	}
}

func (s *lstate) runNormalize() {
	s.reserveDummies()
	s.dummyChains = s.dummyChains[:0]
	for _, e := range s.edges() {
		s.normalizeEdge(e)
	}
}

// maxReserve bounds the dummy nodes reserveDummies plans for; with more (ranks
// that are not modest integers) the arrays grow as they fill.
const maxReserve = 1 << 22

// reserveDummies makes room, once the ranks are final, for the nodes and edges
// the later phases add: runNormalize's edge dummies and chain edges (counted as
// normalizeEdge loops), addBorderSegments' border nodes and edges, order's two
// root nodes per rank and insertSelfEdges' self-edge dummies. A state that is
// not reused (see putLState) then allocates its per-node and per-edge arrays
// about once instead of growing them step by step, which on large graphs is
// most of what a layout allocates. Storage only: nothing reads capacities.
func (s *lstate) reserveDummies() {
	nodes, edges := 0, 0
	maxRank := -1
	// Counting needs no order: the graph's own node and edge lists, not the
	// JavaScript-ordered copies s.nodes() and s.edges() build.
	for v := s.g.nhead; v != inone; v = s.g.nodes[v].next {
		n := &s.nl[v]
		if n.hasRank {
			if r := int(n.rank); r > maxRank {
				maxRank = r
			}
		}
		if n.hasMinMax {
			if size := n.maxRank - n.minRank + 1; size > 0 && size <= maxReserve {
				nodes += 2 * int(size)
				edges += 2 * int(size)
			}
		}
		nodes += len(n.selfEdges)
		if nodes > maxReserve {
			return
		}
	}
	if maxRank > maxReserve {
		return
	}
	nodes += 2 * max(maxRank, 0)
	for e := range s.g.edges {
		ed := &s.g.edges[e]
		if !ed.alive {
			continue
		}
		vRank, wRank := s.nl[ed.v].rank, s.nl[ed.w].rank
		if wRank == vRank+1 {
			continue
		}
		edges++
		for r := vRank; r+1 < wRank; r++ {
			nodes++
			edges++
			if nodes > maxReserve {
				return
			}
		}
	}
	s.nl = reserve(s.nl, nodes)
	s.names.reserve(nodes)
	s.g.nodes = reserve(s.g.nodes, nodes)
	s.el = reserve(s.el, edges)
	s.g.edges = reserve(s.g.edges, edges)
	s.g.counts = reserve(s.g.counts, 2*edges)
}

func (s *lstate) chainEdge(v, w int32, r edgeRef, weight float64) {
	l := newELabel()
	l.weight = weight
	name := int32(0)
	if r.hasName {
		name = r.name
	}
	s.g.setEdge(v, w, name, r.hasName, s.newEdgeLabel(l), true)
}

func (s *lstate) normalizeEdge(e int32) {
	ed := s.g.edges[e]
	v, w := ed.v, ed.w
	vRank, wRank := s.nl[v].rank, s.nl[w].rank
	label := ed.label
	labelRank := s.el[label].labelRank
	if wRank == vRank+1 {
		return
	}
	r := s.refOf(e)
	s.g.removeEdge(e)
	for i := 0; vRank+1 < wRank; i++ {
		vRank++
		s.el[label].points, s.el[label].hasPoints = s.pointBuf(label), true
		attrs := newNLabel()
		attrs.width, attrs.height = 0, 0
		attrs.edgeLabel = label
		attrs.edgeObj, attrs.hasEdgeObj = r, true
		attrs.rank, attrs.hasRank = vRank, true
		dummy := s.addDummy(dkEdge, "_d", attrs)
		if vRank == labelRank {
			d := &s.nl[dummy]
			d.width, d.height = s.el[label].width, s.el[label].height
			d.dummy = dkEdgeLabel
			d.labelpos, d.hasLabelpos = s.el[label].labelpos, true
		}
		s.chainEdge(v, dummy, r, s.el[label].weight)
		if i == 0 {
			s.dummyChains = append(s.dummyChains, dummy)
		}
		v = dummy
	}
	s.chainEdge(v, w, r, s.el[label].weight)
}

type postorderNum struct{ low, lim int }

func (s *lstate) parentDummyChains() {
	s.nums = append(s.nums[:0], make([]postorderNum, len(s.g.nodes))...)
	nums := s.nums
	lim := 0
	roots, mark := s.pushChildren(inone)
	for _, v := range roots {
		lim = s.compoundPostorder(v, lim)
	}
	s.stack = s.stack[:mark]
	succ := s.buf[:0]
	for _, start := range s.dummyChains {
		v := start
		edgeObj := s.nl[v].edgeObj
		path, lca := s.findCompoundPath(nums, edgeObj.v, edgeObj.w)
		pathIdx := 0
		pathV := path[pathIdx]
		ascending := true
		for v != edgeObj.w {
			node := &s.nl[v]
			if ascending {
				for {
					pathV = path[pathIdx]
					if pathV == lca || s.nl[pathV].maxRank >= node.rank {
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
					if s.nl[next].minRank > node.rank {
						break
					}
					pathIdx++
					pathV = next
				}
				pathV = path[pathIdx]
			}
			s.g.setParent(v, pathV)
			succ = s.g.successors(v, s.key(), succ[:0])
			v = succ[0]
		}
	}
	s.buf = succ[:0]
}

func (s *lstate) compoundPostorder(v int32, lim int) int {
	low := lim
	children, mark := s.pushChildren(v)
	for _, child := range children {
		lim = s.compoundPostorder(child, lim)
	}
	s.stack = s.stack[:mark]
	s.nums[v] = postorderNum{low: low, lim: lim}
	return lim + 1
}

func (s *lstate) findCompoundPath(nums []postorderNum, v, w int32) ([]int32, int32) {
	vPath, wPath := s.vPath[:0], s.wPath[:0]
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
		parent = s.g.parentOf(parent)
		vPath = append(vPath, parent)
		if parent == inone || !(nums[parent].low > low || lim > nums[parent].lim) {
			break
		}
	}
	lca := parent
	parent = w
	for {
		parent = s.g.parentOf(parent)
		if parent == lca {
			break
		}
		wPath = append(wPath, parent)
	}
	for i, j := 0, len(wPath)-1; i < j; i, j = i+1, j-1 {
		wPath[i], wPath[j] = wPath[j], wPath[i]
	}
	vPath = append(vPath, wPath...)
	s.vPath, s.wPath = vPath[:0], wPath[:0]
	return vPath, lca
}

func (s *lstate) addBorderSegments() {
	roots, mark := s.pushChildren(inone)
	for _, v := range roots {
		s.borderSegments(v)
	}
	s.stack = s.stack[:mark]
}

func (s *lstate) borderSegments(v int32) {
	{
		children, mark := s.pushChildren(v)
		for _, child := range children {
			s.borderSegments(child)
		}
		s.stack = s.stack[:mark]
		if !s.nl[v].hasMinMax {
			return
		}
		n := &s.nl[v]
		lo, hi := int(n.minRank), int(n.maxRank)+1
		n.hasBorders = true
		n.borderBase = lo
		size := hi - lo
		if size < 0 {
			size = 0
		}
		n.borderLeft = s.borderList(size)
		n.borderRight = s.borderList(size)
		for rank := lo; rank < hi; rank++ {
			s.addSubgraphBorderNode(btLeft, "_bl", v, rank)
			s.addSubgraphBorderNode(btRight, "_br", v, rank)
		}
	}
}

// borderList returns a zeroed list of n border ids carved from the state's
// storage, capped so that it never overlaps another list.
func (s *lstate) borderList(n int) []int32 {
	if n > cap(s.borderBuf)-len(s.borderBuf) {
		// Earlier lists keep the old block; new lists come from a new one.
		s.borderBuf = make([]int32, 0, max(2*cap(s.borderBuf), 64, n))
	}
	at := len(s.borderBuf)
	s.borderBuf = s.borderBuf[:at+n]
	list := s.borderBuf[at : at+n : at+n]
	clear(list)
	return list
}

// borderAt reads a borderLeft/borderRight map at rank; inone when missing.
func (n *nlabel) borderAt(side borderSide, rank int) int32 {
	if !n.hasBorders {
		return inone
	}
	list := n.borderLeft
	if side == btRight {
		list = n.borderRight
	}
	i := rank - n.borderBase
	if i < 0 || i >= len(list) {
		return inone
	}
	return list[i]
}

func (s *lstate) addSubgraphBorderNode(side borderSide, prefix string, sg int32, rank int) {
	label := newNLabel()
	label.width, label.height = 0, 0
	label.rank, label.hasRank = float64(rank), true
	label.borderType = side
	prev := s.nl[sg].borderAt(side, rank-1)
	curr := s.addDummy(dkBorder, prefix, label)
	n := &s.nl[sg]
	if side == btLeft {
		n.borderLeft[rank-n.borderBase] = curr
	} else {
		n.borderRight[rank-n.borderBase] = curr
	}
	s.g.setParent(curr, sg)
	if prev != inone {
		l := newELabel()
		l.weight = 1
		s.g.setEdge(prev, curr, 0, false, s.newEdgeLabel(l), true)
	}
}

// buildLayerMatrix ports buildLayerMatrix for the given node list (the
// graph's JavaScript node order). Missing positions stay inone.
func (s *lstate) buildLayerMatrix(all []int32) [][]int32 {
	max := math.Inf(-1)
	for _, v := range all {
		if n := &s.nl[v]; n.hasRank {
			max = lodashMax2(max, n.rank)
		}
	}
	if max < 0 || math.IsInf(max, -1) {
		return nil
	}
	// The result reuses s.lm: each caller is done with it before the next call.
	size := int(max) + 1
	for len(s.lm) < size {
		s.lm = append(s.lm, nil)
	}
	layers := s.lm[:size]
	before := 0
	for i := range layers {
		before += cap(layers[i])
		layers[i] = layers[i][:0]
	}
	for _, v := range all {
		n := &s.nl[v]
		if !n.hasRank || !n.hasOrder {
			continue
		}
		rank, order := int(n.rank), int(n.order)
		for len(layers[rank]) <= order {
			layers[rank] = append(layers[rank], inone)
		}
		layers[rank][order] = v
	}
	s.rankBytes += 4 * (capsLayers(layers) - before)
	return layers
}

func (s *lstate) insertSelfEdges() {
	for _, layer := range s.buildLayerMatrix(s.nodes()) {
		orderShift := 0
		for i, v := range layer {
			if v == inone {
				// The original reads node["rank"] of the missing entry's nil
				// label (nil: no typed number) and writes 0 into it.
				s.failNilMapWrite("rank")
			}
			node := &s.nl[v]
			if !node.hasRank {
				node.rank, node.hasRank = 0, true
			}
			node.order, node.hasOrder = float64(i+orderShift), true
			for _, selfEdge := range node.selfEdges {
				orderShift++
				l := &s.el[selfEdge.label]
				d := newNLabel()
				d.width, d.height = l.width, l.height
				if !l.hasWidth {
					d.width = math.NaN()
				}
				if !l.hasHeight {
					d.height = math.NaN()
				}
				d.rank, d.hasRank = s.nl[v].rank, true
				d.order, d.hasOrder = float64(i+orderShift), true
				d.edgeObj = selfEdge.e
				d.edgeLabel = selfEdge.label
				s.addDummy(dkSelfEdge, "_se", d)
				l = &s.el[selfEdge.label]
				if !l.hasPoints || len(l.points) != 7 {
					s.setPoints(selfEdge.label, append(s.pointBuf(selfEdge.label),
						Point{X: 0, Y: -10},
						Point{X: 0, Y: -10},
						Point{X: 0, Y: 0},
						Point{X: 0, Y: 10},
						Point{X: 0, Y: 10},
						Point{X: 0, Y: 0},
						Point{X: 0, Y: 0},
					))
				}
			}
			node = &s.nl[v]
			node.selfEdges, node.hasSelfEdges = nil, false
		}
	}
}

func (s *lstate) swapWidthHeight() {
	for _, v := range s.nodes() {
		n := &s.nl[v]
		n.width, n.height = n.height, n.width
	}
	for _, e := range s.edges() {
		l := &s.el[s.g.edges[e].label]
		w, h := l.width, l.height
		if !l.hasWidth {
			w = 0
		}
		if !l.hasHeight {
			h = 0
		}
		l.width, l.height = h, w
		l.hasWidth, l.hasHeight = true, true
	}
}

// lowerRankdir is strings.ToLower(rankdir), without the allocation for the
// upper-case spellings callers usually pass.
func lowerRankdir(rankdir string) string {
	switch rankdir {
	case "TB":
		return "tb"
	case "BT":
		return "bt"
	case "LR":
		return "lr"
	case "RL":
		return "rl"
	}
	return strings.ToLower(rankdir)
}

func (s *lstate) adjustCoordinateSystem() {
	rankDir := lowerRankdir(s.rankdir)
	if rankDir == "lr" || rankDir == "rl" {
		s.swapWidthHeight()
	}
}

func (s *lstate) undoCoordinateSystem() {
	rankDir := lowerRankdir(s.rankdir)
	if rankDir == "bt" || rankDir == "rl" {
		s.reverseY()
	}
	if rankDir == "lr" || rankDir == "rl" {
		s.swapXY()
		s.swapWidthHeight()
	}
}

func (s *lstate) reverseY() {
	for _, v := range s.nodes() {
		n := &s.nl[v]
		n.y, n.hasY = -n.y, true
	}
	for _, e := range s.edges() {
		l := &s.el[s.g.edges[e].label]
		for i := range l.points {
			l.points[i].Y = -l.points[i].Y
		}
		l.hasPoints = true
		if l.hasY {
			l.y = -l.y
		}
	}
}

// swapXY is swapXY. swapXYOne swaps the raw map values, so a coordinate
// swapped in from a missing key is present with the value nil, which every
// later reader (num in translateGraph, assignNodeIntersects and
// updateInputGraph) reads as 0: it is stored as a present 0.
func (s *lstate) swapXY() {
	for _, v := range s.nodes() {
		n := &s.nl[v]
		x, y := n.y, n.x
		if !n.hasY {
			x = 0
		}
		if !n.hasX {
			y = 0
		}
		n.x, n.y, n.hasX, n.hasY = x, y, true, true
	}
	for _, e := range s.edges() {
		l := &s.el[s.g.edges[e].label]
		for i := range l.points {
			l.points[i].X, l.points[i].Y = l.points[i].Y, l.points[i].X
		}
		l.hasPoints = true
		if l.hasX {
			x, y := l.y, l.x
			if !l.hasY {
				x = 0
			}
			l.x, l.y, l.hasX, l.hasY = x, y, true, true
		}
	}
}

func finiteOr(value float64, present bool, fallback float64) float64 {
	if !present || math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	return value
}

func (s *lstate) positionSelfEdges() {
	for _, v := range s.nodes() {
		node := &s.nl[v]
		if node.dummy == dkSelfEdge {
			r := node.edgeObj
			label := node.edgeLabel
			selfNode := &s.nl[r.v]
			x := finiteOr(selfNode.x, selfNode.hasX, 0)
			y := finiteOr(selfNode.y, selfNode.hasY, 0)
			width := finiteOr(selfNode.width, true, 0)
			height := finiteOr(selfNode.height, true, 0)
			nodeX := finiteOr(node.x, node.hasX, x)
			nodeY := finiteOr(node.y, node.hasY, y)
			s.setSelfLoop(label, nodeX, nodeY, width, height)
			l := &s.el[label]
			l.x, l.y, l.hasX, l.hasY = nodeX, nodeY, true, true
			s.setEdgeRef(r, label)
			s.buf = s.g.removeNode(v, s.key(), s.buf)
			continue
		}
		if !node.hasSelfEdges {
			continue
		}
		for _, selfEdge := range node.selfEdges {
			l := &s.el[selfEdge.label]
			if l.hasPoints && len(l.points) == 7 {
				continue
			}
			x := finiteOr(node.x, node.hasX, 0)
			y := finiteOr(node.y, node.hasY, 0)
			s.setSelfLoop(selfEdge.label, x, y, finiteOr(node.width, true, 0), finiteOr(node.height, true, 0))
		}
	}
}

func (s *lstate) removeBorderNodes() {
	all := s.nodes()
	for _, v := range all {
		if !s.g.hasChildren(v) {
			continue
		}
		node := &s.nl[v]
		t, b := s.nodeXY(node.borderTop), s.nodeXY(node.borderBottom)
		if !node.hasBorders {
			panic("dagro: compound node without border segments")
		}
		left, right := inone, inone
		if n := len(node.borderLeft); n > 0 {
			left = node.borderLeft[n-1]
		}
		if n := len(node.borderRight); n > 0 {
			right = node.borderRight[n-1]
		}
		l, r := s.nodeXY(left), s.nodeXY(right)
		node.width = math.Abs(r.X - l.X)
		node.height = math.Abs(b.Y - t.Y)
		node.x, node.hasX = l.X+node.width/2, true
		node.y, node.hasY = t.Y+node.height/2, true
	}
	for _, v := range all {
		if s.nl[v].dummy == dkBorder {
			s.buf = s.g.removeNode(v, s.key(), s.buf)
		}
	}
}

// nodeXY reads x and y of a node as num does: NaN for a missing node.
func (s *lstate) nodeXY(v int32) Point {
	if v == inone || !s.g.hasNode(v) {
		return Point{X: math.NaN(), Y: math.NaN()}
	}
	return Point{X: s.nl[v].x, Y: s.nl[v].y}
}

func (s *lstate) undoNormalize() {
	succ := s.buf[:0]
	for _, start := range s.dummyChains {
		v := start
		node := &s.nl[v]
		origLabel := node.edgeLabel
		origEdge := node.edgeObj
		s.setEdgeRef(origEdge, origLabel)
		for s.nl[v].dummy != dkNone {
			node := s.nl[v]
			succ = s.g.successors(v, s.key(), succ[:0])
			w := succ[0]
			s.buf = s.g.removeNode(v, s.key(), s.buf)
			s.setPoints(origLabel, append(s.el[origLabel].points, Point{X: node.x, Y: node.y}))
			l := &s.el[origLabel]
			if node.dummy == dkEdgeLabel {
				l.x, l.y, l.hasX, l.hasY = node.x, node.y, true, true
				l.width, l.height, l.hasWidth, l.hasHeight = node.width, node.height, true, true
			}
			v = w
		}
	}
	s.buf = succ[:0]
}

func (s *lstate) fixupEdgeLabelCoords() {
	for _, e := range s.edges() {
		l := &s.el[s.g.edges[e].label]
		if !l.hasX {
			continue
		}
		pos := l.labelpos
		if pos == "l" || pos == "r" {
			l.width = l.width - l.labeloffset
		}
		if pos == "l" {
			l.x = l.x - l.width/2 - l.labeloffset
		} else if pos == "r" {
			l.x = l.x + l.width/2 + l.labeloffset
		}
	}
}

func (s *lstate) translateGraph() {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := 0.0, 0.0
	marginX, marginY := s.marginx, s.marginy
	if !jsTruthyNumber(marginX) {
		marginX = 0
	}
	if !jsTruthyNumber(marginY) {
		marginY = 0
	}
	getExtremes := func(x, y, w, h float64) {
		minX, maxX = minJS2(minX, x-w/2), maxJS2(maxX, x+w/2)
		minY, maxY = minJS2(minY, y-h/2), maxJS2(maxY, y+h/2)
	}
	all := s.nodes()
	for _, v := range all {
		n := &s.nl[v]
		getExtremes(n.x, n.y, n.width, n.height)
	}
	edges := s.edges()
	for _, e := range edges {
		l := &s.el[s.g.edges[e].label]
		if l.hasX {
			w, h := l.width, l.height
			if !l.hasWidth {
				w = math.NaN()
			}
			if !l.hasHeight {
				h = math.NaN()
			}
			getExtremes(l.x, l.y, w, h)
		}
	}
	minX, minY = minX-marginX, minY-marginY
	for _, v := range all {
		n := &s.nl[v]
		n.x, n.y = n.x-minX, n.y-minY
		n.hasX, n.hasY = true, true
	}
	for _, e := range edges {
		l := &s.el[s.g.edges[e].label]
		for i := range l.points {
			l.points[i].X -= minX
			l.points[i].Y -= minY
		}
		l.hasPoints = true
		if l.hasX {
			l.x = l.x - minX
		}
		if l.hasY {
			l.y = l.y - minY
		}
	}
	s.width, s.height = maxX-minX+marginX, maxY-minY+marginY
}

func intersectRectXY(x, y, width, height float64, point Point) (Point, error) {
	dx, dy := point.X-x, point.Y-y
	w, h := width/2, height/2
	if !jsTruthyNumber(dx) && !jsTruthyNumber(dy) {
		if w == 0 || h == 0 {
			return Point{X: x, Y: y}, nil
		}
		return Point{}, errIntersectInside
	}
	var sx, sy float64
	if !jsTruthyNumber(dx) {
		if dy < 0 {
			h = -h
		}
		sx, sy = 0, h
	} else if !jsTruthyNumber(dy) {
		if dx < 0 {
			w = -w
		}
		sx, sy = w, 0
	} else if math.Abs(dy)*w > math.Abs(dx)*h {
		if dy < 0 {
			h = -h
		}
		sx, sy = h*dx/dy, h
	} else {
		if dx < 0 {
			w = -w
		}
		sx, sy = w, w*dy/dx
	}
	return Point{X: x + sx, Y: y + sy}, nil
}

func (s *lstate) assignNodeIntersects() error {
	for _, e := range s.edges() {
		ed := s.g.edges[e]
		if ed.v == ed.w {
			continue
		}
		l := &s.el[ed.label]
		nodeV, nodeW := &s.nl[ed.v], &s.nl[ed.w]
		points := l.points
		var p1, p2 Point
		if !l.hasPoints {
			points = []Point{}
			p1 = Point{X: nodeW.x, Y: nodeW.y}
			p2 = Point{X: nodeV.x, Y: nodeV.y}
		} else {
			p1, p2 = points[0], points[len(points)-1]
		}
		start, err := intersectRectXY(nodeV.x, nodeV.y, nodeV.width, nodeV.height, p1)
		if err != nil {
			return err
		}
		end, err := intersectRectXY(nodeW.x, nodeW.y, nodeW.width, nodeW.height, p2)
		if err != nil {
			return err
		}
		// [start] + points + [end], built in the label's own storage.
		out := append(points, Point{}, Point{})
		copy(out[1:], out[:len(points)])
		out[0], out[len(out)-1] = start, end
		s.setPoints(ed.label, out)
	}
	return nil
}

func (s *lstate) reversePointsForReversedEdges() {
	for _, e := range s.edges() {
		l := &s.el[s.g.edges[e].label]
		if !l.reversed {
			continue
		}
		points := l.points
		for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
			points[i], points[j] = points[j], points[i]
		}
	}
}

func (s *lstate) undoAcyclic() {
	for _, e := range s.edges() {
		ed := s.g.edges[e]
		l := &s.el[ed.label]
		if !l.reversed {
			continue
		}
		s.g.removeEdge(e)
		name, present := l.forwardName, l.forwardHasName
		l.reversed = false
		l.forwardName, l.forwardHasName = 0, false
		if present {
			s.g.setEdge(ed.w, ed.v, name, true, ed.label, true)
		} else {
			s.g.setEdge(ed.w, ed.v, 0, false, ed.label, true)
		}
	}
}

var errIntersectInside = errors.New("not possible to find intersection inside of the rectangle")

// setSelfLoop stores as label i's points the route of a self-loop around
// (x, y) on a node of the given width and height. It halves the size itself:
// where the compiler fuses a multiply and an add (arm64, for one), x +
// width/2 is then fused as in dagre's positionSelfEdges.
func (s *lstate) setSelfLoop(i int32, x, y, width, height float64) {
	s.setPoints(i, appendCenteredSelfEdgePoints(s.pointBuf(i), x, y, width/2, height/2))
}

func appendCenteredSelfEdgePoints(buf []Point, x, y, dx, dy float64) []Point {
	return append(buf,
		Point{X: x + dx, Y: y - dy},
		Point{X: x + dx, Y: y - dy},
		Point{X: x, Y: y},
		Point{X: x - dx, Y: y + dy},
		Point{X: x - dx, Y: y + dy},
		Point{X: x, Y: y},
		Point{X: x, Y: y},
	)
}

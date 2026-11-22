package dagro

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// The typed engine runs the same phases as runLayout on interned integer node
// ids and typed labels. Every phase below is a line-by-line port of the
// Attrs-based function of the same name; comments note where a JavaScript or
// Go map semantic is reproduced explicitly.

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
// presence flag; an absent attribute reads as NaN, like num on Attrs.
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

	inputNodes      []int32
	inputNodeLabels []Attrs
	inputEdges      []ekey
	inputEdgeLabels []any

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

	// drawn holds the values this layout has drawn from uniqueIDCounter, in
	// order (see draw and runGuarded).
	drawn []uint64
	// origPanic is set right before run performs an operation that fails
	// with the original's own runtime error (see failNilMapWrite).
	origPanic bool

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
	clear(s.inputNodeLabels)
	s.inputNodeLabels = s.inputNodeLabels[:0]
	clear(s.inputEdgeLabels)
	s.inputEdgeLabels = s.inputEdgeLabels[:0]
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

// draw is uniqueIDCounter.Add(1), the Attrs engine's draw: each value is
// visible to the whole process as soon as it is drawn, and concurrent layouts
// never draw the same value. The value is also recorded in s.drawn, for a
// decline to replay (see runGuarded).
func (s *lstate) draw() uint64 {
	v := uniqueIDCounter.Add(1)
	s.drawn = append(s.drawn, v)
	return v
}

// uniqueID is uniqueID(prefix), drawn through draw.
func (s *lstate) uniqueID(prefix string) string {
	return prefix + strconv.FormatUint(s.draw(), 10)
}

// errDecline is the panic value with which the typed engine gives up on an
// input whose layout it does not reproduce (see runGuarded).
var errDecline = errors.New("dagro: typed engine declined the input")

// runGuarded runs fn, which runs s.run, and reports whether it finished
// without panicking. run changes nothing outside s except the process-wide
// counter, which it draws as the Attrs engine does (draw), the same values in
// the same order.
//
// Where the original fails with a Go runtime error at a known site, run
// performs the original's failing operation itself (failNilMapWrite,
// makeRankList) and that panic goes through unchanged: the caller sees the
// original's panic value, the counter is at the original's value (run has
// made the original's draws up to that point) and nothing was printed (the
// original prints no DebugTiming line between buildLayoutGraph's and its
// panic). The caller drops s.
//
// On any other panic runGuarded recovers and reports false; the caller drops
// s and lets the Attrs engine, which is the original algorithm, lay out the
// untouched input with s.drawn replayed (drawReplay): its first draws return
// the values run drew, in order, as the original's first draws would have,
// and only its later draws take new values. So the Attrs engine raises the
// original's panic (the typed engine's own panics and errDecline mark inputs
// on which the original fails or which the typed engine does not reproduce),
// draws the counter as the original does, or produces the original's layout.
func (s *lstate) runGuarded(fn func() error) (err error, ok bool) {
	s.origPanic = false
	defer func() {
		if !ok && !s.origPanic {
			recover()
		}
	}()
	err = fn()
	return err, true
}

// failNilMapWrite is the original's write of key into the label of a node
// that is not in its graph: asAttrs(g.Node(v)) is a nil map for a missing
// node, so the write panics with Go's "assignment to entry in nil map". The
// sites (assignOrder, insertSelfEdges and positionY reaching a missing entry
// of a layer matrix) call it where the original makes that write.
func (s *lstate) failNilMapWrite(key string, value float64) {
	s.origPanic = true
	var label Attrs
	label[key] = value
}

// addDummy is addDummyNode: the plain prefix when free, otherwise the next
// free uniqueID(prefix). It consumes the process-wide counter exactly as the
// Attrs engine does.
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

// attrLookup resolves canonicalize(a)[keys[i]] for lowercase keys and
// reports whether that result is independent of Go's map iteration order.
// canonicalize stores every key of a under its lower-case form
// (strings.ToLower, which also maps some non-ASCII letters to ASCII ones) as
// it ranges over a, so of several spellings of one key (say "width" and
// "WIDTH") the one the range visits last wins: a random draw. attrLookup
// returns false when a holds two spellings of one of keys; build then
// declines the graph before it coerces any value of a, so that
// buildLayoutGraph's canonicalize makes the only draw, as in the original.
// Otherwise each of keys has at most one spelling in a and the result is
// fixed. When every key of a is one of keys, or every key is ASCII without
// upper-case letters (its own lower-case form, so one of keys, found
// directly, or no spelling of any), no key has two spellings and the direct
// lookups are the result.
func attrLookup(a Attrs, keys []string, out []any, found []bool) bool {
	n := 0
	for i, k := range keys {
		v, ok := a[k]
		out[i], found[i] = v, ok
		if ok {
			n++
		}
	}
	if n == len(a) {
		return true
	}
	plain := true
	for k := range a {
		if !lowerASCII(k) {
			plain = false
			break
		}
	}
	if plain {
		return true
	}
	for i := range keys {
		out[i], found[i] = nil, false
	}
	for k, v := range a {
		lk := strings.ToLower(k)
		for i, key := range keys {
			if lk == key {
				if found[i] {
					return false // a second spelling of keys[i]
				}
				out[i], found[i] = v, true
			}
		}
	}
	return true
}

// lowerASCII reports whether s is ASCII without upper-case letters, so that
// strings.ToLower(s) == s.
func lowerASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 0x80 || 'A' <= c && c <= 'Z' {
			return false
		}
	}
	return true
}

var typedGraphKeys = []string{"nodesep", "edgesep", "ranksep", "marginx", "marginy", "acyclicer", "ranker", "rankdir", "align"}
var typedNodeKeys = []string{"width", "height"}
var typedEdgeKeys = []string{"minlen", "weight", "width", "height", "labeloffset", "labelpos"}

func anyString(v any) string {
	s, _ := v.(string)
	return s
}

// typedEligible reports whether the typed engine reproduces Graph's string
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
	s.drawn = s.drawn[:0]
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
	s.inputNodeLabels = s.inputNodeLabels[:0]
	s.inputEdges = s.inputEdges[:0]
	s.inputEdgeLabels = s.inputEdgeLabels[:0]
}

// build is buildLayoutGraph. It returns false when the input needs the Attrs
// engine (see typedEligibleName, edges incident to compound nodes, which the
// Attrs engine rejects with a panic during ranking, and labels holding two
// spellings of an attribute key, see attrLookup). It coerces the labels in
// buildLayoutGraph's order, so a coercion panic is the original's; when it
// returns false, every label it has coerced holds at most one spelling of
// each key it reads, so buildLayoutGraph coerces the same values again
// without a random draw (number is a pure function of its argument), and its
// first random draw is the original's.
func (s *lstate) build(input *Graph) bool {
	inputNodes := input.Nodes()
	for _, v := range inputNodes {
		if !typedEligibleName(v) {
			return false
		}
	}
	inputEdges := input.Edges()
	s.begin(len(inputNodes), len(inputEdges))
	for _, v := range inputNodes {
		s.names.intern(v)
		s.noteInputName(v)
	}
	for _, e := range inputEdges {
		if e.HasName {
			s.noteInputEdgeName(e.Name)
		}
	}

	vals, found := s.vals[:], s.found[:]
	if a, ok := input.Graph().(Attrs); ok && a != nil {
		if !attrLookup(a, typedGraphKeys, vals, found) {
			return false
		}
		nums := [5]*float64{&s.nodesep, &s.edgesep, &s.ranksep, &s.marginx, &s.marginy}
		for i, p := range nums {
			if found[i] {
				*p = number(vals[i])
			}
		}
		strs := [4]*string{&s.acyclicer, &s.ranker, &s.rankdir, &s.align}
		for i, p := range strs {
			if found[5+i] {
				*p = anyString(vals[5+i])
			}
		}
	}

	for i, v := range inputNodes {
		id := int32(i)
		s.inputNodes = append(s.inputNodes, id)
		label := newNLabel()
		value := input.Node(v)
		a, ok := value.(Attrs)
		if ok && a != nil {
			if !attrLookup(a, typedNodeKeys, vals[:2], found[:2]) {
				return false
			}
			if found[0] {
				label.width = number(vals[0])
			}
			if found[1] {
				label.height = number(vals[1])
			}
		}
		s.inputNodeLabels = append(s.inputNodeLabels, a)
		s.ensureNode(id)
		s.nl[id] = label
		s.g.setNode(id)
		if parent, ok := input.Parent(v); ok {
			pid, _ := s.names.lookup(parent)
			s.g.setParent(id, pid)
		}
	}

	for _, e := range inputEdges {
		v, _ := s.names.lookup(e.V)
		w, _ := s.names.lookup(e.W)
		if s.g.hasChildren(v) || s.g.hasChildren(w) {
			return false
		}
		l := newELabel()
		l.minlen, l.weight, l.width, l.height, l.labeloffset, l.labelpos = 1, 1, 0, 0, 10, "r"
		l.hasWidth, l.hasHeight = true, true
		value := input.Edge(e)
		if a, ok := value.(Attrs); ok && a != nil {
			if !attrLookup(a, typedEdgeKeys, vals[:6], found[:6]) {
				return false
			}
			nums := [5]*float64{&l.minlen, &l.weight, &l.width, &l.height, &l.labeloffset}
			for j, p := range nums {
				if found[j] {
					*p = number(vals[j])
				}
			}
			if found[5] {
				l.labelpos = anyString(vals[5])
			}
		}
		name := int32(0)
		if e.HasName {
			name = s.edgeNameKey(e.Name)
		}
		s.g.setEdge(v, w, name, e.HasName, s.newEdgeLabel(l), true)
		s.inputEdges = append(s.inputEdges, ekey{v, w, name})
		s.inputEdgeLabels = append(s.inputEdgeLabels, value)
	}
	return true
}

// inputEdgeLabel returns the index of the layout label of input edge i. When
// the layout lost the edge or left it without points, updateInputGraph fails
// evaluating layoutLabel["points"].([]Point) on a nil map or on a label
// without points; inputEdgeLabel raises that same runtime error.
func (s *lstate) inputEdgeLabel(i int) int32 {
	k := s.inputEdges[i]
	le := s.g.findEdge(k.v, k.w, k.n)
	if le == inone || s.g.edges[le].label == inone || !s.el[s.g.edges[le].label].hasPoints {
		var layoutLabel Attrs
		_ = append([]Point(nil), layoutLabel["points"].([]Point)...)
	}
	return s.g.edges[le].label
}

// update is updateInputGraph. The input graph is not modified between build
// and update, so the labels recorded by build are the ones updateInputGraph
// would look up again, in the same node and edge order.
func (s *lstate) update(input *Graph) {
	for i, inputLabel := range s.inputNodeLabels {
		if inputLabel == nil {
			continue
		}
		id := s.inputNodes[i]
		n := &s.nl[id]
		inputLabel["x"], inputLabel["y"] = n.x, n.y
		if s.g.hasChildren(id) {
			inputLabel["width"], inputLabel["height"] = n.width, n.height
		}
	}
	for i, value := range s.inputEdgeLabels {
		inputLabel := asAttrs(value)
		li := s.inputEdgeLabel(i)
		l := &s.el[li]
		inputLabel["points"] = append([]Point(nil), l.points...)
		if l.hasX {
			inputLabel["x"], inputLabel["y"] = l.x, l.y
		}
	}
	inputAttrs := asAttrs(input.Graph())
	inputAttrs["width"], inputAttrs["height"] = s.width, s.height
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

// greedyFAS runs the Attrs implementation on a copy of the graph that has the
// same node order, edge order and weights; it is only used with
// acyclicer "greedy", which D2 does not set.
func (s *lstate) greedyFAS() []edgeRef {
	c := NewGraph(GraphOptions{Multigraph: true})
	names := make([]string, len(s.enames)+1)
	names[0] = defaultEdgeName
	for name, k := range s.enames {
		names[k] = name
	}
	for _, v := range s.nodes() {
		c.SetNode(s.names.name(v))
	}
	for _, e := range s.edges() {
		ed := s.g.edges[e]
		edge := Edge{V: s.names.name(ed.v), W: s.names.name(ed.w)}
		if ed.hasName {
			edge.Name, edge.HasName = names[ed.name], true
		}
		c.SetEdgeObject(edge, s.el[ed.label].weight)
	}
	found := greedyFAS(c, func(e Edge) float64 { return c.Edge(e).(float64) })
	out := make([]edgeRef, len(found))
	for i, e := range found {
		v, _ := s.names.lookup(e.V)
		w, _ := s.names.lookup(e.W)
		r := edgeRef{v: v, w: w, hasName: e.HasName}
		if e.HasName {
			r.name = s.edgeNameKey(e.Name)
		}
		out[i] = r
	}
	return out
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
	s.drawn = reserve(s.drawn, nodes)
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
				s.failNilMapWrite("rank", 0)
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
			dx, dy := width/2, height/2
			s.setPoints(label, appendCenteredSelfEdgePoints(s.pointBuf(label), nodeX, nodeY, dx, dy))
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
			dx := finiteOr(node.width, true, 0) / 2
			dy := finiteOr(node.height, true, 0) / 2
			s.setPoints(selfEdge.label, appendCenteredSelfEdgePoints(s.pointBuf(selfEdge.label), x, y, dx, dy))
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

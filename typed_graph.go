package dagro

import (
	"sort"
	"strconv"
)

// This file holds the integer-id graph used by the typed layout engine. It
// re-implements exactly the semantics of graphlib's Graph (insertion order,
// JavaScript property order, ordered predecessor/successor counters, children
// sets) on flat arrays and array-linked lists, so that the layout phases can be
// ported one to one without string-keyed maps.

const inone int32 = -1

// lnames interns the node names of one layout. Every name keeps the
// information needed to place it in JavaScript property order: canonical
// array-index names come first in numeric order, all others follow in
// insertion order.
//
// A generated dummy name prefix+counter can be stored lazily: str holds the
// prefix, ctr the counter (0 for ordinary names) and the name is not in ids.
// Such names are ASCII and never array indices.
type lnames struct {
	str []string
	key []int64 // array-index value, or -1 for an ordinary property name
	ctr []uint64
	off []int32 // lazy names: offset of the built name in buf
	buf []byte
	ids map[string]int32
}

// reset empties the table for reuse; it drops the string references so a
// pooled table does not keep a previous caller's names alive.
func (n *lnames) reset() {
	clear(n.str)
	n.str = n.str[:0]
	n.key = n.key[:0]
	n.ctr = n.ctr[:0]
	n.off = n.off[:0]
	n.buf = n.buf[:0]
	if n.ids == nil {
		n.ids = make(map[string]int32)
	} else {
		clear(n.ids)
	}
}

// reserve makes room for k more names, most of them lazy ones.
func (n *lnames) reserve(k int) {
	n.str = reserve(n.str, k)
	n.key = reserve(n.key, k)
	n.ctr = reserve(n.ctr, k)
	n.off = reserve(n.off, k)
	n.buf = reserve(n.buf, 12*k) // "_d" + digits + NUL
}

func (n *lnames) lookup(s string) (int32, bool) {
	id, ok := n.ids[s]
	return id, ok
}

func (n *lnames) intern(s string) int32 {
	if id, ok := n.ids[s]; ok {
		return id
	}
	id := int32(len(n.str))
	n.str = appendGrow(n.str, s)
	k := int64(-1)
	if v, ok := jsArrayIndex(s); ok {
		k = int64(v)
	}
	n.key = appendGrow(n.key, k)
	n.ctr = appendGrow(n.ctr, 0)
	n.off = appendGrow(n.off, 0)
	n.ids[s] = id
	return id
}

// internLazy adds the generated name prefix+counter without building it.
// The caller guarantees that no other node can have that name.
func (n *lnames) internLazy(prefix string, counter uint64) int32 {
	id := int32(len(n.str))
	n.str = appendGrow(n.str, prefix)
	n.key = appendGrow(n.key, -1)
	n.ctr = appendGrow(n.ctr, counter)
	n.off = appendGrow(n.off, int32(len(n.buf)))
	n.buf = strconv.AppendUint(append(n.buf, prefix...), counter, 10)
	n.buf = append(n.buf, 0) // terminator: names never contain NUL here
	return id
}

// lazyBytes returns the built name of lazy id (without the terminator).
func (n *lnames) lazyBytes(id int32) []byte {
	b := n.buf[n.off[id]:]
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// name returns the name of id, building a lazy name.
func (n *lnames) name(id int32) string {
	if c := n.ctr[id]; c != 0 {
		return n.str[id] + strconv.FormatUint(c, 10)
	}
	return n.str[id]
}

// cmp compares the names of a and b in Go string (byte) order.
func (n *lnames) cmp(a, b int32) int {
	ca, cb := n.ctr[a], n.ctr[b]
	if ca == 0 && cb == 0 {
		sa, sb := n.str[a], n.str[b]
		switch {
		case sa < sb:
			return -1
		case sa > sb:
			return 1
		}
		return 0
	}
	var x, y []byte
	if ca != 0 {
		x = n.lazyBytes(a)
	}
	if cb != 0 {
		y = n.lazyBytes(b)
	}
	switch {
	case ca != 0 && cb != 0:
		return cmpBytes(x, y)
	case ca != 0:
		return cmpBytesString(x, n.str[b])
	default:
		return -cmpBytesString(y, n.str[a])
	}
}

// greater is names[a] > names[b] in Go string order.
func (n *lnames) greater(a, b int32) bool { return n.cmp(a, b) > 0 }

// jsGreater is jsStringGreater(names[a], names[b]). A lazy name is ASCII, and
// comparing an ASCII string with any string byte by byte gives the UTF-16
// code-unit order: the first difference is either two ASCII characters or an
// ASCII byte against a multi-byte lead byte (>= 0xC2), i.e. against a code
// unit >= 0x80.
func (n *lnames) jsGreater(a, b int32) bool {
	if n.ctr[a] == 0 && n.ctr[b] == 0 {
		return jsStringGreater(n.str[a], n.str[b])
	}
	return n.cmp(a, b) > 0
}

func cmpBytes(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

func cmpBytesString(a []byte, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// jsOrderIDs reorders ids in place into JavaScript property order, exactly as
// jsObjectKeyOrder does for the corresponding names: array-index names first
// in ascending numeric order, then all other names in their current order.
func jsOrderIDs(ids []int32, key []int64) {
	if len(ids) < 2 {
		return
	}
	inPlace := true
	sawNonIndex := false
	indexCount := 0
	var previous int64
	for _, id := range ids {
		k := key[id]
		if k < 0 {
			sawNonIndex = true
			continue
		}
		if sawNonIndex || indexCount > 0 && k < previous {
			inPlace = false
		}
		previous = k
		indexCount++
	}
	if inPlace {
		return
	}
	if len(ids) <= 64 {
		// Stable insertion sort under "array-index names first, by value;
		// other names equivalent": the unique result of the partition below.
		for i := 1; i < len(ids); i++ {
			x := ids[i]
			kx := key[x]
			if kx < 0 {
				continue
			}
			j := i
			for j > 0 {
				if ky := key[ids[j-1]]; ky >= 0 && ky < kx {
					break
				}
				ids[j] = ids[j-1]
				j--
			}
			ids[j] = x
		}
		return
	}
	// Array-index names are distinct, so their numeric order is total.
	indices := make([]int32, 0, indexCount)
	rest := make([]int32, 0, len(ids)-indexCount)
	for _, id := range ids {
		if key[id] >= 0 {
			indices = append(indices, id)
		} else {
			rest = append(rest, id)
		}
	}
	sort.Slice(indices, func(i, j int) bool { return key[indices[i]] < key[indices[j]] })
	copy(ids, indices)
	copy(ids[len(indices):], rest)
}

type inode struct {
	prev, next       int32 // node insertion-order list
	parent           int32 // compound parent, inone for the graph root
	sprev, snext     int32 // sibling links in the parent's children list
	chead, ctail     int32 // own children list
	inHead, inTail   int32 // incoming edges, insertion order
	outHead, outTail int32 // outgoing edges, insertion order
	inDeg, outDeg    int32 // lengths of the incoming and outgoing lists
	pHead, pTail     int32 // predecessor counter entries
	sHead, sTail     int32 // successor counter entries
	has              bool
}

type icount struct {
	node, count, prev, next int32
}

type iedge struct {
	v, w, name                       int32
	label                            int32
	inPrev, inNext, outPrev, outNext int32
	pEnt, sEnt                       int32
	hasName, alive                   bool
}

type ekey struct{ v, w, n int32 }

// igraph is a directed graph over interned node ids with graphlib's ordering
// semantics. Edge identity is (v, w, name key), name key 0 being the default
// name; that matches graphlib's string edge ids whenever no node name contains
// the id delimiter, which the typed engine requires. Edges are found by
// scanning the shorter of v's outgoing and w's incoming lists, which hold
// exactly the live edges, so a key finds the one live edge it identifies.
type igraph struct {
	multigraph, compound bool
	nodes                []inode
	nhead, ntail         int32
	ncount               int
	rhead, rtail         int32 // children of the graph root
	edges                []iedge
	ecount               int
	counts               []icount
	freeCount            int32
}

func newIGraph(multigraph, compound bool, nodeCapacity, edgeCapacity int) *igraph {
	g := &igraph{
		multigraph: multigraph, compound: compound,
		nodes: make([]inode, 0, nodeCapacity),
		nhead: inone, ntail: inone, rhead: inone, rtail: inone,
		edges:     make([]iedge, 0, edgeCapacity),
		counts:    make([]icount, 0, 2*edgeCapacity),
		freeCount: inone,
	}
	return g
}

// reset empties g for reuse, keeping its storage.
func (g *igraph) reset(multigraph, compound bool) {
	g.multigraph, g.compound = multigraph, compound
	g.nodes = g.nodes[:0]
	g.nhead, g.ntail, g.rhead, g.rtail = inone, inone, inone, inone
	g.ncount, g.ecount = 0, 0
	g.edges = g.edges[:0]
	g.counts = g.counts[:0]
	g.freeCount = inone
}

func (g *igraph) grow(id int32) {
	for int32(len(g.nodes)) <= id {
		g.nodes = appendGrow(g.nodes, inode{
			prev: inone, next: inone, parent: inone, sprev: inone, snext: inone,
			chead: inone, ctail: inone, inHead: inone, inTail: inone, outHead: inone, outTail: inone,
			pHead: inone, pTail: inone, sHead: inone, sTail: inone,
		})
	}
}

func (g *igraph) hasNode(v int32) bool { return v >= 0 && int(v) < len(g.nodes) && g.nodes[v].has }

// setNode adds v when absent and reports whether it was added.
func (g *igraph) setNode(v int32) bool {
	g.grow(v)
	n := &g.nodes[v]
	if n.has {
		return false
	}
	n.has = true
	n.prev, n.next = g.ntail, inone
	if g.ntail != inone {
		g.nodes[g.ntail].next = v
	} else {
		g.nhead = v
	}
	g.ntail = v
	g.ncount++
	if g.compound {
		n.parent = inone
		g.appendChild(inone, v)
	}
	return true
}

func (g *igraph) appendChild(p, v int32) {
	n := &g.nodes[v]
	if p == inone {
		n.sprev, n.snext = g.rtail, inone
		if g.rtail != inone {
			g.nodes[g.rtail].snext = v
		} else {
			g.rhead = v
		}
		g.rtail = v
		return
	}
	pn := &g.nodes[p]
	n.sprev, n.snext = pn.ctail, inone
	if pn.ctail != inone {
		g.nodes[pn.ctail].snext = v
	} else {
		pn.chead = v
	}
	pn.ctail = v
}

func (g *igraph) unlinkChild(v int32) {
	n := &g.nodes[v]
	p := n.parent
	if n.sprev != inone {
		g.nodes[n.sprev].snext = n.snext
	} else if p == inone {
		g.rhead = n.snext
	} else {
		g.nodes[p].chead = n.snext
	}
	if n.snext != inone {
		g.nodes[n.snext].sprev = n.sprev
	} else if p == inone {
		g.rtail = n.sprev
	} else {
		g.nodes[p].ctail = n.sprev
	}
	n.sprev, n.snext = inone, inone
}

// setParent mirrors graphlib's setParent without the cycle check; p == inone moves
// v to the graph root. Like Graphlib, re-parenting moves v to the end of the
// new parent's children.
func (g *igraph) setParent(v, p int32) {
	if p != inone {
		g.setNode(p)
	}
	g.setNode(v)
	g.unlinkChild(v)
	g.nodes[v].parent = p
	g.appendChild(p, v)
}

func (g *igraph) parentOf(v int32) int32 { return g.nodes[v].parent }

func (g *igraph) hasChildren(v int32) bool { return g.nodes[v].chead != inone }

// children appends v's children (the root's for v == inone) to buf in
// JavaScript order.
func (g *igraph) children(v int32, key []int64, buf []int32) []int32 {
	start := len(buf)
	c := g.rhead
	if v != inone {
		c = g.nodes[v].chead
	}
	for ; c != inone; c = g.nodes[c].snext {
		buf = append(buf, c)
	}
	jsOrderIDs(buf[start:], key)
	return buf
}

// nodeList appends all nodes to buf in JavaScript order.
func (g *igraph) nodeList(key []int64, buf []int32) []int32 {
	start := len(buf)
	for v := g.nhead; v != inone; v = g.nodes[v].next {
		buf = append(buf, v)
	}
	jsOrderIDs(buf[start:], key)
	return buf
}

func (g *igraph) newCount(node int32) int32 {
	if g.freeCount != inone {
		i := g.freeCount
		g.freeCount = g.counts[i].next
		g.counts[i] = icount{node: node, count: 1, prev: inone, next: inone}
		return i
	}
	g.counts = appendGrow(g.counts, icount{node: node, count: 1, prev: inone, next: inone})
	return int32(len(g.counts) - 1)
}

// appendCount is orderedCounter.inc on the list (*head, *tail) for a node
// that has no entry yet: a new entry with count 1 at the end.
func (g *igraph) appendCount(head, tail *int32, node int32) int32 {
	i := g.newCount(node)
	g.counts[i].prev = *tail
	if *tail != inone {
		g.counts[*tail].next = i
	} else {
		*head = i
	}
	*tail = i
	return i
}

func (g *igraph) decCount(head, tail *int32, i int32) {
	c := &g.counts[i]
	if c.count > 1 {
		c.count--
		return
	}
	if c.prev != inone {
		g.counts[c.prev].next = c.next
	} else {
		*head = c.next
	}
	if c.next != inone {
		g.counts[c.next].prev = c.prev
	} else {
		*tail = c.prev
	}
	c.next = g.freeCount
	g.freeCount = i
}

func (g *igraph) successors(v int32, key []int64, buf []int32) []int32 {
	start := len(buf)
	for i := g.nodes[v].sHead; i != inone; i = g.counts[i].next {
		buf = append(buf, g.counts[i].node)
	}
	jsOrderIDs(buf[start:], key)
	return buf
}

// findEdge returns the live edge (v, w, name), or inone.
func (g *igraph) findEdge(v, w, name int32) int32 {
	if v < 0 || int(v) >= len(g.nodes) || w < 0 || int(w) >= len(g.nodes) {
		return inone
	}
	if vn, wn := &g.nodes[v], &g.nodes[w]; vn.outDeg <= wn.inDeg {
		for e := vn.outHead; e != inone; e = g.edges[e].outNext {
			if ed := &g.edges[e]; ed.w == w && ed.name == name {
				return e
			}
		}
	} else {
		for e := wn.inHead; e != inone; e = g.edges[e].inNext {
			if ed := &g.edges[e]; ed.v == v && ed.name == name {
				return e
			}
		}
	}
	return inone
}

// findAnyEdge returns a live edge from v to w of any name, or inone.
func (g *igraph) findAnyEdge(v, w int32) int32 {
	if vn, wn := &g.nodes[v], &g.nodes[w]; vn.outDeg <= wn.inDeg {
		for e := vn.outHead; e != inone; e = g.edges[e].outNext {
			if g.edges[e].w == w {
				return e
			}
		}
	} else {
		for e := wn.inHead; e != inone; e = g.edges[e].inNext {
			if g.edges[e].v == v {
				return e
			}
		}
	}
	return inone
}

// setEdge mirrors graphlib's setEdge: an existing edge only gets its label
// replaced (when labelSet), a new one is appended to every order. It returns
// the edge index.
func (g *igraph) setEdge(v, w, name int32, hasName bool, label int32, labelSet bool) int32 {
	if e := g.findEdge(v, w, name); e != inone {
		if labelSet {
			g.edges[e].label = label
		}
		return e
	}
	if name != 0 && !g.multigraph {
		panic("dagro: cannot set a named edge when Multigraph is false")
	}
	g.setNode(v)
	g.setNode(w)
	if !labelSet {
		label = inone
	}
	// orderedCounter.inc on w's predecessors and v's successors: in a
	// multigraph the entries of another live edge from v to w are the ones
	// incCount would find (an entry lives exactly as long as such an edge),
	// otherwise new entries go at the ends of the lists.
	pEnt, sEnt := inone, inone
	if g.multigraph {
		if other := g.findAnyEdge(v, w); other != inone {
			pEnt, sEnt = g.edges[other].pEnt, g.edges[other].sEnt
			g.counts[pEnt].count++
			g.counts[sEnt].count++
		}
	}
	e := int32(len(g.edges))
	g.edges = appendGrow(g.edges, iedge{v: v, w: w, name: name, hasName: hasName, label: label, alive: true,
		inPrev: inone, inNext: inone, outPrev: inone, outNext: inone})
	g.ecount++
	if pEnt == inone {
		wn := &g.nodes[w]
		pEnt = g.appendCount(&wn.pHead, &wn.pTail, v)
		vn := &g.nodes[v]
		sEnt = g.appendCount(&vn.sHead, &vn.sTail, w)
	}
	g.edges[e].pEnt, g.edges[e].sEnt = pEnt, sEnt
	// incoming list of w
	ed := &g.edges[e]
	wn := &g.nodes[w]
	ed.inPrev = wn.inTail
	if wn.inTail != inone {
		g.edges[wn.inTail].inNext = e
	} else {
		wn.inHead = e
	}
	wn.inTail = e
	wn.inDeg++
	vn := &g.nodes[v]
	ed.outPrev = vn.outTail
	if vn.outTail != inone {
		g.edges[vn.outTail].outNext = e
	} else {
		vn.outHead = e
	}
	vn.outTail = e
	vn.outDeg++
	return e
}

func (g *igraph) removeEdge(e int32) {
	ed := &g.edges[e]
	if !ed.alive {
		return
	}
	ed.alive = false
	g.ecount--
	g.nodes[ed.w].inDeg--
	g.nodes[ed.v].outDeg--
	wn := &g.nodes[ed.w]
	g.decCount(&wn.pHead, &wn.pTail, ed.pEnt)
	vn := &g.nodes[ed.v]
	g.decCount(&vn.sHead, &vn.sTail, ed.sEnt)
	wn = &g.nodes[ed.w]
	if ed.inPrev != inone {
		g.edges[ed.inPrev].inNext = ed.inNext
	} else {
		wn.inHead = ed.inNext
	}
	if ed.inNext != inone {
		g.edges[ed.inNext].inPrev = ed.inPrev
	} else {
		wn.inTail = ed.inPrev
	}
	vn = &g.nodes[ed.v]
	if ed.outPrev != inone {
		g.edges[ed.outPrev].outNext = ed.outNext
	} else {
		vn.outHead = ed.outNext
	}
	if ed.outNext != inone {
		g.edges[ed.outNext].outPrev = ed.outPrev
	} else {
		vn.outTail = ed.outPrev
	}
}

// removeNode mirrors graphlib's removeNode: incident edges (incoming first) are
// removed, children move to the graph root in JavaScript order.
func (g *igraph) removeNode(v int32, key []int64, scratch []int32) []int32 {
	if !g.hasNode(v) {
		return scratch
	}
	for e := g.nodes[v].inHead; e != inone; {
		next := g.edges[e].inNext
		g.removeEdge(e)
		e = next
	}
	for e := g.nodes[v].outHead; e != inone; {
		next := g.edges[e].outNext
		g.removeEdge(e)
		e = next
	}
	if g.compound {
		g.unlinkChild(v)
		scratch = g.children(v, key, scratch[:0])
		for _, c := range scratch {
			g.setParent(c, inone)
		}
		g.nodes[v].parent = inone
	}
	n := &g.nodes[v]
	if n.prev != inone {
		g.nodes[n.prev].next = n.next
	} else {
		g.nhead = n.next
	}
	if n.next != inone {
		g.nodes[n.next].prev = n.prev
	} else {
		g.ntail = n.prev
	}
	g.nodes[v] = inode{
		prev: inone, next: inone, parent: inone, sprev: inone, snext: inone,
		chead: inone, ctail: inone, inHead: inone, inTail: inone, outHead: inone, outTail: inone,
		pHead: inone, pTail: inone, sHead: inone, sTail: inone,
	}
	g.ncount--
	return scratch
}

// edgeList appends the live edges in insertion order.
func (g *igraph) edgeList(buf []int32) []int32 {
	for i := range g.edges {
		if g.edges[i].alive {
			buf = append(buf, int32(i))
		}
	}
	return buf
}

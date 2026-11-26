package dagro

import (
	"reflect"
	"strconv"
)

// The typed engine keeps a state between calls (lstateCache survives garbage
// collections; lstatePool does not in Go, but TinyGo's sync.Pool never frees
// what it holds). Its buffers keep the largest size any call needed, and the
// per-index ones (points per edge label, the layer matrix, the layer graphs,
// the sort levels, the rank buckets) the largest size per index, so a mix of
// graphs can make a state larger than any single graph needs. putLState puts
// a state in lstateCache only while retainedBytes is at most
// lstateKeepMaxBytes, which bounds what stays reachable across garbage
// collections (the D2 corpus needs at most about 2.3 MB); a larger state goes
// to lstatePool under the gc toolchain (the collector empties it) and is
// dropped under TinyGo (see poolOversized).
const lstateKeepMaxBytes = 4 << 20

// Element sizes of the state's buffers (reflect instead of unsafe.Sizeof).
var (
	szInt     = strconv.IntSize / 8
	szPtr     = int(reflect.TypeOf((*int)(nil)).Size())
	szSlice   = int(reflect.TypeOf([]int32(nil)).Size())
	szString  = int(reflect.TypeOf("").Size())
	szIface   = int(reflect.TypeOf((*any)(nil)).Elem().Size())
	szNLabel  = int(reflect.TypeOf(nlabel{}).Size())
	szELabel  = int(reflect.TypeOf(elabel{}).Size())
	szINode   = int(reflect.TypeOf(inode{}).Size())
	szIEdge   = int(reflect.TypeOf(iedge{}).Size())
	szICount  = int(reflect.TypeOf(icount{}).Size())
	szEKey    = int(reflect.TypeOf(ekey{}).Size())
	szPoint   = int(reflect.TypeOf(Point{}).Size())
	szPostNum = int(reflect.TypeOf(postorderNum{}).Size())
	szLayer   = int(reflect.TypeOf(layerGraph{}).Size())
	szLevel   = int(reflect.TypeOf(sortLevel{}).Size())
	szEntry   = int(reflect.TypeOf(typedOrderEntry{}).Size())
	szSubRec  = int(reflect.TypeOf(subRec{}).Size())
	szCNode   = int(reflect.TypeOf(cnode{}).Size())
	szPoolEnt = int(reflect.TypeOf(poolEnt{}).Size())
	szPair    = int(reflect.TypeOf(typedReversedPair{}).Size())
	szChild   = int(reflect.TypeOf(childRec{}).Size())
	szInRec   = int(reflect.TypeOf(inRec{}).Size())
	szFrame   = int(reflect.TypeOf(ioFrame{}).Size())
	szSouth   = int(reflect.TypeOf(southEntry{}).Size())
	szConfl   = int(reflect.TypeOf(bkConflict{}).Size())
	szLP      = int(reflect.TypeOf(lpFrame{}).Size())
	szFT      = int(reflect.TypeOf(ftFrame{}).Size())
	szLL      = int(reflect.TypeOf(lowLimFrame{}).Size())
	szTEntry  = int(reflect.TypeOf(tentry{}).Size())
	szState   = int(reflect.TypeOf(lstate{}).Size())
	szIGraph  = int(reflect.TypeOf(igraph{}).Size())
)

// Map storage per entry, generously: a string-keyed map's slot (key header and
// value) plus control bytes and growth slack; a uint64 set's likewise.
var (
	szStrMapEntry = 3*szString + 8
	szU64SetEntry = 32
)

// retainedBytes estimates the memory the state keeps reachable: the capacity
// of every buffer it holds, the maps' storage (bounded through the buffers
// that limit their sizes) and the structs themselves. Strings the state holds
// belong to the caller and are dropped by release. It reads a few hundred
// capacities: the per-index buffers with many entries keep running totals
// (ptbufCap, orderState.layerBytes).
func (s *lstate) retainedBytes() int {
	n := szState

	// Names: every interned name has a slot in str; ids holds at most one
	// entry per slot, enames at most one per edge label.
	nm := &s.names
	n += cap(nm.str)*(szString+szStrMapEntry) + 8*(cap(nm.key)+cap(nm.ctr)) + 4*cap(nm.off) + cap(nm.buf)
	n += cap(s.el) * szStrMapEntry

	if g := s.g; g != nil {
		n += szIGraph + cap(g.nodes)*szINode + cap(g.edges)*szIEdge + cap(g.counts)*szICount
	}
	n += cap(s.nl)*szNLabel + cap(s.el)*szELabel
	n += 4 * (cap(s.dummyChains) + cap(s.inputNodes) + cap(s.buf) + cap(s.nodesBuf) + cap(s.edgesBuf) + cap(s.stack) +
		cap(s.vPath) + cap(s.wPath) + cap(s.parentBuf) + cap(s.borderBuf))
	n += cap(s.inputEdges) * szEKey
	n += cap(s.ptbuf)*szSlice + s.ptbufCap*szPoint
	n += (cap(s.depths)+cap(s.layerOf))*szInt + cap(s.nonEmpty) + cap(s.seenBuf) + 8*cap(s.deltas) +
		cap(s.nums)*szPostNum
	// The inner storage of the per-rank layerings is s.rankBytes.
	n += (cap(s.lm) + cap(s.o.buckets) + cap(s.o.init) + cap(s.o.layering) + cap(s.o.best) +
		cap(s.p.adjusted)) * szSlice
	n += s.rankBytes

	o := &s.o
	n += cap(o.layers)*szLayer + o.layerBytes
	n += 4 * (cap(o.nodes) + cap(o.local) + cap(o.stamp) + cap(o.mapped) + cap(o.mstamp) + cap(o.subIdx) +
		cap(o.sstamp) + cap(o.prevOf) + cap(o.pstamp) + cap(o.posOf) + cap(o.posGen) + cap(o.fill) + cap(o.globals) +
		cap(o.simple) + cap(o.ordered) + cap(o.succStack) + cap(o.rankStart) + cap(o.rankSorted))
	// cgSeen holds one entry per entry of cgEdges in a sweep.
	n += cap(o.cgEdges) * (8 + szU64SetEntry)
	n += 8*cap(o.tree) + cap(o.sents)*szSouth + cap(o.levels)*szPtr + len(o.levels)*szLevel + o.levelBytes
	n += (cap(o.childRecs)+cap(o.parents))*szChild + cap(o.inRecs)*szInRec + cap(o.visited) + cap(o.frames)*szFrame +
		cap(o.sizes)*szInt

	p := &s.p
	n += 4 * (cap(p.predStart) + cap(p.predList) + cap(p.sucStart) + cap(p.sucList) + cap(p.nodes) + cap(p.chead) +
		cap(p.pos) + cap(p.posStamp) + cap(p.root) + cap(p.align) + cap(p.leaves) + cap(p.fill) + cap(p.compactTo) +
		cap(p.newStart) + cap(p.ws) + cap(p.order) + cap(p.stack) + cap(p.next))
	n += cap(p.cents)*szConfl + cap(p.type2Key) + cap(p.visited) + cap(p.member)
	n += 8 * (cap(p.xs[0]) + cap(p.xs[1]) + cap(p.xs[2]) + cap(p.xs[3]))
	b := &p.bk
	n += 4 * (cap(b.nodes) + cap(b.stamp) + cap(b.ev) + cap(b.ew) + cap(b.firstOut) + cap(b.nextOut) + cap(b.inStart) +
		cap(b.inList) + cap(b.outStart) + cap(b.outList) + cap(b.local) + cap(b.fill) + cap(b.predOrd) +
		cap(b.sucOrd))
	n += 8 * cap(b.weight)

	r := &s.r
	for _, rg := range [2]*rgraph{&r.full, &r.simple} {
		n += 4 * (cap(rg.ids) + cap(rg.ev) + cap(rg.ew) + cap(rg.inStart) + cap(rg.inList) + cap(rg.outStart) +
			cap(rg.outList) + cap(rg.fill))
		n += 8*(cap(rg.weight)+cap(rg.minlen)+cap(rg.rank)) + cap(rg.hasRank)
	}
	t := &r.tree
	n += cap(t.in) + cap(t.alive) + cap(t.visited) + 8*cap(t.cut) + cap(t.ent)*szTEntry
	n += 4 * (cap(t.idxNodes) + cap(t.others) + cap(t.ea) + cap(t.eb) + cap(t.eg) + cap(t.entA) + cap(t.entB) +
		cap(t.pHead) + cap(t.pTail) + cap(t.sHead) + cap(t.sTail) + cap(t.low) + cap(t.lim) + cap(t.parent) +
		cap(t.parentEdge) + cap(t.pre) + cap(t.post) + cap(t.preIdx) + cap(t.remap))
	n += 4 * (cap(r.local) + cap(r.firstOut) + cap(r.nextOut) + cap(r.snapshot) + cap(r.nbr) + cap(r.idx) +
		cap(r.rest) + cap(r.opp))
	n += cap(r.visited) + cap(r.lp)*szLP + cap(r.ft)*szFT + cap(r.ll)*szLL
	return n
}

// capsLayers is the total capacity of the layers.
func capsLayers(layers [][]int32) int {
	n := 0
	for _, l := range layers {
		n += cap(l)
	}
	return n
}

// storageBytes is the storage of the level's buffers. The vs lists of the
// entries in resolved, pairs, sortable and unsortable are slices of expVS and
// resVS; entries left from earlier calls can keep earlier, smaller versions of
// those two arrays reachable. append grows a full slice at least 1.25 times,
// so all earlier versions together hold at most 4 times the current capacity,
// which is counted as well.
func (L *sortLevel) storageBytes() int {
	n := 4 * (cap(L.movable) + cap(L.subVS) + cap(L.vsNext) + cap(L.order) + cap(L.sourceSet) + cap(L.processed) +
		cap(L.resVS) + cap(L.expVS) + cap(L.insKeys) + cap(L.out) + cap(L.final))
	n += 4 * 4 * (cap(L.expVS) + cap(L.resVS))
	n += (cap(L.bary) + cap(L.resolved) + cap(L.sortable) + cap(L.unsortable)) * szEntry
	n += cap(L.subs)*szSubRec + cap(L.cn)*szCNode + (cap(L.inPool)+cap(L.outPool))*szPoolEnt
	n += (cap(L.offs)+cap(L.insCounts))*szInt + cap(L.pairs)*szPair + 8*cap(L.baryVals)
	return n
}

// storageBytes is the storage of the layer graph's buffers.
func (lg *layerGraph) storageBytes() int {
	n := 4 * (cap(lg.ids) + cap(lg.parent) + cap(lg.childStart) + cap(lg.childList) + cap(lg.bl) + cap(lg.br) +
		cap(lg.inStart) + cap(lg.inU))
	return n + cap(lg.replaced) + 8*cap(lg.inW)
}

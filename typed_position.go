package dagro

import (
	"math"
	"sort"
	"strings"
)

// posState holds asNonCompoundGraph(g) as position reads it: the leaves of g
// in JavaScript order and their predecessor/successor lists, built from g's
// edges in insertion order exactly like the copy the Attrs engine makes.
type posState struct {
	s         *lstate
	predStart []int32 // per global id, valid for leaves
	predList  []int32
	sucStart  []int32
	sucList   []int32
	nodes     []int32 // all layering nodes
	chead     []int32 // per node: first entry of its conflict list, inone when empty
	cents     []bkConflict
	type2Key  []bool
	pos       []int32
	posStamp  []int32
	posGen    int32
	root      []int32
	align     []int32
	xs        [4][]float64
	member    []uint8 // per node: bit a when the node is a key of alignment a's xs map
	bk        blockGraph
	leaves    []int32
	fill      []int32
	compactTo []int32
	newStart  []int32
	adjusted  [][]int32
	ws        []int32
	order     []int32
	visited   []bool
	stack     []int32
	next      []int32
}

func (s *lstate) position() {
	p := &s.p
	p.s = s
	all := s.nodes()
	leaves := p.leaves[:0]
	for _, v := range all {
		if !s.g.hasChildren(v) {
			leaves = append(leaves, v)
		}
	}
	p.leaves = leaves
	p.buildNeighbors()
	p.positionY(leaves)
	p.positionX(leaves)
}

func (p *posState) buildNeighbors() {
	s := p.s
	n := len(s.nl)
	edges := s.edges()
	predCount := resize(p.predStart, n+1)
	sucCount := resize(p.sucStart, n+1)
	for _, e := range edges {
		ed := &s.g.edges[e]
		predCount[ed.w+1]++
		sucCount[ed.v+1]++
	}
	for i := 0; i < n; i++ {
		predCount[i+1] += predCount[i]
		sucCount[i+1] += sucCount[i]
	}
	p.predStart, p.sucStart = predCount, sucCount
	p.predList = grow(p.predList, len(edges))
	p.sucList = grow(p.sucList, len(edges))
	p.fill = grow(p.fill, 2*n)
	predFill, sucFill := p.fill[:n], p.fill[n:]
	copy(predFill, predCount[:n])
	copy(sucFill, sucCount[:n])
	for _, e := range edges {
		ed := &s.g.edges[e]
		// orderedCounter.inc: a neighbor is listed once, at its first edge.
		dup := false
		for i := p.predStart[ed.w]; i < predFill[ed.w]; i++ {
			if p.predList[i] == ed.v {
				dup = true
				break
			}
		}
		if !dup {
			p.predList[predFill[ed.w]] = ed.v
			predFill[ed.w]++
		}
		dup = false
		for i := p.sucStart[ed.v]; i < sucFill[ed.v]; i++ {
			if p.sucList[i] == ed.w {
				dup = true
				break
			}
		}
		if !dup {
			p.sucList[sucFill[ed.v]] = ed.w
			sucFill[ed.v]++
		}
	}
	// Shrink each list to its distinct entries and put it in JavaScript order.
	key := s.key()
	// In place: entries only move toward the front.
	compact := func(start []int32, list []int32, fill []int32) []int32 {
		out := int32(0)
		for v := 0; v < n; v++ {
			from, to := start[v], fill[v]
			start[v] = out
			copy(list[out:], list[from:to])
			jsOrderIDs(list[out:out+to-from], key)
			out += to - from
		}
		start[n] = out
		return list[:out]
	}
	p.predList = compact(p.predStart, p.predList, predFill)
	p.sucList = compact(p.sucStart, p.sucList, sucFill)
}

func (p *posState) preds(v int32) []int32 { return p.predList[p.predStart[v]:p.predStart[v+1]] }
func (p *posState) sucs(v int32) []int32  { return p.sucList[p.sucStart[v]:p.sucStart[v+1]] }

func (p *posState) positionY(leaves []int32) {
	s := p.s
	layering := s.buildLayerMatrix(leaves)
	rankSep := s.ranksep
	prevY := 0.0
	for _, layer := range layering {
		maxHeight := 0.0
		for _, v := range layer {
			height := 0.0
			if v != inone {
				height = s.nl[v].height
			}
			if !(maxHeight > height) {
				maxHeight = height
			}
		}
		for _, v := range layer {
			if v == inone {
				// The original's asAttrs(g.Node(v))["y"] = ... on the
				// missing entry's nil label.
				s.failNilMapWrite("y", prevY+maxHeight/2)
			}
			s.nl[v].y, s.nl[v].hasY = prevY+maxHeight/2, true
		}
		prevY += maxHeight + rankSep
	}
}

func (p *posState) dummy(v int32) bool { return p.s.nl[v].dummy != dkNone }

// bkConflict is one side of a conflicting pair {a, b}, where a is the
// endpoint with the smaller name (the key of the Attrs engine's conflict
// tables): the pair has one entry in a's list and, unless b == a, one in b's,
// at indices 2k and 2k+1, both with the bits of both tables (1: type 1, 2:
// type 2). small reports whether the list's owner is a, so a lookup needs no
// name comparison.
type bkConflict struct {
	other int32
	next  int32
	bits  uint8
	small bool
}

func (p *posState) addConflict(v, w int32, bit uint8) {
	if p.s.names.greater(v, w) {
		v, w = w, v
	}
	if bit == 2 {
		p.type2Key[v] = true
	}
	for i := p.chead[v]; i != inone; i = p.cents[i].next {
		if p.cents[i].other == w {
			p.cents[i].bits |= bit
			if v != w {
				p.cents[i^1].bits |= bit
			}
			return
		}
	}
	// Pairs occupy indices 2k and 2k+1 (a pair with v == w pads its second
	// slot with an unlinked entry).
	i := int32(len(p.cents))
	p.cents = append(p.cents, bkConflict{other: w, next: p.chead[v], bits: bit, small: true})
	p.chead[v] = i
	p.cents = append(p.cents, bkConflict{other: v, next: inone, bits: bit})
	if v != w {
		p.cents[i+1].next = p.chead[w]
		p.chead[w] = i + 1
	}
}

// hasConflict reads the Object.assign merge of the type-1 and type-2 tables:
// a key present in the type-2 table replaces the whole type-1 set.
func (p *posState) hasConflict(v, w int32) bool {
	for i := p.chead[v]; i != inone; i = p.cents[i].next {
		e := &p.cents[i]
		if e.other != w {
			continue
		}
		smaller := w
		if e.small {
			smaller = v
		}
		if p.type2Key[smaller] {
			return e.bits&2 != 0
		}
		return e.bits&1 != 0
	}
	// No entry: the pair is in neither table.
	return false
}

func (p *posState) findType1Conflicts(layering [][]int32) {
	s := p.s
	for layerIndex := 1; layerIndex < len(layering); layerIndex++ {
		prevLayer := layering[layerIndex-1]
		layer := layering[layerIndex]
		k0 := 0
		scanPos := 0
		prevLayerLength := len(prevLayer)
		lastNode := inone
		if len(layer) != 0 {
			lastNode = layer[len(layer)-1]
		}
		for i, v := range layer {
			w, found := inone, false
			if p.dummy(v) {
				for _, u := range p.preds(v) {
					if p.dummy(u) {
						w, found = u, true
						break
					}
				}
			}
			k1 := prevLayerLength
			if found {
				k1 = int(s.nl[w].order)
			}
			if found || v == lastNode {
				for _, scanNode := range layer[scanPos : i+1] {
					for _, u := range p.preds(scanNode) {
						uPos := int(s.nl[u].order)
						if (uPos < k0 || k1 < uPos) && !(p.dummy(u) && p.dummy(scanNode)) {
							p.addConflict(u, scanNode, 1)
						}
					}
				}
				scanPos = i + 1
				k0 = k1
			}
		}
	}
}

func (p *posState) findType2Conflicts(layering [][]int32) {
	s := p.s
	scan := func(south []int32, southPos, southEnd int, prevNorthBorder, nextNorthBorder float64) {
		for i := southPos; i < southEnd; i++ {
			v := south[i]
			if !p.dummy(v) {
				continue
			}
			for _, u := range p.preds(v) {
				uOrder := s.nl[u].order
				if p.dummy(u) && (uOrder < prevNorthBorder || uOrder > nextNorthBorder) {
					p.addConflict(u, v, 2)
				}
			}
		}
	}
	for layerIndex := 1; layerIndex < len(layering); layerIndex++ {
		north := layering[layerIndex-1]
		south := layering[layerIndex]
		prevNorthPos := float64(-1)
		nextNorthPos := float64(-1)
		southPos := 0
		// scan only reads the layering and sets conflict bits, so repeating it
		// with the same arguments changes nothing: the tail scan, which runs
		// after every south node, is skipped while its arguments are unchanged.
		tailScanned := false
		tailPos, tailNorth := 0, 0.0
		for southLookahead, v := range south {
			if s.nl[v].dummy == dkBorder {
				predecessors := p.preds(v)
				if len(predecessors) != 0 {
					nextNorthPos = s.nl[predecessors[0]].order
					scan(south, southPos, southLookahead, prevNorthPos, nextNorthPos)
					southPos = southLookahead
					prevNorthPos = nextNorthPos
				}
			}
			if !tailScanned || tailPos != southPos || math.Float64bits(tailNorth) != math.Float64bits(nextNorthPos) {
				scan(south, southPos, len(south), nextNorthPos, float64(len(north)))
				tailScanned, tailPos, tailNorth = true, southPos, nextNorthPos
			}
		}
	}
}

func (p *posState) positionX(leaves []int32) {
	s := p.s
	layering := s.buildLayerMatrix(leaves)
	n := len(s.nl)
	if cap(p.chead) < n {
		p.chead = make([]int32, n)
	}
	p.chead = p.chead[:n]
	for i := range p.chead {
		p.chead[i] = inone
	}
	p.cents = p.cents[:0]
	p.type2Key = resize(p.type2Key, n)
	p.findType1Conflicts(layering)
	p.findType2Conflicts(layering)

	p.pos = grow(p.pos, n)
	p.posStamp = resize(p.posStamp, n)
	p.posGen = 0
	p.root = grow(p.root, n)
	p.align = grow(p.align, n)
	for i := range p.xs {
		p.xs[i] = resize(p.xs[i], n)
	}
	p.member = resize(p.member, n)
	p.nodes = p.nodes[:0]
	for _, layer := range layering {
		p.nodes = append(p.nodes, layer...)
	}

	alignment := 0 // ul, ur, dl, dr
	for _, vert := range [2]byte{'u', 'd'} {
		p.adjusted = copyIDLayering(p.adjusted, layering, &s.rankBytes)
		adjusted := p.adjusted
		if vert == 'd' {
			for i, j := 0, len(adjusted)-1; i < j; i, j = i+1, j-1 {
				adjusted[i], adjusted[j] = adjusted[j], adjusted[i]
			}
		}
		for _, horiz := range [2]byte{'l', 'r'} {
			if horiz == 'r' {
				for _, layer := range adjusted {
					for i, j := 0, len(layer)-1; i < j; i, j = i+1, j-1 {
						layer[i], layer[j] = layer[j], layer[i]
					}
				}
			}
			p.verticalAlignment(adjusted, vert == 'u')
			p.markMembers(alignment)
			xs := p.xs[alignment]
			p.horizontalCompaction(adjusted, xs, horiz == 'r')
			if horiz == 'r' {
				for _, v := range p.nodes {
					xs[v] = -xs[v]
				}
			}
			alignment++
		}
	}

	smallest := p.findSmallestWidthAlignment()
	p.alignCoordinates(smallest)
	p.balance()
}

func (p *posState) verticalAlignment(layering [][]int32, up bool) {
	p.posGen++
	for _, layer := range layering {
		for order, v := range layer {
			p.root[v] = v
			p.align[v] = v
			p.pos[v], p.posStamp[v] = int32(order), p.posGen
		}
	}
	posOf := func(v int32) (int32, bool) {
		if p.posStamp[v] == p.posGen {
			return p.pos[v], true
		}
		return 0, false
	}
	ws := p.ws[:0]
	for _, layer := range layering {
		prevIdx := int32(-1)
		for _, v := range layer {
			if up {
				ws = append(ws[:0], p.preds(v)...)
			} else {
				ws = append(ws[:0], p.sucs(v)...)
			}
			if len(ws) == 0 {
				continue
			}
			// Stable sort by position (Lodash sortBy); positions are ints.
			for i := 1; i < len(ws); i++ {
				for j := i; j > 0; j-- {
					a, _ := posOf(ws[j])
					b, _ := posOf(ws[j-1])
					if !(a < b) {
						break
					}
					ws[j], ws[j-1] = ws[j-1], ws[j]
				}
			}
			mp := float64(len(ws)-1) / 2
			for i, last := int(math.Floor(mp)), int(math.Ceil(mp)); i <= last; i++ {
				w := ws[i]
				wPos, ok := posOf(w)
				if !ok {
					continue
				}
				if p.align[v] == v && prevIdx < wPos && !p.hasConflict(v, w) {
					p.align[w] = v
					p.root[v] = p.root[w]
					p.align[v] = p.root[v]
					prevIdx = wPos
				}
			}
		}
	}
	p.ws = ws[:0]
}

// blockGraph is the simple directed graph of buildBlockGraph.
type blockGraph struct {
	nodes     []int32 // insertion order
	stamp     []int32
	gen       int32
	ev, ew    []int32
	weight    []float64
	firstOut  []int32 // per position in nodes: its first edge while building
	nextOut   []int32 // per edge: the next edge from the same source
	incidence         // in- and out-lists per position in nodes
	local     []int32 // global -> index in nodes
	predOrd   []int32 // in-list sources per node, JavaScript order
	sucOrd    []int32 // out-list targets per node, JavaScript order
}

func (b *blockGraph) reset(n int) {
	if len(b.stamp) < n {
		b.stamp = make([]int32, n)
		b.local = make([]int32, n)
		b.gen = 0
	}
	b.gen++
	b.nodes = b.nodes[:0]
	b.ev, b.ew, b.weight = b.ev[:0], b.ew[:0], b.weight[:0]
	b.firstOut, b.nextOut = b.firstOut[:0], b.nextOut[:0]
}

func (b *blockGraph) setNode(v int32) {
	if b.stamp[v] == b.gen {
		return
	}
	b.stamp[v] = b.gen
	b.local[v] = int32(len(b.nodes))
	b.nodes = append(b.nodes, v)
	b.firstOut = append(b.firstOut, inone)
}

// findEdge returns the edge from v to w, or inone; v must be a node.
func (b *blockGraph) findEdge(v, w int32) int32 {
	for e := b.firstOut[b.local[v]]; e != inone; e = b.nextOut[e] {
		if b.ew[e] == w {
			return e
		}
	}
	return inone
}

// addEdge appends the edge from v to w, which must not exist yet.
func (b *blockGraph) addEdge(v, w int32, weight float64) {
	lv := b.local[v]
	b.nextOut = append(b.nextOut, b.firstOut[lv])
	b.firstOut[lv] = int32(len(b.ev))
	b.ev = append(b.ev, v)
	b.ew = append(b.ew, w)
	b.weight = append(b.weight, weight)
}

func (b *blockGraph) finish() { b.incidence.build(len(b.nodes), b.ev, b.ew, b.local) }

func (p *posState) positionSep(v, w int32, nodeSep, edgeSep float64, reverseSep bool) float64 {
	vLabel, wLabel := &p.s.nl[v], &p.s.nl[w]
	sum := float64(0)
	delta := float64(0)
	sum += vLabel.width / 2
	if vLabel.hasLabelpos {
		switch strings.ToLower(vLabel.labelpos) {
		case "l":
			delta = -vLabel.width / 2
		case "r":
			delta = vLabel.width / 2
		}
	}
	if jsTruthyNumber(delta) {
		if reverseSep {
			sum += delta
		} else {
			sum -= delta
		}
	}
	delta = 0
	if vLabel.dummy != dkNone {
		sum += edgeSep / 2
	} else {
		sum += nodeSep / 2
	}
	if wLabel.dummy != dkNone {
		sum += edgeSep / 2
	} else {
		sum += nodeSep / 2
	}
	sum += wLabel.width / 2
	if wLabel.hasLabelpos {
		switch strings.ToLower(wLabel.labelpos) {
		case "l":
			delta = wLabel.width / 2
		case "r":
			delta = -wLabel.width / 2
		}
	}
	if jsTruthyNumber(delta) {
		if reverseSep {
			sum += delta
		} else {
			sum -= delta
		}
	}
	return sum
}

func (p *posState) horizontalCompaction(layering [][]int32, xs []float64, reverse bool) {
	s := p.s
	b := &p.bk
	b.reset(len(s.nl))
	nodeSep, edgeSep := s.nodesep, s.edgesep
	for _, layer := range layering {
		u := inone
		for _, v := range layer {
			vRoot := p.root[v]
			b.setNode(vRoot)
			// `if (u)`: node ids are non-empty strings in the typed engine.
			if u != inone {
				// uRoot became a node at u, the previous position.
				uRoot := p.root[u]
				previous := float64(0)
				e := b.findEdge(uRoot, vRoot)
				if e != inone {
					previous = b.weight[e]
					if !jsTruthyNumber(previous) {
						previous = 0
					}
				}
				value := maxJS2(p.positionSep(v, u, nodeSep, edgeSep, reverse), previous)
				if e != inone {
					b.weight[e] = value
				} else {
					b.addEdge(uRoot, vRoot, value)
				}
			}
			u = v
		}
	}
	b.finish()
	key := s.key()
	order := append(p.order[:0], b.nodes...)
	p.order = order
	jsOrderIDs(order, key)

	// xs is this alignment's array, zeroed for the layout and written only
	// here, so an entry not yet set reads 0 like a missing map key.
	// Predecessors and successors in JavaScript order, per block node.
	b.predOrd = grow(b.predOrd, len(b.ev))
	b.sucOrd = grow(b.sucOrd, len(b.ev))
	for le := range b.nodes {
		ps := b.predOrd[b.inStart[le]:b.inStart[le+1]]
		for i := range ps {
			ps[i] = b.ev[b.inList[int(b.inStart[le])+i]]
		}
		jsOrderIDs(ps, key)
		ss := b.sucOrd[b.outStart[le]:b.outStart[le+1]]
		for i := range ss {
			ss[i] = b.ew[b.outList[int(b.outStart[le])+i]]
		}
		jsOrderIDs(ss, key)
	}
	borderType := btRight
	if reverse {
		borderType = btLeft
	}
	p.visited = grow(p.visited, len(b.nodes))
	visited := p.visited
	stack := p.stack[:0]
	iterate := func(first bool) {
		stack = append(stack[:0], order...)
		clear(visited)
		for len(stack) != 0 {
			elem := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			le := b.local[elem]
			if visited[le] {
				if first {
					x := float64(0)
					for i := b.inStart[le]; i < b.inStart[le+1]; i++ {
						e := b.inList[i]
						x = maxJS2(x, xs[b.ev[e]]+b.weight[e])
					}
					xs[elem] = x
				} else {
					minimum := math.Inf(1)
					for i := b.outStart[le]; i < b.outStart[le+1]; i++ {
						e := b.outList[i]
						minimum = minJS2(minimum, xs[b.ew[e]]-b.weight[e])
					}
					if !math.IsInf(minimum, 1) && s.nl[elem].borderType != borderType {
						xs[elem] = maxJS2(xs[elem], minimum)
					}
				}
				continue
			}
			visited[le] = true
			stack = append(stack, elem)
			if first {
				stack = append(stack, b.predOrd[b.inStart[le]:b.inStart[le+1]]...)
			} else {
				stack = append(stack, b.sucOrd[b.outStart[le]:b.outStart[le+1]]...)
			}
		}
	}
	iterate(true)
	iterate(false)
	for _, v := range p.nodes {
		xs[v] = xs[p.root[v]]
	}
	p.stack = stack[:0]
}

// markMembers records which nodes are keys of alignment a's xs map in the
// Attrs engine: horizontalCompaction gives an entry to every node of the block
// graph (the roots, root[u] for every layering node u) and then, ranging over
// align, to every value of align. root is idempotent (a node aligns only while
// align[v] == v, and once some node joins v's block align[v] != v forever), so
// the roots are the nodes with root[v] == v, and xs[v] = xs[root[v]] gives the
// values horizontalCompaction computes for exactly those keys, whatever the
// order of the range. In a regular alignment every block is a cycle of align
// and every node is a key; with a neighbor in its own layer (fractional or
// zero minlen) an alignment can overwrite align[w], and a node that is neither
// a root nor a value of align has no entry: findSmallestWidthAlignment,
// alignCoordinates and balance skip it, balance reads it as 0 in the other
// alignments, and position gives it no x unless it is a key of "ul".
func (p *posState) markMembers(a int) {
	bit := uint8(1) << a
	for _, v := range p.nodes {
		if p.root[v] == v {
			p.member[v] |= bit
		}
		p.member[p.align[v]] |= bit
	}
}

func (p *posState) isMember(a int, v int32) bool { return p.member[v]&(1<<a) != 0 }

func (p *posState) findSmallestWidthAlignment() int {
	smallest := -1
	smallestWidth := math.Inf(1)
	for a := 0; a < 4; a++ {
		xs := p.xs[a]
		maximum := math.Inf(-1)
		minimum := math.Inf(1)
		for _, v := range p.nodes {
			if !p.isMember(a, v) {
				continue
			}
			halfWidth := p.s.nl[v].width / 2
			maximum = maxJS2(maximum, xs[v]+halfWidth)
			minimum = minJS2(minimum, xs[v]-halfWidth)
		}
		width := maximum - minimum
		if smallest < 0 || width < smallestWidth {
			smallest = a
			smallestWidth = width
		}
	}
	return smallest
}

// extremes is coordinateExtremes of alignment a's xs map. Lodash min and max
// keep the first of equal values, so the map's order can only decide the sign
// of a zero result, which alignCoordinates cannot observe: a zero delta is
// skipped, and a nonzero one does not depend on the sign of a zero operand.
func (p *posState) extremes(a int) (minimum, maximum float64) {
	xs := p.xs[a]
	minimum, maximum = math.NaN(), math.NaN()
	for _, v := range p.nodes {
		if !p.isMember(a, v) {
			continue
		}
		minimum = lodashMin2(minimum, xs[v])
		maximum = lodashMax2(maximum, xs[v])
	}
	return minimum, maximum
}

func (p *posState) alignCoordinates(alignTo int) {
	alignToMin, alignToMax := p.extremes(alignTo)
	for a := 0; a < 4; a++ {
		xs := p.xs[a]
		xsMin, xsMax := p.extremes(a)
		delta := alignToMax - xsMax
		if a%2 == 0 { // "l"
			delta = alignToMin - xsMin
		}
		if delta == 0 || math.IsNaN(delta) {
			continue
		}
		for _, v := range p.nodes {
			xs[v] = xs[v] + delta
		}
	}
}

// balance is balance followed by position's x assignment: the keys of "ul"
// get an x; a missing entry of another alignment reads 0.
func (p *posState) balance() {
	s := p.s
	if s.align != "" {
		a := -1
		switch strings.ToLower(s.align) {
		case "ul":
			a = 0
		case "ur":
			a = 1
		case "dl":
			a = 2
		case "dr":
			a = 3
		}
		for _, v := range p.nodes {
			if !p.isMember(0, v) {
				continue
			}
			x := 0.0
			if a >= 0 && p.isMember(a, v) {
				x = p.xs[a][v]
			}
			s.nl[v].x, s.nl[v].hasX = x, true
		}
		return
	}
	for _, v := range p.nodes {
		m := p.member[v]
		if m&1 == 0 {
			continue
		}
		var values [4]float64
		for a := range values {
			if m&(1<<a) != 0 {
				values[a] = p.xs[a][v]
			}
		}
		sort.Float64s(values[:])
		s.nl[v].x, s.nl[v].hasX = (values[1]+values[2])/2, true
	}
}

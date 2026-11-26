package dagro

import (
	"math"
	"unicode/utf16"
)

const (
	// defaultEdgeName is graphlib's name of an edge set without a name.
	defaultEdgeName = "\x00"
	// edgeKeyDelim separates the parts of graphlib's edge ids; node ids
	// containing it are not accepted.
	edgeKeyDelim = "\x01"
)

// southEntry is a south-layer position and an edge weight in dagre's
// twoLayerCrossCount.
type southEntry struct {
	pos    int
	weight float64
}

func jsTruthyNumber(value float64) bool {
	return value != 0 && !math.IsNaN(value)
}

// maxJS2 and minJS2 are mathMaxJS(a, b) and mathMinJS(a, b) without the
// variadic slice. Past the NaN test the builtin max and min equal math.Max
// and math.Min (also for infinities and signed zeros: max(-0, +0) = +0,
// min(-0, +0) = -0) and compile to one instruction instead of a call.
func maxJS2(a, b float64) float64 {
	if a != a || b != b {
		return math.NaN()
	}
	return max(a, b)
}

func minJS2(a, b float64) float64 {
	if a != a || b != b {
		return math.NaN()
	}
	return min(a, b)
}

// lodashMin2 and lodashMax2 are lodashMinJS(a, b) and lodashMaxJS(a, b)
// without the variadic slice: NaN arguments are skipped, a is kept on ties,
// and two NaNs give math.NaN().
func lodashMin2(a, b float64) float64 {
	if a != a {
		if b != b {
			return math.NaN()
		}
		return b
	}
	if b < a {
		return b
	}
	return a
}

func lodashMax2(a, b float64) float64 {
	if a != a {
		if b != b {
			return math.NaN()
		}
		return b
	}
	if b > a {
		return b
	}
	return a
}

func jsStringGreater(a, b string) bool {
	// Layout-generated IDs and D2's numeric IDs are ASCII. Compare that common
	// prefix without allocating UTF-16 copies; retain JS code-unit ordering
	// when either string reaches a non-ASCII byte.
	i := 0
	for i < len(a) && i < len(b) && a[i] < 0x80 && b[i] < 0x80 {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
		i++
	}
	if i == len(a) || i == len(b) {
		return len(a) > len(b)
	}
	a, b = a[i:], b[i:]
	a16, b16 := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(a16) && i < len(b16); i++ {
		if a16[i] != b16[i] {
			return a16[i] > b16[i]
		}
	}
	return len(a16) > len(b16)
}

// jsArrayIndex implements the array-index portion of ECMAScript property-key
// ordering: a canonical unsigned decimal string in [0, 2^32-2].
func jsArrayIndex(key string) (uint32, bool) {
	if key == "0" {
		return 0, true
	}
	if len(key) == 0 || len(key) > 10 || key[0] == '0' {
		return 0, false
	}
	var value uint64
	for i := 0; i < len(key); i++ {
		digit := key[i]
		if digit < '0' || digit > '9' {
			return 0, false
		}
		value = value*10 + uint64(digit-'0')
		if value >= 1<<32-1 {
			return 0, false
		}
	}
	return uint32(value), true
}

// inputEdgeLabel returns the index of the layout label of input edge i.
// When the layout lost the edge or left it without points, dagre's
// updateInputGraph fails reading the edge's points; so does inputEdgeLabel.
func (s *lstate) inputEdgeLabel(i int) int32 {
	k := s.inputEdges[i]
	le := s.g.findEdge(k.v, k.w, k.n)
	if le == inone || s.g.edges[le].label == inone || !s.el[s.g.edges[le].label].hasPoints {
		panic("dagre fails on this input: an edge has no route")
	}
	return s.g.edges[le].label
}

// fasEntry is a node of greedyFAS's state: its index and the total weights
// of its remaining in- and out-edges.
type fasEntry struct {
	v          int
	in, out    float64
	prev, next *fasEntry
	list       *fasList
}

// fasList is dagre's List: a doubly linked queue that enqueues at the head
// and dequeues at the tail; enqueueing an entry moves it from its list.
type fasList struct {
	head, tail *fasEntry
}

func (l *fasList) dequeue() *fasEntry {
	entry := l.tail
	if entry != nil {
		l.unlink(entry)
	}
	return entry
}

func (l *fasList) enqueue(entry *fasEntry) {
	if entry.list != nil {
		entry.list.unlink(entry)
	}
	entry.list = l
	entry.prev = nil
	entry.next = l.head
	if l.head != nil {
		l.head.prev = entry
	} else {
		l.tail = entry
	}
	l.head = entry
}

func (l *fasList) unlink(entry *fasEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		l.head = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		l.tail = entry.prev
	}
	entry.prev, entry.next, entry.list = nil, nil, nil
}

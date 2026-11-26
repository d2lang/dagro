package dagro

import (
	"errors"
	"fmt"
	"math"
)

// LayoutInput describes a graph for LayoutTyped: a compound multigraph with
// every attribute dagre reads as a typed field.
//
// LayoutTyped(in, out) produces exactly the numbers Dagre 3.1.1's layout
// produces for the graphlib graph whose graph label holds rankdir, ranksep,
// edgesep and nodesep (and marginx, marginy, acyclicer, ranker and align
// when set), built by calling, for each node in order, setNode(ID, {width,
// height}) and, when Parent >= 0, setParent(ID, Nodes[Parent].ID), and then
// setEdge for each edge in order with all six edge attributes and, when
// HasName, its name.
//
// Zero values are not dagre's defaults: start from DefaultLayoutInput and
// NewLayoutEdge, or set every field. In particular RankDir "" and LabelPos ""
// are valid values that differ from the defaults "TB" and "r".
type LayoutInput struct {
	RankDir   string  // default "TB"
	RankSep   float64 // default 50
	EdgeSep   float64 // default 20
	NodeSep   float64 // default 50
	MarginX   float64 // 0 means no margin, as when absent
	MarginY   float64
	Acyclicer string // "greedy", or "" for the default DFS
	Ranker    string // "network-simplex", "tight-tree", "longest-path", or "" for network simplex
	Align     string // "", or one of ul, ur, dl, dr (any case)
	Nodes     []LayoutNode
	Edges     []LayoutEdge
}

// LayoutNode is a node of a LayoutInput.
type LayoutNode struct {
	ID            string
	Parent        int // index of the parent in Nodes, or -1 for none
	Width, Height float64
}

// LayoutEdge is an edge of a LayoutInput between Nodes[V] and Nodes[W].
type LayoutEdge struct {
	V, W        int
	Name        string
	HasName     bool
	Minlen      float64 // default 1
	Weight      float64 // default 1
	Width       float64 // default 0
	Height      float64 // default 0
	LabelOffset float64 // default 10
	LabelPos    string  // default "r"
}

// LayoutOutput is the result of LayoutTyped, indexed like the input.
type LayoutOutput struct {
	Width, Height float64
	Nodes         []NodeLayout
	Edges         []EdgeLayout
}

// NodeLayout is a node's center and size. Width and Height are computed for
// nodes with children and are the input values for the others, as dagre
// leaves them in the node labels.
type NodeLayout struct {
	X, Y, Width, Height float64
}

// Point is a point of an edge's route.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// EdgeLayout is an edge's route and, when HasXY, the center of its label.
type EdgeLayout struct {
	Points []Point
	X, Y   float64
	HasXY  bool
}

// DefaultLayoutInput returns a LayoutInput with dagre's default graph
// attributes and no nodes or edges.
func DefaultLayoutInput() LayoutInput {
	return LayoutInput{RankDir: "TB", RankSep: 50, EdgeSep: 20, NodeSep: 50}
}

// NewLayoutEdge returns an edge from Nodes[v] to Nodes[w] with dagre's
// default edge attributes.
func NewLayoutEdge(v, w int) LayoutEdge {
	return LayoutEdge{V: v, W: w, Minlen: 1, Weight: 1, Width: 0, Height: 0, LabelOffset: 10, LabelPos: "r"}
}

// ErrUnsupportedInput is wrapped by the errors LayoutTyped returns for inputs
// it does not lay out: no nodes; a node id that is empty or contains
// "\x01"; two nodes with the same id; a Parent out of range; parents that
// form a cycle; an edge endpoint out of range or with children; two edges
// with the same V, W and name (HasName false counting as one more name); more
// than 2^26 ranks.
var ErrUnsupportedInput = errors.New("dagro: unsupported input")

// PanicError is the error LayoutTyped returns when the layout panics, which
// it does where dagre itself fails (for example "dagre fails on this input:
// it sets \"order\" on a node missing from a layer") and on a defect of the
// engine. Value is the value passed to panic.
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("dagro: layout failed: %v", e.Value)
}

// Unwrap returns Value when it is an error (a Go runtime error, say).
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

// LayoutTyped lays out in and stores the result in out, reusing out's
// slices. The points of all edges share one newly allocated array.
//
// It returns an error wrapping ErrUnsupportedInput, without touching out,
// for an input it does not lay out, and a *PanicError when the layout panics
// (out may then be partly written). It returns the layout's own error
// "not possible to find intersection inside of the rectangle" as dagre
// raises it. LayoutTyped never panics, except on a fatal error such as
// running out of memory.
func LayoutTyped(in *LayoutInput, out *LayoutOutput) error {
	if typedFastPaths && layoutSingleNode(in, out) {
		return nil
	}
	s := getLState()
	if err := s.buildTyped(in); err != nil {
		putLState(s)
		return err
	}
	if err := s.runGuarded(in, out); err != nil {
		if _, ok := err.(*PanicError); !ok && err != errDecline {
			putLState(s)
		}
		// After a panic s is dropped: it may be inconsistent.
		return err
	}
	putLState(s)
	return nil
}

// typedFastPaths enables the closed-form results below (tests turn it off to
// compare them with the engine).
var typedFastPaths = true

// maxExactHalf bounds the sizes layoutSingleNode accepts: integers up to it
// halve and double back exactly.
const maxExactHalf = 1 << 52

// layoutSingleNode stores the result of a graph that has one node and no
// edges, when the node's name cannot be one of the engine's generated names
// (it does not start with '_'), the margins are absent and the node's width
// w and height h are integers in [0, 2^52] other than -0. It reports whether
// it did.
//
// For such a graph every phase of the engine leaves the node alone except
// these, whatever the other attributes: ranking puts it on rank 0, position
// gives it x = +-0 (one block in every alignment) and y = h/2 (half the
// height of its only layer), with width and height swapped around it for
// rankdir lr/rl and y negated for bt/rl, and translateGraph moves it by
// (min(x-w/2), min(y-h/2)) and sizes the graph as (max(0, x+w/2) - minX,
// max(0, y+h/2) - minY). In every case that is the node at (w/2, h/2) in a
// w by h graph: the operands are integers or halves of integers below 2^53,
// so every operation is exact, and a zero size gives +0 throughout (the
// engine gives +0 for a -0 size too, which is why -0 is excluded). No
// dummy node is named from the process-wide counter: the nesting graph's
// root takes the plain name "_root", which no input name can hold, and with
// one rank no layer graph is built. TestSingleNodeMatchesEngine
// compares the results bit for bit with the engine.
func layoutSingleNode(in *LayoutInput, out *LayoutOutput) bool {
	if len(in.Nodes) != 1 || len(in.Edges) != 0 || in.MarginX != 0 || in.MarginY != 0 {
		return false
	}
	n := &in.Nodes[0]
	if n.Parent != -1 || !typedEligibleName(n.ID) || n.ID[0] == '_' {
		return false
	}
	w, h := n.Width, n.Height
	if !(w >= 0 && w <= maxExactHalf && w == math.Trunc(w)) || !(h >= 0 && h <= maxExactHalf && h == math.Trunc(h)) {
		return false
	}
	// -0 passes the tests above, but the engine sizes the graph and places
	// the node with +0 where the closed form below would give -0.
	if math.Signbit(w) || math.Signbit(h) {
		return false
	}
	out.Width, out.Height = w, h
	out.Nodes = resize(out.Nodes, 1)
	out.Nodes[0] = NodeLayout{X: w / 2, Y: h / 2, Width: w, Height: h}
	out.Edges = resize(out.Edges, 0)
	return true
}

// buildTyped is dagre's buildLayoutGraph for in. It returns an error
// wrapping ErrUnsupportedInput for an input the engine does not lay out (see
// ErrUnsupportedInput).
func (s *lstate) buildTyped(in *LayoutInput) error {
	n := len(in.Nodes)
	if n == 0 {
		return fmt.Errorf("%w: no nodes", ErrUnsupportedInput)
	}
	for i, node := range in.Nodes {
		if !typedEligibleName(node.ID) {
			return fmt.Errorf("%w: node %d has the id %q (empty or containing \"\\x01\")", ErrUnsupportedInput, i, node.ID)
		}
	}
	s.begin(n, len(in.Edges))
	for i, node := range in.Nodes {
		if j := s.names.intern(node.ID); j != int32(i) {
			return fmt.Errorf("%w: nodes %d and %d have the same id %q", ErrUnsupportedInput, j, i, node.ID)
		}
		s.noteInputName(node.ID)
	}
	for i := range in.Edges {
		if in.Edges[i].HasName {
			s.noteInputEdgeName(in.Edges[i].Name)
		}
	}
	// graphlib's setParent cycle check, in call order.
	s.parentBuf = resize(s.parentBuf, n)
	parent := s.parentBuf
	for i := range parent {
		parent[i] = inone
	}
	for i, node := range in.Nodes {
		p := node.Parent
		if p < 0 {
			continue
		}
		if p >= n {
			return fmt.Errorf("%w: node %d has parent %d out of range", ErrUnsupportedInput, i, p)
		}
		for a := int32(p); a != inone; a = parent[a] {
			if a == int32(i) {
				return fmt.Errorf("%w: making node %d the parent of node %d would create a cycle", ErrUnsupportedInput, p, i)
			}
		}
		parent[i] = int32(p)
	}
	// The input graph's node order: first appearance (a node, then a parent
	// SetParent creates), in JavaScript order.
	order := s.buf[:0]
	seen := resize(s.seenBuf, n)
	for i, node := range in.Nodes {
		if !seen[i] {
			seen[i] = true
			order = append(order, int32(i))
		}
		if p := node.Parent; p >= 0 && !seen[p] {
			seen[p] = true
			order = append(order, int32(p))
		}
	}
	s.seenBuf = seen
	jsOrderIDs(order, s.key())

	s.rankdir, s.ranksep, s.edgesep, s.nodesep = in.RankDir, in.RankSep, in.EdgeSep, in.NodeSep
	// marginx/marginy are only read through jsTruthyNumber: 0 equals absent.
	s.marginx, s.marginy = in.MarginX, in.MarginY
	s.acyclicer, s.ranker, s.align = in.Acyclicer, in.Ranker, in.Align

	for _, id := range order {
		node := &in.Nodes[id]
		label := newNLabel()
		label.width, label.height = node.Width, node.Height
		s.ensureNode(id)
		s.nl[id] = label
		s.g.setNode(id)
		if p := parent[id]; p != inone {
			s.g.setParent(id, p)
		}
	}
	s.buf = order[:0]
	for i := range in.Edges {
		e := &in.Edges[i]
		if e.V < 0 || e.V >= n || e.W < 0 || e.W >= n {
			return fmt.Errorf("%w: edge %d has an endpoint out of range", ErrUnsupportedInput, i)
		}
		v, w := int32(e.V), int32(e.W)
		if s.g.hasChildren(v) || s.g.hasChildren(w) {
			// dagre fails on such an edge while ranking.
			return fmt.Errorf("%w: edge %d has an endpoint with children", ErrUnsupportedInput, i)
		}
		name := int32(0)
		if e.HasName {
			name = s.edgeNameKey(e.Name)
		}
		if s.g.findEdge(v, w, name) != inone {
			return fmt.Errorf("%w: edge %d repeats the endpoints and name of an earlier edge", ErrUnsupportedInput, i)
		}
		l := newELabel()
		l.minlen, l.weight, l.width, l.height, l.labeloffset, l.labelpos = e.Minlen, e.Weight, e.Width, e.Height, e.LabelOffset, e.LabelPos
		l.hasWidth, l.hasHeight = true, true
		s.g.setEdge(v, w, name, e.HasName, s.newEdgeLabel(l), true)
		s.inputEdges = append(s.inputEdges, ekey{v, w, name})
	}
	return nil
}

// updateTyped is dagre's updateInputGraph for LayoutTyped. The edges' labels
// are resolved before out is written: where updateInputGraph fails (an
// input edge without a layout label), out stays untouched.
func (s *lstate) updateTyped(in *LayoutInput, out *LayoutOutput) {
	total := 0
	labels := s.buf[:0]
	for i := range s.inputEdges {
		label := s.inputEdgeLabel(i)
		labels = append(labels, label)
		total += len(s.el[label].points)
	}
	out.Width, out.Height = s.width, s.height
	out.Nodes = resize(out.Nodes, len(in.Nodes))
	for i := range in.Nodes {
		id := int32(i)
		n := &s.nl[id]
		r := NodeLayout{X: n.x, Y: n.y, Width: in.Nodes[i].Width, Height: in.Nodes[i].Height}
		if s.g.hasChildren(id) {
			r.Width, r.Height = n.width, n.height
		}
		out.Nodes[i] = r
	}
	points := make([]Point, total)
	out.Edges = resize(out.Edges, len(in.Edges))
	at := 0
	for i, label := range labels {
		l := &s.el[label]
		end := at + copy(points[at:], l.points)
		// Like append([]Point(nil), points...): nil when there are none.
		var r EdgeLayout
		if end > at {
			r.Points = points[at:end:end]
		}
		if l.hasX {
			r.X, r.Y, r.HasXY = l.x, l.y, true
		}
		out.Edges[i] = r
		at = end
	}
	s.buf = labels[:0]
}

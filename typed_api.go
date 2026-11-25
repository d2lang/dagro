package dagro

import (
	"errors"
	"fmt"
	"math"
)

// LayoutInput describes a graph for LayoutTyped with every attribute Layout
// reads as a typed field, so no Graph or Attrs labels have to be built.
//
// LayoutTyped(in, out) produces exactly the numbers Layout produces for the
// Graph returned by in.Graph(): a compound multigraph whose graph label holds
// rankdir, ranksep, edgesep and nodesep (and marginx, marginy, acyclicer,
// ranker and align when set), built by calling, for each node in order,
// SetNode(ID, Attrs{"width", "height"}) and, when Parent >= 0,
// SetParent(ID, Nodes[Parent].ID), and then SetEdge for each edge in order
// with all six edge attributes and, when HasName, its name.
//
// Zero values are not Layout's defaults: start from DefaultLayoutInput and
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
// nodes with children and are the input values for the others, as Layout
// leaves them in the node labels.
type NodeLayout struct {
	X, Y, Width, Height float64
}

// EdgeLayout is an edge's route and, when HasXY, the center of its label.
type EdgeLayout struct {
	Points []Point
	X, Y   float64
	HasXY  bool
}

// DefaultLayoutInput returns a LayoutInput with Layout's default graph
// attributes and no nodes or edges.
func DefaultLayoutInput() LayoutInput {
	return LayoutInput{RankDir: "TB", RankSep: 50, EdgeSep: 20, NodeSep: 50}
}

// NewLayoutEdge returns an edge from Nodes[v] to Nodes[w] with Layout's
// default edge attributes.
func NewLayoutEdge(v, w int) LayoutEdge {
	return LayoutEdge{V: v, W: w, Minlen: 1, Weight: 1, Width: 0, Height: 0, LabelOffset: 10, LabelPos: "r"}
}

// Graph returns the Graph whose Layout LayoutTyped reproduces.
func (in *LayoutInput) Graph() (*Graph, error) {
	label := Attrs{"rankdir": in.RankDir, "ranksep": in.RankSep, "edgesep": in.EdgeSep, "nodesep": in.NodeSep}
	if in.MarginX != 0 {
		label["marginx"] = in.MarginX
	}
	if in.MarginY != 0 {
		label["marginy"] = in.MarginY
	}
	if in.Acyclicer != "" {
		label["acyclicer"] = in.Acyclicer
	}
	if in.Ranker != "" {
		label["ranker"] = in.Ranker
	}
	if in.Align != "" {
		label["align"] = in.Align
	}
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(label)
	for i, n := range in.Nodes {
		g.SetNode(n.ID, Attrs{"width": n.Width, "height": n.Height})
		if n.Parent >= 0 {
			if n.Parent >= len(in.Nodes) {
				return nil, fmt.Errorf("dagro: node %d has parent %d out of range", i, n.Parent)
			}
			if err := g.SetParent(n.ID, in.Nodes[n.Parent].ID); err != nil {
				return nil, err
			}
		}
	}
	for i, e := range in.Edges {
		if e.V < 0 || e.V >= len(in.Nodes) || e.W < 0 || e.W >= len(in.Nodes) {
			return nil, fmt.Errorf("dagro: edge %d endpoint out of range", i)
		}
		label := Attrs{
			"minlen": e.Minlen, "weight": e.Weight, "width": e.Width, "height": e.Height,
			"labeloffset": e.LabelOffset, "labelpos": e.LabelPos,
		}
		// SetEdge(v, w, label) or, when HasName, SetEdge(v, w, label, Name).
		var name *string
		if e.HasName {
			name = &e.Name
		}
		g.setEdge(in.Nodes[e.V].ID, in.Nodes[e.W].ID, label, true, name)
	}
	return g, nil
}

// ErrGraphEngine is what LayoutTyped returns, in a build with the
// dagro_typed_only tag (see LayoutTyped), for a valid input that the typed
// engine does not lay out. No other build returns it.
var ErrGraphEngine = errors.New("dagro: LayoutTyped: this input needs the Graph engine, which a build with the dagro_typed_only tag leaves out of LayoutTyped (build without the tag, or lay out in.Graph() with Layout)")

// LayoutTyped lays out in and stores the result in out, reusing out's
// slices. The points of all edges share one newly allocated array.
//
// The typed engine lays out the inputs it accepts: valid ones whose node ids
// are distinct, not empty and free of "\x01", whose edges are distinct and
// touch no node with children (and, without the fallback below, that do not
// ask for the "greedy" acyclicer). For every other input, and where the
// typed engine gives up on one it accepted, LayoutTyped falls back to the
// Graph engine: it lays out in.Graph() as Layout does.
//
// A build that sets the dagro_typed_only tag leaves that fallback out, and
// the Graph engine with it, so that a program which only uses the typed API
// and never needs the fallback (D2's WebAssembly module) does not carry a
// second engine. Nothing else leaves it out: without the tag LayoutTyped is
// the same under every compiler and on every platform. With the tag,
// LayoutTyped returns the error of in.Graph() for an invalid input and
// ErrGraphEngine for any other input the typed engine does not accept, and a
// panic of the typed engine reaches the caller: it stands for an input on
// which the Graph engine fails as well or which the typed engine does not
// reproduce (see lstate.runGuarded). Layout lays out every graph in every
// build.
func LayoutTyped(in *LayoutInput, out *LayoutOutput) error {
	if typedFastPaths && layoutSingleNode(in, out) {
		return nil
	}
	s := getLState()
	if !s.buildTyped(in) {
		putLState(s)
		return layoutNotTyped(in, out)
	}
	return layoutBuilt(s, in, out)
}

// finishTyped ends a LayoutTyped call whose run returned err: it stores the
// result and keeps s for the next call.
func (s *lstate) finishTyped(in *LayoutInput, out *LayoutOutput, err error) error {
	if err == nil {
		s.updateTyped(in, out)
	}
	putLState(s)
	return err
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
// compares the results bit for bit with the engine and with the Graph path.
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

// buildTyped is build for a LayoutInput: it reproduces buildLayoutGraph on
// in.Graph() without building it. It returns false for inputs that take the
// Graph path: names the typed engine does not accept, duplicate node ids or
// edge keys, parent cycles, indices out of range, edges touching a node
// with children, and acyclicer "greedy" in a build without typedGreedy.
func (s *lstate) buildTyped(in *LayoutInput) bool {
	n := len(in.Nodes)
	for _, node := range in.Nodes {
		if !typedEligibleName(node.ID) {
			return false
		}
	}
	if !typedGreedy && in.Acyclicer == "greedy" {
		return false
	}
	s.begin(n, len(in.Edges))
	for i, node := range in.Nodes {
		if s.names.intern(node.ID) != int32(i) {
			return false // duplicate id
		}
		s.noteInputName(node.ID)
	}
	for i := range in.Edges {
		if in.Edges[i].HasName {
			s.noteInputEdgeName(in.Edges[i].Name)
		}
	}
	// Graph.SetParent's cycle check, in call order.
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
			return false
		}
		for a := int32(p); a != inone; a = parent[a] {
			if a == int32(i) {
				return false
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
			return false
		}
		v, w := int32(e.V), int32(e.W)
		if s.g.hasChildren(v) || s.g.hasChildren(w) {
			return false
		}
		name := int32(0)
		if e.HasName {
			name = s.edgeNameKey(e.Name)
		}
		if s.g.findEdge(v, w, name) != inone {
			return false // duplicate edge key
		}
		l := newELabel()
		l.minlen, l.weight, l.width, l.height, l.labeloffset, l.labelpos = e.Minlen, e.Weight, e.Width, e.Height, e.LabelOffset, e.LabelPos
		l.hasWidth, l.hasHeight = true, true
		s.g.setEdge(v, w, name, e.HasName, s.newEdgeLabel(l), true)
		s.inputEdges = append(s.inputEdges, ekey{v, w, name})
	}
	return true
}

// updateTyped is update for LayoutTyped. The edges' labels are resolved
// before out is written: where updateInputGraph fails, Layout(in.Graph())
// fails before layoutTypedViaGraph writes out, so out stays untouched.
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

package dagro

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	defaultEdgeName = "\x00"
	graphNode       = "\x00"
	edgeKeyDelim    = "\x01"
)

// GraphOptions matches graphlib 2.1.8's Graph constructor options.
type GraphOptions struct {
	Directed   bool
	Undirected bool
	Multigraph bool
	Compound   bool
}

// Edge uniquely identifies an edge. HasName distinguishes an unnamed edge
// from a named edge whose name happens to be empty.
type Edge struct {
	V       string
	W       string
	Name    string
	HasName bool
}

type orderedSet struct {
	order []string
	has   map[string]bool
}

func newOrderedSet() *orderedSet { return &orderedSet{has: map[string]bool{}} }

func (s *orderedSet) add(v string) {
	if !s.has[v] {
		s.has[v] = true
		s.order = append(s.order, v)
	}
}

func (s *orderedSet) remove(v string) {
	if !s.has[v] {
		return
	}
	delete(s.has, v)
	for i, item := range s.order {
		if item == v {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}

func (s *orderedSet) values() []string {
	out := append([]string(nil), s.order...)
	return jsObjectKeyOrder(out)
}

type orderedCounter struct {
	order []string
	count map[string]int
}

func newOrderedCounter() *orderedCounter { return &orderedCounter{count: map[string]int{}} }

func (m *orderedCounter) inc(k string) {
	if m.count[k] == 0 {
		m.order = append(m.order, k)
	}
	m.count[k]++
}

func (m *orderedCounter) dec(k string) {
	if m.count[k] > 1 {
		m.count[k]--
		return
	}
	delete(m.count, k)
	for i, item := range m.order {
		if item == k {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}

func (m *orderedCounter) keys() []string { return jsObjectKeyOrder(append([]string(nil), m.order...)) }

type edgeMap struct {
	order []string
	items map[string]Edge
}

func newEdgeMap() *edgeMap { return &edgeMap{items: map[string]Edge{}} }

func (m *edgeMap) set(id string, e Edge) {
	if _, ok := m.items[id]; !ok {
		m.order = append(m.order, id)
	}
	m.items[id] = e
}

func (m *edgeMap) remove(id string) {
	if _, ok := m.items[id]; !ok {
		return
	}
	delete(m.items, id)
	for i, item := range m.order {
		if item == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}

func (m *edgeMap) values() []Edge {
	keys := jsObjectKeyOrder(append([]string(nil), m.order...))
	out := make([]Edge, 0, len(keys))
	for _, id := range keys {
		out = append(out, m.items[id])
	}
	return out
}

// Graph is a behavior-compatible Go implementation of the graphlib 2.1.8
// graph used by Dagre 0.8.5.
type Graph struct {
	directed, multigraph, compound bool
	label                          any
	defaultNodeLabel               func(string) any
	defaultEdgeLabel               func(string, string, *string) any
	nodes                          map[string]any
	nodeOrder                      []string
	parent                         map[string]string
	children                       map[string]*orderedSet
	in, out                        map[string]*edgeMap
	preds, sucs                    map[string]*orderedCounter
	edgeObjs                       *edgeMap
	edgeLabels                     map[string]any
	nextID                         uint64
}

// NewGraph constructs a graph. Graphs are directed by default, matching
// graphlib; set Undirected for an undirected graph.
func NewGraph(opts ...GraphOptions) *Graph {
	o := GraphOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	directed := true
	if o.Undirected {
		directed = false
	} else if o.Directed {
		directed = true
	}
	g := &Graph{
		directed: directed, multigraph: o.Multigraph, compound: o.Compound,
		defaultNodeLabel: func(string) any { return nil },
		defaultEdgeLabel: func(string, string, *string) any { return nil },
		nodes:            map[string]any{}, parent: map[string]string{}, children: map[string]*orderedSet{},
		in: map[string]*edgeMap{}, out: map[string]*edgeMap{},
		preds: map[string]*orderedCounter{}, sucs: map[string]*orderedCounter{},
		edgeObjs: newEdgeMap(), edgeLabels: map[string]any{},
	}
	if g.compound {
		g.children[graphNode] = newOrderedSet()
	}
	return g
}

func (g *Graph) IsDirected() bool          { return g.directed }
func (g *Graph) IsMultigraph() bool        { return g.multigraph }
func (g *Graph) IsCompound() bool          { return g.compound }
func (g *Graph) SetGraph(label any) *Graph { g.label = label; return g }
func (g *Graph) Graph() any                { return g.label }

func (g *Graph) SetDefaultNodeLabel(value any) *Graph {
	if isCallable(value) {
		g.defaultNodeLabel = func(v string) any { return callCallable(value, v) }
	} else {
		g.defaultNodeLabel = func(string) any { return value }
	}
	return g
}

func (g *Graph) SetDefaultEdgeLabel(value any) *Graph {
	if isCallable(value) {
		g.defaultEdgeLabel = func(v, w string, name *string) any {
			return callCallable(value, v, w, optionalEdgeName{name})
		}
	} else {
		g.defaultEdgeLabel = func(string, string, *string) any { return value }
	}
	return g
}

func (g *Graph) NodeCount() int { return len(g.nodes) }

func (g *Graph) Nodes() []string { return jsObjectKeyOrder(append([]string(nil), g.nodeOrder...)) }

func (g *Graph) Sources() []string {
	var out []string
	for _, v := range g.Nodes() {
		if len(g.in[v].items) == 0 {
			out = append(out, v)
		}
	}
	return out
}

func (g *Graph) Sinks() []string {
	var out []string
	for _, v := range g.Nodes() {
		if len(g.out[v].items) == 0 {
			out = append(out, v)
		}
	}
	return out
}

func (g *Graph) SetNodes(vs []string, value ...any) *Graph {
	for _, v := range vs {
		g.SetNode(v, value...)
	}
	return g
}

func (g *Graph) SetNode(v string, value ...any) *Graph {
	if _, ok := g.nodes[v]; ok {
		if len(value) > 0 {
			g.nodes[v] = value[0]
		}
		return g
	}
	if len(value) > 0 {
		g.nodes[v] = value[0]
	} else {
		g.nodes[v] = g.defaultNodeLabel(v)
	}
	g.nodeOrder = append(g.nodeOrder, v)
	if g.compound {
		g.parent[v] = graphNode
		g.children[v] = newOrderedSet()
		g.children[graphNode].add(v)
	}
	g.in[v] = newEdgeMap()
	g.out[v] = newEdgeMap()
	g.preds[v] = newOrderedCounter()
	g.sucs[v] = newOrderedCounter()
	return g
}

func (g *Graph) Node(v string) any     { return g.nodes[v] }
func (g *Graph) HasNode(v string) bool { _, ok := g.nodes[v]; return ok }

func (g *Graph) RemoveNode(v string) *Graph {
	if !g.HasNode(v) {
		return g
	}
	incident := append(g.in[v].values(), g.out[v].values()...)
	seen := map[string]bool{}
	for _, e := range incident {
		id := edgeObjToID(g.directed, e)
		if !seen[id] {
			seen[id] = true
			g.RemoveEdge(e)
		}
	}
	if g.compound {
		g.removeFromParentsChildList(v)
		for _, child := range g.Children(v) {
			_ = g.SetParent(child)
		}
		delete(g.parent, v)
		delete(g.children, v)
	}
	delete(g.nodes, v)
	delete(g.in, v)
	delete(g.out, v)
	delete(g.preds, v)
	delete(g.sucs, v)
	for i, item := range g.nodeOrder {
		if item == v {
			g.nodeOrder = append(g.nodeOrder[:i], g.nodeOrder[i+1:]...)
			break
		}
	}
	return g
}

func (g *Graph) SetParent(v string, parent ...string) error {
	if !g.compound {
		return fmt.Errorf("cannot set parent in a non-compound graph")
	}
	p := graphNode
	if len(parent) > 0 {
		p = parent[0]
		for ancestor, defined := p, true; defined; {
			if ancestor == v {
				return fmt.Errorf("setting %s as parent of %s would create a cycle", p, v)
			}
			ancestor, defined = g.Parent(ancestor)
		}
		g.SetNode(p)
	}
	g.SetNode(v)
	g.removeFromParentsChildList(v)
	g.parent[v] = p
	g.children[p].add(v)
	return nil
}

func (g *Graph) removeFromParentsChildList(v string) {
	if p, ok := g.parent[v]; ok {
		g.children[p].remove(v)
	}
}

func (g *Graph) Parent(v string) (string, bool) {
	if !g.compound {
		return "", false
	}
	p, ok := g.parent[v]
	if !ok || p == graphNode {
		return "", false
	}
	return p, true
}

func (g *Graph) Children(v ...string) []string {
	key := graphNode
	if len(v) > 0 {
		key = v[0]
	}
	if g.compound {
		if children := g.children[key]; children != nil {
			return children.values()
		}
		return nil
	}
	if key == graphNode {
		return g.Nodes()
	}
	if g.HasNode(key) {
		return []string{}
	}
	return nil
}

func (g *Graph) Predecessors(v string) []string {
	if m := g.preds[v]; m != nil {
		return m.keys()
	}
	return nil
}
func (g *Graph) Successors(v string) []string {
	if m := g.sucs[v]; m != nil {
		return m.keys()
	}
	return nil
}
func (g *Graph) Neighbors(v string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{g.Predecessors(v), g.Successors(v)} {
		for _, w := range list {
			if !seen[w] {
				seen[w] = true
				out = append(out, w)
			}
		}
	}
	return out
}
func (g *Graph) IsLeaf(v string) bool {
	if g.directed {
		return len(g.Successors(v)) == 0
	}
	return len(g.Neighbors(v)) == 0
}

// FilterNodes returns a graph containing the nodes accepted by filter and the
// edges between them. A retained compound node is attached to its nearest
// retained ancestor when its immediate parent is filtered out.
func (g *Graph) FilterNodes(filter func(string) bool) *Graph {
	copy := NewGraph(GraphOptions{
		Undirected: !g.directed,
		Multigraph: g.multigraph,
		Compound:   g.compound,
	}).SetGraph(g.Graph())
	for _, v := range g.Nodes() {
		if filter(v) {
			copy.SetNode(v, g.Node(v))
		}
	}
	for _, e := range g.Edges() {
		if copy.HasNode(e.V) && copy.HasNode(e.W) {
			copy.SetEdgeObject(e, g.Edge(e))
		}
	}
	if g.compound {
		for _, v := range copy.Nodes() {
			parent, ok := g.Parent(v)
			for ok && !copy.HasNode(parent) {
				parent, ok = g.Parent(parent)
			}
			if ok {
				if err := copy.SetParent(v, parent); err != nil {
					panic(err)
				}
			} else if err := copy.SetParent(v); err != nil {
				panic(err)
			}
		}
	}
	return copy
}

func (g *Graph) EdgeCount() int { return len(g.edgeLabels) }
func (g *Graph) Edges() []Edge  { return g.edgeObjs.values() }

func (g *Graph) SetPath(vs []string, value ...any) *Graph {
	for i := 1; i < len(vs); i++ {
		g.SetEdge(vs[i-1], vs[i], value...)
	}
	return g
}

func (g *Graph) SetEdge(v, w string, args ...any) *Graph {
	var value any
	valueSpecified := len(args) > 0
	if valueSpecified {
		value = args[0]
	}
	var name *string
	if len(args) > 1 {
		n := jsConcatString(args[1])
		name = &n
	}
	id := edgeArgsToID(g.directed, v, w, name)
	if _, ok := g.edgeLabels[id]; ok {
		if valueSpecified {
			g.edgeLabels[id] = value
		}
		return g
	}
	if name != nil && !g.multigraph {
		panic("dagro: cannot set a named edge when Multigraph is false")
	}
	g.SetNode(v)
	g.SetNode(w)
	if valueSpecified {
		g.edgeLabels[id] = value
	} else {
		g.edgeLabels[id] = g.defaultEdgeLabel(v, w, name)
	}
	e := edgeArgsToObj(g.directed, v, w, name)
	g.edgeObjs.set(id, e)
	g.preds[e.W].inc(e.V)
	g.sucs[e.V].inc(e.W)
	g.in[e.W].set(id, e)
	g.out[e.V].set(id, e)
	return g
}

// SetEdgeObject is the edge-object form of graphlib's setEdge.
func (g *Graph) SetEdgeObject(e Edge, value ...any) *Graph {
	if len(value) > 0 {
		if e.HasName {
			return g.SetEdge(e.V, e.W, value[0], e.Name)
		}
		return g.SetEdge(e.V, e.W, value[0])
	}
	if e.HasName {
		return g.setEdge(e.V, e.W, nil, false, &e.Name)
	}
	return g.SetEdge(e.V, e.W)
}

func (g *Graph) setEdge(v, w string, value any, valueSpecified bool, name *string) *Graph {
	args := []any{}
	if valueSpecified {
		args = append(args, value)
	}
	if name != nil {
		if !valueSpecified {
			id := edgeArgsToID(g.directed, v, w, name)
			if _, ok := g.edgeLabels[id]; ok {
				return g
			}
			if !g.multigraph {
				panic("dagro: cannot set a named edge when Multigraph is false")
			}
			g.SetNode(v)
			g.SetNode(w)
			g.edgeLabels[id] = g.defaultEdgeLabel(v, w, name)
			e := edgeArgsToObj(g.directed, v, w, name)
			g.edgeObjs.set(id, e)
			g.preds[e.W].inc(e.V)
			g.sucs[e.V].inc(e.W)
			g.in[e.W].set(id, e)
			g.out[e.V].set(id, e)
			return g
		}
		args = append(args, *name)
	}
	return g.SetEdge(v, w, args...)
}

func (g *Graph) uniqueID(prefix string) string {
	g.nextID++
	return prefix + strconv.FormatUint(g.nextID, 10)
}

func (g *Graph) Edge(e Edge) any { return g.edgeLabels[edgeObjToID(g.directed, e)] }

func (g *Graph) EdgeByArgs(v, w string, name ...string) any {
	var n *string
	if len(name) > 0 {
		n = &name[0]
	}
	return g.edgeLabels[edgeArgsToID(g.directed, v, w, n)]
}

func (g *Graph) HasEdge(v, w string, name ...string) bool {
	var n *string
	if len(name) > 0 {
		n = &name[0]
	}
	_, ok := g.edgeLabels[edgeArgsToID(g.directed, v, w, n)]
	return ok
}
func (g *Graph) HasEdgeObject(e Edge) bool {
	_, ok := g.edgeLabels[edgeObjToID(g.directed, e)]
	return ok
}

func (g *Graph) RemoveEdge(e Edge) *Graph {
	id := edgeObjToID(g.directed, e)
	stored, ok := g.edgeObjs.items[id]
	if !ok {
		return g
	}
	delete(g.edgeLabels, id)
	g.edgeObjs.remove(id)
	g.preds[stored.W].dec(stored.V)
	g.sucs[stored.V].dec(stored.W)
	g.in[stored.W].remove(id)
	g.out[stored.V].remove(id)
	return g
}

func (g *Graph) RemoveEdgeByArgs(v, w string, name ...string) *Graph {
	var n *string
	if len(name) > 0 {
		n = &name[0]
	}
	id := edgeArgsToID(g.directed, v, w, n)
	if e, ok := g.edgeObjs.items[id]; ok {
		return g.RemoveEdge(e)
	}
	return g
}

func (g *Graph) InEdges(v string, u ...string) []Edge {
	m := g.in[v]
	if m == nil {
		return nil
	}
	edges := m.values()
	if len(u) == 0 || u[0] == "" {
		return edges
	}
	out := edges[:0]
	for _, e := range edges {
		if e.V == u[0] {
			out = append(out, e)
		}
	}
	return out
}
func (g *Graph) OutEdges(v string, w ...string) []Edge {
	m := g.out[v]
	if m == nil {
		return nil
	}
	edges := m.values()
	if len(w) == 0 || w[0] == "" {
		return edges
	}
	out := edges[:0]
	for _, e := range edges {
		if e.W == w[0] {
			out = append(out, e)
		}
	}
	return out
}
func (g *Graph) NodeEdges(v string, w ...string) []Edge {
	return append(g.InEdges(v, w...), g.OutEdges(v, w...)...)
}

func edgeArgsToID(directed bool, v, w string, name *string) string {
	if !directed && jsStringGreater(v, w) {
		v, w = w, v
	}
	n := defaultEdgeName
	if name != nil {
		n = *name
	}
	return v + edgeKeyDelim + w + edgeKeyDelim + n
}

func edgeArgsToObj(directed bool, v, w string, name *string) Edge {
	if !directed && jsStringGreater(v, w) {
		v, w = w, v
	}
	e := Edge{V: v, W: w}
	if name != nil {
		e.Name, e.HasName = *name, true
	}
	return e
}

func jsStringGreater(a, b string) bool {
	a16, b16 := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(a16) && i < len(b16); i++ {
		if a16[i] != b16[i] {
			return a16[i] > b16[i]
		}
	}
	return len(a16) > len(b16)
}

func edgeObjToID(directed bool, e Edge) string {
	var n *string
	if e.HasName {
		n = &e.Name
	}
	return edgeArgsToID(directed, e.V, e.W, n)
}

// JavaScript Object.keys enumerates array-index keys first in numeric order.
// Dagre mostly uses opaque IDs, but matching this detail keeps numeric IDs
// compatible with graphlib.
func jsObjectKeyOrder(keys []string) []string {
	type indexed struct {
		key   string
		value uint64
	}
	var indices []indexed
	var rest []string
	for _, key := range keys {
		if key == "0" {
			indices = append(indices, indexed{key, 0})
			continue
		}
		if strings.HasPrefix(key, "0") || key == "" {
			rest = append(rest, key)
			continue
		}
		n, err := strconv.ParseUint(key, 10, 32)
		if err == nil && n < 4294967295 && strconv.FormatUint(n, 10) == key {
			indices = append(indices, indexed{key, n})
		} else {
			rest = append(rest, key)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool { return indices[i].value < indices[j].value })
	out := make([]string, 0, len(keys))
	for _, item := range indices {
		out = append(out, item.key)
	}
	return append(out, rest...)
}

func preorder(g *Graph, starts []string) []string  { return dfs(g, starts, false) }
func postorder(g *Graph, starts []string) []string { return dfs(g, starts, true) }

func dfs(g *Graph, starts []string, post bool) []string {
	visited := map[string]bool{}
	var out []string
	var visit func(string)
	visit = func(v string) {
		if visited[v] {
			return
		}
		visited[v] = true
		if !post {
			out = append(out, v)
		}
		next := g.Successors(v)
		if !g.directed {
			next = g.Neighbors(v)
		}
		for _, w := range next {
			visit(w)
		}
		if post {
			out = append(out, v)
		}
	}
	for _, v := range starts {
		if !g.HasNode(v) {
			panic(fmt.Sprintf("dagro: graph does not have node: %s", v))
		}
		visit(v)
	}
	return out
}

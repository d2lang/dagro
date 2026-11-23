package dagro

import "fmt"

// sameRankConstraints maps each input node to its rank representative. It has
// no entries for synthetic rank nodes: each border and the nesting root must
// keep its own rank variables, even when leaves in two clusters share one.
type sameRankConstraints struct {
	representative []int32
}

func (s *lstate) prepareSameRanks(groups [][]string) (*sameRankConstraints, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	parent := make([]int32, len(s.g.nodes))
	for i := range parent {
		parent[i] = int32(i)
	}
	find := func(v int32) int32 {
		for parent[v] != v {
			parent[v] = parent[parent[v]]
			v = parent[v]
		}
		return v
	}
	merged := false
	for i, group := range groups {
		first := inone
		for _, name := range group {
			v, exists := s.names.lookup(name)
			if !exists || !s.g.hasNode(v) {
				return nil, fmt.Errorf("dagro: same-rank group %d: unknown node %q", i, name)
			}
			if s.g.hasChildren(v) {
				return nil, fmt.Errorf("dagro: same-rank group %d: %q is a compound node", i, name)
			}
			root := find(v)
			if first == inone {
				first = root
			} else if first != root {
				// Canonical representatives make the result independent of both
				// the order of the groups and the order of their members.
				parent[max(first, root)] = min(first, root)
				first = min(first, root)
				merged = true
			}
		}
	}
	if !merged {
		return nil, nil
	}
	for i := range parent {
		parent[i] = find(int32(i))
	}
	return &sameRankConstraints{representative: parent}, nil
}

// makeAcyclic chooses reversals on the graph of rank groups. Breaking cycles
// on individual nodes first would miss cycles introduced by contracting a
// path's endpoints into the same rank. Original edges stay individually
// addressable, including parallel edges between different group members.
func (c *sameRankConstraints) makeAcyclic(s *lstate) []detachedEdge {
	quotient := lstate{
		names: s.names,
		g:     newIGraph(false, false, len(s.g.nodes), s.g.ecount),
	}
	for _, v := range s.nodes() {
		if !s.g.hasChildren(v) {
			quotient.g.setNode(c.representative[v])
		}
	}
	useGreedy := s.acyclicer == "greedy"
	var flat []detachedEdge
	for _, e := range s.edges() {
		ed := &s.g.edges[e]
		v, w := c.representative[ed.v], c.representative[ed.w]
		if v == w {
			flat = append(flat, detachedEdge{e: s.refOf(e), label: ed.label})
			s.g.removeEdge(e)
			continue
		}
		qe := quotient.g.setEdge(v, w, 0, false, 0, false)
		if useGreedy {
			weight := s.el[ed.label].weight
			// The weighted heuristic can treat zero-weight edges as a sink.
			// Fall back to DFS so the ranker always receives a DAG.
			if !(weight > 0) {
				useGreedy = false
				continue
			}
			edge := &quotient.g.edges[qe]
			if edge.label == inone {
				edge.label = quotient.newEdgeLabel(elabel{})
			}
			quotient.el[edge.label].weight += weight
		}
	}
	var feedback []edgeRef
	if useGreedy {
		feedback = quotient.greedyFAS()
	} else {
		feedback = quotient.dfsFAS()
	}
	if len(feedback) != 0 {
		reverse := make(map[[2]int32]bool, len(feedback))
		for _, r := range feedback {
			reverse[[2]int32{r.v, r.w}] = true
		}
		for _, e := range s.edges() {
			r := s.refOf(e)
			if reverse[[2]int32{c.representative[r.v], c.representative[r.w]}] {
				s.reverseAcyclicEdge(r)
			}
		}
	}
	return flat
}

// reverseAcyclicEdge records the original identity so undoAcyclic restores
// it and reversePointsForReversedEdges restores the route's direction.
func (s *lstate) reverseAcyclicEdge(r edgeRef) {
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

// rank contracts only the temporary ranker's variables; ordering, positioning
// and routing will see every original leaf in its own compound parent.
func (c *sameRankConstraints) rank(s *lstate) {
	rg := s.nonCompoundRankGraph()
	oldIDs := rg.ids
	local := make([]int32, len(oldIDs))
	// Use one-based indices, leaving zero for "unseen". Synthetic nodes
	// remain singleton classes; their incoming and outgoing minlen
	// constraints still bound every descendant's rank.
	class := make([]int32, len(s.g.nodes))
	rg.ids = rg.ids[:0]
	for i, id := range oldIDs {
		rep := id
		if int(id) < len(c.representative) {
			rep = c.representative[id]
		}
		if class[rep] == 0 {
			rg.ids = append(rg.ids, rep)
			class[rep] = int32(len(rg.ids))
		}
		local[i] = class[rep] - 1
	}
	for i := range rg.ev {
		rg.ev[i], rg.ew[i] = local[rg.ev[i]], local[rg.ew[i]]
		// Ordinary arcs must still advance at least one layer for the
		// normalizer. Intra-group edges are routed separately.
		rg.minlen[i] = max(1, rg.minlen[i])
	}
	s.rankGraph(rg)
	for i, rep := range c.representative {
		if int32(i) != rep {
			s.nl[i].rank, s.nl[i].hasRank = s.nl[rep].rank, s.nl[rep].hasRank
		}
	}
}

package dagro

// orderFlatEdges shortens flat edges after the inter-rank crossing minimizer
// has chosen an order. Only adjacent sibling leaves with no inter-rank edges
// may trade places. This keeps all other slots, cluster boundaries, and the
// crossing count chosen by order intact.
func (s *lstate) orderFlatEdges(flat []detachedEdge) {
	if len(flat) == 0 {
		return
	}
	layers := s.buildLayerMatrix(s.nodes())
	active := make([]bool, len(layers))
	pull := make(map[int32]float64)
	between := make(map[[2]int32]float64)
	pair := func(a, b int32) [2]int32 { return [2]int32{min(a, b), max(a, b)} }
	for _, f := range flat {
		weight := s.el[f.label].weight
		if !(weight > 0) {
			continue
		}
		a, b := f.e.v, f.e.w
		if s.nl[a].order > s.nl[b].order {
			a, b = b, a
		}
		// Moving one position right shortens every edge to the right by
		// one slot and lengthens every edge to the left by one slot.
		pull[a] += weight
		pull[b] -= weight
		between[pair(a, b)] += weight
		active[int(s.nl[a].rank)] = true
	}

	free := func(v int32) bool {
		n := &s.g.nodes[v]
		return s.nl[v].dummy == dkNone && n.inDeg == 0 && n.outDeg == 0
	}
	swapIfShorter := func(layer []int32, i int) {
		a, b := layer[i], layer[i+1]
		if !free(a) || !free(b) || s.g.parentOf(a) != s.g.parentOf(b) {
			return
		}
		// Edges between a and b keep their length. Both pulls count them
		// as gains, so subtract their contribution before deciding to swap.
		shared := between[pair(a, b)]
		if !(pull[a]-shared > pull[b]+shared) {
			return
		}
		layer[i], layer[i+1] = b, a
		s.nl[a].order, s.nl[b].order = float64(i+1), float64(i)
		pull[a] -= 2 * shared
		pull[b] += 2 * shared
	}
	// One pass from each end lets a node move across an entire free run.
	// Each trial is constant time even when many parallel edges share a node.
	for rank, layer := range layers {
		if active[rank] {
			for i := 0; i+1 < len(layer); i++ {
				swapIfShorter(layer, i)
			}
			for i := len(layer) - 2; i >= 0; i-- {
				swapIfShorter(layer, i)
			}
		}
	}
}

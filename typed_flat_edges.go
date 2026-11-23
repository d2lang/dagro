package dagro

import (
	"math"
	"sort"
	"strings"
)

// positionWithFlatEdges gives the horizontal coordinate assignment room for
// wide edge labels. Keeping a label between its endpoints' centers also keeps
// it inside their common cluster, even when unrelated clusters flank the row.
// Only the extra separation survives; the published node sizes stay intact.
func (s *lstate) positionWithFlatEdges(flat []detachedEdge) {
	if len(flat) == 0 {
		s.position()
		return
	}
	dir := lowerRankdir(s.rankdir)
	var extra map[int32]float64
	for _, f := range flat {
		l := &s.el[f.label]
		// The detached labels missed adjustCoordinateSystem. Undo the original
		// side-label padding, then use the canonical top-to-bottom axes.
		if strings.ToLower(l.labelpos) != "c" {
			if s.rankdir == "TB" || s.rankdir == "BT" {
				l.width -= l.labeloffset
			} else {
				l.height -= l.labeloffset
			}
		}
		if dir == "lr" || dir == "rl" {
			l.width, l.height = l.height, l.width
		}
		v, w := f.e.v, f.e.w
		shortfall := l.width - ((s.nl[v].width+s.nl[w].width)/2 + s.nodesep)
		if l.height > 0 && shortfall > 0 {
			if extra == nil {
				extra = make(map[int32]float64)
			}
			extra[v] = math.Max(extra[v], shortfall)
			extra[w] = math.Max(extra[w], shortfall)
		}
	}
	for v, amount := range extra {
		width := s.nl[v].width
		s.nl[v].width = width + amount
		extra[v] = width
	}
	s.position()
	for v, width := range extra {
		s.nl[v].width = width
	}
	s.routeFlatEdges(flat)
}

// Flat edges are absent while Dagre orders and positions the graph: the
// crossing minimizer and Brandes–Köpf alignments require neighbors in
// different layers. Once ordering is final, route adjacent endpoints directly
// and allocate space above the layer for the remaining spans and their labels.
func (s *lstate) routeFlatEdges(flat []detachedEdge) {
	layers := s.buildLayerMatrix(s.p.leaves)
	rows := make([][]flatSpan, len(layers))
	// Number of obstacles preceding each node, ignoring the zero-sized chain
	// and border dummies. A cluster boundary can be crossed by a flat edge.
	before := make([]int, len(s.nl))
	for _, layer := range layers {
		count := 0
		for _, v := range layer {
			before[v] = count
			switch s.nl[v].dummy {
			case dkNone, dkEdgeLabel, dkSelfEdge:
				count++
			}
		}
	}

	straight := make(map[[2]int32]int32)
	dir := lowerRankdir(s.rankdir)
	for _, f := range flat {
		l := &s.el[f.label]

		v, w := f.e.v, f.e.w
		if s.nl[v].order > s.nl[w].order {
			v, w = w, v
		}
		a, b := s.nl[v], s.nl[w]
		gapLeft, gapRight := a.x+a.width/2, b.x-b.width/2
		labeled := l.width > 0 && l.height > 0
		pair := [2]int32{v, w}
		_, used := straight[pair]
		if !labeled && !used && before[w] == before[v]+1 && gapRight > gapLeft {
			straight[pair] = f.label
			continue
		}

		span := flatSpan{edge: f, lo: a.x, hi: b.x}
		if labeled {
			middle := (a.x + b.x) / 2
			span.lo = math.Min(span.lo, middle-l.width/2)
			span.hi = math.Max(span.hi, middle+l.width/2)
			// Place a side label relative to the direction of travel; BT and
			// LR reflect the canonical coordinate system on the way out.
			var side float64
			switch strings.ToLower(l.labelpos) {
			case "l":
				side = -1
			case "r":
				side = 1
			}
			if (f.e.v != v) != (dir == "bt" || dir == "lr") {
				side = -side
			}
			span.labelDY = side * (l.height/2 + l.labeloffset)
		}
		rank := int(a.rank)
		rows[rank] = append(rows[rank], span)
	}

	// Reserve space before moving any geometry derived from dummies (cluster
	// bounds, self-loops, long-edge bends). A later row and all its dummies
	// move together, preserving ranks and the separation from preceding rows.
	shift := 0.0
	clearance := math.Max(1, s.edgesep)
	for rank, layer := range layers {
		spans := rows[rank]
		if len(spans) > 0 {
			top := math.Inf(1)
			for _, v := range layer {
				n := s.nl[v]
				top = math.Min(top, n.y-n.height/2)
			}
			shift += s.routeFlatSpans(spans, top+shift, clearance)
		}
		for _, v := range layer {
			s.nl[v].y += shift
		}
	}

	for pair, label := range straight {
		a, b := s.nl[pair[0]], s.nl[pair[1]]
		middle := Point{X: (a.x + a.width/2 + b.x - b.width/2) / 2, Y: a.y}
		s.setPoints(label, append(s.pointBuf(label), middle))
	}
}

type flatSpan struct {
	edge    detachedEdge
	lo, hi  float64 // horizontal extent of path and label
	labelDY float64 // signed offset of label center from track
	track   int
}

type flatTrack struct {
	above, below float64 // room on either side of the track for labels
	y            float64
}

func (s *lstate) routeFlatSpans(spans []flatSpan, top, clearance float64) float64 {
	// Sweep interval starts and ends. Disjoint spans can share a track; a
	// set of parallel edges gets distinct tracks without a quadratic scan.
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
	ends := make([]int, len(spans))
	for i := range ends {
		ends[i] = i
	}
	sort.SliceStable(ends, func(i, j int) bool { return spans[ends[i]].hi < spans[ends[j]].hi })
	var tracks []flatTrack
	var free []int
	nextEnd := 0
	for i := range spans {
		r := &spans[i]
		for nextEnd < len(ends) && spans[ends[nextEnd]].hi+clearance <= r.lo {
			free = append(free, spans[ends[nextEnd]].track)
			nextEnd++
		}
		if len(free) == 0 {
			r.track = len(tracks)
			tracks = append(tracks, flatTrack{})
		} else {
			r.track = free[len(free)-1]
			free = free[:len(free)-1]
		}
		track := &tracks[r.track]
		l := &s.el[r.edge.label]
		if l.width > 0 && l.height > 0 {
			track.above = math.Max(track.above, l.height/2-r.labelDY)
			track.below = math.Max(track.below, l.height/2+r.labelDY)
		}
	}

	y := top
	for i := range tracks {
		t := &tracks[i]
		t.y = y + clearance + t.above
		y = t.y + t.below
	}
	for _, r := range spans {
		f := r.edge
		v, w := &s.nl[f.e.v], &s.nl[f.e.w]
		y := tracks[r.track].y
		s.setPoints(f.label, append(s.pointBuf(f.label), Point{X: v.x, Y: y}, Point{X: w.x, Y: y}))
		l := &s.el[f.label]
		if l.width > 0 && l.height > 0 {
			l.x, l.y, l.hasX, l.hasY = (v.x+w.x)/2, y+r.labelDY, true, true
		}
	}
	return y + clearance - top
}

// The flat router places its own side labels. Restore these edges after
// fixupEdgeLabelCoords and before the standard coordinate transform, graph
// translation, and clipping against the endpoint rectangles.
func (s *lstate) restoreFlatEdges(flat []detachedEdge) {
	for _, f := range flat {
		s.setEdgeRef(f.e, f.label)
	}
}

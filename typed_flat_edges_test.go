package dagro

import (
	"math"
	"testing"
)

// Even when a label is much wider than its nodes, it should stay inside
// their cluster. Its requested side follows the edge's *original* direction
// after the layout's reflections or rotations.
func TestSameRankFlatLabelGeometry(t *testing.T) {
	for _, dir := range []string{"TB", "BT", "LR", "RL"} {
		for _, reverse := range []bool{false, true} {
			for _, pos := range []string{"l", "r", "c"} {
				in := DefaultLayoutInput()
				in.RankDir = dir
				in.RankGroups = [][]string{{"inside", "peer"}}
				in.Nodes = []LayoutNode{
					{ID: "frame", Parent: -1},
					{ID: "inside", Parent: 0, Width: 20, Height: 15},
					{ID: "peer", Parent: 0, Width: 30, Height: 24},
					{ID: "upstream", Parent: -1, Width: 40, Height: 30},
				}
				e := NewLayoutEdge(1, 2)
				if reverse {
					e.V, e.W = e.W, e.V
				}
				e.Width, e.Height, e.LabelPos = 520, 380, pos
				in.Edges = []LayoutEdge{e, NewLayoutEdge(3, 1), NewLayoutEdge(2, 2)}
				var out LayoutOutput
				if err := LayoutTyped(&in, &out); err != nil {
					t.Fatalf("%s (reverse=%t): %v", dir, reverse, err)
				}
				line := out.Edges[0]
				// The middle segment of the four-point route carries the label.
				if !line.HasXY || len(line.Points) != 4 {
					t.Fatalf("%s: missing flat label or track: %+v", dir, line)
				}
				a, b := line.Points[1], line.Points[2]
				dx, dy := b.X-a.X, b.Y-a.Y
				signedDistance := (dx*(line.Y-a.Y) - dy*(line.X-a.X)) / math.Hypot(dx, dy)
				want := 0.0
				if pos != "c" {
					h := e.Height
					if dy != 0 {
						h = e.Width
					}
					want = e.LabelOffset + h/2
					if pos == "l" {
						want = -want
					}
				}
				if math.Abs(want-signedDistance) > 1e-8 {
					t.Errorf("%s (reverse=%t pos=%s): label distance %g, want %g", dir, reverse, pos, signedDistance, want)
				}
				box := NodeLayout{X: line.X, Y: line.Y, Width: e.Width, Height: e.Height}
				if !sameRankContains(out.Nodes[0], box) {
					t.Errorf("%s: label %+v extends outside its frame %+v", dir, box, out.Nodes[0])
				}
			}
		}
	}
}

// Disjoint horizontal intervals can occupy the same track. Overlapping
// intervals (including parallel edges) must be separated.
func TestSameRankFlatEdgeTrackReuse(t *testing.T) {
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{
		{ID: "a", Parent: -1, Width: 40, Height: 20},
		{ID: "b", Parent: -1, Width: 40, Height: 20},
		{ID: "c", Parent: -1, Width: 40, Height: 20},
		{ID: "d", Parent: -1, Width: 40, Height: 20},
	}
	in.RankGroups = [][]string{{"a", "b", "c", "d"}}
	for _, edge := range []struct {
		v, w int
		name string
	}{{0, 1, "left"}, {2, 3, "right"}, {0, 1, "parallel"}} {
		e := NewLayoutEdge(edge.v, edge.w)
		e.HasName, e.Name = true, edge.name
		e.Width, e.Height, e.LabelPos = 80, 12, "c"
		in.Edges = append(in.Edges, e)
	}
	var out LayoutOutput
	if err := LayoutTyped(&in, &out); err != nil {
		t.Fatal(err)
	}
	y0, y1, y2 := out.Edges[0].Y, out.Edges[1].Y, out.Edges[2].Y
	if (y1 != y0 && y1 != y2) || math.Abs(y0-y2) < in.EdgeSep+in.Edges[0].Height {
		t.Fatalf("edge track label centers = %v, %v, %v", y0, y1, y2)
	}
}

// Reciprocal labeled transitions must retain distinct arrowheads even when a
// regular edge also enters the peer perpendicularly to the rank.
func TestSameRankReciprocalPorts(t *testing.T) {
	for _, dir := range []string{"TB", "BT", "LR", "RL"} {
		t.Run(dir, func(t *testing.T) {
			in := DefaultLayoutInput()
			in.RankDir = dir
			in.Nodes = []LayoutNode{
				{ID: "source", Parent: -1, Width: 60, Height: 60},
				{ID: "playing", Parent: -1, Width: 80, Height: 80},
				{ID: "paused", Parent: -1, Width: 80, Height: 80},
			}
			in.RankGroups = [][]string{{"playing", "paused"}}
			in.Edges = []LayoutEdge{NewLayoutEdge(0, 1), NewLayoutEdge(1, 2), NewLayoutEdge(2, 1)}
			for i := 1; i < len(in.Edges); i++ {
				in.Edges[i].Width, in.Edges[i].Height, in.Edges[i].LabelPos = 25, 13, "c"
			}
			var out LayoutOutput
			if err := LayoutTyped(&in, &out); err != nil {
				t.Fatal(err)
			}
			direct, back := out.Edges[1], out.Edges[2]
			if len(direct.Points) != 3 || !direct.HasXY || direct.Points[1] != (Point{X: direct.X, Y: direct.Y}) {
				t.Fatalf("label should fit the direct edge between adjacent boxes: %+v", direct)
			}
			incoming := out.Edges[0].Points
			if len(back.Points) != 4 || back.Points[3] == incoming[len(incoming)-1] {
				t.Fatalf("return arrow shares the incoming tip: %+v, %+v", back, incoming)
			}
			for _, i := range []int{0, 2} {
				a, b := back.Points[i], back.Points[i+1]
				if a.X != b.X && a.Y != b.Y {
					t.Fatalf("diagonal stem between %+v and %+v", a, b)
				}
			}
		})
	}
}

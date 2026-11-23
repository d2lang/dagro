package dagro

import (
	"math"
	"reflect"
	"testing"
)

// Commerce belongs to a tier inside the system; the payment channel and
// Payments both live outside the system. Their edges still run along one rank.
func paymentRankInput() LayoutInput {
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{
		{ID: "system", Parent: -1},
		{ID: "tier", Parent: 0},
		{ID: "before", Parent: -1, Width: 42, Height: 24},
		{ID: "commerce", Parent: 1, Width: 96, Height: 48},
		{ID: "channel", Parent: -1, Width: 18, Height: 18},
		{ID: "payments", Parent: -1, Width: 72, Height: 30},
		{ID: "after", Parent: -1, Width: 42, Height: 24},
	}
	in.Edges = []LayoutEdge{
		NewLayoutEdge(2, 3), NewLayoutEdge(3, 4), NewLayoutEdge(4, 5), NewLayoutEdge(5, 6),
	}
	in.RankGroups = [][]string{{"commerce", "channel"}, {"channel", "payments"}}
	return in
}

func TestSameRankCrossContainer(t *testing.T) {
	for _, dir := range []string{"TB", "BT", "LR", "RL"} {
		for _, ranker := range []string{"network-simplex", "longest-path", "tight-tree"} {
			t.Run(dir+"/"+ranker, func(t *testing.T) {
				in := paymentRankInput()
				in.RankDir, in.Ranker = dir, ranker
				var out LayoutOutput
				if err := LayoutTyped(&in, &out); err != nil {
					t.Fatal(err)
				}
				rank := func(n NodeLayout) float64 {
					switch dir {
					case "BT":
						return -n.Y
					case "LR":
						return n.X
					case "RL":
						return -n.X
					default:
						return n.Y
					}
				}
				system, tier := out.Nodes[0], out.Nodes[1]
				commerce, channel, payments := out.Nodes[3], out.Nodes[4], out.Nodes[5]
				if rank(commerce) != rank(channel) || rank(commerce) != rank(payments) {
					t.Fatalf("different rank centers: commerce %+v, channel %+v, payments %+v", commerce, channel, payments)
				}
				if !(rank(out.Nodes[2]) < rank(commerce) && rank(commerce) < rank(out.Nodes[6])) {
					t.Errorf("expected the group to occupy an interior rank: %+v", out.Nodes)
				}
				if !sameRankContains(system, tier) || !sameRankContains(tier, commerce) {
					t.Errorf("nested bounds do not enclose children: system %+v, tier %+v, commerce %+v", system, tier, commerce)
				}
				for _, n := range []NodeLayout{channel, payments} {
					if sameRankOverlaps(system, n) {
						t.Errorf("external node %+v overlaps container %+v", n, system)
					}
				}
				for i, e := range in.Edges {
					checkSameRankRoute(t, out.Edges[i], out.Nodes[e.V], out.Nodes[e.W])
				}
				// The Graph API must expose the same constraints and preserve each
				// named object and edge. Compare at the same generated-ID seed.
				typedAPICheck(t, t.Name(), &in)
			})
		}
	}
}

func sameRankContains(outer, inner NodeLayout) bool {
	return math.Abs(outer.X-inner.X)+inner.Width/2 <= outer.Width/2+1e-8 &&
		math.Abs(outer.Y-inner.Y)+inner.Height/2 <= outer.Height/2+1e-8
}

func sameRankOverlaps(a, b NodeLayout) bool {
	return math.Abs(a.X-b.X) < (a.Width+b.Width)/2-1e-8 &&
		math.Abs(a.Y-b.Y) < (a.Height+b.Height)/2-1e-8
}

func checkSameRankRoute(t *testing.T, edge EdgeLayout, src, dst NodeLayout) {
	t.Helper()
	points := edge.Points
	if len(points) < 2 {
		t.Fatalf("missing route from %+v to %+v: %v", src, dst, points)
	}
	onBoundary := func(p Point, n NodeLayout) bool {
		dx := math.Abs(p.X-n.X) - n.Width/2
		dy := math.Abs(p.Y-n.Y) - n.Height/2
		return math.Abs(math.Max(dx, dy)) < 1e-8
	}
	if !onBoundary(points[0], src) || !onBoundary(points[len(points)-1], dst) {
		t.Errorf("route %v does not start/end on %+v / %+v", points, src, dst)
	}
}

func TestSameRankCycleAfterContraction(t *testing.T) {
	// The original is acyclic. Equating the first and last nodes creates a
	// cycle in the rank graph; both visible edges retain their directions.
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{
		{ID: "a", Parent: -1, Width: 20, Height: 20},
		{ID: "b", Parent: -1, Width: 20, Height: 20},
		{ID: "c", Parent: -1, Width: 20, Height: 20},
	}
	in.Edges = []LayoutEdge{NewLayoutEdge(0, 1), NewLayoutEdge(1, 2)}
	in.RankGroups = [][]string{{"a", "c"}}
	for _, acyclicer := range []string{"", "greedy"} {
		in.Acyclicer = acyclicer
		var out LayoutOutput
		if err := LayoutTyped(&in, &out); err != nil {
			t.Fatal(err)
		}
		if out.Nodes[0].Y != out.Nodes[2].Y || out.Nodes[0].Y == out.Nodes[1].Y {
			t.Fatalf("acyclicer %q: bad ranks %+v", acyclicer, out.Nodes)
		}
		for i, edge := range in.Edges {
			checkSameRankRoute(t, out.Edges[i], out.Nodes[edge.V], out.Nodes[edge.W])
		}
	}
}

func TestSameRankFlatMultiedges(t *testing.T) {
	in := DefaultLayoutInput()
	in.MarginX, in.MarginY = 11, 17
	in.Nodes = []LayoutNode{
		{ID: "a", Parent: -1, Width: 60, Height: 35},
		{ID: "b", Parent: -1, Width: 78, Height: 50},
		{ID: "c", Parent: -1, Width: 45, Height: 25},
	}
	in.RankGroups = [][]string{{"a", "b", "c"}}
	for _, endpoints := range [][2]int{{0, 1}, {1, 2}, {2, 0}, {0, 1}} {
		e := NewLayoutEdge(endpoints[0], endpoints[1])
		e.Width, e.Height = 45, 18
		e.Name, e.HasName = string(rune('a'+len(in.Edges))), true
		in.Edges = append(in.Edges, e)
	}
	var out LayoutOutput
	if err := LayoutTyped(&in, &out); err != nil {
		t.Fatal(err)
	}
	if !(out.Nodes[0].Y == out.Nodes[1].Y && out.Nodes[0].Y == out.Nodes[2].Y) {
		t.Fatalf("different rank centers: %+v", out.Nodes)
	}
	bounds := NodeLayout{X: out.Width / 2, Y: out.Height / 2, Width: out.Width - 2*in.MarginX, Height: out.Height - 2*in.MarginY}
	for i, e := range in.Edges {
		route := out.Edges[i]
		checkSameRankRoute(t, route, out.Nodes[e.V], out.Nodes[e.W])
		label := NodeLayout{X: route.X, Y: route.Y, Width: e.Width, Height: e.Height}
		if !route.HasXY || !sameRankContains(bounds, label) {
			t.Errorf("flat edge %s: missing or out-of-bounds label %+v", e.Name, route)
		}
		for _, node := range out.Nodes {
			if sameRankOverlaps(label, node) {
				t.Errorf("flat edge %s: label %+v overlaps node %+v", e.Name, label, node)
			}
		}
		for _, p := range route.Points {
			if !sameRankContains(bounds, NodeLayout{X: p.X, Y: p.Y}) {
				t.Errorf("route point %+v lies outside margins %+v", p, bounds)
			}
		}
		// Every pair of nodes is connected, so at least one flat edge
		// must pass the third node. Check its crossing of that centerline.
		for k, other := range out.Nodes {
			if k == e.V || k == e.W {
				continue
			}
			for j := 1; j < len(route.Points); j++ {
				a, b := route.Points[j-1], route.Points[j]
				if !(math.Min(a.X, b.X) < other.X && other.X < math.Max(a.X, b.X)) {
					continue
				}
				y := a.Y + (b.Y-a.Y)*(other.X-a.X)/(b.X-a.X)
				if math.Abs(y-other.Y) < other.Height/2 {
					t.Errorf("edge %s passes through another node: %v and %+v", e.Name, route.Points, other)
				}
			}
		}
	}
	if reflect.DeepEqual(out.Edges[0].Points, out.Edges[3].Points) {
		t.Errorf("parallel flat edges share an entire route: %v", out.Edges[0].Points)
	}
}

func TestSameRankValidation(t *testing.T) {
	for _, name := range []string{"absent", "system"} {
		in := paymentRankInput()
		in.RankGroups = [][]string{{name}}
		out := LayoutOutput{Width: 17}
		if err := LayoutTyped(&in, &out); err == nil || out.Width != 17 {
			t.Errorf("invalid group member %q: err %v, output %+v", name, err, out)
		}
	}
}

func TestSameRankTrivialGroupsPreserveLayout(t *testing.T) {
	in := paymentRankInput()
	in.RankGroups = nil
	var original LayoutOutput
	if err := LayoutTyped(&in, &original); err != nil {
		t.Fatal(err)
	}
	// Interleave an actual constraint to exercise pooled state as well.
	in.RankGroups = [][]string{{"commerce", "payments"}}
	var current LayoutOutput
	if err := LayoutTyped(&in, &current); err != nil {
		t.Fatal(err)
	}
	for _, groups := range [][][]string{nil, {}, {{}, {"commerce"}, {"payments", "payments"}}} {
		in.RankGroups = groups
		if err := LayoutTyped(&in, &current); err != nil {
			t.Fatal(err)
		}
		typedAPICompare(t, "only trivial groups", original, current)
	}
}

package dagro

import (
	"math"
	"testing"
)

func externalChannelInput() LayoutInput {
	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{
		{ID: "system", Parent: -1},
		{ID: "upstream", Parent: 0, Width: 40, Height: 20},
		{ID: "commerce", Parent: 0, Width: 60, Height: 20},
		{ID: "payments", Parent: -1, Width: 40, Height: 20},
		{ID: "channel", Parent: -1, Width: 20, Height: 20},
	}
	in.Edges = []LayoutEdge{NewLayoutEdge(1, 2), NewLayoutEdge(2, 4), NewLayoutEdge(4, 3)}
	in.RankGroups = [][]string{{"commerce", "channel", "payments"}}
	return in
}

// The provider and channel are both outside the system. Only the channel has
// a connection across the boundary, so it should sit between the two peers.
func TestSameRankOrderExternalChannel(t *testing.T) {
	in := externalChannelInput()
	var out LayoutOutput
	if err := LayoutTyped(&in, &out); err != nil {
		t.Fatal(err)
	}
	commerce, payments, channel := out.Nodes[2].X, out.Nodes[3].X, out.Nodes[4].X
	if !(math.Min(commerce, payments) < channel && channel < math.Max(commerce, payments)) {
		t.Fatalf("channel at %g falls outside commerce (%g) and payments (%g)", channel, commerce, payments)
	}
	for _, edge := range out.Edges[1:] {
		for _, p := range edge.Points {
			if p.Y != out.Nodes[2].Y {
				t.Fatalf("expected a straight horizontal edge, got %v", edge.Points)
			}
		}
	}
}

func TestSameRankOrderWeightedSpans(t *testing.T) {
	for _, tc := range []struct {
		name           string
		paymentWeight  float64
		channelNearest bool
	}{
		{"channel has greater weight", 2, true},
		{"payments has greater weight", 4, false},
		{"tie keeps existing order", 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := externalChannelInput()
			in.Edges[1].Weight = 3
			// These two edges keep the same length in either arrangement.
			in.Edges[2].Weight = 100
			back := NewLayoutEdge(3, 4)
			back.Weight = 50
			peer := NewLayoutEdge(2, 3)
			peer.Weight = tc.paymentWeight
			in.Edges = append(in.Edges, back, peer)
			var out LayoutOutput
			if err := LayoutTyped(&in, &out); err != nil {
				t.Fatal(err)
			}
			commerce := out.Nodes[2].X
			channelDistance := math.Abs(out.Nodes[4].X - commerce)
			paymentDistance := math.Abs(out.Nodes[3].X - commerce)
			if (channelDistance < paymentDistance) != tc.channelNearest {
				t.Fatalf("channel distance %g, payments distance %g, want channel nearest %t", channelDistance, paymentDistance, tc.channelNearest)
			}
		})
	}
}

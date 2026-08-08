package dagro

import (
	"reflect"
	"testing"
)

func TestAdjustCoordinateSystem(t *testing.T) {
	for _, tt := range []struct {
		rankdir    string
		wantWidth  float64
		wantHeight float64
	}{
		{rankdir: "TB", wantWidth: 100, wantHeight: 200},
		{rankdir: "BT", wantWidth: 100, wantHeight: 200},
		{rankdir: "LR", wantWidth: 200, wantHeight: 100},
		{rankdir: "RL", wantWidth: 200, wantHeight: 100},
	} {
		t.Run(tt.rankdir, func(t *testing.T) {
			g := NewGraph().SetGraph(Attrs{"rankdir": tt.rankdir}).
				SetNode("a", Attrs{"width": float64(100), "height": float64(200)}).
				SetNode("b", Attrs{"width": float64(0), "height": float64(0)}).
				SetEdge("a", "b", Attrs{"width": float64(10), "height": float64(20)})
			adjustCoordinateSystem(g)
			node := asAttrs(g.Node("a"))
			if num(node, "width") != tt.wantWidth || num(node, "height") != tt.wantHeight {
				t.Fatalf("node dimensions = %#v", node)
			}
			edge := asAttrs(g.EdgeByArgs("a", "b"))
			if tt.rankdir == "LR" || tt.rankdir == "RL" {
				if num(edge, "width") != 20 || num(edge, "height") != 10 {
					t.Fatalf("edge dimensions = %#v", edge)
				}
			} else if num(edge, "width") != 10 || num(edge, "height") != 20 {
				t.Fatalf("edge dimensions changed = %#v", edge)
			}
		})
	}
}

func TestUndoCoordinateSystem(t *testing.T) {
	for _, tt := range []struct {
		rankdir   string
		wantNode  Attrs
		wantPts   []Point
		wantLabel Point
	}{
		{
			rankdir:  "TB",
			wantNode: Attrs{"x": float64(20), "y": float64(40), "width": float64(100), "height": float64(200)},
			wantPts:  []Point{{X: 1, Y: 2}, {X: 3, Y: 4}}, wantLabel: Point{X: 5, Y: 6},
		},
		{
			rankdir:  "BT",
			wantNode: Attrs{"x": float64(20), "y": float64(-40), "width": float64(100), "height": float64(200)},
			wantPts:  []Point{{X: 1, Y: -2}, {X: 3, Y: -4}}, wantLabel: Point{X: 5, Y: -6},
		},
		{
			rankdir:  "LR",
			wantNode: Attrs{"x": float64(40), "y": float64(20), "width": float64(200), "height": float64(100)},
			wantPts:  []Point{{X: 2, Y: 1}, {X: 4, Y: 3}}, wantLabel: Point{X: 6, Y: 5},
		},
		{
			rankdir:  "RL",
			wantNode: Attrs{"x": float64(-40), "y": float64(20), "width": float64(200), "height": float64(100)},
			wantPts:  []Point{{X: -2, Y: 1}, {X: -4, Y: 3}}, wantLabel: Point{X: -6, Y: 5},
		},
	} {
		t.Run(tt.rankdir, func(t *testing.T) {
			g := NewGraph().SetGraph(Attrs{"rankdir": tt.rankdir}).
				SetNode("a", Attrs{
					"x": float64(20), "y": float64(40),
					"width": float64(100), "height": float64(200),
				}).
				SetNode("b", Attrs{
					"x": float64(0), "y": float64(0),
					"width": float64(0), "height": float64(0),
				}).
				SetEdge("a", "b", Attrs{
					"x": float64(5), "y": float64(6),
					"width": float64(10), "height": float64(20),
					"points": []Point{{X: 1, Y: 2}, {X: 3, Y: 4}},
				})
			undoCoordinateSystem(g)
			if got := asAttrs(g.Node("a")); !reflect.DeepEqual(got, tt.wantNode) {
				t.Fatalf("node = %#v, want %#v", got, tt.wantNode)
			}
			edge := asAttrs(g.EdgeByArgs("a", "b"))
			if got := edge["points"].([]Point); !reflect.DeepEqual(got, tt.wantPts) {
				t.Fatalf("points = %+v, want %+v", got, tt.wantPts)
			}
			if got := (Point{X: num(edge, "x"), Y: num(edge, "y")}); got != tt.wantLabel {
				t.Fatalf("label point = %+v, want %+v", got, tt.wantLabel)
			}
		})
	}
}

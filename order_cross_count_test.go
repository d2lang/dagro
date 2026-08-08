package dagro

import "testing"

func TestCrossCount(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*Graph)
		layering [][]string
		want     float64
	}{
		{name: "empty", layering: nil, want: 0},
		{
			name: "no crossings",
			build: func(g *Graph) {
				g.SetEdge("a1", "b1")
				g.SetEdge("a2", "b2")
			},
			layering: [][]string{{"a1", "a2"}, {"b1", "b2"}},
			want:     0,
		},
		{
			name: "one crossing",
			build: func(g *Graph) {
				g.SetEdge("a1", "b1")
				g.SetEdge("a2", "b2")
			},
			layering: [][]string{{"a1", "a2"}, {"b2", "b1"}},
			want:     1,
		},
		{
			name: "weighted crossing",
			build: func(g *Graph) {
				g.SetEdge("a1", "b1", Attrs{"weight": float64(2)})
				g.SetEdge("a2", "b2", Attrs{"weight": float64(3)})
			},
			layering: [][]string{{"a1", "a2"}, {"b2", "b1"}},
			want:     6,
		},
		{
			name: "multiple layers",
			build: func(g *Graph) {
				g.SetPath([]string{"a1", "b1", "c1"})
				g.SetPath([]string{"a2", "b2", "c2"})
			},
			layering: [][]string{{"a1", "a2"}, {"b2", "b1"}, {"c1", "c2"}},
			want:     2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := newOrderTestGraph(false)
			if test.build != nil {
				test.build(g)
			}
			if got := crossCount(g, test.layering); got != test.want {
				t.Fatalf("crossCount = %v, want %v", got, test.want)
			}
		})
	}

	t.Run("source graph number one", func(t *testing.T) {
		g := newOrderTestGraph(false)
		g.SetPath([]string{"a", "b", "c"})
		g.SetPath([]string{"d", "e", "c"})
		g.SetPath([]string{"a", "f", "i"})
		g.SetEdge("a", "e")
		if got := crossCount(g, [][]string{{"a", "d"}, {"b", "e", "f"}, {"c", "i"}}); got != 1 {
			t.Fatalf("first crossCount = %v, want 1", got)
		}
		if got := crossCount(g, [][]string{{"d", "a"}, {"e", "b", "f"}, {"c", "i"}}); got != 0 {
			t.Fatalf("second crossCount = %v, want 0", got)
		}
	})
}

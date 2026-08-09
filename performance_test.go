package dagro

import (
	"fmt"
	"strconv"
	"testing"
)

func BenchmarkGraphNodesNumericOrder(b *testing.B) {
	for _, size := range []int{100, 1000, 2500} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			g := NewGraph()
			for i := 0; i < size; i++ {
				g.SetNode(strconv.Itoa(i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := len(g.Nodes()); got != size {
					b.Fatalf("Nodes length = %d, want %d", got, size)
				}
			}
		})
	}
}

func BenchmarkLayoutDeepCompound(b *testing.B) {
	for _, depth := range []int{20, 50, 100} {
		b.Run(fmt.Sprintf("depth-%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				g := benchmarkDeepCompoundGraph(depth)
				b.StartTimer()
				if err := Layout(g); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkDeepCompoundGraph(depth int) *Graph {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{
		"rankdir": "TB", "ranksep": float64(100), "nodesep": float64(60), "edgesep": float64(20),
	})
	for i := 0; i <= depth; i++ {
		id := strconv.Itoa(i)
		g.SetNode(id, Attrs{"id": id, "width": float64(100), "height": float64(50)})
		if i > 0 {
			if err := g.SetParent(id, strconv.Itoa(i-1)); err != nil {
				panic(err)
			}
		}
	}
	return g
}

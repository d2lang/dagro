package dagro

import (
	"math"
	"sort"
	"testing"
)

func unitFASWeight(Edge) float64 { return 1 }

func checkGreedyFAS(t *testing.T, g *Graph, fas []Edge) {
	t.Helper()
	n, m := g.NodeCount(), g.EdgeCount()
	for _, e := range fas {
		g.RemoveEdge(e)
	}
	if graphHasDirectedCycle(g) {
		t.Fatalf("feedback arc set did not break all cycles: %#v", fas)
	}
	bound := int(math.Floor(float64(m)/2)) - int(math.Floor(float64(n)/6))
	if len(fas) > bound {
		t.Fatalf("feedback arc set too large: %d > %d", len(fas), bound)
	}
}

func TestGreedyFASBasicGraphs(t *testing.T) {
	if got := greedyFAS(NewGraph(), unitFASWeight); len(got) != 0 {
		t.Fatalf("empty graph FAS = %#v", got)
	}
	if got := greedyFAS(NewGraph().SetNode("a"), unitFASWeight); len(got) != 0 {
		t.Fatalf("single node FAS = %#v", got)
	}
	acyclic := NewGraph().
		SetEdge("a", "b").
		SetEdge("b", "c").
		SetEdge("b", "d").
		SetEdge("a", "e")
	if got := greedyFAS(acyclic, unitFASWeight); len(got) != 0 {
		t.Fatalf("acyclic graph FAS = %#v", got)
	}

	cycle := NewGraph().SetEdge("a", "b").SetEdge("b", "a")
	checkGreedyFAS(t, cycle, greedyFAS(cycle, unitFASWeight))

	four := NewGraph().
		SetEdge("n1", "n2").
		SetPath([]string{"n2", "n3", "n4", "n5", "n2"}).
		SetEdge("n3", "n5").
		SetEdge("n4", "n2").
		SetEdge("n4", "n6")
	checkGreedyFAS(t, four, greedyFAS(four, unitFASWeight))
}

// This is the upstream "returns two edges for two 4-node cycles" case. It
// exercises cleanup and bucket selection across separate cyclic components.
func TestGreedyFASTwoFourNodeCycles(t *testing.T) {
	g := NewGraph().
		SetEdge("n1", "n2").
		SetPath([]string{"n2", "n3", "n4", "n5", "n2"}).
		SetEdge("n3", "n5").
		SetEdge("n4", "n2").
		SetEdge("n4", "n6").
		SetPath([]string{"n6", "n7", "n8", "n9", "n6"}).
		SetEdge("n7", "n9").
		SetEdge("n8", "n6").
		SetEdge("n8", "n10")
	fas := greedyFAS(g, unitFASWeight)
	checkGreedyFAS(t, g, fas)
}

func TestGreedyFASWeightsAndMultiedges(t *testing.T) {
	weightFn := func(g *Graph) func(Edge) float64 {
		return func(e Edge) float64 { return number(g.Edge(e)) }
	}

	g1 := NewGraph().SetEdge("n1", "n2", float64(2)).SetEdge("n2", "n1", float64(1))
	fas1 := greedyFAS(g1, weightFn(g1))
	if len(fas1) != 1 || fas1[0].V != "n2" || fas1[0].W != "n1" {
		t.Fatalf("weighted FAS #1 = %#v", fas1)
	}
	g2 := NewGraph().SetEdge("n1", "n2", float64(1)).SetEdge("n2", "n1", float64(2))
	fas2 := greedyFAS(g2, weightFn(g2))
	if len(fas2) != 1 || fas2[0].V != "n1" || fas2[0].W != "n2" {
		t.Fatalf("weighted FAS #2 = %#v", fas2)
	}

	multi := NewGraph(GraphOptions{Multigraph: true})
	multi.SetEdge("a", "b", float64(5), "foo")
	multi.SetEdge("b", "a", float64(2), "bar")
	multi.SetEdge("b", "a", float64(2), "baz")
	fas := greedyFAS(multi, weightFn(multi))
	sort.Slice(fas, func(i, j int) bool { return fas[i].Name < fas[j].Name })
	want := []Edge{
		{V: "b", W: "a", Name: "bar", HasName: true},
		{V: "b", W: "a", Name: "baz", HasName: true},
	}
	if len(fas) != len(want) || fas[0] != want[0] || fas[1] != want[1] {
		t.Fatalf("multigraph FAS = %#v, want %#v", fas, want)
	}
}

func TestAssignFASBucketUsesJavaScriptNumberTruthiness(t *testing.T) {
	newBuckets := func() []*fasList {
		return []*fasList{{}, {}, {}}
	}

	buckets := newBuckets()
	sink := &fasEntry{V: "sink", In: 1, Out: math.NaN()}
	assignFASBucket(buckets, 1, sink)
	if got := buckets[0].dequeue(); got != sink {
		t.Fatalf("NaN out-degree bucket = %#v, want sink bucket", got)
	}

	buckets = newBuckets()
	source := &fasEntry{V: "source", In: math.NaN(), Out: 1}
	assignFASBucket(buckets, 1, source)
	if got := buckets[len(buckets)-1].dequeue(); got != source {
		t.Fatalf("NaN in-degree bucket = %#v, want source bucket", got)
	}
}

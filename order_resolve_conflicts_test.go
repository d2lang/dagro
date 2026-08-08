package dagro

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func sortedOrderEntries(entries []orderEntry) []orderEntry {
	result := append([]orderEntry(nil), entries...)
	sort.SliceStable(result, func(i, j int) bool {
		return strings.Join(result[i].VS, "\x00") < strings.Join(result[j].VS, "\x00")
	})
	return result
}

func TestResolveConflicts(t *testing.T) {
	base := []orderEntry{
		{V: "a", Barycenter: 2, Weight: 3, HasBarycenter: true},
		{V: "b", Barycenter: 1, Weight: 2, HasBarycenter: true},
	}
	unchanged := []orderEntry{
		{VS: []string{"a"}, I: 0, Barycenter: 2, Weight: 3, HasBarycenter: true},
		{VS: []string{"b"}, I: 1, Barycenter: 1, Weight: 2, HasBarycenter: true},
	}

	t.Run("no constraints", func(t *testing.T) {
		got := sortedOrderEntries(resolveConflicts(base, NewGraph()))
		if !reflect.DeepEqual(got, unchanged) {
			t.Fatalf("resolveConflicts = %#v, want %#v", got, unchanged)
		}
	})

	t.Run("constraint without conflict", func(t *testing.T) {
		cg := NewGraph()
		cg.SetEdge("b", "a")
		got := sortedOrderEntries(resolveConflicts(base, cg))
		if !reflect.DeepEqual(got, unchanged) {
			t.Fatalf("resolveConflicts = %#v, want %#v", got, unchanged)
		}
	})

	t.Run("unconstrained node without barycenter remains a singleton", func(t *testing.T) {
		input := []orderEntry{
			{V: "a"},
			{V: "b", Barycenter: 1, Weight: 2, HasBarycenter: true},
		}
		got := sortedOrderEntries(resolveConflicts(input, NewGraph()))
		want := []orderEntry{
			{VS: []string{"a"}, I: 0},
			{VS: []string{"b"}, I: 1, Barycenter: 1, Weight: 2, HasBarycenter: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("resolveConflicts = %#v, want %#v", got, want)
		}
	})

	t.Run("coalesces a conflict", func(t *testing.T) {
		cg := NewGraph()
		cg.SetEdge("a", "b")
		got := resolveConflicts(base, cg)
		want := []orderEntry{{
			VS: []string{"a", "b"}, I: 0, Barycenter: 8.0 / 5.0,
			Weight: 5, HasBarycenter: true,
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("resolveConflicts = %#v, want %#v", got, want)
		}
	})

	t.Run("coalesces a chain", func(t *testing.T) {
		input := []orderEntry{
			{V: "a", Barycenter: 4, Weight: 1, HasBarycenter: true},
			{V: "b", Barycenter: 3, Weight: 1, HasBarycenter: true},
			{V: "c", Barycenter: 2, Weight: 1, HasBarycenter: true},
			{V: "d", Barycenter: 1, Weight: 1, HasBarycenter: true},
		}
		cg := NewGraph()
		cg.SetPath([]string{"a", "b", "c", "d"})
		got := resolveConflicts(input, cg)
		want := []orderEntry{{
			VS: []string{"a", "b", "c", "d"}, I: 0, Barycenter: 2.5,
			Weight: 4, HasBarycenter: true,
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("resolveConflicts = %#v, want %#v", got, want)
		}
	})

	t.Run("multiple constraints preserve topology", func(t *testing.T) {
		input := []orderEntry{
			{V: "a", Barycenter: 4, Weight: 1, HasBarycenter: true},
			{V: "b", Barycenter: 3, Weight: 1, HasBarycenter: true},
			{V: "c", Barycenter: 2, Weight: 1, HasBarycenter: true},
			{V: "d", Barycenter: 1, Weight: 1, HasBarycenter: true},
		}
		cg := NewGraph()
		cg.SetEdge("a", "c")
		cg.SetEdge("a", "d")
		cg.SetEdge("b", "c")
		cg.SetEdge("c", "d")
		got := resolveConflicts(input, cg)
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1: %#v", len(got), got)
		}
		positions := map[string]int{}
		for i, v := range got[0].VS {
			positions[v] = i
		}
		if !(positions["c"] > positions["a"] && positions["c"] > positions["b"] && positions["d"] > positions["c"]) {
			t.Fatalf("constraints violated by %v", got[0].VS)
		}
		if got[0].I != 0 || got[0].Barycenter != 2.5 || got[0].Weight != 4 {
			t.Fatalf("aggregate = %#v, want i=0 barycenter=2.5 weight=4", got[0])
		}
	})

	t.Run("missing barycenter always conflicts", func(t *testing.T) {
		input := []orderEntry{
			{V: "a"},
			{V: "b", Barycenter: 1, Weight: 2, HasBarycenter: true},
		}
		for _, edge := range [][2]string{{"a", "b"}, {"b", "a"}} {
			cg := NewGraph()
			cg.SetEdge(edge[0], edge[1])
			got := resolveConflicts(input, cg)
			if len(got) != 1 || got[0].Barycenter != 1 || got[0].Weight != 2 || !got[0].HasBarycenter {
				t.Fatalf("constraint %v produced %#v", edge, got)
			}
			wantVS := []string{edge[0], edge[1]}
			if !reflect.DeepEqual(got[0].VS, wantVS) {
				t.Fatalf("constraint %v order = %v, want %v", edge, got[0].VS, wantVS)
			}
		}
	})

	t.Run("ignores unrelated constraint edges", func(t *testing.T) {
		cg := NewGraph()
		cg.SetEdge("c", "d")
		got := sortedOrderEntries(resolveConflicts(base, cg))
		if !reflect.DeepEqual(got, unchanged) {
			t.Fatalf("resolveConflicts = %#v, want %#v", got, unchanged)
		}
	})
}

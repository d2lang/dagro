package dagro

import (
	"reflect"
	"testing"
)

func TestSortOrderEntries(t *testing.T) {
	tests := []struct {
		name      string
		entries   []orderEntry
		biasRight bool
		want      orderResult
	}{
		{
			name: "sorts by barycenter",
			entries: []orderEntry{
				{VS: []string{"a"}, I: 0, Barycenter: 2, Weight: 3, HasBarycenter: true},
				{VS: []string{"b"}, I: 1, Barycenter: 1, Weight: 2, HasBarycenter: true},
			},
			want: orderResult{VS: []string{"b", "a"}, Barycenter: 8.0 / 5.0, Weight: 5, HasBarycenter: true},
		},
		{
			name: "sorts supernodes",
			entries: []orderEntry{
				{VS: []string{"a", "c", "d"}, I: 0, Barycenter: 2, Weight: 3, HasBarycenter: true},
				{VS: []string{"b"}, I: 1, Barycenter: 1, Weight: 2, HasBarycenter: true},
			},
			want: orderResult{VS: []string{"b", "a", "c", "d"}, Barycenter: 8.0 / 5.0, Weight: 5, HasBarycenter: true},
		},
		{
			name: "left bias",
			entries: []orderEntry{
				{VS: []string{"a"}, I: 0, Barycenter: 1, Weight: 1, HasBarycenter: true},
				{VS: []string{"b"}, I: 1, Barycenter: 1, Weight: 1, HasBarycenter: true},
			},
			want: orderResult{VS: []string{"a", "b"}, Barycenter: 1, Weight: 2, HasBarycenter: true},
		},
		{
			name: "right bias",
			entries: []orderEntry{
				{VS: []string{"a"}, I: 0, Barycenter: 1, Weight: 1, HasBarycenter: true},
				{VS: []string{"b"}, I: 1, Barycenter: 1, Weight: 1, HasBarycenter: true},
			},
			biasRight: true,
			want:      orderResult{VS: []string{"b", "a"}, Barycenter: 1, Weight: 2, HasBarycenter: true},
		},
		{
			name: "nodes without barycenters",
			entries: []orderEntry{
				{VS: []string{"a"}, I: 0, Barycenter: 2, Weight: 1, HasBarycenter: true},
				{VS: []string{"b"}, I: 1, Barycenter: 6, Weight: 1, HasBarycenter: true},
				{VS: []string{"c"}, I: 2},
				{VS: []string{"d"}, I: 3, Barycenter: 3, Weight: 1, HasBarycenter: true},
			},
			want: orderResult{VS: []string{"a", "d", "c", "b"}, Barycenter: 11.0 / 3.0, Weight: 3, HasBarycenter: true},
		},
		{
			name: "no barycenters",
			entries: []orderEntry{
				{VS: []string{"a"}, I: 0}, {VS: []string{"b"}, I: 3},
				{VS: []string{"c"}, I: 2}, {VS: []string{"d"}, I: 1},
			},
			want: orderResult{VS: []string{"a", "d", "c", "b"}},
		},
		{
			name: "zero barycenter remains present",
			entries: []orderEntry{
				{VS: []string{"a"}, I: 0, Barycenter: 0, Weight: 1, HasBarycenter: true},
				{VS: []string{"b"}, I: 3}, {VS: []string{"c"}, I: 2}, {VS: []string{"d"}, I: 1},
			},
			want: orderResult{VS: []string{"a", "d", "c", "b"}, Barycenter: 0, Weight: 1, HasBarycenter: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := sortOrderEntries(test.entries, test.biasRight)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("sortOrderEntries = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSortOrderEntriesRestoresEveryReversedPartner(t *testing.T) {
	entry := func(v string, i int, barycenter float64) orderEntry {
		return orderEntry{
			VS: []string{v}, I: i, Barycenter: barycenter, Weight: 1, HasBarycenter: true,
		}
	}
	got := sortOrderEntriesWithReversedPairs(
		[]orderEntry{entry("forward", 0, 1), entry("other", 3, 2)},
		[]reversedOrderPair{
			{key: "forward", entry: entry("reverse-1", 1, 1)},
			{key: "forward", entry: entry("reverse-2", 2, 1)},
		},
		false,
	)
	want := []string{"forward", "reverse-1", "reverse-2", "other"}
	if !reflect.DeepEqual(got.VS, want) {
		t.Fatalf("reversed partners = %v, want %v", got.VS, want)
	}
}

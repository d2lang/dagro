package dagro

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSameRankGraphAttribute(t *testing.T) {
	var attrs Attrs
	if err := json.Unmarshal([]byte(`{"RankGroups":[null,[],["a","b"]]}`), &attrs); err != nil {
		t.Fatal(err)
	}
	g := NewGraph().SetGraph(attrs)
	g.SetNode("a", Attrs{"width": 40, "height": 20})
	g.SetNode("b", Attrs{"width": 40, "height": 20})
	g.SetEdge("a", "b", Attrs{})
	if err := Layout(g); err != nil {
		t.Fatal(err)
	}
	a, b := asAttrs(g.Node("a")), asAttrs(g.Node("b"))
	if a["y"] != b["y"] || a["x"] == b["x"] {
		t.Fatalf("expected adjacent nodes on one row, got %v and %v", a, b)
	}
}

func TestSameRankMalformedAttribute(t *testing.T) {
	for _, tc := range []struct {
		attrs Attrs
		want  string
	}{
		{Attrs{"rankgroups": "a,b"}, "list of lists"},
		{Attrs{"rankgroups": []any{"a"}}, "list of node IDs"},
		{Attrs{"rankgroups": []any{[]any{"a", 10}}}, "non-string node ID"},
		{Attrs{"RankGroups": nil, "rankgroups": nil}, "duplicate"},
	} {
		g := NewGraph().SetGraph(tc.attrs)
		a := Attrs{"width": 40, "height": 20}
		g.SetNode("a", a)
		if err := Layout(g); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("for %v got %v, want %q", tc.attrs, err, tc.want)
		}
		if _, wrote := a["x"]; wrote {
			t.Errorf("invalid input changed node coordinates: %v", a)
		}
	}
}

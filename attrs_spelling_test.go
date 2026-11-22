package dagro

import (
	"fmt"
	"sort"
	"testing"
)

// Several spellings of one attribute key: canonicalize keeps the spelling its
// range over the label visits last, so the original's result follows Go's map
// iteration order. The typed engine reproduces that mechanism and must show
// the same set of outcomes (it used to pick one spelling always).
func TestDuplicateSpellingsFollowMapOrder(t *testing.T) {
	for _, label := range []map[string]any{
		{"width": 10.0, "Width": 20.0, "height": 5.0},
		{"WIDTH": 30.0, "Width": 20.0, "height": 5.0},
		{"width": 10.0, "WIDTH": 30.0, "wIdth": 40.0},
	} {
		outcomes := func(layout func(a map[string]any) string) []string {
			set := map[string]bool{}
			for i := 0; i < 400; i++ {
				m := map[string]any{}
				for k, v := range label {
					m[k] = v
				}
				set[layout(m)] = true
			}
			var out []string
			for k := range set {
				out = append(out, k)
			}
			sort.Strings(out)
			return out
		}
		want := outcomes(func(a map[string]any) string {
			g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{})
			g.SetNode("a", Attrs(a))
			if err := referenceLayoutOpts(g); err != nil {
				return err.Error()
			}
			return fmt.Sprint(g.Graph().(Attrs)["width"])
		})
		got := outcomes(func(a map[string]any) string {
			g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{})
			g.SetNode("a", Attrs(a))
			if err := Layout(g); err != nil {
				return err.Error()
			}
			return fmt.Sprint(g.Graph().(Attrs)["width"])
		})
		if len(want) < 2 {
			t.Fatalf("label %v: the original showed only %v", label, want)
		}
		if fmt.Sprint(want) != fmt.Sprint(got) {
			t.Errorf("label %v: outcomes differ: original %v, new %v", label, want, got)
		}
	}
}

package dagro

import (
	"math/rand"
	"reflect"
	"testing"
	"unicode/utf16"
)

// These helpers freeze v0.2.0's adjacency enumeration, including the optional
// endpoint's empty-string behavior and self-edges appearing in both halves.
func originalEdgeValues(m *edgeMap) []Edge {
	if m == nil {
		return nil
	}
	keys := jsObjectKeyOrder(append([]string(nil), m.order...))
	out := make([]Edge, 0, len(keys))
	for _, id := range keys {
		out = append(out, m.items[id])
	}
	return out
}

func TestJSStringGreaterMatchesUTF16(t *testing.T) {
	strings := []string{"", "0", "10", "a", "a\x00", "a\x7f", "a\u0080", "a\ufffd", "a\ue000", "a\U00010000", "a\U0010ffff", "\xff", "a\xc0\x80"}
	rng := rand.New(rand.NewSource(1039))
	for i := 0; i < 300; i++ {
		b := make([]byte, rng.Intn(20))
		for j := range b {
			b[j] = byte(rng.Intn(256))
		}
		strings = append(strings, string(b))
	}
	original := func(a, b string) bool {
		a16, b16 := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
		for i := 0; i < len(a16) && i < len(b16); i++ {
			if a16[i] != b16[i] {
				return a16[i] > b16[i]
			}
		}
		return len(a16) > len(b16)
	}
	for _, a := range strings {
		for _, b := range strings {
			if got, want := jsStringGreater(a, b), original(a, b); got != want {
				t.Fatalf("jsStringGreater(%q, %q) = %v; want %v", a, b, got, want)
			}
		}
	}
}

func originalNodeEdges(g *Graph, v string, w ...string) []Edge {
	edges := func(m *edgeMap, incoming bool) []Edge {
		if m == nil {
			if g.HasNode(v) {
				return []Edge{}
			}
			return nil
		}
		all := originalEdgeValues(m)
		if len(w) == 0 || w[0] == "" {
			return all
		}
		out := all[:0]
		for _, e := range all {
			if incoming && e.V == w[0] || !incoming && e.W == w[0] {
				out = append(out, e)
			}
		}
		return out
	}
	return append(edges(g.in[v], true), edges(g.out[v], false)...)
}

func TestAdjacencyMatchesOriginalAfterMutations(t *testing.T) {
	ids := []string{"", "10", "2", "0", "01", "4294967294", "4294967295", "a", "z", "\U00010000", "\ue000", "a\x01b", "b\x01c"}
	for _, undirected := range []bool{false, true} {
		for _, multigraph := range []bool{false, true} {
			g := NewGraph(GraphOptions{Undirected: undirected, Multigraph: multigraph})
			rng := rand.New(rand.NewSource(7231))
			for step := 0; step < 150; step++ {
				v, w := ids[rng.Intn(len(ids))], ids[rng.Intn(len(ids))]
				if step%5 == 0 {
					g.RemoveNode(v)
				} else if step%3 == 0 {
					g.RemoveEdgeByArgs(v, w)
				} else if multigraph && step%2 == 0 {
					g.SetEdge(v, w, nil, ids[rng.Intn(len(ids))])
				} else {
					g.SetEdge(v, w)
				}
				g.SetNode("isolated")
				if got, want := g.Edges(), originalEdgeValues(g.edgeObjs); !reflect.DeepEqual(got, want) {
					t.Fatalf("step %d: edges = %#v; want %#v", step, got, want)
				}
				for _, node := range append(append([]string(nil), ids...), "isolated", "missing") {
					var neighbors []string
					seen := map[string]bool{}
					for _, list := range [][]string{g.Predecessors(node), g.Successors(node)} {
						for _, v := range list {
							if !seen[v] {
								seen[v] = true
								neighbors = append(neighbors, v)
							}
						}
					}
					if got := g.Neighbors(node); !reflect.DeepEqual(got, neighbors) {
						t.Fatalf("step %d: Neighbors(%q) = %#v; want %#v", step, node, got, neighbors)
					}
					for _, filter := range [][]string{nil, {""}, {v}, {w}, {"missing"}} {
						got, want := g.NodeEdges(node, filter...), originalNodeEdges(g, node, filter...)
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("step %d: NodeEdges(%q, %q) = %#v; want %#v", step, node, filter, got, want)
						}
						if len(got) > 0 {
							got[0] = Edge{V: "mutated"}
							if !reflect.DeepEqual(g.NodeEdges(node, filter...), want) {
								t.Fatal("returned edges alias graph storage")
							}
						}
					}
				}
			}
		}
	}
}

func TestTreeEdgeMatchesOriginal(t *testing.T) {
	ids := []string{"z", "10", "2", "", "\ue000", "\U00010000", "a\x01b", "b\x01c", "a", "c"}
	for _, undirected := range []bool{false, true} {
		for _, multigraph := range []bool{false, true} {
			g := NewGraph(GraphOptions{Undirected: undirected, Multigraph: multigraph})
			for i := 1; i < len(ids); i++ {
				g.SetEdge(ids[i/2], ids[i])
				if multigraph {
					g.SetEdge(ids[i/2], ids[i], nil, "named")
				}
			}
			for _, u := range ids {
				for _, v := range ids {
					want := originalNodeEdges(g, u, v)
					var got Edge
					panicked := false
					func() {
						defer func() { panicked = recover() != nil }()
						got = treeEdge(g, u, v)
					}()
					if len(want) == 0 {
						if !panicked {
							t.Fatalf("treeEdge(%q, %q) should panic", u, v)
						}
					} else if panicked || got != want[0] {
						t.Fatalf("treeEdge(%q, %q) = %#v (panic %v); want %#v", u, v, got, panicked, want[0])
					}
				}
			}
		}
	}
}

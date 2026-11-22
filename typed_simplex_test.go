package dagro

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// simplexInput draws larger graphs than typedRandomInput (up to 60
// nodes, a few edges per node) with fractional weights and minlens, so that
// network simplex makes many exchanges per layout and its cut values and
// ranks are not integers.
func simplexInput(rng *rand.Rand) diffInput {
	in := diffInput{Options: GraphOptions{Compound: true, Multigraph: true}, Graph: Attrs{}}
	in.Graph["rankdir"] = pick(rng, "TB", "LR")
	in.Graph["ranksep"] = float64(10 + rng.Intn(60))
	n := 10 + rng.Intn(50)
	ids := make([]string, n)
	for i := range ids {
		if rng.Intn(8) == 0 {
			ids[i] = fmt.Sprintf("n%d", i)
		} else {
			ids[i] = strconv.Itoa(i)
		}
	}
	var clusters []string
	if rng.Intn(3) == 0 {
		for i := 0; i < 1+rng.Intn(6); i++ {
			clusters = append(clusters, fmt.Sprintf("c%d", i))
		}
	}
	for _, c := range clusters {
		in.Nodes = append(in.Nodes, diffNodeInput{ID: c, Attrs: Attrs{}})
	}
	for i, c := range clusters {
		if i > 0 && rng.Intn(2) == 0 {
			in.Nodes[i].Parent = strptr(clusters[rng.Intn(i)])
		}
		_ = c
	}
	for _, id := range ids {
		node := diffNodeInput{ID: id, Attrs: Attrs{
			"width":  float64(1 + rng.Intn(150)),
			"height": float64(1 + rng.Intn(80)),
		}}
		if len(clusters) > 0 && rng.Intn(3) == 0 {
			node.Parent = strptr(clusters[rng.Intn(len(clusters))])
		}
		in.Nodes = append(in.Nodes, node)
	}
	edges := n + rng.Intn(3*n)
	for i := 0; i < edges; i++ {
		v, w := ids[rng.Intn(n)], ids[rng.Intn(n)]
		if v == w {
			continue
		}
		a := Attrs{"width": float64(rng.Intn(40)), "height": float64(rng.Intn(20))}
		if rng.Intn(2) == 0 {
			a["weight"] = pick[any](rng, 1.0, 2.0, 0.5, 1.5, 3.0, 0.1, 7.25, 1e-3)
		}
		if rng.Intn(3) == 0 {
			a["minlen"] = pick[any](rng, 1.0, 2.0, 3.0)
		}
		in.Edges = append(in.Edges, diffEdgeInput{V: v, W: w, Name: strptr(fmt.Sprintf("e%d", i)), Attrs: a})
	}
	return in
}

// TestNetworkSimplexMatchesAttrs checks the typed network simplex
// (whose exchanges redo only the subtree that changed) against the Attrs
// engine (which redoes the whole tree) on many-exchange inputs, half of them
// with the tree's edge list compacted after every exchange.
func TestNetworkSimplexMatchesAttrs(t *testing.T) {
	slowUnderRace(t)
	defer func() { rtreeCompactAlways = false }()
	cases := 200
	if s := os.Getenv("DAGRO_SIMPLEX_DIFF_CASES"); s != "" {
		cases, _ = strconv.Atoi(s)
	}
	seed := int64(31120261)
	if s := os.Getenv("DAGRO_SIMPLEX_DIFF_SEED"); s != "" {
		seed, _ = strconv.ParseInt(s, 10, 64)
	}
	rng := rand.New(rand.NewSource(seed))
	failures := 0
	for i := 0; i < cases; i++ {
		in := simplexInput(rng)
		rtreeCompactAlways = i%2 == 1
		if ok, _ := typedDiffCheck(t, fmt.Sprintf("simplex seed %d case %d", seed, i), in); !ok {
			failures++
			if failures >= 3 {
				t.Fatalf("stopping after %d failures", failures)
			}
		}
	}
}

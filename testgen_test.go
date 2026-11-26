package dagro

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

// inputGen generates random LayoutInputs. The features switch on unusual
// values; the generator draws from rng in a fixed order, so a seed and a
// feature list always give the same inputs (the golden results depend on it).
type inputGen struct {
	rng *rand.Rand

	fracMinlen  bool // fractional minlen
	fracWeight  bool // fractional weight
	minlen0     bool // minlen 0, -1 or NaN
	oddNames    bool // node ids the engine does not accept ("", "\x01")
	weirdNums   bool // -0, negative, NaN, infinite, 0.1 and 1e300 sizes and separations
	weirdLabels bool // duplicate ids, self-parents, out-of-range indices
	accepted    bool // leave out edges the engine does not take (repeated keys, ends on clusters)
}

func newInputGen(seed int64, features string) *inputGen {
	z := &inputGen{rng: rand.New(rand.NewSource(seed))}
	for _, f := range strings.Split(features, ",") {
		switch f {
		case "":
		case "fracminlen":
			z.fracMinlen = true
		case "fracweight":
			z.fracWeight = true
		case "minlen0":
			z.minlen0 = true
		case "oddnames":
			z.oddNames = true
		case "weirdnums":
			z.weirdNums = true
		case "weirdlabels":
			z.weirdLabels = true
		case "accepted":
			z.accepted = true
		default:
			panic("unknown feature " + f)
		}
	}
	return z
}

func (z *inputGen) pickS(xs ...string) string { return xs[z.rng.Intn(len(xs))] }
func (z *inputGen) pickA(xs ...any) any       { return xs[z.rng.Intn(len(xs))] }
func (z *inputGen) p(n int) bool              { return z.rng.Intn(n) == 0 }

// names returns n distinct node ids (fewer when a mode runs out), in one of
// several styles: integers (JavaScript array indices sort first), the
// engine's dummy-name prefixes, non-ASCII text.
func (z *inputGen) names(n int) []string {
	mode := z.rng.Intn(8)
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for i := 0; len(out) < n && i < 4*n; i++ {
		var s string
		switch mode {
		case 0, 1:
			s = strconv.Itoa(i)
		case 2:
			s = strconv.Itoa(z.rng.Intn(3 * n))
		case 3:
			s = string(rune('a'+i%26)) + strconv.Itoa(i/26)
		case 4:
			s = z.pickS("_d", "_d1", "_d7", "_d12", "_root", "_root1", "_root2", "_bt", "_bt1", "_bb", "_bl", "_br",
				"_se", "_ep", "_ep3", "_root3", "01", "4294967295", "4294967294", "-1", "1.5", "z", strconv.Itoa(i),
				"é", "Z", "rev1", "_", "__d1")
		case 5:
			s = z.pickS("｡", "\U0001F600", "a", "é", "日本", "A", "b", "0", "10", "9", "퟿", "\U00010000", "x\U0001F600", "x｡")
			if seen[s] {
				s = s + strconv.Itoa(i)
			}
		case 6:
			if z.p(2) {
				s = strconv.Itoa(z.rng.Intn(1000))
			} else {
				s = fmt.Sprintf("n%d", z.rng.Intn(1000))
			}
		default:
			s = strconv.Itoa(i)
			if z.p(3) {
				s = "_" + s
			}
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if mode == 1 {
		z.rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	if z.oddNames && len(out) > 1 {
		out[z.rng.Intn(len(out))] = z.pickS("", "a\x01b", "\x01", "x\x01")
	}
	return out
}

func (z *inputGen) num(weird bool, def float64, normal func() float64) float64 {
	if weird && z.weirdNums && z.p(8) {
		return z.pickA(math.Copysign(0, -1), -3.0, math.NaN(), math.Inf(1), math.Inf(-1), 0.1, 1e300).(float64)
	}
	if z.p(6) {
		return def
	}
	return normal()
}

// input returns a random input with up to maxNodes nodes: up to three
// clusters, every option of the graph, and edges between leaves (some to a
// cluster, some loops) with random names and attributes.
func (z *inputGen) input(maxNodes int) LayoutInput {
	in := DefaultLayoutInput()
	in.RankDir = z.pickS("TB", "BT", "LR", "RL", "TB", "tb", "lr", "", "x", "Rl")
	sep := func() float64 { return float64(z.rng.Intn(120)) + z.pickA(0.0, 0.0, 0.5, 0.25).(float64) }
	in.RankSep = z.num(true, 50, sep)
	in.EdgeSep = z.num(true, 20, sep)
	in.NodeSep = z.num(true, 50, sep)
	if z.p(4) {
		in.MarginX = z.num(true, 0, sep)
		in.MarginY = z.num(true, 0, sep)
	}
	if z.p(6) {
		in.Acyclicer = z.pickS("greedy", "x")
	}
	if z.p(5) {
		in.Ranker = z.pickS("network-simplex", "tight-tree", "longest-path", "other")
	}
	if z.p(5) {
		in.Align = z.pickS("UL", "ur", "dl", "DR", "x")
	}
	n := 1 + z.rng.Intn(maxNodes)
	names := z.names(n)
	if z.p(3) {
		names = names[:0]
		for i := 0; i < n; i++ {
			names = append(names, strconv.Itoa(i))
		}
	}
	if z.weirdLabels && z.p(40) && len(names) > 1 {
		names[1] = names[0] // duplicate id
	}
	if z.weirdLabels && z.p(60) {
		names[0] = z.pickS("", "a\x01b")
	}
	size := func() float64 { return float64(z.rng.Intn(150)) + z.pickA(0.0, 0.0, 0.0, 0.5).(float64) }
	clusters := z.rng.Intn(4)
	if clusters >= len(names) {
		clusters = 0
	}
	for i, id := range names {
		node := LayoutNode{ID: id, Parent: -1, Width: z.num(true, 0, size), Height: z.num(true, 0, size)}
		if i >= clusters && clusters > 0 && z.rng.Intn(3) != 0 {
			node.Parent = z.rng.Intn(clusters)
		} else if i > 0 && i < clusters && z.p(2) {
			node.Parent = z.rng.Intn(i)
		}
		if z.weirdLabels && z.p(80) {
			node.Parent = z.pickA(i, len(names)+3, -2).(int) // self-parent, out of range, other negative
		}
		in.Nodes = append(in.Nodes, node)
	}
	leaves := []int{}
	for i := clusters; i < len(names); i++ {
		leaves = append(leaves, i)
	}
	if len(leaves) == 0 {
		return in
	}
	type edgeKey struct {
		v, w    int
		name    string
		hasName bool
	}
	keys := map[edgeKey]bool{}
	m := z.rng.Intn(3*len(leaves) + 1)
	for k := 0; k < m; k++ {
		v, w := leaves[z.rng.Intn(len(leaves))], leaves[z.rng.Intn(len(leaves))]
		if z.p(25) && clusters > 0 {
			w = z.rng.Intn(clusters)
		}
		if z.p(10) {
			w = v
		}
		e := NewLayoutEdge(v, w)
		if z.p(2) {
			e.Name, e.HasName = z.pickS(fmt.Sprintf("(%d -> %d)[%d]", v, w, k), "", "rev1", "\x00", "e"), true
		}
		if z.p(4) {
			e.Width, e.Height = z.num(true, 0, size), z.num(true, 0, size)
			e.LabelPos = z.pickS("c", "l", "r", "C", "", "x")
		}
		if z.p(6) {
			if z.fracMinlen && z.p(2) {
				e.Minlen = z.pickA(0.5, 1.5, 0.25, 2.5).(float64)
			} else if z.minlen0 && z.p(2) {
				e.Minlen = z.pickA(0.0, -1.0, math.NaN()).(float64)
			} else {
				e.Minlen = float64(1 + z.rng.Intn(3))
			}
		}
		if z.p(6) {
			if z.fracWeight && z.p(2) {
				e.Weight = z.pickA(0.5, 1.5, 0.25).(float64)
			} else {
				e.Weight = float64(1 + z.rng.Intn(4))
			}
		}
		if z.p(8) {
			e.LabelOffset = z.pickA(0.0, 5.0, 2.5, -3.0).(float64)
		}
		if z.weirdLabels && z.p(80) {
			e.V = len(names) + 2
		}
		if z.accepted {
			key := edgeKey{e.V, e.W, e.Name, e.HasName}
			if keys[key] || e.W < clusters {
				continue
			}
			keys[key] = true
		}
		in.Edges = append(in.Edges, e)
	}
	return in
}

// d2ProfileInput returns a random input shaped like D2's: a compound
// multigraph with up to three clusters first, 2 to 11 leaves with integer
// ids in random order, integer sizes and separations, and named edges between
// leaves with centered labels.
func d2ProfileInput(rng *rand.Rand, index int) LayoutInput {
	directions := []string{"TB", "BT", "LR", "RL"}
	in := LayoutInput{RankDir: directions[rng.Intn(len(directions))]}
	in.NodeSep = float64(10 + rng.Intn(101))
	in.EdgeSep = float64(5 + rng.Intn(51))
	in.RankSep = float64(20 + rng.Intn(181))
	nodeCount := 2 + rng.Intn(10)
	clusterCount := rng.Intn(4)
	for cluster := 0; cluster < clusterCount; cluster++ {
		in.Nodes = append(in.Nodes, LayoutNode{
			ID: fmt.Sprintf("cluster%d", cluster), Parent: -1,
			Width: float64(40 + rng.Intn(120)), Height: float64(40 + rng.Intn(100)),
		})
	}
	ids := make([]string, nodeCount)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	index0 := len(in.Nodes)
	for i, id := range ids {
		node := LayoutNode{ID: id, Parent: -1, Width: float64(10 + rng.Intn(121)), Height: float64(10 + rng.Intn(101))}
		if clusterCount > 0 && (i+index)%3 != 0 {
			node.Parent = rng.Intn(clusterCount)
		}
		in.Nodes = append(in.Nodes, node)
	}
	position := map[string]int{}
	for i, id := range ids {
		position[id] = index0 + i
	}
	edgeCount := 1 + rng.Intn(nodeCount*3)
	for i := 0; i < edgeCount; i++ {
		v, w := ids[rng.Intn(nodeCount)], ids[rng.Intn(nodeCount)]
		e := NewLayoutEdge(position[v], position[w])
		e.Name, e.HasName = fmt.Sprintf("(%s -> %s)[%d]", v, w, i), true
		e.Width, e.Height, e.LabelPos = float64(rng.Intn(81)), float64(rng.Intn(41)), "c"
		in.Edges = append(in.Edges, e)
	}
	return in
}

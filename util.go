package dagro

import (
	"fmt"
	"math"
	"time"
)

// Time evaluates fn and logs its elapsed wall time, matching Dagre's public
// util.time helper. The deferred log preserves the upstream finally behavior.
func Time(name string, fn any) (result any) {
	start := time.Now()
	defer func() {
		fmt.Printf("%s time: %dms\n", name, time.Since(start).Milliseconds())
	}()
	return callCallable(fn)
}

// NoTime evaluates fn without timing or logging, matching Dagre's util.notime.
func NoTime(_ string, fn any) any { return callCallable(fn) }

func addDummyNode(g *Graph, typ string, attrs Attrs, name string) string {
	var v string
	for {
		v = g.uniqueID(name)
		if !g.HasNode(v) {
			break
		}
	}
	attrs["dummy"] = typ
	g.SetNode(v, attrs)
	return v
}

func simplify(g *Graph) *Graph {
	simplified := NewGraph().SetGraph(g.Graph())
	for _, v := range g.Nodes() {
		simplified.SetNode(v, g.Node(v))
	}
	for _, e := range g.Edges() {
		simpleLabel := Attrs{"weight": float64(0), "minlen": float64(1)}
		if existing := simplified.EdgeByArgs(e.V, e.W); existing != nil {
			simpleLabel = asAttrs(existing)
		}
		label := asAttrs(g.Edge(e))
		simplified.SetEdge(e.V, e.W, Attrs{
			"weight": num(simpleLabel, "weight") + num(label, "weight"),
			"minlen": mathMaxJS(num(simpleLabel, "minlen"), num(label, "minlen")),
		})
	}
	return simplified
}

func asNonCompoundGraph(g *Graph) *Graph {
	simplified := NewGraph(GraphOptions{Multigraph: g.IsMultigraph()}).SetGraph(g.Graph())
	for _, v := range g.Nodes() {
		if len(g.Children(v)) == 0 {
			simplified.SetNode(v, g.Node(v))
		}
	}
	for _, e := range g.Edges() {
		simplified.SetEdgeObject(e, g.Edge(e))
	}
	return simplified
}

func successorWeights(g *Graph) map[string]map[string]float64 {
	out := make(map[string]map[string]float64, g.NodeCount())
	for _, v := range g.Nodes() {
		weights := map[string]float64{}
		for _, e := range g.OutEdges(v) {
			weights[e.W] += num(asAttrs(g.Edge(e)), "weight")
		}
		out[v] = weights
	}
	return out
}

func predecessorWeights(g *Graph) map[string]map[string]float64 {
	out := make(map[string]map[string]float64, g.NodeCount())
	for _, v := range g.Nodes() {
		weights := map[string]float64{}
		for _, e := range g.InEdges(v) {
			weights[e.V] += num(asAttrs(g.Edge(e)), "weight")
		}
		out[v] = weights
	}
	return out
}

func intersectRect(rect Attrs, point Point) (Point, error) {
	x, y := num(rect, "x"), num(rect, "y")
	dx, dy := point.X-x, point.Y-y
	w, h := num(rect, "width")/2, num(rect, "height")/2
	if !jsTruthyNumber(dx) && !jsTruthyNumber(dy) {
		return Point{}, fmt.Errorf("not possible to find intersection inside of the rectangle")
	}
	var sx, sy float64
	if math.Abs(dy)*w > math.Abs(dx)*h {
		if dy < 0 {
			h = -h
		}
		sx, sy = h*dx/dy, h
	} else {
		if dx < 0 {
			w = -w
		}
		sx, sy = w, w*dy/dx
	}
	return Point{X: x + sx, Y: y + sy}, nil
}

func buildLayerMatrix(g *Graph) [][]string {
	max := maxRank(g)
	if max < 0 || math.IsInf(max, -1) {
		return nil
	}
	layers := make([][]string, int(max)+1)
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if !has(node, "rank") || !has(node, "order") {
			continue
		}
		rank, order := integer(node, "rank"), integer(node, "order")
		for len(layers[rank]) <= order {
			layers[rank] = append(layers[rank], "")
		}
		layers[rank][order] = v
	}
	return layers
}

func normalizeRanks(g *Graph) {
	min := math.Inf(1)
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if has(node, "rank") {
			min = lodashMinJS(min, num(node, "rank"))
		}
	}
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if has(node, "rank") {
			node["rank"] = num(node, "rank") - min
		}
	}
}

func removeEmptyRanks(g *Graph) {
	offset := math.Inf(1)
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if has(node, "rank") {
			offset = lodashMinJS(offset, num(node, "rank"))
		}
	}
	var layers [][]string
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if !has(node, "rank") {
			continue
		}
		rankValue := num(node, "rank") - offset
		// A non-finite, fractional, negative, or 2^32-1-and-larger JavaScript
		// array key is an ordinary object property, not an indexed layer, so
		// Lodash's array iteration never visits it.
		if math.IsNaN(rankValue) || math.IsInf(rankValue, 0) ||
			rankValue < 0 || rankValue != math.Trunc(rankValue) || rankValue >= 4294967295 {
			continue
		}
		rank := int(rankValue)
		for len(layers) <= rank {
			layers = append(layers, nil)
		}
		layers[rank] = append(layers[rank], v)
	}
	delta := 0.0
	factor := num(asAttrs(g.Graph()), "nodeRankFactor")
	for i, vs := range layers {
		if vs == nil && math.Mod(float64(i), factor) != 0 {
			delta--
		} else if delta != 0 {
			for _, v := range vs {
				node := asAttrs(g.Node(v))
				node["rank"] = num(node, "rank") + delta
			}
		}
	}
}

func addBorderNode(g *Graph, prefix string, values ...float64) string {
	node := Attrs{"width": float64(0), "height": float64(0)}
	if len(values) >= 2 {
		node["rank"], node["order"] = values[0], values[1]
	}
	return addDummyNode(g, "border", node, prefix)
}

func maxRank(g *Graph) float64 {
	max := math.Inf(-1)
	for _, v := range g.Nodes() {
		node := asAttrs(g.Node(v))
		if has(node, "rank") {
			max = lodashMaxJS(max, num(node, "rank"))
		}
	}
	return max
}

func setEdgePreservingName(g *Graph, v, w string, label any, e Edge) {
	if e.HasName {
		g.SetEdge(v, w, label, e.Name)
	} else {
		g.SetEdge(v, w, label)
	}
}

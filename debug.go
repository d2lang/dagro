package dagro

import "fmt"

// DebugOrdering returns the ordering graph produced by Dagre's debug helper.
func DebugOrdering(g *Graph) *Graph {
	layers := buildLayerMatrix(g)
	h := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{})
	for _, v := range g.Nodes() {
		h.SetNode(v, Attrs{"label": v})
		node := asAttrs(g.Node(v))
		rankText := "undefined"
		if rank, ok := node["rank"]; ok {
			rankText = jsConcatString(rank)
		}
		_ = h.SetParent(v, "layer"+rankText)
	}
	for _, e := range g.Edges() {
		setEdgePreservingName(h, e.V, e.W, Attrs{}, e)
	}
	for i, layer := range layers {
		layerV := fmt.Sprintf("layer%d", i)
		h.SetNode(layerV, Attrs{"rank": "same"})
		for j := 1; j < len(layer); j++ {
			h.SetEdge(layer[j-1], layer[j], Attrs{"style": "invis"})
		}
	}
	return h
}

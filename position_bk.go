package dagro

import (
	"math"
	"sort"
	"strings"
)

// The horizontal coordinate assignment below is a direct port of Dagre
// 0.8.5's Brandes-Kopf implementation in lib/position/bk.js.

type positionConflicts map[string]map[string]bool

type positionAlignment struct {
	root  map[string]string
	align map[string]string
}

type positionAlignments map[string]map[string]float64

// findType1Conflicts finds crossings between non-inner and inner segments.
func findType1Conflicts(g *Graph, layering [][]string) positionConflicts {
	conflicts := positionConflicts{}

	// Dagre uses reduce without an initial value, so the first layer is the
	// accumulator and scanning begins with the second layer.
	for layerIndex := 1; layerIndex < len(layering); layerIndex++ {
		prevLayer := layering[layerIndex-1]
		layer := layering[layerIndex]
		k0 := 0
		scanPos := 0
		prevLayerLength := len(prevLayer)
		lastNode := ""
		if len(layer) != 0 {
			lastNode = layer[len(layer)-1]
		}

		for i, v := range layer {
			w, found := findOtherInnerSegmentNode(g, v)
			// JavaScript tests the returned node ID for truthiness.
			found = found && w != ""
			k1 := prevLayerLength
			if found {
				k1 = integer(asAttrs(g.Node(w)), "order")
			}

			if found || v == lastNode {
				for _, scanNode := range layer[scanPos : i+1] {
					for _, u := range g.Predecessors(scanNode) {
						uLabel := asAttrs(g.Node(u))
						uPos := integer(uLabel, "order")
						if (uPos < k0 || k1 < uPos) &&
							!(positionTruthy(uLabel["dummy"]) && positionTruthy(asAttrs(g.Node(scanNode))["dummy"])) {
							addConflict(conflicts, u, scanNode)
						}
					}
				}
				scanPos = i + 1
				k0 = k1
			}
		}
	}

	return conflicts
}

// findType2Conflicts finds crossings between inner segments, favoring border
// segments. The scan placement intentionally mirrors Dagre 0.8.5, including
// its repeated tail scan inside the south-layer loop.
func findType2Conflicts(g *Graph, layering [][]string) positionConflicts {
	conflicts := positionConflicts{}

	scan := func(south []string, southPos, southEnd int, prevNorthBorder, nextNorthBorder float64) {
		for i := southPos; i < southEnd; i++ {
			v := south[i]
			if !positionTruthy(asAttrs(g.Node(v))["dummy"]) {
				continue
			}
			for _, u := range g.Predecessors(v) {
				uNode := asAttrs(g.Node(u))
				uOrder := num(uNode, "order")
				if positionTruthy(uNode["dummy"]) &&
					(uOrder < prevNorthBorder || uOrder > nextNorthBorder) {
					addConflict(conflicts, u, v)
				}
			}
		}
	}

	for layerIndex := 1; layerIndex < len(layering); layerIndex++ {
		north := layering[layerIndex-1]
		south := layering[layerIndex]
		prevNorthPos := float64(-1)
		// Undefined in the original until a border predecessor is found. NaN
		// has the same false-for-both-comparisons behavior here.
		nextNorthPos := math.NaN()
		southPos := 0

		for southLookahead, v := range south {
			if stringValue(asAttrs(g.Node(v)), "dummy") == "border" {
				predecessors := g.Predecessors(v)
				if len(predecessors) != 0 {
					nextNorthPos = num(asAttrs(g.Node(predecessors[0])), "order")
					scan(south, southPos, southLookahead, prevNorthPos, nextNorthPos)
					southPos = southLookahead
					prevNorthPos = nextNorthPos
				}
			}
			scan(south, southPos, len(south), nextNorthPos, float64(len(north)))
		}
	}

	return conflicts
}

func findOtherInnerSegmentNode(g *Graph, v string) (string, bool) {
	if !positionTruthy(asAttrs(g.Node(v))["dummy"]) {
		return "", false
	}
	for _, u := range g.Predecessors(v) {
		if positionTruthy(asAttrs(g.Node(u))["dummy"]) {
			return u, true
		}
	}
	return "", false
}

func addConflict(conflicts positionConflicts, v, w string) {
	if v > w {
		v, w = w, v
	}
	if conflicts[v] == nil {
		conflicts[v] = map[string]bool{}
	}
	conflicts[v][w] = true
}

func hasConflict(conflicts positionConflicts, v, w string) bool {
	if v > w {
		v, w = w, v
	}
	return conflicts[v] != nil && conflicts[v][w]
}

// verticalAlignment aligns nodes with median neighbors where doing so does not
// introduce a conflict or split an already-created block.
func verticalAlignment(
	g *Graph,
	layering [][]string,
	conflicts positionConflicts,
	neighborFn func(string) []string,
) positionAlignment {
	root := map[string]string{}
	align := map[string]string{}
	pos := map[string]int{}

	for _, layer := range layering {
		for order, v := range layer {
			root[v] = v
			align[v] = v
			pos[v] = order
		}
	}

	for _, layer := range layering {
		prevIdx := -1
		for _, v := range layer {
			ws := append([]string(nil), neighborFn(v)...)
			if len(ws) == 0 {
				continue
			}
			// Lodash sortBy is stable. Preserve neighbor order for equal positions.
			sort.SliceStable(ws, func(i, j int) bool {
				return pos[ws[i]] < pos[ws[j]]
			})
			mp := float64(len(ws)-1) / 2
			for i, last := int(math.Floor(mp)), int(math.Ceil(mp)); i <= last; i++ {
				w := ws[i]
				if align[v] == v && prevIdx < pos[w] && !hasConflict(conflicts, v, w) {
					align[w] = v
					root[v] = root[w]
					align[v] = root[v]
					prevIdx = pos[w]
				}
			}
		}
	}

	return positionAlignment{root: root, align: align}
}

func horizontalCompaction(
	g *Graph,
	layering [][]string,
	root, align map[string]string,
	reverseSep ...bool,
) map[string]float64 {
	reverse := len(reverseSep) != 0 && reverseSep[0]
	xs := map[string]float64{}
	blockG := buildBlockGraph(g, layering, root, reverse)
	borderType := "borderRight"
	if reverse {
		borderType = "borderLeft"
	}

	iterate := func(setXs func(string), nextNodes func(string) []string) {
		stack := blockG.Nodes()
		visited := map[string]bool{}
		for len(stack) != 0 {
			elem := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if visited[elem] {
				setXs(elem)
			} else {
				visited[elem] = true
				stack = append(stack, elem)
				stack = append(stack, nextNodes(elem)...)
			}
		}
	}

	iterate(func(elem string) {
		x := float64(0)
		for _, e := range blockG.InEdges(elem) {
			x = mathMaxJS(x, xs[e.V]+number(blockG.Edge(e)))
		}
		xs[elem] = x
	}, blockG.Predecessors)

	iterate(func(elem string) {
		minimum := math.Inf(1)
		for _, e := range blockG.OutEdges(elem) {
			minimum = mathMinJS(minimum, xs[e.W]-number(blockG.Edge(e)))
		}
		node := asAttrs(g.Node(elem))
		if !math.IsInf(minimum, 1) && stringValue(node, "borderType") != borderType {
			xs[elem] = mathMaxJS(xs[elem], minimum)
		}
	}, blockG.Successors)

	for _, v := range align {
		xs[v] = xs[root[v]]
	}

	return xs
}

func buildBlockGraph(g *Graph, layering [][]string, root map[string]string, reverseSep bool) *Graph {
	blockGraph := NewGraph()
	graphLabel := asAttrs(g.Graph())
	nodeSep := num(graphLabel, "nodesep")
	edgeSep := num(graphLabel, "edgesep")

	for _, layer := range layering {
		u := ""
		for _, v := range layer {
			vRoot := root[v]
			blockGraph.SetNode(vRoot)
			// This is intentionally a string truthiness check, matching `if (u)`.
			if u != "" {
				uRoot := root[u]
				previous := float64(0)
				if value := blockGraph.EdgeByArgs(uRoot, vRoot); value != nil {
					previous = number(value)
					if !jsTruthyNumber(previous) {
						previous = 0
					}
				}
				blockGraph.SetEdge(uRoot, vRoot, mathMaxJS(positionSep(g, v, u, nodeSep, edgeSep, reverseSep), previous))
			}
			u = v
		}
	}

	return blockGraph
}

func findSmallestWidthAlignment(g *Graph, xss positionAlignments) map[string]float64 {
	var smallest map[string]float64
	smallestWidth := math.Inf(1)
	for _, alignment := range []string{"ul", "ur", "dl", "dr"} {
		xs := xss[alignment]
		maximum := math.Inf(-1)
		minimum := math.Inf(1)
		for v, x := range xs {
			halfWidth := num(asAttrs(g.Node(v)), "width") / 2
			maximum = mathMaxJS(maximum, x+halfWidth)
			minimum = mathMinJS(minimum, x-halfWidth)
		}
		width := maximum - minimum
		// minBy retains the first value on ties.
		if smallest == nil || width < smallestWidth {
			smallest = xs
			smallestWidth = width
		}
	}
	return smallest
}

func alignCoordinates(xss positionAlignments, alignTo map[string]float64) {
	alignToMin, alignToMax := coordinateExtremes(alignTo)

	for _, vert := range []string{"u", "d"} {
		for _, horiz := range []string{"l", "r"} {
			alignment := vert + horiz
			xs := xss[alignment]
			xsMin, xsMax := coordinateExtremes(xs)
			delta := alignToMax - xsMax
			if horiz == "l" {
				delta = alignToMin - xsMin
			}
			// JavaScript treats both zero and NaN as false here.
			if delta == 0 || math.IsNaN(delta) {
				continue
			}
			shifted := make(map[string]float64, len(xs))
			for v, x := range xs {
				shifted[v] = x + delta
			}
			xss[alignment] = shifted
		}
	}
}

func balance(xss positionAlignments, align string) map[string]float64 {
	out := make(map[string]float64, len(xss["ul"]))
	for v := range xss["ul"] {
		if align != "" {
			out[v] = xss[strings.ToLower(align)][v]
			continue
		}
		values := []float64{xss["ul"][v], xss["ur"][v], xss["dl"][v], xss["dr"][v]}
		sort.Float64s(values)
		out[v] = (values[1] + values[2]) / 2
	}
	return out
}

func positionX(g *Graph) map[string]float64 {
	layering := buildLayerMatrix(g)
	conflicts := findType1Conflicts(g, layering)
	for v, ws := range findType2Conflicts(g, layering) {
		if conflicts[v] == nil {
			conflicts[v] = map[string]bool{}
		}
		for w := range ws {
			conflicts[v][w] = true
		}
	}

	xss := positionAlignments{}
	for _, vert := range []string{"u", "d"} {
		adjustedLayering := clonePositionLayering(layering)
		if vert == "d" {
			reverseStrings2DOuter(adjustedLayering)
		}
		for _, horiz := range []string{"l", "r"} {
			if horiz == "r" {
				for i := range adjustedLayering {
					reverseStrings(adjustedLayering[i])
				}
			}

			neighborFn := g.Predecessors
			if vert == "d" {
				neighborFn = g.Successors
			}
			alignment := verticalAlignment(g, adjustedLayering, conflicts, neighborFn)
			xs := horizontalCompaction(g, adjustedLayering, alignment.root, alignment.align, horiz == "r")
			if horiz == "r" {
				for v, x := range xs {
					xs[v] = -x
				}
			}
			xss[vert+horiz] = xs
		}
	}

	smallestWidth := findSmallestWidthAlignment(g, xss)
	alignCoordinates(xss, smallestWidth)
	return balance(xss, stringValue(asAttrs(g.Graph()), "align"))
}

func positionSep(g *Graph, v, w string, nodeSep, edgeSep float64, reverseSep bool) float64 {
	vLabel := asAttrs(g.Node(v))
	wLabel := asAttrs(g.Node(w))
	sum := float64(0)
	delta := float64(0)

	sum += num(vLabel, "width") / 2
	if has(vLabel, "labelpos") {
		switch strings.ToLower(stringValue(vLabel, "labelpos")) {
		case "l":
			delta = -num(vLabel, "width") / 2
		case "r":
			delta = num(vLabel, "width") / 2
		}
	}
	if jsTruthyNumber(delta) {
		if reverseSep {
			sum += delta
		} else {
			sum -= delta
		}
	}
	delta = 0

	if positionTruthy(vLabel["dummy"]) {
		sum += edgeSep / 2
	} else {
		sum += nodeSep / 2
	}
	if positionTruthy(wLabel["dummy"]) {
		sum += edgeSep / 2
	} else {
		sum += nodeSep / 2
	}

	sum += num(wLabel, "width") / 2
	if has(wLabel, "labelpos") {
		switch strings.ToLower(stringValue(wLabel, "labelpos")) {
		case "l":
			delta = num(wLabel, "width") / 2
		case "r":
			delta = -num(wLabel, "width") / 2
		}
	}
	if jsTruthyNumber(delta) {
		if reverseSep {
			sum += delta
		} else {
			sum -= delta
		}
	}

	return sum
}

func coordinateExtremes(xs map[string]float64) (minimum, maximum float64) {
	minimum, maximum = math.NaN(), math.NaN()
	for _, x := range xs {
		minimum = lodashMinJS(minimum, x)
		maximum = lodashMaxJS(maximum, x)
	}
	return minimum, maximum
}

func clonePositionLayering(layering [][]string) [][]string {
	out := make([][]string, len(layering))
	for i := range layering {
		out[i] = append([]string(nil), layering[i]...)
	}
	return out
}

func reverseStrings(values []string) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseStrings2DOuter(values [][]string) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func positionTruthy(v any) bool {
	switch value := v.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != ""
	case float64:
		return value != 0 && !math.IsNaN(value)
	case float32:
		return value != 0 && !float32IsNaN(value)
	case int:
		return value != 0
	case int8:
		return value != 0
	case int16:
		return value != 0
	case int32:
		return value != 0
	case int64:
		return value != 0
	case uint:
		return value != 0
	case uint8:
		return value != 0
	case uint16:
		return value != 0
	case uint32:
		return value != 0
	case uint64:
		return value != 0
	default:
		// JavaScript objects, including empty arrays and objects, are truthy.
		return true
	}
}

func float32IsNaN(value float32) bool { return value != value }

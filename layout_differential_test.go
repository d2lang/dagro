package dagro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"testing"
)

type diffNodeInput struct {
	ID     string  `json:"id"`
	Attrs  Attrs   `json:"attrs"`
	Parent *string `json:"parent,omitempty"`
}

type diffEdgeInput struct {
	V     string  `json:"v"`
	W     string  `json:"w"`
	Name  *string `json:"name,omitempty"`
	Attrs Attrs   `json:"attrs"`
}

type diffInput struct {
	Options GraphOptions    `json:"-"`
	Graph   Attrs           `json:"graph"`
	Nodes   []diffNodeInput `json:"nodes"`
	Edges   []diffEdgeInput `json:"edges"`
}

func (in diffInput) MarshalJSON() ([]byte, error) {
	type wire struct {
		Options map[string]bool `json:"options"`
		Graph   Attrs           `json:"graph"`
		Nodes   []diffNodeInput `json:"nodes"`
		Edges   []diffEdgeInput `json:"edges"`
	}
	return json.Marshal(wire{
		Options: map[string]bool{
			"directed": !in.Options.Undirected, "multigraph": in.Options.Multigraph, "compound": in.Options.Compound,
		},
		Graph: in.Graph, Nodes: in.Nodes, Edges: in.Edges,
	})
}

func strptr(s string) *string { return &s }

func TestLayoutUpstreamExamples(t *testing.T) {
	t.Run("single node", func(t *testing.T) {
		g := newLayoutTestGraph()
		g.SetNode("a", Attrs{"width": 50.0, "height": 100.0})
		if err := Layout(g); err != nil {
			t.Fatal(err)
		}
		a := asAttrs(g.Node("a"))
		assertNear(t, num(a, "x"), 25)
		assertNear(t, num(a, "y"), 50)
	})

	t.Run("connected nodes and route", func(t *testing.T) {
		g := newLayoutTestGraph()
		asAttrs(g.Graph())["ranksep"] = 200.0
		g.SetNode("a", Attrs{"width": 100.0, "height": 100.0})
		g.SetNode("b", Attrs{"width": 100.0, "height": 100.0})
		g.SetEdge("a", "b", Attrs{})
		if err := Layout(g); err != nil {
			t.Fatal(err)
		}
		points := asAttrs(g.EdgeByArgs("a", "b"))["points"].([]Point)
		if len(points) != 3 {
			t.Fatalf("points len = %d, want 3: %#v", len(points), points)
		}
		want := []Point{{X: 50, Y: 100}, {X: 50, Y: 200}, {X: 50, Y: 300}}
		for i := range want {
			assertNear(t, points[i].X, want[i].X)
			assertNear(t, points[i].Y, want[i].Y)
		}
	})
}

func TestLayoutMatchesDagreJS(t *testing.T) {
	dagreJS := os.Getenv("DAGRO_DAGRE_JS")
	if dagreJS == "" {
		t.Skip("set DAGRO_DAGRE_JS to the Dagre 0.8.5 bundle to run differential tests")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	fixtures := differentialFixtures()
	fixtures = append(fixtures, randomDifferentialFixtures(100, 805)...)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			inputJSON, err := json.Marshal(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "testdata/differential/oracle.js")
			cmd.Env = append(os.Environ(), "DAGRO_DAGRE_JS="+dagreJS)
			cmd.Stdin = bytes.NewReader(inputJSON)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			jsJSON, err := cmd.Output()
			if err != nil {
				t.Fatalf("JS oracle: %v: %s", err, stderr.String())
			}
			goJSON := runDifferentialGo(t, fixture.input)
			var want, got any
			if err := json.Unmarshal(jsJSON, &want); err != nil {
				t.Fatalf("decode JS: %v\n%s", err, jsJSON)
			}
			if err := json.Unmarshal(goJSON, &got); err != nil {
				t.Fatalf("decode Go: %v\n%s", err, goJSON)
			}
			compareJSON(t, "$", want, got)
		})
	}
}

type namedFixture struct {
	name  string
	input diffInput
}

func differentialFixtures() []namedFixture {
	base := func() diffInput {
		return diffInput{Options: GraphOptions{Compound: true, Multigraph: true}, Graph: Attrs{}}
	}
	var out []namedFixture
	in := base()
	in.Nodes = []diffNodeInput{{ID: "a", Attrs: Attrs{"width": 50.0, "height": 100.0}}}
	out = append(out, namedFixture{"single", in})

	in = base()
	in.Graph = Attrs{"ranksep": 200.0}
	in.Nodes = []diffNodeInput{{"a", Attrs{"width": 100.0, "height": 100.0}, nil}, {"b", Attrs{"width": 100.0, "height": 100.0}, nil}}
	in.Edges = []diffEdgeInput{{"a", "b", nil, Attrs{}}}
	out = append(out, namedFixture{"chain", in})

	in = base()
	in.Graph = Attrs{"rankdir": "LR", "nodesep": 31.0, "edgesep": 17.0, "ranksep": 77.0}
	in.Nodes = []diffNodeInput{{"0", Attrs{"width": 80.0, "height": 40.0}, nil}, {"1", Attrs{"width": 60.0, "height": 50.0}, nil}}
	in.Edges = []diffEdgeInput{
		{"0", "1", strptr("edge-a"), Attrs{"width": 30.0, "height": 10.0, "labelpos": "c"}},
		{"0", "1", strptr("edge-b"), Attrs{"width": 20.0, "height": 15.0, "labelpos": "c"}},
	}
	out = append(out, namedFixture{"d2-multiedge", in})

	in = base()
	in.Graph = Attrs{"rankdir": "BT", "acyclicer": "greedy"}
	in.Nodes = []diffNodeInput{{"a", Attrs{"width": 100.0, "height": 60.0}, nil}, {"b", Attrs{"width": 70.0, "height": 70.0}, nil}, {"c", Attrs{"width": 40.0, "height": 80.0}, nil}}
	in.Edges = []diffEdgeInput{{"a", "b", strptr("ab"), Attrs{"weight": 2.0}}, {"b", "c", strptr("bc"), Attrs{}}, {"c", "a", strptr("ca"), Attrs{}}}
	out = append(out, namedFixture{"cycle-greedy", in})

	in = base()
	in.Graph = Attrs{"rankdir": "RL", "edgesep": 75.0}
	in.Nodes = []diffNodeInput{{"a", Attrs{"width": 100.0, "height": 100.0}, nil}}
	in.Edges = []diffEdgeInput{{"a", "a", strptr("self"), Attrs{"width": 50.0, "height": 50.0}}}
	out = append(out, namedFixture{"self-loop", in})

	in = base()
	in.Nodes = []diffNodeInput{
		{"cluster", Attrs{}, nil},
		{"a", Attrs{"width": 50.0, "height": 50.0}, strptr("cluster")},
		{"b", Attrs{"width": 80.0, "height": 30.0}, strptr("cluster")},
		{"outside", Attrs{"width": 40.0, "height": 40.0}, nil},
	}
	in.Edges = []diffEdgeInput{{"a", "b", strptr("inside"), Attrs{}}, {"b", "outside", strptr("out"), Attrs{"width": 30.0, "height": 12.0, "labelpos": "l"}}}
	out = append(out, namedFixture{"compound", in})

	in = base()
	in.Graph = Attrs{"nodeSep": 23.0, "marginX": 7.0, "marginY": 9.0}
	in.Nodes = []diffNodeInput{
		{"10", Attrs{"width": 12.0, "height": 12.0}, nil}, {"2", Attrs{"width": 14.0, "height": 14.0}, nil},
		{"00", Attrs{"width": 16.0, "height": 16.0}, nil}, {"4294967294", Attrs{"width": 18.0, "height": 18.0}, nil},
		{"4294967295", Attrs{"width": 20.0, "height": 20.0}, nil},
	}
	out = append(out, namedFixture{"js-key-order-and-case", in})

	in = base()
	in.Graph = Attrs{"nodesep": "0b110010", "edgesep": "0o24", "ranksep": "5e1", "marginx": "0xA"}
	in.Nodes = []diffNodeInput{
		{"a", Attrs{"width": "0x32", "height": "1e2"}, nil},
		{"b", Attrs{"width": "0b110010", "height": "0o144"}, nil},
	}
	in.Edges = []diffEdgeInput{{"a", "b", nil, Attrs{"minlen": "0b1", "weight": true}}}
	out = append(out, namedFixture{"number-coercion", in})
	return out
}

func randomDifferentialFixtures(count int, seed int64) []namedFixture {
	rng := rand.New(rand.NewSource(seed))
	directions := []string{"TB", "BT", "LR", "RL"}
	rankers := []string{"network-simplex", "tight-tree", "longest-path"}
	alignments := []string{"", "UL", "UR", "DL", "DR"}
	labelPositions := []string{"c", "l", "r"}
	fixtures := make([]namedFixture, 0, count)
	for caseIndex := 0; caseIndex < count; caseIndex++ {
		in := diffInput{
			Options: GraphOptions{Compound: true, Multigraph: true},
			Graph: Attrs{
				"rankdir": directions[rng.Intn(len(directions))],
				"ranker":  rankers[rng.Intn(len(rankers))],
				"nodesep": float64(10 + rng.Intn(91)),
				"edgesep": float64(5 + rng.Intn(46)),
				"ranksep": float64(20 + rng.Intn(181)),
			},
		}
		if rng.Intn(3) == 0 {
			in.Graph["acyclicer"] = "greedy"
		}
		if align := alignments[rng.Intn(len(alignments))]; align != "" {
			in.Graph["align"] = align
		}
		if rng.Intn(4) == 0 {
			in.Graph["marginx"], in.Graph["marginy"] = float64(rng.Intn(20)), float64(rng.Intn(20))
		}
		n := 2 + rng.Intn(7)
		compound := rng.Intn(3) == 0
		if compound {
			in.Nodes = append(in.Nodes, diffNodeInput{ID: "cluster", Attrs: Attrs{}})
		}
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf("%d", i)
		}
		rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		for i, id := range ids {
			node := diffNodeInput{
				ID: id,
				Attrs: Attrs{
					"width":  float64(10+rng.Intn(111)) + float64(rng.Intn(4))/4,
					"height": float64(10+rng.Intn(91)) + float64(rng.Intn(4))/4,
				},
			}
			if compound && i < n/2 {
				node.Parent = strptr("cluster")
			}
			in.Nodes = append(in.Nodes, node)
		}
		edgeCount := 1 + rng.Intn(n*2+1)
		for i := 0; i < edgeCount; i++ {
			v, w := ids[rng.Intn(n)], ids[rng.Intn(n)]
			attrs := Attrs{
				"minlen": float64(1 + rng.Intn(3)),
				"weight": float64(1 + rng.Intn(4)),
			}
			if rng.Intn(2) == 0 {
				attrs["width"] = float64(rng.Intn(61))
				attrs["height"] = float64(rng.Intn(31))
				attrs["labelpos"] = labelPositions[rng.Intn(len(labelPositions))]
				attrs["labeloffset"] = float64(rng.Intn(21))
			}
			name := fmt.Sprintf("e%d", i)
			in.Edges = append(in.Edges, diffEdgeInput{V: v, W: w, Name: &name, Attrs: attrs})
		}
		fixtures = append(fixtures, namedFixture{name: fmt.Sprintf("random-%03d", caseIndex), input: in})
	}
	return fixtures
}

func runDifferentialGo(t *testing.T, input diffInput) []byte {
	t.Helper()
	g := NewGraph(input.Options).SetGraph(cloneAttrs(input.Graph))
	g.SetDefaultNodeLabel(func(string) any { return Attrs{} })
	g.SetDefaultEdgeLabel(func(string, string, *string) any { return Attrs{} })
	for _, node := range input.Nodes {
		g.SetNode(node.ID, cloneAttrs(node.Attrs))
	}
	for _, node := range input.Nodes {
		if node.Parent != nil {
			if err := g.SetParent(node.ID, *node.Parent); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, edge := range input.Edges {
		if edge.Name != nil {
			g.SetEdge(edge.V, edge.W, cloneAttrs(edge.Attrs), *edge.Name)
		} else {
			g.SetEdge(edge.V, edge.W, cloneAttrs(edge.Attrs))
		}
	}
	if err := Layout(g); err != nil {
		t.Fatal(err)
	}
	output := map[string]any{
		"graph": map[string]any{"width": num(asAttrs(g.Graph()), "width"), "height": num(asAttrs(g.Graph()), "height")},
	}
	nodes := make([]any, 0, g.NodeCount())
	for _, id := range g.Nodes() {
		n := asAttrs(g.Node(id))
		nodes = append(nodes, map[string]any{"id": id, "x": num(n, "x"), "y": num(n, "y"), "width": n["width"], "height": n["height"]})
	}
	output["nodes"] = nodes
	edges := make([]any, 0, g.EdgeCount())
	for _, edgeObj := range g.Edges() {
		e := asAttrs(g.Edge(edgeObj))
		item := map[string]any{
			"v": edgeObj.V, "w": edgeObj.W, "namePresent": edgeObj.HasName, "name": edgeObj.Name,
			"points": e["points"], "xPresent": has(e, "x"), "yPresent": has(e, "y"),
		}
		if has(e, "x") {
			item["x"] = num(e, "x")
		}
		if has(e, "y") {
			item["y"] = num(e, "y")
		}
		edges = append(edges, item)
	}
	output["edges"] = edges
	b, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func compareJSON(t *testing.T, path string, want, got any) {
	t.Helper()
	switch w := want.(type) {
	case float64:
		g, ok := got.(float64)
		if !ok || math.Abs(w-g) > 1e-9+1e-12*math.Max(math.Abs(w), math.Abs(g)) {
			t.Errorf("%s: want %.17g, got %#v", path, w, got)
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			t.Errorf("%s: want array, got %T", path, got)
			return
		}
		if len(w) != len(g) {
			t.Errorf("%s: array length want %d, got %d", path, len(w), len(g))
			return
		}
		for i := range w {
			compareJSON(t, fmt.Sprintf("%s[%d]", path, i), w[i], g[i])
		}
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			t.Errorf("%s: want object, got %T", path, got)
			return
		}
		wantKeys, gotKeys := make([]string, 0, len(w)), make([]string, 0, len(g))
		for key := range w {
			wantKeys = append(wantKeys, key)
		}
		for key := range g {
			gotKeys = append(gotKeys, key)
		}
		sort.Strings(wantKeys)
		sort.Strings(gotKeys)
		if !reflect.DeepEqual(wantKeys, gotKeys) {
			t.Errorf("%s: keys want %v, got %v", path, wantKeys, gotKeys)
			return
		}
		for _, key := range wantKeys {
			compareJSON(t, path+"."+key, w[key], g[key])
		}
	default:
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s: want %#v, got %#v", path, want, got)
		}
	}
}

func newLayoutTestGraph() *Graph {
	return NewGraph(GraphOptions{Multigraph: true, Compound: true}).
		SetGraph(Attrs{}).
		SetDefaultEdgeLabel(func(string, string, *string) any { return Attrs{} })
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %.17g, want %.17g", got, want)
	}
}

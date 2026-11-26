package dagro

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

// The tests in this file compare LayoutTyped with Dagre 3.1.1 itself: with
// the recorded outputs of the D2 corpus (testdata/differential/d2-corpus),
// and, when CI provides the JavaScript bundles (see .github/workflows/ci.yml),
// with dagre.js run by testdata/differential/oracle.js.

const compatibilityOracleSHA256 = "8e34c25ed53dbccca2fa206780b0b46974b285c74e0cd7b34d0d1fafa5506cab"

type wireNode struct {
	ID     string         `json:"id"`
	Attrs  map[string]any `json:"attrs"`
	Parent *string        `json:"parent,omitempty"`
}

type wireEdge struct {
	V     string         `json:"v"`
	W     string         `json:"w"`
	Name  *string        `json:"name,omitempty"`
	Attrs map[string]any `json:"attrs"`
}

type wireInput struct {
	Options map[string]bool `json:"options"`
	Graph   map[string]any  `json:"graph"`
	Nodes   []wireNode      `json:"nodes"`
	Edges   []wireEdge      `json:"edges"`
}

// wireJSON returns in as the input of oracle.js: the graph dagre.js lays out
// as LayoutTyped lays out in. Attributes that equal dagre's defaults and are
// optional in LayoutInput are left out.
func wireJSON(in *LayoutInput) ([]byte, error) {
	w := wireInput{
		Options: map[string]bool{"directed": true, "multigraph": true, "compound": true},
		Graph:   map[string]any{"rankdir": in.RankDir, "ranksep": in.RankSep, "edgesep": in.EdgeSep, "nodesep": in.NodeSep},
	}
	if in.MarginX != 0 {
		w.Graph["marginx"] = in.MarginX
	}
	if in.MarginY != 0 {
		w.Graph["marginy"] = in.MarginY
	}
	for k, v := range map[string]string{"acyclicer": in.Acyclicer, "ranker": in.Ranker, "align": in.Align} {
		if v != "" {
			w.Graph[k] = v
		}
	}
	for _, n := range in.Nodes {
		node := wireNode{ID: n.ID, Attrs: map[string]any{"width": n.Width, "height": n.Height}}
		if n.Parent >= 0 {
			node.Parent = &in.Nodes[n.Parent].ID
		}
		w.Nodes = append(w.Nodes, node)
	}
	for _, e := range in.Edges {
		edge := wireEdge{V: in.Nodes[e.V].ID, W: in.Nodes[e.W].ID, Attrs: map[string]any{"width": e.Width, "height": e.Height, "labelpos": e.LabelPos}}
		if e.HasName {
			name := e.Name
			edge.Name = &name
		}
		if e.Minlen != 1 {
			edge.Attrs["minlen"] = e.Minlen
		}
		if e.Weight != 1 {
			edge.Attrs["weight"] = e.Weight
		}
		if e.LabelOffset != 10 {
			edge.Attrs["labeloffset"] = e.LabelOffset
		}
		w.Edges = append(w.Edges, edge)
	}
	return json.Marshal(w)
}

// oracleArrayIndex reports whether s is a JavaScript array index, which object
// key order puts first, in numeric order.
func oracleArrayIndex(s string) (uint64, bool) {
	n, err := strconv.ParseUint(s, 10, 32)
	return n, err == nil && n < math.MaxUint32 && strconv.FormatUint(n, 10) == s
}

// outputJSON returns LayoutTyped's result for in as oracle.js reports
// dagre.js's: nodes in JavaScript key order, edges in input order.
func outputJSON(in *LayoutInput, out *LayoutOutput) ([]byte, error) {
	order := make([]int, len(in.Nodes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		na, ia := oracleArrayIndex(in.Nodes[order[a]].ID)
		nb, ib := oracleArrayIndex(in.Nodes[order[b]].ID)
		if ia && ib {
			return na < nb
		}
		return ia && !ib
	})
	nodes := make([]any, 0, len(order))
	for _, i := range order {
		n := out.Nodes[i]
		nodes = append(nodes, map[string]any{"id": in.Nodes[i].ID, "x": n.X, "y": n.Y, "width": n.Width, "height": n.Height})
	}
	edges := make([]any, 0, len(in.Edges))
	for i, e := range in.Edges {
		l := out.Edges[i]
		item := map[string]any{
			"v": in.Nodes[e.V].ID, "w": in.Nodes[e.W].ID, "namePresent": e.HasName, "name": e.Name,
			"points": l.Points, "xPresent": l.HasXY, "yPresent": l.HasXY,
		}
		if l.HasXY {
			item["x"], item["y"] = l.X, l.Y
		}
		edges = append(edges, item)
	}
	return json.Marshal(map[string]any{
		"graph": map[string]any{"width": out.Width, "height": out.Height},
		"nodes": nodes, "edges": edges,
	})
}

// layoutJSON lays out in with LayoutTyped and returns outputJSON's result
// decoded.
func layoutJSON(t *testing.T, what string, in *LayoutInput) any {
	t.Helper()
	var out LayoutOutput
	if err := LayoutTyped(in, &out); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	b, err := outputJSON(in, &out)
	if err != nil {
		t.Fatalf("%s: encode output: %v", what, err)
	}
	var got any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("%s: decode output: %v", what, err)
	}
	return got
}

// runOracle runs oracle.js with the dagre.js bundle dagreJS on in.
func runOracle(t *testing.T, what, node, dagreJS string, in *LayoutInput) any {
	t.Helper()
	inputJSON, err := wireJSON(in)
	if err != nil {
		t.Fatalf("%s: encode input: %v", what, err)
	}
	cmd := exec.Command(node, "testdata/differential/oracle.js")
	cmd.Env = append(os.Environ(), "DAGRO_DAGRE_JS="+dagreJS)
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	jsJSON, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: JS oracle: %v: %s", what, err, stderr.String())
	}
	var want any
	if err := json.Unmarshal(jsJSON, &want); err != nil {
		t.Fatalf("%s: decode JS: %v\n%s", what, err, jsJSON)
	}
	return want
}

func oracleFixtures() []struct {
	name string
	in   LayoutInput
} {
	node := func(id string, parent int, w, h float64) LayoutNode {
		return LayoutNode{ID: id, Parent: parent, Width: w, Height: h}
	}
	named := func(v, w int, name string) LayoutEdge {
		e := NewLayoutEdge(v, w)
		e.Name, e.HasName = name, true
		return e
	}
	var out []struct {
		name string
		in   LayoutInput
	}
	add := func(name string, in LayoutInput) {
		out = append(out, struct {
			name string
			in   LayoutInput
		}{name, in})
	}

	in := DefaultLayoutInput()
	in.Nodes = []LayoutNode{node("a", -1, 50, 100)}
	add("single", in)

	in = DefaultLayoutInput()
	in.RankSep = 200
	in.Nodes = []LayoutNode{node("a", -1, 100, 100), node("b", -1, 100, 100)}
	in.Edges = []LayoutEdge{NewLayoutEdge(0, 1)}
	add("chain", in)

	in = LayoutInput{RankDir: "LR", NodeSep: 31, EdgeSep: 17, RankSep: 77}
	in.Nodes = []LayoutNode{node("0", -1, 80, 40), node("1", -1, 60, 50)}
	in.Edges = []LayoutEdge{named(0, 1, "edge-a"), named(0, 1, "edge-b")}
	in.Edges[0].Width, in.Edges[0].Height, in.Edges[0].LabelPos = 30, 10, "c"
	in.Edges[1].Width, in.Edges[1].Height, in.Edges[1].LabelPos = 20, 15, "c"
	add("d2-multiedge", in)

	in = DefaultLayoutInput()
	in.RankDir, in.Acyclicer = "BT", "greedy"
	in.Nodes = []LayoutNode{node("a", -1, 100, 60), node("b", -1, 70, 70), node("c", -1, 40, 80)}
	in.Edges = []LayoutEdge{named(0, 1, "ab"), named(1, 2, "bc"), named(2, 0, "ca")}
	in.Edges[0].Weight = 2
	add("cycle-greedy", in)

	in = DefaultLayoutInput()
	in.RankDir, in.EdgeSep = "RL", 75
	in.Nodes = []LayoutNode{node("a", -1, 100, 100)}
	in.Edges = []LayoutEdge{named(0, 0, "self")}
	in.Edges[0].Width, in.Edges[0].Height = 50, 50
	add("self-loop", in)

	in = DefaultLayoutInput()
	in.Nodes = []LayoutNode{node("cluster", -1, 0, 0), node("a", 0, 50, 50), node("b", 0, 80, 30), node("outside", -1, 40, 40)}
	in.Edges = []LayoutEdge{named(1, 2, "inside"), named(2, 3, "out")}
	in.Edges[1].Width, in.Edges[1].Height, in.Edges[1].LabelPos = 30, 12, "l"
	add("compound", in)

	in = DefaultLayoutInput()
	for i, id := range []string{"10", "2", "alpha", "1", "beta", "01", "4294967294", "4294967295", "z", "3", "leaf"} {
		in.Nodes = append(in.Nodes, node(id, i-1, 40+float64(i), 30+float64(i)))
	}
	add("deep-compound-key-order", in)
	return out
}

func TestLayoutMatchesDagreJS(t *testing.T) {
	dagreJS := os.Getenv("DAGRO_DAGRE_JS")
	if dagreJS == "" {
		t.Skip("set DAGRO_DAGRE_JS to the pinned Dagre 3.1.1 CommonJS bundle to run differential tests")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	for _, fixture := range oracleFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			want := runOracle(t, fixture.name, node, dagreJS, &fixture.in)
			compareJSON(t, "$", want, layoutJSON(t, fixture.name, &fixture.in))
		})
	}
}

func TestD2ProfileRandomLayoutsAreFinite(t *testing.T) {
	rng := rand.New(rand.NewSource(3112026))
	for i := 0; i < 300; i++ {
		input := d2ProfileInput(rng, i)
		if i == 24 {
			wire, err := wireJSON(&input)
			if err != nil {
				t.Fatal(err)
			}
			const wantSHA256 = "abfd93c91884b993c19a70c48f26c83816e95f22f750750cf966f4a2fa80089b"
			if got := fmt.Sprintf("%x", sha256.Sum256(wire)); got != wantSHA256 {
				t.Fatalf("seed case 24 SHA-256 = %s, want %s", got, wantSHA256)
			}
		}
		what := fmt.Sprintf("seed[3112026].case[%d]", i)
		assertFiniteJSON(t, what, layoutJSON(t, what, &input))
	}
}

func TestD2ProfileRandomLayoutsMatchCompatibilityOracle(t *testing.T) {
	dagreJS := os.Getenv("DAGRO_DAGRE_JS_COMPAT")
	if dagreJS == "" {
		t.Skip("set DAGRO_DAGRE_JS_COMPAT to the reproducibly built compatibility oracle")
	}
	oracle, err := os.ReadFile(dagreJS)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(oracle)); got != compatibilityOracleSHA256 {
		t.Fatalf("compatibility oracle SHA-256 = %s, want %s", got, compatibilityOracleSHA256)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	rng := rand.New(rand.NewSource(3112026))
	for i := 0; i < 300; i++ {
		input := d2ProfileInput(rng, i)
		what := fmt.Sprintf("seed[3112026].case[%d]", i)
		want := runOracle(t, what, node, dagreJS, &input)
		compareJSON(t, what, want, layoutJSON(t, what, &input))
	}
}

// d2CorpusEntry is an input of the D2 corpus and dagre's output for it.
type d2CorpusEntry struct {
	Input                  string   `json:"input"`
	Expected               string   `json:"expected"`
	ExpectedSHA256         string   `json:"expected_sha256"`
	ExpectedSource         string   `json:"expected_source"`
	OracleStatus           string   `json:"oracle_status"`
	OfficialNonfinitePaths []string `json:"official_nonfinite_paths"`
}

type d2CorpusManifest struct {
	Graphs                     map[string]d2CorpusEntry `json:"graphs"`
	CompatibilityRootFixInputs []string                 `json:"compatibility_root_fix_inputs"`
	D2                         struct {
		Commit        string `json:"commit"`
		Copyright     string `json:"copyright"`
		License       string `json:"license"`
		LicenseFile   string `json:"license_file"`
		LicenseSHA256 string `json:"license_sha256"`
		NoticeFile    string `json:"notice_file"`
	} `json:"d2"`
	Oracle struct {
		DagreVersion     string `json:"dagre_version"`
		DagreGitHead     string `json:"dagre_git_head"`
		DagreCJSHA256    string `json:"dagre_cjs_sha256"`
		GraphlibVersion  string `json:"graphlib_version"`
		GraphlibGitHead  string `json:"graphlib_git_head"`
		GraphlibCJSHA256 string `json:"graphlib_cjs_sha256"`
	} `json:"oracle"`
	Counts struct {
		UniqueInputs               int `json:"unique_inputs"`
		OfficialSuccesses          int `json:"official_successes"`
		OfficialErrors             int `json:"official_errors"`
		CompatibilityRootFixInputs int `json:"compatibility_root_fix_inputs"`
	} `json:"counts"`
}

const d2CorpusDir = "testdata/differential/d2-corpus"

func readD2CorpusManifest(t *testing.T, dir string) d2CorpusManifest {
	t.Helper()
	var manifest d2CorpusManifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// readD2CorpusInput reads a corpus input (the graph D2's dagre adapter
// built) as a LayoutInput.
func readD2CorpusInput(t *testing.T, path string) LayoutInput {
	t.Helper()
	var wire struct {
		Options map[string]bool `json:"options"`
		Graph   struct {
			RankDir string  `json:"rankdir"`
			RankSep float64 `json:"ranksep"`
			EdgeSep float64 `json:"edgesep"`
			NodeSep float64 `json:"nodesep"`
		} `json:"graph"`
		Nodes []struct {
			ID     string  `json:"id"`
			Parent *string `json:"parent"`
			Attrs  struct {
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"attrs"`
		} `json:"nodes"`
		Edges []struct {
			V, W  string
			Name  *string `json:"name"`
			Attrs struct {
				Width    float64 `json:"width"`
				Height   float64 `json:"height"`
				LabelPos string  `json:"labelpos"`
			} `json:"attrs"`
		} `json:"edges"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	if !wire.Options["directed"] || !wire.Options["multigraph"] || !wire.Options["compound"] {
		t.Fatalf("%s: options %v", path, wire.Options)
	}
	in := LayoutInput{RankDir: wire.Graph.RankDir, RankSep: wire.Graph.RankSep, EdgeSep: wire.Graph.EdgeSep, NodeSep: wire.Graph.NodeSep}
	index := map[string]int{}
	for i, n := range wire.Nodes {
		index[n.ID] = i
		in.Nodes = append(in.Nodes, LayoutNode{ID: n.ID, Parent: -1, Width: n.Attrs.Width, Height: n.Attrs.Height})
	}
	lookup := func(id string) int {
		i, ok := index[id]
		if !ok {
			t.Fatalf("%s: unknown node %q", path, id)
		}
		return i
	}
	for i, n := range wire.Nodes {
		if n.Parent != nil {
			in.Nodes[i].Parent = lookup(*n.Parent)
		}
	}
	for _, e := range wire.Edges {
		edge := NewLayoutEdge(lookup(e.V), lookup(e.W))
		if e.Name != nil {
			edge.Name, edge.HasName = *e.Name, true
		}
		edge.Width, edge.Height, edge.LabelPos = e.Attrs.Width, e.Attrs.Height, e.Attrs.LabelPos
		in.Edges = append(in.Edges, edge)
	}
	return in
}

func TestLayoutMatchesD2Corpus(t *testing.T) {
	corpusDir := os.Getenv("DAGRO_D2_CORPUS")
	if corpusDir == "" {
		corpusDir = d2CorpusDir
	}
	manifest := readD2CorpusManifest(t, corpusDir)
	if manifest.D2.Commit != "1a60d69e4df9b9557923e61bf10f9aa3aa5422e1" {
		t.Fatalf("D2 corpus commit = %q", manifest.D2.Commit)
	}
	if manifest.D2.Copyright != "Copyright 2022 Terrastruct Inc." || manifest.D2.License != "MPL-2.0" || manifest.D2.NoticeFile == "" {
		t.Fatalf("unexpected D2 corpus attribution: %#v", manifest.D2)
	}
	d2License, err := os.ReadFile(filepath.Join(corpusDir, manifest.D2.LicenseFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(d2License)); got != manifest.D2.LicenseSHA256 {
		t.Fatalf("D2 license SHA-256 = %s, want %s", got, manifest.D2.LicenseSHA256)
	}
	if _, err := os.Stat(filepath.Join(corpusDir, manifest.D2.NoticeFile)); err != nil {
		t.Fatal(err)
	}
	if manifest.Oracle.DagreVersion != "3.1.1" || manifest.Oracle.DagreGitHead != "c3ed0802cd98de74c21cff1f754689ebbb0f8dae" || manifest.Oracle.DagreCJSHA256 != "70b9a4367932dd436075d98892a7968d65cf66ae83263f995e0531823b59b671" {
		t.Fatalf("unexpected Dagre oracle provenance: %#v", manifest.Oracle)
	}
	if manifest.Oracle.GraphlibVersion != "4.0.5" || manifest.Oracle.GraphlibGitHead != "d3a0cf36f55ebd75f28b6acf7a436a54e1b990dc" || manifest.Oracle.GraphlibCJSHA256 != "271f39d50dbcf2f795808cb4f5b90fb42a096b5f84b4dd6bb672487b454011e7" {
		t.Fatalf("unexpected Graphlib oracle provenance: %#v", manifest.Oracle)
	}
	if manifest.Counts.UniqueInputs != 311 || len(manifest.Graphs) != 311 {
		t.Fatalf("corpus unique inputs = %d (graphs %d), want 311", manifest.Counts.UniqueInputs, len(manifest.Graphs))
	}
	if manifest.Counts.OfficialSuccesses != 309 || manifest.Counts.OfficialErrors != 2 || manifest.Counts.CompatibilityRootFixInputs != 3 {
		t.Fatalf("corpus official successes/errors/compat = %d/%d/%d, want 309/2/3",
			manifest.Counts.OfficialSuccesses, manifest.Counts.OfficialErrors, manifest.Counts.CompatibilityRootFixInputs)
	}
	officialFinite, compatibility := 0, 0
	for _, entry := range manifest.Graphs {
		if entry.OracleStatus == "success" && len(entry.OfficialNonfinitePaths) == 0 {
			officialFinite++
		}
		switch entry.ExpectedSource {
		case "official-3.1.1-with-d2-json-bridge-normalization":
		case "parallel-edge-order-and-zero-size-intersection-root-fix":
			compatibility++
		default:
			t.Fatalf("unexpected corpus expected_source %q", entry.ExpectedSource)
		}
	}
	if officialFinite != 308 || compatibility != 3 || officialFinite+compatibility != len(manifest.Graphs) {
		t.Fatalf("corpus oracle partition = %d official-finite + %d compatibility, want 308 + 3 = 311", officialFinite, compatibility)
	}
	wantCompatibilityIDs := []string{
		"7cfd90e29056db3a1a4d2b45690869ff537734eb5d809ab5f1bb832e59a0bc67",
		"e052b4c21cba3edb2df9b001d3f64058bf66eb4b30aeec321afa063278915d88",
		"e2cfd977b7a3bf293fced2080851d9ce8e6bf5425153799b0f3efed58ac27853",
	}
	sort.Strings(manifest.CompatibilityRootFixInputs)
	if !reflect.DeepEqual(manifest.CompatibilityRootFixInputs, wantCompatibilityIDs) {
		t.Fatalf("compatibility input IDs = %v, want %v", manifest.CompatibilityRootFixInputs, wantCompatibilityIDs)
	}

	ids := make([]string, 0, len(manifest.Graphs))
	for id := range manifest.Graphs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		id, entry := id, manifest.Graphs[id]
		t.Run(id[:12], func(t *testing.T) {
			inputPath := filepath.Join(corpusDir, entry.Input)
			inputJSON, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(inputJSON)); got != id {
				t.Fatalf("input SHA-256 = %s, want content-addressed ID %s", got, id)
			}
			input := readD2CorpusInput(t, inputPath)
			got := layoutJSON(t, id[:12], &input)
			wantJSON, err := os.ReadFile(filepath.Join(corpusDir, entry.Expected))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(wantJSON)); got != entry.ExpectedSHA256 {
				t.Fatalf("expected output SHA-256 = %s, want %s", got, entry.ExpectedSHA256)
			}
			var want any
			if err := json.Unmarshal(wantJSON, &want); err != nil {
				t.Fatal(err)
			}
			compareJSON(t, "$", want, got)
			assertFiniteJSON(t, "$", got)
		})
	}
}

func assertFiniteJSON(t *testing.T, path string, value any) {
	t.Helper()
	switch value := value.(type) {
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Errorf("%s: non-finite value %v", path, value)
		}
	case nil:
		t.Errorf("%s: null numeric output", path)
	case []any:
		for i, child := range value {
			assertFiniteJSON(t, fmt.Sprintf("%s[%d]", path, i), child)
		}
	case map[string]any:
		for key, child := range value {
			assertFiniteJSON(t, path+"."+key, child)
		}
	}
}

func compareJSON(t *testing.T, path string, want, got any) {
	t.Helper()
	switch w := want.(type) {
	case float64:
		g, ok := got.(float64)
		if !ok || math.Float64bits(w) != math.Float64bits(g) {
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

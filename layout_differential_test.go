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
	"strings"
	"sync"
	"testing"
)

const compatibilityOracleSHA256 = "8e34c25ed53dbccca2fa206780b0b46974b285c74e0cd7b34d0d1fafa5506cab"

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

func TestLayoutModernDagreKeepsLastEqualCrossingSweep(t *testing.T) {
	g := NewGraph(GraphOptions{Compound: true, Multigraph: true}).SetGraph(Attrs{
		"rankdir": "LR", "nodesep": 31.0, "edgesep": 17.0, "ranksep": 77.0,
	})
	g.SetNode("0", Attrs{"width": 80.0, "height": 40.0})
	g.SetNode("1", Attrs{"width": 60.0, "height": 50.0})
	g.SetEdge("0", "1", Attrs{"width": 30.0, "height": 10.0, "labelpos": "c"}, "edge-a")
	g.SetEdge("0", "1", Attrs{"width": 20.0, "height": 15.0, "labelpos": "c"}, "edge-b")

	if err := Layout(g); err != nil {
		t.Fatal(err)
	}

	// Dagre 3.1.1 keeps the final solution when crossing counts tie. This
	// places edge-a below edge-b; Dagre 0.8.5 retained the opposite sweep.
	assertNear(t, num(asAttrs(g.EdgeByArgs("0", "1", "edge-a")), "y"), 39.75)
	assertNear(t, num(asAttrs(g.EdgeByArgs("0", "1", "edge-b")), "y"), 10.25)
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
	fixtures := differentialFixtures()
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			inputJSON, err := json.Marshal(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			goJSON := runDifferentialGo(t, fixture.input)
			cmd := exec.Command(node, "testdata/differential/oracle.js")
			cmd.Env = append(os.Environ(), "DAGRO_DAGRE_JS="+dagreJS)
			cmd.Stdin = bytes.NewReader(inputJSON)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			jsJSON, err := cmd.Output()
			if err != nil {
				t.Fatalf("JS oracle: %v: %s", err, stderr.String())
			}
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

func TestLayoutDagre311CompatibilityCases(t *testing.T) {
	const compatibilityDir = "testdata/differential/compatibility"
	type compatibilityCase struct {
		Name               string `json:"name"`
		ID                 string `json:"id"`
		Input              string `json:"input"`
		Expected           string `json:"expected"`
		ExpectedSHA256     string `json:"expected_sha256"`
		OfficialStatus     string `json:"official_status"`
		OfficialNullValues int    `json:"official_null_values"`
		OfficialError      string `json:"official_error"`
		OfficialSHA256     string `json:"official_sha256"`
		Reason             string `json:"reason"`
	}
	var manifest struct {
		Source struct {
			CompatibilityPatch       string `json:"compatibility_patch"`
			CompatibilityPatchSHA256 string `json:"compatibility_patch_sha256"`
			CompatibilityOracle      string `json:"compatibility_oracle_builder"`
			CompatibilityOracleSHA   string `json:"compatibility_oracle_sha256"`
			D2Copyright              string `json:"d2_copyright"`
			D2License                string `json:"d2_license"`
			D2LicenseFile            string `json:"d2_license_file"`
			D2LicenseSHA256          string `json:"d2_license_sha256"`
			D2NoticeFile             string `json:"d2_notice_file"`
		} `json:"source"`
		Cases []compatibilityCase `json:"cases"`
	}
	manifestJSON, err := os.ReadFile(filepath.Join(compatibilityDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Cases) != 4 {
		t.Fatalf("compatibility cases = %d, want 4", len(manifest.Cases))
	}
	patchData, err := os.ReadFile(filepath.Join(compatibilityDir, manifest.Source.CompatibilityPatch))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(patchData)); got != manifest.Source.CompatibilityPatchSHA256 {
		t.Fatalf("compatibility patch SHA-256 = %s, want %s", got, manifest.Source.CompatibilityPatchSHA256)
	}
	if manifest.Source.CompatibilityOracleSHA != compatibilityOracleSHA256 {
		t.Fatalf("compatibility oracle SHA-256 = %s, want %s", manifest.Source.CompatibilityOracleSHA, compatibilityOracleSHA256)
	}
	if _, err := os.Stat(filepath.Join(compatibilityDir, manifest.Source.CompatibilityOracle)); err != nil {
		t.Fatal(err)
	}
	if manifest.Source.D2Copyright != "Copyright 2022 Terrastruct Inc." || manifest.Source.D2License != "MPL-2.0" || manifest.Source.D2NoticeFile == "" {
		t.Fatalf("unexpected D2 corpus attribution: %#v", manifest.Source)
	}
	d2License, err := os.ReadFile(filepath.Join(compatibilityDir, manifest.Source.D2LicenseFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(d2License)); got != manifest.Source.D2LicenseSHA256 {
		t.Fatalf("D2 license SHA-256 = %s, want %s", got, manifest.Source.D2LicenseSHA256)
	}
	if _, err := os.Stat(filepath.Join(compatibilityDir, manifest.Source.D2NoticeFile)); err != nil {
		t.Fatal(err)
	}

	dagreJS := os.Getenv("DAGRO_DAGRE_JS")
	node, nodeErr := exec.LookPath("node")
	for _, testCase := range manifest.Cases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			inputPath := filepath.Join(compatibilityDir, testCase.Input)
			inputJSON, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(inputJSON)); got != testCase.ID {
				t.Fatalf("input SHA-256 = %s, want content-addressed ID %s", got, testCase.ID)
			}
			input := decodeDifferentialInput(t, inputPath)
			gotJSON := runDifferentialGo(t, input)
			wantJSON, err := os.ReadFile(filepath.Join(compatibilityDir, testCase.Expected))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(wantJSON)); got != testCase.ExpectedSHA256 {
				t.Fatalf("expected output SHA-256 = %s, want %s", got, testCase.ExpectedSHA256)
			}
			var want, got any
			if err := json.Unmarshal(wantJSON, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(gotJSON, &got); err != nil {
				t.Fatal(err)
			}
			compareJSON(t, "$", want, got)
			assertFiniteJSON(t, "$", got)

			t.Run("official-3.1.1-behavior", func(t *testing.T) {
				if dagreJS == "" {
					t.Skip("DAGRO_DAGRE_JS is unset; CI supplies the pinned official 3.1.1 bundle")
				}
				if nodeErr != nil {
					t.Skip("node is not on PATH; CI pins Node and runs this check")
				}
				inputJSON, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(node, "testdata/differential/oracle.js")
				cmd.Env = append(os.Environ(), "DAGRO_DAGRE_JS="+dagreJS)
				cmd.Stdin = bytes.NewReader(inputJSON)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				jsJSON, err := cmd.Output()
				switch testCase.OfficialStatus {
				case "finite-different":
					if err != nil {
						t.Fatalf("official oracle unexpectedly failed: %v: %s", err, stderr.String())
					}
					if got := fmt.Sprintf("%x", sha256.Sum256(jsJSON)); got != testCase.OfficialSHA256 {
						t.Fatalf("official output SHA-256 = %s, want %s", got, testCase.OfficialSHA256)
					}
					var official any
					if err := json.Unmarshal(jsJSON, &official); err != nil {
						t.Fatal(err)
					}
					assertFiniteJSON(t, "official", official)
					if reflect.DeepEqual(official, want) {
						t.Fatal("official output unexpectedly matches the compatibility result")
					}
				case "nonfinite":
					if err != nil {
						t.Fatalf("official oracle unexpectedly failed: %v: %s", err, stderr.String())
					}
					var official any
					if err := json.Unmarshal(jsJSON, &official); err != nil {
						t.Fatal(err)
					}
					if nulls := countJSONNulls(official); nulls != testCase.OfficialNullValues {
						t.Fatalf("official null values = %d, want %d", nulls, testCase.OfficialNullValues)
					}
				case "error":
					if err == nil {
						t.Fatal("official oracle unexpectedly succeeded")
					}
					if !strings.Contains(stderr.String(), testCase.OfficialError) {
						t.Fatalf("official error does not contain %q: %s", testCase.OfficialError, stderr.String())
					}
				default:
					t.Fatalf("unknown official status %q", testCase.OfficialStatus)
				}
				t.Log(testCase.Reason)
			})
		})
	}
}

func TestD2ProfileRandomLayoutsAreFinite(t *testing.T) {
	rng := rand.New(rand.NewSource(3112026))
	for i := 0; i < 300; i++ {
		input := d2ProfileRandomInput(rng, i)
		if i == 24 {
			wire, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			const wantSHA256 = "abfd93c91884b993c19a70c48f26c83816e95f22f750750cf966f4a2fa80089b"
			if got := fmt.Sprintf("%x", sha256.Sum256(wire)); got != wantSHA256 {
				t.Fatalf("seed case 24 SHA-256 = %s, want %s", got, wantSHA256)
			}
		}
		output, err := runDifferentialGoResult(input)
		if err != nil {
			t.Fatalf("seed 3112026 case %d: %v", i, err)
		}
		var decoded any
		if err := json.Unmarshal(output, &decoded); err != nil {
			t.Fatalf("seed 3112026 case %d: decode output: %v", i, err)
		}
		assertFiniteJSON(t, fmt.Sprintf("seed[3112026].case[%d]", i), decoded)
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
		input := d2ProfileRandomInput(rng, i)
		inputJSON, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("seed 3112026 case %d: encode input: %v", i, err)
		}
		goJSON, err := runDifferentialGoResult(input)
		if err != nil {
			t.Fatalf("seed 3112026 case %d: Go layout: %v", i, err)
		}
		cmd := exec.Command(node, "testdata/differential/oracle.js")
		cmd.Env = append(os.Environ(), "DAGRO_DAGRE_JS="+dagreJS)
		cmd.Stdin = bytes.NewReader(inputJSON)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		jsJSON, err := cmd.Output()
		if err != nil {
			t.Fatalf("seed 3112026 case %d: compatibility oracle: %v: %s", i, err, stderr.String())
		}
		var want, got any
		if err := json.Unmarshal(jsJSON, &want); err != nil {
			t.Fatalf("seed 3112026 case %d: decode oracle output: %v", i, err)
		}
		if err := json.Unmarshal(goJSON, &got); err != nil {
			t.Fatalf("seed 3112026 case %d: decode Go output: %v", i, err)
		}
		compareJSON(t, fmt.Sprintf("seed[3112026].case[%d]", i), want, got)
	}
}

func d2ProfileRandomInput(rng *rand.Rand, index int) diffInput {
	directions := []string{"TB", "BT", "LR", "RL"}
	input := diffInput{
		Options: GraphOptions{Directed: true, Multigraph: true, Compound: true},
		Graph: Attrs{
			"rankdir": directions[rng.Intn(len(directions))],
			"nodesep": float64(10 + rng.Intn(101)),
			"edgesep": float64(5 + rng.Intn(51)),
			"ranksep": float64(20 + rng.Intn(181)),
		},
	}
	nodeCount := 2 + rng.Intn(10)
	clusterCount := rng.Intn(4)
	for cluster := 0; cluster < clusterCount; cluster++ {
		input.Nodes = append(input.Nodes, diffNodeInput{
			ID: fmt.Sprintf("cluster%d", cluster),
			Attrs: Attrs{
				"width": float64(40 + rng.Intn(120)), "height": float64(40 + rng.Intn(100)),
			},
		})
	}
	ids := make([]string, nodeCount)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	for i, id := range ids {
		node := diffNodeInput{
			ID: id,
			Attrs: Attrs{
				"width": float64(10 + rng.Intn(121)), "height": float64(10 + rng.Intn(101)),
			},
		}
		if clusterCount > 0 && (i+index)%3 != 0 {
			node.Parent = strptr(fmt.Sprintf("cluster%d", rng.Intn(clusterCount)))
		}
		input.Nodes = append(input.Nodes, node)
	}
	edgeCount := 1 + rng.Intn(nodeCount*3)
	for i := 0; i < edgeCount; i++ {
		v, w := ids[rng.Intn(nodeCount)], ids[rng.Intn(nodeCount)]
		input.Edges = append(input.Edges, diffEdgeInput{
			V: v, W: w, Name: strptr(fmt.Sprintf("(%s -> %s)[%d]", v, w, i)),
			Attrs: Attrs{
				"width": float64(rng.Intn(81)), "height": float64(rng.Intn(41)), "labelpos": "c",
			},
		})
	}
	return input
}

func TestLayoutConcurrentDummyIDsAreDeterministic(t *testing.T) {
	fixture := concurrentDummyIDFixture()
	want, err := runDifferentialGoResult(fixture)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 24
	const iterations = 20
	errCh := make(chan error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for worker := 0; worker < goroutines; worker++ {
		worker := worker
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				got, err := runDifferentialGoResult(fixture)
				if err != nil {
					errCh <- fmt.Errorf("worker %d iteration %d: %w", worker, iteration, err)
					return
				}
				if !bytes.Equal(want, got) {
					errCh <- fmt.Errorf("worker %d iteration %d produced nondeterministic output\nwant %s\n got %s", worker, iteration, want, got)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestLayoutMatchesD2Corpus(t *testing.T) {
	corpusDir := os.Getenv("DAGRO_D2_CORPUS")
	if corpusDir == "" {
		corpusDir = "testdata/differential/d2-corpus"
	}

	type corpusEntry struct {
		Input                  string   `json:"input"`
		Expected               string   `json:"expected"`
		ExpectedSHA256         string   `json:"expected_sha256"`
		ExpectedSource         string   `json:"expected_source"`
		OracleStatus           string   `json:"oracle_status"`
		OfficialNonfinitePaths []string `json:"official_nonfinite_paths"`
	}
	var manifest struct {
		Graphs                     map[string]corpusEntry `json:"graphs"`
		CompatibilityRootFixInputs []string               `json:"compatibility_root_fix_inputs"`
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
	manifestJSON, err := os.ReadFile(filepath.Join(corpusDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		t.Fatal(err)
	}
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
			input := decodeDifferentialInput(t, inputPath)
			gotJSON := runDifferentialGo(t, input)
			wantJSON, err := os.ReadFile(filepath.Join(corpusDir, entry.Expected))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(wantJSON)); got != entry.ExpectedSHA256 {
				t.Fatalf("expected output SHA-256 = %s, want %s", got, entry.ExpectedSHA256)
			}
			var want, got any
			if err := json.Unmarshal(wantJSON, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(gotJSON, &got); err != nil {
				t.Fatal(err)
			}
			compareJSON(t, "$", want, got)
			assertFiniteJSON(t, "$", got)
		})
	}
}

func decodeDifferentialInput(t *testing.T, path string) diffInput {
	t.Helper()
	var wire struct {
		Options map[string]bool `json:"options"`
		Graph   Attrs           `json:"graph"`
		Nodes   []diffNodeInput `json:"nodes"`
		Edges   []diffEdgeInput `json:"edges"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	return diffInput{
		Options: GraphOptions{
			Directed: wire.Options["directed"], Undirected: !wire.Options["directed"],
			Multigraph: wire.Options["multigraph"], Compound: wire.Options["compound"],
		},
		Graph: wire.Graph, Nodes: wire.Nodes, Edges: wire.Edges,
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
	deepIDs := []string{"10", "2", "alpha", "1", "beta", "01", "4294967294", "4294967295", "z", "3", "leaf"}
	for i, id := range deepIDs {
		node := diffNodeInput{ID: id, Attrs: Attrs{"width": 40.0 + float64(i), "height": 30.0 + float64(i)}}
		if i > 0 {
			node.Parent = strptr(deepIDs[i-1])
		}
		in.Nodes = append(in.Nodes, node)
	}
	out = append(out, namedFixture{"deep-compound-key-order", in})

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

func concurrentDummyIDFixture() diffInput {
	cluster := "cluster"
	return diffInput{
		Options: GraphOptions{Compound: true, Multigraph: true},
		Graph:   Attrs{"rankdir": "LR", "nodesep": 31.0, "edgesep": 17.0, "ranksep": 77.0},
		Nodes: []diffNodeInput{
			{ID: "cluster", Attrs: Attrs{}},
			{ID: "a", Attrs: Attrs{"width": 80.0, "height": 40.0}, Parent: &cluster},
			{ID: "b", Attrs: Attrs{"width": 60.0, "height": 50.0}, Parent: &cluster},
			{ID: "c", Attrs: Attrs{"width": 70.0, "height": 35.0}},
			{ID: "_d", Attrs: Attrs{"width": 10.0, "height": 10.0}},
			{ID: "_se", Attrs: Attrs{"width": 10.0, "height": 10.0}},
			{ID: "_root", Attrs: Attrs{"width": 10.0, "height": 10.0}},
			{ID: "_bt", Attrs: Attrs{"width": 10.0, "height": 10.0}},
			{ID: "_bb", Attrs: Attrs{"width": 10.0, "height": 10.0}},
		},
		Edges: []diffEdgeInput{
			{V: "a", W: "b", Name: strptr("rev1"), Attrs: Attrs{}},
			{V: "a", W: "b", Name: strptr("parallel-a"), Attrs: Attrs{"width": 30.0, "height": 10.0, "labelpos": "c"}},
			{V: "a", W: "b", Name: strptr("parallel-b"), Attrs: Attrs{"width": 20.0, "height": 15.0, "labelpos": "c"}},
			{V: "b", W: "c", Name: strptr("out"), Attrs: Attrs{}},
			{V: "c", W: "a", Name: strptr("back"), Attrs: Attrs{}},
			{V: "a", W: "a", Name: strptr("self"), Attrs: Attrs{"width": 20.0, "height": 10.0}},
		},
	}
}

func runDifferentialGo(t *testing.T, input diffInput) []byte {
	t.Helper()
	b, err := runDifferentialGoResult(input)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func runDifferentialGoResult(input diffInput) ([]byte, error) {
	g := NewGraph(input.Options).SetGraph(cloneAttrs(input.Graph))
	g.SetDefaultNodeLabel(func(string) any { return Attrs{} })
	g.SetDefaultEdgeLabel(func(string, string, *string) any { return Attrs{} })
	for _, node := range input.Nodes {
		g.SetNode(node.ID, cloneAttrs(node.Attrs))
	}
	for _, node := range input.Nodes {
		if node.Parent != nil {
			if err := g.SetParent(node.ID, *node.Parent); err != nil {
				return nil, err
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
		return nil, err
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
		return nil, err
	}
	return b, nil
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

func countJSONNulls(value any) int {
	switch value := value.(type) {
	case nil:
		return 1
	case []any:
		count := 0
		for _, child := range value {
			count += countJSONNulls(child)
		}
		return count
	case map[string]any:
		count := 0
		for _, child := range value {
			count += countJSONNulls(child)
		}
		return count
	default:
		return 0
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

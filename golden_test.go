package dagro

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The golden tests pin LayoutTyped's results on a broad corpus: the D2
// corpus, D2-shaped random inputs and random inputs with every option and
// unusual values. testdata/golden/<set>.txt has one line per input:
//
//	<index> <kind> <draws> <hash>
//
// kind is "ok" (a layout), "error" (an error from the engine), "unsupported"
// (an input the engine does not lay out) or "panic" (the engine failed);
// draws is how many values the layout took from the process-wide dummy-name
// counter; hash is the first 16 hex digits of the SHA-256 of the layout's
// exact bits (goldenDump) or of the error text, and "-" for the last two
// kinds.
//
// The results were recorded with dagro v0.3.1's typed engine, which the
// Graph engine and dagre.js checked then, by running its steps directly (an
// "unsupported" input then took the Graph engine, and a panic was recovered
// into it or passed through to the caller). go test -run TestGolden
// -update-golden rewrites the files from the current code.

var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden from the current LayoutTyped")

const goldenCounterStart = 1000

type goldenSet struct {
	name   string
	inputs func(t *testing.T) []LayoutInput
}

func goldenRandom(seed int64, features string, n, maxNodes int) func(*testing.T) []LayoutInput {
	return func(*testing.T) []LayoutInput {
		z := newInputGen(seed, features)
		out := make([]LayoutInput, n)
		for i := range out {
			out[i] = z.input(maxNodes)
		}
		return out
	}
}

func goldenSets() []goldenSet {
	return []goldenSet{
		{"d2corpus", func(t *testing.T) []LayoutInput {
			manifest := readD2CorpusManifest(t, d2CorpusDir)
			ids := make([]string, 0, len(manifest.Graphs))
			for id := range manifest.Graphs {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			out := make([]LayoutInput, len(ids))
			for i, id := range ids {
				out[i] = readD2CorpusInput(t, filepath.Join(d2CorpusDir, manifest.Graphs[id].Input))
			}
			return out
		}},
		{"d2profile", func(*testing.T) []LayoutInput {
			rng := rand.New(rand.NewSource(20261125))
			out := make([]LayoutInput, 3000)
			for i := range out {
				out[i] = d2ProfileInput(rng, i)
			}
			return out
		}},
		{"plain", goldenRandom(1, "", 2000, 12)},
		{"large", goldenRandom(2, "", 300, 40)},
		{"weirdnums", goldenRandom(3, "weirdnums", 1000, 12)},
		{"weirdlabels", goldenRandom(4, "weirdlabels,oddnames", 1000, 12)},
		{"fractional", goldenRandom(5, "fracminlen,fracweight", 1000, 12)},
		{"minlen0", goldenRandom(6, "minlen0", 1000, 12)},
		{"accepted", goldenRandom(7, "accepted", 3000, 16)},
		{"accepted-weird", goldenRandom(8, "accepted,weirdnums,fracminlen,fracweight", 1500, 16)},
	}
}

// goldenDump writes out's exact bits.
func goldenDump(out *LayoutOutput) string {
	var b strings.Builder
	f := func(x float64) { fmt.Fprintf(&b, " %016x", math.Float64bits(x)) }
	b.WriteString("graph")
	f(out.Width)
	f(out.Height)
	for i, n := range out.Nodes {
		fmt.Fprintf(&b, "\nnode %d", i)
		f(n.X)
		f(n.Y)
		f(n.Width)
		f(n.Height)
	}
	for i, e := range out.Edges {
		fmt.Fprintf(&b, "\nedge %d points %d", i, len(e.Points))
		for _, p := range e.Points {
			f(p.X)
			f(p.Y)
		}
		if e.HasXY {
			b.WriteString(" xy")
			f(e.X)
			f(e.Y)
		}
	}
	return b.String()
}

func goldenHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:8])
}

// goldenLine returns the golden line of input i and the text it hashes.
func goldenLine(i int, in *LayoutInput) (line, text string) {
	uniqueIDCounter.Store(goldenCounterStart)
	kind, text := goldenOutcome(in)
	draws := uniqueIDCounter.Load() - goldenCounterStart
	hash := "-"
	if kind == "ok" || kind == "error" {
		hash = goldenHash(text)
	}
	return fmt.Sprintf("%d %s %d %s", i, kind, draws, hash), text
}

// goldenOutcome lays out in with LayoutTyped and classifies the result.
func goldenOutcome(in *LayoutInput) (kind, text string) {
	var out LayoutOutput
	err := LayoutTyped(in, &out)
	var perr *PanicError
	switch {
	case err == nil:
		return "ok", goldenDump(&out)
	case errors.Is(err, ErrUnsupportedInput):
		return "unsupported", err.Error()
	case errors.As(err, &perr):
		return "panic", err.Error()
	default:
		return "error", err.Error()
	}
}

// TestGolden checks LayoutTyped against testdata/golden. It runs serially
// (it resets the process-wide counter for each input).
func TestGolden(t *testing.T) {
	for _, set := range goldenSets() {
		inputs := set.inputs(t)
		path := filepath.Join("testdata", "golden", set.name+".txt")
		kinds := map[string]int{}
		if *updateGolden {
			var b strings.Builder
			for i := range inputs {
				line, _ := goldenLine(i, &inputs[i])
				kinds[strings.Fields(line)[1]]++
				b.WriteString(line + "\n")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: wrote %d inputs %v", set.name, len(inputs), kinds)
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			want = append(want, sc.Text())
		}
		f.Close()
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
		if len(want) != len(inputs) {
			t.Fatalf("%s: %d golden lines for %d inputs", set.name, len(want), len(inputs))
		}
		failed := 0
		for i := range inputs {
			line, text := goldenLine(i, &inputs[i])
			kinds[strings.Fields(line)[1]]++
			if line != want[i] {
				failed++
				if failed <= 5 {
					t.Errorf("%s input %d: got %q, golden %q\n%s", set.name, i, line, want[i], text)
				}
			}
		}
		if failed > 0 {
			t.Errorf("%s: %d of %d inputs differ from the golden results", set.name, failed, len(inputs))
		}
		t.Logf("%s: %d inputs %v", set.name, len(inputs), kinds)
	}
}

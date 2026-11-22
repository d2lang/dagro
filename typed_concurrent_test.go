package dagro

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// TestConcurrentLayouts calls Layout and LayoutTyped from 4 goroutines at
// once (sharing the pooled engine states and the process-wide counter) on
// 40 graphs, 3 rounds: every result must equal the sequential one. Only
// graphs whose results do not depend on the counter value (the same result
// for two very different counter starts) are used.
func TestConcurrentLayouts(t *testing.T) {
	runConcurrentLayouts(t, 40, 3, 4)
}

func runConcurrentLayouts(t *testing.T, n, rounds, workers int) {
	z := newSpecGenerator(int64(envInt("DAGRO_DIFF_SEED", 7)), os.Getenv("DAGRO_DIFF_FEATURES"))
	tz := inputGenerator{z}
	type item struct {
		spec   graphSpec
		typed  LayoutInput
		isSpec bool
		want   string
	}
	var items []item
	for len(items) < n {
		if z.p(2) {
			s := z.spec(envInt("DAGRO_DIFF_MAXNODES", 20))
			a := runSpecLayout(s, 1000)
			b := runSpecLayout(s, 7000000)
			o := runSpecReference(s, 1000)
			if a.hung || a.panicked != "" || diffOutcomes(a, b) != "" || diffOutcomes(o, a) != "" {
				continue
			}
			items = append(items, item{spec: s, isSpec: true, want: a.dump + a.err})
		} else {
			in := tz.input(envInt("DAGRO_DIFF_MAXNODES", 20))
			_, a := runTypedPair(in, 1000)
			_, b := runTypedPair(in, 7000000)
			if a.hung || a.panicked != "" || a.dump != b.dump || a.err != b.err {
				continue
			}
			items = append(items, item{typed: in, want: a.dump + a.err})
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	fails := 0
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				for k := range items {
					it := &items[(k+w*7+r*13)%len(items)]
					var got string
					if it.isSpec {
						g, _ := buildFromSpec(it.spec)
						err := Layout(g)
						got = dumpGraph(g)
						if err != nil {
							got += err.Error()
						}
					} else {
						var out LayoutOutput
						in := it.typed
						if err := LayoutTyped(&in, &out); err != nil {
							got = err.Error()
						} else {
							got = dumpTypedOutput(&out)
						}
					}
					if got != it.want {
						mu.Lock()
						fails++
						if fails <= 3 {
							t.Errorf("worker %d round %d: concurrent result differs\nwant %s\ngot  %s", w, r, it.want, got)
						}
						mu.Unlock()
					}
				}
			}
		}(w)
	}
	wg.Wait()
	t.Logf("%d graphs x %d rounds x %d workers, %d differences", len(items), rounds, workers, fails)
	_ = fmt.Sprint
}

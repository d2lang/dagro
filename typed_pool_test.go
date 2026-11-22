package dagro

import (
	"runtime"
	"runtime/debug"
	"testing"
)

// poolOversized comes from the build tags of typed_retain_gc.go (!tinygo:
// true) and typed_retain_tinygo.go (tinygo: false): a state above
// lstateKeepMaxBytes goes to lstatePool only where the garbage collector
// empties that pool. The test pins the constant to the compiler that built
// it (TinyGo 0.44 reports runtime.Compiler "tinygo"), so that setting it to
// false under gc, or swapping the two files' build tags, fails a test.
func TestPoolOversizedFollowsCompiler(t *testing.T) {
	if want := runtime.Compiler != "tinygo"; poolOversized != want {
		t.Fatalf("poolOversized is %v under compiler %q, want %v", poolOversized, runtime.Compiler, want)
	}
}

// A state above lstateKeepMaxBytes goes to lstatePool under the gc
// toolchain, so a large graph laid out again before the garbage collector
// runs reuses its storage. Dropping the state after every call cost
// 9.8 / 28 / 52 MB allocated per call for 300 / 800 / 1500 nodes
// against 0.02 / 0.07 / 0.12 MB. It skips by compiler, not by poolOversized,
// so a gc build that stops pooling oversized states fails here.
func TestOversizedStateReusedUnderGC(t *testing.T) {
	if runtime.Compiler == "tinygo" {
		t.Skip("oversized states are dropped under TinyGo")
	}
	if raceEnabled {
		t.Skip("the race detector's sync.Pool drops Puts at random")
	}
	// Empty lstatePool of states earlier tests left in it (the first
	// collection moves them to the pool's victim cache, the second drops
	// them), then keep the collector from running in between: the Get below
	// must find the state this layout put back.
	runtime.GC()
	runtime.GC()
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	in := bigInput(1500, 3)
	var out LayoutOutput
	if err := LayoutTyped(&in, &out); err != nil {
		t.Fatal(err)
	}
	s := lstatePool.Get().(*lstate)
	if s.retainedBytes() <= lstateKeepMaxBytes {
		t.Fatalf("test state too small: %d bytes", s.retainedBytes())
	}
	lstatePool.Put(s)
	allocs := testing.AllocsPerRun(5, func() {
		if err := LayoutTyped(&in, &out); err != nil {
			t.Fatal(err)
		}
	})
	// The output's point array and a few small buffers; a fresh state
	// allocates thousands of objects.
	if allocs > 500 {
		t.Fatalf("%.0f allocations per layout of a pooled large state", allocs)
	}
	t.Logf("%.0f allocations per layout", allocs)
}

// In a process that collects garbage between layouts (D2 renders between
// them), a large state is not reused, so a fresh state's allocation is what
// a large graph costs: the per-node and per-edge arrays are reserved once the
// ranks are known (reserveDummies) and otherwise double (appendGrow), so a
// fresh state allocates about 1.5 times what it keeps; with append's 1.25x
// steps it allocated 2.6 times (52.6 MB for this 1500-node graph's 20 MB).
// Over 286 large generated diagrams: 425 garbage collections and 85.5-86.5 G
// instructions before, 263 and 77.5-78.0 G after (keeping every state across
// collections: 216 and 75.6-75.8 G).
func TestFreshStateAllocation(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation sizes differ under -race")
	}
	in := bigInput(1500, 3)
	var before, after runtime.MemStats
	s := new(lstate)
	runtime.ReadMemStats(&before)
	if !s.buildTyped(&in) {
		t.Fatal("declined")
	}
	if _, ok := s.runGuarded(s.run); !ok {
		t.Fatal("panicked")
	}
	runtime.ReadMemStats(&after)
	allocated, kept := after.TotalAlloc-before.TotalAlloc, uint64(s.retainedBytes())
	t.Logf("fresh state: %.1f MB allocated, %.1f MB kept", float64(allocated)/1e6, float64(kept)/1e6)
	if allocated > kept*7/4 {
		t.Errorf("a fresh state allocated %d bytes for %d kept (more than 1.75 times)", allocated, kept)
	}
}

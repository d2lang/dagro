package dagro

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"testing"
)

// Retention of the kept engine state.

// reachableBytes walks everything reachable from s with reflect and
// returns the union of the memory of every slice backing array and pointed-to
// struct (aliasing slices counted once). Strings and map contents are not
// included: the strings belong to the caller (release drops them) and the
// maps are estimated by retainedBytes from the buffers that bound them.
func reachableBytes(s *lstate) int {
	type span struct{ lo, hi uintptr }
	var spans []span
	seen := map[uintptr]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return
			}
			p := v.Pointer()
			if seen[p] {
				return
			}
			seen[p] = true
			spans = append(spans, span{p, p + v.Type().Elem().Size()})
			walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Slice:
			if v.IsNil() || v.Cap() == 0 {
				return
			}
			p := v.Pointer()
			size := v.Type().Elem().Size()
			spans = append(spans, span{p, p + uintptr(v.Cap())*size})
			switch v.Type().Elem().Kind() {
			case reflect.Struct, reflect.Slice, reflect.Pointer, reflect.Array, reflect.Interface:
				full := v.Slice(0, v.Cap())
				for i := 0; i < full.Len(); i++ {
					walk(full.Index(i))
				}
			}
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		}
	}
	walk(reflect.ValueOf(s))
	sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
	total := 0
	var cur span
	for i, sp := range spans {
		if i == 0 || sp.lo >= cur.hi {
			total += int(cur.hi - cur.lo)
			cur = sp
			continue
		}
		if sp.hi > cur.hi {
			cur.hi = sp.hi
		}
	}
	return total + int(cur.hi-cur.lo)
}

func mapEstimate(s *lstate) int {
	return cap(s.names.str)*szStrMapEntry + cap(s.el)*szStrMapEntry + cap(s.o.cgEdges)*szU64SetEntry
}

// bigInput is a layered graph with long edges, compound nodes and
// self-edges, of about n nodes.
func bigInput(n, span int) LayoutInput {
	in := DefaultLayoutInput()
	for i := 0; i < n; i++ {
		parent := -1
		if i%10 != 0 && i > 0 {
			parent = i - i%10
		}
		in.Nodes = append(in.Nodes, LayoutNode{ID: "n" + strconv.Itoa(i), Parent: parent, Width: float64(10 + i%7), Height: 20})
	}
	for i := 1; i < n; i++ {
		if i%10 == 0 || (i-1)%10 == 0 {
			continue
		}
		e := NewLayoutEdge(i-1, i)
		e.Minlen = float64(1 + i%span)
		e.Width, e.Height = float64(i%3), float64(i%2)
		in.Edges = append(in.Edges, e)
		if i%17 == 0 {
			in.Edges = append(in.Edges, NewLayoutEdge(i, i))
		}
	}
	return in
}

// retainedBytes counts every buffer the state keeps: after layouts of many
// different graphs (each released as putLState does), it is at least the
// memory a reflect walk finds reachable from the state, and not much more.
func TestRetainedBytesCountsEveryBuffer(t *testing.T) {
	s := new(lstate)
	check := func(label string) {
		t.Helper()
		got := s.retainedBytes() - mapEstimate(s)
		want := reachableBytes(s)
		// storageBytes allows for earlier versions of expVS and resVS that
		// stale order entries may keep reachable.
		allowance := 0
		for _, L := range s.o.levels {
			allowance += 4 * 4 * (cap(L.expVS) + cap(L.resVS))
		}
		if got < want || got > want+allowance+want/50+4096 {
			t.Fatalf("%s: retainedBytes without maps %d, reachable %d, stale allowance %d", label, got, want, allowance)
		}
	}
	run := func(label string, layout func() bool) {
		t.Helper()
		if !layout() {
			s = new(lstate) // a failed call drops its state
			return
		}
		s.release()
		check(label)
	}
	for i, in := range []LayoutInput{bigInput(300, 5), bigInput(60, 30), bigInput(900, 2)} {
		in := in
		run(fmt.Sprint("big ", i), func() bool {
			var out LayoutOutput
			if s.buildTyped(&in) != nil {
				return false
			}
			if err := s.runGuarded(&in, &out); err != nil {
				return false
			}
			return true
		})
	}
	for _, features := range []string{"", "fracminlen", "weirdlabels,oddnames", "minlen0"} {
		z := newInputGen(17, features)
		for i := 0; i < 120; i++ {
			in := z.input(30)
			run(fmt.Sprintf("%q %d", features, i), func() bool {
				var out LayoutOutput
				return s.buildTyped(&in) == nil && s.runGuarded(&in, &out) == nil
			})
		}
	}
}

func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// After layouts of any size, the state kept across garbage collections holds
// at most lstateKeepMaxBytes (an earlier version kept 30 MB after one layout
// of 6000 nodes and 10000 edges).
func TestKeptStateIsBounded(t *testing.T) {
	for _, size := range []struct{ nodes, edges, minlen int }{{2000, 3000, 1}, {6000, 10000, 1}, {200, 200, 60}} {
		lstateCache.Store(nil)
		before := liveHeap()
		func() {
			in := DefaultLayoutInput()
			for i := 0; i < size.nodes; i++ {
				in.Nodes = append(in.Nodes, LayoutNode{ID: strconv.Itoa(i), Parent: -1, Width: 30, Height: 20})
			}
			for i := 0; i < size.edges; i++ {
				e := NewLayoutEdge(i%size.nodes, (i*7+1)%size.nodes)
				e.Minlen = float64(size.minlen)
				// Edges with the same endpoints need different names.
				e.Name, e.HasName = strconv.Itoa(i), true
				in.Edges = append(in.Edges, e)
			}
			var out LayoutOutput
			if err := LayoutTyped(&in, &out); err != nil {
				t.Fatal(err)
			}
		}()
		retained := int64(liveHeap()) - int64(before)
		kept := 0
		if s := lstateCache.Load(); s != nil {
			kept = s.retainedBytes()
		}
		t.Logf("nodes %d edges %d minlen %d: heap retained %.2f MB, kept state %.2f MB", size.nodes, size.edges, size.minlen,
			float64(retained)/1e6, float64(kept)/1e6)
		if kept > lstateKeepMaxBytes || retained > lstateKeepMaxBytes+1<<20 {
			t.Errorf("kept %d bytes, heap retained %d bytes; limit %d", kept, retained, lstateKeepMaxBytes)
		}
	}
}

// A state above the limit is never cached (lstateCache survives garbage
// collections). Under the gc toolchain it goes to lstatePool, which the
// collector empties (so that large graphs keep their speed);
// under TinyGo, whose sync.Pool would keep it for the life of the program, it
// is dropped (poolOversized).
func TestOversizedStateNotKept(t *testing.T) {
	big := new(lstate)
	in := bigInput(3000, 40)
	if big.buildTyped(&in) != nil {
		t.Fatal("declined")
	}
	if err := big.run(); err != nil {
		t.Fatal(err)
	}
	big.release()
	if b := big.retainedBytes(); b <= lstateKeepMaxBytes {
		t.Fatalf("test state too small: %d bytes", b)
	}
	small := new(lstate)
	lstateCache.Store(small)
	putLState(big)
	if lstateCache.Load() != small {
		t.Fatal("cache changed")
	}
	pooled := func() bool {
		for i := 0; i < 8; i++ {
			if lstatePool.Get().(*lstate) == big {
				return true
			}
		}
		return false
	}
	// The race detector's sync.Pool drops a quarter of its Puts at random.
	if found := pooled(); found != poolOversized && !(poolOversized && raceEnabled) {
		t.Fatalf("oversized state pooled: %v, want %v", found, poolOversized)
	}
	lstateCache.Store(nil)
	putLState(big)
	if lstateCache.Load() == big {
		t.Fatal("oversized state was cached")
	}
	if found := pooled(); found != poolOversized && !(poolOversized && raceEnabled) {
		t.Fatalf("oversized state pooled: %v, want %v", found, poolOversized)
	}
}

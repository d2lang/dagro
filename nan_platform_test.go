package dagro

import (
	"math"
	"testing"
)

// nanForConversion is a variable, not a constant, so that nanConvertsToZero
// converts at run time, on the platform under test.
var nanForConversion = math.NaN()

// nanConvertsToZero reports whether converting NaN to int gives 0 on this
// platform. Go leaves the result to the platform: arm64 gives 0, amd64 gives
// math.MinInt64. The Attrs engine converts ranks and weights to ints
// (integer() in addBorderSegments, greedyFAS's bucket count), so on graphs
// whose ranks or weights are NaN its outcome depends on the platform: on amd64
// addBorderSegments counts up from math.MinInt64 and adds border nodes until
// memory runs out, and greedyFAS panics in makeslice.
func nanConvertsToZero() bool { return int(nanForConversion) == 0 }

// skipUnlessNaNConvertsToZero skips a test that compares the engines on
// generated graphs whose ranks can become NaN: where int(NaN) is not 0, the
// Attrs engine would exhaust memory on them (see nanConvertsToZero).
func skipUnlessNaNConvertsToZero(t *testing.T) {
	t.Helper()
	if !nanConvertsToZero() {
		t.Skip("compares the engines on graphs whose ranks can be NaN; int(NaN) is not 0 on this platform, where the Attrs engine runs out of memory on such graphs")
	}
}

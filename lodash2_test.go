package dagro

import (
	"math"
	"testing"
)

// TestLodash2MatchesVariadic compares lodashMin2/lodashMax2 with
// lodashMinJS/lodashMaxJS on all pairs of special and ordinary values,
// bit for bit.
func TestLodash2MatchesVariadic(t *testing.T) {
	nan2 := math.Float64frombits(0x7ff8000000000123)
	values := []float64{math.NaN(), nan2, math.Inf(1), math.Inf(-1), 0, math.Copysign(0, -1), 1, -1, 2.5, -2.5,
		math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64, 1e300, -3}
	for _, a := range values {
		for _, b := range values {
			if got, want := lodashMin2(a, b), lodashMinJS(a, b); math.Float64bits(got) != math.Float64bits(want) {
				t.Errorf("lodashMin2(%v, %v) = %v, want %v", a, b, got, want)
			}
			if got, want := lodashMax2(a, b), lodashMaxJS(a, b); math.Float64bits(got) != math.Float64bits(want) {
				t.Errorf("lodashMax2(%v, %v) = %v, want %v", a, b, got, want)
			}
		}
	}
}

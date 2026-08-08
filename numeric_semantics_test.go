package dagro

import (
	"math"
	"testing"
)

func TestJavaScriptMathExtremaPropagateNaN(t *testing.T) {
	if got := mathMinJS(1, math.NaN()); !math.IsNaN(got) {
		t.Fatalf("mathMinJS(1, NaN) = %v, want NaN", got)
	}
	if got := mathMaxJS(math.NaN(), 1); !math.IsNaN(got) {
		t.Fatalf("mathMaxJS(NaN, 1) = %v, want NaN", got)
	}
	if got := lodashMinJS(math.NaN(), 1); got != 1 {
		t.Fatalf("lodashMinJS(NaN, 1) = %v, want 1", got)
	}
	if got := lodashMaxJS(1, math.NaN()); got != 1 {
		t.Fatalf("lodashMaxJS(1, NaN) = %v, want 1", got)
	}
	if got := lodashMaxJS(math.NaN()); !math.IsNaN(got) {
		t.Fatalf("lodashMaxJS(NaN) = %v, want undefined-like NaN", got)
	}
}

func TestJavaScriptNumberString(t *testing.T) {
	for _, test := range []struct {
		value float64
		want  string
	}{
		{math.Copysign(0, -1), "0"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
		{math.NaN(), "NaN"},
		{1e-6, "0.000001"},
		{1e-7, "1e-7"},
		{1e21, "1e+21"},
	} {
		if got := jsNumberString(test.value); got != test.want {
			t.Errorf("jsNumberString(%v) = %q, want %q", test.value, got, test.want)
		}
	}
}

package dagro

import (
	"math"
	"testing"
)

func TestNumberMatchesJavaScriptCoercion(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   any
		want float64
		nan  bool
	}{
		{name: "null", in: nil, want: 0},
		{name: "empty string", in: " \t", want: 0},
		{name: "binary", in: "0b101", want: 5},
		{name: "octal", in: "0O17", want: 15},
		{name: "hexadecimal", in: "0x20", want: 32},
		{name: "positive infinity", in: "+Infinity", want: math.Inf(1)},
		{name: "decimal overflow", in: "1e400", want: math.Inf(1)},
		{name: "decimal underflow", in: "1e-4000", want: 0},
		{name: "Go infinity spelling", in: "+Inf", nan: true},
		{name: "bad binary", in: "0b12", nan: true},
		{name: "signed hex", in: "+0x20", nan: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := number(tt.in)
			if tt.nan {
				if !math.IsNaN(got) {
					t.Fatalf("number(%#v) = %v, want NaN", tt.in, got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("number(%#v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNumDistinguishesUndefinedFromNull(t *testing.T) {
	attrs := Attrs{"null": nil}
	if got := num(attrs, "missing"); !math.IsNaN(got) {
		t.Fatalf("missing property = %v, want NaN", got)
	}
	if got := num(attrs, "null"); got != 0 {
		t.Fatalf("null property = %v, want 0", got)
	}
}

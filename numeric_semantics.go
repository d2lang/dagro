package dagro

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func jsTruthyNumber(value float64) bool {
	return value != 0 && !math.IsNaN(value)
}

// mathMinJS and mathMaxJS model JavaScript's Math.min and Math.max. Unlike
// Lodash's extrema helpers, Math propagates NaN if any argument is NaN.
func mathMinJS(values ...float64) float64 {
	if len(values) == 0 {
		return math.Inf(1)
	}
	result := values[0]
	for _, value := range values[1:] {
		if math.IsNaN(result) || math.IsNaN(value) {
			return math.NaN()
		}
		result = math.Min(result, value)
	}
	return result
}

func mathMaxJS(values ...float64) float64 {
	if len(values) == 0 {
		return math.Inf(-1)
	}
	result := values[0]
	for _, value := range values[1:] {
		if math.IsNaN(result) || math.IsNaN(value) {
			return math.NaN()
		}
		result = math.Max(result, value)
	}
	return result
}

// maxJS2 and minJS2 are mathMaxJS(a, b) and mathMinJS(a, b) without the
// variadic slice. Past the NaN test the builtin max and min equal math.Max
// and math.Min (also for infinities and signed zeros: max(-0, +0) = +0,
// min(-0, +0) = -0) and compile to one instruction instead of a call.
func maxJS2(a, b float64) float64 {
	if a != a || b != b {
		return math.NaN()
	}
	return max(a, b)
}

func minJS2(a, b float64) float64 {
	if a != a || b != b {
		return math.NaN()
	}
	return min(a, b)
}

// lodashMinJS and lodashMaxJS model the Lodash 4 extrema helpers used by
// Dagre. Lodash skips undefined and NaN values rather than propagating NaN.
// A NaN return represents Lodash's undefined result when no numeric value is
// present.
func lodashMinJS(values ...float64) float64 {
	result := math.NaN()
	found := false
	for _, value := range values {
		if math.IsNaN(value) {
			continue
		}
		if !found || value < result {
			result = value
			found = true
		}
	}
	return result
}

func lodashMaxJS(values ...float64) float64 {
	result := math.NaN()
	found := false
	for _, value := range values {
		if math.IsNaN(value) {
			continue
		}
		if !found || value > result {
			result = value
			found = true
		}
	}
	return result
}

// lodashMin2 and lodashMax2 are lodashMinJS(a, b) and lodashMaxJS(a, b)
// without the variadic slice: NaN arguments are skipped, a is kept on ties,
// and two NaNs give math.NaN().
func lodashMin2(a, b float64) float64 {
	if a != a {
		if b != b {
			return math.NaN()
		}
		return b
	}
	if b < a {
		return b
	}
	return a
}

func lodashMax2(a, b float64) float64 {
	if a != a {
		if b != b {
			return math.NaN()
		}
		return b
	}
	if b > a {
		return b
	}
	return a
}

// jsConcatString covers the primitive values Dagre concatenates into debug
// identifiers. In particular, JavaScript distinguishes an absent property
// (handled by the caller as "undefined") from an explicit null value.
func jsConcatString(value any) string {
	switch value := value.(type) {
	case nil:
		return "null"
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return jsNumberString(value)
	case float32:
		return jsNumberString(float64(value))
	case int:
		return strconv.Itoa(value)
	case int8:
		return strconv.FormatInt(int64(value), 10)
	case int16:
		return strconv.FormatInt(int64(value), 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case int64:
		return strconv.FormatInt(value, 10)
	case uint:
		return strconv.FormatUint(uint64(value), 10)
	case uint8:
		return strconv.FormatUint(uint64(value), 10)
	case uint16:
		return strconv.FormatUint(uint64(value), 10)
	case uint32:
		return strconv.FormatUint(uint64(value), 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	default:
		return fmt.Sprint(value)
	}
}

func jsNumberString(value float64) string {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	case value == 0:
		return "0"
	}

	abs := math.Abs(value)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}

	text := strconv.FormatFloat(value, 'e', -1, 64)
	mantissa, exponent, ok := strings.Cut(text, "e")
	if !ok {
		return text
	}
	sign := ""
	if strings.HasPrefix(exponent, "+") || strings.HasPrefix(exponent, "-") {
		sign, exponent = exponent[:1], exponent[1:]
	}
	exponent = strings.TrimLeft(exponent, "0")
	if exponent == "" {
		exponent = "0"
	}
	return mantissa + "e" + sign + exponent
}

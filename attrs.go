package dagro

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// Attrs is Dagro's equivalent of a JavaScript object used as a graph, node,
// or edge label by Dagre 0.8.5.
type Attrs map[string]any

// Point is a point in the coordinate system produced by Layout.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func asAttrs(v any) Attrs {
	if v == nil {
		return nil
	}
	a, ok := v.(Attrs)
	if !ok {
		panic(fmt.Sprintf("dagro: expected Attrs, got %T", v))
	}
	return a
}

func has(a Attrs, key string) bool {
	_, ok := a[key]
	return ok
}

func number(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case nil:
		return 0
	case bool:
		if n {
			return 1
		}
		return 0
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0
		}
		if s == "Infinity" || s == "+Infinity" {
			return math.Inf(1)
		}
		if s == "-Infinity" {
			return math.Inf(-1)
		}
		for _, radix := range []struct {
			lower, upper string
			base         int
		}{
			{lower: "0b", upper: "0B", base: 2},
			{lower: "0o", upper: "0O", base: 8},
			{lower: "0x", upper: "0X", base: 16},
		} {
			if strings.HasPrefix(s, radix.lower) || strings.HasPrefix(s, radix.upper) {
				return parseRadixNumber(s[2:], radix.base)
			}
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return math.NaN()
		}
		// ParseFloat accepts spellings such as "Inf" that JavaScript Number
		// rejects. Exact "Infinity" spellings were handled above; an infinity
		// accompanied by ErrRange is instead a valid decimal overflow.
		if err == nil && math.IsInf(v, 0) {
			return math.NaN()
		}
		return v
	default:
		panic(fmt.Sprintf("dagro: expected number, got %T", v))
	}
}

func parseRadixNumber(digits string, base int) float64 {
	if digits == "" {
		return math.NaN()
	}
	for _, r := range digits {
		value := -1
		switch {
		case '0' <= r && r <= '9':
			value = int(r - '0')
		case 'a' <= r && r <= 'f':
			value = int(r-'a') + 10
		case 'A' <= r && r <= 'F':
			value = int(r-'A') + 10
		}
		if value < 0 || value >= base {
			return math.NaN()
		}
	}
	integer, ok := new(big.Int).SetString(digits, base)
	if !ok {
		return math.NaN()
	}
	value, _ := new(big.Float).SetInt(integer).Float64()
	return value
}

func num(a Attrs, key string) float64 {
	v, ok := a[key]
	if !ok {
		return math.NaN()
	}
	return number(v)
}

func integer(a Attrs, key string) int { return int(num(a, key)) }

func stringValue(a Attrs, key string) string {
	v, _ := a[key].(string)
	return v
}

func boolValue(a Attrs, key string) bool {
	v, _ := a[key].(bool)
	return v
}

func cloneAttrs(a Attrs) Attrs {
	if a == nil {
		return nil
	}
	out := make(Attrs, len(a))
	for k, v := range a {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case Attrs:
		return cloneAttrs(x)
	case []string:
		return append([]string(nil), x...)
	case []Point:
		return append([]Point(nil), x...)
	case []Edge:
		return append([]Edge(nil), x...)
	case []Attrs:
		out := make([]Attrs, len(x))
		for i := range x {
			out[i] = cloneAttrs(x[i])
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(x))
		for k, v := range x {
			out[k] = v
		}
		return out
	case map[string]float64:
		out := make(map[string]float64, len(x))
		for k, v := range x {
			out[k] = v
		}
		return out
	case map[int]string:
		out := make(map[int]string, len(x))
		for k, v := range x {
			out[k] = v
		}
		return out
	default:
		return v
	}
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	mid := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[mid]
	}
	return (copyValues[mid-1] + copyValues[mid]) / 2
}

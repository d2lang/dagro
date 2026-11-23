package dagro

import (
	"errors"
	"fmt"
	"strings"
)

// graphRankGroups also accepts arrays decoded by encoding/json into Attrs.
func graphRankGroups(attrs Attrs) ([][]string, error) {
	var value any
	found := false
	for k, v := range attrs {
		if !strings.EqualFold(k, "rankgroups") {
			continue
		}
		if found {
			return nil, errors.New("dagro: duplicate rankgroups attribute")
		}
		found, value = true, v
	}
	switch groups := value.(type) {
	case nil:
		return nil, nil
	case [][]string:
		return groups, nil
	case []any:
		result := make([][]string, len(groups))
		for i, row := range groups {
			switch ids := row.(type) {
			case nil:
				// A nil inner []string is encoded as JSON null.
			case []string:
				result[i] = ids
			case []any:
				for _, id := range ids {
					name, ok := id.(string)
					if !ok {
						return nil, fmt.Errorf("dagro: rank group %d contains a non-string node ID", i)
					}
					result[i] = append(result[i], name)
				}
			default:
				return nil, fmt.Errorf("dagro: rank group %d must be a list of node IDs", i)
			}
		}
		return result, nil
	default:
		return nil, errors.New("dagro: rankgroups must be a list of lists of node IDs")
	}
}

// Repeated mentions of a single node still need validation, but impose no
// equality and can use the ordinary layout and its compatibility fallback.
func hasRankEquality(groups [][]string) bool {
	for _, group := range groups {
		for _, id := range group {
			if id != group[0] {
				return true
			}
		}
	}
	return false
}

// The plain layout uses the Attrs engine for ambiguous attribute spellings.
// For constrained layouts do the same canonicalization here, as only the
// typed engine implements the constraints.
func layoutAttrLookup(a Attrs, keys []string, vals []any, found []bool, constrained bool) bool {
	if attrLookup(a, keys, vals, found) {
		return true
	}
	if !constrained {
		return false
	}
	return attrLookup(canonicalize(a), keys, vals, found)
}

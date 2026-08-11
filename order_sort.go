package dagro

import gosort "sort"

func sortOrderEntries(entries []orderEntry, biasRight bool) orderResult {
	return sortOrderEntriesWithReversedPairs(entries, nil, biasRight)
}

type reversedOrderPair struct {
	key   string
	entry orderEntry
}

func sortOrderEntriesWithReversedPairs(entries []orderEntry, reversedPairs []reversedOrderPair, biasRight bool) orderResult {
	sortable := make([]orderEntry, 0, len(entries))
	unsortable := make([]orderEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.HasBarycenter {
			sortable = append(sortable, entry)
		} else {
			unsortable = append(unsortable, entry)
		}
	}
	gosort.SliceStable(unsortable, func(i, j int) bool { return -unsortable[i].I < -unsortable[j].I })
	gosort.SliceStable(sortable, func(i, j int) bool {
		entryV := sortable[i]
		entryW := sortable[j]
		if entryV.Barycenter < entryW.Barycenter {
			return true
		}
		if entryV.Barycenter > entryW.Barycenter {
			return false
		}
		if !biasRight {
			return entryV.I < entryW.I
		}
		return entryV.I > entryW.I
	})

	// Keep a reversed edge dummy immediately after its parallel counterpart.
	// The pair slice preserves encounter order, including multiple reversed
	// dummies paired with the same forward dummy. Upstream's single object value
	// silently overwrites all but the last partner in that case.
	insertedAfter := map[string]int{}
	for _, pair := range reversedPairs {
		keyIndex := -1
		for i := range sortable {
			if len(sortable[i].VS) != 0 && sortable[i].VS[0] == pair.key {
				keyIndex = i
				break
			}
		}
		insertAt := keyIndex + 1 + insertedAfter[pair.key]
		sortable = append(sortable, orderEntry{})
		copy(sortable[insertAt+1:], sortable[insertAt:])
		sortable[insertAt] = pair.entry
		insertedAfter[pair.key]++
	}

	vs := make([]string, 0)
	sum := 0.0
	weight := 0.0
	vsIndex := 0
	vsIndex = consumeUnsortable(&vs, &unsortable, vsIndex)
	for _, entry := range sortable {
		vsIndex += len(entry.VS)
		vs = append(vs, entry.VS...)
		sum += entry.Barycenter * entry.Weight
		weight += entry.Weight
		vsIndex = consumeUnsortable(&vs, &unsortable, vsIndex)
	}

	result := orderResult{VS: vs}
	if jsTruthyNumber(weight) {
		result.Barycenter = sum / weight
		result.Weight = weight
		result.HasBarycenter = true
	}
	return result
}

func consumeUnsortable(vs *[]string, unsortable *[]orderEntry, index int) int {
	for len(*unsortable) != 0 {
		last := (*unsortable)[len(*unsortable)-1]
		if last.I > index {
			break
		}
		*unsortable = (*unsortable)[:len(*unsortable)-1]
		*vs = append(*vs, last.VS...)
		index++
	}
	return index
}

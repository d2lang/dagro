package dagro

type conflictEntry struct {
	orderEntry
	indegree int
	in       []*conflictEntry
	out      []*conflictEntry
	merged   bool
}

func resolveConflicts(entries []orderEntry, cg *Graph) []orderEntry {
	mappedEntries := make(map[string]*conflictEntry, len(entries))
	mappedOrder := newOrderedSet()
	for i, entry := range entries {
		tmp := &conflictEntry{orderEntry: orderEntry{
			V:             entry.V,
			VS:            []string{entry.V},
			I:             i,
			Barycenter:    entry.Barycenter,
			Weight:        entry.Weight,
			HasBarycenter: entry.HasBarycenter,
		}}
		mappedEntries[entry.V] = tmp
		mappedOrder.add(entry.V)
	}

	for _, e := range cg.Edges() {
		entryV, okV := mappedEntries[e.V]
		entryW, okW := mappedEntries[e.W]
		if okV && okW {
			entryW.indegree++
			entryV.out = append(entryV.out, entryW)
		}
	}

	sourceSet := make([]*conflictEntry, 0, len(mappedEntries))
	for _, v := range mappedOrder.values() {
		entry := mappedEntries[v]
		if entry.indegree == 0 {
			sourceSet = append(sourceSet, entry)
		}
	}

	return doResolveConflicts(sourceSet)
}

func doResolveConflicts(sourceSet []*conflictEntry) []orderEntry {
	entries := make([]*conflictEntry, 0)
	for len(sourceSet) > 0 {
		entry := sourceSet[len(sourceSet)-1]
		sourceSet = sourceSet[:len(sourceSet)-1]
		entries = append(entries, entry)

		for left, right := 0, len(entry.in)-1; left < right; left, right = left+1, right-1 {
			entry.in[left], entry.in[right] = entry.in[right], entry.in[left]
		}
		for _, incoming := range entry.in {
			if incoming.merged {
				continue
			}
			if !incoming.HasBarycenter || !entry.HasBarycenter || incoming.Barycenter >= entry.Barycenter {
				mergeEntries(entry, incoming)
			}
		}
		for _, outgoing := range entry.out {
			outgoing.in = append(outgoing.in, entry)
			outgoing.indegree--
			if outgoing.indegree == 0 {
				sourceSet = append(sourceSet, outgoing)
			}
		}
	}

	result := make([]orderEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.merged {
			continue
		}
		result = append(result, orderEntry{
			VS:            append([]string(nil), entry.VS...),
			I:             entry.I,
			Barycenter:    entry.Barycenter,
			Weight:        entry.Weight,
			HasBarycenter: entry.HasBarycenter,
		})
	}
	return result
}

func mergeEntries(target, source *conflictEntry) {
	sum := 0.0
	weight := 0.0
	if jsTruthyNumber(target.Weight) {
		sum += target.Barycenter * target.Weight
		weight += target.Weight
	}
	if jsTruthyNumber(source.Weight) {
		sum += source.Barycenter * source.Weight
		weight += source.Weight
	}

	target.VS = append(append([]string(nil), source.VS...), target.VS...)
	target.Barycenter = sum / weight
	target.Weight = weight
	target.HasBarycenter = true
	if source.I < target.I {
		target.I = source.I
	}
	source.merged = true
}

package ledger

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// mergedWritten merges two days of one account as a summary writes them,
// as mergedDays merges them: a field neither knows of, as a later version
// wrote it, isn't kept.
func mergedWritten(a, b json.RawMessage) (json.RawMessage, error) {
	var dayA, dayB AccountDay
	if err := json.Unmarshal(a, &dayA); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &dayB); err != nil {
		return nil, err
	}
	return json.Marshal(mergedDays(dayA, dayB))
}

// mergedDays merges two days of one account into one, as a summary renamed
// names them, a's account its: their work in every pool together, and in
// each pool, as mergedWork merges it; each window's highest use the higher
// of the two, and its rises summed; the resets and limits of both, in the
// order they came, each once, as limitsKnowing and resetsKnowing have them;
// the minutes summed; and the windows read before of both. A window's rises,
// resets and minutes that one of them never read, as its readings were under
// the other's id, are the other's.
func mergedDays(a, b AccountDay) AccountDay {
	return AccountDay{
		Account:        a.Account,
		Work:           mergedWork(a.Work, b.Work),
		Highest:        mergedMaps(a.Highest, b.Highest, func(x, y float64) float64 { return max(x, y) }),
		Rise:           mergedMaps(a.Rise, b.Rise, func(x, y float64) float64 { return x + y }),
		Resets:         resetsKnowing(a.Resets, b.Resets),
		Limits:         limitsKnowing(slices.Clone(a.Limits), b.Limits),
		MinutesAtCap:   summedMinutes(a.MinutesAtCap, b.MinutesAtCap),
		MinutesAtLimit: summedMinutes(a.MinutesAtLimit, b.MinutesAtLimit),
		ReadBefore:     unionOf(a.ReadBefore, b.ReadBefore),
	}.pooled(mergedBy(slices.Concat(a.poolDays(), b.poolDays()), PoolDay.key, mergedPool, strings.Compare))
}

// mergedWork merges two accounts' days' work into one: each model's requests
// and usage summed, as mergedModels has them; each session counted once, as
// mergedSessions says; and the moves summed.
func mergedWork(a, b Work) Work {
	sessions, ids := mergedSessions(a.Sessions, a.SessionIDs, b.Sessions, b.SessionIDs)
	return Work{Models: mergedModels(a.Models, b.Models), Sessions: sessions, SessionIDs: ids, MovedOn: a.MovedOn + b.MovedOn,
		MovedOff: a.MovedOff + b.MovedOff}
}

// mergedPool merges two accounts' days' work in one pool, as mergedWork
// merges it.
func mergedPool(a, b PoolDay) PoolDay {
	a.Work = mergedWork(a.Work, b.Work)
	return a
}

// mergedModels merges two accounts' days' models, those of one model and
// geo merged into one, in the order a summary keeps them: their counts and
// hours summed, the hours not known of both where either never counted
// them, their sessions counted once, as mergedSessions says, and their usage
// summed field for field, as a summary sums it.
func mergedModels(a, b []ModelDay) []ModelDay {
	return mergedBy(slices.Concat(a, b), ModelDay.key, mergedModel, byModel)
}

// mergedBy merges days, those of one key, as key gives each, merged into one
// as merge merges two, in the order of their keys, as compare orders them:
// nil where there are none.
func mergedBy[D any, K comparable](days []D, key func(D) K, merge func(a, b D) D, compare func(a, b K) int) []D {
	merged := make(map[K]D, len(days))
	for _, d := range days {
		if held, ok := merged[key(d)]; ok {
			d = merge(held, d)
		}
		merged[key(d)] = d
	}
	var all []D
	for _, k := range slices.SortedFunc(maps.Keys(merged), compare) {
		all = append(all, merged[k])
	}
	return all
}

// mergedModel merges two days of one model, as mergedModels says.
func mergedModel(a, b ModelDay) ModelDay {
	m := a
	m.Upstream, m.NoUsage, m.Unsent = a.Upstream+b.Upstream, a.NoUsage+b.NoUsage, a.Unsent+b.Unsent
	m.Checks, m.Counts = a.Checks+b.Checks, a.Counts+b.Counts
	m.Hours = nil
	if a.Hours != nil && b.Hours != nil {
		var hours [24]int
		for i := range hours {
			hours[i] = a.Hours[i] + b.Hours[i]
		}
		m.Hours = &hours
	}
	m.Sessions, m.SessionIDs = mergedSessions(a.Sessions, a.SessionIDs, b.Sessions, b.SessionIDs)
	usage := make(counts)
	usage.add(a.Usage)
	usage.add(b.Usage)
	m.Usage = nil
	if len(usage) > 0 {
		// A map of counts and maps of them always marshals.
		m.Usage, _ = json.Marshal(usage)
	}
	return m
}

// mergedSessions merges two counts of sessions and their ids: each session
// named counted once, and those neither names summed; or, where either never
// named its sessions, the counts summed, naming none.
func mergedSessions(countA int, idsA []string, countB int, idsB []string) (int, []string) {
	if idsA == nil || idsB == nil {
		return countA + countB, nil
	}
	named := make(sessions)
	for _, id := range slices.Concat(idsA, idsB) {
		named.add(id)
	}
	ids := named.ids()
	return len(ids) + unnamed(countA, idsA) + unnamed(countB, idsB), ids
}

// mergedMaps merges two maps of windows' figures by their keys, a key both
// hold given what merge makes of the two: nil where neither holds any.
func mergedMaps(a, b map[string]float64, merge func(x, y float64) float64) map[string]float64 {
	if a == nil && b == nil {
		return nil
	}
	merged := maps.Clone(a)
	if merged == nil {
		merged = make(map[string]float64, len(b))
	}
	for key, v := range b {
		if held, ok := merged[key]; ok {
			v = merge(held, v)
		}
		merged[key] = v
	}
	return merged
}

// summedMinutes sums two counts of minutes: the one there is where one of
// them was never counted, and nil where neither was.
func summedMinutes(a, b *int) *int {
	if a == nil || b == nil {
		return cmp.Or(a, b)
	}
	sum := *a + *b
	return &sum
}

// unionOf returns the strings of a and b, in order, each once: nil where
// neither holds any.
func unionOf(a, b []string) []string {
	both := slices.Concat(a, b)
	slices.Sort(both)
	return slices.Compact(both)
}

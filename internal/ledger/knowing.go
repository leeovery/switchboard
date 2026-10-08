package ledger

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// knowing returns s, a day summarised afresh, as it replaces held, the day's
// summary before, so it knows no less. Every count is held's where that's
// more, as a line can't be taken away while the day's are kept, so fewer
// means some were lost since, as to a damaged file: so are a model's
// requests and usage, and an account's sessions and moves; and the accounts
// and models held that the lines no longer give are kept. Which lines were
// lost can't be told, so where some of a model's were lost and as many or
// more filed since, its requests are the more of the two summaries', fewer
// than there were. Its lines are as many as the more of the two were made
// from, and as its requests where those are more, as the lines lost were
// held's. Each window's highest use is held's where that's higher, and
// held's limits, and the windows it read in the week before, are among its
// own, as the readings they came from may have been pruned since.
func (s Summary) knowing(held Summary) Summary {
	s.Accounts = knowingAll(s.Accounts, held.Accounts, func(a AccountDay) string { return a.Account }, AccountDay.knowing, strings.Compare)
	s.Lines = max(s.Lines, held.Lines, s.requests())
	return s
}

// knowing returns a, an account's day summarised afresh, knowing no less
// than held, its day as summarised before, as Summary.knowing says.
func (a AccountDay) knowing(held AccountDay) AccountDay {
	a.Sessions, a.MovedOn, a.MovedOff = max(a.Sessions, held.Sessions), max(a.MovedOn, held.MovedOn), max(a.MovedOff, held.MovedOff)
	for key, u := range held.Highest {
		if highest, ok := a.Highest[key]; !ok || u > highest {
			if a.Highest == nil {
				a.Highest = make(map[string]float64)
			}
			a.Highest[key] = u
		}
	}
	for _, l := range held.Limits {
		if !slices.ContainsFunc(a.Limits, l.same) {
			a.Limits = append(a.Limits, l)
		}
	}
	slices.SortStableFunc(a.Limits, func(x, y Limit) int { return x.At.Compare(y.At) })
	a.ReadBefore = append(a.ReadBefore, held.ReadBefore...)
	slices.Sort(a.ReadBefore)
	a.ReadBefore = slices.Compact(a.ReadBefore)
	a.Models = knowingAll(a.Models, held.Models, ModelDay.key, ModelDay.knowing, byModel)
	return a
}

// knowing returns m, a model's day on an account summarised afresh, knowing
// no less than held, its day as summarised before: each count held's where
// that's more, its usage's among them.
func (m ModelDay) knowing(held ModelDay) ModelDay {
	m.Upstream, m.NoUsage, m.Unsent = max(m.Upstream, held.Upstream), max(m.NoUsage, held.NoUsage), max(m.Unsent, held.Unsent)
	m.Checks, m.Counts, m.Sessions = max(m.Checks, held.Checks), max(m.Counts, held.Counts), max(m.Sessions, held.Sessions)
	usage, heldUsage := make(counts), make(counts)
	usage.add(m.Usage)
	heldUsage.add(held.Usage)
	if usage.atLeast(heldUsage); len(usage) > 0 {
		// A map of counts and maps of them always marshals.
		m.Usage, _ = json.Marshal(usage)
	}
	return m
}

// key names the model's day on its account.
func (m ModelDay) key() modelKey {
	return modelKey{model: m.Model, geo: m.InferenceGeo}
}

// same reports whether l and other are one limit: of one window, reached at
// one time, to reset at one time.
func (l Limit) same(other Limit) bool {
	return l.Window == other.Window && l.At.Equal(other.At) && l.ResetsAt.Equal(other.ResetsAt)
}

// knowingAll returns days, summarised afresh, as they replace held, those
// summarised before, in the order of their keys, as key gives each and
// compare orders them: each of days knowing no less than held's of its key,
// as knowing says, where held has one, and each of held's of a key none of
// days has, as it was.
func knowingAll[D any, K comparable](days, held []D, key func(D) K, knowing func(day, held D) D, compare func(a, b K) int) []D {
	byKey := make(map[K]D, len(days)+len(held))
	for _, d := range days {
		byKey[key(d)] = d
	}
	for _, h := range held {
		if d, ok := byKey[key(h)]; ok {
			byKey[key(h)] = knowing(d, h)
		} else {
			byKey[key(h)] = h
		}
	}
	all := make([]D, 0, len(byKey))
	for _, k := range slices.SortedFunc(maps.Keys(byKey), compare) {
		all = append(all, byKey[k])
	}
	return all
}

// atLeast raises each count c holds to the one other holds of its name,
// within the objects both hold alike, where that's more, adding those c
// lacks: usage summed field for field only grows with the lines summed.
func (c counts) atLeast(other counts) {
	for name, v := range other {
		switch v := v.(type) {
		case int64:
			if n, ok := c[name].(int64); !ok || v > n {
				c[name] = v
			}
		case counts:
			c.within(name).atLeast(v)
		}
	}
}

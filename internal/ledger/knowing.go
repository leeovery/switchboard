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
// held's. Each window's highest use, and its rise, is held's where that's
// higher, as are the minutes at its cap and at a limit; and held's limits,
// resets, session ids and the windows it read in the week before are among
// its own, as the readings they came from may have been pruned since.
func (s Summary) knowing(held Summary) Summary {
	s.Accounts = knowingAll(s.Accounts, held.Accounts, func(a AccountDay) string { return a.Account }, AccountDay.knowing, strings.Compare)
	s.Lines = max(s.Lines, held.Lines, s.requests())
	return s
}

// made returns s, a day summarised from its lines, made from as many as its
// files hold, as lines counts them, knowing no less than held, the day's
// summary before, where there's one, as knowing says.
func (s Summary) made(lines int, held *Summary) Summary {
	s.Lines = lines
	if held != nil {
		return s.knowing(*held)
	}
	return s
}

// knowing returns a, an account's day summarised afresh, knowing no less
// than held, its day as summarised before, as Summary.knowing says.
func (a AccountDay) knowing(held AccountDay) AccountDay {
	a.SessionIDs = together(a.SessionIDs, held.SessionIDs)
	a.Sessions = max(a.Sessions, held.Sessions, len(a.SessionIDs))
	a.MovedOn, a.MovedOff = max(a.MovedOn, held.MovedOn), max(a.MovedOff, held.MovedOff)
	a.Highest, a.Rise = higher(a.Highest, held.Highest), higher(a.Rise, held.Rise)
	for _, l := range held.Limits {
		if !slices.ContainsFunc(a.Limits, l.same) {
			a.Limits = append(a.Limits, l)
		}
	}
	slices.SortStableFunc(a.Limits, func(x, y Limit) int { return x.At.Compare(y.At) })
	a.Resets = resetsKnowing(a.Resets, held.Resets)
	a.MinutesAtCap, a.MinutesAtLimit = more(a.MinutesAtCap, held.MinutesAtCap), more(a.MinutesAtLimit, held.MinutesAtLimit)
	a.ReadBefore = together(a.ReadBefore, held.ReadBefore)
	a.Models = knowingAll(a.Models, held.Models, ModelDay.key, ModelDay.knowing, byModel)
	return a
}

// together returns the ids of ids and held together, in order, each once:
// nil only where both are.
func together(ids, held []string) []string {
	if ids == nil && held == nil {
		return nil
	}
	all := append(append(make([]string, 0, len(ids)+len(held)), ids...), held...)
	slices.Sort(all)
	return slices.Compact(all)
}

// higher returns each of uses, by its key, held's where that's higher, those
// of keys uses lacks among them: nil only where both are.
func higher(uses, held map[string]float64) map[string]float64 {
	if uses == nil && held == nil {
		return nil
	}
	all := maps.Clone(uses)
	if all == nil {
		all = make(map[string]float64, len(held))
	}
	for key, u := range held {
		if have, ok := all[key]; !ok || u > have {
			all[key] = u
		}
	}
	return all
}

// resetsKnowing returns resets, summarised afresh, with held's, those
// summarised before, among them, in the order they came: a reset of one
// window at one time is the same reset, its use before it the higher of the
// two. It's nil only where both are.
func resetsKnowing(resets, held []Reset) []Reset {
	if resets == nil && held == nil {
		return nil
	}
	resets = append([]Reset{}, resets...)
	for _, h := range held {
		if i := slices.IndexFunc(resets, h.same); i >= 0 {
			resets[i].Before = max(resets[i].Before, h.Before)
		} else {
			resets = append(resets, h)
		}
	}
	slices.SortFunc(resets, byWhen)
	return resets
}

// more returns the more of n and held, where they're known: nil only where
// both are.
func more(n, held *int) *int {
	if n == nil || held != nil && *held > *n {
		return held
	}
	return n
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

// same reports whether r and other are one reset: of one window, at one time.
func (r Reset) same(other Reset) bool {
	return r.Window == other.Window && r.At.Equal(other.At)
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

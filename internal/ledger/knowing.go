package ledger

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// knowing returns the day with the given date as its summary, made from the
// number of lines given, as it replaces held, the day's summary before, so
// it knows no less, as Summary.knowing says: but for the limits it reached
// that were read first, as readings says, which are its own only where held
// has them. The readings history prunes the readings of a day past keeping,
// whatever the ledger keeps, and a clock moved can leave it short of them,
// so a summary made again may find a limit's window without the reading
// before it that it had then: read first, it would be taken for a limit
// reached, though it held already.
func (t tally) knowing(date string, lines int, held Summary) Summary {
	s := t.summary(date, lines)
	for i, a := range s.Accounts {
		first := t[a.Account].firstRead
		s.Accounts[i].Limits = slices.DeleteFunc(a.Limits, func(l Limit) bool { return slices.ContainsFunc(first, l.same) })
	}
	return s.knowing(held)
}

// knowing returns s, a day summarised afresh, as it replaces held, the day's
// summary before, so it knows no less. Every count is held's where that's
// more, as a line can't be taken away while the day's are kept, so fewer
// means some were lost since, as to a damaged file: so are a model's
// requests and usage, an account's sessions and moves, and the lines it was
// made from; and the accounts and models held that the lines no longer give
// are kept. Each window's highest use is held's where that's higher, and
// held's limits are among its own, as the readings they came from may have
// been pruned since.
func (s Summary) knowing(held Summary) Summary {
	s.Lines = max(s.Lines, held.Lines)
	s.Accounts = knowingAll(s.Accounts, held.Accounts, func(a AccountDay) string { return a.Account }, AccountDay.knowing, strings.Compare)
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

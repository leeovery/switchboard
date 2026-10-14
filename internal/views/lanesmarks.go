package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/status"
)

// useOf is the account a's use of the view's window, as Use has it: as the
// moment has the window stand, live; stepped back, the highest the days shown
// read.
func (from LanesFrom) useOf(a status.AccountNow) Use {
	key := from.viewKey()
	if _, _, named := from.Timeline.Named(); named {
		return from.highest(a.ID, key)
	}
	w, ok := a.Window(key)
	if !ok {
		return Use{Tokenless: !a.TokenSet, Unread: a.TokenSet && !a.Read}
	}
	use := Use{Key: key}
	switch w.Kind {
	case status.WindowTokenless:
		use.Tokenless = true
	case status.WindowUnread:
		use.Unread = true
	default:
		use.Used, use.Read = w.Used, true
		use.Stale = w.Kind == status.WindowStale
		if use.Stale {
			use.ReadAt = w.At
		}
	}
	return use
}

// highest is the use of the window with the given key by the account with
// the given id the days a timeline stepped back to shows read, as their
// summaries have it: the highest any of them read, not read where none did.
func (from LanesFrom) highest(id, key string) Use {
	first, last, _ := from.Timeline.Named()
	use := Use{Key: key, Highest: true}
	for _, s := range from.Days {
		if day, ok := dayfile.Civil(s.Day); !ok || day.Before(first) || day.After(last) {
			continue
		}
		for _, a := range s.Accounts {
			if used, read := a.Highest[key]; a.Account == id && read {
				use.Used, use.Read = max(use.Used, used), true
			}
		}
	}
	return use
}

// resetsKnown are the resets of the view's window of the account a that are
// known: those its readings give, as its past holds them, and its next, as
// the moment has the window.
func (from LanesFrom) resetsKnown(a status.AccountNow, past pastOf) []time.Time {
	known := past.resets
	if w, ok := a.Window(from.viewKey()); ok {
		known = append(slices.Clip(known), w.Next)
	}
	return known
}

// marked are the resets of the view's window the account a's lane marks, in
// order, Back set where one of its stretches ends at it: its next, as the
// moment has the window, and, stepped back, those the days shown saw, as
// their summaries have them, each where the timeline holds it.
func (from LanesFrom) marked(a status.AccountNow, stretches []LaneStretch) []Reset {
	tl, key := from.Timeline, from.viewKey()
	var at []time.Time
	if w, ok := a.Window(key); ok {
		at = append(at, w.Next)
	}
	if _, _, named := tl.Named(); named {
		at = append(at, from.resetsSeen(a.ID, key)...)
	}
	slices.SortFunc(at, time.Time.Compare)
	var resets []Reset
	for _, t := range at {
		if c := tl.Column(t); c >= 0 && c < tl.Columns {
			back := slices.ContainsFunc(stretches, func(s LaneStretch) bool { return s.Back.Equal(t) })
			resets = append(resets, Reset{Column: c, At: t, Back: back})
		}
	}
	return resets
}

// resetsSeen are the resets of the window with the given key of the account
// with the given id that the days given saw, as their summaries have them.
func (from LanesFrom) resetsSeen(id, key string) []time.Time {
	var seen []time.Time
	for _, s := range from.Days {
		for _, a := range s.Accounts {
			for _, r := range a.Resets {
				if a.Account == id && r.Window == key {
					seen = append(seen, r.At)
				}
			}
		}
	}
	return seen
}

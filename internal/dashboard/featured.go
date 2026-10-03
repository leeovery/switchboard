package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// Feature is which window every card features, with its big digits, its
// chart and its axis, the rest its bars: Auto, or a window's key, as w
// cycles them, for every card at once, so the cards stay comparable.
type Feature string

// Auto features on each card what will stop its account first: the window
// holding it back now, else the one that runs out soonest, else its most
// used.
const Auto Feature = ""

// Next is the setting w moves on to from f, as doc stands at now: auto, then
// the window a request starts, then the week, then any other window in use,
// and round to auto. A setting w doesn't reach, as one a later document has
// no use for, moves on as auto does.
func (f Feature) Next(doc status.Document, now time.Time, policy score.Policy) Feature {
	cycle := []Feature{Auto}
	for _, key := range slices.Concat([]string{policy.Started, policy.Perishable}, shownWindows(doc, now, policy)) {
		if key != "" && !slices.Contains(cycle, Feature(key)) {
			cycle = append(cycle, Feature(key))
		}
	}
	return cycle[(max(slices.Index(cycle, f), 0)+1)%len(cycle)]
}

// Name is what the footer calls the setting: "auto"; the window a request
// starts by its key, as ROOM LEFT calls it, "5h"; and any other by its label
// as a narrow column has it, in a sentence, as "week" and "Fable wk".
func (f Feature) Name(doc status.Document, policy score.Policy) string {
	switch key := string(f); {
	case f == Auto:
		return "auto"
	case key == policy.Started:
		return status.Clean(key)
	default:
		return status.InProse(status.Short(labelOf(doc, key)))
	}
}

// labelOf is the label of the window with the given key, as doc's first
// account with it has it, or the key where none has.
func labelOf(doc status.Document, key string) string {
	for _, a := range doc.Accounts {
		if w, ok := a.Window(key); ok && w.Label != "" {
			return w.Label
		}
	}
	return key
}

// shownWindows are the keys of the windows every card shows, of doc at now,
// in quota's order: those every model shares, always, and any other an
// account has used this period, or is heading to, as Project says. A model's
// own window that no account has used, nor is heading to use, hides from
// every card.
func shownWindows(doc status.Document, now time.Time, policy score.Policy) []string {
	shown, _ := windowsOf(doc, now, policy)
	return shown
}

// hiddenWindows are the keys of the windows of doc's accounts that hide from
// every card at now, unused on every account, in quota's order.
func hiddenWindows(doc status.Document, now time.Time, policy score.Policy) []string {
	_, hidden := windowsOf(doc, now, policy)
	return hidden
}

// windowsOf are the keys of doc's accounts' windows at now, in quota's
// order: those every card shows, and those hidden from every card, as
// shownWindows says.
func windowsOf(doc status.Document, now time.Time, policy score.Policy) (shown, hidden []string) {
	used := make(map[string]bool)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			used[w.Key] = used[w.Key] || policy.IsShared(w.Key) || inUse(a, w, now)
		}
	}
	for key, inUse := range used {
		if inUse {
			shown = append(shown, key)
		} else {
			hidden = append(hidden, key)
		}
	}
	slices.SortFunc(shown, quota.CompareKeys)
	slices.SortFunc(hidden, quota.CompareKeys)
	return shown, hidden
}

// inUse reports whether account a has used its window w this period at now,
// or is heading to: some of it is used, or its projection has some used by
// its reset.
func inUse(a status.Account, w quota.Window, now time.Time) bool {
	if w.Utilization > 0 {
		return true
	}
	switch p := a.Project(w, now).Projection; p.Kind {
	case score.RunsOut, score.Exhausted:
		return true
	case score.OnPace:
		return p.AtReset > score.Tolerance
	default:
		return false
	}
}

// hiddenNote says which windows hide from every card, as in "Fable wk
// hidden: unused on every account": nil when none does.
func hiddenNote(doc status.Document, keys []string) line {
	if len(keys) == 0 {
		return nil
	}
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = status.Short(labelOf(doc, key))
	}
	return line{{prose.List(names), mutedInk}, {" hidden: unused on every account", dimInk}}
}

// featured is the window doc's account a features at now, of those shown,
// reporting false where it has none of them: the one setting names, where it
// has it; else, as on auto, what will stop it first. That's the window
// holding it back now, at a limit or its reserve; else the one that runs out
// soonest, where one runs out before it resets, as RunsOut has it; else its
// most used, by share, a lapsed window reading empty.
func featured(doc status.Document, a status.Account, now time.Time, policy score.Policy, setting Feature, shown []string) (quota.Window, bool) {
	windows := slices.DeleteFunc(slices.Clone(a.Windows), func(w quota.Window) bool { return !slices.Contains(shown, w.Key) })
	if len(windows) == 0 {
		return quota.Window{}, false
	}
	if w, ok := withKey(windows, string(setting)); ok && setting != Auto {
		return w, true
	}
	if w, ok := holding(a, windows, now, policy); ok {
		return w, true
	}
	if w, ok := soonestOut(doc, a, windows, now); ok {
		return w, true
	}
	return slices.MaxFunc(windows, func(x, y quota.Window) int { return cmp.Compare(x.Utilization, y.Utilization) }), true
}

// withKey is the window of windows with the given key, reporting false
// where there's none.
func withKey(windows []quota.Window, key string) (quota.Window, bool) {
	i := slices.IndexFunc(windows, func(w quota.Window) bool { return w.Key == key })
	if i < 0 {
		return quota.Window{}, false
	}
	return windows[i], true
}

// holding is the window of account a's windows holding it back at now, as
// policy says which every model shares: one at its limit, else one at its
// reserve; reporting false where none is.
func holding(a status.Account, windows []quota.Window, now time.Time, policy score.Policy) (quota.Window, bool) {
	var keys []string
	if held, ok := a.Held(now, policy); ok {
		keys = held.Windows
	}
	for _, key := range slices.Concat(keys, a.AtReserve) {
		if w, ok := withKey(windows, key); ok {
			return w, true
		}
	}
	return quota.Window{}, false
}

// soonestOut is the window of doc's account a's windows that runs out
// soonest at now, of those that run out before they reset, as RunsOut has
// it; reporting false where none does.
func soonestOut(doc status.Document, a status.Account, windows []quota.Window, now time.Time) (quota.Window, bool) {
	var soonest quota.Window
	var at time.Time
	for _, w := range windows {
		if out, ok := doc.RunsOut(a, w, now); ok && (at.IsZero() || out.At.Before(at)) {
			soonest, at = w, out.At
		}
	}
	return soonest, !at.IsZero()
}

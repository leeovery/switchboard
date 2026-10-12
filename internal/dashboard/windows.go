package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

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

// shownWindows are the keys of the windows of doc's accounts worth showing at
// now, in quota's order: those every model shares, always, and any other an
// account has used this period, or is heading to, as inUse says, or a probe
// expected but couldn't read. A model's own window that no account has used,
// nor is heading to use, hides.
func shownWindows(doc status.Document, now time.Time, policy score.Policy) []string {
	shown, _ := windowsOf(doc, now, policy)
	return shown
}

// hiddenWindows are the keys of the windows of doc's accounts that hide at
// now, unused on every account, in quota's order.
func hiddenWindows(doc status.Document, now time.Time, policy score.Policy) []string {
	_, hidden := windowsOf(doc, now, policy)
	return hidden
}

// windowsOf are the keys of doc's accounts' windows at now, in quota's
// order: those shown, and those hidden, as shownWindows says.
func windowsOf(doc status.Document, now time.Time, policy score.Policy) (shown, hidden []string) {
	used := make(map[string]bool)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			used[w.Key] = used[w.Key] || policy.IsShared(w.Key) || inUse(doc, a, w, now, policy)
		}
		for _, f := range a.Failures {
			used[f.Window] = true
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

// inUse reports whether doc's account a has used its window w this period
// at now, as policy judges it, or is heading to: some of it is used, as it
// stands then, as WindowAt has it, so none once it has lapsed or reset since
// it was read; or, where it hasn't, its projection, as doc's Project says,
// has some used by its reset.
func inUse(doc status.Document, a status.Account, w quota.Window, now time.Time, policy score.Policy) bool {
	stands, _ := a.WindowAt(w.Key, now, policy)
	switch {
	case stands.Utilization > 0:
		return true
	case resetSince(w, stands.ResetsAt):
		return false
	}
	switch p := doc.Project(a, w, now).Projection; p.Kind {
	case score.RunsOut, score.Exhausted:
		return true
	case score.OnPace:
		return p.AtReset > score.Tolerance
	default:
		return false
	}
}

// resetSince reports whether the window read has reset since it was read,
// next being its next reset as it stands at a moment, as WindowAt has it:
// next isn't the reset it was read with, as it's none once the window has
// lapsed, and a length on once that reset has passed.
func resetSince(read quota.Window, next time.Time) bool {
	return !next.Equal(read.ResetsAt)
}

// hiddenNote says which windows hide, as in "Fable wk hidden: unused on
// every account": nil when none does.
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

package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// windowNamed is the window with the given key as m names it, as its Shown
// and Hidden name theirs: one none of its accounts has, by its key, as
// nameOfWindow names it.
func windowNamed(m status.Moment, key string) status.WindowName {
	for _, windows := range [][]status.WindowName{m.Shown, m.Hidden} {
		if i := slices.IndexFunc(windows, func(w status.WindowName) bool { return w.Key == key }); i >= 0 {
			return windows[i]
		}
	}
	return status.WindowName{Key: key, Label: key, Name: nameOfWindow(key, key)}
}

// labelsOf names m's windows by their keys by their labels, as windowNamed
// has them.
func labelsOf(m status.Moment) func(key string) string {
	return func(key string) string { return windowNamed(m, key).Label }
}

// windowKeys are the keys of the windows given, in their order.
func windowKeys(windows []status.WindowName) []string {
	keys := make([]string, len(windows))
	for i, w := range windows {
		keys[i] = w.Key
	}
	return keys
}

// hiddenNote says which windows hide, as in "Fable wk hidden: unused on
// every account", each by its label, short: nil when none does.
func hiddenNote(hidden []status.WindowName) line {
	if len(hidden) == 0 {
		return nil
	}
	names := make([]string, len(hidden))
	for i, w := range hidden {
		names[i] = status.Short(w.Label)
	}
	return line{{prose.List(names), mutedInk}, {" hidden: unused on every account", dimInk}}
}

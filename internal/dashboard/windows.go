package dashboard

import (
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// windowNamed is the window with the given key as m names it, as its Window
// has it: one none of its accounts has, by its key, as status.NameOfWindow
// names it.
func windowNamed(m status.Moment, key string) status.WindowName {
	if w, ok := m.Window(key); ok {
		return w
	}
	return status.WindowName{Key: key, Label: key, Name: status.NameOfWindow(key, key)}
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

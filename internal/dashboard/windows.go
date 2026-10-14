package dashboard

import "github.com/leeovery/switchboard/internal/status"

// windowNamed is the window with the given key as m names it, as its Window
// has it: one none of its accounts has labelled by its key, and named as
// m.NameOf names it.
func windowNamed(m status.Moment, key string) status.WindowName {
	if w, ok := m.Window(key); ok {
		return w
	}
	return status.WindowName{Key: key, Label: key, Name: m.NameOf(key)}
}

// labelsOf names m's windows by their keys by their labels, as windowNamed
// has them.
func labelsOf(m status.Moment) func(key string) string {
	return func(key string) string { return windowNamed(m, key).Label }
}

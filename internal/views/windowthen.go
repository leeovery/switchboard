package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/claude"
)

// WindowThen is a window of an account as an answer read it, as the Log's
// cards say one, a request's and an event's: its key; its name, as a
// sentence has it, "5-hour" or "week"; its share used; and when it resets,
// zero where that isn't known.
type WindowThen struct {
	Key, Name string
	Use       float64
	Resets    time.Time
}

// windowsThen are the windows an answer read, as claude.WindowsRead reads
// them, each as WindowThen has it, named by names as an event's windows are,
// or as status.NameOfKey names it where names is nil: none where it read
// none.
func windowsThen(windows []claude.WindowRead, names func(key string) string) []WindowThen {
	named := eventFacts{name: names}
	var then []WindowThen
	for _, w := range windows {
		then = append(then, WindowThen{Key: w.Key, Name: named.windowOf(w.Key), Use: w.Use, Resets: w.Resets})
	}
	return then
}

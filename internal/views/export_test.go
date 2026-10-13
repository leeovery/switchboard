package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// TakenFacts are an event's facts as eventFacts work them out, for the
// package's tests outside it, which the shared hard moments' package, itself
// importing views, keeps out of the package.
type TakenFacts struct {
	RunsOut time.Time
	Moved   int
	To      string
	Windows []string
}

// TakeFacts works out the facts of each of the events, taken in turn, oldest
// first, their windows named as name names them.
func TakeFacts(name func(key string) string, events []status.Event) []TakenFacts {
	facts := eventFacts{name: name}
	taken := make([]TakenFacts, len(events))
	for i, e := range events {
		f := facts.take(e)
		taken[i] = TakenFacts{RunsOut: f.runsOut, Moved: f.moved, To: f.to, Windows: f.windows}
	}
	return taken
}

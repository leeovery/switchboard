package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
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

// RowOfLine is the row of the request l, of a session whose lines, oldest
// first, are those given, its story told from them, at now's prices.
func RowOfLine(l ledger.Line, session []ledger.Line, prices ledger.Table, now time.Time) RequestRow {
	today := Today(now)
	return lineRow(&l, storyOf(requestsOf(session, prices, today.Date), prices, today), prices, today)
}

// RowOfTold is the row of the request t, as the stream told of it, at now.
func RowOfTold(t Told, now time.Time) RequestRow {
	return toldRow(t, now)
}

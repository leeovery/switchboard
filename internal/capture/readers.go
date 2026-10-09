package capture

import (
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// fakeLedger reads as the request ledger does, in place of the real one: it
// holds no line yet.
type fakeLedger struct{}

// Days gives no day's summary.
func (fakeLedger) Days(time.Time) []ledger.Summary {
	return nil
}

// Today gives none of today's lines.
func (fakeLedger) Today(ledger.Mark) ([]ledger.Held, ledger.Mark, bool) {
	return nil, ledger.Mark{}, false
}

// Session gives none of a session's lines.
func (fakeLedger) Session(string) iter.Seq[ledger.Held] {
	return func(func(ledger.Held) bool) {}
}

// fakeReadings reads as the readings history does, in place of the real one:
// it holds no reading yet.
type fakeReadings struct{}

// Between gives no reading.
func (fakeReadings) Between(time.Time, time.Time) iter.Seq[readings.Reading] {
	return func(func(readings.Reading) bool) {}
}

// fakeEvents reads as the router's events do, in place of the real ones: it
// holds no event yet.
type fakeEvents struct{}

// Between gives no event.
func (fakeEvents) Between(time.Time, time.Time) iter.Seq[events.Line] {
	return func(func(events.Line) bool) {}
}

package ledger

import (
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
)

// Empty reads as a Follower of a ledger that holds no line, with no files
// to read: for one who reads the ledger where there's none to find.
type Empty struct {
	now  func() time.Time
	caps Caps
}

// NewEmpty returns a ledger that holds no line, by now's clock, which
// summarises days with the accounts' caps as caps gives them.
func NewEmpty(now func() time.Time, caps Caps) Empty {
	return Empty{now: now, caps: caps}
}

// Days returns today's summary alone, as a Follower of a ledger that holds
// no line gives it whatever day from is: a day of no requests.
func (e Empty) Days(time.Time) []Summary {
	now := e.now()
	// Span gives only dates, so Summarise can't fail.
	today, _ := Summarise(dayfile.Span(now, now)[0], func(func(Line) bool) {}, readings.Empty{}.Between, nil, e.caps, now)
	return []Summary{today}
}

// Today returns no line, and the Mark it's read to, reporting afresh from the
// zero Mark alone, as a Follower does where today's lines begin to be given.
func (Empty) Today(mark Mark) (lines []Held, next Mark, afresh bool) {
	return nil, Mark{gen: 1}, mark == Mark{}
}

// Session returns no line.
func (Empty) Session(string) iter.Seq[Held] {
	return func(func(Held) bool) {}
}

// DayLines returns no line.
func (Empty) DayLines(string) iter.Seq[Line] {
	return func(func(Line) bool) {}
}

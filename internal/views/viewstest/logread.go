package viewstest

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// LogReading is the Log's read as the watch grows it, and the store its
// rows are told by, as the watch feeds it at each read: both what the
// Log's tables, its panel and its cards are built of.
type LogReading struct {
	Read views.LogRead
	Held views.HeldSessions
}

// ReadingLog is a read for the Log holding no line, beside a store holding
// no session, each pricing at prices.
func ReadingLog(prices ledger.Table) LogReading {
	return LogReading{Read: views.ReadLog(prices), Held: views.HoldSessions(prices)}
}

// ReadLog is the Log's read of lines, through l, at now, taken in at once,
// every one of a day's, as the Log reads today's on opening, as Took takes
// them, of a stream that told nothing.
func ReadLog(l views.LedgerSessions, lines []ledger.Held, prices ledger.Table, now time.Time) LogReading {
	return ReadingLog(prices).Took(l, lines, true, views.Stream{}, now)
}

// Took is r with lines taken in, through l, at now, as a read of the
// ledger's tail takes them: the store fed them, today's as the tail's gain
// and each day's before today as that day read again whole, wanting the
// sessions of the Log's lines and of those given, then the read taking them
// in, whole where they're every one of a day's, each placed as s tells, and
// told, as its rows the feed changed told again, of the store as fed.
func (r LogReading) Took(l views.LedgerSessions, lines []ledger.Held, whole bool, s views.Stream, now time.Time) LogReading {
	today := views.Today(now)
	var gained []ledger.Held
	days := make(map[string][]ledger.Held)
	for _, h := range lines {
		if date := views.Today(h.At).Date; date != today.Date {
			days[date] = append(days[date], h)
			continue
		}
		gained = append(gained, h)
	}
	recorded := views.RecordsOf(lines)
	want := views.StreamWanted(s)
	want.Log = r.Read.Wanted(recorded)
	held, gains := r.Held.Fed(l, views.Took(gained, days), want, now)
	return LogReading{Read: r.Read.Taken(recorded, whole, views.LogTaking{Held: held, Gains: gains, Stream: s, Now: now}), Held: held}
}

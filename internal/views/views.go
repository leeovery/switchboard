// Package views builds each view's data, by one function a view, from what's
// read: the status document, the router's sessions and what its request
// stream tells, the readers of the request ledger, the readings history and
// the router's events, the price table and a clock. What it builds is what a
// page of the dashboard draws and its data verb prints, so the two never
// disagree. It opens nothing itself: it reads through the readers it's given.
package views

import (
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// LedgerDays reads the request ledger's days where they lie, with no router,
// as a ledger.Reader does.
type LedgerDays interface {
	// Days gives the summaries of the local days from from's to today's.
	Days(from time.Time) []ledger.Summary
}

// LedgerStart tells where the request ledger begins, as a ledger.Follower
// does: History's Year steps back through the calendar years from its
// first.
type LedgerStart interface {
	// FirstDay gives the start of the first local day the ledger holds, at
	// now, failing where it holds none.
	FirstDay(now time.Time) (time.Time, error)
}

// Ledger reads the request ledger where it lies, with no router, as it
// grows, as a ledger.Follower does.
type Ledger interface {
	LedgerDays
	LedgerDates
	// Today gives today's lines read since mark, and the Mark they're read
	// to, reporting afresh where they're every one of today's, for those
	// read before to be let go of.
	Today(mark ledger.Mark) (lines []ledger.Held, next ledger.Mark, afresh bool)
	// Day gives the lines of the requests that arrived on the local day with
	// the given date, by now, oldest first, read whole at each call: none of
	// a day the ledger holds no lines of.
	Day(date string) []ledger.Held
	// FirstLinesDay gives the start of the first local day the ledger holds
	// lines of, at now: the first Day reads back to, a summary of an earlier
	// day, whose lines are pruned, aside. It fails with fs.ErrNotExist where
	// it holds no day's lines.
	FirstLinesDay(now time.Time) (time.Time, error)
	LedgerSession
	LedgerSessions
}

// LedgerSession reads one session's lines where they lie, with no router, as
// a ledger.Follower does: of the days that name it alone.
type LedgerSession interface {
	// Session gives the lines of the session with the given id, newest
	// first, read as far back as the caller goes on.
	Session(id string) iter.Seq[ledger.Held]
}

// LedgerSessions reads the request ledger's sessions where they lie, with no
// router, every line of each, whatever day it's of, as a ledger.Follower
// does.
type LedgerSessions interface {
	// Sessions gives the lines of today's sessions, those with a request
	// today, and of those with the given ids, that arrived by now, by each
	// one's id, oldest first, as Ledger's Session reads one's, each day's files
	// read once; and hands take, where it's given, each line it reads, of
	// whichever session, with the date of the day whose files hold it.
	Sessions(ids []string, take func(date string, l ledger.Line)) map[string][]ledger.Line
}

// Readings reads the readings history where it lies, with no router, as a
// readings.Reader does.
type Readings interface {
	// Between gives the readings of times from from up to to.
	Between(from, to time.Time) iter.Seq[readings.Reading]
	// Tail gives a read of the readings from start up to end, oldest first,
	// each window's run from from up to start carried in, as
	// readings.CarriedRuns has them, so the lanes tell when a hold begun
	// before start began: the first call reads them whole, and each after
	// only what the history gained since, as Runway's lanes read the past at
	// each look. A read isn't safe for concurrent use.
	Tail(from, start, end time.Time) func() []readings.Reading
	// FirstDay gives the start of the first local day the history holds
	// readings of, a day before it drawn from its summary alone, as Runway's
	// lanes draw it. It fails with fs.ErrNotExist where it holds none.
	FirstDay() (time.Time, error)
}

// Events reads the router's events where they lie, with no router, as an
// events.Reader does.
type Events interface {
	// Between gives the events that happened from from up to to, each as it
	// last stands, oldest first.
	Between(from, to time.Time) iter.Seq[events.Line]
	// FirstDay gives the start of the first local day the router filed
	// events on, which the Log's Events read back no further than. It fails
	// with fs.ErrNotExist where it filed none.
	FirstDay() (time.Time, error)
}

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

// Ledger reads the request ledger where it lies, with no router, as it
// grows, as a ledger.Follower does.
type Ledger interface {
	LedgerDays
	// Today gives today's lines read since mark, and the Mark they're read
	// to, reporting afresh where they're every one of today's, for those
	// read before to be let go of.
	Today(mark ledger.Mark) (lines []ledger.Held, next ledger.Mark, afresh bool)
	// Session gives the lines of the session with the given id, newest
	// first, read as far back as the caller goes on.
	Session(id string) iter.Seq[ledger.Held]
}

// Readings reads the readings history where it lies, with no router, as a
// readings.Reader does.
type Readings interface {
	// Between gives the readings of times from from up to to.
	Between(from, to time.Time) iter.Seq[readings.Reading]
}

// Events reads the router's events where they lie, with no router, as an
// events.Reader does.
type Events interface {
	// Between gives the events that happened from from up to to, each as it
	// last stands, oldest first.
	Between(from, to time.Time) iter.Seq[events.Line]
}

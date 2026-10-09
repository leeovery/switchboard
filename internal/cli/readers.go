package cli

import (
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// caps are the caps of the accounts cfg configures, as the request ledger
// summarises its days with them.
func caps(cfg *config.Config) ledger.Caps {
	return ledger.CapsOf(cfg.Accounts, claude.SharedWindows)
}

// readers has wc read the request ledger, the readings history and the
// router's events where they lie, in the state directory, by the commands'
// clock, the ledger's days summarised with the caps of the accounts cfg
// configures. Without a state directory to find, they read nothing.
func (a *app) readers(wc *watch.Config, cfg *config.Config) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		logger.Debug("can't find the ledger, the readings history or the router's events", "error", err)
		wc.Ledger, wc.Readings, wc.Events = noLedger{now: a.Now, caps: caps(cfg)}, noReadings{}, noEvents{}
		return
	}
	wc.Ledger = ledger.NewFollower(dir, a.Now, caps(cfg), logger)
	wc.Readings = readings.NewReader(dir, logger)
	wc.Events = events.NewReader(dir, a.Now, logger)
}

// noLedger reads as a ledger.Follower of a ledger that holds no line, by now's
// clock, its days summarised with caps.
type noLedger struct {
	now  func() time.Time
	caps ledger.Caps
}

// Days gives today's summary alone, as a Follower of a ledger that holds no
// line does, whatever day from is: a day of no requests.
func (l noLedger) Days(time.Time) []ledger.Summary {
	now := l.now()
	// Span gives only dates, so Summarise can't fail.
	today, _ := ledger.Summarise(dayfile.Span(now, now)[0], func(func(ledger.Line) bool) {}, noReadings{}.Between, nil, l.caps, now)
	return []ledger.Summary{today}
}

// Today gives no line, afresh where mark is the zero Mark, as a Follower
// gives every one of today's lines from it. The Mark it gives is the zero
// Mark, as none of the ledger package's own can be made here, so each call is
// afresh, of no line.
func (noLedger) Today(mark ledger.Mark) ([]ledger.Held, ledger.Mark, bool) {
	return nil, ledger.Mark{}, mark == ledger.Mark{}
}

// Session gives no line.
func (noLedger) Session(string) iter.Seq[ledger.Held] { return func(func(ledger.Held) bool) {} }

// noReadings reads as a readings history that holds no reading.
type noReadings struct{}

func (noReadings) Between(time.Time, time.Time) iter.Seq[readings.Reading] {
	return func(func(readings.Reading) bool) {}
}

// noEvents reads as the router's events where there are none.
type noEvents struct{}

func (noEvents) Between(time.Time, time.Time) iter.Seq[events.Line] {
	return func(func(events.Line) bool) {}
}

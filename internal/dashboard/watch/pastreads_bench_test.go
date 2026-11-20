package watch

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// yearDays is how many days back from today a year's ledger holds, as the
// ledger keeps them by default.
const yearDays = 400

// yearLedger is a busy world's request ledger as a Follower reads it, but
// holding a busy day's summary of each day from first to today's, as days
// holds it by its date, as a year of busy days would: each held as the
// Follower holds those it has read, read again from none of their files,
// to today by clock.
type yearLedger struct {
	*ledger.Follower
	clock func() time.Time
	first string
	days  map[string]ledger.Summary
}

// yearOf is the busy world's request ledger read by f, holding yearDays of
// summaries to today by clock, each day before it summarised as one of the
// world's ended days in turn, its date its own, and today as the world's
// today.
func yearOf(f *ledger.Follower, w busyWorld, clock func() time.Time) yearLedger {
	held := f.Days(w.day(0))
	ended, today := held[:len(held)-1], held[len(held)-1]
	l := yearLedger{Follower: f, clock: clock, first: dayfile.DateOf(dayfile.DayStart(w.now, -yearDays)), days: map[string]ledger.Summary{}}
	for i, date := range dayfile.Span(dayfile.DayStart(w.now, -yearDays), w.now) {
		d := ended[i%len(ended)]
		if date == today.Day {
			d = today
		}
		d.Day = date
		l.days[date] = d
	}
	return l
}

func (l yearLedger) Days(from time.Time) []ledger.Summary {
	var days []ledger.Summary
	for _, date := range dayfile.Span(from, l.clock()) {
		if d, ok := l.days[date]; ok {
			days = append(days, d)
		}
	}
	return days
}

func (l yearLedger) FirstDay(time.Time) (time.Time, error) {
	start, _, _ := dayfile.Day(l.first)
	return start, nil
}

// onAccountsOverAYear is the busy world's harness reading a year of its
// days, as yearOf holds them, on Accounts' Compare, weeks starting on
// Monday, its period of the length given at now, as the keys given lengthen
// it, its page built.
func (w busyWorld) onAccountsOverAYear(b *testing.B, keys string) *harness {
	b.Helper()
	h := routedHarness(b, busyDocument())
	h.clock.now = w.now
	h.source.sessions = w.running()
	follower, history, filed := w.readers(h.clock.Now)
	h.model.cfg.Ledger, h.model.cfg.Readings, h.model.cfg.Events = yearOf(follower, w, h.clock.Now), history, filed
	h.model.cfg.WeekStarts = time.Monday
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.start()
	h.model.view = dashboard.Accounts
	h.deliver(h.update(nudgeMsg{})...)
	h.keyed(keys)
	if h.model.accounts.page == nil {
		b.Fatalf("Accounts over %q built no page", said(h, h.model.period))
	}
	return h
}

// BenchmarkAStepBackOnAccounts times a step back on Accounts' Compare from
// now, on a Thursday, of a busy world whose ledger holds a year of busy
// days, as onAccountsOverAYear has it: from the 12 months to the 12 months
// before them, reading back the days they reach, and again with those days
// held; and from this week to last week, whose weeks reach back before the
// 12 months' read, so it reads back too. Each step's page built. The
// ledger's own read of its summaries' files is the ledger's to time: the
// first read of the 12 months reads as many.
func BenchmarkAStepBackOnAccounts(b *testing.B) {
	w := newBusyWorld(b, start.AddDate(0, 0, 3))
	steps := []struct {
		name, keys, to string
		held           bool
	}{
		{name: "to 12 months to 1 Oct 2025", keys: "+ + +", to: "12 months to 1 Oct 2025"},
		{name: "to 12 months to 1 Oct 2025, its days held", keys: "+ + +", to: "12 months to 1 Oct 2025", held: true},
		{name: "to last week", to: "last week"},
	}
	for _, step := range steps {
		b.Run(step.name, func(b *testing.B) {
			h := w.onAccountsOverAYear(b, step.keys)
			if step.held {
				h.keyed("< n")
			}
			on := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = on, nil, nil
				h.keyed("<")
			}
			if got := said(h, h.model.period); got != step.to || h.model.accounts.page == nil || h.model.accounts.page.Period != h.model.period {
				b.Fatalf("stepped back to %q, its page built %t; want %q's", got, h.model.accounts.page != nil, step.to)
			}
		})
	}
}

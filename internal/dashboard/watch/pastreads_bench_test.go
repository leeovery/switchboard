package watch

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// yearDays is how many days back from today a year's ledger holds, as the
// ledger keeps them by default.
const yearDays = 400

// yearUsage is the usage of each request of a year's sides.
const yearUsage = `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
	`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":500}`

// yearLedger is a busy world's request ledger as a Follower reads it, but
// holding a busy day's summary and side of each day from first, as days and
// sides hold them by their dates, as a year of busy days would, to today by
// clock: each held in memory, as the Follower holds the summaries it has
// read, so none of their files is read.
type yearLedger struct {
	*ledger.Follower
	clock func() time.Time
	first string
	days  map[string]ledger.Summary
	sides map[string]json.RawMessage
}

// yearOf is the busy world's request ledger read by f, holding yearDays of
// summaries to today by clock, each day before it summarised as one of the
// world's ended days in turn, its date its own, and today as the world's
// today; and the side of each day before today, as yearOfSides builds them.
func yearOf(b *testing.B, f *ledger.Follower, w busyWorld, clock func() time.Time) yearLedger {
	b.Helper()
	first := dayfile.DayStart(w.now, -yearDays)
	held := f.Days(w.day(0))
	ended, today := held[:len(held)-1], held[len(held)-1]
	l := yearLedger{Follower: f, clock: clock, first: dayfile.DateOf(first), days: map[string]ledger.Summary{}, sides: yearOfSides(b, first, w.now)}
	for i, date := range dayfile.Span(first, w.now) {
		d := ended[i%len(ended)]
		if date == today.Day {
			d = today
		}
		d.Day = date
		l.days[date] = d
	}
	return l
}

// yearOfSides are the sides of each day from first's to the day before
// now's, as the views' Digests build them, each of the busy world's: a
// dozen sessions running, four starting each day and running three, each on
// the account busyOn gives it that day, in one of eight projects, asking
// for two models every hour of eight.
func yearOfSides(b *testing.B, first, now time.Time) map[string]json.RawMessage {
	b.Helper()
	sides := make(map[string]json.RawMessage)
	for d, date := range dayfile.Span(first, dayfile.DayStart(now, -1)) {
		start, _, _ := dayfile.Day(date)
		build := views.Digests{}.Day()
		for j := range busyRunning {
			g := busyStarting*(d-busyLasts+1) + j
			for hour := 9; hour < 17; hour++ {
				for i, model := range []string{"claude-opus-5-5", "claude-haiku-4-5"} {
					build.Take(ledger.Line{
						At: start.Add(time.Duration(hour)*time.Hour + time.Duration(j*4+i)*time.Minute).UTC(), Request: fmt.Sprintf("%s-%d-%d-%d", date, j, hour, i),
						Kind: ledger.KindMessage, Session: busySession(g), Dir: fmt.Sprintf("~/Code/project-%d", (g%8+8)%8), Model: model, Account: busyOn(g, d),
						Status: 200, Class: ledger.ClassMain, Usage: json.RawMessage(yearUsage),
					})
				}
			}
		}
		built, err := build.Built()
		if err != nil {
			b.Fatal(err)
		}
		sides[date] = built.Side
	}
	return sides
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

func (l yearLedger) DaySide(date string, _ int) (json.RawMessage, bool) {
	side, ok := l.sides[date]
	return side, ok
}

func (l yearLedger) FirstDay(time.Time) (time.Time, error) {
	start, _, _ := dayfile.Day(l.first)
	return start, nil
}

func (l yearLedger) FirstLinesDay(now time.Time) (time.Time, error) {
	return l.FirstDay(now)
}

// overAYear is the busy world's harness reading a year of its days, as
// yearOf holds them, on the view given, Accounts' Compare or Projects,
// weeks starting on Monday, its period of the length given at now, as the
// keys given lengthen it, its page built.
func (w busyWorld) overAYear(b *testing.B, view dashboard.View, keys string) *harness {
	b.Helper()
	h := routedHarness(b, busyDocument())
	h.clock.now = w.now
	h.source.sessions = w.running()
	follower, history, filed := w.readers(h.clock.Now)
	h.model.cfg.Ledger, h.model.cfg.Readings, h.model.cfg.Events = yearOf(b, follower, w, h.clock.Now), history, filed
	h.model.cfg.WeekStarts = time.Monday
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.start()
	h.model.view = view
	h.deliver(h.update(nudgeMsg{})...)
	h.keyed(keys)
	if !periodPageBuilt(h) {
		b.Fatalf("%v over %q built no page", view, said(h, h.model.period))
	}
	return h
}

// periodPageBuilt reports whether the page of the tab h shows, Accounts or
// Projects, is built of the period its dial shows.
func periodPageBuilt(h *harness) bool {
	if h.model.view == dashboard.Projects {
		page := h.model.projects.shown.Page
		return page != nil && page.Period == h.model.period
	}
	return pageOfThePeriod(h)
}

// BenchmarkAStepBack times a step back on Accounts' Compare and on
// Projects from now, on a Thursday, of a busy world whose ledger holds a
// year of busy days, as overAYear has it: from the 12 months to the 12
// months before them, reading back the days they reach, and again on
// Accounts with those days held; and from this week to last week, whose
// weeks reach back before Accounts' 12 months' read, so it reads back too,
// and before the week Projects holds. Each step's page built. The days'
// summaries and sides are held in memory, so a step reads none of their
// files: the follower's read of the sessions of each of the busy world's 27
// digested days, as Projects reads each day's of its side, times that.
func BenchmarkAStepBack(b *testing.B) {
	w := newPooledWorld(b, start.AddDate(0, 0, 3), unpooled)
	steps := []struct {
		name, keys, to string
		view           dashboard.View
		held           bool
	}{
		{name: "on Accounts to 12 months to 1 Oct 2025", view: dashboard.Accounts, keys: "+ + +", to: "12 months to 1 Oct 2025"},
		{name: "on Accounts to 12 months to 1 Oct 2025, its days held", view: dashboard.Accounts, keys: "+ + +", to: "12 months to 1 Oct 2025", held: true},
		{name: "on Accounts to last week", view: dashboard.Accounts, to: "last week"},
		{name: "on Projects to 12 months to 1 Oct 2025", view: dashboard.Projects, keys: "+ + +", to: "12 months to 1 Oct 2025"},
		{name: "on Projects to last week", view: dashboard.Projects, to: "last week"},
	}
	for _, step := range steps {
		b.Run(step.name, func(b *testing.B) {
			h := w.overAYear(b, step.view, step.keys)
			if step.held {
				h.keyed("< n")
			}
			on := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = on, nil, nil
				h.keyed("<")
			}
			if got := said(h, h.model.period); got != step.to || !periodPageBuilt(h) {
				b.Fatalf("stepped back to %q, its page built %t; want %q's", got, periodPageBuilt(h), step.to)
			}
		})
	}
	b.Run("the follower reading the sessions of 27 digested days", func(b *testing.B) {
		follower, _, _ := w.readers(w.clock)
		for b.Loop() {
			for d := range busyDays - 1 {
				views.DaySessionsOn(follower, dayfile.DateOf(w.day(d)), views.PoolRead{})
			}
		}
	})
}

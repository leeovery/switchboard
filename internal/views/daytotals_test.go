package views_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// ledgerIn files the moment's lines and summaries in the request ledger of
// a state directory of the test's own, as the design lays its files out,
// each line under the local day its request arrived on, and returns a
// follower of it by a clock stopped at the moment's now, as the dashboard
// reads it.
func ledgerIn(t *testing.T, hm viewstest.DayMoment) *ledger.Follower {
	t.Helper()
	state := t.TempDir()
	dir := ledger.Dir(state)
	for _, d := range []string{dir, readings.Dir(state)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	byDay := make(map[string][]byte)
	for _, l := range hm.Lines {
		date := dayfile.DateOf(l.At)
		byDay[date] = append(append(byDay[date], jsonLine(t, l)...), '\n')
	}
	for date, lines := range byDay {
		writeIn(t, dir, "requests-"+date+".jsonl", lines)
	}
	for _, s := range hm.Summaries {
		writeIn(t, dir, "day-"+s.Day+".json", jsonLine(t, s))
	}
	return ledger.NewFollower(state, func() time.Time { return hm.Now }, viewstest.Caps, unheard)
}

// jsonLine is v as JSON.
func jsonLine(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// writeIn writes data to the file with the given name in dir, the user's
// alone.
func writeIn(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// figuresOf are a day's totals as viewstest has them.
func figuresOf(d views.DayTotals) viewstest.DayFigures {
	return viewstest.DayFigures{Date: d.Date, Requests: d.Requests, Sessions: d.Sessions, Limits: d.Limits, Worth: d.Amount}
}

// sameFigures reports whether a and b are the same figures, their worths
// the same sum, or both none.
func sameFigures(a, b viewstest.DayFigures) bool {
	worthAlike := (a.Worth == nil) == (b.Worth == nil) && (a.Worth == nil || *a.Worth == *b.Worth)
	return a.Date == b.Date && a.Requests == b.Requests && a.Sessions == b.Sessions && a.Limits == b.Limits && worthAlike
}

// At each hard moment, each day's figures are its summary's, as the ledger
// reads its days: a summarised day's from its summary, the limits the
// readings history told of among them; today's, and a day whose summary is
// older than its last line, from its lines as they're read, knowing no less
// than the summary held; none of a day with no lines; a request across
// midnight on the day it arrived; and two at 01:30 as the clocks go back on
// their one day.
func TestEachDaysTotalsAreItsSummarysAsTheLedgerReadsItsDays(t *testing.T) {
	for _, hm := range viewstest.DayMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			got := views.TotalsOn(ledgerIn(t, hm), dayfile.Span(hm.From, hm.Now), hm.Now, ledger.Pricing)
			if len(got) != len(hm.Want) {
				t.Fatalf("the days' totals are %+v, want %+v", got, hm.Want)
			}
			for i, want := range hm.Want {
				if figures := figuresOf(got[i]); !sameFigures(figures, want) || len(got[i].Unpriced) > 0 {
					t.Errorf("%s's totals are %+v, unpriced %v; want %+v", want.Date, figures, got[i].Unpriced, want)
				}
			}
		})
	}
}

// daysAsked reads a ledger's days, noting the dates of each read of them by
// their dates.
type daysAsked struct {
	views.LedgerDates
	on [][]string
}

func (d *daysAsked) DaysOn(dates []string) []ledger.Summary {
	d.on = append(d.on, dates)
	return d.LedgerDates.DaysOn(dates)
}

// The figures on a day given, at each hard moment, are its alone, as they
// are among every day's, the ledger's days read once, that day's alone,
// however long ago it was; none where none is given.
func TestTheTotalsOnTheDaysGivenAreEachDaysAlone(t *testing.T) {
	for _, hm := range viewstest.DayMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			follower := ledgerIn(t, hm)
			for _, want := range hm.Want {
				days := &daysAsked{LedgerDates: follower}
				got := views.TotalsOn(days, []string{want.Date}, hm.Now, ledger.Pricing)
				if len(got) != 1 || !sameFigures(figuresOf(got[0]), want) {
					t.Errorf("the totals on %s are %+v, want %+v alone", want.Date, got, want)
				}
				if !slices.EqualFunc(days.on, [][]string{{want.Date}}, slices.Equal) {
					t.Errorf("the ledger's days were read on %q, want once, on %s alone", days.on, want.Date)
				}
			}
			days := &daysAsked{LedgerDates: follower}
			if got := views.TotalsOn(days, nil, hm.Now, ledger.Pricing); got != nil || days.on != nil {
				t.Errorf("given no day, the totals are %+v, the days read on %q; want none, nothing read", got, days.on)
			}
		})
	}
}

// dayAlone reads a ledger's days as days reads them from from, but gives the
// day with the given date alone, as a History of that day reads it.
type dayAlone struct {
	days views.LedgerDays
	from time.Time
	date string
}

func (d dayAlone) Days(time.Time) []ledger.Summary {
	for _, s := range d.days.Days(d.from) {
		if s.Day == d.date {
			return []ledger.Summary{s}
		}
	}
	return nil
}

// A day's totals are History's every account's over that day alone, at each
// hard moment: built on History's rules, never beside them.
func TestADaysTotalsAreHistorysOfTheDayAlone(t *testing.T) {
	for _, hm := range viewstest.DayMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			follower := ledgerIn(t, hm)
			for _, d := range views.TotalsOn(follower, dayfile.Span(hm.From, hm.Now), hm.Now, ledger.Pricing) {
				day, _, _ := dayfile.Day(d.Date)
				in := views.HistoryInput{Ledger: dayAlone{days: follower, from: hm.From, date: d.Date}, From: day, Now: hm.Now, Prices: ledger.Pricing,
					WeekStarts: time.Monday, WeekWindow: "7d", Family: family}
				all := views.NewHistory(in).Totals.Accounts[0]
				limits := 0
				for _, n := range all.Limits {
					limits += n
				}
				history := viewstest.DayFigures{Date: d.Date, Requests: all.Requests, Sessions: all.Sessions, Limits: limits, Worth: all.Amount}
				if figures := figuresOf(d); !sameFigures(figures, history) {
					t.Errorf("%s's totals are %+v, History's of the day %+v", d.Date, figures, history)
				}
			}
		})
	}
}

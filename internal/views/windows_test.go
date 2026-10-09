package views_test

import (
	"iter"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// historyOf is a readings history that holds the readings given, as a
// readings.Reader gives them: those of times from from up to to, in the
// order given.
type historyOf []readings.Reading

func (h historyOf) Between(from, to time.Time) iter.Seq[readings.Reading] {
	return func(yield func(readings.Reading) bool) {
		for _, r := range h {
			if !r.At.Before(from) && r.At.Before(to) && !yield(r) {
				return
			}
		}
	}
}

// reading is a reading of the window with the given key of the account with
// the given id, at the local time on the day with the given date, at the
// clock given, its use the one given, from an answer.
func reading(id, key, date, clock string, use float64) readings.Reading {
	return readings.Reading{At: onAt(date, clock).UTC(), Account: id, Key: key, Utilization: use, Status: quota.StatusAllowed, Source: readings.FromAnswer}
}

func TestWindowsAreEachAccountsWindowsReadingsOverTheDaysAskedFor(t *testing.T) {
	limit := reading("work", "5h", "2026-10-06", "22:14", 1)
	limit.Status, limit.ResetsAt = quota.StatusRejected, onAt("2026-10-06", "23:10").UTC()
	probed := reading("side", "7d_oi", "2026-10-07", "08:00", 0.2)
	probed.Source, probed.Status = readings.FromProbe, ""
	history := historyOf{
		reading("work", "5h", "2026-10-05", "23:59", 0.1),
		reading("former", "5h", "2026-10-06", "09:00", 0.5),
		reading("side", "5h", "2026-10-06", "10:00", 0.3),
		reading("work", "5h", "2026-10-06", "21:00", 0.9),
		limit,
		probed,
		reading("work", "7d", "2026-10-07", "09:00", 0.41),
		reading("side", "7d", "2026-10-07", "12:00", 0.6),
		reading("work", "5h", "2026-10-08", "00:00", 0.2),
	}

	// The days are the 6th and today, whatever the time on the 6th asked
	// from; the configured accounts in the config's order, and then the
	// one no longer configured; each account's windows in quota's order;
	// each window's readings in the order they were read.
	got := views.NewWindows(history, accounts, onAt("2026-10-06", "14:00"), now)
	want := `{"windows":[` +
		`{"account":"work","window":"5h","readings":[{"at":"` + utc("2026-10-06", "21:00") + `","utilization":0.9,"status":"allowed","source":"answer"},` +
		`{"at":"` + utc("2026-10-06", "22:14") + `","utilization":1,"resets_at":"` + utc("2026-10-06", "23:10") + `","status":"rejected","source":"answer"}]},` +
		`{"account":"work","window":"7d","readings":[{"at":"` + utc("2026-10-07", "09:00") + `","utilization":0.41,"status":"allowed","source":"answer"}]},` +
		`{"account":"side","window":"5h","readings":[{"at":"` + utc("2026-10-06", "10:00") + `","utilization":0.3,"status":"allowed","source":"answer"}]},` +
		`{"account":"side","window":"7d","readings":[{"at":"` + utc("2026-10-07", "12:00") + `","utilization":0.6,"status":"allowed","source":"answer"}]},` +
		`{"account":"side","window":"7d_oi","readings":[{"at":"` + utc("2026-10-07", "08:00") + `","utilization":0.2,"source":"probe"}]},` +
		`{"account":"former","window":"5h","readings":[{"at":"` + utc("2026-10-06", "09:00") + `","utilization":0.5,"status":"allowed","source":"answer"}]}]}`
	if got := jsonOf(t, got); got != want {
		t.Errorf("the windows are\n%s\nwant\n%s", got, want)
	}
}

func TestWindowsOfAnEmptyHistoryAreNone(t *testing.T) {
	if got, want := jsonOf(t, views.NewWindows(historyOf{}, accounts, now, now)), `{"windows":[]}`; got != want {
		t.Errorf("the windows of an empty history are %s, want %s", got, want)
	}
}

// utc is the local time on the day with the given date, at the clock given,
// in UTC, as the readings history writes it.
func utc(date, clock string) string {
	return onAt(date, clock).UTC().Format(time.RFC3339)
}

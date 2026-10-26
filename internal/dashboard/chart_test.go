package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// chartAt is a chart of a window ten hours long, from 08:00, at now, its
// readings as given.
func chartAt(now time.Time, readings ...score.Reading) chart {
	start := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	return chart{start: start, length: 10 * time.Hour, spanned: true, readings: readings, now: now}
}

// readOn is a reading of use, at the time given on 1 October 2026.
func readOn(hour, minute int, used float64) score.Reading {
	return score.Reading{At: time.Date(2026, 10, 1, hour, minute, 0, 0, time.UTC), Utilization: used}
}

// clockOn is the time given on 1 October 2026, in UTC.
func clockOn(hour, minute int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, 0, 0, time.UTC)
}

// accountAt is doc's account with the given id as it stands at the moment
// given.
func accountAt(doc status.Document, id string, at time.Time) status.AccountNow {
	a, _ := doc.At(at, claudeLike).Account(id)
	return a
}

func TestAWindowResetSinceItWasReadIsChartedFromItsReset(t *testing.T) {
	doc := trio()
	work, _ := doc.Account("work")
	window, _ := work.Window("7d")
	before := Trail{Readings: []score.Reading{{At: window.ResetsAt.Add(-time.Hour), Utilization: 0.5}}}
	after := window.ResetsAt.Add(time.Hour)
	got := chartOf(accountAt(doc, "work", after), "7d", before, after)
	if len(got.readings) != 0 || !got.start.Equal(window.ResetsAt) {
		t.Errorf("charted from %v with %d readings, want from its reset, %v, with none of before it", got.start, len(got.readings), window.ResetsAt)
	}
}

// Graphs charts a window as the router's next document has it: on the tick
// after work's session resets, before it's read again, the session has
// lapsed, and its card is drawn as once the router's next document has it
// empty, without a reset, rather than over a span it hasn't started.
func TestOnTheTickASessionLapsesItsCardIsAsTheRoutersNextDocumentHasIt(t *testing.T) {
	doc := trio()
	work, _ := doc.Account("work")
	session, _ := work.Window("5h")
	after := session.ResetsAt.Add(time.Second)
	trail := Trail{Readings: []score.Reading{{At: session.ResetsAt.Add(-time.Hour), Utilization: 0.5}}}
	nord := Screen(builtin(t, "nord"))
	nextDoc := trio()
	nextDoc.Accounts[0] = work.AsOf(claudeLike, after)

	if tick := chartOf(accountAt(doc, "work", after), "5h", trail, after); tick.spanned {
		t.Errorf("the session is charted from %v over %v, want no span, as it has lapsed", tick.start, tick.length)
	}
	tick := burnCardOf(accountAt(doc, "work", after), "5h", trail, after)
	next := burnCardOf(accountAt(nextDoc, "work", after), "5h", trail, after)
	if got, want := cardIn(tick, nord, 30, 5).rows(nord), cardIn(next, nord, 30, 5).rows(nord); !slices.Equal(got, want) {
		t.Errorf("on the tick, the card is\n%s\nwant it as the router's next document has it\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

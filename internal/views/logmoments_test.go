package views_test

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// logSourcesOf are what the Log's table is built from at the moment given:
// the lines of the days it spans, read with their sessions' through the
// ledger holding the moment's lines, as readAtOnce reads them, and the rest
// as the moment gives them.
func logSourcesOf(hm viewstest.LogMoment) views.LogSources {
	return views.LogSources{
		From: hm.From, Now: hm.Now, Read: readAtOnce(hm, ledgerHolding(hm.Now, hm.Lines...)),
		Stream: hm.Stream, Totals: hm.Totals, ShowChecks: hm.Shown, ShowCounts: hm.Shown,
	}
}

// readAtOnce is the Log's read of the moment's lines through l, as the watch
// reads it on opening: today's, as the ledger's Today gives them, then each
// day before back to the table's first, newest first, as its Day gives one.
func readAtOnce(hm viewstest.LogMoment, l views.LedgerSessions) views.LogRead {
	days := spannedDays(hm)
	read := views.ReadLogHeld(l, days[0], ledger.Pricing, hm.Now)
	for _, day := range days[1:] {
		read = read.Back(l, day, hm.Now)
	}
	return read
}

// spannedDays are the moment's lines of the days its table spans, as the ledger
// holds them, a day's together, in the order the moment gives them: today's
// first, none where it has none, then each day before with any, newest first.
func spannedDays(hm viewstest.LogMoment) [][]ledger.Held {
	first, today := views.Today(hm.From), views.Today(hm.Now)
	byDate := make(map[string][]ledger.Held)
	for _, l := range hm.Lines {
		if day := views.Today(l.At); first.Holds(l.At) && day.Date != today.Date {
			byDate[day.Date] = append(byDate[day.Date], ledger.Held{Line: l})
		}
	}
	days := [][]ledger.Held{heldOf(slices.DeleteFunc(slices.Clone(hm.Lines), func(l ledger.Line) bool { return !today.Holds(l.At) })...)}
	for _, date := range slices.Backward(slices.Sorted(maps.Keys(byDate))) {
		days = append(days, byDate[date])
	}
	return days
}

// logLineOf is what a line of the table says of what the Log's rules decide,
// as a moment wants it.
func logLineOf(l views.LogLine) viewstest.LogLine {
	if r := l.Rule; r != nil {
		return viewstest.LogLine{Rule: r.Ended.Date + " → " + r.Began.Date}
	}
	r := l.Row
	row := viewstest.Row{
		Request: r.Request, Session: r.Session, Dir: r.Dir, At: r.At.Local().Format(time.TimeOnly), Mark: r.Mark, Stage: r.Stage,
		Failed: r.Failed, Rewrote: r.Rewrote, First: secondsOf(r.First), Took: secondsOf(r.Took),
		Stopped: r.Stopped().String(), Phone: r.PhoneStopped().String(), Routing: r.Routing.Words.String(),
	}
	if r.Routing.Cost != nil {
		row.Cost = *r.Routing.Cost
	}
	return viewstest.LogLine{Row: row}
}

// secondsOf is a time a row knows, as prose.Seconds says it: "" for one it
// doesn't.
func secondsOf(d *time.Duration) string {
	if d == nil {
		return ""
	}
	return prose.Seconds(*d)
}

func TestTheLogsRowsAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			table := views.LogRows(logSourcesOf(hm))
			var got []viewstest.LogLine
			for _, l := range table.Lines {
				got = append(got, logLineOf(l))
			}
			if !reflect.DeepEqual(got, hm.Want) {
				t.Errorf("the table is\n%s\nwant\n%s", logSaid(got), logSaid(hm.Want))
			}
			for _, l := range table.Lines {
				if r := l.Rule; r != nil && !reflect.DeepEqual(r.Totals, ruleFigures(hm.Totals, r.Ended.Date)) {
					t.Errorf("the rule after %s says %+v, want the day's figures, %+v", r.Ended.Date, r.Totals, ruleFigures(hm.Totals, r.Ended.Date))
				}
			}
			dates := slices.Concat([]string{views.Today(hm.From).Date, views.Today(hm.Now).Date}, slices.Collect(maps.Keys(hm.Counts)))
			for _, date := range dates {
				if got, want := table.Count(date), hm.Counts[date]; got != want {
					t.Errorf("%s counts %+v, want %+v", date, got, want)
				}
			}
		})
	}
}

// ruleFigures are the figures of the day with the given date among those
// given: none but its date where they hold none.
func ruleFigures(totals []views.DayTotals, date string) views.DayTotals {
	if i := slices.IndexFunc(totals, func(t views.DayTotals) bool { return t.Date == date }); i >= 0 {
		return totals[i]
	}
	return views.DayTotals{Date: date}
}

// logSaid says a table's lines, a line each, for a failure to show.
func logSaid(lines []viewstest.LogLine) string {
	said := make([]string, len(lines))
	for i, l := range lines {
		if l.Rule != "" {
			said[i] = "── " + l.Rule
			continue
		}
		said[i] = strings.TrimSpace(strings.Join([]string{
			l.Row.Mark.String(), l.Row.Request, stageNames[l.Row.Stage], l.Row.At, l.Row.Session, l.Row.Dir,
			"first " + l.Row.First, "took " + l.Row.Took, "failed " + boolSaid(l.Row.Failed), "rewrote " + boolSaid(l.Row.Rewrote),
			"stopped " + l.Row.Stopped, "phone " + l.Row.Phone, "routing " + l.Row.Routing, "cost " + l.Row.Cost.Cents(),
		}, " | "))
	}
	return strings.Join(said, "\n")
}

func boolSaid(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// agreeing is a moment the List and the Log are held to each other at: the
// lines the ledger holds, the router's sessions, whether no router lists
// them, what the stream told, and when the table starts and both are read.
type agreeing struct {
	name      string
	lines     []ledger.Line
	sessions  []status.Session
	unlisted  bool
	stream    views.Stream
	from, now time.Time
}

// agreeingAt are the moments of the ledger and the stream, and those of the
// sessions, the table of each of the latter spanning every day its lines
// are of.
func agreeingAt() []agreeing {
	var at []agreeing
	for _, hm := range viewstest.LogMoments() {
		at = append(at, agreeing{name: hm.Name, lines: hm.Lines, sessions: hm.Sessions, stream: hm.Stream, from: hm.From, now: hm.Now})
	}
	for _, hm := range viewstest.SessionMoments() {
		from := hm.Now
		for _, l := range hm.Lines {
			if l.At.Before(from) {
				from = l.At
			}
		}
		at = append(at, agreeing{name: hm.Name, lines: hm.Lines, sessions: hm.Sessions, unlisted: hm.Unlisted, stream: hm.Stream, from: from, now: hm.Now})
	}
	return at
}

// TestTheListAndTheLogAgreeOnASessionsLatestRequest holds the List's mark
// and routing to the Log's row of each listed session's latest request, at
// each hard moment of the ledger and the stream, and of the sessions, where
// both apply: the row in flight, the session's one request in flight, or the
// row's move the one that brought the session's listed model to its account.
func TestTheListAndTheLogAgreeOnASessionsLatestRequest(t *testing.T) {
	compared := 0
	for _, hm := range agreeingAt() {
		table := views.LogRows(logSourcesOf(viewstest.LogMoment{Lines: hm.lines, Stream: hm.stream, From: hm.from, Now: hm.now}))
		list := views.ListSessions(views.SessionSources{
			Running: hm.sessions, Unlisted: hm.unlisted, Stream: hm.stream, Ledger: ledgerHolding(hm.now, hm.lines...), Prices: ledger.Pricing, Now: hm.now,
		})
		for _, s := range list.Sessions {
			row, flying, ok := latestOf(table, s.Session)
			if !ok || !bothApply(row, flying, s) {
				continue
			}
			compared++
			if s.Mark != row.Mark || row.Mark == views.MarkMoved && !reflect.DeepEqual(s.Routing, row.Routing) {
				t.Errorf("%s: the List marks %s %q, routed %q, but the Log marks its latest request %q, routed %q", hm.name,
					s.Session[:4], s.Mark, routingWords(s.Routing), row.Mark, routingWords(row.Routing))
			}
		}
	}
	if compared < 16 {
		t.Errorf("the List and the Log are compared at %d rows, want at least the 16 of the moments asking, answering and moved", compared)
	}
}

// latestOf is the row of the latest request of the session with the given
// id among the table's, as it places them, and how many of its requests are
// in flight: false where it holds none.
func latestOf(table views.LogTable, session string) (latest views.RequestRow, flying int, ok bool) {
	for _, l := range table.Lines {
		if l.Rule != nil || l.Row.Session != session || l.Row.Kind != views.RowRequest {
			continue
		}
		latest, ok = l.Row, true
		if l.Row.Stage.InFlight() {
			flying++
		}
	}
	return latest, flying, ok
}

// bothApply reports whether the row's mark says what the List's says of the
// session s: in flight, where it's the session's one request in flight; a
// move, where it brought the session's listed model to its account.
func bothApply(row views.RequestRow, flying int, s views.ListedSession) bool {
	switch row.Mark {
	case views.MarkAsking, views.MarkAnswering:
		return flying == 1
	case views.MarkMoved:
		return row.Model == s.Model && row.Account == s.Account
	}
	return false
}

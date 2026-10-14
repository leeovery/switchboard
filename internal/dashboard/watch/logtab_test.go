package watch

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// daysCounted is a request ledger followed as it grows, as followed notes
// its reads of today's, which notes too the dates of the days read whole.
type daysCounted struct {
	followed
	days []string
}

func (l *daysCounted) Day(date string) []ledger.Held {
	l.days = append(l.days, date)
	return l.Ledger.Day(date)
}

// onTheLog has h show the Log, reading the request ledger in the state
// directory given, as daysCounted counts its reads.
func onTheLog(h *harness, state string) *daysCounted {
	l := &daysCounted{}
	l.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	h.model.cfg.Ledger, h.model.view = l, dashboard.Log
	return l
}

// shownRequests are the ids of the requests the Log's Requests show, the
// oldest first.
func shownRequests(h *harness) []string {
	var ids []string
	for _, l := range h.model.log.page.Requests.Lines {
		if l.IsRow() {
			ids = append(ids, l.Row.Request)
		}
	}
	return ids
}

// logState is the Log's state line as drawn, the screen's last row but one,
// without its margins.
func (h *harness) logState() string {
	rows := strings.Split(h.view(), "\n")
	return strings.TrimSpace(rows[len(rows)-2])
}

// onTheDay is a time of the day the clock starts on, in the local zone the
// ledger files its days by.
func onTheDay(hour, minute, second int) time.Time {
	return time.Date(start.Year(), start.Month(), start.Day(), hour, minute, second, 0, time.Local)
}

// arrivedAt is arrived, arriving at the time given.
func arrivedAt(request string, at time.Time) ledger.Held {
	l := arrived(request, "work", 0)
	l.At = at
	return l
}

func TestTheLogReadsTodaysLinesOnceThenWhatEachLookGains(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	h := newHarness(t, probedWithoutTheRouter())
	l := onTheLog(h, state)
	h.start()

	writeLines(t, state, arrived("0a000003", "work", -3*time.Minute))
	h.read(probedWithoutTheRouter())
	if !slices.Equal(l.given, []int{2, 1}) || len(l.days) > 0 {
		t.Errorf("today's lines were read giving %v, and the days %v read whole; want them read once as the Log opened, then what the look gained, [2 1], and no day", l.given, l.days)
	}
	if got, want := shownRequests(h), []string{"0a000001", "0a000002", "0a000003"}; !slices.Equal(got, want) {
		t.Errorf("the Requests show %v, want %v", got, want)
	}
}

// At midnight, Today gives today's lines afresh: the Log keeps the day
// before's, and reads it again whole, for the line of a request across
// midnight, filed under the day it arrived on.
func TestTheLogKeepsTheDayBeforesLinesAsTheDayTurns(t *testing.T) {
	state := t.TempDir()
	night := onTheDay(23, 59, 30)
	writeLines(t, state, arrivedAt("0a000001", night.Add(-10*time.Minute)))
	h := newHarness(t, probedWithoutTheRouter())
	l := onTheLog(h, state)
	h.clock.now = night
	h.start()

	h.clock.now = night.Add(time.Minute)
	writeLines(t, state, arrivedAt("0a000002", night.Add(20*time.Second)), arrivedAt("0a000003", night.Add(45*time.Second)))
	h.read(probedWithoutTheRouter())
	if want := []string{dayfile.DateOf(night)}; !slices.Equal(l.days, want) {
		t.Errorf("past midnight, the days %v were read whole, want %v", l.days, want)
	}
	if got, want := shownRequests(h), []string{"0a000001", "0a000002", "0a000003"}; !slices.Equal(got, want) {
		t.Errorf("past midnight, the Requests show %v, want %v: the day before's kept, the request across midnight among them", got, want)
	}
}

// Away from the Log over two midnights, the tail read last on the day the
// Log opened, the Log reads again whole every day from that one to the day
// before today: none of those between is lost.
func TestTheLogReadsEveryDayItWasAwayForAgain(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -time.Minute))
	h := newHarness(t, probedWithoutTheRouter())
	l := onTheLog(h, state)
	h.start()

	h.keys(tabKey)
	writeLines(t, state, arrived("0a000002", "work", day), arrived("0a000003", "work", 2*day-time.Minute))
	h.clock.now = start.Add(2 * day)
	h.read(probedWithoutTheRouter())
	h.keys(shiftTabKey)
	if want := []string{dayfile.DateOf(start), dayfile.DateOf(start.Add(day))}; !slices.Equal(l.days, want) {
		t.Errorf("back on the Log two days on, the days %v were read whole, want %v", l.days, want)
	}
	if got, want := shownRequests(h), []string{"0a000001", "0a000002", "0a000003"}; !slices.Equal(got, want) {
		t.Errorf("back on the Log two days on, the Requests show %v, want %v", got, want)
	}
}

// A request that arrived at 23:59:50 and ended at 00:00:12, after the first
// look past midnight read the day before again, has its line taken in, shown
// or hidden: the stream told it ended, so the day it arrived on is read again
// whole at the next look, as its line is filed under that day, which today's
// tail never reads; and that day's figures read again, its rule counting it.
func TestALineOfTheDayBeforeWrittenAfterTheTurnComesAsTheStreamTellsItEnded(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		requests int
	}{
		{name: "a request, shown", kind: ledger.KindMessage, requests: 2},
		{name: "a quota check, hidden as the Log opens", kind: ledger.KindCheck, requests: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			night := onTheDay(23, 59, 30)
			writeLines(t, state, arrivedAt("0a000001", night.Add(-10*time.Minute)))
			h := routedHarness(t, routerDocument(three()...))
			l := onTheLog(h, state)
			h.clock.now = night
			h.start()

			arrival := night.Add(20 * time.Second)
			at := func(kind string, when time.Time) router.StreamEvent {
				return alter(told(kind, "0a000002", "work", 0), func(e *router.StreamEvent) { e.At, e.Check = when.UTC(), tt.kind == ledger.KindCheck })
			}
			h.hear(at(router.StreamSent, arrival.Add(100*time.Millisecond)))
			h.clock.now = night.Add(35 * time.Second)
			h.read(routerDocument(three()...))

			line := took(arrivedAt("0a000002", arrival), 22*time.Second)
			line.Kind = tt.kind
			writeLines(t, state, line)
			ended := line.At.Add(22 * time.Second)
			h.hear(alter(at(router.StreamDone, ended), func(e *router.StreamEvent) { e.Status = 200 }))
			h.clock.now = night.Add(45 * time.Second)
			h.read(routerDocument(three()...))

			yesterday := dayfile.DateOf(night)
			if want := []string{yesterday, yesterday}; !slices.Equal(l.days, want) {
				t.Errorf("the days %v were read whole, want %v: the day before at the turn, then again as the stream told its request ended", l.days, want)
			}
			if got := h.model.log.read.WrittenTo(); !got.Equal(ended) {
				t.Errorf("the Log's read holds lines written to %s, want %s, the line of the request across midnight taken in", got, ended)
			}
			if i := slices.IndexFunc(h.model.log.totals, func(d views.DayTotals) bool { return d.Date == yesterday }); i < 0 || h.model.log.totals[i].Requests != tt.requests {
				t.Errorf("the figures of the day before are %+v, want them of %d requests", h.model.log.totals, tt.requests)
			}
		})
	}
}

func TestTheLogReadsBackADayAtATimeToTheOldestDayKept(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state,
		arrived("0a000001", "work", -3*day), arrived("0a000002", "work", -2*day), arrived("0a000003", "work", -day),
		arrived("0a000004", "work", -time.Minute),
	)
	h := newHarness(t, probedWithoutTheRouter())
	l := onTheLog(h, state)
	h.model.cfg.LedgerKeep = 2 * day
	h.start()

	if want := []string{dayfile.DateOf(past(-day)), dayfile.DateOf(past(-2 * day))}; !slices.Equal(l.days, want) {
		t.Errorf("short of its view, the Log read back the days %v, want %v, a day at a time, no further than the oldest its keep of 2 days keeps", l.days, want)
	}
}

// The Events read back as they want to fill their view no further than the
// first day the router filed events on: days before show none, so a ledger
// of quiet days is never read back through for them.
func TestTheEventsReadBackNoFurtherThanTheRouterFiledEvents(t *testing.T) {
	tests := []struct {
		name  string
		filed []time.Time
		want  []string
	}{
		{name: "none filed", want: nil},
		{name: "filed 3 days ago", filed: []time.Time{past(-3 * day)}, want: []string{dayfile.DateOf(past(-day)), dayfile.DateOf(past(-2 * day)), dayfile.DateOf(past(-3 * day))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			var lines []ledger.Held
			for i := range 60 {
				lines = append(lines, arrived(fmt.Sprintf("0b%06d", i), "work", -time.Duration(60-i)*time.Second))
			}
			for d := 1; d <= 30; d++ {
				lines = append(lines, arrived(fmt.Sprintf("0c%06d", d), "work", -time.Duration(d)*day))
			}
			writeLines(t, state, lines...)
			for i, at := range tt.filed {
				writeEvents(t, state, events.Line{ID: i + 1, At: at, Kind: status.EventPin, Accounts: []string{"work"}, Run: at.Add(-time.Hour)})
			}
			h := newHarness(t, probedWithoutTheRouter())
			l := onTheLog(h, state)
			h.model.cfg.Events = events.NewReader(state, h.clock.Now, logs.For("watch"))
			h.start()
			if len(l.days) > 0 {
				t.Fatalf("its view filled, the Requests read back the days %v, want none", l.days)
			}

			h.typed("]")
			if !slices.Equal(l.days, tt.want) {
				t.Errorf("on Events, the days %v were read back, want %v", l.days, tt.want)
			}
		})
	}
}

// An event the router filed after the last look at its document, as it
// stopped or restarted, is read off its files: the next look finds the
// router gone, or another run's document, which tells of none of it.
func TestTheEventsReadWhatTheRouterFiledAsItStoppedOrRestarted(t *testing.T) {
	run := past(-time.Hour)
	tests := []struct {
		name string
		then func(h *harness)
	}{
		{name: "stopped", then: (*harness).stopRouter},
		{name: "restarted", then: func(h *harness) {
			h.source.health = router.Health{OK: true, PID: 2, StartedAt: past(time.Minute)}
			h.startRouter(routerDocument(three()...))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, arrived("0a000001", "work", -time.Minute))
			pinned := status.Event{ID: 1, At: past(-30 * time.Minute), Kind: status.EventPin, Accounts: []string{"work"}}
			writeEvents(t, state, events.Line{Event: pinned, Run: run})
			doc := routerDocument(three()...)
			doc.Events = []status.Event{pinned}
			h := routedHarness(t, doc)
			h.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
			onTheLog(h, state)
			h.model.cfg.Events = events.NewReader(state, h.clock.Now, logs.For("watch"))
			h.start()
			h.typed("]")

			h.clock.now = past(2 * time.Second)
			writeEvents(t, state, events.Line{ID: 2, At: past(time.Second), Kind: status.EventPin, Accounts: []string{"side"}, Run: run})
			tt.then(h)
			h.typed("R")
			var shown []int
			for _, e := range h.model.log.page.Events.Rows {
				shown = append(shown, e.Key.ID)
			}
			if !slices.Equal(shown, []int{1, 2}) {
				t.Errorf("the router %s, the Events show the events %v, want [1 2], the one it filed after the last look among them", tt.name, shown)
			}
		})
	}
}

// writeEvents files the router's events given in the state directory given,
// each under the day it happened on, as the router files them as they're told.
func writeEvents(t *testing.T, state string, lines ...events.Line) {
	t.Helper()
	for _, line := range lines {
		data, err := events.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(ledger.Dir(state), "events-"+dayfile.DateOf(line.At)+".jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(append(data, '\n'))
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// ↑ pauses the Log at its newest line, chosen; lines come since are counted
// under the paused foot; and end follows again. Paused, the key line puts
// what acts on the line chosen together, f and / apart.
func TestTheLogPausesOnTheLineChosenAndFollowsAgain(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()

	h.keys(tea.KeyPressMsg{Code: tea.KeyUp})
	writeLines(t, state, arrived("0a000003", "work", -3*time.Minute))
	h.typed("R")
	if got := h.logState(); !strings.HasPrefix(got, "‖ paused at ") || !strings.Contains(got, "▼ 1 new   end to follow") {
		t.Errorf("paused, a line come since, the state line reads %q, want it paused, 1 new", got)
	}
	l := h.model.live(h.clock.now).KeyLine
	var got [][]string
	for _, group := range l.Groups {
		got = append(got, listedKeys(group))
	}
	want := [][]string{{"tab"}, {"[ ]"}, {"↑↓", "enter", "s", "end"}, {"f", "/"}, {"p"}, {"?", "q"}}
	if !slices.EqualFunc(got, want, slices.Equal) || !slices.Equal(listedKeys(l.Phone), []string{"↑↓", "enter", "end", "[ ]"}) {
		t.Errorf("paused, the key line is %q, a phone's keeping %q; want %q, and [↑↓ enter end [ ]]", got, listedKeys(l.Phone), want)
	}

	h.keys(tea.KeyPressMsg{Code: tea.KeyEnd})
	if got := h.logState(); !strings.HasPrefix(got, "● following") {
		t.Errorf("after end, the state line reads %q, want following", got)
	}
}

// An event's moved count is a number of the Events', brightening as a look
// raises it; the counts the heading row gives are none.
func TestAnEventsMovedCountBrightensAsALookRaisesIt(t *testing.T) {
	capped := func(moved int) status.Document {
		doc := routerDocument(three()...)
		doc.Events = []status.Event{{
			ID: 1, At: past(-time.Minute), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, Count: moved, To: "side",
		}}
		return doc
	}
	h := routedHarness(t, capped(2))
	h.model.view = dashboard.Log
	h.start()
	h.typed("]")

	h.startRouter(capped(3))
	h.typed("R")
	got := h.brightened()
	if len(got) != 1 || !slices.ContainsFunc(slices.Collect(maps.Keys(got)), func(n dashboard.Number) bool { return n.Kind == dashboard.NumberMoved }) {
		t.Errorf("the cap's sessions moved rising from 2 to 3, the numbers brightened are %v, want its count alone", got)
	}
}

// On a terminal too short for the Log's table, its view has no row: each key
// that moves the table, g from an event's details, and the find's landing on
// a session ended, still pause it as their rules have it, crashing nothing.
func TestTheLogsKeysWorkOnATerminalWithNoRowForItsTable(t *testing.T) {
	tests := []struct {
		name  string
		press func(h *harness)
		shown dashboard.LogView
	}{
		{name: "↑", press: func(h *harness) { h.keys(upKey) }},
		{name: "k", press: func(h *harness) { h.typed("k") }},
		{name: "↑ on Events", press: func(h *harness) { h.typed("]"); h.keys(upKey) }, shown: dashboard.LogEvents},
		{name: "g from an event's details", press: func(h *harness) { h.typed("]"); h.keys(upKey, enter); h.typed("g") }},
		{name: "the find landing", press: func(h *harness) { h.typed("/", "a", "p", "i"); h.keys(enter) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, ranIn("0a000001", apiSession, "~/Code/api", -day), arrived("0a000002", "work", -5*time.Minute))
			doc := routerDocument(three()...)
			doc.Events = []status.Event{{ID: 1, At: past(-6 * time.Minute), Kind: status.EventPin, Accounts: []string{"work"}}}
			h := routedHarness(t, doc)
			onTheLog(h, state)
			h.deliver(h.update(tea.WindowSizeMsg{Width: 150, Height: 10})...)
			h.start()
			if rows := h.model.logRows(); rows != 0 {
				t.Fatalf("the Log's view has %d rows, want none", rows)
			}

			tt.press(h)
			p := h.model.log.page
			if !p.Paused() || p.Shown != tt.shown {
				t.Errorf("the table shown is %d, paused %t; want %d, paused", p.Shown, p.Paused(), tt.shown)
			}
		})
	}
}

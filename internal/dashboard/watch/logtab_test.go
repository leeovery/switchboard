package watch

import (
	"errors"
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
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// daysCounted is a request ledger followed as it grows, as followed notes
// its reads of today's, which notes too the dates of the days read whole,
// and of the days' summaries read: the first each read from a day is from,
// and each read by its date.
type daysCounted struct {
	followed
	days, summed []string
}

func (l *daysCounted) Day(date string) []ledger.Held {
	l.days = append(l.days, date)
	return l.Ledger.Day(date)
}

func (l *daysCounted) Days(from time.Time) []ledger.Summary {
	l.summed = append(l.summed, dayfile.DateOf(from))
	return l.Ledger.Days(from)
}

func (l *daysCounted) DaysOn(dates []string) []ledger.Summary {
	l.summed = append(l.summed, dates...)
	return l.Ledger.DaysOn(dates)
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

// The router drops a reader that falls behind: the Log fills the gap from
// the ledger as the stream joins again, so a request that ended while it was
// away shows as its line has it, never lost, with no look between.
func TestARejoinedStreamFillsTheGapFromTheLedger(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	h.hear(told(router.StreamSent, "0a000002", "work", 0))

	h.endStream()
	writeLines(t, state, took(arrived("0a000002", "work", 0), 2*time.Second))
	rejoin, _ := h.pendingRejoin()
	h.fire(rejoin)
	joining, ok := h.pending(joinedMsg{gen: h.model.stream.gen})
	if !ok {
		t.Fatal("the stream opened again, nothing waits for its first look")
	}
	h.fire(joining)

	i := slices.IndexFunc(h.model.log.page.Requests.Lines, func(l views.LogLine) bool { return l.IsRow() && l.Row.Request == "0a000002" })
	if i < 0 {
		t.Fatalf("the Requests show %v, want 0a000002 among them", shownRequests(h))
	}
	if row := h.model.log.page.Requests.Lines[i].Row; row.Stage != views.RowLined || row.Failed {
		t.Errorf("the stream joined again, 0a000002, which ended while it was away, is %+v, want it as its line has it", row)
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

// A request in flight across midnight, whose line is written to the day it
// arrived on after a look past midnight on the Log read that day again, has
// its line taken in while the Overview shows as it does at first, ROUTING
// hearing the stream and SESSIONS hidden, so folding no tail of its own:
// the stream keeps the request, ended, till a read of the ledger's tail
// reads its line, and the tail is followed at each look whatever shows, the
// Log awaiting its line on the day before. Back on the Log ten minutes on,
// its row is its line's, and the stream has let it go.
func TestALineOfTheDayBeforeComesWhileAnotherTabShows(t *testing.T) {
	state := t.TempDir()
	night := onTheDay(23, 59, 30)
	writeLines(t, state, arrivedAt("0a000001", night.Add(-10*time.Minute)))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.clock.now = night
	h.start()

	arrival := night.Add(20 * time.Second)
	at := func(kind string, when time.Time) router.StreamEvent {
		return alter(told(kind, "0a000002", "work", 0), func(e *router.StreamEvent) { e.At = when.UTC() })
	}
	h.hear(at(router.StreamSent, arrival.Add(100*time.Millisecond)))
	h.clock.now = night.Add(35 * time.Second)
	h.read(routerDocument(three()...))
	h.keys(tabKey)
	if h.model.view != dashboard.Overview {
		t.Fatalf("tab from the Log shows %s, want the Overview", h.model.view)
	}

	line := took(arrivedAt("0a000002", arrival), 22*time.Second)
	writeLines(t, state, line)
	ended := line.At.Add(22 * time.Second)
	h.hear(alter(at(router.StreamDone, ended), func(e *router.StreamEvent) { e.Status = 200 }))
	h.tickUntil(night.Add(10 * time.Minute))
	h.keys(shiftTabKey)

	if got := h.model.log.read.WrittenTo(); !got.Equal(ended) {
		t.Errorf("back on the Log, its read holds lines written to %s, want %s, the line of the request across midnight taken in", got, ended)
	}
	if _, ok := toldOf(h.model.traffic, "0a000002"); ok {
		t.Error("its line read, the stream keeps the request still, want it let go")
	}
}

// A request's row stays where it sat in flight as its line takes over,
// though its line arrived a moment before it was sent and another's line
// lies between: the stream lets it go only once the Log has placed its line
// where the stream told it was sent.
func TestARowStaysWhereItSatInFlightAsTheStreamLetsItsRequestGo(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, took(arrived("0a000002", "work", 200*time.Millisecond), time.Second))
	doc := routerDocument(three()...)
	h := routedHarness(t, doc)
	onTheLog(h, state)
	h.clock.now = past(3 * time.Second)
	h.start()
	h.hear(told(router.StreamSent, "0a000001", "work", 500*time.Millisecond),
		alter(told(router.StreamDone, "0a000001", "work", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))
	want := []string{"0a000002", "0a000001"}
	if got := shownRequests(h); !slices.Equal(got, want) {
		t.Fatalf("0a000001 ended, its line unread, the Log shows %q, want %q", got, want)
	}

	writeLines(t, state, took(arrived("0a000001", "work", 0), 2*time.Second))
	h.read(doc)
	if _, ok := toldOf(h.model.traffic, "0a000001"); ok {
		t.Fatal("its line read, the stream keeps 0a000001 still, want it let go")
	}
	if got := shownRequests(h); !slices.Equal(got, want) {
		t.Errorf("0a000001's line read, the Log shows %q, want %q, its row where it sat in flight", got, want)
	}
}

// The Log keeps up while another tab shows: ten minutes on History, the
// requests that came meanwhile, their lines written, and the events the
// router told of, are on the Log as it shows again, before any read lands.
func TestTheLogKeepsUpWhileAnotherTabShows(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000000", "work", -time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	h.keys(shiftTabKey, shiftTabKey, shiftTabKey)
	if h.model.view != dashboard.History {
		t.Fatalf("shift-tab three times from the Log shows %s, want History", h.model.view)
	}

	doc := routerDocument(three()...)
	want := []string{"0a000000"}
	for i := range 10 {
		sent := time.Duration(i+1) * time.Minute
		id := fmt.Sprintf("0a%06d", i+1)
		writeLines(t, state, took(arrived(id, "work", sent), 20*time.Second))
		doc.Events = append(doc.Events, status.Event{ID: i + 1, At: past(sent), Kind: status.EventPin, Accounts: []string{"side"}})
		h.startRouter(doc)
		h.clock.now = past(sent + 30*time.Second)
		h.read(doc)
		want = append(want, id)
	}
	h.update(tabKey)
	h.update(tabKey)
	h.update(tabKey)

	if h.model.view != dashboard.Log {
		t.Fatalf("tab three times from History shows %s, want the Log", h.model.view)
	}
	if got := shownRequests(h); !slices.Equal(got, want) {
		t.Errorf("back on the Log, before any read lands, the Requests show %v, want %v", got, want)
	}
	if got := len(h.model.log.page.Events.Rows); got != 10 {
		t.Errorf("back on the Log, before any read lands, the Events show %d, want the 10 the router told of", got)
	}
}

// A request that arrived at 23:59:58 and first went upstream at 00:00:01
// has its line filed under the day it arrived on, written after the turn:
// the stream tells no arrival, so the Log awaits its line on the day before
// the one it was sent on, reading that day again whole as the stream tells
// it ended.
func TestALineOfARequestSentJustAfterMidnightComesOnTheDayItArrived(t *testing.T) {
	state := t.TempDir()
	night := onTheDay(23, 59, 30)
	writeLines(t, state, arrivedAt("0a000001", night.Add(-10*time.Minute)))
	h := routedHarness(t, routerDocument(three()...))
	l := onTheLog(h, state)
	h.clock.now = night
	h.start()

	at := func(kind string, when time.Time) router.StreamEvent {
		return alter(told(kind, "0a000002", "work", 0), func(e *router.StreamEvent) { e.At = when.UTC() })
	}
	h.hear(at(router.StreamSent, onTheDay(24, 0, 1)))
	h.clock.now = onTheDay(24, 0, 5)
	h.read(routerDocument(three()...))

	line := took(arrivedAt("0a000002", onTheDay(23, 59, 58)), 20*time.Second)
	writeLines(t, state, line)
	ended := line.At.Add(20 * time.Second)
	h.hear(alter(at(router.StreamDone, ended), func(e *router.StreamEvent) { e.Status = 200 }))
	h.clock.now = onTheDay(24, 0, 25)
	h.read(routerDocument(three()...))
	yesterday := dayfile.DateOf(night)
	if want := []string{yesterday, yesterday}; !slices.Equal(l.days, want) {
		t.Errorf("the days %v were read whole, want %v: the day before at the turn, then again as the stream told its request ended", l.days, want)
	}
	if got := h.model.log.read.WrittenTo(); !got.Equal(ended) {
		t.Errorf("the Log's read holds lines written to %s, want %s, the line of the request sent after midnight taken in", got, ended)
	}
}

// A day awaiting a line is read whole again only as its line could have
// come: at the look after a request whose line is filed on it ended, then
// once a minute while none comes, as for one written late; never at every
// look. Its figures alone are read again, the ledger's days read from it,
// not from the first the tables span.
func TestADayAwaitingALineIsReadAgainOnlyAsItsLineCouldHaveCome(t *testing.T) {
	state := t.TempDir()
	twoBefore := onTheDay(23, 50, 0).AddDate(0, 0, -1)
	writeLines(t, state, arrivedAt("0a000001", twoBefore), arrivedAt("0a000002", onTheDay(23, 50, 0)))
	h := routedHarness(t, routerDocument(three()...))
	l := onTheLog(h, state)
	h.clock.now = onTheDay(24, 0, 30)
	h.start()
	if h.model.log.from.Date != dayfile.DateOf(twoBefore) {
		t.Fatalf("the Log spans from %s as it opens, want %s, the first day the ledger keeps", h.model.log.from.Date, dayfile.DateOf(twoBefore))
	}

	at := func(kind, request string, when time.Time) router.StreamEvent {
		return alter(told(kind, request, "work", 0), func(e *router.StreamEvent) { e.At = when.UTC() })
	}
	ended := func(request string, when time.Time) router.StreamEvent {
		return alter(at(router.StreamDone, request, when), func(e *router.StreamEvent) { e.Status = 200 })
	}
	sent := onTheDay(23, 59, 50)
	h.hear(at(router.StreamSent, "0a000003", sent), at(router.StreamSent, "0a000004", sent))
	opened, summed := len(l.days), len(l.summed)
	yesterday := dayfile.DateOf(sent)
	steps := []struct {
		name     string
		ends     string
		from, to time.Time
		readings int
	}{
		{name: "the looks after the first ended", ends: "0a000003", from: onTheDay(24, 0, 45), to: onTheDay(24, 1, 10), readings: 1},
		{name: "the look after the second ended", ends: "0a000004", from: onTheDay(24, 1, 15), to: onTheDay(24, 1, 15), readings: 2},
		{name: "the looks of the minute after", from: onTheDay(24, 1, 20), to: onTheDay(24, 2, 10), readings: 2},
		{name: "a minute on", from: onTheDay(24, 2, 15), to: onTheDay(24, 2, 15), readings: 3},
	}
	for _, s := range steps {
		if s.ends != "" {
			h.hear(ended(s.ends, s.from.Add(-3*time.Second)))
		}
		for h.clock.now = s.from; !h.clock.now.After(s.to); h.clock.now = h.clock.now.Add(lookEvery) {
			h.read(routerDocument(three()...))
		}
		if got := l.days[opened:]; !slices.Equal(got, slices.Repeat([]string{yesterday}, s.readings)) {
			t.Errorf("after %s, the days %v were read whole, want the day before %d times", s.name, got, s.readings)
		}
	}
	for _, from := range l.summed[summed:] {
		if from != yesterday {
			t.Errorf("the days' summaries were read from %v, want from the day before alone", l.summed[summed:])
			break
		}
	}
}

// madeAt is where the slice s begins, which a slice made afresh begins
// elsewhere than: nil where it's empty.
func madeAt[T any](s []T) *T {
	if len(s) == 0 {
		return nil
	}
	return &s[0]
}

// The Log's tables are made again only as what they're made of changes: a
// frame or the pointer moving makes neither afresh, the clock moved on a
// tenth, a request in flight or not; the stream telling of a request makes
// the Requests again; a look the Events only where it tells of an event
// anew, not where it tells of those it told before alike; a look whose read
// of the ledger's tail gains a line both, as a move's cost on the Events is
// its session's story; and the filter changing the table it filters. Its panel, open, is made again as what it picks changes, not as
// the pointer moves over it, nor its cursor; and of the Requests, as a
// line comes, not at a look listing the sessions alike and gaining none,
// nor as an answer streams on.
func TestTheLogsTablesAreMadeAgainOnlyAsWhatTheyreMadeOfChanges(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	doc := routerDocument(three()...)
	doc.Events = []status.Event{{ID: 1, At: past(-time.Minute), Kind: status.EventPin, Accounts: []string{"personal"}}}
	h := routedHarness(t, doc)
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", time.Second)}
	onTheLog(h, state)
	h.start()

	type made struct{ requests, events, panel any }
	madeNow := func() made {
		p := h.model.log.page
		return made{madeAt(p.Requests.Lines), madeAt(p.Events.Lines), madeAt(h.model.log.panel.Sessions)}
	}
	unmoved := func() {
		h.clock.now = h.clock.now.Add(100 * time.Millisecond)
		h.deliver(frameMsg{gen: h.model.frames}, tea.MouseMotionMsg{X: 10, Y: 10})
	}
	steps := []struct {
		name                    string
		step                    func()
		requests, events, panel bool
	}{
		{name: "a frame and the pointer moving", step: unmoved},
		{name: "the stream telling of a request", step: func() { h.hear(told(router.StreamSent, "0a000003", "work", 0)) }, requests: true},
		{name: "a look, the router telling of the events it told of before", step: func() { h.read(doc) }},
		{name: "a look telling of an event", step: func() {
			doc.Events = append(slices.Clone(doc.Events), status.Event{ID: 2, At: past(0), Kind: status.EventPin, Accounts: []string{"side"}})
			h.startRouter(doc)
			h.read(doc)
		}, events: true},
		{name: "a look whose tail read gains a line", step: func() {
			writeLines(t, state, arrived("0a000004", "work", 0))
			h.read(doc)
		}, requests: true, events: true},
		{name: "the panel opening", step: func() { h.typed("f") }, panel: true},
		{name: "the pointer moving over the panel, and its cursor", step: func() {
			unmoved()
			h.keys(tea.KeyPressMsg{Code: tea.KeyRight})
		}},
		{name: "a look under the panel, listing the sessions alike, its tail gaining nothing", step: func() {
			h.source.sessions = slices.Clone(h.source.sessions)
			h.tickUntilAsked()
		}},
		{name: "an answer streaming on under the panel", step: func() {
			h.hear(alter(told(router.StreamProgress, "0a000003", "work", time.Second), func(e *router.StreamEvent) { e.Chars = 400 }))
		}},
		{name: "a look under the panel whose tail read gains a line", step: func() {
			writeLines(t, state, arrived("0a000005", "work", 0))
			h.tickUntilAsked()
		}, requests: true, events: true, panel: true},
		{name: "a tick on the panel", step: func() { h.keys(spaceKey) }, panel: true},
		{name: "what the panel picks applied", step: func() { h.keys(enter) }, requests: true},
	}
	for _, s := range steps {
		before := madeNow()
		s.step()
		after := madeNow()
		if got := [3]bool{after.requests != before.requests, after.events != before.events, after.panel != before.panel}; got != [3]bool{s.requests, s.events, s.panel} {
			t.Errorf("after %s, the Requests, the Events and the panel were made again: %v, want %v", s.name, got, [3]bool{s.requests, s.events, s.panel})
		}
	}
}

// A request in flight counts how long it has taken frame by frame, a tenth
// at a time, the Requests it's among made not once more as it does.
func TestARequestInFlightCountsOnWithoutItsTableMadeAgain(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	h.hear(told(router.StreamSent, "0a000003", "work", 0))

	made := madeAt(h.model.log.page.Requests.Lines)
	for range 10 {
		h.clock.now = h.clock.now.Add(100 * time.Millisecond)
		h.deliver(frameMsg{gen: h.model.frames})
		if took := prose.Seconds(h.clock.now.Sub(past(0))); !strings.Contains(h.view(), took) {
			t.Fatalf("at %s, the row in flight doesn't read %s:\n%s", h.clock.now.Format(time.StampMilli), took, h.view())
		}
	}
	if madeAt(h.model.log.page.Requests.Lines) != made {
		t.Error("over ten frames a tenth apart, a request in flight, the Requests were made again, want them made once")
	}
}

// Paused, a request in flight's took counts on each second, as following;
// behind a line's details card, the page stands still, its took with it,
// and no frame is drawn for it.
func TestARowInFlightCountsOnPausedAndStandsStillBehindACard(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	h.hear(told(router.StreamSent, "0a000003", "work", 0))
	h.keys(tea.KeyPressMsg{Code: tea.KeyUp})
	if !h.model.log.page.Paused() {
		t.Fatal("after ↑, the Log follows, want it paused")
	}

	for _, at := range []time.Duration{time.Second, 2 * time.Second} {
		h.tickUntil(past(at + 50*time.Millisecond))
		if took := prose.Seconds(h.clock.now.Sub(past(0))); !strings.Contains(h.view(), took) {
			t.Fatalf("paused, at %s, the row in flight doesn't read %s:\n%s", h.clock.now.Format(time.StampMilli), took, h.view())
		}
	}

	h.keys(tea.KeyPressMsg{Code: tea.KeyEnter})
	opened := h.view()
	h.tickUntil(past(5 * time.Second))
	if _, ok := h.pendingFrame(); ok {
		t.Error("the card open over the Log, a frame is armed, want none: the page behind it stands still")
	}
	if got := h.view(); got != opened || strings.Contains(got, prose.Seconds(5*time.Second)) {
		t.Errorf("3s on, behind the card, the screen is\n%s\nwant it as the card opened\n%s", got, opened)
	}
}

// The stream telling how far a request's answer has streamed makes nothing
// afresh, its row counting its characters as it's drawn; its end makes the
// Requests again, once.
func TestARequestsProgressMakesNothingAfreshItsEndOnce(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	h.hear(told(router.StreamSent, "0a000003", "work", 0))
	h.hear(told(router.StreamFirst, "0a000003", "work", time.Second))

	made := madeAt(h.model.log.page.Requests.Lines)
	for i := range 10 {
		chars := (i + 1) * 500
		h.clock.now = past(time.Second + time.Duration(i+1)*250*time.Millisecond)
		h.hear(alter(told(router.StreamProgress, "0a000003", "work", h.clock.now.Sub(start)), func(e *router.StreamEvent) { e.Chars = chars }))
		if want := "answering · " + prose.Characters(chars) + " characters"; !strings.Contains(h.view(), want) {
			t.Fatalf("%d characters streamed, the row doesn't read %q:\n%s", chars, want, h.view())
		}
	}
	if madeAt(h.model.log.page.Requests.Lines) != made {
		t.Error("ten times told how far a request's answer has streamed, the Requests were made again, want them made once")
	}
	h.hear(alter(told(router.StreamDone, "0a000003", "work", 5*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))
	ended := madeAt(h.model.log.page.Requests.Lines)
	if ended == made {
		t.Error("told the request ended, the Requests weren't made again, want its row ended")
	}
	h.clock.now = past(6 * time.Second)
	h.deliver(frameMsg{gen: h.model.frames})
	if madeAt(h.model.log.page.Requests.Lines) != ended {
		t.Error("a frame after the request ended made the Requests again, want them made the once")
	}
}

// The figures of the days read are folded into those held by their dates,
// the oldest first, each in place of the one held of its day, those held
// left as they were.
func TestTheFiguresReadAreFoldedIntoThoseHeldByTheirDays(t *testing.T) {
	counted := func(totals []views.DayTotals) []string {
		var dated []string
		for _, d := range totals {
			dated = append(dated, fmt.Sprintf("%s %d", d.Date, d.Requests))
		}
		return dated
	}
	day := func(date string, requests int) views.DayTotals {
		return views.DayTotals{Date: date, Requests: requests}
	}
	held := []views.DayTotals{day("2026-09-26", 1), day("2026-09-28", 1)}
	tests := []struct {
		name string
		read []views.DayTotals
		want []string
	}{
		{name: "none read", want: []string{"2026-09-26 1", "2026-09-28 1"}},
		{name: "a day held, read again", read: []views.DayTotals{day("2026-09-28", 2)}, want: []string{"2026-09-26 1", "2026-09-28 2"}},
		{
			name: "days before, between and after", read: []views.DayTotals{day("2026-09-25", 3), day("2026-09-27", 3), day("2026-09-29", 3)},
			want: []string{"2026-09-25 3", "2026-09-26 1", "2026-09-27 3", "2026-09-28 1", "2026-09-29 3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := counted(foldedTotals(held, tt.read)); !slices.Equal(got, tt.want) {
				t.Errorf("folded, the figures are %v, want %v", got, tt.want)
			}
			if got := counted(held); !slices.Equal(got, []string{"2026-09-26 1", "2026-09-28 1"}) {
				t.Errorf("the figures held became %v, want them left as they were", got)
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

// firstFailing is a request ledger followed as it grows, as daysCounted
// counts its reads, whose ask of where its lines begin fails while failing
// is set.
type firstFailing struct {
	*daysCounted
	failing bool
}

func (l *firstFailing) FirstLinesDay(now time.Time) (time.Time, error) {
	if l.failing {
		return time.Time{}, errors.New("the ledger's first day couldn't be read")
	}
	return l.daysCounted.FirstLinesDay(now)
}

// The oldest day the Log reads back to is asked again as the day turns:
// past midnight, the oldest day of the ledger's keep before it is no longer
// kept, whether or not the router has pruned its lines yet. An ask that
// fails is asked again at the next look, as on the first day, rather than
// leaving the day before's oldest day kept for the rest of the day.
func TestTheOldestDayKeptIsAskedAgainAsTheDayTurns(t *testing.T) {
	tests := []struct {
		name string
		// failed is how many of the looks past midnight fail to ask, the
		// first of them, before the last.
		failed int
	}{
		{name: "its first look past midnight asking"},
		{name: "its first look past midnight failing to ask, the next asking", failed: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			night := onTheDay(23, 50, 0)
			writeLines(t, state,
				arrivedAt("0a000001", night.AddDate(0, 0, -3)), arrivedAt("0a000002", night.AddDate(0, 0, -2)),
				arrivedAt("0a000003", night.AddDate(0, 0, -1)), arrivedAt("0a000004", night),
			)
			h := routedHarness(t, routerDocument(three()...))
			l := &firstFailing{daysCounted: onTheLog(h, state)}
			h.model.cfg.Ledger = l
			h.model.cfg.LedgerKeep = 2 * day
			h.clock.now = night
			h.start()
			if got, want := h.model.log.oldest.Day.Date, dayfile.DateOf(night.AddDate(0, 0, -2)); got != want {
				t.Fatalf("the oldest day kept is %s, want %s, its keep of 2 days back", got, want)
			}

			for i := range tt.failed + 1 {
				l.failing = i < tt.failed
				h.clock.now = onTheDay(24, 0, 5+i)
				h.read(routerDocument(three()...))
			}
			if got, want := h.model.log.oldest.Day.Date, dayfile.DateOf(night.AddDate(0, 0, -1)); got != want {
				t.Errorf("past midnight, the oldest day kept is %s, want %s, a day on", got, want)
			}
		})
	}
}

// summariesAsked is a request ledger followed as it grows, as daysCounted
// counts its reads, each read of its days' summaries telling, of every day,
// as many sessions on each of its accounts as there have been reads of
// them, its requests as they are: the figures of a day tell which read
// priced them.
type summariesAsked struct {
	daysCounted
	asked int
}

func (l *summariesAsked) Days(from time.Time) []ledger.Summary {
	return l.told(l.daysCounted.Days(from))
}

func (l *summariesAsked) DaysOn(dates []string) []ledger.Summary {
	return l.told(l.daysCounted.DaysOn(dates))
}

// told are the days a read of their summaries gave, as the read tells them.
func (l *summariesAsked) told(days []ledger.Summary) []ledger.Summary {
	l.asked++
	ids := make([]string, l.asked)
	for i := range ids {
		ids[i] = fmt.Sprintf("s%d", i)
	}
	for i := range days {
		days[i].Accounts = slices.Clone(days[i].Accounts)
		for a := range days[i].Accounts {
			days[i].Accounts[a].Sessions, days[i].Accounts[a].SessionIDs = l.asked, ids
		}
	}
	return days
}

// Reading back a day prices its figures alone, folded into those held: the
// days read back before aren't priced again with each. The first read of
// the summaries is how far back the Requests' lines reach, which bounds
// them, so the day before today is priced by the second.
func TestReadingBackADayPricesItsFiguresAlone(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state,
		arrived("0a000001", "work", -3*day), arrived("0a000002", "work", -2*day), arrived("0a000003", "work", -day),
		arrived("0a000004", "work", -time.Minute),
	)
	h := newHarness(t, probedWithoutTheRouter())
	l := &summariesAsked{daysCounted: *onTheLog(h, state)}
	h.model.cfg.Ledger = l
	h.start()

	var got []string
	for _, d := range h.model.log.totals {
		got = append(got, fmt.Sprintf("%s %d", d.Date, d.Sessions))
	}
	want := []string{dayfile.DateOf(past(-3*day)) + " 4", dayfile.DateOf(past(-2*day)) + " 3", dayfile.DateOf(past(-day)) + " 2"}
	if !slices.Equal(got, want) {
		t.Errorf("read back to the oldest day, the figures held are %v, want %v: each day's priced as it was read back, alone", got, want)
	}
}

// Opened on a terminal too short for its table, or with a row or two for
// it, which today's lines fill, the Log reads nothing back; grown, it reads
// back to fill the rows it's given, as it follows.
func TestTheLogOpenedShortReadsBackAsItsViewGrows(t *testing.T) {
	tests := []struct {
		name   string
		height int
		rows   int
	}{
		{name: "too short for its table", height: 10, rows: 0},
		{name: "a row for its table", height: 11, rows: 1},
		{name: "three rows for its table", height: 13, rows: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state,
				arrived("0a000001", "work", -day),
				arrived("0a000002", "work", -3*time.Minute), arrived("0a000003", "work", -2*time.Minute), arrived("0a000004", "work", -time.Minute),
			)
			h := newHarness(t, probedWithoutTheRouter())
			l := onTheLog(h, state)
			h.deliver(h.update(tea.WindowSizeMsg{Width: 150, Height: tt.height})...)
			h.start()
			if got := h.model.logRows(); got != tt.rows {
				t.Fatalf("%d rows tall, the Log's view has %d rows, want %d", tt.height, got, tt.rows)
			}
			if len(l.days) > 0 {
				t.Fatalf("with %d rows for its table, today's lines filling them, the Log read back the days %v, want none", tt.rows, l.days)
			}

			h.deliver(h.update(tea.WindowSizeMsg{Width: 150, Height: 50})...)
			if want := []string{dayfile.DateOf(past(-day))}; !slices.Equal(l.days, want) {
				t.Errorf("grown, the Log read back the days %v, want %v, to fill its view", l.days, want)
			}
		})
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

// A burst of more events than the router gives in its document, between two
// looks at it, skips those no document tells of: the Log reads them off the
// router's files, from the look before, as it does as the router stops.
func TestTheEventsReadABurstTheDocumentSkippedOffTheFiles(t *testing.T) {
	run := past(-time.Hour)
	pinned := func(id int, at time.Time) status.Event {
		return status.Event{ID: id, At: at, Kind: status.EventPin, Accounts: []string{"work"}}
	}
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -time.Minute))
	first := pinned(1, past(-30*time.Minute))
	writeEvents(t, state, events.Line{Event: first, Run: run})
	doc := routerDocument(three()...)
	doc.Events = []status.Event{first}
	h := routedHarness(t, doc)
	h.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
	onTheLog(h, state)
	h.model.cfg.Events = events.NewReader(state, h.clock.Now, logs.For("watch"))
	h.start()
	h.typed("]")

	var burst []status.Event
	want := []int{1}
	for id := 2; id <= 61; id++ {
		e := pinned(id, past(time.Duration(id)*time.Second))
		burst, want = append(burst, e), append(want, id)
		writeEvents(t, state, events.Line{Event: e, Run: run})
	}
	h.clock.now = past(2 * time.Minute)
	doc.Events = burst[len(burst)-50:]
	h.startRouter(doc)
	h.typed("R")
	var shown []int
	for _, e := range h.model.log.page.Events.Rows {
		shown = append(shown, e.Key.ID)
	}
	if !slices.Equal(shown, want) {
		t.Errorf("60 events told between two looks, the document giving the newest 50, the Events show %v, want %v", shown, want)
	}
}

// The events looks take in while a read of the router's files is under way
// are kept as it lands, what it found taken in alongside them; and a skip
// found meanwhile is read from the earliest moment any skip wants, so none
// told is lost.
func TestEventsTakenInWhileAReadIsUnderWayAreKept(t *testing.T) {
	run := past(-time.Hour)
	var told []status.Event
	for id := 1; id <= 160; id++ {
		told = append(told, status.Event{ID: id, At: past(time.Duration(id) * time.Second), Kind: status.EventPin, Accounts: []string{"work"}})
	}
	filed := func(es []status.Event) []events.Line {
		var lines []events.Line
		for _, e := range es {
			lines = append(lines, events.Line{Event: e, Run: run})
		}
		return lines
	}
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -time.Minute))
	writeEvents(t, state, filed(told[:10])...)
	doc := routerDocument(three()...)
	doc.Events = told[:10]
	h := routedHarness(t, doc)
	h.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
	onTheLog(h, state)
	h.model.cfg.Events = events.NewReader(state, h.clock.Now, logs.For("watch"))
	h.clock.now = past(10*time.Second + 500*time.Millisecond)
	h.source.router.GeneratedAt = h.clock.now.UTC()
	h.start()
	h.typed("]")

	// look files the events from to to as told, and looks at the router's
	// document at at, which gives the newest 50 of them, holding back the
	// Log's reads it asks.
	look := func(at time.Duration, from, to int) []tea.Msg {
		writeEvents(t, state, filed(told[from:to])...)
		h.clock.now = past(at)
		d := doc
		d.Events, d.GeneratedAt = told[max(to-50, 0):to], h.clock.now.UTC()
		h.startRouter(d)
		return h.holdingReads(h.press("R")...)
	}
	underWay := look(71*time.Second, 10, 70)
	if len(underWay) != 1 {
		t.Fatalf("the document skipping 11 to 20, the look asked %d reads of the Log's own, want one, of the files", len(underWay))
	}
	if more := look(101*time.Second, 70, 100); len(more) != 0 {
		t.Fatal("a look asked a read while one was under way")
	}
	if more := look(161*time.Second, 100, 160); len(more) != 0 {
		t.Fatal("a look asked a read while one was under way")
	}
	h.deliver(underWay...)
	h.typed("R")

	var missing []int
	for id := 1; id <= 160; id++ {
		if !slices.ContainsFunc(h.model.log.page.Events.Rows, func(e views.LogEvent) bool { return e.Key.ID == id }) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		t.Errorf("160 events told, each filed, looks taking in 71 to 160 while a read of the files was under way, the Events miss %v", missing)
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

// Help lists esc where it acts on the Log, as the key line leaves it to
// help: paused, following again, as end does; following with a filter on,
// clearing it; and nowhere else.
func TestTheLogsHelpListsEscWhereItActs(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, ranIn("0a000001", apiSession, "~/Code/api", -day), arrived("0a000002", "work", -5*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	steps := []struct {
		name  string
		press func()
		want  []string
	}{
		{name: "following", press: func() {}},
		{name: "paused", press: func() { h.keys(upKey) }, want: []string{"follow"}},
		{name: "following again", press: func() { h.keys(escKey) }},
		{name: "filtered, following", press: func() { h.typed("/", "a", "p", "i"); h.keys(enter, tea.KeyPressMsg{Code: tea.KeyEnd}) }, want: []string{"clear the filter"}},
		{name: "filtered, paused", press: func() { h.keys(upKey) }, want: []string{"follow"}},
	}
	for _, step := range steps {
		step.press()
		var escs []string
		for _, k := range helpOver(h.model) {
			if k.Key == "esc" {
				escs = append(escs, k.Does)
			}
		}
		if !slices.Equal(escs, step.want) {
			t.Errorf("%s, help lists esc for %q, want %q", step.name, escs, step.want)
		}
		rows := strings.Split(h.view(), "\n")
		if line := rows[len(rows)-1]; strings.Contains(line, "esc") {
			t.Errorf("%s, the key line reads %q, want esc left to help", step.name, line)
		}
	}
	if !h.model.log.page.Filtered() {
		t.Error("after the find landed, the Log isn't filtered")
	}
}

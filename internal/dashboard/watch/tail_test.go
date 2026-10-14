package watch

import (
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// Where the Log stands as a session's page reads the ledger's tail: never
// shown, opened then left for another tab, or shown under the page.
type logBeside int

const (
	logNever logBeside = iota
	logBehind
	logUnder
)

// pageBeside is a model probing three accounts without the router, so the
// traces, which keep a mark of their own, never read the request ledger;
// reading it in the state directory given, as daysCounted counts its reads,
// with d28c's page open, the Log standing as log says.
func pageBeside(t *testing.T, state string, log logBeside) (*harness, *daysCounted) {
	t.Helper()
	h := newHarness(t, probedWithoutTheRouter())
	l := onTheLog(h, state)
	if log == logNever {
		h.model.view = dashboard.Overview
	}
	h.start()
	if log == logBehind {
		h.keys(tabKey)
	}
	h.openPage(idD28C)
	return h, l
}

// A session's page takes in the lines each look's read of the ledger's tail
// gains, one read of today's lines for every reader of them: the page alone,
// the Log never shown; the Log opened, then left for another tab, which
// takes them in all the same, as the tail's mark moves past them for both;
// and the Log under the page.
func TestASessionsPageTakesInWhatEachLookReadsOfTheTailForEveryReader(t *testing.T) {
	tests := []struct {
		name string
		log  logBeside
	}{
		{name: "the Log never shown", log: logNever},
		{name: "the Log opened, then left", log: logBehind},
		{name: "over the Log", log: logUnder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
			h, l := pageBeside(t, state, tt.log)

			gained := arrived("0a000003", "work", -3*time.Minute)
			writeLines(t, state, gained)
			h.read(probedWithoutTheRouter())
			if want := []int{2, 1}; !slices.Equal(l.given, want) {
				t.Errorf("today's lines were read giving %v, want %v: today's whole once, then once what the look gained", l.given, want)
			}
			if got := h.shownPage().Page.Totals.Requests; got != 3 {
				t.Errorf("after the look, the page counts %d requests, want 3, the line gained taken in", got)
			}
			if got := h.model.log.read.WrittenTo(); tt.log != logNever && !got.Equal(gained.At) {
				t.Errorf("after the look, the Log's read holds lines written to %s, want %s, the line gained taken in", got, gained.At)
			}
		})
	}
}

// readingTheTail has h's model ask a read of the ledger's tail, as a look
// does, returning the read, not yet run.
func (h *harness) readingTheTail() tea.Cmd {
	h.t.Helper()
	h.model.tail.due = true
	looked, read := h.model.Update(tea.WindowSizeMsg{Width: h.model.size.Width, Height: h.model.size.Height})
	h.model = looked.(Model)
	if !h.model.tail.reading {
		h.t.Fatal("the look asked no read of the tail")
	}
	return read
}

// deliverHolding gives the model msgs, and whatever they send back, until
// they're all delivered, but those hold picks, which it returns undelivered.
func (h *harness) deliverHolding(hold func(tea.Msg) bool, msgs ...tea.Msg) []tea.Msg {
	h.t.Helper()
	var held []tea.Msg
	for len(msgs) > 0 {
		msg := msgs[0]
		msgs = msgs[1:]
		if hold(msg) {
			held = append(held, msg)
			continue
		}
		msgs = append(msgs, h.update(msg)...)
	}
	return held
}

// isTailed and isLogRead pick a read of the ledger's tail, and of the Log's
// own, as they land.
func isTailed(msg tea.Msg) bool {
	_, ok := msg.(tailedMsg)
	return ok
}

func isLogRead(msg tea.Msg) bool {
	_, ok := msg.(logReadMsg)
	return ok
}

// One read of today's lines at a time: a session's page asked to open while
// a read of the ledger's tail is under way reads its lines once that lands,
// or never, where esc dropped it meanwhile; no read of the tail starts while
// the page's is under way, nor while a read of the Log's own is. So the page,
// and the Log, take in every line either read.
func TestASessionsPagesReadAndTheTailsReadNeverRunTogether(t *testing.T) {
	onTheLogOf := func(t *testing.T) (*harness, string) {
		state := t.TempDir()
		writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
		h := routedHarness(t, routerDocument(three()...))
		onTheLog(h, state)
		h.start()
		return h, state
	}

	t.Run("a page asked while the tail is read, dropped before it lands", func(t *testing.T) {
		h, _ := onTheLogOf(t)
		tail := h.readingTheTail()
		opened, read := h.model.openSession(idD28C)
		h.model = opened
		h.keys(escKey)
		h.deliver(h.run(tail)...)
		if read != nil || h.model.session != nil {
			t.Errorf("the page's read ran %t, and it opened %t; want neither", read != nil, h.model.session != nil)
		}
	})

	t.Run("a page asked while the tail is read", func(t *testing.T) {
		h, state := onTheLogOf(t)
		tail := h.readingTheTail()
		opened, read := h.model.openSession(idD28C)
		h.model = opened
		writeLines(t, state, arrived("0a000002", "side", -4*time.Minute))
		h.deliver(h.run(tail)...)
		if read != nil || h.model.session == nil || h.shownPage().Page.Totals.Requests != 2 {
			t.Errorf("the page's read ran beside the tail's %t, and once that landed the page opened %t; want it open after, of both lines", read != nil, h.model.session != nil)
		}
	})

	t.Run("a look while a page's read is under way", func(t *testing.T) {
		h, state := onTheLogOf(t)
		opened, read := h.model.openSession(idD28C)
		h.model = opened
		writeLines(t, state, arrived("0a000002", "side", -4*time.Minute))
		h.model.tail.due = true
		h.update(tea.WindowSizeMsg{Width: h.model.size.Width, Height: h.model.size.Height})
		if h.model.tail.reading {
			t.Fatal("the tail was read beside the page's read, want it waiting till that lands")
		}
		h.deliver(h.run(read)...)
		if h.model.tail.due || h.shownPage().Page.Totals.Requests != 2 {
			t.Errorf("the page's read landed, the tail read %t, the page counting %d requests; want the tail read then, the page of both lines",
				!h.model.tail.due, h.shownPage().Page.Totals.Requests)
		}
	})

	t.Run("a look over the Log while a read of its own is under way", func(t *testing.T) {
		h, state := onTheLogOf(t)
		h.openPage(idD28C)
		h.stopRouter()
		refiling := h.deliverHolding(isLogRead, h.press("R")...)
		if len(refiling) == 0 || !h.model.log.reading {
			t.Fatal("the router gone, the Log read none of its own, want its events' files read again")
		}
		writeLines(t, state, arrived("0a000002", "side", -4*time.Minute))
		tailed := h.deliverHolding(isTailed, h.press("R")...)
		if len(tailed) > 0 {
			t.Error("a look read the tail beside the Log's own read, want it waiting till that lands")
		}
		h.deliver(slices.Concat(refiling, tailed)...)
		h.read(probedWithoutTheRouter())
		if got, want := shownRequests(h), []string{"0a000001", "0a000002"}; !slices.Equal(got, want) || h.shownPage().Page.Totals.Requests != 2 {
			t.Errorf("the reads landed, the Log shows %v and the page counts %d requests; want %v, and 2", got, h.shownPage().Page.Totals.Requests, want)
		}
	})
}

// Past midnight, a session's page takes in what its session's day before
// gained after the tail last read it: the day read again whole at the first
// look past the turn, then again as the stream tells a request across
// midnight ended, its line filed under the day it arrived on; each once for
// every reader. The first look rolls the page held on to the new day, so no
// look after prices it afresh, as it takes nothing new of today.
func TestASessionsPageTakesInTheDayBeforeAsTheDayTurns(t *testing.T) {
	tests := []struct {
		name string
		log  logBeside
	}{
		{name: "the page alone", log: logNever},
		{name: "over the Log", log: logUnder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			night := onTheDay(23, 59, 30)
			writeLines(t, state, arrivedAt("0a000001", night.Add(-10*time.Minute)))
			h := routedHarness(t, routerDocument(three()...))
			l := onTheLog(h, state)
			if tt.log == logNever {
				h.model.view = dashboard.Overview
			}
			h.clock.now = night
			h.start()
			h.openPage(idD28C)

			writeLines(t, state, arrivedAt("0a000003", night.Add(5*time.Second)))
			arrival := night.Add(20 * time.Second)
			sent := func(kind string, when time.Time) router.StreamEvent {
				return alter(told(kind, "0a000002", "work", 0), func(e *router.StreamEvent) { e.At = when.UTC() })
			}
			h.hear(sent(router.StreamSent, arrival.Add(100*time.Millisecond)))
			h.clock.now = night.Add(35 * time.Second)
			h.read(routerDocument(three()...))
			if held := h.model.session.held; !reflect.DeepEqual(held, held.Take(views.Today(h.clock.now).Date, nil)) {
				t.Error("the first look past midnight left the page held at the day before, want it rolled on to the new day")
			}

			line := took(arrivedAt("0a000002", arrival), 22*time.Second)
			writeLines(t, state, line)
			ended := line.At.Add(22 * time.Second)
			h.hear(alter(sent(router.StreamDone, ended), func(e *router.StreamEvent) { e.Status = 200 }))
			h.clock.now = night.Add(45 * time.Second)
			h.read(routerDocument(three()...))

			yesterday := dayfile.DateOf(night)
			if want := []string{yesterday, yesterday}; !slices.Equal(l.days, want) {
				t.Errorf("the days %v were read whole, want %v: the day before at the turn, then as the stream told its request ended, once for every reader", l.days, want)
			}
			if got := h.shownPage().Page.Totals.Requests; got != 3 {
				t.Errorf("the page counts %d requests by their lines, want 3, the day before's last and the request across midnight's taken in", got)
			}
			if got := h.model.log.read.WrittenTo(); tt.log == logUnder && !got.Equal(ended) {
				t.Errorf("the Log's read holds lines written to %s, want %s", got, ended)
			}
		})
	}
}

package watch

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// enter opens the details of the Log's line chosen over it; ↑ steps the card
// to the line before, the lit line following; esc closes it, the table left
// paused at the line it was of.
func TestTheLogsDetailsCardStepsThroughTheLines(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()

	h.keys(tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(h.view(), "├ id           0a000002") {
		t.Errorf("enter on the newest line opened\n%s\nwant its details, 0a000002's", h.view())
	}
	h.keys(tea.KeyPressMsg{Code: tea.KeyUp})
	if row, _, _ := h.model.log.page.Chosen(); row.Request != "0a000001" || !strings.Contains(h.view(), "├ id           0a000001") {
		t.Errorf("after ↑, the line chosen is %s and the screen\n%s\nwant 0a000001 chosen, its details in the card", row.Request, h.view())
	}
	h.keys(tea.KeyPressMsg{Code: tea.KeyEscape})
	at := past(-5 * time.Minute).In(time.Local).Format(time.TimeOnly)
	if got := h.logState(); h.model.open != nothingOpen || !strings.HasPrefix(got, "‖ paused at "+at) {
		t.Errorf("after esc, the state line reads %q, want the card closed, the table paused at %s", got, at)
	}
}

// On a terminal too short for the card by its line, the wheel over it, as j
// does, scrolls its body; off it, nothing.
func TestTheWheelScrollsTheLogsCardUnderThePointer(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	h.update(tea.WindowSizeMsg{Width: 150, Height: 16})
	h.keys(tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyEnter})
	f, _, _ := h.model.screen(h.clock.now)
	most := f.CardScrolling().Most
	if most == 0 {
		t.Fatal("the card scrolls no rows, want some")
	}
	left, top := h.drawnAt("╭─ REQUEST")

	h.wheel(tea.MouseWheelDown, left-1, top+2)
	off := h.model.log.card.scroll
	h.wheel(tea.MouseWheelDown, left+4, top+2)
	if over, want := h.model.log.card.scroll, min(wheelRows, most); off != 0 || over != want {
		t.Errorf("a turn down off the card scrolled it %d rows, and over it %d; want none, then %d", off, over, want)
	}
}

// A card that lifted the table to fit under its line, closed, leaves the
// table as scrolling reaches it: no blank rows under its paused foot, the
// line it was of still in view.
func TestTheLogsDetailsCardClosedLeavesTheTableAsScrollingReachesIt(t *testing.T) {
	state := t.TempDir()
	for i := range 40 {
		writeLines(t, state, arrived(fmt.Sprintf("0a0000%02d", i+1), "work", time.Duration(i-40)*time.Minute))
	}
	h := routedHarness(t, routerDocument(three()...))
	onTheLog(h, state)
	h.start()
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.keys(slices.Repeat([]tea.KeyPressMsg{{Code: tea.KeyUp}}, 12)...)
	h.keys(tea.KeyPressMsg{Code: tea.KeyEnter}, tea.KeyPressMsg{Code: tea.KeyEscape})

	rows := strings.Split(h.view(), "\n")
	_, heads := h.drawnAt("  at  ")
	table := rows[heads+1 : heads+1+h.model.logRows()]
	at := past(-12 * time.Minute).In(time.Local).Format(time.TimeOnly)
	blank := slices.IndexFunc(table, func(row string) bool { return strings.TrimSpace(row) == "" })
	if blank >= 0 || !slices.ContainsFunc(table, func(row string) bool { return strings.Contains(row, at) }) {
		t.Errorf("closed, the table reads\n%s\nwant no blank row in it, and %s, the line chosen, in view", strings.Join(table, "\n"), at)
	}
}

// A cap's card that fits under its line with no row to spare keeps its place
// as a look tells of its moves and it grows, its body scrolling within it,
// its line left in sight.
func TestTheLogsEventCardKeepsItsPlaceAsItGrows(t *testing.T) {
	capped := status.Event{ID: 1, At: past(-time.Minute), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, To: "side", Until: past(3 * time.Hour)}
	doc := routerDocument(three()...)
	doc.Events = []status.Event{capped}
	h := routedHarness(t, doc)
	h.model.view = dashboard.Log
	h.start()
	h.update(tea.WindowSizeMsg{Width: 150, Height: 21})
	h.typed("]")
	h.keys(tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyEnter})
	_, line := h.drawnAt("╎ 13:11:00")
	_, top := h.drawnAt("╭─ EVENT")

	capped.Count = 2
	doc.Events = []status.Event{capped,
		{ID: 2, At: past(-50 * time.Second), Kind: status.EventMoved, Session: idD28C, Model: opus, From: "work", To: "side", ForcedBy: 1},
		{ID: 3, At: past(-40 * time.Second), Kind: status.EventMoved, Session: "7f3a0b2c-0000-4000-8000-000000000002", Model: opus, From: "work", To: "side", ForcedBy: 1},
	}
	h.startRouter(doc)
	h.tickUntilAsked()
	_, lineNow := h.drawnAt("╎ 13:11:00")
	_, topNow := h.drawnAt("╭─ EVENT")
	if lineNow != line || topNow != top || !strings.Contains(h.view(), "j k scroll") {
		t.Errorf("grown by two moves, the card's top is on row %d and its line on %d, the screen\n%s\nwant them on %d and %d, the card scrolling", topNow+1, lineNow+1, h.view(), top+1, line+1)
	}
}

// An event's card follows each look, the moves its cap counted growing in
// it while the page behind stands still; g shows Requests, the card closed.
func TestTheLogsEventCardFollowsEachLook(t *testing.T) {
	capped := status.Event{ID: 1, At: past(-time.Minute), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, To: "side"}
	doc := routerDocument(three()...)
	doc.Events = []status.Event{capped}
	h := routedHarness(t, doc)
	h.model.view = dashboard.Log
	h.start()
	h.typed("]")
	h.keys(tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyEnter})

	capped.Count = 1
	moved := status.Event{ID: 2, At: past(-30 * time.Second), Kind: status.EventMoved, Session: idD28C, Model: opus, From: "work", To: "side", ForcedBy: 1}
	doc.Events = []status.Event{capped, moved}
	h.startRouter(doc)
	h.tickUntilAsked()
	if view := h.view(); !strings.Contains(view, "└ d28c         moved to Side") || strings.Contains(view, "its session moves") {
		t.Errorf("after a look told of the cap's move, the screen is\n%s\nwant the move among what followed in the card, and the cap's row behind it as it stood", view)
	}

	h.typed("g")
	if h.model.open != nothingOpen || h.model.log.page.Shown != dashboard.LogRequests {
		t.Errorf("after g, the card open is %v and the Log shows %v, want none open, and Requests", h.model.open, h.model.log.page.Shown)
	}
}

// An event's card opened within views.FactsReach of the first day the Log
// has read reads the day before, though its tables fill their views, so
// what told of the event shows: a cap just after midnight, known from a
// request before it.
func TestAnEventsCardReadsBackTheDayItsFactsBeganOn(t *testing.T) {
	midnight := onTheDay(0, 0, 0)
	known := arrivedAt("0a000001", midnight.Add(-5*time.Minute))
	known.Limits = map[string]string{"5h-utilization": "0.96", "status": "allowed"}
	lines := []ledger.Held{known}
	events := []status.Event{
		{ID: 1, At: midnight.Add(30 * time.Second), Kind: status.EventPin, Accounts: []string{"personal"}},
		{ID: 2, At: midnight.Add(time.Minute), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05},
	}
	for i := range 12 {
		at := midnight.Add(time.Duration(2+i) * time.Minute)
		lines = append(lines, arrivedAt(fmt.Sprintf("0a1000%02d", i), at))
		events = append(events, status.Event{ID: 3 + i, At: at, Kind: status.EventStarted, Account: "side", Session: idD28C})
	}
	state := t.TempDir()
	writeLines(t, state, lines...)
	h := routedHarness(t, status.Document{})
	h.update(tea.WindowSizeMsg{Width: 150, Height: 16})
	h.clock.now = midnight.Add(20 * time.Minute)
	doc := routerDocument(three()...)
	doc.GeneratedAt, doc.Events = h.clock.now.UTC(), events
	h.startRouter(doc)
	l := onTheLog(h, state)
	h.model.cfg.LedgerKeep = 2 * day
	h.start()
	h.typed("]")
	h.keys(slices.Repeat([]tea.KeyPressMsg{{Code: tea.KeyUp}}, 13)...)
	if len(l.days) > 0 {
		t.Fatalf("the Log read back %v before the card opened, want none: its tables fill their views", l.days)
	}

	h.keys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if want := []string{dayfile.DateOf(known.At)}; !slices.Equal(l.days, want) || !strings.Contains(h.view(), "known from   d28c's request at 23:55:00") {
		t.Errorf("the cap's card opened, the Log read back %v and the screen is\n%s\nwant %v read, its card known from d28c's request at 23:55:00", l.days, h.view(), want)
	}
}

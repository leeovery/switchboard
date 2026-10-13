package watch

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
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

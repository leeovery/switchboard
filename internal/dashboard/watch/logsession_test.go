package watch

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// lineChosen names the Log's line chosen: a request's by its id, an event's
// by its key; "" where none is.
func lineChosen(h *harness) string {
	row, event, ok := h.model.log.page.Chosen()
	switch {
	case !ok:
		return ""
	case event != nil:
		return fmt.Sprint(event.Key)
	}
	return row.Request
}

// s on the Log opens the page of the session its line chosen is of, from
// the table or from the line's details card, which s closes as it asks; esc
// on the page comes back to the Log, paused at that line, no card open. A
// quota check's line, which counts toward no session, and an account's
// event list no s, and s opens nothing on them.
func TestSOnTheLogOpensTheChosenLinesSessionAndEscComesBack(t *testing.T) {
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	check := arrived("0a000003", "work", -3*time.Minute)
	check.Kind = ledger.KindCheck
	tests := []struct {
		name   string
		events bool
		checks bool
		keys   []tea.KeyPressMsg
		card   bool
		opens  bool
	}{
		{name: "a request's line, from the table", keys: []tea.KeyPressMsg{upKey}, opens: true},
		{name: "a request's line, from its card", keys: []tea.KeyPressMsg{upKey, enter}, card: true, opens: true},
		{name: "a quota check's line", checks: true, keys: []tea.KeyPressMsg{upKey}},
		{name: "a session's event, from its card", events: true, keys: []tea.KeyPressMsg{upKey, enter}, card: true, opens: true},
		{name: "an account's event", events: true, keys: []tea.KeyPressMsg{upKey, upKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute), check)
			doc := routerDocument(three()...)
			doc.Events = []status.Event{
				{ID: 1, At: past(-2 * time.Minute), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, To: "side"},
				{ID: 2, At: past(-time.Minute), Kind: status.EventMoved, Session: idD28C, Model: opus, From: "work", To: "side"},
			}
			h := routedHarness(t, doc)
			onTheLog(h, state)
			if tt.checks {
				h.model.log.page.Filter.Kinds = append(h.model.log.page.Filter.Kinds, views.RowCheck)
			}
			h.start()
			if tt.events {
				h.typed("]")
			}
			h.keys(tt.keys...)
			chosen, asked := lineChosen(h), h.model.opening
			listed := slices.Contains(h.listed(), "s")
			if tt.card {
				listed = strings.Contains(h.view(), "s its session")
			}
			if listed != tt.opens {
				t.Errorf("%s chosen, s is listed %t, want %t", chosen, listed, tt.opens)
			}

			h.typed("s")
			if !tt.opens {
				if h.model.session != nil || h.model.opening != asked {
					t.Errorf("s on %s opened a page %t, asked for %d more; want none asked", chosen, h.model.session != nil, h.model.opening-asked)
				}
				return
			}
			if s := h.model.session; s == nil || s.id != idD28C || h.model.open != nothingOpen || h.model.view != dashboard.Log {
				t.Fatalf("s on %s left a page open %t, the card open %v, over %s; want d28c's page open over the Log, no card", chosen, s != nil, h.model.open, h.model.view)
			}
			h.keys(escKey)
			if got := lineChosen(h); h.model.session != nil || h.model.open != nothingOpen || !h.model.log.page.Paused() || got != chosen {
				t.Errorf("esc on the page, a page open %t, a card %v, the Log paused %t at %s; want the Log back, paused at %s, no card",
					h.model.session != nil, h.model.open, h.model.log.page.Paused(), got, chosen)
			}
		})
	}
}

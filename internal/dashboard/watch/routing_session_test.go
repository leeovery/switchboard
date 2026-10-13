package watch

import (
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// pagedID is the session whose page the session's card tests show.
const pagedID = "9c2e47b1-0d5a-4e83-b6f2-8a1c3e9d0f57"

// sessionPaged is the router of three, started, showing pagedID's page as it
// runs on personal, its own pin as given, the router listing it so and
// naming side as where auto would send it; rebuilding its cache elsewhere
// costs $1.56.
func sessionPaged(t *testing.T, pin string) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{{ID: pagedID, Pin: pin, Assignments: []status.Assignment{
		{Model: "claude-opus-5-5", Account: "personal", Auto: "side", Pinned: pin == "personal", AssignedAt: start, LastSeen: start},
	}}}
	h.start()
	cost := ledger.Picodollars(156 * 10_000_000_000)
	h.model.session = &sessionShown{shown: dashboard.SessionShown{Page: views.SessionPage{
		Session: pagedID, Dir: "~/Code/website", Account: "personal", Model: "claude-opus-5-5", Pin: pin, Auto: "side", MoveCost: &cost,
	}}}
	return h
}

func TestASessionsPageOpensItsCardPickingItsOwnPin(t *testing.T) {
	cost := ledger.Picodollars(156 * 10_000_000_000)
	for _, tt := range []struct {
		pin    string
		picked []string
		cursor int
	}{
		{pin: "", cursor: 0},
		{pin: "side", picked: []string{"side"}, cursor: 3},
	} {
		t.Run("its own pin "+tt.pin, func(t *testing.T) {
			h := sessionPaged(t, tt.pin)
			h.keyed("p")
			card := h.card()
			want := dashboard.RoutingSession{ID: pagedID, Dir: "~/Code/website", On: "personal", Auto: "side"}
			if h.model.open != routingOpen || card.Mode != dashboard.ThisSession || card.Session != want {
				t.Fatalf("after p, the card is open %v, routing %v %+v; want the session's card, routing %+v", h.model.open == routingOpen, card.Mode, card.Session, want)
			}
			if !slices.Equal(card.Picked, tt.picked) || card.Cursor != tt.cursor || card.Cost == nil || *card.Cost != cost {
				t.Errorf("the card picks %q, its cursor on row %d, costing %v; want %q, on row %d, costing $1.56", card.Picked, card.Cursor, card.Cost, tt.picked, tt.cursor)
			}
			if h.keyed("m"); h.card().Move {
				t.Error("m ticked moving the sessions running, want it doing nothing on a session's card")
			}
		})
	}
}

func TestEnterSendsTheSessionsPick(t *testing.T) {
	pin, unpin := "pin "+pagedID+" to side", "unpin "+pagedID
	tests := []struct {
		name string
		pin  string
		keys string
		// want is the order sent, as fakeSource notes it: none where
		// nothing is sent.
		want []string
	}{
		{name: "an account", keys: "3 enter", want: []string{pin}},
		{name: "one account alone, the last picked", keys: "2 3 enter", want: []string{pin}},
		{name: "by the cursor and space", keys: "down down down space enter", want: []string{pin}},
		{name: "m doing nothing", keys: "m 3 enter", want: []string{pin}},
		{name: "auto, clearing its own pin", pin: "side", keys: "0 enter", want: []string{unpin}},
		{name: "its own pin picked again, which keeps it", pin: "side", keys: "3 enter"},
		{name: "nothing changed", keys: "enter"},
		{name: "its own pin as it stands", pin: "side", keys: "enter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sessionPaged(t, tt.pin)
			looks, listed := h.source.looks(), h.source.listed
			h.keyed("p " + tt.keys)

			if !slices.Equal(h.source.orders, tt.want) {
				t.Errorf("orders = %q, want %q", h.source.orders, tt.want)
			}
			if h.model.open != nothingOpen {
				t.Error("after enter, the card is still open, want it closed")
			}
			wantLooks := 0
			if len(tt.want) > 0 {
				wantLooks = 1
			}
			if got, sessions := h.source.looks()-looks, h.source.listed-listed; got != wantLooks || sessions != wantLooks {
				t.Errorf("looked at the router's document %d times and its sessions %d, want %d: again at once, once the router took an order", got, sessions, wantLooks)
			}
		})
	}
}

func TestARefusedSessionOrderKeepsItsCardOpenSayingWhy(t *testing.T) {
	refusal := errors.New(`account "side" has no usable token, so nothing can go out on it`)
	h := sessionPaged(t, "")
	h.source.refuse = refusal
	h.keyed("p 3 enter")
	if h.model.open != routingOpen || h.card().Refused != refusal.Error() {
		t.Errorf("refused, the card is open %v, saying %q; want it open, saying %q", h.model.open == routingOpen, h.card().Refused, refusal)
	}
}

func TestASessionsCardClosesOnceItsSessionEnds(t *testing.T) {
	h := sessionPaged(t, "side")
	h.keyed("p 0")
	h.fire(h.lastTick())
	if h.model.open != routingOpen {
		t.Fatal("while the session runs, a tick closed its card, want it open")
	}

	h.model.session.shown.Page.Activity.Doing = views.Ended
	h.fire(h.lastTick())
	if h.model.open != nothingOpen || len(h.source.orders) > 0 {
		t.Errorf("once its session ended, the card is open %v, orders %q; want it closed, sending nothing", h.model.open == routingOpen, h.source.orders)
	}
}

func TestASessionsPageNotYetBuiltOpensNoCard(t *testing.T) {
	h := sessionPaged(t, "")
	h.model.session.shown.Page = views.SessionPage{}
	h.keyed("p")
	if h.model.open != nothingOpen {
		t.Error("p on a session's page naming no session opened a card, want none: it would route new sessions")
	}
}

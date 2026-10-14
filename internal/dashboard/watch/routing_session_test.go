package watch

import (
	"encoding/json"
	"errors"
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

// pagedID is the session whose page the session's card tests show.
const pagedID = "9c2e47b1-0d5a-4e83-b6f2-8a1c3e9d0f57"

// sessionPaged is the router of three, started, showing pagedID's page as
// its lines and the router's listing build it: running on personal in
// ~/Code/website, its own pin as given, the router naming side as where
// auto would send it; its last request a minute ago wrote the cache for an
// hour, so rebuilding it elsewhere costs $0.81.
func sessionPaged(t *testing.T, pin string) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{{ID: pagedID, Pin: pin, Assignments: []status.Assignment{
		{Model: opus, Account: "personal", Auto: "side", Pinned: pin == "personal", Dir: "~/Code/website", AssignedAt: start, LastSeen: start},
	}}}
	h.start()
	h.withLines([]ledger.Line{{
		At: start.Add(-time.Minute).UTC(), Kind: ledger.KindMessage, Session: pagedID, Model: opus, Account: "personal", Reason: "new",
		Status: 200, Attempts: 1, Prompt: "prompt-1", Class: ledger.ClassMain, Answer: ledger.Answer{Stop: "end_turn"}, Usage: json.RawMessage(hourUsage),
	}})
	h.openPage(pagedID)
	return h
}

func TestASessionsPageOpensItsCardPickingItsOwnPin(t *testing.T) {
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
			h.model.cfg.Home = "/home/dev"
			h.keyed("p")
			card := h.card()
			want := dashboard.RoutingSession{ID: pagedID, Dir: "~/Code/website", Home: "/home/dev", On: "personal", Auto: "side"}
			if h.model.open != routingOpen || card.Mode != dashboard.ThisSession || card.Session != want {
				t.Fatalf("after p, the card is open %v, routing %v %+v; want the session's card, routing %+v", h.model.open == routingOpen, card.Mode, card.Session, want)
			}
			if !slices.Equal(card.Picked, tt.picked) || card.Cursor != tt.cursor || card.Cost == nil || card.Cost.Cents() != "$0.81" {
				t.Errorf("the card picks %q, its cursor on row %d, costing %v; want %q, on row %d, costing $0.81", card.Picked, card.Cursor, card.Cost, tt.picked, tt.cursor)
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

// A session's card closes once its session ends, as the router lists it no
// more: in the Update that takes in the listing without it, as its page is
// built afresh of it, sending nothing.
func TestASessionsCardClosesOnceItsSessionEnds(t *testing.T) {
	h := sessionPaged(t, "side")
	h.keyed("p 0")
	h.fire(h.lastTick())
	if h.model.open != routingOpen {
		t.Fatal("while the session runs, a tick closed its card, want it open")
	}

	h.source.sessions = nil
	for asked := len(h.source.asked); len(h.source.asked) == asked; {
		tick := h.lastTick()
		h.clock.now, tick.fired = tick.due, true
		for msgs := []tea.Msg{tick.msg}; len(msgs) > 0; {
			msgs = append(msgs[1:], h.update(msgs[0])...)
			if views.Routed(h.model.sessions, pagedID) == nil && h.model.open != nothingOpen {
				t.Fatalf("the router listing it no more, the Update taking that in left its card open, want it closed")
			}
		}
	}
	if page := h.shownPage().Page; page.Runs() || len(h.source.orders) > 0 {
		t.Errorf("listed no more, the page runs %t, orders %q; want it ended, sending nothing", page.Runs(), h.source.orders)
	}
}

// A session's page under its card stands still, as the card found it, while
// the card's session follows each look: the account it runs on, and where
// auto would send it, as the router lists them since.
func TestASessionsPageUnderItsCardStandsStill(t *testing.T) {
	h := sessionPaged(t, "")
	h.keyed("p")
	under := h.view()
	if !strings.Contains(under, "runs on Personal") || !strings.Contains(under, "Side for now") {
		t.Fatalf("the card opened reads\n%s\nwant it running on personal, auto sending it to side", under)
	}

	h.source.sessions[0].Assignments[0].Account, h.source.sessions[0].Assignments[0].Auto = "work", "personal"
	h.tickUntilAsked()
	got := h.view()
	if behindCard(got) != behindCard(under) {
		t.Errorf("a look moved the session, and the page under its card reads\n%s\nwant it as the card opened over it\n%s", got, under)
	}
	if !strings.Contains(got, "runs on Work") || !strings.Contains(got, "Personal for now") {
		t.Errorf("a look moved the session, and the card reads\n%s\nwant it running on work, auto sending it to personal", got)
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

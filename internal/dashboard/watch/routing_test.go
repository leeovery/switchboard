package watch

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// namedKeys are the keys the routing card takes that the terminal sends by
// name, as a test names them.
var namedKeys = map[string]tea.KeyPressMsg{
	"up": upKey, "down": downKey, "space": spaceKey, "esc": escKey, "enter": {Code: tea.KeyEnter},
}

// keyed presses the keys given, a space between each, as "2 3 m enter", in
// turn, delivering what each sends back.
func (h *harness) keyed(keys string) {
	h.t.Helper()
	for k := range strings.FieldsSeq(keys) {
		key, named := namedKeys[k]
		if !named {
			key = tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
		}
		h.deliver(h.update(key)...)
	}
}

// card is the routing card as it stands.
func (h *harness) card() dashboard.RoutingCard {
	return h.model.routingCard(h.clock.now)
}

// tokenless is an account without a usable token.
func tokenless(id, label string) status.Account {
	return status.Account{ID: id, Label: label, Error: "token missing: write it to tokens/" + id}
}

// pinnedTo is doc with new sessions pinned to the accounts given since the
// clock started.
func pinnedTo(doc status.Document, accounts ...string) status.Document {
	doc.Pin = status.Pin{Accounts: accounts, Since: start.UTC()}
	return doc
}

func TestTheRoutingCardOpensWithNothingPickedButWhatsSetNow(t *testing.T) {
	moving := pinnedTo(routerDocument(three()...), "side")
	moving.Pin.Move = true
	tests := []struct {
		name       string
		doc        status.Document
		wantPicked []string
		wantCursor int
	}{
		{name: "routing automatically", doc: routerDocument(three()...), wantCursor: 0},
		{name: "pinned to one", doc: pinnedTo(routerDocument(three()...), "side"), wantPicked: []string{"side"}, wantCursor: 3},
		{name: "pinned to two", doc: pinnedTo(routerDocument(three()...), "personal", "side"), wantPicked: []string{"personal", "side"}, wantCursor: 2},
		{name: "pinned to one, moving the sessions running", doc: moving, wantPicked: []string{"side"}, wantCursor: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.start()
			h.keyed("p")

			got := h.card()
			if !slices.Equal(got.Picked, tt.wantPicked) || got.Cursor != tt.wantCursor || got.Move || got.Refused != "" {
				t.Errorf("the card opens picking %q, the cursor on row %d, m ticked %v, refused %q; want %q, on row %d, m clear, nothing refused",
					got.Picked, got.Cursor, got.Move, got.Refused, tt.wantPicked, tt.wantCursor)
			}
		})
	}
}

func TestTheRoutingCardsKeysPickItsRows(t *testing.T) {
	doc := routerDocument(three()[0], tokenless("personal", "Personal"), three()[2])
	tests := []struct {
		name       string
		keys       string
		wantPicked []string
		wantCursor int
		wantMove   bool
	}{
		{name: "nothing", keys: "", wantCursor: 0},
		{name: "down to work, and space", keys: "down space", wantPicked: []string{"work"}, wantCursor: 1},
		{name: "a number, the cursor going to its row", keys: "3", wantPicked: []string{"side"}, wantCursor: 3},
		{name: "two, in the config's order whatever order they're picked in", keys: "3 1", wantPicked: []string{"work", "side"}, wantCursor: 1},
		{name: "one picked again, which clears it", keys: "1 3 1", wantPicked: []string{"side"}, wantCursor: 1},
		{name: "every one cleared, which is auto", keys: "1 1", wantCursor: 1},
		{name: "auto, which clears the accounts", keys: "1 3 0", wantCursor: 0},
		{name: "auto with space, which clears the accounts", keys: "1 3 up up up space", wantCursor: 0},
		{name: "an account that can't be picked, by its number", keys: "1 2", wantPicked: []string{"work"}, wantCursor: 1},
		{name: "an account that can't be picked, by space", keys: "1 down space", wantPicked: []string{"work"}, wantCursor: 2},
		{name: "a number past the accounts there are", keys: "1 4 9", wantPicked: []string{"work"}, wantCursor: 1},
		{name: "up from the first row", keys: "up up", wantCursor: 0},
		{name: "down past the last row", keys: "down down down down down", wantCursor: 3},
		{name: "m", keys: "m", wantMove: true},
		{name: "m again, which clears it", keys: "m m"},
		{name: "m with auto picked, as it's never dimmed", keys: "0 m", wantMove: true},
		{name: "keys that do nothing in it", keys: "a r s w g t R ? q tab", wantCursor: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, doc)
			h.start()
			h.keyed("p")
			h.keyed(tt.keys)

			got := h.card()
			if h.model.open != routingOpen || !slices.Equal(got.Picked, tt.wantPicked) || got.Cursor != tt.wantCursor || got.Move != tt.wantMove {
				t.Errorf("after %q, the card is open %v, picking %q, the cursor on row %d, m ticked %v; want it open, picking %q, on row %d, m ticked %v",
					tt.keys, h.model.open == routingOpen, got.Picked, got.Cursor, got.Move, tt.wantPicked, tt.wantCursor, tt.wantMove)
			}
			if len(h.source.orders) > 0 {
				t.Errorf("after %q, orders = %q, want none before enter", tt.keys, h.source.orders)
			}
		})
	}
}

func TestPastNineAccountsTheRestHaveNoNumber(t *testing.T) {
	accounts := make([]status.Account, 10)
	for i := range accounts {
		accounts[i] = account("a"+strconv.Itoa(i+1), "A"+strconv.Itoa(i+1), session(0.1, 3*time.Hour), week(0.2))
	}
	h := routedHarness(t, routerDocument(accounts...))
	h.start()
	h.keyed("p 9")
	if got := h.card().Picked; !slices.Equal(got, []string{"a9"}) {
		t.Fatalf("9 picks %q, want the ninth account", got)
	}

	h.keyed("down space")
	if got := h.card().Picked; !slices.Equal(got, []string{"a9", "a10"}) {
		t.Errorf("down from the ninth and space pick %q, want the ninth and the tenth", got)
	}
}

func TestEnterSendsThePicks(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
		keys string
		// want is the order sent, as fakeSource notes it: none where
		// nothing is sent.
		want []string
	}{
		{name: "two accounts", doc: routerDocument(three()...), keys: "2 3 enter", want: []string{"pin personal,side"}},
		{name: "two accounts, in the config's order", doc: routerDocument(three()...), keys: "3 1 enter", want: []string{"pin work,side"}},
		{name: "an account, moving the sessions running", doc: routerDocument(three()...), keys: "2 m enter", want: []string{"move personal"}},
		{name: "auto, from a pin", doc: pinnedTo(routerDocument(three()...), "side"), keys: "0 enter", want: []string{"unpin"}},
		{name: "the pin's account cleared, which is auto", doc: pinnedTo(routerDocument(three()...), "side"), keys: "3 enter", want: []string{"unpin"}},
		{name: "auto with m, which moves nothing", doc: pinnedTo(routerDocument(three()...), "side"), keys: "0 m enter", want: []string{"unpin"}},
		{name: "the pin as it stands, with m", doc: pinnedTo(routerDocument(three()...), "side"), keys: "m enter", want: []string{"move side"}},
		{name: "nothing changed", doc: routerDocument(three()...), keys: "enter"},
		{name: "auto as it opened, with m, which moves nothing", doc: routerDocument(three()...), keys: "m enter"},
		{name: "auto as it opened, an account picked and cleared, with m", doc: routerDocument(three()...), keys: "1 m 1 enter"},
		{name: "the pin as it stands", doc: pinnedTo(routerDocument(three()...), "personal", "side"), keys: "enter"},
		{name: "picks changed and changed back", doc: pinnedTo(routerDocument(three()...), "side"), keys: "1 1 m m enter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.start()
			looks := h.source.looks()
			h.keyed("p " + tt.keys)

			if !slices.Equal(h.source.orders, tt.want) {
				t.Errorf("orders = %q, want %q", h.source.orders, tt.want)
			}
			if h.model.open != nothingOpen {
				t.Errorf("after enter, the card is still open, want it closed")
			}
			wantLooks := 0
			if len(tt.want) > 0 {
				wantLooks = 1
			}
			if got := h.source.looks() - looks; got != wantLooks {
				t.Errorf("looked at the router's document %d times, want %d: again at once, once the router took an order", got, wantLooks)
			}
		})
	}
}

func TestOnceTheRouterTakesThePicksTheCardShowsWhatItDid(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.keyed("p 2 3 enter")
	if got := h.model.doc.Pin.Accounts; !slices.Equal(got, []string{"personal", "side"}) {
		t.Fatalf("the document on screen is pinned to %q, want personal and side, read again once the router took it", got)
	}

	h.keyed("p")
	if got := h.card().Picked; !slices.Equal(got, []string{"personal", "side"}) {
		t.Errorf("opened again, the card picks %q, want the pin it set", got)
	}
}

func TestARefusalKeepsTheCardOpenSayingWhyUntilThePicksChange(t *testing.T) {
	refusal := errors.New("account personal has no usable token, so nothing can go out on it: token missing")
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.source.refuse = refusal
	h.keyed("p 2 enter")

	if h.model.open != routingOpen || h.card().Refused != refusal.Error() {
		t.Fatalf("refused, the card is open %v, saying %q; want it open, saying %q", h.model.open == routingOpen, h.card().Refused, refusal)
	}
	if h.model.note != "" {
		t.Errorf("the key line notes %q, want nothing: the card says why", h.model.note)
	}
	h.keyed("down")
	if got := h.card().Refused; got != refusal.Error() {
		t.Errorf("once the cursor moved, the card says %q, want %q still", got, refusal)
	}
	h.keyed("3")
	if got := h.card().Refused; got != "" {
		t.Errorf("once the picks changed, the card says %q, want the refusal gone", got)
	}
	h.keyed("esc")
	if h.model.open != nothingOpen || len(h.source.orders) != 1 {
		t.Errorf("after esc, the card is open %v, and orders = %q; want it closed, sending nothing more", h.model.open == routingOpen, h.source.orders)
	}
}

func TestARefusalLandingOnceTheCardHasClosedSaysWhyInTheKeyLinesPlace(t *testing.T) {
	refusal := errors.New("account personal has no usable token, so nothing can go out on it: token missing")
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.source.refuse = refusal
	h.keyed("p 2")
	answer := h.update(namedKeys["enter"])
	h.keyed("esc")
	h.deliver(answer...)

	if h.model.open != nothingOpen {
		t.Errorf("the card opened again, want it closed")
	}
	if got := h.keyLine(); got != refusal.Error() {
		t.Errorf("the key line reads %q, want %q in its place", got, refusal)
	}
}

func TestALateOrdersAnswerLeavesTheCardOpenedSinceAlone(t *testing.T) {
	refusal := errors.New("account personal has no usable token, so nothing can go out on it: token missing")
	tests := []struct {
		name     string
		refuse   error
		wantNote string
	}{
		{name: "taken"},
		{name: "refused, saying why in the key line's place", refuse: refusal, wantNote: refusal.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			h.source.refuse = tt.refuse
			h.keyed("p 2")
			answer := h.update(namedKeys["enter"])
			h.keyed("esc p 3")
			looks := h.source.looks()
			h.deliver(answer...)

			if got := h.card(); h.model.open != routingOpen || !slices.Equal(got.Picked, []string{"side"}) || got.Refused != "" {
				t.Errorf("the card is open %v, picking %q, saying %q; want the card opened since open, picking side, saying nothing",
					h.model.open == routingOpen, got.Picked, got.Refused)
			}
			if h.model.note != tt.wantNote {
				t.Errorf("the key line notes %q, want %q", h.model.note, tt.wantNote)
			}
			if got := h.source.looks() - looks; got != 1 {
				t.Errorf("looked at the router's document %d times once the answer landed, want once", got)
			}
		})
	}
}

func TestEnterWhileTheRouterCarriesOutAnOrderSaysSoInTheCard(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.keyed("p 2")
	answer := h.update(namedKeys["enter"])
	h.keyed("esc p 3 enter")

	if got, want := h.card().Refused, errOrdering.Error(); h.model.open != routingOpen || got != want {
		t.Fatalf("the card is open %v, saying %q; want it open, saying %q", h.model.open == routingOpen, got, want)
	}
	h.deliver(answer...)
	if got := h.card().Refused; got != "" {
		t.Errorf("once the router carried out the last order, the card says %q, want nothing", got)
	}
	h.keyed("enter")
	if want := []string{"pin personal", "pin side"}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q: the second card's sent once the first's was carried out", h.source.orders, want)
	}
}

func TestProbingAsAskedThereIsNoP(t *testing.T) {
	h := newHarness(t, document(three()...))
	h.model.cfg.ProbeOnly = true
	h.start()

	if slices.Contains(h.listed(), "p") || slices.Contains(listedKeys(helpOver(h.model)), "p") {
		t.Errorf("the key line lists %q, and help %q; want neither to list p: there's no router to pin through", h.listed(), listedKeys(helpOver(h.model)))
	}
	h.keyed("p")
	if h.model.open != nothingOpen || h.model.note != "" {
		t.Errorf("p opened %v, noting %q; want nothing", h.model.open, h.model.note)
	}
}

// atItsLimit is a, a limit holding back its every request till an hour after
// the clock starts.
func atItsLimit(a status.Account) status.Account {
	a.Limit = status.Limit{Windows: []string{"5h"}, Until: start.Add(time.Hour).UTC()}
	return a
}

func TestAnAccountThatCantBePickedPinnedOpensPicked(t *testing.T) {
	doc := pinnedTo(routerDocument(atItsLimit(three()[0]), three()[1], three()[2]), "work", "side")
	tests := []struct {
		name       string
		keys       string
		wantPicked []string
		wantOrders []string
	}{
		{name: "its number clearing it", keys: "1 enter", wantOrders: []string{"pin side"}},
		{name: "space clearing it", keys: "space enter", wantOrders: []string{"pin side"}},
		{name: "cleared, its number picking it no more", keys: "1 1", wantPicked: []string{"side"}},
		{name: "cleared, space picking it no more", keys: "space space", wantPicked: []string{"side"}},
		{name: "sent with the rest", keys: "2 enter", wantOrders: []string{"pin work,personal,side"}},
		{name: "kept as another is cleared", keys: "3 enter", wantOrders: []string{"pin work"}},
		{name: "cleared by auto", keys: "0", wantPicked: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, doc)
			h.start()
			h.keyed("p")
			if got := h.card(); !slices.Equal(got.Picked, []string{"work", "side"}) || got.Cursor != 1 || got.Rows[1].Pickable {
				t.Fatalf("the card opens picking %q, on row %d, work pickable %v; want work and side, on work's, which can't be picked",
					got.Picked, got.Cursor, got.Rows[1].Pickable)
			}

			h.keyed(tt.keys)
			if len(tt.wantOrders) > 0 {
				if !slices.Equal(h.source.orders, tt.wantOrders) {
					t.Errorf("orders = %q, want %q", h.source.orders, tt.wantOrders)
				}
				return
			}
			if got := h.card().Picked; !slices.Equal(got, tt.wantPicked) {
				t.Errorf("after %q, the card picks %q, want %q", tt.keys, got, tt.wantPicked)
			}
		})
	}
}

func TestAPinNamingAnAccountWithoutATokenShowsItPickedAndIsSentWithoutItOnceChanged(t *testing.T) {
	doc := routerDocument(three()[0], tokenless("personal", "Personal"), three()[2])
	tests := []struct {
		name       string
		pin        []string
		keys       string
		wantPicked []string
		// wantOrders are the orders sent, as fakeSource notes them: none
		// where nothing is sent.
		wantOrders []string
	}{
		{name: "it opens picked with the rest", pin: []string{"personal", "side"}, wantPicked: []string{"personal", "side"}},
		{name: "alone, it opens picked", pin: []string{"personal"}, wantPicked: []string{"personal"}},
		{name: "its number clearing it, never to pick it again", pin: []string{"personal", "side"}, keys: "2 2", wantPicked: []string{"side"}},
		{name: "nothing changed, which sends nothing", pin: []string{"personal", "side"}, keys: "enter"},
		{name: "alone, nothing changed, which sends nothing", pin: []string{"personal"}, keys: "enter"},
		{name: "another picked, sent without it", pin: []string{"personal", "side"}, keys: "1 enter", wantOrders: []string{"pin work,side"}},
		{name: "m ticked, sent without it", pin: []string{"personal", "side"}, keys: "m enter", wantOrders: []string{"move side"}},
		{name: "its number clearing it", pin: []string{"personal", "side"}, keys: "2 enter", wantOrders: []string{"pin side"}},
		{name: "alone, m ticked, which sends auto", pin: []string{"personal"}, keys: "m enter", wantOrders: []string{"unpin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, pinnedTo(doc, tt.pin...))
			h.start()
			h.keyed("p " + tt.keys)
			if !slices.Equal(h.source.orders, tt.wantOrders) {
				t.Errorf("after %q, orders = %q, want %q", tt.keys, h.source.orders, tt.wantOrders)
			}
			if strings.HasSuffix(tt.keys, "enter") {
				if h.model.open != nothingOpen {
					t.Errorf("after %q, the card is still open, want it closed", tt.keys)
				}
				return
			}
			if got := h.card().Picked; !slices.Equal(got, tt.wantPicked) {
				t.Errorf("after %q, the card picks %q, want %q", tt.keys, got, tt.wantPicked)
			}
		})
	}
}

func TestTheCardsOutcomeAndCostAreOfTheOrderEnterSends(t *testing.T) {
	h, _ := pricedHarness(t)
	doc := four()
	doc.Accounts[0] = tokenless("work", "Work")
	h.source.router = new(pinnedTo(doc, "work", "side"))
	h.tickUntilAsked()
	h.keyed("p m")

	got := h.card()
	if !slices.Equal(got.Picked, []string{"work", "side"}) || !slices.Equal(got.Unsent, []string{"work"}) {
		t.Errorf("the card picks %q, leaving %q unsent; want work and side picked, work unsent", got.Picked, got.Unsent)
	}
	if got.Moving != 3 || got.Cost == nil || got.Cost.Cents() != "$0.81" {
		t.Errorf("the card moves %d sessions at %v, want work's two and client's one at $0.81, as the order pins side alone", got.Moving, got.Cost)
	}
	if view := h.view(); !strings.Contains(view, "New sessions go to Side. When it's full, the router chooses.") {
		t.Errorf("the card says\n%s\nwant new sessions sent to side alone", view)
	}
}

func TestACardOpenedBeforeAnOrderLandsFollowsThePinItSets(t *testing.T) {
	tests := []struct {
		name string
		keys string
		want []string
	}{
		{name: "nothing changed", keys: "enter", want: []string{"pin personal"}},
		{name: "auto picked", keys: "0 enter", want: []string{"pin personal", "unpin"}},
		{name: "another account picked", keys: "3 enter", want: []string{"pin personal", "pin personal,side"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			h.keyed("p 2")
			answer := h.update(namedKeys["enter"])
			h.keyed("esc p")
			h.deliver(answer...)
			if got := h.card().Picked; h.model.open != routingOpen || !slices.Equal(got, []string{"personal"}) {
				t.Fatalf("once the order landed, the card is open %v, picking %q; want it open, picking personal, as the pin now stands",
					h.model.open == routingOpen, got)
			}

			h.keyed(tt.keys)
			if !slices.Equal(h.source.orders, tt.want) {
				t.Errorf("orders = %q, want %q", h.source.orders, tt.want)
			}
		})
	}
}

func TestTheCardFollowsAPinSetFromTheCommandLineTillAKeyPicks(t *testing.T) {
	tests := []struct {
		name       string
		keys       string
		wantPicked []string
		// wantOrders are the orders enter sends then, against the pin as it
		// stands.
		wantOrders []string
	}{
		{name: "untouched", wantPicked: []string{"side"}},
		{name: "the cursor moved and m ticked, which pick nothing", keys: "down m", wantPicked: []string{"side"}, wantOrders: []string{"move side"}},
		{name: "an account picked", keys: "2", wantPicked: []string{"personal"}, wantOrders: []string{"pin personal"}},
		{name: "the pin's account picked", keys: "3", wantPicked: []string{"side"}},
		{name: "an account picked and cleared again", keys: "2 2", wantOrders: []string{"unpin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			h.keyed("p " + tt.keys)
			h.source.router.Pin = status.Pin{Accounts: []string{"side"}, Since: start.UTC()}
			h.tickUntilAsked()

			if got := h.card().Picked; !slices.Equal(got, tt.wantPicked) {
				t.Errorf("once a look found side pinned, the card picks %q, want %q", got, tt.wantPicked)
			}
			h.keyed("enter")
			if !slices.Equal(h.source.orders, tt.wantOrders) {
				t.Errorf("orders = %q, want %q", h.source.orders, tt.wantOrders)
			}
		})
	}
}

func TestPDoesNothingWithOneAccountUnlessAPinStands(t *testing.T) {
	one := routerDocument(three()[0])
	tests := []struct {
		name string
		doc  status.Document
		want bool
	}{
		{name: "one account", doc: one},
		{name: "one account, pinned", doc: pinnedTo(one, "work"), want: true},
		{name: "two accounts", doc: routerDocument(three()[:2]...), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.start()
			if listed := slices.Contains(h.listed(), "p"); listed != tt.want {
				t.Errorf("the key line lists p: %v, want %v", listed, tt.want)
			}
			h.keyed("p")
			if opened := h.model.open == routingOpen; opened != tt.want {
				t.Errorf("p opened the routing card: %v, want %v", opened, tt.want)
			}
			if h.model.note != "" {
				t.Errorf("p noted %q, want nothing", h.model.note)
			}
		})
	}
}

func TestOneAccountsPinIsClearedFromTheCard(t *testing.T) {
	h := routedHarness(t, pinnedTo(routerDocument(three()[0]), "work"))
	h.start()
	h.keyed("p")
	if rows := h.card().Rows; len(rows) != 2 || rows[1].Account != "work" {
		t.Fatalf("the card's rows are %+v, want auto's and work's", rows)
	}
	h.keyed("0 enter")
	if want := []string{"unpin"}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q", h.source.orders, want)
	}
}

func TestPIsFaintAndSaysWhyWhileTheRouterDoesntAnswer(t *testing.T) {
	tests := []struct {
		name  string
		h     func(t *testing.T) *harness
		after func(h *harness)
	}{
		{
			name: "showing the router's last document",
			h:    func(t *testing.T) *harness { return routedHarness(t, routerDocument(three()...)) },
			after: func(h *harness) {
				h.stopRouter()
				h.tickUntilAsked()
			},
		},
		{
			name:  "probing, as the router isn't running",
			h:     func(t *testing.T) *harness { return newHarness(t, probedWithoutTheRouter()) },
			after: func(*harness) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.h(t)
			h.start()
			tt.after(h)

			if pin, ok := h.model.pinKey(); !ok || !pin.Off {
				t.Errorf("p is %+v, listed %v; want it listed, faint", pin, ok)
			}
			h.keyed("p")
			if h.model.open != nothingOpen {
				t.Fatal("p opened the routing card, want nothing open")
			}
			if got, want := h.keyLine(), status.ErrNoRouter.Error(); got != want {
				t.Errorf("the key line reads %q, want %q in its place", got, want)
			}
			if lapse, ok := h.pending(noteMsg{}); !ok || lapse.delay != noteFor {
				t.Errorf("the note lapses after %v (armed %v), want %v", lapse, ok, noteFor)
			}
		})
	}
}

func TestPIsntFaintWhileTheRouterAnswers(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	if pin, ok := h.model.pinKey(); !ok || pin.Off {
		t.Errorf("p is %+v, listed %v; want it listed, and not faint", pin, ok)
	}
}

func TestEnterSaysSoInTheCardWhileTheRouterDoesntAnswer(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.keyed("p 2")
	h.stopRouter()
	h.tickUntilAsked()

	h.keyed("enter")
	if len(h.source.orders) > 0 {
		t.Errorf("orders = %q, want none", h.source.orders)
	}
	if h.model.open != routingOpen || h.card().Refused != status.ErrNoRouter.Error() {
		t.Errorf("the card is open %v, saying %q; want it open, saying %q", h.model.open == routingOpen, h.card().Refused, status.ErrNoRouter)
	}
}

// sessionsLedger is a request ledger of the lines given, by session, which
// counts the times its sessions' lines are read.
type sessionsLedger struct {
	ledger.Empty
	lines map[string][]ledger.Line
	reads int
}

func (l *sessionsLedger) Sessions([]string, func(string, ledger.Line)) map[string][]ledger.Line {
	l.reads++
	return l.lines
}

// The usages of the priced sessions' answers. hourUsage writes the cache for
// an hour: its prompt, 101,010 tokens, costs $0.81 to write again for an hour
// at Claude Opus 5.5's prices. fiveMinuteUsage writes as much for five
// minutes.
const (
	hourUsage = `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":500}`
	fiveMinuteUsage = `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":1000,"ephemeral_1h_input_tokens":0},"output_tokens":500}`
)

// pricedHarness is gridHarness, its moves priced at the price table's prices
// from the ledger it returns: idD28C's last request on work a minute before
// the clock starts, writing the cache for an hour; 7f3a's nine minutes
// before, writing it for five, which have run out; and 9e21, on client,
// without a line.
func pricedHarness(t *testing.T) (*harness, *sessionsLedger) {
	t.Helper()
	asked := func(id, model, usage string, ago time.Duration) []ledger.Line {
		return []ledger.Line{{
			At: start.Add(-ago).UTC(), Kind: ledger.KindMessage, Session: id, Model: model, Account: "work", Reason: "new", Status: 200, Attempts: 1,
			Usage: json.RawMessage(usage),
		}}
	}
	l := &sessionsLedger{Empty: ledger.NewEmpty(func() time.Time { return start }, ledger.Caps{Shared: policy.Shared}), lines: map[string][]ledger.Line{
		idD28C: asked(idD28C, "claude-opus-5-5", hourUsage, time.Minute),
		id7F3A: asked(id7F3A, "claude-haiku-4-5", fiveMinuteUsage, 9*time.Minute),
	}}
	h := gridHarness(t)
	h.model.cfg.Ledger, h.model.cfg.Prices = l, ledger.Pricing
	return h, l
}

func TestTheCardPricesTheMovesOnceMIsTicked(t *testing.T) {
	h, l := pricedHarness(t)
	h.keyed("p 2")
	if got := h.card(); got.Moving != 0 || got.Cost != nil || l.reads > 0 {
		t.Fatalf("m clear, the card moves %d sessions at %v, the ledger read %d times; want none, unread", got.Moving, got.Cost, l.reads)
	}

	h.keyed("m")
	got := h.card()
	if got.Moving != 3 || got.Cost == nil || got.Cost.Cents() != "$0.81" {
		t.Fatalf("m ticked, the card moves %d sessions at %v, want work's two and client's one at $0.81: d28c's alone, 7f3a's cache run out, 9e21 unpriced",
			got.Moving, got.Cost)
	}
	if l.reads != 1 {
		t.Errorf("the ledger was read %d times, want once", l.reads)
	}

	h.keyed("down 3")
	if got := h.card(); got.Moving != 3 || l.reads != 1 {
		t.Errorf("side picked too, the card moves %d sessions, the ledger read %d times; want 3, still once for the document on screen", got.Moving, l.reads)
	}
	h.keyed("1")
	if got := h.card(); got.Moving != 1 || got.Cost != nil {
		t.Errorf("work picked too, the card moves %d sessions at %v, want client's one, unpriced", got.Moving, got.Cost)
	}

	h.tickUntilAsked()
	if l.reads != 1 {
		t.Errorf("once a look found the sessions as they were, the ledger was read %d times, want once still", l.reads)
	}
	h.source.sessions[1] = sessionOn(id9E21, "claude-opus-5-5", "client", 0)
	h.tickUntilAsked()
	if l.reads != 2 {
		t.Errorf("once a look found 9e21 on another model, the ledger was read %d times, want twice", l.reads)
	}
}

func TestTheCardPricesTheMovesAgainOnlyAsTheSessionsPlacesChange(t *testing.T) {
	tests := []struct {
		name string
		// look changes the sessions the router lists, a look on.
		look      func(sessions []status.Session) []status.Session
		wantReads int
	}{
		{name: "as they were", look: func(s []status.Session) []status.Session { return s }, wantReads: 1},
		{
			name: "each seen again",
			look: func(s []status.Session) []status.Session {
				for i := range s {
					s[i].Assignments[0].LastSeen = start.Add(time.Minute).UTC()
				}
				return s
			},
			wantReads: 1,
		},
		{
			name:      "one busy",
			look:      func(s []status.Session) []status.Session { s[0].Assignments[0].InFlight = status.Answering; return s },
			wantReads: 1,
		},
		{name: "in another order", look: func(s []status.Session) []status.Session { slices.Reverse(s); return s }, wantReads: 1},
		{
			name: "one with its models listed in another order",
			look: func(s []status.Session) []status.Session {
				s[0].Assignments = []status.Assignment{s[0].Assignments[1], s[0].Assignments[0]}
				return s
			},
			wantReads: 1,
		},
		{
			name: "one new",
			look: func(s []status.Session) []status.Session {
				return append(s, sessionOn("5b0c2e91-7d3a-4c18-9f42-0a6e8d1b3c57", "claude-sonnet-5-5", "client", 0))
			},
			wantReads: 2,
		},
		{name: "one gone", look: func(s []status.Session) []status.Session { return s[:2] }, wantReads: 2},
		{
			name:      "one on another account",
			look:      func(s []status.Session) []status.Session { s[1].Assignments[0].Account = "side"; return s },
			wantReads: 2,
		},
		{
			name:      "one with a model on another account",
			look:      func(s []status.Session) []status.Session { s[0].Assignments[1].Account = "client"; return s },
			wantReads: 2,
		},
		{name: "one pinned", look: func(s []status.Session) []status.Session { s[0].Pin = "work"; return s }, wantReads: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := pricedHarness(t)
			// d28c also asks Claude Haiku 4.5, on work.
			d28c := &h.source.sessions[0]
			d28c.Assignments = append(d28c.Assignments, sessionOn(idD28C, "claude-haiku-4-5", "work", time.Minute).Assignments...)
			h.tickUntilAsked()
			h.keyed("p 2 m")
			if l.reads != 1 {
				t.Fatalf("m ticked, the ledger was read %d times, want once", l.reads)
			}

			h.source.sessions = tt.look(apart(h.source.sessions))
			h.tickUntilAsked()
			if l.reads != tt.wantReads {
				t.Errorf("a look on, the ledger was read %d times, want %d", l.reads, tt.wantReads)
			}
		})
	}
}

// apart is a copy of the sessions that shares no assignments with them, for
// a look to change without changing those listed before.
func apart(sessions []status.Session) []status.Session {
	copied := slices.Clone(sessions)
	for i := range copied {
		copied[i].Assignments = slices.Clone(copied[i].Assignments)
	}
	return copied
}

func TestTheCardListsTheSessionsToPriceOnceAtATime(t *testing.T) {
	tests := []struct {
		name string
		// meanwhile is what happens while the listing runs.
		meanwhile func(h *harness)
		// wantReads is how many times the ledger has been read once the
		// listing lands, and anything it calls for after.
		wantReads int
	}{
		{
			name: "a look finding the sessions changed, which are listed again once it lands",
			meanwhile: func(h *harness) {
				h.source.sessions[1] = sessionOn(id9E21, "claude-opus-5-5", "client", 0)
				h.tickUntilAsked()
			},
			wantReads: 2,
		},
		{name: "the card opened again, which takes the listing", meanwhile: func(h *harness) { h.keyed("esc p 2 m") }, wantReads: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := pricedHarness(t)
			h.keyed("p 2")
			listing := h.update(tea.KeyPressMsg{Code: 'm', Text: "m"})
			tt.meanwhile(h)
			if l.reads != 1 {
				t.Fatalf("while a listing ran, the ledger was read %d times, want once: none starts while one runs", l.reads)
			}

			var after []tea.Msg
			for _, msg := range listing {
				after = append(after, h.update(msg)...)
			}
			if got := h.card(); got.Cost == nil || got.Cost.Cents() != "$0.81" {
				t.Errorf("once the listing landed, the card prices its moves at %v, want $0.81, as it listed them", got.Cost)
			}
			h.deliver(after...)
			if l.reads != tt.wantReads {
				t.Errorf("once it landed, the ledger was read %d times, want %d", l.reads, tt.wantReads)
			}
		})
	}
}

func TestTheCardDoesntPriceMovesThatDontHappen(t *testing.T) {
	tests := []struct {
		name string
		keys string
	}{
		{name: "m clear", keys: "p 2"},
		{name: "auto with m", keys: "p m"},
		{name: "every account with a session picked", keys: "p 1 4 m"},
		{name: "the card closed", keys: "p 2 m esc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := pricedHarness(t)
			h.keyed(tt.keys)
			reads := l.reads
			h.tickUntilAsked()
			if l.reads > reads {
				t.Errorf("a look read the ledger, want it unread")
			}
			if h.model.open == routingOpen {
				if got := h.card(); got.Moving != 0 || got.Cost != nil {
					t.Errorf("the card moves %d sessions at %v, want none", got.Moving, got.Cost)
				}
			}
		})
	}
}

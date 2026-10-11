package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

// commanded has the model give the router o, as the routing card does, and
// returns what that sends back, undelivered.
func (h *harness) commanded(o order) []tea.Msg {
	h.t.Helper()
	next, cmd := h.model.command(o)
	h.model = next
	return h.run(cmd)
}

func TestAnOrderIsGivenAndTheRoutersDocumentLookedAtAgain(t *testing.T) {
	tests := []struct {
		name       string
		order      order
		wantOrders []string
		wantPin    []string
		wantLog    []string
	}{
		{
			name:       "a pin",
			order:      order{accounts: []string{"work", "side"}},
			wantOrders: []string{"pin work,side"},
			wantPin:    []string{"work", "side"},
			wantLog:    []string{"level=INFO", "msg=pinned", "accounts=work,side", "move=false"},
		},
		{
			name:       "a pin that moves running sessions",
			order:      order{accounts: []string{"side"}, move: true},
			wantOrders: []string{"move side"},
			wantPin:    []string{"side"},
			wantLog:    []string{"level=INFO", "msg=pinned", "accounts=side", "move=true"},
		},
		{
			name:       "routing automatically again",
			order:      order{},
			wantOrders: []string{"unpin"},
			wantLog:    []string{"level=INFO", "msg=unpinned"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			looks := h.source.looks()

			h.deliver(h.commanded(tt.order)...)
			if !slices.Equal(h.source.orders, tt.wantOrders) {
				t.Errorf("orders = %q, want %q", h.source.orders, tt.wantOrders)
			}
			if got := h.source.looks(); got != looks+1 {
				t.Errorf("looked at the router's document %d times after the order, want once", got-looks)
			}
			if got := h.model.doc.Pin.Accounts; !slices.Equal(got, tt.wantPin) {
				t.Errorf("the document on screen is pinned to %q, want %q", got, tt.wantPin)
			}
			if h.model.note != "" {
				t.Errorf("the key line notes %q, want its keys: an order taken says nothing", h.model.note)
			}
			if !log.Has(tt.wantLog...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
		})
	}
}

func TestAnOrderOfOneSessionPinsItOrTakesItsPinOff(t *testing.T) {
	log := logstest.Capture(t)
	h := gridHarness(t)
	looks := h.source.looks()

	h.deliver(h.commanded(order{session: idD28C, to: "side"})...)
	if want := []string{"pin " + idD28C + " to side"}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q", h.source.orders, want)
	}
	if got := h.source.looks(); got != looks+1 {
		t.Errorf("looked at the router's document %d times after the order, want once", got-looks)
	}
	if !h.source.router.Pin.IsZero() {
		t.Errorf("the router's pin is %+v, want none: the session's pin is its own", h.source.router.Pin)
	}
	if want := []string{"level=INFO", `msg="pinned a session"`, "session=d28c5e17", "account=side"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}

	h.deliver(h.commanded(order{session: idD28C})...)
	if want := []string{"pin " + idD28C + " to side", "unpin " + idD28C}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q", h.source.orders, want)
	}
	if want := []string{"level=INFO", `msg="unpinned a session"`, "session=d28c5e17"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	if strings.Contains(log.String(), idD28C) {
		t.Errorf("log reads\n%s\nwant the session's id cut short, as the router logs it", log)
	}
}

func TestARefusalOnceTheRoutingCardHasClosedSaysWhyInTheKeyLinesPlace(t *testing.T) {
	refusal := errors.New("account personal has no usable token, so nothing can go out on it: token missing")
	tests := []struct {
		name    string
		order   order
		wantLog []string
	}{
		{
			name:    "a pin",
			order:   order{accounts: []string{"personal"}},
			wantLog: []string{"level=WARN", `msg="the router didn't take an order"`, "accounts=personal", "move=false", "error="},
		},
		{
			name:    "a session's pin",
			order:   order{session: idD28C, to: "personal"},
			wantLog: []string{"level=WARN", `msg="the router didn't take an order"`, "session=d28c5e17", "account=personal", "error="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			h := gridHarness(t)
			h.source.refuse = refusal

			h.deliver(h.commanded(tt.order)...)
			if got, want := h.keyLine(), refusal.Error(); got != want {
				t.Errorf("the key line reads %q, want %q", got, want)
			}
			if !log.Has(tt.wantLog...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
		})
	}
}

func TestANoteLapses(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.refuse = errors.New("account personal has no usable token")
	h.start()
	h.deliver(h.commanded(order{accounts: []string{"personal"}})...)

	lapse, ok := h.pending(noteMsg{})
	if !ok {
		t.Fatal("no timer armed for the note to lapse")
	}
	if lapse.delay != noteFor {
		t.Errorf("the note lapses after %v, want %v", lapse.delay, noteFor)
	}
	h.clock.now = lapse.due.Add(-time.Millisecond)
	if got, want := h.keyLine(), "account personal has no usable token"; got != want {
		t.Errorf("just before it lapses, the key line reads %q, want %q", got, want)
	}
	h.fire(lapse)
	if got, want := h.keyLine(), threeKeys; got != want {
		t.Errorf("once it lapses, the key line reads %q, want %q", got, want)
	}
}

func TestAnOrderWhileTheRouterCarriesOutAnotherIsDropped(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	pending := h.commanded(order{accounts: []string{"work"}})
	h.deliver(h.commanded(order{accounts: []string{"personal"}})...)
	h.deliver(pending...)
	if want := []string{"pin work"}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q", h.source.orders, want)
	}
}

func TestAnOrderSaysSoWhileTheRouterDoesntAnswer(t *testing.T) {
	tests := []struct {
		name string
		h    func(t *testing.T) *harness
	}{
		{
			name: "showing the router's last document",
			h: func(t *testing.T) *harness {
				h := routedHarness(t, routerDocument(three()...))
				h.start()
				h.stopRouter()
				h.tickUntilAsked() // 13:12:05.
				return h
			},
		},
		{
			name: "probing, as the router isn't running",
			h: func(t *testing.T) *harness {
				h := newHarness(t, probedWithoutTheRouter())
				h.start()
				return h
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.h(t)
			h.deliver(h.commanded(order{accounts: []string{"work"}})...)
			if len(h.source.orders) > 0 {
				t.Errorf("orders = %q, want none", h.source.orders)
			}
			if got, want := h.keyLine(), status.ErrNoRouter.Error(); got != want {
				t.Errorf("the key line reads %q, want %q", got, want)
			}
		})
	}
}

func TestALookAskedForWhileAReadIsUnderWayComesOnceItLands(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	reading := h.press("R")
	h.deliver(h.commanded(order{accounts: []string{"work"}})...)
	if got, want := h.source.asked, []status.Read{{Refresh: interval, Probe: true}, {Refresh: status.FreshFor, Probe: true}}; !slices.Equal(got, want) {
		t.Fatalf("while R's read is under way, asked for %+v, want %+v", got, want)
	}
	h.deliver(reading...)
	want := []status.Read{{Refresh: interval, Probe: true}, {Refresh: status.FreshFor, Probe: true}, {}}
	if !slices.Equal(h.source.asked, want) {
		t.Errorf("once it lands, asked for %+v, want %+v", h.source.asked, want)
	}
	if got := h.model.doc.Pin.Accounts; !slices.Equal(got, []string{"work"}) {
		t.Errorf("the document on screen is pinned to %q, want work", got)
	}
}

func TestALookAskedForAsTheRouterGoesProbesNoMore(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.stopRouter()
	reading := h.press("R")
	h.deliver(h.commanded(order{accounts: []string{"work"}})...)
	h.deliver(reading...)
	if h.model.routed() || h.source.reads != 1 {
		t.Errorf("routed = %v, and probed %d times, want probing, once: the look the order asked for is moot", h.model.routed(), h.source.reads)
	}
}

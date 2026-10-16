package watch

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// pricedFromNew is newHarness, its model made with the price table's
// prices, as the dashboard's is.
func pricedFromNew(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{now: start}, source: &fakeSource{doc: probedWithoutTheRouter()}, notifier: &fakeNotifier{}}
	cfg := h.config(Size{Width: 80, Height: 24})
	cfg.Prices = ledger.Pricing
	h.model = New(t.Context(), cfg)
	h.update(tea.WindowSizeMsg{Width: 150, Height: 50})
	return h
}

// listedToday is the List of the store as the watch holds it at the clock's
// time.
func listedToday(h *harness) views.SessionsToday {
	now := h.clock.Now()
	return h.model.held.List(h.model.listLive(now, h.model.traffic.stream(now))).Today
}

// A store first fed by the Log, opened over an empty ledger, prices the
// session of the line its next reads of the tail find, as the store is
// priced from the start.
func TestAStoreTheLogFeedsFirstPricesItsSessions(t *testing.T) {
	state := t.TempDir()
	h := pricedFromNew(t)
	onTheLog(h, state)
	h.start()
	l := arrived("0a000001", "work", -time.Minute)
	l.Usage = json.RawMessage(hourUsage)
	writeLines(t, state, l)
	h.read(probedWithoutTheRouter())
	h.read(probedWithoutTheRouter())
	if today := listedToday(h); today.Requests != 1 || today.Amount == nil {
		t.Errorf("the store holds %d requests today, worth %v, want its one request priced", today.Requests, today.Amount)
	}
}

// The Log asked to open while the List's first read is under way opens as
// it lands, the tail unread: it reads the tail for every reader, so the
// store is fed the line filed after the List read its session, and the List
// counts what the ledger holds.
func TestTheLogOpeningAfterTheListsFirstReadFeedsTheStoreWhatCameBetween(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "work", -4*time.Minute))
	h := pricedFromNew(t)
	onTheLog(h, state)
	h.model.view = dashboard.Sessions
	listing := h.deliverHolding(func(msg tea.Msg) bool { _, ok := msg.(listReadMsg); return ok }, h.init()...)
	if len(listing) != 1 || !h.model.tail.unread() {
		t.Fatalf("%d of the List's reads held, the tail unread %t, want the List's first read under way, the tail unread", len(listing), h.model.tail.unread())
	}
	writeLines(t, state, arrived("0a000003", "work", -3*time.Minute))
	h.keys(tabKey, tabKey)
	if h.model.view != dashboard.Log || h.model.log.opened {
		t.Fatalf("the Log shows %t, opened %t, want it shown, its open waiting on the List's read", h.model.view == dashboard.Log, h.model.log.opened)
	}
	h.deliver(listing...)
	if !h.model.log.opened {
		t.Fatal("the List's read landed, the Log isn't open, want it opened as it landed")
	}
	h.read(probedWithoutTheRouter())
	if got := listedToday(h).Requests; got != 3 {
		t.Errorf("the List of the store counts %d requests today, want the 3 the ledger holds", got)
	}
}

// The Log opening after the tail was read reads the tail afresh, so the
// next read of it gives none of the lines it took again: each line without
// an id is shown once.
func TestTheLogOpeningAfterTheTailWasReadIsGivenNoLineTwice(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	h := pricedFromNew(t)
	onTheLog(h, state)
	h.model.view = dashboard.Sessions
	h.start()
	h.read(probedWithoutTheRouter())
	if h.model.tail.unread() {
		t.Fatal("the List shown, the tail is unread, want it read")
	}
	idless := arrived("", "work", -4*time.Minute)
	writeLines(t, state, idless, arrived("", "side", -3*time.Minute))
	h.keys(tabKey, tabKey)
	h.read(probedWithoutTheRouter())
	h.read(probedWithoutTheRouter())
	shown := 0
	for _, id := range shownRequests(h) {
		if id == "" {
			shown++
		}
	}
	if shown != 2 {
		t.Errorf("the Log shows %d rows without an id, want the 2 the ledger holds", shown)
	}
}

// The Log, opened then left for another tab, lets go of the day before once
// today's rows fill the view it last filled, as a read of the tail grows it
// across midnight: its read holds today's lines alone.
func TestTheLogLetsGoOfTheDayBeforeWhileAnotherTabShows(t *testing.T) {
	state := t.TempDir()
	midnight := onTheDay(0, 0, 0).AddDate(0, 0, 1)
	var yesterday []ledger.Held
	for i := range 45 {
		yesterday = append(yesterday, arrivedAt(fmt.Sprintf("0b%06d", i), midnight.Add(-time.Hour+time.Duration(i)*time.Minute)))
	}
	writeLines(t, state, yesterday...)
	h := pricedFromNew(t)
	onTheLog(h, state)
	h.clock.now = midnight.Add(-10 * time.Minute)
	h.start()
	h.keys(tabKey)
	if h.model.view == dashboard.Log || !h.model.log.opened {
		t.Fatalf("the Log shows %t, opened %t, want it opened, another tab shown", h.model.view == dashboard.Log, h.model.log.opened)
	}
	var today []ledger.Held
	for i := range 45 {
		today = append(today, arrivedAt(fmt.Sprintf("0c%06d", i), midnight.Add(time.Duration(i)*time.Second)))
	}
	h.clock.now = midnight.Add(time.Minute)
	writeLines(t, state, today...)
	for tick := h.lastTick(); tick.due.Before(midnight.Add(3 * time.Minute)); tick = h.lastTick() {
		h.fire(tick)
	}
	if got, want := h.model.log.from.Date, views.Today(midnight).Date; got != want || held(h.model.log.read, "days") != 1 {
		t.Errorf("the Log spans from %s, its read holding %d days, want it from %s, holding today's alone", got, held(h.model.log.read, "days"), want)
	}
}

// A reader's read joining the store, as heldWith takes it in, makes the
// Log's Events again at the next build, as their rows are told by the
// store.
func TestTheLogsEventsAreMadeAgainAsAReadersReadJoinsTheStore(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	doc := routerDocument(three()...)
	doc.Events = []status.Event{{ID: 1, At: past(-time.Minute), Kind: status.EventPin, Accounts: []string{"personal"}}}
	h := routedHarness(t, doc)
	onTheLog(h, state)
	h.start()
	made := h.model.log.built.events
	h.model = h.model.heldWith(views.HoldSessions(ledger.Pricing))
	h.deliver(frameMsg{gen: h.model.frames})
	if h.model.log.built.events == made {
		t.Error("a reader's read joined the store, the Log's Events weren't made again, want them made of the store as it stands")
	}
}

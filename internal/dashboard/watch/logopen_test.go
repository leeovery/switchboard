package watch

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

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
	h := newHarness(t, probedWithoutTheRouter())
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
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.model.view = dashboard.Sessions
	listing := h.deliverHolding(func(msg tea.Msg) bool { _, ok := msg.(listReadMsg); return ok }, h.init()...)
	if len(listing) != 1 || !h.model.tail.unread() {
		t.Fatalf("%d of the List's reads held, the tail unread %t, want the List's first read under way, the tail unread", len(listing), h.model.tail.unread())
	}
	writeLines(t, state, arrived("0a000003", "work", -3*time.Minute))
	h.keys(tabKey, tabKey, tabKey)
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
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.model.view = dashboard.Sessions
	h.start()
	h.read(probedWithoutTheRouter())
	if h.model.tail.unread() {
		t.Fatal("the List shown, the tail is unread, want it read")
	}
	idless := arrived("", "work", -4*time.Minute)
	writeLines(t, state, idless, arrived("", "side", -3*time.Minute))
	h.keys(tabKey, tabKey, tabKey)
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
	h := newHarness(t, probedWithoutTheRouter())
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

// daysHeld are the dates before the one given of every day of the Log's
// lines v reaches, by any path: of its read's days, and of the days its
// tables were tabled and drawn of, as views holds each, by its LedgerDay.
func daysHeld(v any, before string) []string {
	var held []string
	walked(v, func(v reflect.Value) bool {
		if t := v.Type(); v.Kind() == reflect.Struct && t.PkgPath() == viewsPkg && slices.Contains(daysOfLines, t.Name()) {
			if date := v.FieldByName("day").FieldByName("Date").String(); date < before {
				held = append(held, t.Name()+" "+date)
			}
		}
		return true
	})
	return held
}

// walked hands visit every value v reaches, by any path, each pointer and
// map once, going on into those it reaches where visit says to.
func walked(v any, visit func(reflect.Value) bool) {
	seen := make(map[uintptr]bool)
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		if k := v.Kind(); (k == reflect.Pointer || k == reflect.Map) && !v.IsNil() {
			if seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
		}
		if !visit(v) {
			return
		}
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Slice, reflect.Array:
			for i := range v.Len() {
				walk(v.Index(i))
			}
		case reflect.Map:
			for it := v.MapRange(); it.Next(); {
				walk(it.Key())
				walk(it.Value())
			}
		case reflect.Struct:
			for _, field := range v.Fields() {
				walk(field)
			}
		}
	}
	walk(reflect.ValueOf(v))
}

// viewsPkg is the views package's path, and daysOfLines the names of its
// types each holding a day's lines of the Log's: the read's, the lines
// tabled, and the table's sections.
var (
	viewsPkg    = reflect.TypeFor[views.LogRead]().PkgPath()
	daysOfLines = []string{"loggedDay", "tableDay", "tableSection"}
)

// The Log shown again before midnight, after its read changed while another
// tab showed, then letting go of the day before as today's rows fill its
// view, holds nothing of that day anywhere: no read kept while it was
// hidden reaches it.
func TestTheLogShownAgainHoldsNothingOfADayLetGo(t *testing.T) {
	state := t.TempDir()
	midnight := onTheDay(0, 0, 0).AddDate(0, 0, 1)
	var yesterday []ledger.Held
	for i := range 45 {
		yesterday = append(yesterday, arrivedAt(fmt.Sprintf("0b%06d", i), midnight.Add(-time.Hour+time.Duration(i)*time.Minute)))
	}
	writeLines(t, state, yesterday...)
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.clock.now = midnight.Add(-10 * time.Minute)
	h.start()
	h.keys(tabKey)
	writeLines(t, state, arrivedAt("0b100000", midnight.Add(-9*time.Minute)))
	for tick := h.lastTick(); tick.due.Before(midnight.Add(-2 * time.Minute)); tick = h.lastTick() {
		h.fire(tick)
	}
	h.keys(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if h.model.view != dashboard.Log {
		t.Fatal("shift-tab didn't show the Log again")
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
	from := h.model.log.from.Date
	if want := views.Today(midnight).Date; from != want {
		t.Fatalf("the Log spans from %s, want %s, the day before let go", from, want)
	}
	if held := daysHeld(h.model.log, from); len(held) > 0 {
		t.Errorf("the Log, spanning from %s, still reaches %v", from, held)
	}
}

// eventsOnTheLog is a harness showing the Log over a ledger of one line, the
// router telling of an event, started, and the Events built of it.
func eventsOnTheLog(t *testing.T) *harness {
	t.Helper()
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	doc := routerDocument(three()...)
	doc.Events = []status.Event{{ID: 1, At: past(-time.Minute), Kind: status.EventPin, Accounts: []string{"personal"}}}
	h := routedHarness(t, doc)
	onTheLog(h, state)
	h.start()
	return h
}

// heldTail is the next read of the ledger's tail h asks, held back
// undelivered, the Log's own read as it was.
func heldTail(t *testing.T, h *harness) tailedMsg {
	t.Helper()
	held := h.deliverHolding(isTailed, h.press("R")...)
	if len(held) != 1 {
		t.Fatalf("%d reads of the tail held back, want the one", len(held))
	}
	return held[0].(tailedMsg)
}

// A read of the tail landing, the Log's own read as it was, makes the Log's
// Events again where its feed told otherwise of a session the store holds,
// as Retell has it, and where a reader's read joined the store while it
// was under way, the store the read fed taking it in; and only there.
func TestAReadOfTheTailMakesTheEventsAgainAsItChangesTheStore(t *testing.T) {
	tests := []struct {
		name  string
		while func(h *harness, msg *tailedMsg)
		made  bool
	}{
		{name: "its feed retelling a session it holds", while: func(_ *harness, msg *tailedMsg) { msg.retells = true }, made: true},
		{name: "a reader's read joining the store while it was under way", while: func(h *harness, _ *tailedMsg) {
			h.model = h.model.heldWith(views.HoldSessions(ledger.Pricing))
			h.deliver(frameMsg{gen: h.model.frames})
		}, made: true},
		{name: "its feed retelling nothing, and none joining", while: func(*harness, *tailedMsg) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := eventsOnTheLog(t)
			msg := heldTail(t, h)
			tt.while(h, &msg)
			read, made := h.model.log.read, h.model.log.built.events
			h.deliver(msg)
			if !h.model.log.read.Same(read) {
				t.Fatal("the read of the tail grew the Log's own read, want it as it was")
			}
			if got := h.model.log.built.events != made; got != tt.made {
				t.Errorf("the read of the tail landed, the Events made again %t, want %t", got, tt.made)
			}
		})
	}
}

// A reader's read joining the store makes the Log's panel's figures again
// at the next build, as its sessions are told by the store.
func TestTheLogsPanelIsMadeAgainAsAReadersReadJoinsTheStore(t *testing.T) {
	h := eventsOnTheLog(t)
	h.typed("f")
	made := h.model.log.built.panel
	if made == nil {
		t.Fatal("f opened no panel")
	}
	h.model = h.model.heldWith(views.HoldSessions(ledger.Pricing))
	h.deliver(frameMsg{gen: h.model.frames})
	if h.model.log.built.panel == made {
		t.Error("a reader's read joined the store, the panel's figures weren't made again, want them made of the store as it stands")
	}
}

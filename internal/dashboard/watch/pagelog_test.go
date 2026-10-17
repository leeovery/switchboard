package watch

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// openedOn is a model at the clock's start of d28c and 7f3a running, over
// a ledger in state holding lines, priced at the price table's prices,
// opened on the view given, and the ledger. On Runway it reads no List.
func openedOn(t *testing.T, state string, view dashboard.View, lines ...ledger.Held) (*harness, *ledger.Follower) {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 0), sessionOn(id7F3A, "claude-haiku-4-5", "work", 0)}
	writeLines(t, state, lines...)
	l := ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Ledger, cfg.Prices, cfg.View = l, ledger.Pricing, view
	h.model = New(t.Context(), cfg)
	h.start()
	return h, l
}

// helds are the lines given, each as the ledger holds it.
func helds(lines []ledger.Line) []ledger.Held {
	held := make([]ledger.Held, len(lines))
	for i, l := range lines {
		held[i] = ledger.Held{Line: l}
	}
	return held
}

// The Log opens only as it shows, and it shows only once the session's page
// open over another tab is closed, as showing a tab closes it; and, where a
// page's read is under way, only once that has landed, opening nothing: so
// the read of the ledger's tail the Log opens with, of every one of today's
// lines afresh, is given to no page.
func TestTheLogsOpenGivesNoSessionsPageTodaysLinesAfresh(t *testing.T) {
	tests := []struct {
		name string
		open func(*harness) []tea.Msg
	}{
		{name: "the page open", open: func(h *harness) []tea.Msg { h.openPage(idD28C); return nil }},
		{name: "the page's read under way", open: func(h *harness) []tea.Msg { read, _ := h.askedPage(idD28C); return read }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := openedOn(t, t.TempDir(), dashboard.Runway, helds(turnsOfRequests(start.Add(-30*time.Minute), 8, 1))...)
			read := tt.open(h)
			opening := holdingBack[logReadMsg](h, h.update(tabKey)...)
			if h.model.view != dashboard.Log || h.model.session != nil {
				t.Fatalf("tab pressed, the Log shows %t, a page open %t; want the Log shown, the page closed", h.model.view == dashboard.Log, h.model.session != nil)
			}
			if read != nil {
				if len(opening) != 0 {
					t.Fatalf("the Log shown, a page's read under way, the Log's open was asked %d times; want it waiting on the page's read", len(opening))
				}
				opening = holdingBack[logReadMsg](h, read...)
				if h.model.session != nil {
					t.Fatal("the read of the page asked for before the Log showed landed, it opened; want it opening nothing")
				}
				opening = append(opening, holdingBack[logReadMsg](h, h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)...)
			}
			if len(opening) != 1 {
				t.Fatalf("the Log shown, its open was asked %d times, want once", len(opening))
			}
			if msg := opening[0].(logReadMsg); msg.tailed == nil || msg.tailed.page != nil {
				t.Errorf("the Log's open read the tail %t, for a page %t; want it read afresh for no page", msg.tailed != nil, msg.tailed != nil && msg.tailed.page != nil)
			}
		})
	}
}

// A session's page, its turn open, given every one of today's lines afresh
// by a read of the ledger's tail, as the Log's open gives them, and the
// tail gives them as the day's file is replaced, holds each line once,
// those without an id too: the page is the page read whole, its turn open
// shows each of its requests once, and the store counts each once.
func TestAPageGivenTodaysLinesAfreshHoldsEachLineOnce(t *testing.T) {
	state := t.TempDir()
	lines := turnsOfRequests(start.Add(-30*time.Minute), 8, 1)
	h, l := openedOn(t, state, dashboard.Runway, helds(lines)...)
	h.openPage(idD28C)
	h.keys(downKey)
	read, _ := h.openingTurn()
	h.deliver(read...)
	if shown := h.shownPage(); len(shown.Requests) != 8 {
		t.Fatalf("turn 1 opened, it shows %d rows, want its 8 requests", len(shown.Requests))
	}

	file := filepath.Join(ledger.Dir(state), "requests-"+dayfile.DateOf(start)+".jsonl")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	replaced := filepath.Join(t.TempDir(), "requests")
	if err := os.WriteFile(replaced, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replaced, file); err != nil {
		t.Fatal(err)
	}
	h.model.tail.due = true
	tailed := holdingBack[tailedMsg](h, h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
	if len(tailed) != 1 || !tailed[0].(tailedMsg).lines.afresh || len(tailed[0].(tailedMsg).lines.gained) != len(lines) {
		t.Fatalf("the day's file replaced, the tail was read %d times; want it read once, given every one of today's %d lines afresh", len(tailed), len(lines))
	}
	h.deliver(tailed...)

	shown := h.shownPage()
	if len(shown.Requests) != 8 {
		t.Errorf("given today's lines afresh, turn 1 shows %d rows, want its 8 requests, each once", len(shown.Requests))
	}
	now := h.clock.now
	live := h.model.pageLive(idD28C, now)
	live.Stream = live.Stream.Of(idD28C)
	if want, _ := views.ReadPage(l, idD28C, live.Stream, h.model.cfg.Prices, h.model.cfg.LedgerKeep, now).Build(live); !reflect.DeepEqual(shown.Page, want.Page) {
		t.Errorf("given today's lines afresh, the page is\n%+v\nwant it read whole\n%+v", shown.Page, want.Page)
	}
	if got := listedToday(h).Requests; got != len(lines) {
		t.Errorf("given today's lines afresh, the store counts %d requests today, want the %d the ledger holds", got, len(lines))
	}
}

// The store, fed by the page's open, the Log's open and its read of the day
// before, in either order, a read of the ledger's tail between and after,
// holds what one fed whole holds, wanting what its last feed wanted: the
// List of it alike, and alike its requests held open.
func TestTheStoreFedByThePageAndTheLogInEitherOrderIsAsOneFedWhole(t *testing.T) {
	gained := func(id string) func(*harness, string) {
		return func(h *harness, state string) {
			h.clock.now = h.clock.now.Add(time.Minute)
			writeLines(t, state, arrivedAt(id, h.clock.now.Add(-time.Second)))
			h.model.tail.due = true
			h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
		}
	}
	page := func(h *harness, _ string) { h.openPage(idD28C) }
	log := func(h *harness, _ string) {
		h.keys(escKey, tabKey)
		h.settle()
	}
	tests := []struct {
		name  string
		view  dashboard.View
		steps []func(*harness, string)
	}{
		{name: "the page, then the Log", view: dashboard.Runway, steps: []func(*harness, string){page, gained("0a000101"), log, gained("0a000102")}},
		{name: "the Log, then the page", view: dashboard.Log, steps: []func(*harness, string){func(h *harness, _ string) { h.settle() }, gained("0a000101"), page, gained("0a000102")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			h, l := openedOn(t, state, tt.view,
				arrived("0a000001", "work", -20*time.Minute), ranIn("0a000002", id7F3A, "~/Code/api", -10*time.Minute),
				ranIn("0b000001", id9E21, "~/Code/web", -day),
			)
			for _, step := range tt.steps {
				step(h, state)
			}
			if !h.model.log.opened || h.model.log.from.Date != dayfile.DateOf(start.Add(-day)) {
				t.Fatalf("the Log opened %t, spanning from %s; want it opened, read back to the day before", h.model.log.opened, h.model.log.from.Date)
			}
			if got := h.model.held.Len(); got != 3 {
				t.Errorf("the store holds %d sessions, want the 3 the Log shows", got)
			}
			asFedWhole(t, h, l)
		})
	}
}

// asFedWhole checks the store h holds is as one fed whole through l at the
// clock's now holds, wanting what its last feed wanted, the sessions of
// the Log's lines among them: the List of it alike, and alike its requests
// held open.
func asFedWhole(t *testing.T, h *harness, l views.Ledger) {
	t.Helper()
	now := h.clock.now
	want := h.model.heldFold(now).want
	want.Log = h.model.log.read.Wanted()
	whole, _ := views.HoldSessions(h.model.cfg.Prices).Fed(l, views.Taken{}, want, now)
	live := h.model.listLive(now, h.model.traffic.stream(now))
	if got, want := h.model.held.List(live), whole.List(live); !reflect.DeepEqual(got, want) {
		t.Errorf("the List of the store is\n%+v\nwant it as one fed whole\n%+v", got, want)
	}
	if got, want := h.model.held.Holding().Open, whole.Holding().Open; !maps.Equal(got, want) {
		t.Errorf("the store holds %v requests open, want %v, as one fed whole", got, want)
	}
}

// The Log opened past midnight, a page closed before it, nothing reading
// the ledger's tail between, feeds the store as the Log wants it, whatever
// it still holds of the page: the day before read again, as the tail
// followed it no further, so the line filed on it after the tail's last
// read is the store's too.
func TestTheLogOpenedAfterAPageClosedFeedsTheStoreTheDayTheTailMissed(t *testing.T) {
	state := t.TempDir()
	h, l := openedOn(t, state, dashboard.Runway, arrivedAt("0a000001", onTheDay(23, 40, 0)))
	h.clock.now = onTheDay(23, 50, 0)
	h.openPage(idD28C)
	h.model.tail.due = true
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
	if h.model.tail.unread() || h.model.held.Len() != 1 {
		t.Fatalf("the page open, the tail unread %t, the store holding %d sessions; want the tail read, the page's session held", h.model.tail.unread(), h.model.held.Len())
	}
	h.clock.now = onTheDay(23, 59, 0)
	writeLines(t, state, took(arrivedAt("0a000002", onTheDay(23, 58, 30)), time.Second))
	h.keys(escKey)
	h.clock.now = onTheDay(24, 1, 0)
	writeLines(t, state, took(arrivedAt("0a000003", onTheDay(24, 0, 30)), time.Second))
	h.keys(tabKey)
	if !h.model.log.opened {
		t.Fatal("tab pressed, the Log isn't open, want it opened")
	}
	asFedWhole(t, h, l)
}

package watch

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
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
	l := ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"))
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Ledger, cfg.View = l, view
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

// The Log opens only as it shows: a page open over another tab is closed as
// the Log shows, and, where a page's read is under way, the Log opens only
// once that has landed, opening nothing; a page opened over the Log itself,
// from one of its lines, opens over a Log already shown, its open asked. So
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
// tail gives them as the day's file is replaced: its turn open shows each of
// its requests once, the page drawn is the page read whole, its points among
// it, each rise of work's shared with another session's lines without ids,
// and the store counts each line once. Of its own lines, some without an id,
// the store reads the session whole, as it can't place a line without one;
// each with one, it reads nothing whole, its points taking each line in as
// often as the day's file holds it.
func TestAPageGivenTodaysLinesAfreshHoldsEachLineOnce(t *testing.T) {
	reading := func(l ledger.Line, used float64) ledger.Line {
		l.Usage = json.RawMessage(`{"input_tokens":1000}`)
		l.Limits = map[string]string{"5h-utilization": strconv.FormatFloat(used, 'f', 2, 64), "5h-reset": strconv.FormatInt(start.Add(4*time.Hour).Unix(), 10)}
		return l
	}
	firstMS := int64(1000)
	tests := []struct {
		name  string
		every int
		whole bool
	}{
		{name: "its own lines with ids and without", every: 2, whole: true},
		{name: "its own lines each with an id", every: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			var lines []ledger.Line
			for i, l := range turnsOfRequests(start.Add(-30*time.Minute), 8, 1) {
				if i%tt.every == 0 {
					l.Request = fmt.Sprintf("0a%06d", i+1)
				}
				other := ledger.Line{At: l.At.Add(5 * time.Second), Kind: ledger.KindMessage, Session: id7F3A, Model: "claude-haiku-4-5", Account: "work",
					Reason: "new", Status: 200, Attempts: 1, FirstMS: &firstMS, TotalMS: 3000, Class: ledger.ClassMain}
				lines = append(lines, reading(l, 0.10+0.08*float64(i)), reading(other, 0.14+0.08*float64(i)))
			}
			h, l := openedOn(t, state, dashboard.Runway, helds(lines)...)
			h.openPage(idD28C)
			h.keys(downKey)
			read, _ := h.openingTurn()
			h.deliver(read...)
			shown := h.shownPage()
			if len(shown.Requests) != 8 || len(shown.Page.Accounts) != 1 || shown.Page.Accounts[0].Points["5h"] == 0 {
				t.Fatalf("turn 1 opened, it shows %d rows, the page's accounts %+v; want its 8 requests, points of work's 5-hour", len(shown.Requests),
					shown.Page.Accounts)
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

			shown = h.shownPage()
			if len(shown.Requests) != 8 {
				t.Errorf("given today's lines afresh, turn 1 shows %d rows, want its 8 requests, each once", len(shown.Requests))
			}
			now := h.clock.now
			live := h.model.pageLive(idD28C, now)
			live.Stream = live.Stream.Of(idD28C)
			if want, _ := views.ReadPage(l, idD28C, live.Stream, h.model.cfg.Prices, h.model.cfg.LedgerKeep, now).Build(live); !reflect.DeepEqual(shown.Page, want.Page) {
				t.Errorf("given today's lines afresh, the page is\n%+v\nwant it read whole\n%+v", shown.Page, want.Page)
			}
			if got, want := listedToday(h).Requests, len(lines)/2; got != want {
				t.Errorf("given today's lines afresh, the store counts %d requests today, want the %d of the page's session the ledger holds", got, want)
			}
			if whole := h.model.held.Holding().Read > 0; whole != tt.whole {
				t.Errorf("given today's lines afresh, the store read the page's session whole %t, want %t", whole, tt.whole)
			}
		})
	}
}

// The store, fed by the page's open, the Log's open and its read of the day
// before, in either order, a read of the ledger's tail between and after,
// holds what one fed whole holds, wanting what its last feed wanted: the
// List of it alike, alike its requests held open, and alike what moving the
// session read back would cost, by the prices its story was read at, once
// a line today has it writing its cache.
func TestTheStoreFedByThePageAndTheLogInEitherOrderIsAsOneFedWhole(t *testing.T) {
	gained := func(l ledger.Held) func(*harness, string) {
		return func(h *harness, state string) {
			h.clock.now = h.clock.now.Add(time.Minute)
			l.At = h.clock.now.Add(-time.Second)
			writeLines(t, state, l)
			h.model.tail.due = true
			h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
		}
	}
	readBack := ranIn("0b000101", id9E21, "~/Code/web", 0)
	readBack.Usage = json.RawMessage(hourUsage)
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
		{name: "the page, then the Log", view: dashboard.Runway, steps: []func(*harness, string){page, gained(arrived("0a000101", "work", 0)), log, gained(readBack)}},
		{name: "the Log, then the page", view: dashboard.Log, steps: []func(*harness, string){func(h *harness, _ string) { h.settle() }, gained(arrived("0a000101", "work", 0)), page, gained(readBack)}},
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
// the Log's lines among them: the List of it alike, alike its requests held
// open, and alike every session it holds, each listed as running, the List
// telling of a running session's story more than of the rest, its move's
// cost among it.
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
	every := live
	every.Running = slices.Clone(live.Running)
	for _, id := range slices.Sorted(maps.Keys(h.model.held.Holding().Open)) {
		if !slices.ContainsFunc(live.Running, func(s status.Session) bool { return s.ID == id }) {
			every.Running = append(every.Running, sessionOn(id, opus, "work", 0))
		}
	}
	if got, want := h.model.held.List(every).Sessions, whole.List(every).Sessions; !reflect.DeepEqual(got, want) {
		t.Errorf("every session the store holds, listed as running, is\n%+v\nwant each as one fed whole\n%+v", got, want)
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

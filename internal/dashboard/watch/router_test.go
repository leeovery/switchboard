package watch

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// routerKeys are the keys the footer lists while the dashboard reads the
// router of three accounts, and probingKeys those it lists while it probes.
const (
	routerKeys  = "tab views · w window: auto · ←→ focus · space flip · g chart · 1-3 pin · a auto · m move · ? keys · q quit"
	probingKeys = "tab views · w window: auto · ←→ focus · space flip · g chart · ? keys · q quit"
	// unreadKeys are those listed before anything is read, w doing nothing
	// till there are cards.
	unreadKeys = "tab views · ←→ focus · space flip · g chart · ? keys · q quit"
)

func TestLooksAtTheRoutersDocumentEveryFiveSeconds(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	if got, want := h.source.asked, []status.Read{{Refresh: interval, Probe: true}}; !slices.Equal(got, want) {
		t.Fatalf("starting asked for %+v, want %+v: the router refreshing what it hasn't read in an interval", got, want)
	}
	for i := range 3 {
		want := start.Add(time.Duration(i+1)*lookEvery + tickSlack)
		if got := h.tickUntilAsked(); !got.Equal(want) {
			t.Errorf("look %d at %s, want %s", i+1, got.Format(time.StampMilli), want.Format(time.StampMilli))
		}
	}
	want := []status.Read{{Refresh: interval, Probe: true}, {}, {}, {}}
	if !slices.Equal(h.source.asked, want) {
		t.Errorf("asked for %+v, want %+v: a look at the router's document every five seconds, which never probes", h.source.asked, want)
	}
	if h.source.reads != 0 {
		t.Errorf("probed %d times, want never while the router answers", h.source.reads)
	}
	if got, want := h.footer(), routerKeys+" · read 0s ago"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestTheRoutersDocumentIsShownWithWhatsWorkedOutOfIt(t *testing.T) {
	given := routerDocument(three()...)
	want := given.WorkedOut(policy)
	if want.ComingUp == nil || want.Pool.Windows == nil {
		t.Fatalf("WorkedOut() = %+v, want what's coming up and the pool", want)
	}
	tests := []struct {
		name string
		doc  status.Document
	}{
		{name: "from a router from before, which gives none of it", doc: given},
		{name: "from a router that gives it, as it gave it", doc: want},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.start()

			if !reflect.DeepEqual(h.model.doc, want) {
				t.Errorf("the document shown is\n%+v\nwant the router's, with what's worked out of it\n%+v", h.model.doc, want)
			}
		})
	}
}

func TestEveryReadOfTheRouterListsItsSessionsForTheCards(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{
		{ID: "d28c5e17", Assignments: []status.Assignment{{Model: "claude-opus-5-5", Account: "work", LastSeen: start.Add(-50 * time.Second).UTC()}}},
		{ID: "7f3a0c94", Assignments: []status.Assignment{{Model: "claude-haiku-4-5", Account: "work", LastSeen: start.Add(-9 * time.Minute).UTC()}}},
	}
	h.start()

	if h.source.listed != 1 || len(h.model.sessions) != 2 {
		t.Fatalf("listed the sessions %d times, holding %d, want once with the first read, holding both", h.source.listed, len(h.model.sessions))
	}
	h.tickUntilAsked()
	if h.source.listed != 2 {
		t.Errorf("listed the sessions %d times, want again with a look at the router's document", h.source.listed)
	}
	if !strings.Contains(h.view(), "╰─ ● ○ ─") {
		t.Errorf("the screen is\n%s\nwant work's card with a dot for each session, the one seen in the last minute lit", h.view())
	}
}

func TestProbingListsNoSessions(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	if h.source.listed != 0 || h.model.sessions != nil {
		t.Errorf("listed the sessions %d times, holding %v, want never while probing", h.source.listed, h.model.sessions)
	}
}

func TestSessionsTheRouterDoesntListLeaveTheCardsWithoutDots(t *testing.T) {
	work := account("work", "Work", session(0.25, 3*time.Hour), week(0.5))
	work.Sessions = 2
	h := routedHarness(t, routerDocument(work, three()[1]))
	h.source.sessions = []status.Session{{ID: "d28c5e17", Assignments: []status.Assignment{{Account: "work", LastSeen: start.UTC()}}}}
	h.source.sessionsErr = errors.New("connection reset")
	h.start()

	if h.model.sessions != nil {
		t.Errorf("holding the sessions %v, want none listed", h.model.sessions)
	}
	if view := h.view(); strings.Contains(view, "╰─ ●") || !strings.Contains(view, " 2 sessions ─╯") {
		t.Errorf("the screen is\n%s\nwant work's card without dots, counting its sessions as the document does", view)
	}
}

func TestHasTheRouterRefreshOnceAnInterval(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	var refreshed []time.Time
	for h.clock.now.Before(at(14, 12, 30)) {
		asked := len(h.source.asked)
		h.fire(h.lastTick())
		for _, r := range h.source.asked[asked:] {
			if r.Refresh > 0 {
				refreshed = append(refreshed, h.clock.now)
			}
		}
	}
	if len(refreshed) != 2 {
		t.Fatalf("the router refreshed at %v, want twice more in the hour", refreshed)
	}
	for i, due := range []time.Time{at(13, 42, 0), refreshed[0].Add(interval)} {
		if got := refreshed[i]; got.Before(due) || got.After(due.Add(lookEvery+tickSlack)) {
			t.Errorf("refresh %d at %s, want the first tick at or after %s", i+1, got.Format(time.StampMilli), due.Format(time.StampMilli))
		}
	}
}

func TestHasTheRouterRefreshSoonerWhileAnAccountCantBeRead(t *testing.T) {
	h := routedHarness(t, routerDocument(account("work", "Work", session(0.25, 3*time.Hour), week(0.5)), unreadable("side", "Side")))
	h.start()

	var refreshed []time.Time
	for h.clock.now.Before(at(13, 19, 0)) {
		asked := len(h.source.asked)
		h.fire(h.lastTick())
		if slices.ContainsFunc(h.source.asked[asked:], func(r status.Read) bool { return r.Refresh > 0 }) {
			refreshed = append(refreshed, h.clock.now)
		}
	}
	for i, want := range []time.Time{at(13, 14, 0), at(13, 18, 0)} {
		if i >= len(refreshed) || refreshed[i].Before(want) || refreshed[i].After(want.Add(lookEvery+tickSlack)) {
			t.Errorf("the router refreshed at %v, want refresh %d at the look after %s, backing off", refreshed, i+1, want.Format(time.Kitchen))
		}
	}
}

func TestHasTheRouterRefreshOnceAWindowOnScreenResets(t *testing.T) {
	// The router's document shows work's week resetting at 13:20, and goes on
	// showing it after, as it would were work idle.
	h := routedHarness(t, routerDocument(
		account("work", "Work", session(0.25, 3*time.Hour), window("7d", "Week", 0.5, 8*time.Minute)),
		account("side", "Side", session(0.4, 2*time.Hour), week(0.6)),
	))
	h.start()

	refreshed := h.refreshesUntil(at(13, 25, 0))
	due := at(13, 21, 0)
	if len(refreshed) != 1 || refreshed[0].Before(due) || refreshed[0].After(due.Add(lookEvery+tickSlack)) {
		t.Errorf("the router refreshed what it hadn't read in the last minute at %v, want once, at the first look from %s, a minute after work's week reset",
			refreshed, due.Format(time.Kitchen))
	}
}

func TestShowsASessionThatHasLapsedEmptyRatherThanRefreshingIt(t *testing.T) {
	// Work's session resets at 13:20, and nothing is read of work since, so
	// from then on the router's document shows it lapsed.
	side := account("side", "Side", session(0.4, 2*time.Hour), week(0.6))
	h := routedHarness(t, routerDocument(account("work", "Work", session(0.25, 8*time.Minute), week(0.5)), side))
	h.start()
	h.tickUntil(at(13, 20, 0))
	work := account("work", "Work", quota.Window{Key: "5h", Label: "Session"}, week(0.5))
	work.Lapsed = []string{"5h"}
	h.startRouter(routerDocument(work, side))

	if refreshed := h.refreshesUntil(at(13, 25, 0)); len(refreshed) > 0 {
		t.Errorf("the router refreshed what it hadn't read in the last minute at %v, want never: it doesn't probe an account whose session has lapsed", refreshed)
	}
	if !strings.Contains(h.view(), "not started") {
		t.Errorf("the screen is\n%s\nwant work's session empty, not started", h.view())
	}
}

func TestAWindowResettingOnAnAccountWhoseSessionHasLapsedHasTheRouterRefreshNothing(t *testing.T) {
	// Work's session has lapsed, and its week resets at 13:20; side's session
	// runs, and its week resets at 13:22.
	work := account("work", "Work", quota.Window{Key: "5h", Label: "Session"}, window("7d", "Week", 0.5, 8*time.Minute))
	work.Lapsed = []string{"5h"}
	h := routedHarness(t, routerDocument(work, account("side", "Side", session(0.4, 2*time.Hour), window("7d", "Week", 0.6, 10*time.Minute))))
	h.start()

	refreshed := h.refreshesUntil(at(13, 25, 0))
	due := at(13, 23, 0)
	if len(refreshed) != 1 || refreshed[0].Before(due) || refreshed[0].After(due.Add(lookEvery+tickSlack)) {
		t.Errorf("the router refreshed what it hadn't read in the last minute at %v, want once, at the first look from %s, a minute after side's week reset: the router won't probe work",
			refreshed, due.Format(time.Kitchen))
	}
}

func TestHasTheRouterRefreshAtOnceForAWindowThatResetBeforeTheWatchBegan(t *testing.T) {
	h := routedHarness(t, routerDocument(account("work", "Work", session(0.25, 3*time.Hour), window("7d", "Week", 0.5, -12*time.Minute))))
	h.start()

	h.tickUntilAsked()
	h.tickUntilAsked()
	want := []status.Read{{Refresh: interval, Probe: true}, {Refresh: status.FreshFor}, {}}
	if !slices.Equal(h.source.asked, want) {
		t.Errorf("asked for %+v, want %+v: the first look has the router refresh what it hasn't read in the last minute, once", h.source.asked, want)
	}
}

func TestKeepsTheRoutersDocumentWhileItDoesntAnswerALook(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.stopRouter()
	h.tickUntilAsked() // 13:12:05.
	looks := h.source.looks()
	h.tickUntil(at(13, 13, 5).Add(tickSlack))
	if h.source.reads != 0 {
		t.Fatalf("while the router doesn't answer a look, probed %d times, want never: the router may be away for a moment", h.source.reads)
	}
	if got := h.source.looks() - looks; got < 10 || got > 12 {
		t.Errorf("looked at the router's document %d times in the minute after it stopped answering, want every 5 seconds or so", got)
	}
	if !h.model.routed() {
		t.Error("the document on screen isn't the router's, want its last one kept")
	}
	if got, want := h.footer(), probingKeys+" · no router since 13:12"; got != want {
		t.Errorf("footer = %q, want %q: the keys that give the router orders not working, and since when there's been no router", got, want)
	}
	if !strings.Contains(h.view(), "○ no router since 13:12") {
		t.Errorf("the screen is\n%s\nwant ROUTER to say there's been no router since it stopped answering", h.view())
	}

	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked()
	if got, want := h.footer(), routerKeys+" · read 0s ago"; got != want {
		t.Errorf("once the router answers a look again, footer = %q, want %q", got, want)
	}
	if strings.Contains(h.view(), "no router") {
		t.Errorf("the screen is\n%s\nwant ROUTER to say no more of there being no router", h.view())
	}
}

func TestFallsBackToProbingWhenTheRouterStops(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.stopRouter()
	h.tickUntil(at(13, 41, 59))
	if h.source.reads != 0 {
		t.Fatalf("before the full read is due, probed %d times, want never", h.source.reads)
	}
	h.tickUntil(at(13, 42, 0).Add(tickSlack))
	if h.source.reads != 1 {
		t.Fatalf("once the full read is due, probed %d times, want once", h.source.reads)
	}
	if got, want := h.footer(), probingKeys+" · read 0s ago · next 14:12"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if view := h.view(); !strings.Contains(view, "○ probing") || !strings.Contains(view, "router not running") {
		t.Errorf("the screen is\n%s\nwant it to say the accounts are probed, as the router isn't running", view)
	}
	h.deliver(h.press("1")...)
	if len(h.source.orders) > 0 {
		t.Errorf("while probing, 1 gave the router %q, want nothing", h.source.orders)
	}
}

func TestReadsTheRouterAgainOnceItAnswers(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.stopRouter()
	h.deliver(h.press("r")...) // Probing from 13:12.

	h.tickUntil(at(13, 13, 30))
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked() // 13:14.
	if got, want := h.footer(), routerKeys+" · read 0s ago"; got != want {
		t.Errorf("once the router answers again, footer = %q, want %q", got, want)
	}
	if got, want := h.source.asked[len(h.source.asked)-1], (status.Read{Refresh: interval}); got != want {
		t.Errorf("the read that found the router asked for %+v, want %+v: a question after the router, having it refresh", got, want)
	}
	if h.source.reads != 1 {
		t.Errorf("probed %d times, want once, as the router stopped: asking after it probes nothing", h.source.reads)
	}
}

func TestAsksAfterTheRouterOnceAMinuteAtMost(t *testing.T) {
	// The session is back at 13:17, so until then a countdown ticks every
	// second.
	h := newHarness(t, document(account("work", "Work", refused(session(1, 5*time.Minute)), week(0.5))))
	h.start()

	ticks := h.tickUntil(at(13, 16, 30))
	if ticks < 200 {
		t.Fatalf("ticked %d times by 13:16:30, want every second", ticks)
	}
	if got := countReads(h.source.asked, status.Read{Refresh: interval}); got != 4 {
		t.Errorf("asked after the router %d times by 13:16:30, want 4: once at each minute", got)
	}
}

func TestAsksAfterTheRouterWhenAProbeIsDue(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	h.tickUntil(at(13, 41, 59).Add(tickSlack))
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked() // 13:42, when the probe is due.
	if got, want := h.source.asked[len(h.source.asked)-1], (status.Read{Refresh: interval, Probe: true}); got != want {
		t.Errorf("the read due at 13:42 asked for %+v, want %+v", got, want)
	}
	if !h.model.routed() || h.source.reads != 1 {
		t.Errorf("after the read due at 13:42, routed = %v and probes = %d, want the router read, and no probe", h.model.routed(), h.source.reads)
	}
}

func TestALookIsQuiet(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.tickUntil(at(13, 12, 4).Add(tickSlack))
	tick := h.lastTick()
	h.clock.now, tick.fired = tick.due, true
	looking := h.update(tick.msg)
	if len(looking) == 0 {
		t.Fatal("the tick didn't look at the router's document")
	}
	if got, want := h.footer(), routerKeys+" · read 5s ago"; got != want {
		t.Errorf("while looking at the router's document, footer = %q, want %q", got, want)
	}
	h.deliver(looking...)
}

func TestRefreshKeyReadingTheRouter(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	pending := h.press("r")
	if got, want := h.source.asked[len(h.source.asked)-1], (status.Read{Refresh: status.FreshFor, Probe: true}); got != want {
		t.Errorf("r asked for %+v, want %+v: the router refreshing what it hasn't read in the last minute", got, want)
	}
	if got, want := h.footer(), routerKeys+" · refreshing…"; got != want {
		t.Errorf("while refreshing, footer = %q, want %q", got, want)
	}
	h.clock.now = at(13, 20, 0)
	h.deliver(pending...)
	if got, want := h.footer(), routerKeys+" · read 0s ago"; got != want {
		t.Errorf("after refreshing, footer = %q, want %q", got, want)
	}
}

func TestKeysTellTheRouterWhereToSendSessions(t *testing.T) {
	pinned := func(ids ...string) status.Document {
		doc := routerDocument(three()...)
		doc.Pin = status.Pin{Accounts: ids, Since: start.UTC()}
		return doc
	}
	// Personal, pinned beside work, has lost its token since.
	lost := pinned("work", "personal")
	lost.Accounts[1] = tokenless("personal", "Personal")
	tests := []struct {
		name       string
		doc        status.Document
		key        string
		wantOrders []string
		wantNote   string
		wantPin    []string
	}{
		{
			name:       "1 pins new sessions to the first account",
			doc:        routerDocument(three()...),
			key:        "1",
			wantOrders: []string{"pin work"},
			wantNote:   "new sessions go to work · Work",
			wantPin:    []string{"work"},
		},
		{
			name:       "3 pins the third beside the first",
			doc:        pinned("work"),
			key:        "3",
			wantOrders: []string{"pin work,side"},
			wantNote:   "new sessions go to the best of work · Work and side · Side",
			wantPin:    []string{"work", "side"},
		},
		{
			name:       "2 pins the second between them, in the order configured",
			doc:        pinned("work", "side"),
			key:        "2",
			wantOrders: []string{"pin work,personal,side"},
			wantNote:   "new sessions go to the best of work · Work, personal · Personal and side · Side",
			wantPin:    []string{"work", "personal", "side"},
		},
		{
			name:       "3 unpins the third, pinned already, leaving the first",
			doc:        pinned("work", "side"),
			key:        "3",
			wantOrders: []string{"pin work"},
			wantNote:   "new sessions go to work · Work",
			wantPin:    []string{"work"},
		},
		{
			name:       "3 unpins the last account pinned, routing every session automatically again",
			doc:        pinned("side"),
			key:        "3",
			wantOrders: []string{"unpin"},
			wantNote:   "routing automatically",
		},
		{
			name:       "a routes every session automatically again",
			doc:        pinned("work", "side"),
			key:        "a",
			wantOrders: []string{"unpin"},
			wantNote:   "routing automatically",
		},
		{
			name:       "m moves running sessions to the account pinned",
			doc:        pinned("side"),
			key:        "m",
			wantOrders: []string{"move side"},
			wantNote:   "running sessions move to side · Side",
			wantPin:    []string{"side"},
		},
		{
			name:       "m moves running sessions to the accounts pinned",
			doc:        pinned("work", "side"),
			key:        "m",
			wantOrders: []string{"move work,side"},
			wantNote:   "running sessions move to the best of work · Work and side · Side",
			wantPin:    []string{"work", "side"},
		},
		{
			name:       "3 pins the third beside the first, leaving out one that has lost its token",
			doc:        lost,
			key:        "3",
			wantOrders: []string{"pin work,side"},
			wantNote:   "new sessions go to the best of work · Work and side · Side",
			wantPin:    []string{"work", "side"},
		},
		{
			name:       "1 unpins the first, leaving out one that has lost its token",
			doc:        lost,
			key:        "1",
			wantOrders: []string{"unpin"},
			wantNote:   "routing automatically",
		},
		{
			name:       "m moves running sessions to the accounts pinned with a token",
			doc:        lost,
			key:        "m",
			wantOrders: []string{"move work"},
			wantNote:   "running sessions move to work · Work",
			wantPin:    []string{"work"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.start()
			looks := h.source.looks()

			h.deliver(h.press(tt.key)...)
			if !slices.Equal(h.source.orders, tt.wantOrders) {
				t.Errorf("orders = %q, want %q", h.source.orders, tt.wantOrders)
			}
			if got, want := h.footer(), tt.wantNote+" · read 0s ago"; got != want {
				t.Errorf("footer = %q, want %q", got, want)
			}
			if got := h.source.looks(); got != looks+1 {
				t.Errorf("looked at the router's document %d times after the key, want once", got-looks)
			}
			if got := h.model.doc.Pin.Accounts; !slices.Equal(got, tt.wantPin) {
				t.Errorf("the document on screen is pinned to %q, want %q", got, tt.wantPin)
			}
		})
	}
}

func TestMoveWithoutAnAccountPinnedToMoveToSaysSo(t *testing.T) {
	lost := routerDocument(three()...)
	lost.Accounts[1] = tokenless("personal", "Personal")
	lost.Pin = status.Pin{Accounts: []string{"personal"}, Since: start.UTC()}
	tests := []struct {
		name     string
		doc      status.Document
		wantNote string
	}{
		{name: "nothing pinned", doc: routerDocument(three()...), wantNote: "nothing's pinned to move sessions to: pin an account with 1-3"},
		{name: "the one pinned without a token", doc: lost, wantNote: "no account pinned has a usable token to move sessions to: pin another with 1-3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.start()
			asked := len(h.source.asked)

			h.deliver(h.press("m")...)
			if len(h.source.orders) > 0 || len(h.source.asked) > asked {
				t.Errorf("orders = %q, and read %d times, want neither", h.source.orders, len(h.source.asked)-asked)
			}
			if got, want := h.footer(), tt.wantNote+" · read 0s ago"; got != want {
				t.Errorf("footer = %q, want %q", got, want)
			}
		})
	}
}

func TestKeysWithoutAnAccountInTheirPlaceDoNothing(t *testing.T) {
	for _, key := range []string{"4", "9", "0"} {
		t.Run(key, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()

			h.deliver(h.press(key)...)
			if len(h.source.orders) > 0 {
				t.Errorf("orders = %q, want none", h.source.orders)
			}
			if got, want := h.footer(), routerKeys+" · read 0s ago"; got != want {
				t.Errorf("footer = %q, want %q", got, want)
			}
		})
	}
}

func TestPinKeysDoNothingWhileProbing(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.start()

	for _, key := range []string{"1", "a", "m"} {
		h.deliver(h.press(key)...)
	}
	if len(h.source.orders) > 0 {
		t.Errorf("orders = %q, want none", h.source.orders)
	}
	if got, want := h.footer(), probingKeys+" · read 0s ago · next 13:42"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestPinKeysSaySoWhileTheRouterDoesntAnswer(t *testing.T) {
	for _, key := range []string{"1", "a", "m"} {
		t.Run(key, func(t *testing.T) {
			doc := routerDocument(three()...)
			doc.Pin = status.Pin{Accounts: []string{"work"}, Since: start.UTC()}
			h := routedHarness(t, doc)
			h.start()
			h.stopRouter()
			h.tickUntilAsked() // 13:12:05.

			h.deliver(h.press(key)...)
			if len(h.source.orders) > 0 {
				t.Errorf("orders = %q, want none", h.source.orders)
			}
			if got, want := h.footer(), "the router isn't answering · no router since 13:12"; got != want {
				t.Errorf("footer = %q, want %q", got, want)
			}
		})
	}
}

func TestAKeysNoteLapses(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.deliver(h.press("2")...)

	lapse, ok := h.pending(noteMsg{})
	if !ok {
		t.Fatal("no timer armed for the note to lapse")
	}
	if lapse.delay != noteFor {
		t.Errorf("the note lapses after %v, want %v", lapse.delay, noteFor)
	}
	h.clock.now = lapse.due.Add(-time.Millisecond)
	if got, want := h.footer(), "new sessions go to personal · Personal · read 3s ago"; got != want {
		t.Errorf("just before it lapses, footer = %q, want %q", got, want)
	}
	h.fire(lapse)
	if got, want := h.footer(), routerKeys+" · read 4s ago"; got != want {
		t.Errorf("once it lapses, footer = %q, want %q", got, want)
	}
}

func TestAnOrderTheRouterRefusesSaysWhy(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.source.refuse = errors.New("account personal has no usable token, so nothing can go out on it: token missing")
	h.start()

	h.deliver(h.press("2")...)
	if got, want := h.footer(), h.source.refuse.Error()+" · read 0s ago"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	want := []string{"level=WARN", `msg="the router didn't take an order"`, "accounts=personal", "move=false", "error="}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestAKeyWhileTheRouterCarriesOutAnOrderIsDropped(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	pending := h.press("1")
	h.deliver(h.press("2")...)
	h.deliver(pending...)
	if want := []string{"pin work"}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q", h.source.orders, want)
	}
}

func TestALookAskedForWhileAReadIsUnderWayComesOnceItLands(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	reading := h.press("r")
	h.deliver(h.press("1")...)
	if got, want := h.source.asked, []status.Read{{Refresh: interval, Probe: true}, {Refresh: status.FreshFor, Probe: true}}; !slices.Equal(got, want) {
		t.Fatalf("while r's read is under way, asked for %+v, want %+v", got, want)
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
	reading := h.press("r")
	h.deliver(h.press("1")...)
	h.deliver(reading...)
	if h.model.routed() || h.source.reads != 1 {
		t.Errorf("routed = %v, and probed %d times, want probing, once: the look the key asked for is moot", h.model.routed(), h.source.reads)
	}
}

func TestFooterKeysByWhatTheDashboardReads(t *testing.T) {
	many := make([]status.Account, 12)
	for i := range many {
		many[i] = account(string(rune('a'+i)), "Account", session(0.1, 3*time.Hour))
	}
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{name: "the router of three accounts", doc: routerDocument(three()...), want: routerKeys},
		{name: "the router of one, which has none other to pin", doc: routerDocument(three()[0]), want: probingKeys},
		{name: "the router of twelve", doc: routerDocument(many...), want: "tab views · w window: auto · ←→ focus · space flip · g chart · 1-9 pin · a auto · m move · ? keys · q quit"},
		{name: "a probe", doc: probedWithoutTheRouter(), want: probingKeys},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.doc)
			if routed(tt.doc) {
				h.startRouter(tt.doc)
			}
			h.start()

			if got := h.footer(); !strings.HasPrefix(got, tt.want+" · read ") {
				t.Errorf("footer = %q, want it to list %q", got, tt.want)
			}
		})
	}
}

func TestPostsNothingWhileReadingTheRouter(t *testing.T) {
	full := routerDocument(account("work", "Work", refused(session(1, 30*time.Minute)), week(0.85), fableWeek(0.5)))
	room := routerDocument(account("work", "Work", session(0.02, 5*time.Hour), week(0.91), fableWeek(0.5)))
	h := routedHarness(t, full)
	h.start()

	h.startRouter(room)
	h.tickUntilAsked()
	if len(h.notifier.posted) > 0 {
		t.Errorf("reading the router, posted %q, want nothing: the router posts its own", h.notifier.posted)
	}

	h.stopRouter()
	h.source.doc = document(account("work", "Work", session(0.02, 5*time.Hour), week(0.93), fableWeek(0.5)))
	h.clock.now = at(13, 20, 0)
	h.deliver(h.press("r")...)
	if h.model.routed() || len(h.notifier.posted) > 0 {
		t.Errorf("probing once the router stopped, routed = %v and posted %q, want probing, and nothing yet: nothing has changed since the router's last document", h.model.routed(), h.notifier.posted)
	}
	h.source.doc = document(account("work", "Work", session(0.02, 5*time.Hour), week(0.93), fableWeek(0.95)))
	h.clock.now = at(13, 25, 0)
	h.deliver(h.press("r")...)
	if want := []string{"work · Work: Fable week at 95%"}; !slices.Equal(h.notifier.posted, want) {
		t.Errorf("probing once the router stopped, posted %q, want %q: the dashboard's own", h.notifier.posted, want)
	}
}

func TestPostsNothingProbingAsAskedWhileTheRouterAnswers(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, document(account("work", "Work", refused(session(1, 30*time.Minute)), week(0.85), fableWeek(0.5))))
	h.source.probing = true
	h.startRouter(routerDocument(three()...))
	h.start()

	h.clock.now = at(13, 20, 0)
	h.read(document(account("work", "Work", session(0.02, 5*time.Hour), week(0.91), fableWeek(0.5))))
	if h.model.routed() || len(h.notifier.posted) > 0 {
		t.Errorf("probing as asked while the router answers, routed = %v and posted %q, want probing, and nothing: the router posts its own", h.model.routed(), h.notifier.posted)
	}
	for _, news := range []string{`news="room again"`, `news="Week at 91%"`} {
		if want := []string{"level=DEBUG", `msg="notification left to the router" component=watch`, "account=work", news}; !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}

	h.stopRouter()
	h.clock.now = at(13, 25, 0)
	h.read(document(account("work", "Work", session(0.02, 5*time.Hour), week(0.91), fableWeek(0.95))))
	if want := []string{"work · Work: Fable week at 95%"}; !slices.Equal(h.notifier.posted, want) {
		t.Errorf("probing as asked once the router stopped, posted %q, want %q: the dashboard's own, of what changed since", h.notifier.posted, want)
	}
}

func TestLogsTheRouterGoingAndComingBack(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.tickUntilAsked()
	h.stopRouter()
	h.tickUntilAsked()
	h.deliver(h.press("r")...)
	h.startRouter(routerDocument(three()...))
	h.tickUntil(at(13, 13, 30))
	h.deliver(h.press("1")...)

	for _, want := range [][]string{
		{"level=INFO", `msg="reading the router" component=watch`},
		{"level=INFO", `msg="usage read" component=watch`, "source=router", "read=work,personal,side"},
		{"level=DEBUG", `msg="usage read" component=watch`, "source=router"},
		{"level=WARN", `msg="the router stopped answering; showing its last document" component=watch`},
		{"level=WARN", `msg="the router stopped answering; probing directly" component=watch`, `router="not running"`},
		{"level=INFO", `msg="usage read" component=watch`, "source=probe"},
		{"level=INFO", "msg=pinned component=watch", "accounts=work", "move=false"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if n := strings.Count(log.String(), `msg="reading the router"`); n != 2 {
		t.Errorf("log reads\n%s\nwant the router read twice: at the start, and once it came back", log)
	}
}

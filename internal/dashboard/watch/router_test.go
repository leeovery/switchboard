package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

// routerKeys is what the footer says the keys do while the dashboard reads
// the router of three accounts.
const routerKeys = "r refresh · 1–3 pin · a auto · m move · q quit"

func TestLooksAtTheRoutersDocumentEveryFiveSeconds(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	if got, want := h.source.asked, []Read{{Refresh: interval, Probe: true}}; !slices.Equal(got, want) {
		t.Fatalf("starting asked for %+v, want %+v: the router refreshing what it hasn't read in an interval", got, want)
	}
	for i := range 3 {
		tick := h.lastTick()
		if tick.delay != lookEvery+tickSlack {
			t.Errorf("look %d: the tick came %v after the last read, want %v", i+1, tick.delay, lookEvery+tickSlack)
		}
		h.fire(tick)
	}
	want := []Read{{Refresh: interval, Probe: true}, {Probe: true}, {Probe: true}, {Probe: true}}
	if !slices.Equal(h.source.asked, want) {
		t.Errorf("asked for %+v, want %+v: a look at the router's document at each tick", h.source.asked, want)
	}
	if h.source.reads != 0 {
		t.Errorf("probed %d times, want never while the router answers", h.source.reads)
	}
	if got, want := h.footer(), "updated 13:12 · "+routerKeys; got != want {
		t.Errorf("footer = %q, want %q", got, want)
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
		if slices.ContainsFunc(h.source.asked[asked:], func(r Read) bool { return r.Refresh > 0 }) {
			refreshed = append(refreshed, h.clock.now)
		}
	}
	for i, want := range []time.Time{at(13, 14, 0), at(13, 18, 0)} {
		if i >= len(refreshed) || refreshed[i].Before(want) || refreshed[i].After(want.Add(lookEvery+tickSlack)) {
			t.Errorf("the router refreshed at %v, want refresh %d at the look after %s, backing off", refreshed, i+1, want.Format(time.Kitchen))
		}
	}
}

func TestFallsBackToProbingWhenTheRouterStops(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.stopRouter()
	h.fire(h.lastTick())
	if h.source.reads != 1 {
		t.Fatalf("once the router stopped, probed %d times, want once", h.source.reads)
	}
	if got, want := h.footer(), "no router since 13:12 · updated 13:12 · next 13:42 · r refresh · q quit"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if !strings.Contains(h.view(), "probing directly (router not running)") {
		t.Errorf("the screen is\n%s\nwant it to say the accounts are probed, as the router isn't running", h.view())
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
	h.fire(h.lastTick()) // Probing from 13:12:05.

	h.tickUntil(at(13, 13, 30))
	h.startRouter(routerDocument(three()...))
	h.fire(h.lastTick()) // 13:14.
	if got, want := h.footer(), "updated 13:14 · "+routerKeys; got != want {
		t.Errorf("once the router answers again, footer = %q, want %q", got, want)
	}
	if got, want := h.source.asked[len(h.source.asked)-1], (Read{Refresh: interval}); got != want {
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
	if got := countReads(h.source.asked, Read{Refresh: interval}); got != 4 {
		t.Errorf("asked after the router %d times by 13:16:30, want 4: once at each minute", got)
	}
}

func TestAsksAfterTheRouterWhenAProbeIsDue(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	h.tickUntil(at(13, 41, 30))
	h.startRouter(routerDocument(three()...))
	h.fire(h.lastTick()) // 13:42, when the probe is due.
	if got, want := h.source.asked[len(h.source.asked)-1], (Read{Refresh: interval, Probe: true}); got != want {
		t.Errorf("the read due at 13:42 asked for %+v, want %+v", got, want)
	}
	if !h.model.routed() || h.source.reads != 1 {
		t.Errorf("after the read due at 13:42, routed = %v and probes = %d, want the router read, and no probe", h.model.routed(), h.source.reads)
	}
}

func TestALookIsQuiet(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	tick := h.lastTick()
	h.clock.now, tick.fired = tick.due, true
	looking := h.update(tick.msg)
	if len(looking) == 0 {
		t.Fatal("the tick didn't look at the router's document")
	}
	if got, want := h.footer(), "updated 13:12 · "+routerKeys; got != want {
		t.Errorf("while looking at the router's document, footer = %q, want %q", got, want)
	}
	h.deliver(looking...)
}

func TestRefreshKeyReadingTheRouter(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	pending := h.press("r")
	if got, want := h.source.asked[len(h.source.asked)-1], (Read{Refresh: freshFor, Probe: true}); got != want {
		t.Errorf("r asked for %+v, want %+v: the router refreshing what it hasn't read in the last minute", got, want)
	}
	if got, want := h.footer(), "refreshing… · "+routerKeys; got != want {
		t.Errorf("while refreshing, footer = %q, want %q", got, want)
	}
	h.clock.now = at(13, 20, 0)
	h.deliver(pending...)
	if got, want := h.footer(), "updated 13:20 · "+routerKeys; got != want {
		t.Errorf("after refreshing, footer = %q, want %q", got, want)
	}
}

func TestKeysTellTheRouterWhereToSendSessions(t *testing.T) {
	pinned := routerDocument(three()...)
	pinned.Pin = status.Pin{Account: "side", Since: start.UTC()}
	tests := []struct {
		name       string
		doc        status.Document
		key        string
		wantOrders []string
		wantNote   string
		wantPin    string
	}{
		{
			name:       "1 pins new sessions to the first account",
			doc:        routerDocument(three()...),
			key:        "1",
			wantOrders: []string{"pin work"},
			wantNote:   "new sessions go to work · Work",
			wantPin:    "work",
		},
		{
			name:       "3 to the third",
			doc:        pinned,
			key:        "3",
			wantOrders: []string{"pin side"},
			wantNote:   "new sessions go to side · Side",
			wantPin:    "side",
		},
		{
			name:       "a routes every session automatically again",
			doc:        pinned,
			key:        "a",
			wantOrders: []string{"unpin"},
			wantNote:   "routing automatically",
		},
		{
			name:       "m moves running sessions to the account pinned",
			doc:        pinned,
			key:        "m",
			wantOrders: []string{"move side"},
			wantNote:   "running sessions move to side · Side",
			wantPin:    "side",
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
			if got, want := h.footer(), tt.wantNote+" · "+routerKeys; got != want {
				t.Errorf("footer = %q, want %q", got, want)
			}
			if got := h.source.looks(); got != looks+1 {
				t.Errorf("looked at the router's document %d times after the key, want once", got-looks)
			}
			if got := h.model.doc.Pin.Account; got != tt.wantPin {
				t.Errorf("the document on screen is pinned to %q, want %q", got, tt.wantPin)
			}
		})
	}
}

func TestMoveWithoutAPinSaysSo(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	asked := len(h.source.asked)

	h.deliver(h.press("m")...)
	if len(h.source.orders) > 0 || len(h.source.asked) > asked {
		t.Errorf("orders = %q, and read %d times, want neither", h.source.orders, len(h.source.asked)-asked)
	}
	if got, want := h.footer(), "nothing's pinned to move sessions to: pin an account with 1–3 · "+routerKeys; got != want {
		t.Errorf("footer = %q, want %q", got, want)
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
			if got, want := h.footer(), "updated 13:12 · "+routerKeys; got != want {
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
	if got, want := h.footer(), "updated 13:12 · next 13:42 · r refresh · q quit"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
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
	if got, want := h.footer(), "new sessions go to personal · Personal · "+routerKeys; got != want {
		t.Errorf("just before it lapses, footer = %q, want %q", got, want)
	}
	h.fire(lapse)
	if got, want := h.footer(), "updated 13:12 · "+routerKeys; got != want {
		t.Errorf("once it lapses, footer = %q, want %q", got, want)
	}
}

func TestAnOrderTheRouterRefusesSaysWhy(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.source.refuse = errors.New("account personal has no token, so nothing can go out on it: set CLAUDE_TOKEN_PERSONAL")
	h.start()

	h.deliver(h.press("2")...)
	if got, want := h.footer(), h.source.refuse.Error()+" · "+routerKeys; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	want := []string{"level=WARN", `msg="the router didn't take an order"`, "account=personal", "move=false", "error="}
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
	if got, want := h.source.asked, []Read{{Refresh: interval, Probe: true}, {Refresh: freshFor, Probe: true}}; !slices.Equal(got, want) {
		t.Fatalf("while r's read is under way, asked for %+v, want %+v", got, want)
	}
	h.deliver(reading...)
	want := []Read{{Refresh: interval, Probe: true}, {Refresh: freshFor, Probe: true}, {Probe: true}}
	if !slices.Equal(h.source.asked, want) {
		t.Errorf("once it lands, asked for %+v, want %+v", h.source.asked, want)
	}
	if got := h.model.doc.Pin.Account; got != "work" {
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
		{name: "the router of one", doc: routerDocument(three()[0]), want: "r refresh · 1 pin · a auto · m move · q quit"},
		{name: "the router of twelve", doc: routerDocument(many...), want: "r refresh · 1–9 pin · a auto · m move · q quit"},
		{name: "a probe", doc: probedWithoutTheRouter(), want: "r refresh · q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.doc)
			if routed(tt.doc) {
				h.startRouter(tt.doc)
			}
			h.start()

			if got := h.footer(); !strings.HasSuffix(got, " · "+tt.want) {
				t.Errorf("footer = %q, want it to end %q", got, tt.want)
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
	h.fire(h.lastTick())
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

func TestLogsTheRouterGoingAndComingBack(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.fire(h.lastTick())
	h.stopRouter()
	h.fire(h.lastTick())
	h.startRouter(routerDocument(three()...))
	h.tickUntil(at(13, 13, 30))
	h.deliver(h.press("1")...)

	for _, want := range [][]string{
		{"level=INFO", `msg="reading the router" component=watch`},
		{"level=INFO", `msg="usage read" component=watch`, "source=router", "read=work,personal,side"},
		{"level=DEBUG", `msg="usage read" component=watch`, "source=router"},
		{"level=WARN", `msg="the router stopped answering; probing directly" component=watch`, `router="not running"`},
		{"level=INFO", `msg="usage read" component=watch`, "source=probe"},
		{"level=INFO", "msg=pinned component=watch", "account=work", "move=false"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if n := strings.Count(log.String(), `msg="reading the router"`); n != 2 {
		t.Errorf("log reads\n%s\nwant the router read twice: at the start, and once it came back", log)
	}
}

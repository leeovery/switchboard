package router_test

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	haiku = "claude-haiku-4-5-20251001"
	fable = "claude-fable-5-1"
	// sonnet's thinking is bound to the account that produced it.
	sonnet = "claude-sonnet-5-5"
)

func TestNewSessionsLandByPerishability(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// Side's week resets tomorrow, work's in five days: side's quota needs
	// using first, until serving a session uses most of it.
	sideWeek, workWeek := weekOf(0.5, 24*time.Hour), weekOf(0.5, 5*24*time.Hour)
	r.readsAs(workToken, session, workWeek)
	r.readsAs(sideToken, session, sideWeek)
	r.api.set(sideToken, session, weekOf(0.99, 24*time.Hour))

	if got := r.ask(t, "first", opus, ""); got != "side" {
		t.Errorf("the first session went to %s, want side, whose quota needed using first", got)
	}
	if got := r.ask(t, "second", opus, ""); got != "work" {
		t.Errorf("the second session went to %s, want work, now side's week is nearly spent", got)
	}
	waitForLine(t, log, "msg=routed", "session=first", "account=side", "reason=new")
	waitForLine(t, log, "msg=routed", "session=second", "account=work", "reason=new")
}

func TestASessionStaysOnItsAccount(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// Side's week is all but spent, so the session goes to work.
	r.readsAs(workToken, session, weekOf(0.5, 5*24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.99, 24*time.Hour))
	if got := r.ask(t, "one", opus, ""); got != "work" {
		t.Fatalf("the session went to %s, want work", got)
	}
	// Side's week starts afresh, which a request pinned there reads, and its
	// quota needs using before work's.
	r.api.set(sideToken, session, weekOf(0, 7*24*time.Hour))
	r.ask(t, "pinned", opus, "side")

	for range 3 {
		if got := r.ask(t, "one", opus, ""); got != "work" {
			t.Errorf("the session's next request went to %s, want work, where its cache is warm", got)
		}
	}
	waitForLine(t, log, "msg=routed", "session=one", "account=work", "reason=sticky")
	if got := r.ask(t, "another", opus, ""); got != "side" {
		t.Errorf("a new session went to %s, want side, whose quota now needs using first", got)
	}
}

func TestASessionIdlePastTheHourIsRescored(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 5*24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.99, 24*time.Hour))
	if got := r.ask(t, "one", opus, ""); got != "work" {
		t.Fatalf("the session went to %s, want work", got)
	}
	// While the session idles, side's week starts afresh, which probes read.
	r.readsAs(sideToken, session, weekOf(0, 7*24*time.Hour))
	r.clock.advance(time.Hour + time.Minute)

	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Errorf("after an hour and a minute idle, the session went to %s, want side, whose quota needs using first", got)
	}
	waitForLine(t, log, "level=INFO", "msg=moved", "session=one", "from=work", "to=side", `reason="rescored after 1h 1m idle"`)
	waitForLine(t, log, "msg=routed", "session=one", "account=side", `reason="rescored after 1h 1m idle"`)
	want := []router.Event{router.Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "rescored after 1h 1m idle"}}
	if got := slices.DeleteFunc(r.events.heard(), isStart); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v: work could still take the request", got, want)
	}
}

func TestASessionWhoseThinkingIsBoundStaysOnItsAccountUntilItMustMove(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// Side's week is all but spent, so the session's requests of both models
	// go to work.
	r.readsAs(workToken, session, weekOf(0.5, 5*24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.99, 24*time.Hour))
	for _, model := range []string{sonnet, opus} {
		if got := r.ask(t, "one", model, ""); got != "work" {
			t.Fatalf("the session's first %s request went to %s, want work", model, got)
		}
	}
	// While the session idles, side's week starts afresh, which probes read.
	r.readsAs(sideToken, session, weekOf(0, 7*24*time.Hour))
	r.clock.advance(2 * time.Hour)

	if got := r.ask(t, "one", sonnet, ""); got != "work" {
		t.Errorf("two hours idle, the session's Sonnet request went to %s, want work, which its thinking is bound to", got)
	}
	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Errorf("two hours idle, the session's Opus request went to %s, want side, whose quota needs using first", got)
	}
	r.api.script(workToken, limitReached("You've hit your limit", sessionSpent, weekOf(0.5, 5*24*time.Hour)))
	if got := r.ask(t, "one", sonnet, ""); got != "side" {
		t.Errorf("at work's limit, the session's Sonnet request went to %s, want side", got)
	}

	waitForLine(t, log, "msg=routed", "session=one", "model="+sonnet, "account=work", "reason=bound")
	waitForLine(t, log, "msg=routed", "session=one", "model="+opus, "account=side", `reason="rescored after 2h idle"`)
	waitForLine(t, log, "msg=routed", "session=one", "model="+sonnet, "account=side", `reason="moved: work hit its limit"`, "attempts=2")
	want := []router.Event{
		router.Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "rescored after 2h idle"},
		router.LimitReached{Account: "work", Windows: []string{"5h"}, Until: sessionSpent.ResetsAt, Limit: 1},
		router.Moved{Session: "one", Model: sonnet, From: "work", To: "side", Reason: "moved: work hit its limit", Held: router.HeldByLimit, Limit: 1},
	}
	if got := slices.DeleteFunc(r.events.heard(), isStart); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestTheSessionPinHeader(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))

	if got := r.ask(t, "one", opus, "side"); got != "side" {
		t.Errorf("the pinned session went to %s, want side, its pin, over work", got)
	}
	waitForLine(t, log, "msg=routed", "session=one", "account=side", "reason=pinned")

	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	r.api.set(sideToken, spent, weekOf(0.5, 5*24*time.Hour))
	r.ask(t, "one", opus, "side")
	if got := r.ask(t, "one", opus, "side"); got != "work" {
		t.Errorf("with side spent, the pinned session went to %s, want work", got)
	}
	waitForLine(t, log, "msg=routed", "session=one", "account=work", `reason="pin yields: side has no room"`)
	want := []router.Event{router.Moved{Session: "one", Model: opus, From: "side", To: "work", Reason: "pin yields: side has no room"}}
	if got := slices.DeleteFunc(r.events.heard(), isStart); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestTheGlobalPin(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	client := router.NewClient(serveControl(t, r.rt))
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	if got := r.ask(t, "running", opus, ""); got != "work" {
		t.Fatalf("the session went to %s, want work", got)
	}
	r.clock.advance(time.Minute)

	doc, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"side"}})
	if want := (status.Pin{Accounts: []string{"side"}, Since: r.clock.read()}); err != nil || !reflect.DeepEqual(doc.Pin, want) {
		t.Fatalf("Pin() = pin %+v, %v, want %+v", doc.Pin, err, want)
	}
	if got := r.ask(t, "new", opus, ""); got != "side" {
		t.Errorf("a new session went to %s, want side, the pin's", got)
	}
	waitForLine(t, log, "msg=routed", "session=new", "account=side", `reason="pinned (global)"`)
	if got := r.ask(t, "running", opus, ""); got != "work" {
		t.Errorf("the running session went to %s, want work, where it was", got)
	}

	r.clock.advance(time.Minute)
	if _, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"side"}, Move: true}); err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if got := r.ask(t, "running", opus, ""); got != "side" {
		t.Errorf("once the pin moves sessions, the running session went to %s, want side", got)
	}
	waitForLine(t, log, "level=INFO", "msg=moved", "session=running", "from=work", "to=side", `reason="moved by pin"`)
	want := []router.Event{
		router.Pinned{Accounts: []string{"side"}, Account: "side"},
		router.Pinned{Accounts: []string{"side"}, Account: "side", Move: true},
		router.Moved{Session: "running", Model: opus, From: "work", To: "side", Reason: "moved by pin"},
	}
	if got := slices.DeleteFunc(r.events.heard(), isStart); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v: work could still take the request", got, want)
	}
	if got := r.ask(t, "running", opus, ""); got != "side" {
		t.Errorf("the moved session's next request went to %s, want side", got)
	}
	waitForLine(t, log, "msg=routed", "session=running", "account=side", "reason=sticky")

	doc = r.rt.Status()
	if want := (status.Pin{Accounts: []string{"side"}, Since: r.clock.read(), Move: true}); !reflect.DeepEqual(doc.Pin, want) {
		t.Errorf("the document's pin = %+v, want %+v", doc.Pin, want)
	}
	for id, want := range map[string]int{"work": 0, "side": 2} {
		if account, _ := doc.Account(id); account.Sessions != want {
			t.Errorf("the document gives %s %d sessions, want %d", id, account.Sessions, want)
		}
	}

	if doc, err := client.Unpin(t.Context(), false, ""); err != nil || !doc.Pin.IsZero() {
		t.Fatalf("Unpin() = pin %+v, %v, want none", doc.Pin, err)
	}
	if got := r.ask(t, "after", opus, ""); got != "work" {
		t.Errorf("once unpinned, a new session went to %s, want work, whose quota needs using first", got)
	}
	for _, want := range [][]string{
		{"level=INFO", "msg=pinned", "accounts=side", "move=false"},
		{"level=INFO", "msg=pinned", "accounts=side", "move=true"},
		{"level=INFO", "msg=unpinned", "accounts=side"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
}

func TestTheGlobalPinOnSeveralAccounts(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t, withPersonalToken)
	client := router.NewClient(serveControl(t, r.rt))
	// Work's quota needs using first, then side's, then personal's.
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 2*24*time.Hour))
	r.readsAs(personalToken, session, weekOf(0.5, 5*24*time.Hour))
	if got := r.ask(t, "on work", opus, ""); got != "work" {
		t.Fatalf("the first session went to %s, want work", got)
	}
	pinning(t, client, router.PinRequest{Accounts: []string{"personal"}})
	if got := r.ask(t, "on personal", opus, ""); got != "personal" {
		t.Fatalf("the second session went to %s, want personal, as pinned", got)
	}
	r.clock.advance(time.Minute)

	if _, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"side", "personal"}, Move: true}); err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if got := r.ask(t, "new", opus, ""); got != "side" {
		t.Errorf("a new session went to %s, want side, the best of those pinned", got)
	}
	if got := r.ask(t, "on work", opus, ""); got != "side" {
		t.Errorf("the session on work went to %s, want side: the pin moves it to the best of those pinned", got)
	}
	if got := r.ask(t, "on personal", opus, ""); got != "personal" {
		t.Errorf("the session on personal went to %s, want personal: the pin names it", got)
	}
	waitForLine(t, log, "level=INFO", "msg=pinned", "accounts=personal,side", "move=true")
	waitForLine(t, log, "msg=routed", "session=new", "account=side", `reason="pinned (global)"`)
	waitForLine(t, log, "level=INFO", "msg=moved", `session="on work"`, "from=work", "to=side", `reason="moved by pin"`)
}

func TestAPinGivenWhileASessionRunsMovesItOnItsNextRequest(t *testing.T) {
	tests := []struct {
		name  string
		model string
		// launched is the pin the session was launched with.
		launched string
	}{
		{name: "passing over the pin it was launched with", model: opus, launched: "work"},
		{name: "launched without a pin", model: opus},
		{name: "its thinking bound to its account, as the move is the user's", model: sonnet},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r := newRouted(t)
			client := router.NewClient(serveControl(t, r.rt))
			// Work's quota needs using first.
			r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
			r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
			if got := r.ask(t, "one", tt.model, tt.launched); got != "work" {
				t.Fatalf("the session went to %s, want work", got)
			}

			got, err := client.PinSession(t.Context(), "one", "side", "")
			if err != nil || got.Pin != "side" {
				t.Fatalf("PinSession() = %+v, %v, want the session, pinned to side", got, err)
			}
			for range 2 {
				if got := r.ask(t, "one", tt.model, tt.launched); got != "side" {
					t.Errorf("the session's next request went to %s, want side, the pin it was given", got)
				}
			}
			waitForLine(t, log, "level=INFO", `msg="pinned session"`, "session=one", "account=side")
			waitForLine(t, log, "msg=routed", "session=one", "account=side", "reason=pinned")
			want := []router.Event{
				router.Pinned{Account: "side", Session: "one"},
				router.Moved{Session: "one", Model: tt.model, From: "work", To: "side", Reason: "pinned"},
			}
			if got := slices.DeleteFunc(r.events.heard(), isStart); !reflect.DeepEqual(got, want) {
				t.Errorf("events = %+v, want %+v", got, want)
			}
		})
	}
}

func TestASessionsPinClearedWhileItRunsPassesOverTheOneItWasLaunchedWith(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	client := router.NewClient(serveControl(t, r.rt))
	// Side's quota needs using first, but the session is launched pinned to
	// work. Nothing is read of side until the session is rescored, when its
	// session, a little used, is on no pace to run out.
	r.readsAs(workToken, session, weekOf(0.5, 5*24*time.Hour))
	light := session
	light.Utilization = 0.05
	r.readsAs(sideToken, light, weekOf(0.5, 24*time.Hour))
	if got := r.ask(t, "one", opus, "work"); got != "work" {
		t.Fatalf("the session went to %s, want work, its pin", got)
	}

	got, err := client.UnpinSession(t.Context(), "one", "")
	if err != nil || got.Pin != "" {
		t.Fatalf("UnpinSession() = %+v, %v, want the session, without a pin", got, err)
	}
	if got := r.ask(t, "one", opus, "work"); got != "work" {
		t.Errorf("the session's next request went to %s, want work, where its cache is warm", got)
	}
	waitForLine(t, log, "msg=routed", "session=one", "account=work", "reason=sticky")
	r.clock.advance(time.Hour + time.Minute)
	if got := r.ask(t, "one", opus, "work"); got != "side" {
		t.Errorf("idle past the hour, the session went to %s, want side, routed like any other: its launch pin is passed over", got)
	}
	waitForLine(t, log, "msg=routed", "session=one", "account=side", `reason="rescored after 1h 1m idle"`)
	waitForLine(t, log, "level=INFO", `msg="unpinned session"`, "session=one")
}

func TestForceClearsEverySessionsOwnPinLaunchPinsIncluded(t *testing.T) {
	tests := []struct {
		name  string
		force func(t *testing.T, client *router.Client) status.Document
		// idle is how long the sessions idle once forced.
		idle time.Duration
		// wantPin is the global pin once forced, and wantReason why the
		// sessions go to work on their next requests.
		wantPin    status.Pin
		wantReason string
	}{
		{
			name: "pinning, moving every session",
			force: func(t *testing.T, client *router.Client) status.Document {
				return pinning(t, client, router.PinRequest{Accounts: []string{"work"}, Move: true, Force: true})
			},
			wantPin:    status.Pin{Accounts: []string{"work"}, Since: now.Add(time.Minute), Move: true},
			wantReason: `reason="moved by pin"`,
		},
		{
			name: "pinning, running sessions staying while their caches are warm",
			force: func(t *testing.T, client *router.Client) status.Document {
				return pinning(t, client, router.PinRequest{Accounts: []string{"work"}, Force: true})
			},
			idle:       2 * time.Hour,
			wantPin:    status.Pin{Accounts: []string{"work"}, Since: now.Add(time.Minute)},
			wantReason: `reason="pinned (global)"`,
		},
		{
			name: "unpinning",
			force: func(t *testing.T, client *router.Client) status.Document {
				pinning(t, client, router.PinRequest{Accounts: []string{"work"}})
				doc, err := client.Unpin(t.Context(), true, "")
				if err != nil {
					t.Fatalf("Unpin() error = %v", err)
				}
				return doc
			},
			idle:       2 * time.Hour,
			wantReason: `reason="rescored after 2h 1m idle"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r := newRouted(t)
			client := router.NewClient(serveControl(t, r.rt))
			// Work's quota needs using first, but both sessions are pinned to
			// side: one as it's launched, the other while it runs.
			r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
			r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
			r.ask(t, "launched", opus, "side")
			r.ask(t, "given", opus, "")
			if _, err := client.PinSession(t.Context(), "given", "side", ""); err != nil {
				t.Fatalf("PinSession() error = %v", err)
			}
			if got := r.ask(t, "given", opus, ""); got != "side" {
				t.Fatalf("the session given a pin went to %s, want side", got)
			}
			r.clock.advance(time.Minute)

			if doc := tt.force(t, client); !reflect.DeepEqual(doc.Pin, tt.wantPin) {
				t.Errorf("once forced, the global pin = %+v, want %+v", doc.Pin, tt.wantPin)
			}
			r.clock.advance(tt.idle)
			for id, launched := range map[string]string{"launched": "side", "given": ""} {
				if got := r.ask(t, id, opus, launched); got != "work" {
					t.Errorf("once forced, session %s went to %s, want work: no session keeps a pin of its own", id, got)
				}
				waitForLine(t, log, "msg=routed", "session="+id, "account=work", tt.wantReason)
			}
			if got := r.ask(t, "later", opus, "side"); got != "side" {
				t.Errorf("a session launched afterwards, pinned to side, went to %s, want side, its pin", got)
			}
			waitForLine(t, log, "level=INFO", "force=true", "sessions_unpinned=2")
		})
	}
}

// pinning has the router pin as p asks, and returns the status document as
// that leaves it.
func pinning(t *testing.T, client *router.Client, p router.PinRequest) status.Document {
	t.Helper()
	doc, err := client.Pin(t.Context(), p)
	if err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	return doc
}

func TestSessionsCountOnceAnHourAnAccount(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.ask(t, "one", opus, "")
	r.ask(t, "one", haiku, "")
	r.ask(t, "two", opus, "side")
	r.ask(t, "old", opus, "side")
	r.clock.advance(59 * time.Minute)
	r.ask(t, "two", opus, "side")

	for id, want := range map[string]int{"work": 1, "side": 2} {
		if account, _ := r.rt.Status().Account(id); account.Sessions != want {
			t.Errorf("%s has %d sessions, want %d", id, account.Sessions, want)
		}
	}
	r.clock.advance(2 * time.Minute)
	for id, want := range map[string]int{"work": 0, "side": 1} {
		if account, _ := r.rt.Status().Account(id); account.Sessions != want {
			t.Errorf("an hour on, %s has %d sessions, want %d: those used in the last hour", id, account.Sessions, want)
		}
	}
}

func TestAFableWeekLearntFromProbesHoldsBackFableRequestsAlone(t *testing.T) {
	r := newRouted(t)
	fableSpent := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 1, ResetsAt: now.Add(48 * time.Hour), Status: quota.StatusRejected}
	models := map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}}
	r.prober.answer(workToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, weekOf(0.5, 5*24*time.Hour), fableWeek}}, models: models})
	r.prober.answer(sideToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, weekOf(0.5, 24*time.Hour), fableSpent}}, models: models})

	if got := r.ask(t, "writing", fable, ""); got != "work" {
		t.Errorf("a Fable session went to %s, want work: side's Fable week is spent", got)
	}
	if got := r.ask(t, "titling", haiku, ""); got != "side" {
		t.Errorf("a Haiku session went to %s, want side, whose quota needs using first: its Fable week doesn't count Haiku", got)
	}
}

func TestAFableWeekLearntFromResponsesHoldsBackFableRequestsAlone(t *testing.T) {
	r := newRouted(t)
	fableSpent := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 1, ResetsAt: fableWeek.ResetsAt, Status: quota.StatusRejected}
	r.readsAs(workToken, session, weekOf(0.5, 5*24*time.Hour), fableWeek)
	r.readsAs(sideToken, session, weekOf(0.5, 24*time.Hour), fableWeek)
	r.api.set(sideToken, session, weekOf(0.5, 24*time.Hour), fableSpent)

	if got := r.ask(t, "first", fable, ""); got != "side" {
		t.Fatalf("the first Fable session went to %s, want side, whose quota needs using first", got)
	}
	if got := r.ask(t, "titling", haiku, ""); got != "side" {
		t.Errorf("a Haiku session went to %s, want side: its spent Fable week has been seen only on Fable", got)
	}
	if got := r.ask(t, "second", fable, ""); got != "work" {
		t.Errorf("the second Fable session went to %s, want work: side's Fable week is spent", got)
	}
}

func TestARequestWithoutASessionIsChosenAlone(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 5*24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 24*time.Hour))

	if got := r.ask(t, "", opus, ""); got != "side" {
		t.Errorf("a request without a session went to %s, want side", got)
	}
	waitForLine(t, log, "msg=routed", "account=side", "reason=unsessioned")
	if doc := r.rt.Status(); doc.Accounts[2].Sessions != 0 {
		t.Errorf("side has %d sessions, want none: a request without a session isn't remembered", doc.Accounts[2].Sessions)
	}
}

// routed is a router in front of an accountsAPI, on a clock the test moves,
// probing with a prober the test sets, and telling its events to a log.
type routed struct {
	api    *accountsAPI
	prober *fakeProber
	clock  *fakeClock
	events *eventLog
	rt     *router.Router
	proxy  string
}

// newRouted builds a routed router from testConfig, as each of configure
// changes it.
func newRouted(t *testing.T, configure ...func(*router.Config)) *routed {
	t.Helper()
	api := newAccountsAPI(t)
	r := &routed{api: api, prober: &fakeProber{}, clock: newFakeClock(now), events: &eventLog{}}
	cfg := testConfig(api.URL)
	cfg.Now, cfg.Prober, cfg.Events = r.clock.read, r.prober, r.events.hear
	for _, c := range configure {
		c(&cfg)
	}
	r.rt = newRouterFrom(t, cfg)
	r.proxy = serveProxy(t, r.rt)
	return r
}

// readsAs has the account whose token is token read as windows by probes,
// and by the API's answers, from now on.
func (r *routed) readsAs(token string, windows ...quota.Window) {
	r.prober.answer(token, probeResult{usage: quota.Usage{Windows: windows}})
	r.api.set(token, windows...)
}

// ask sends a messages request of the session given for model, pinned to pin
// unless it's empty, and returns the account it went out on.
func (r *routed) ask(t *testing.T, session, model, pin string) string {
	t.Helper()
	header := with(with(claudeCode(workToken), claude.SessionHeader, session), "X-Switchboard-Account", pin)
	body := `{"model":"` + model + `","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
	readAll(t, send(t, http.MethodPost, r.proxy+"/v1/messages", header, strings.NewReader(body)))
	return r.api.lastAccount()
}

// weekOf is a week used as given, which resets after left.
func weekOf(used float64, left time.Duration) quota.Window {
	return quota.Window{Key: "7d", Label: "Week", Utilization: used, ResetsAt: now.Add(left), Status: quota.StatusAllowed}
}

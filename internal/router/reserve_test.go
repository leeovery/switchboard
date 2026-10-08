package router_test

import (
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
)

// workPrimary makes work the primary, keeping a tenth of every window back as
// its reserve.
func workPrimary(cfg *router.Config) {
	cfg.Accounts[0].Primary, cfg.Accounts[0].Reserve = true, 0.1
}

// sessionAt is the session window, used as given.
func sessionAt(used float64) quota.Window {
	w := session
	w.Utilization = used
	return w
}

func TestASessionMovesOffAnAccountAtItsReserve(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t, workPrimary)
	// Work's quota needs using first.
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	if got := r.ask(t, "one", opus, ""); got != "work" {
		t.Fatalf("the session went to %s, want work", got)
	}
	// Work's next answer reads its session past its reserve.
	r.api.set(workToken, sessionAt(0.91), weekOf(0.5, 24*time.Hour))
	r.ask(t, "one", opus, "")

	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Errorf("with work at its reserve, the session went to %s, want side", got)
	}
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("a new session went to %s, want side: work is at its reserve", got)
	}
	waitForLine(t, log, "level=INFO", `msg="held back by its reserve"`, "account=work", "windows=5h", "reserve=0.1")
	waitForLine(t, log, "level=INFO", "msg=moved", "session=one", "from=work", "to=side", `reason="moved: work is at its reserve"`)
	waitForLine(t, log, "msg=routed", "session=one", "account=side", `reason="moved: work is at its reserve"`)
	want := []router.Event{router.Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work is at its reserve", Held: router.HeldByReserve}}
	if got := slices.DeleteFunc(r.events.heard(), isStart); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
	doc := r.rt.Status()
	work, _ := doc.Account("work")
	if doc.Primary != "work" || !work.Primary || work.Reserve != 0.1 || !slices.Equal(work.AtReserve, []string{"5h"}) || doc.Best != "side" {
		t.Errorf("the document gives primary %q, work as %+v, and best %q; want work the primary, its session at its reserve, and side the best",
			doc.Primary, work, doc.Best)
	}

	// Once work's session resets, its reserve lets it go, and its quota needs
	// using first again.
	r.clock.advance(session.ResetsAt.Sub(now))
	if got := r.ask(t, "three", opus, ""); got != "work" {
		t.Errorf("once work's session reset, a new session went to %s, want work", got)
	}
	waitForLine(t, log, "level=INFO", `msg="let go at its reset"`, "account=work", "windows=5h")
	if n := strings.Count(log.String(), `msg="held back by its reserve"`); n != 1 {
		t.Errorf("log reads\n%s\nwant work held back by its reserve noted once, not %d times", log, n)
	}
}

func TestAPinSpendsItsAccountsReserve(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t, workPrimary)
	client := router.NewClient(serveControl(t, r.rt))
	// Work's quota would need using first, but its session is at its reserve.
	r.readsAs(workToken, sessionAt(0.95), weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the session went to %s, want side: work is at its reserve", got)
	}
	if got := r.ask(t, "own", opus, "work"); got != "work" {
		t.Errorf("a session pinned to work went to %s, want work: its own pin spends the reserve", got)
	}

	r.clock.advance(time.Minute)
	if _, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"work"}, Move: true}); err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	for _, reason := range []string{`reason="moved by pin"`, "reason=sticky"} {
		if got := r.ask(t, "one", opus, ""); got != "work" {
			t.Errorf("under a pin to work that moves sessions, the session went to %s, want work (%s)", got, reason)
		}
		waitForLine(t, log, "msg=routed", "session=one", "account=work", reason)
	}
	if got := r.ask(t, "two", opus, ""); got != "work" {
		t.Errorf("under a pin to work, a new session went to %s, want work", got)
	}
	doc := r.rt.Status()
	if work, _ := doc.Account("work"); doc.Reserved(work) != "spending its reserve (pinned)" {
		t.Errorf("the document says of work's reserve %q, want it spent by the pin", doc.Reserved(work))
	}

	if _, err := client.Unpin(t.Context(), false, ""); err != nil {
		t.Fatalf("Unpin() error = %v", err)
	}
	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Errorf("once unpinned, the session went to %s, want side, work being at its reserve", got)
	}
	waitForLine(t, log, "msg=routed", "session=two", "account=work", `reason="pinned (global)"`)
	waitForLine(t, log, "msg=routed", "session=one", "account=side", `reason="moved: work is at its reserve"`)
}

func TestWithNoRoomButInTheReservesTheRouterAnswers429Itself(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t, workPrimary, func(cfg *router.Config) { cfg.Accounts[2].Reserve = 0.1 })
	// Both accounts are at their reserves: work in its session, which resets
	// first, and side in its week.
	r.readsAs(workToken, sessionAt(0.95), weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.93, 30*time.Hour))

	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("answered %d %s, want a 429", resp.StatusCode, body)
	}
	checkAPIError(t, body, "rate_limit_error", "switchboard: no account has room outside its reserve")
	wantHeaders := map[string]string{
		"Anthropic-Ratelimit-Unified-Status": "rejected",
		"Anthropic-Ratelimit-Unified-Reset":  strconv.FormatInt(session.ResetsAt.Unix(), 10),
		"Content-Type":                       "application/json",
	}
	for name, want := range wantHeaders {
		if got := resp.Header.Get(name); got != want {
			t.Errorf("answered with %s %q, want %q", name, got, want)
		}
	}
	if outcome := (claude.Provider{}).Classify(resp.StatusCode, resp.Header); outcome.Verdict != quota.LimitReached || !outcome.LimitedUntil.Equal(session.ResetsAt) {
		t.Errorf("Claude Code reads the answer as %+v, want a limit until work's session resets", outcome)
	}
	if n := r.api.count(); n != 0 {
		t.Errorf("the upstream got %d requests, want none: an account at its reserve takes none of the router's own choosing", n)
	}
	waitForLine(t, log, "level=WARN", `msg="no account has room outside its reserve; answering 429"`, "until=")
	waitForLine(t, log, "level=INFO", "msg=routed", `account=""`, `reason="no account has room"`, "status=429")
	if h := r.rt.Status().Router; h.Requests != 1 || h.Failures != 0 {
		t.Errorf("the router's health counts %d requests and %d failures, want the one answered, not failed", h.Requests, h.Failures)
	}
	if n := r.rt.Status().Sessions; n != 0 {
		t.Errorf("the router has %d sessions, want none: no account answered the session's request", n)
	}

	if got := r.ask(t, "pinned", opus, "side"); got != "side" {
		t.Errorf("a session pinned to side went to %s, want side: its pin spends the reserve", got)
	}
}

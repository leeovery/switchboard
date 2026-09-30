package router_test

import (
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestAnOverallRejectionBarsItsAccountUntilItsReset(t *testing.T) {
	r := newRouted(t)
	// Work's quota needs using first, and its windows read as having room,
	// even as it rejects a request overall.
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	reset := now.Add(30 * time.Minute)
	r.api.script(workToken, rejectedUntil(reset, session, weekOf(0.5, 24*time.Hour)))

	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the request went to %s last, want side, work having rejected it", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{Until: reset})
	if best := r.rt.Status().Best; best != "side" {
		t.Errorf("the best account is %q, want side: work is barred", best)
	}
	r.clock.advance(30*time.Minute - time.Second)
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("a new session just before work's reset went to %s, want side", got)
	}
	r.clock.advance(time.Second)
	if got := r.ask(t, "three", opus, ""); got != "work" {
		t.Errorf("a new session at work's reset went to %s, want work again", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{})
}

func TestARejectionReadBelowTheUseLastReadBarsItsAccount(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// Work's session reads as nearly spent, and its quota needs using first.
	nearlySpent := session
	nearlySpent.Utilization = 0.99
	r.readsAs(workToken, nearlySpent, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// The 429 rejecting it reads it a little lower, as an answer to a request
	// sent earlier can: the higher use stands, and so does the rejection.
	rejected := nearlySpent
	rejected.Utilization, rejected.Status = 0.98, quota.StatusRejected
	r.api.script(workToken, limitReached("You've hit your limit", rejected))

	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the request went to %s last, want side, work having rejected it", got)
	}
	spent := nearlySpent
	spent.Status = quota.StatusRejected
	if work, _ := r.rt.Status().Account("work"); !reflect.DeepEqual(work.Windows[0], spent) {
		t.Fatalf("work's session reads %+v, want %+v: its higher use, rejected", work.Windows[0], spent)
	}
	checkLimit(t, r.rt, "work", status.Limit{Windows: []string{"5h"}, Until: session.ResetsAt})
	r.clock.advance(time.Minute)
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("a new session went to %s, want side: work's session rejected it", got)
	}
	waitForLine(t, log, "level=WARN", `msg="limit reached"`, "account=work", "windows=5h", "until=")
	want := router.LimitReached{Account: "work", Windows: []string{"5h"}, Until: session.ResetsAt}
	if got := r.events.heard(); len(got) == 0 || !reflect.DeepEqual(got[0], want) {
		t.Errorf("events = %+v, want %+v first", got, want)
	}
}

func TestALimitThatDoesntSayForHowLongHoldsFiveMinutes(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.api.script(workToken, limitReached("You've hit your limit", session, weekOf(0.5, 24*time.Hour)))

	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the request went to %s last, want side, work having rejected it", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{Until: now.Add(5 * time.Minute)})
	r.clock.advance(5*time.Minute - time.Second)
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("a new session just under five minutes on went to %s, want side", got)
	}
	r.clock.advance(time.Second)
	if got := r.ask(t, "three", opus, ""); got != "work" {
		t.Errorf("a new session five minutes on went to %s, want work again", got)
	}
}

func TestAReadingShowingRoomLiftsALimitEarly(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// Work's session is spent until half an hour on, and the limit holds for
	// two hours, as the 429 says overall.
	shortSpent := quota.Window{Key: "5h", Label: "Session", Utilization: 1, ResetsAt: now.Add(30 * time.Minute), Status: quota.StatusRejected}
	r.api.script(workToken, rejectedUntil(now.Add(2*time.Hour), shortSpent, weekOf(0.5, 24*time.Hour)))
	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the request went to %s last, want side, work having rejected it", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{Windows: []string{"5h"}, Until: now.Add(2 * time.Hour)})

	// Once the session resets, a probe of work, stale by then, reads it
	// afresh.
	r.clock.advance(31 * time.Minute)
	fresh := quota.Window{Key: "5h", Label: "Session", Utilization: 0.01, ResetsAt: now.Add(5*time.Hour + 30*time.Minute), Status: quota.StatusAllowed}
	r.readsAs(workToken, fresh, weekOf(0.5, 24*time.Hour))
	if got := r.ask(t, "two", opus, ""); got != "work" {
		t.Errorf("once a probe read work's session afresh, a new session went to %s, want work", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{})
}

func TestAResetMadeByHandIsSeenOnceTheRouterRefreshes(t *testing.T) {
	r, client := limitedOnItsWeek(t)
	// Work's session lapses, and then its week is reset by hand, a new week
	// begun, well before the limit's own reset.
	r.clock.advance(5 * time.Hour)
	fresh := quota.Window{Key: "5h", Label: "Session", Utilization: 0.01, ResetsAt: now.Add(10 * time.Hour), Status: quota.StatusAllowed}
	r.readsAs(workToken, fresh, weekOf(0.01, 7*24*time.Hour))

	if _, err := client.Refresh(t.Context(), time.Minute); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	checkLimit(t, r.rt, "work", status.Limit{})
	if got := r.ask(t, "two", opus, ""); got != "work" {
		t.Errorf("a new session went to %s, want work, its week reset", got)
	}
	if got := limitsReached(r.events.heard()); got != 1 {
		t.Errorf("%d limits reached, want 1: the probe's reading lifted it", got)
	}
}

func TestAProbeAnsweredWithTheLimitKeepsItsBar(t *testing.T) {
	r, client := limitedOnItsWeek(t)
	r.clock.advance(5 * time.Hour)
	fresh := quota.Window{Key: "5h", Label: "Session", Utilization: 0, ResetsAt: now.Add(10 * time.Hour), Status: quota.StatusAllowed}
	r.readsAs(workToken, fresh, spentWeek())
	probes := r.prober.probes(workToken)

	if _, err := client.Refresh(t.Context(), time.Minute); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if r.prober.probes(workToken) == probes {
		t.Fatal("work wasn't probed, want it probed: its limit holds back every request, so its session lapsing doesn't stop a probe")
	}
	checkLimit(t, r.rt, "work", status.Limit{Windows: []string{"7d"}, Until: spentWeek().ResetsAt})
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("a new session went to %s, want side: work's week is still spent", got)
	}
	if got := limitsReached(r.events.heard()); got != 1 {
		t.Errorf("%d limits reached, want 1: a probe reading the limit is no news", got)
	}
}

// limitedOnItsWeek is a routed router whose work, its quota needing using
// first, has just reached the limit of its week, and a client of its control
// API.
func limitedOnItsWeek(t *testing.T) (*routed, *router.Client) {
	t.Helper()
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.api.script(workToken, limitReached("You've hit your weekly limit", session, spentWeek()))
	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the request went to %s last, want side, work having rejected it", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{Windows: []string{"7d"}, Until: spentWeek().ResetsAt})
	return r, router.NewClient(serveControl(t, r.rt))
}

// spentWeek is a week the API rejects, spent until tomorrow.
func spentWeek() quota.Window {
	w := weekOf(1, 24*time.Hour)
	w.Status = quota.StatusRejected
	return w
}

// limitsReached counts the limits reached among events.
func limitsReached(events []router.Event) int {
	n := 0
	for _, e := range events {
		if _, ok := e.(router.LimitReached); ok {
			n++
		}
	}
	return n
}

func TestAWindowRejectedWithoutItsUseBarsTheRequestsItCountsAlone(t *testing.T) {
	r := newRouted(t)
	models := map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}}
	for token, left := range map[string]time.Duration{workToken: 24 * time.Hour, sideToken: 5 * 24 * time.Hour} {
		windows := []quota.Window{session, weekOf(0.5, left), fableWeek}
		r.prober.answer(token, probeResult{usage: quota.Usage{Windows: windows}, models: models})
		r.api.set(token, windows...)
	}
	// Work's Fable week rejects a request, its status and reset read, but
	// not its use.
	reset := now.Add(48 * time.Hour)
	r.api.script(workToken, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Anthropic-Ratelimit-Unified-7d_oi-Status", "rejected")
		w.Header().Set("Anthropic-Ratelimit-Unified-7d_oi-Reset", strconv.FormatInt(reset.Unix(), 10))
		limitReached("You've hit your Fable limit", session, weekOf(0.5, 24*time.Hour))(w, req)
	})

	if got := r.ask(t, "writing", fable, ""); got != "side" {
		t.Fatalf("the Fable request went to %s last, want side, work having rejected it", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{Windows: []string{"7d_oi"}, Until: reset})
	if got := r.ask(t, "titling", haiku, ""); got != "work" {
		t.Errorf("a Haiku session went to %s, want work: its Fable week doesn't count Haiku", got)
	}
	if got := r.ask(t, "drafting", fable, ""); got != "side" {
		t.Errorf("another Fable session went to %s, want side: work's Fable week rejected Fable", got)
	}
}

// rejectedUntil answers with a 429 rejecting the request overall, until the
// time given, whatever windows report.
func rejectedUntil(until time.Time, windows ...quota.Window) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Anthropic-Ratelimit-Unified-Reset", strconv.FormatInt(until.Unix(), 10))
		limitReached("You've hit your limit", windows...)(w, r)
	}
}

// checkLimit checks the router's status document gives the account with the
// given id the limit want, none when it's zero.
func checkLimit(t *testing.T, rt *router.Router, id string, want status.Limit) {
	t.Helper()
	if got, _ := rt.Status().Account(id); !reflect.DeepEqual(got.Limit, want) {
		t.Errorf("%s's limit = %+v, want %+v", id, got.Limit, want)
	}
}

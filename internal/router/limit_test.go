package router_test

import (
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
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
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Until: reset})
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
	// The 429 rejecting it reads it a little lower: sent since the session
	// was read, it's the upstream's latest word, and so is the rejection.
	rejected := nearlySpent
	rejected.Utilization, rejected.Status = 0.98, quota.StatusRejected
	r.api.script(workToken, limitReached("You've hit your limit", rejected))

	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the request went to %s last, want side, work having rejected it", got)
	}
	if work, _ := r.rt.Status().Account("work"); !reflect.DeepEqual(asRead(work.Windows)[0], rejected) {
		t.Fatalf("work's session reads %+v, want %+v: the 429's reading", work.Windows[0], rejected)
	}
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Windows: []string{"5h"}, Until: session.ResetsAt})
	r.clock.advance(time.Minute)
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("a new session went to %s, want side: work's session rejected it", got)
	}
	waitForLine(t, log, "level=WARN", `msg="limit reached"`, "account=work", "windows=5h", "until=")
	want := router.LimitReached{Account: "work", Windows: []string{"5h"}, Until: session.ResetsAt, Limit: 1}
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
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Until: now.Add(5 * time.Minute)})
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
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Windows: []string{"5h"}, Until: now.Add(2 * time.Hour)})

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
	keptReset := spentWeek()
	keptReset.Utilization, keptReset.Status = 0.01, quota.StatusAllowed
	tests := []struct {
		name string
		// week is work's week as the reset by hand leaves it.
		week quota.Window
	}{
		{name: "a new week begun", week: weekOf(0.01, 7*24*time.Hour)},
		{name: "the week's use dropped, its reset kept", week: keptReset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, client := limitedOnItsWeek(t)
			// Work's session lapses, and then its week is reset by hand,
			// well before the limit's own reset.
			r.clock.advance(5 * time.Hour)
			checkLapsed(t, r.rt, "work")
			fresh := quota.Window{Key: "5h", Label: "Session", Utilization: 0.01, ResetsAt: now.Add(10 * time.Hour), Status: quota.StatusAllowed}
			r.readsAs(workToken, fresh, tt.week)

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
		})
	}
}

func TestAResetMadeByHandJustAfterTheLimitIsSeenOnceTheRouterRefreshes(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	if got := r.ask(t, "one", opus, ""); got != "work" {
		t.Fatalf("the request went to %s, want work, its quota needing using first", got)
	}
	// Minutes on, work reaches the limit of its week, whose answer is the
	// latest reading of it when its week is reset by hand half a minute on.
	r.clock.advance(5 * time.Minute)
	r.api.script(workToken, limitReached("You've hit your weekly limit", session, spentWeek()))
	if got := r.ask(t, "one", opus, ""); got != "side" {
		t.Fatalf("the request went to %s last, want side, work having rejected it", got)
	}
	r.clock.advance(30 * time.Second)
	r.readsAs(workToken, session, weekOf(0.01, 7*24*time.Hour))

	if _, err := router.NewClient(serveControl(t, r.rt)).Refresh(t.Context(), time.Minute); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	checkLimit(t, r.rt, "work", status.Limit{})
	if got := r.ask(t, "two", opus, ""); got != "work" {
		t.Errorf("a new session went to %s, want work, its week reset", got)
	}
}

func TestALimitFromBeforeAResetMadeByHandIsPassedOver(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// A request goes out on work, and is answered late, rejected in its
	// session; another, sent after it, is answered first, reading the
	// session reset by hand in between.
	arrived, release := make(chan struct{}), make(chan struct{})
	r.api.script(workToken, func(w http.ResponseWriter, req *http.Request) {
		close(arrived)
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			t.Error("the request sent before the reset was never let go")
		}
		limitReached("You've hit your limit", sessionSpent, weekOf(0.5, 24*time.Hour))(w, req)
	})
	answered := make(chan int, 1)
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	t.Cleanup(client.CloseIdleConnections)
	go func() {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, r.proxy+"/v1/messages", strings.NewReader(messages))
		if err != nil {
			t.Error(err)
			answered <- 0
			return
		}
		req.Header = claudeCode(workToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Error(err)
			answered <- 0
			return
		}
		_ = resp.Body.Close()
		answered <- resp.StatusCode
	}()
	<-arrived
	reset := session
	reset.Utilization = 0.02
	r.readsAs(workToken, reset, weekOf(0.5, 24*time.Hour))
	if got := r.ask(t, "two", opus, ""); got != "work" {
		t.Errorf("the request sent second went to %s, want work, its quota needing using first", got)
	}
	close(release)

	if got := <-answered; got != http.StatusOK {
		t.Errorf("the request sent first was answered %d, want 200, sent again on work since its reset", got)
	}
	if got := r.api.accounts(); !slices.Equal(got, []string{"work", "work", "work"}) {
		t.Errorf("the requests went out on %q, want work each time", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{})
	if work, _ := r.rt.Status().Account("work"); !slices.ContainsFunc(work.Windows, func(w quota.Window) bool {
		return w.Key == "5h" && w.Utilization == reset.Utilization && w.Status == quota.StatusAllowed
	}) {
		t.Errorf("work reads %+v, want its session as the reset left it: the 429 is from before", work.Windows)
	}
	if got := limitsReached(r.events.heard()); got != 0 {
		t.Errorf("%d limits reached, want none", got)
	}
	for _, want := range [][]string{
		{"level=INFO", `msg="limit from before a reset passed over"`, "account=work", "windows=5h"},
		{"level=INFO", "msg=replaying", "attempt=2", "from=work", "to=work", `why="was reset by hand"`},
	} {
		waitForLine(t, log, want...)
	}
}

func TestALimitReachedInNoWindowNamedLiftsOnceAProbeSinceIsTaken(t *testing.T) {
	tests := []struct {
		name string
		// admitted is set when the probe is answered with success.
		admitted  bool
		wantLimit status.Limit
	}{
		{name: "a probe answered with success", admitted: true},
		{name: "a probe answered with the limit", wantLimit: status.Limit{ID: 1, Until: now.Add(2 * time.Hour)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRouted(t)
			r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
			r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
			r.api.script(workToken, rejectedUntil(now.Add(2*time.Hour), session, weekOf(0.5, 24*time.Hour)))
			if got := r.ask(t, "one", opus, ""); got != "side" {
				t.Fatalf("the request went to %s last, want side, work having rejected it", got)
			}
			checkLimit(t, r.rt, "work", status.Limit{ID: 1, Until: now.Add(2 * time.Hour)})

			r.clock.advance(2 * time.Minute)
			r.prober.answer(workToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, weekOf(0.5, 24*time.Hour)}}, admitted: tt.admitted})
			probes := r.prober.probes(workToken)
			if _, err := router.NewClient(serveControl(t, r.rt)).Refresh(t.Context(), time.Minute); err != nil {
				t.Fatalf("Refresh() error = %v", err)
			}
			if r.prober.probes(workToken) == probes {
				t.Fatal("work wasn't probed, want it probed: it can take no request, and nothing has been read of it for longer than the refresh asks")
			}
			checkLimit(t, r.rt, "work", tt.wantLimit)
		})
	}
}

func TestALimitReachedInNoWindowNamedLiftsOnceARequestSentSinceIsTaken(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// Both accounts reject a request overall, holding back every request.
	until := now.Add(2 * time.Hour)
	r.api.script(workToken, rejectedUntil(until, session, weekOf(0.5, 24*time.Hour)))
	r.api.script(sideToken, rejectedUntil(until, session, weekOf(0.5, 5*24*time.Hour)))
	r.ask(t, "one", opus, "")
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Until: until})
	checkLimit(t, r.rt, "side", status.Limit{ID: 2, Until: until})

	// With no account left, the next requests go out on their client's, work,
	// which takes them. Counting a message's tokens spends no quota, so says
	// nothing of the limit.
	body := `{"model":"` + opus + `","messages":[{"role":"user","content":"hello"}]}`
	readAll(t, send(t, http.MethodPost, r.proxy+"/v1/messages/count_tokens", claudeCode(workToken), strings.NewReader(body)))
	if got := r.api.lastAccount(); got != "work" {
		t.Fatalf("counting tokens went to %s, want work, its client's", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Until: until})
	if got := r.ask(t, "two", opus, ""); got != "work" {
		t.Fatalf("the request went to %s, want work, its client's", got)
	}
	checkLimit(t, r.rt, "work", status.Limit{})
	checkLimit(t, r.rt, "side", status.Limit{ID: 2, Until: until})
}

func TestAProbeAnsweredWithTheLimitKeepsItsBar(t *testing.T) {
	r, client := limitedOnItsWeek(t)
	r.clock.advance(5 * time.Hour)
	checkLapsed(t, r.rt, "work")
	fresh := quota.Window{Key: "5h", Label: "Session", Utilization: 0, ResetsAt: now.Add(10 * time.Hour), Status: quota.StatusAllowed}
	r.readsAs(workToken, fresh, spentWeek())
	probes := r.prober.probes(workToken)

	if _, err := client.Refresh(t.Context(), time.Minute); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if r.prober.probes(workToken) == probes {
		t.Fatal("work wasn't probed, want it probed: its limit holds back every request, so its session lapsing doesn't stop a probe")
	}
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Windows: []string{"7d"}, Until: spentWeek().ResetsAt})
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
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Windows: []string{"7d"}, Until: spentWeek().ResetsAt})
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
	checkLimit(t, r.rt, "work", status.Limit{ID: 1, Windows: []string{"7d_oi"}, Until: reset})
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

// checkLapsed checks the router's status document has the session of the
// account with the given id lapsed.
func checkLapsed(t *testing.T, rt *router.Router, id string) {
	t.Helper()
	if got, _ := rt.Status().Account(id); !slices.Contains(got.Lapsed, "5h") {
		t.Fatalf("%s's lapsed windows are %q, want its session among them", id, got.Lapsed)
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

package router_test

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
)

// sessionSpent is a session window at its limit, as a 429 reports it.
var sessionSpent = quota.Window{Key: "5h", Label: "Session", Utilization: 1, ResetsAt: session.ResetsAt, Status: quota.StatusRejected}

func TestARequestOverItsAccountsLimitIsReplayedOnAnother(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// Work's quota needs using first, so the session goes there.
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.api.script(workToken, limitReached("You've hit your limit", sessionSpent, weekOf(0.5, 24*time.Hour)))
	const first, second = "event: message_start\ndata: {}\n\n", "event: message_stop\ndata: {}\n\n"
	read := make(chan struct{})
	r.api.script(sideToken, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, first)
		_ = http.NewResponseController(w).Flush()
		select {
		case <-read:
		case <-time.After(5 * time.Second):
			t.Error("the client never got the first event while the stream was open: the proxy held it back")
		}
		_, _ = io.WriteString(w, second)
	})

	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("answered %d (%s), want side's 200 stream", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	events := bufio.NewReader(resp.Body)
	if got := readEvent(t, events); got != first {
		t.Fatalf("first event = %q, want %q", got, first)
	}
	close(read)
	if rest, err := io.ReadAll(events); err != nil || string(rest) != second {
		t.Errorf("rest of the stream = %q (%v), want %q", rest, err, second)
	}
	if got := r.api.accounts(); !slices.Equal(got, []string{"work", "side"}) {
		t.Errorf("the request went out on %q, want work, then side", got)
	}

	if work, _ := r.rt.Status().Account("work"); !slices.Contains(work.Windows, sessionSpent) {
		t.Errorf("work reads %+v, want its spent session recorded off the 429", work.Windows)
	}
	if got := r.ask(t, sessionID, opus, ""); got != "side" {
		t.Errorf("the session's next request went to %s, want side, where it moved", got)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="limit reached"`, "account=work", "windows=5h"},
		{"level=INFO", "msg=moved", "session=0b5c6f2e", "from=work", "to=side", `reason="moved: work hit its limit"`},
		{"level=INFO", "msg=replaying", "attempt=2", "from=work", "to=side", `why="hit its limit"`},
		{"level=INFO", "msg=routed", "account=side", `reason="moved: work hit its limit"`, "status=200", "attempts=2"},
		{"level=INFO", "msg=routed", "account=side", "reason=sticky", "status=200"},
	} {
		waitForLine(t, log, want...)
	}
	wantEvents := []router.Event{
		router.LimitReached{Account: "work", Windows: []string{"5h"}, Until: sessionSpent.ResetsAt},
		router.Moved{Session: sessionID, Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit"},
	}
	if got := r.events.heard(); !reflect.DeepEqual(got, wantEvents) {
		t.Errorf("events = %+v, want %+v", got, wantEvents)
	}
}

func TestAReplayedBodyGoesUpstreamByteForByte(t *testing.T) {
	odd := `{ "model" : "claude-opus-5-5",` + "\n\t" + `"messages":[{"role":"user","content":"café or café, \"q\" \\ ☃"}] }  `
	large := `{"model":"claude-opus-5-5","messages":[{"role":"user","content":"` + strings.Repeat("0123456789abcdef", 1<<16) + `"}]}`
	tests := []struct {
		name string
		body string
		// chunked sends the body without a length.
		chunked bool
	}{
		{name: "with a length", body: odd},
		{name: "without a length", body: odd, chunked: true},
		{name: "a megabyte", body: large},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRouted(t)
			r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
			r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
			r.api.script(workToken, limitReached("You've hit your limit", sessionSpent))
			var body io.Reader = strings.NewReader(tt.body)
			if tt.chunked {
				body = io.MultiReader(body)
			}

			readAll(t, send(t, http.MethodPost, r.proxy+"/v1/messages", claudeCode(workToken), body))
			bodies := r.api.bodies()
			if len(bodies) != 2 {
				t.Fatalf("upstream received %d requests, want 2: work's, then side's", len(bodies))
			}
			for i, got := range bodies {
				if got != tt.body {
					t.Errorf("attempt %d went up with a body of %d bytes that differs from the %d sent", i+1, len(got), len(tt.body))
				}
			}
		})
	}
}

func TestWhenEveryAccountIsAtItsLimitTheLast429IsTheClients(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t, withPersonalToken)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 3*24*time.Hour))
	r.readsAs(personalToken, session, weekOf(0.5, 5*24*time.Hour))
	for _, token := range []string{workToken, sideToken, personalToken} {
		r.api.script(token, limitReached(accountOf(token)+" is at its limit", sessionSpent))
	}

	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	body := readAll(t, resp)
	want := `{"type":"error","error":{"type":"rate_limit_error","message":"personal is at its limit"}}`
	if resp.StatusCode != http.StatusTooManyRequests || body != want {
		t.Errorf("answered %d %s, want the last account's 429 as it came: %s", resp.StatusCode, body, want)
	}
	if got := resp.Header.Get("Anthropic-Ratelimit-Unified-5h-Status"); got != "rejected" {
		t.Errorf("the 429's session status = %q, want its own headers as they came", got)
	}
	if got := r.api.accounts(); !slices.Equal(got, []string{"work", "side", "personal"}) {
		t.Errorf("the request went out on %q, want each account once, the one whose quota most needs using first", got)
	}
	for _, want := range [][]string{
		{"level=INFO", "msg=replaying", "attempt=2", "from=work", "to=side", `why="hit its limit"`},
		{"level=INFO", "msg=replaying", "attempt=3", "from=side", "to=personal", `why="hit its limit"`},
		{"level=WARN", `msg="no account left to try"`, "attempts=3"},
		{"level=INFO", "msg=routed", "account=personal", "status=429", "attempts=3"},
	} {
		waitForLine(t, log, want...)
	}
}

func TestARequestIsntReplayedWhereTheresNoRoomEither(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// The client's own account, work, is spent, so the request goes to side,
	// and has nowhere with room to go from there.
	r.readsAs(workToken, sessionSpent, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.api.script(sideToken, limitReached("You've hit your limit", sessionSpent))

	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", with(claudeCode(workToken), "X-Claude-Code-Session-Id", ""), strings.NewReader(messages))
	if body := readAll(t, resp); resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("answered %d %s, want side's 429", resp.StatusCode, body)
	}
	if got := r.api.accounts(); !slices.Equal(got, []string{"side"}) {
		t.Errorf("the request went out on %q, want side alone: work, the client's, has no room either", got)
	}
	waitForLine(t, log, "level=WARN", `msg="no account left to try"`, "attempts=1")
}

func TestAnAccountWhoseTokenIsRefusedIsSkippedForTenMinutes(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// Work's quota needs using first, so every new session goes there while
	// it can.
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.api.script(workToken, refuseWith(http.StatusForbidden, "This organization has been disabled."))

	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", with(claudeCode(workToken), "X-Claude-Code-Session-Id", "one"), strings.NewReader(messages))
	if body := readAll(t, resp); resp.StatusCode != http.StatusOK || body != `{"type":"message"}` {
		t.Errorf("answered %d %s, want side's 200", resp.StatusCode, body)
	}
	if got := r.api.accounts(); !slices.Equal(got, []string{"work", "side"}) {
		t.Errorf("the request went out on %q, want work, then side", got)
	}
	r.clock.advance(10*time.Minute - time.Second)
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("a new session just under ten minutes on went to %s, want side: work was refused", got)
	}
	r.clock.advance(time.Second)
	if got := r.ask(t, "three", opus, ""); got != "work" {
		t.Errorf("a new session ten minutes on went to %s, want work again", got)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="upstream refused the account's token"`, "account=work", "status=403", `error="This organization has been disabled."`},
		{"level=INFO", "msg=replaying", "attempt=2", "from=work", "to=side", `why="was refused"`},
		{"level=INFO", "msg=moved", "session=one", "from=work", "to=side", `reason="moved: work was refused"`},
		{"level=INFO", "msg=routed", "session=one", "account=side", "status=200", "attempts=2"},
	} {
		waitForLine(t, log, want...)
	}
	wantEvents := []router.Event{
		router.Refused{Account: "work", Status: http.StatusForbidden},
		router.Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work was refused"},
	}
	if got := r.events.heard(); !reflect.DeepEqual(got, wantEvents) {
		t.Errorf("events = %+v, want %+v", got, wantEvents)
	}
}

func TestNothingIsReplayedOnceTheClientHasPartOfTheAnswer(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.api.script(workToken, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
		_ = http.NewResponseController(w).Flush()
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("upstream: hijack: %v", err)
			return
		}
		_ = conn.Close()
	})

	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Error("the stream read to its end, want it cut short, for Claude Code to retry")
	}
	waitForLine(t, log, "level=INFO", "msg=routed", "account=work", "status=200")
	if got := r.api.accounts(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("the request went out on %q, want work alone: the client had the stream's start", got)
	}
	if log.Has("msg=replaying") {
		t.Errorf("log reads\n%s\nwant no replay", log)
	}
}

func TestAFailureToReachTheUpstreamIsntReplayed(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	r.api.script(workToken, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("upstream: hijack: %v", err)
			return
		}
		_ = conn.Close()
	})

	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	if resp.StatusCode != http.StatusBadGateway || resp.Header.Get("X-Should-Retry") != "" {
		t.Errorf("answered %d, X-Should-Retry %q, want a 502 Claude Code retries", resp.StatusCode, resp.Header.Get("X-Should-Retry"))
	}
	checkAPIError(t, readAll(t, resp), "api_error", "switchboard: the request to the upstream failed: ")
	if got := r.api.accounts(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("the request went out on %q, want work alone: a failure to reach the upstream isn't the account's", got)
	}
	waitForLine(t, log, "level=ERROR", `msg="upstream request failed"`, "account=work")
	waitForLine(t, log, "level=INFO", "msg=routed", "account=work", "status=502")
	if log.Has("msg=replaying") {
		t.Errorf("log reads\n%s\nwant no replay", log)
	}
}

func TestWithNoAccountWithRoomTheAccountsWithoutAreProbedAgain(t *testing.T) {
	log := logstest.Capture(t)
	r := newRouted(t)
	// Both sessions are spent, and when each resets isn't known.
	unknownReset := sessionSpent
	unknownReset.ResetsAt = time.Time{}
	r.readsAs(workToken, unknownReset, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, unknownReset, weekOf(0.5, 5*24*time.Hour))
	r.api.script(workToken, limitReached("You've hit your limit", unknownReset, weekOf(0.5, 24*time.Hour)))
	resp := send(t, http.MethodPost, r.proxy+"/v1/messages", with(claudeCode(workToken), "X-Claude-Code-Session-Id", "one"), strings.NewReader(messages))
	if body := readAll(t, resp); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("with every account spent, answered %d %s, want the 429", resp.StatusCode, body)
	}
	waitForLine(t, log, "level=WARN", `msg="no account left to try"`, "attempts=1")

	// Side's session resets unseen: its probe reads it afresh. Work, barred
	// for five minutes as its 429 didn't say for how long, isn't among those
	// probed again.
	r.clock.advance(2 * time.Minute)
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	if got := r.ask(t, "two", opus, ""); got != "side" {
		t.Errorf("once side's session reset, a new session went to %s, want side", got)
	}
	waitForLine(t, log, "level=WARN", `msg="no account has room; probing again"`, "accounts=side")
	waitForLine(t, log, "msg=routed", "session=two", "account=side", "reason=new", "status=200")
}

func TestAccountsWithoutRoomAreProbedAgainOnceAMinuteAtMost(t *testing.T) {
	r := newRouted(t)
	unknownReset := sessionSpent
	unknownReset.ResetsAt = time.Time{}
	r.readsAs(workToken, unknownReset, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, unknownReset, weekOf(0.5, 5*24*time.Hour))
	// Each new session falls back to work, whose answers read it anew: side,
	// which nothing goes out on, is read by its probes alone.
	steps := []struct {
		after time.Duration
		// probes is how many times side has been probed once the step's
		// request is chosen.
		probes int
	}{
		{after: 0, probes: 1},
		{after: 2 * time.Minute, probes: 2},
		{after: 59 * time.Second, probes: 2},
		{after: time.Second, probes: 3},
	}
	for i, step := range steps {
		r.clock.advance(step.after)
		r.ask(t, fmt.Sprint("session", i), opus, "")
		if got := r.prober.probes(sideToken); got != step.probes {
			t.Errorf("after the request %s in, side has been probed %d times, want %d", step.after, got, step.probes)
		}
	}
}

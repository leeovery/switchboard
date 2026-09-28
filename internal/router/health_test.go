package router_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestTheRoutersHealthCountsTheFailuresItMakes(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	tests := []struct {
		name     string
		upstream string
		// answer is how the upstream answers, when there is one.
		answer     http.HandlerFunc
		wantStatus int
		wantFailed bool
	}{
		{name: "an answer", answer: answerOK, wantStatus: http.StatusOK},
		{name: "a 500 passed on", answer: answerWith(http.StatusInternalServerError), wantStatus: http.StatusInternalServerError},
		{name: "a 529 passed on", answer: answerWith(529), wantStatus: 529},
		{name: "a 429 passed on", answer: limitReached("You've hit your limit", sessionSpent), wantStatus: http.StatusTooManyRequests},
		{name: "a refusal with no account to fail over to", answer: refuseWith(http.StatusForbidden, "no"), wantStatus: http.StatusBadGateway, wantFailed: true},
		{name: "an upstream that can't be reached", upstream: gone.URL, wantStatus: http.StatusBadGateway, wantFailed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.upstream
			if tt.answer != nil {
				url = newUpstream(t, tt.answer).URL
			}
			rt := newRouter(t, url)

			resp := send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
			if readAll(t, resp); resp.StatusCode != tt.wantStatus {
				t.Errorf("answered %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			waitUntil(t, "the request is done", func() bool { return rt.Status().Router.Requests == 1 })
			want := status.Health{Healthy: true, Requests: 1}
			if tt.wantFailed {
				want.Failures = 1
			}
			if got := rt.Status().Router; got != want {
				t.Errorf("router health = %+v, want %+v", got, want)
			}
		})
	}
}

func TestTheRouterTurnsUnhealthyAndHealthyAgain(t *testing.T) {
	log := logstest.Capture(t)
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	clock := newFakeClock(now)
	heard := &eventLog{}
	cfg := testConfig(gone.URL)
	cfg.Now, cfg.Events = clock.read, heard.hear
	rt := newRouterFrom(t, cfg)
	proxy := serveProxy(t, rt)
	client := router.NewClient(serveControl(t, rt))
	fail := func(n int) {
		for range n {
			readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
		}
	}

	fail(4)
	waitUntil(t, "four requests are done", func() bool { return rt.Status().Router.Requests == 4 })
	if h, err := client.Health(t.Context()); err != nil || !h.OK || h.Reason != "" {
		t.Errorf("after four failures, Health() = %+v, %v, want ok: too few to judge by", h, err)
	}
	fail(1)
	waitUntil(t, "five requests are done", func() bool { return rt.Status().Router.Requests == 5 })
	const reason = "5 of the 5 requests in the last 5 minutes failed"
	if h, err := client.Health(t.Context()); err != nil || h.OK || h.Reason != reason {
		t.Errorf("after five failures, Health() = %+v, %v, want not ok, saying %q", h, err, reason)
	}
	doc, err := client.Status(t.Context())
	if want := (status.Health{Requests: 5, Failures: 5, Reason: reason}); err != nil || doc.Router != want {
		t.Errorf("the status document's router = %+v, %v, want %+v", doc.Router, err, want)
	}
	waitForLine(t, log, "level=WARN", "msg=unhealthy", `reason="`+reason+`"`)

	clock.advance(5 * time.Minute)
	if h, err := client.Health(t.Context()); err != nil || !h.OK || h.Reason != "" {
		t.Errorf("five minutes on, Health() = %+v, %v, want ok again", h, err)
	}
	waitForLine(t, log, "level=INFO", `msg="healthy again"`, "requests=0", "failures=0")
	if n := strings.Count(log.String(), "msg=unhealthy"); n != 1 {
		t.Errorf("log reads\n%s\nwant the turn to unhealthy logged once, however often it's asked", log)
	}
	want := []router.Event{router.HealthChanged{Reason: reason}, router.HealthChanged{Healthy: true}}
	if got := heard.heard(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

package router_test

import (
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestANewSessionIsToldOfAsStartedOnceItsAnswered(t *testing.T) {
	r := newRouted(t)
	control := serveControl(t, r.rt)
	// Work's quota needs using first, so new sessions go there.
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// The quota check Claude Code sends as it starts, which the API refuses
	// with a 429 that says nothing of the account's quota.
	r.api.script(workToken, answerWith(http.StatusTooManyRequests))
	r.ask(t, "check", opus, "")
	r.ask(t, "one", opus, "")

	doc := waitForStatus(t, control, func(doc status.Document) bool { return len(doc.Events) > 0 })
	want := []status.Event{{ID: 1, At: now, Kind: status.EventStarted, Account: "work", Session: "one", Model: opus, Reason: "new"}}
	if !reflect.DeepEqual(doc.Events, want) {
		t.Errorf("GET /status gives the events %+v, want %+v: one's start alone, as the check was refused", doc.Events, want)
	}
}

func TestAQuotaCheckAnsweredWithSuccessStartsNoSession(t *testing.T) {
	r := newRouted(t)
	control := serveControl(t, r.rt)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// The quota check Claude Code sends as it starts, on Claude Haiku, which
	// the API answers with success, under the session's own id.
	if got := asking(r.proxy, "one", quotaCheck(haiku)); got != http.StatusOK {
		t.Fatalf("the quota check was answered %d, want 200", got)
	}
	waitUntil(t, "the quota check is done", func() bool { return r.rt.Status().Router.Requests == 1 })
	if sessions, err := router.NewClient(control).Sessions(t.Context()); err != nil || len(sessions) > 0 {
		t.Errorf("GET /sessions gives %+v (%v), want none: the check starts no session", sessions, err)
	}
	r.clock.advance(time.Minute)
	r.ask(t, "one", haiku, "")

	doc := waitForStatus(t, control, func(doc status.Document) bool { return len(doc.Events) > 0 })
	want := []status.Event{{ID: 1, At: now.Add(time.Minute), Kind: status.EventStarted, Account: "work", Session: "one", Model: haiku, Reason: "new"}}
	if !reflect.DeepEqual(doc.Events, want) {
		t.Errorf("GET /status gives the events %+v, want %+v: one's start with its first request, not its check", doc.Events, want)
	}
}

func TestAQuotaCheckSentAlongsideASessionsFirstRequestLeavesItToldOfAsStarted(t *testing.T) {
	r := newRouted(t)
	control := serveControl(t, r.rt)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// Work answers the session's quota check only once its first request, on
	// the same model, has been answered.
	arrived, release := make(chan struct{}), make(chan struct{})
	r.api.script(workToken, func(w http.ResponseWriter, req *http.Request) {
		close(arrived)
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			t.Error("the quota check was never let go")
		}
		answerWith(http.StatusOK, session, weekOf(0.5, 24*time.Hour))(w, req)
	})
	checked := make(chan int, 1)
	go func() { checked <- asking(r.proxy, "one", quotaCheck(opus)) }()
	<-arrived

	if got := r.ask(t, "one", opus, ""); got != "work" {
		t.Errorf("the session's first request went to %s, want work, its quota needing using first", got)
	}
	waitUntil(t, "the first request is done", func() bool { return r.rt.Status().Router.Requests == 1 })
	r.clock.advance(time.Minute)
	close(release)
	if got := <-checked; got != http.StatusOK {
		t.Errorf("the quota check was answered %d, want 200", got)
	}
	waitUntil(t, "both requests are done", func() bool { return r.rt.Status().Router.Requests == 2 })
	want := []status.Event{{ID: 1, At: now, Kind: status.EventStarted, Account: "work", Session: "one", Model: opus, Reason: "new"}}
	if got := r.rt.Status().Events; !reflect.DeepEqual(got, want) {
		t.Errorf("the events are %+v, want %+v: the session's start with its first request, the check alongside it remembered for nothing", got, want)
	}
	if sessions, err := router.NewClient(control).Sessions(t.Context()); err != nil || len(sessions) != 1 || sessions[0].ID != "one" {
		t.Errorf("GET /sessions gives %+v (%v), want session one", sessions, err)
	}
}

func TestAQuotaCheckForcedOffALimitedAccountMovesNoSession(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	// Work's quota needs using first, so a new session's check goes there,
	// reaches work's limit, and goes again on side.
	r.api.script(workToken, limitReached("You've hit your limit", sessionSpent, weekOf(0.5, 24*time.Hour)))

	if got := asking(r.proxy, "one", quotaCheck(opus)); got != http.StatusOK {
		t.Fatalf("the quota check was answered %d, want side's 200", got)
	}
	if got := r.api.accounts(); !slices.Equal(got, []string{"work", "side"}) {
		t.Fatalf("the quota check went out on %q, want work, then side", got)
	}
	waitUntil(t, "the quota check is done", func() bool { return r.rt.Status().Router.Requests == 1 })
	if moved := slices.ContainsFunc(r.events.heard(), func(e router.Event) bool { _, ok := e.(router.Moved); return ok }); moved {
		t.Errorf("events = %+v, want no move: the check moves no session", r.events.heard())
	}
	want := []status.Event{{ID: 1, At: now, Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: sessionSpent.ResetsAt, Limit: 1}}
	if got := r.rt.Status().Events; !reflect.DeepEqual(got, want) {
		t.Errorf("the events are %+v, want %+v: work's limit, having moved no session", got, want)
	}
}

// quotaCheck is the body of the quota check Claude Code sends as it starts,
// on model, its main.
func quotaCheck(model string) string {
	return `{"model":"` + model + `","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`
}

// asking posts a messages request of session, whose body is body, to the
// proxy at proxy on work's token, and returns the status it's answered with,
// or 0 when it isn't.
func asking(proxy, session, body string) int {
	req, err := http.NewRequest(http.MethodPost, proxy+"/v1/messages", strings.NewReader(body))
	if err != nil {
		return 0
	}
	req.Header = with(claudeCode(workToken), claude.SessionHeader, session)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestALimitIsToldOfWithTheSessionsItMovedAndWhereTheyWent(t *testing.T) {
	r := newRouted(t)
	control := serveControl(t, r.rt)
	// Work's quota needs using first, so sessions one and two start there,
	// two on two models.
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
	sessions := []struct{ id, model string }{{"one", opus}, {"two", opus}, {"two", haiku}}
	for i, s := range sessions {
		r.ask(t, s.id, s.model, "")
		waitForStatus(t, control, func(doc status.Document) bool { return len(doc.Events) == i+1 })
	}

	// One's next request reaches work's limit, and is replayed on side, and
	// two's follow it there.
	r.api.script(workToken, limitReached("You've hit your limit", sessionSpent, weekOf(0.5, 24*time.Hour)))
	for _, s := range sessions {
		if got := r.ask(t, s.id, s.model, ""); got != "side" {
			t.Fatalf("session %s's %s request went out on %s last, want side", s.id, s.model, got)
		}
	}
	doc := waitForStatus(t, control, func(doc status.Document) bool { return len(doc.Events) == 7 })
	want := []status.Event{
		{ID: 7, At: now, Kind: status.EventMoved, Session: "two", Model: haiku, From: "work", To: "side", Reason: "moved: work has no room", Limit: 4},
		{ID: 6, At: now, Kind: status.EventMoved, Session: "two", Model: opus, From: "work", To: "side", Reason: "moved: work has no room", Limit: 4},
		{ID: 5, At: now, Kind: status.EventMoved, Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit", Limit: 4},
		{ID: 4, At: now, Kind: status.EventLimit, Account: "work", To: "side", Windows: []string{"5h"}, Until: sessionSpent.ResetsAt, Count: 2, Limit: 1},
		{ID: 3, At: now, Kind: status.EventStarted, Account: "work", Session: "two", Model: haiku, Reason: "new"},
		{ID: 2, At: now, Kind: status.EventStarted, Account: "work", Session: "two", Model: opus, Reason: "new"},
		{ID: 1, At: now, Kind: status.EventStarted, Account: "work", Session: "one", Model: opus, Reason: "new"},
	}
	if !reflect.DeepEqual(doc.Events, want) {
		t.Errorf("GET /status gives the events\n%+v\nwant\n%+v", doc.Events, want)
	}
}

func TestARestartFallingDueIsToldOf(t *testing.T) {
	s := newSelfWatching(t, false)
	startRouter(t, s.cfg)
	socket := router.SocketPath(s.cfg.StateDir)

	s.upgrade(t)
	waitForStatus(t, socket, func(doc status.Document) bool { return doc.Restart.Reason == "upgraded" })
	writeFile(t, s.config, twoAccounts)
	doc := waitForStatus(t, socket, func(doc status.Document) bool { return doc.Restart.Reason == "config changed" })
	want := []status.Event{{ID: 1, At: now, Kind: status.EventRestart, Reason: "upgraded"}}
	if !reflect.DeepEqual(doc.Events, want) {
		t.Errorf("GET /status gives the events %+v, want %+v: the restart as it fell due, its reason changing since", doc.Events, want)
	}
}

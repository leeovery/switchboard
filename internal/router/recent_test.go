package router_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
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
	check := `{"model":"` + haiku + `","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`
	header := with(claudeCode(workToken), claude.SessionHeader, "one")
	if resp := send(t, http.MethodPost, r.proxy+"/v1/messages", header, strings.NewReader(check)); resp.StatusCode != http.StatusOK {
		t.Fatalf("the quota check was answered %d, want 200", resp.StatusCode)
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
		{ID: 4, At: now, Kind: status.EventLimit, Account: "work", To: "side", Windows: []string{"5h"}, Until: sessionSpent.ResetsAt, Count: 2},
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
	// Written whole: a look at the file caught as it's written would read a
	// config that isn't valid, which has no restart due, and the look after
	// would tell of another restart falling due.
	if err := atomicfile.Write(s.config, []byte(twoAccounts), 0o600); err != nil {
		t.Fatal(err)
	}
	doc := waitForStatus(t, socket, func(doc status.Document) bool { return doc.Restart.Reason == "config changed" })
	want := []status.Event{{ID: 1, At: now, Kind: status.EventRestart, Reason: "upgraded"}}
	if !reflect.DeepEqual(doc.Events, want) {
		t.Errorf("GET /status gives the events %+v, want %+v: the restart as it fell due, its reason changing since", doc.Events, want)
	}
}

package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestClientHealth(t *testing.T) {
	client := router.NewClient(serveControl(t, newRouter(t, "http://127.0.0.1:1")))

	got, err := client.Health(t.Context())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if want := (router.Health{OK: true, Version: "1.2.3", PID: os.Getpid(), StartedAt: now, StripsOwnHeaders: true}); got != want {
		t.Errorf("Health() = %+v, want %+v", got, want)
	}
}

func TestHealthSaysTheRouterStripsItsOwnHeaders(t *testing.T) {
	rt := newRouter(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()

	rt.Control().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, `"strips_own_headers":true`) {
		t.Errorf("GET /health answered %d %s, want 200, saying the router takes every X-Switchboard- header off what it sends upstream", rec.Code, body)
	}
}

func TestClientHealthOfARouterFromBeforeItStrippedItsOwnHeaders(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok": true, "listen": "127.0.0.1:4747", "version": "0.1.0", "pid": 4242, "started_at": "2026-10-03T09:00:00Z"}`)
	})
	serveOn(t, path, mux)

	got, err := router.NewClient(path).Health(t.Context())
	if err != nil || !got.OK || got.StripsOwnHeaders {
		t.Errorf("Health() = %+v, %v, want it healthy, and not taking every X-Switchboard- header off", got, err)
	}
}

func TestClientStatus(t *testing.T) {
	up := newUpstream(t, answerWith(http.StatusOK, session, week))
	rt := newRouter(t, up.URL)
	client := router.NewClient(serveControl(t, rt))
	readAll(t, send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
	waitUntil(t, "the request is done", func() bool { return rt.Status().Router.Requests == 1 })

	got, err := client.Status(t.Context())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	want := status.Document{
		GeneratedAt: now,
		Source:      "router",
		Best:        "work",
		Router:      status.Health{Healthy: true, Requests: 1},
		Sessions:    1,
		Events:      []status.Event{{ID: 1, At: now, Kind: status.EventStarted, Account: "work", Session: sessionID, Model: opus, Reason: "no account has room"}},
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}, Sessions: 1},
			{ID: "personal", Label: "Personal", Error: personalMissing},
			{ID: "side", Label: "Side", TokenSet: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Status() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestClientPin(t *testing.T) {
	tests := []struct {
		name string
		pin  router.PinRequest
		want status.Pin
	}{
		{
			name: "one account, moving running sessions",
			pin:  router.PinRequest{Accounts: []string{"side"}, Move: true},
			want: status.Pin{Accounts: []string{"side"}, Since: now, Move: true},
		},
		{
			name: "several, once each, in the order configured",
			pin:  router.PinRequest{Accounts: []string{"side", "work", "side"}},
			want: status.Pin{Accounts: []string{"work", "side"}, Since: now},
		},
		{
			name: "one, as a switchboard from before pins named several asks",
			pin:  router.PinRequest{Account: "side"},
			want: status.Pin{Accounts: []string{"side"}, Since: now},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newRouter(t, "http://127.0.0.1:1")
			client := router.NewClient(serveControl(t, rt))

			doc, err := client.Pin(t.Context(), tt.pin)
			if err != nil || !reflect.DeepEqual(doc.Pin, tt.want) || len(doc.Accounts) != 3 {
				t.Errorf("Pin() = %+v, %v, want the status document, pinned %+v", doc, err, tt.want)
			}
			if got := rt.Status().Pin; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("once pinned, the router's pin = %+v, want %+v", got, tt.want)
			}
			doc, err = client.Unpin(t.Context(), false)
			if err != nil || !doc.Pin.IsZero() || len(doc.Accounts) != 3 {
				t.Errorf("Unpin() = %+v, %v, want the status document, without a pin", doc, err)
			}
			if _, err := client.Unpin(t.Context(), false); err != nil {
				t.Errorf("Unpin() without a pin: %v, want nil", err)
			}
		})
	}
}

func TestClientPinNamesOneAccountAsARouterFromBeforePinsNamedSeveralReadsIt(t *testing.T) {
	tests := []struct {
		name     string
		accounts []string
		want     map[string]any
	}{
		{name: "one account", accounts: []string{"side"}, want: map[string]any{"accounts": []any{"side"}, "account": "side", "move": false, "force": false}},
		{name: "several", accounts: []string{"work", "side"}, want: map[string]any{"accounts": []any{"work", "side"}, "move": false, "force": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodies := make(chan map[string]any, 1)
			path := filepath.Join(shortTempDir(t), "control.sock")
			serveOn(t, path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode the pin: %v", err)
				}
				bodies <- body
				_, _ = io.WriteString(w, "{}")
			}))

			if _, err := router.NewClient(path).Pin(t.Context(), router.PinRequest{Accounts: tt.accounts}); err != nil {
				t.Fatalf("Pin() error = %v", err)
			}
			if got := <-bodies; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("POST /pin was sent %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPinningTakesAccountBesideAccounts(t *testing.T) {
	tests := []struct {
		body string
		want []string
	}{
		{body: `{"accounts": ["work"], "account": "work"}`, want: []string{"work"}},
		{body: `{"accounts": ["side"], "account": "work"}`, want: []string{"work", "side"}},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			rt := newRouter(t, "http://127.0.0.1:1")
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pin", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			rt.Control().ServeHTTP(rec, req)
			if got := rt.Status().Pin.Accounts; rec.Code != http.StatusOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("POST /pin %s answered %d, pinning %q, want 200, pinning %q, each once in the order configured", tt.body, rec.Code, got, tt.want)
			}
		})
	}
}

func TestPinningReplacesThePin(t *testing.T) {
	rt := newRouter(t, "http://127.0.0.1:1")
	client := router.NewClient(serveControl(t, rt))
	if _, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"work", "side"}, Move: true}); err != nil {
		t.Fatalf("Pin() error = %v", err)
	}

	doc, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"side"}})
	if want := (status.Pin{Accounts: []string{"side"}, Since: now}); err != nil || !reflect.DeepEqual(doc.Pin, want) {
		t.Errorf("Pin() = pin %+v, %v, want %+v alone", doc.Pin, err, want)
	}
}

func TestClientPinRefusesAnAccountNothingCanGoOutOn(t *testing.T) {
	tests := []struct {
		name string
		// tokens are what the accounts' token files hold, work's and side's
		// tokens unless given.
		tokens   tokenstest.Files
		accounts []string
		wantErr  string
	}{
		{
			name:     "an account there's none of",
			accounts: []string{"nope"},
			wantErr:  `there's no account "nope": pin work or side`,
		},
		{
			name:     "an account there's none of, among those there are",
			accounts: []string{"work", "nope", "side"},
			wantErr:  `there's no account "nope": pin work or side`,
		},
		{
			name:     "an account there's none of, with no account to pin",
			tokens:   tokenstest.Files{},
			accounts: []string{"nope"},
			wantErr:  `there's no account "nope", and no account has a usable token to pin`,
		},
		{
			name:     "an account without a usable token",
			accounts: []string{"personal"},
			wantErr:  "account personal has no usable token, so nothing can go out on it: " + personalMissing,
		},
		{
			name:     "an account without a usable token, among those with one",
			accounts: []string{"side", "personal"},
			wantErr:  "account personal has no usable token, so nothing can go out on it: " + personalMissing,
		},
		{
			name:     "an account without a usable token, with no account to pin",
			tokens:   tokenstest.Files{},
			accounts: []string{"work"},
			wantErr:  "account work has no usable token, so nothing can go out on it: " + tokenstest.Missing("work").Error(),
		},
		{
			name:    "no account",
			wantErr: `give the accounts to pin, such as {"accounts": ["work"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig("http://127.0.0.1:1")
			if tt.tokens != nil {
				cfg.Token = tt.tokens.Read
			}
			rt := newRouterFrom(t, cfg)
			client := router.NewClient(serveControl(t, rt))

			if _, err := client.Pin(t.Context(), router.PinRequest{Accounts: tt.accounts}); err == nil || err.Error() != tt.wantErr {
				t.Errorf("Pin() error = %v, want %q", err, tt.wantErr)
			}
			if got := rt.Status().Pin; !got.IsZero() {
				t.Errorf("the router's pin = %+v, want none", got)
			}
		})
	}
}

func TestPinningTakesWhatItAsksFor(t *testing.T) {
	tests := []struct {
		method, path, body string
		wantErr            string
	}{
		{
			method:  http.MethodPost,
			path:    "/pin",
			body:    "side",
			wantErr: `give the accounts to pin as JSON, such as {"accounts": ["work"], "move": false, "force": false}`,
		},
		{
			method:  http.MethodPost,
			path:    "/sessions/" + sessionID + "/pin",
			body:    "side",
			wantErr: `give the account to pin the session to as JSON, such as {"account": "work"}`,
		},
		{
			method:  http.MethodDelete,
			path:    "/pin?force=soon",
			wantErr: `force is true or false, not "soon"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rt := newRouter(t, "http://127.0.0.1:1")
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			rt.Control().ServeHTTP(rec, req)
			want, _ := json.Marshal(map[string]string{"error": tt.wantErr})
			if got := rec.Body.String(); rec.Code != http.StatusBadRequest || rec.Header().Get("Content-Type") != "application/json" || got != string(want)+"\n" {
				t.Errorf("%s %s answered %d (%s) %s, want 400 (application/json) %s", tt.method, tt.path, rec.Code, rec.Header().Get("Content-Type"), got, want)
			}
		})
	}
}

func TestClientRefresh(t *testing.T) {
	cfg := testConfig("http://127.0.0.1:1")
	prober := readingEvery(session, week)
	cfg.Prober = prober
	client := router.NewClient(serveControl(t, newRouterFrom(t, cfg)))

	got, err := client.Refresh(t.Context(), 30*time.Minute)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	want := status.Document{
		GeneratedAt: now,
		Source:      "router",
		Best:        "work",
		Router:      status.Health{Healthy: true},
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}},
			{ID: "personal", Label: "Personal", Error: personalMissing},
			{ID: "side", Label: "Side", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Refresh() =\n%+v\nwant the document with what the probes read\n%+v", got, want)
	}
	if probed, want := prober.probed(), []string{sideToken, workToken}; !reflect.DeepEqual(probed, want) {
		t.Errorf("probed %q, want %q", probed, want)
	}
}

func TestRefreshTakesHowOldUsageCanBe(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "not JSON", body: "30m", wantErr: `give how old usage can be as JSON, such as {"max_age": "30m"}`},
		{name: "a number", body: `{"max_age": 30}`, wantErr: `give how old usage can be as JSON, such as {"max_age": "30m"}`},
		{name: "without it", body: `{}`, wantErr: `give how old usage can be, such as {"max_age": "30m"}`},
		{name: "not a duration", body: `{"max_age": "soon"}`, wantErr: `max_age "soon" isn't a duration, such as 30m`},
		{name: "less than nothing", body: `{"max_age": "-5m"}`, wantErr: "max_age -5m is less than nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig("http://127.0.0.1:1")
			prober := &fakeProber{}
			cfg.Prober = prober
			rt := newRouterFrom(t, cfg)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/refresh", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			rt.Control().ServeHTTP(rec, req)
			want, _ := json.Marshal(map[string]string{"error": tt.wantErr})
			if got := rec.Body.String(); rec.Code != http.StatusBadRequest || rec.Header().Get("Content-Type") != "application/json" || got != string(want)+"\n" {
				t.Errorf("POST /refresh %s answered %d (%s) %s, want 400 (application/json) %s", tt.body, rec.Code, rec.Header().Get("Content-Type"), got, want)
			}
			if probed := prober.probed(); len(probed) > 0 {
				t.Errorf("probed %q, want nothing", probed)
			}
		})
	}
}

func TestClientSession(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, week)
	r.readsAs(sideToken, session, week)
	client := router.NewClient(serveControl(t, r.rt))
	r.ask(t, sessionID, haiku, "side")
	r.clock.advance(time.Minute)
	r.ask(t, sessionID, opus, "work")

	got, err := client.Session(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("Session() error = %v", err)
	}
	later := now.Add(time.Minute)
	want := status.Session{
		ID:  sessionID,
		Pin: "work",
		Assignments: []status.Assignment{
			{Model: opus, Family: "opus", Account: "work", Pinned: true, Reason: "pinned", AssignedAt: later, LastSeen: later},
			{Model: haiku, Family: "haiku", Account: "side", Pinned: true, Reason: "pinned", AssignedAt: now, LastSeen: now},
		},
		Account: status.Account{ID: "work", Label: "Work", TokenSet: true, FetchedAt: later, Windows: []quota.Window{session, week}, Sessions: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Session() =\n%+v\nwant, the pin its last request carried, and the account of the model used last,\n%+v", got, want)
	}
}

func TestClientSessionOfASessionNeverSeen(t *testing.T) {
	client := router.NewClient(serveControl(t, newRouter(t, "http://127.0.0.1:1")))

	_, err := client.Session(t.Context(), "0b5c6f2e/../nope")
	if want := "the router hasn't seen session 0b5c6f2e/../nope"; !errors.Is(err, router.ErrUnknownSession) || err.Error() != want {
		t.Errorf("Session() error = %v, want ErrUnknownSession, reading %q", err, want)
	}
}

func TestClientSessionOfASocketThatIsntARouters(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "other.sock")
	serveOn(t, path, http.NotFoundHandler())

	_, err := router.NewClient(path).Session(t.Context(), "nope")
	if want := "the router answered GET /sessions/nope with 404 Not Found"; err == nil || err.Error() != want || errors.Is(err, router.ErrUnknownSession) {
		t.Errorf("Session() error = %v, want %q, and not ErrUnknownSession: something else answers", err, want)
	}
}

func TestClientSessions(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, week)
	r.readsAs(sideToken, session, week)
	client := router.NewClient(serveControl(t, r.rt))
	if got, err := client.Sessions(t.Context()); err != nil || got == nil || len(got) > 0 {
		t.Errorf("Sessions() with none routed = %#v, %v, want an empty list", got, err)
	}
	r.ask(t, "idle", opus, "")
	r.clock.advance(30 * time.Minute)
	r.ask(t, "earlier", opus, "")
	r.clock.advance(31 * time.Minute)
	r.ask(t, "latest", haiku, "side")

	got, err := client.Sessions(t.Context())
	if err != nil {
		t.Fatalf("Sessions() error = %v", err)
	}
	ids := make([]string, len(got))
	for i, s := range got {
		ids[i] = s.ID
	}
	if want := []string{"latest", "earlier"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("Sessions() lists %q, want %q: those routed in the last hour, the one seen last first", ids, want)
	}
	latest := r.clock.read()
	want := status.Session{
		ID:          "latest",
		Pin:         "side",
		Assignments: []status.Assignment{{Model: haiku, Family: "haiku", Account: "side", Pinned: true, Reason: "pinned", AssignedAt: latest, LastSeen: latest}},
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("Sessions() lists\n%+v\nwant it as Session() gives it, but for its account\n%+v", got[0], want)
	}

	rec := httptest.NewRecorder()
	r.rt.Control().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sessions", nil))
	var listed []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed) != 2 {
		t.Fatalf("GET /sessions answered %s (%v), want two sessions", rec.Body, err)
	}
	for _, s := range listed {
		if account, ok := s["account"]; ok {
			t.Errorf("GET /sessions lists a session with account %s, want none", account)
		}
	}
}

func TestClientPinSession(t *testing.T) {
	r := newRouted(t)
	r.readsAs(workToken, session, week)
	r.readsAs(sideToken, session, week)
	client := router.NewClient(serveControl(t, r.rt))
	r.ask(t, sessionID, opus, "work")

	got, err := client.PinSession(t.Context(), sessionID, "side")
	want := status.Session{
		ID:          sessionID,
		Pin:         "side",
		Assignments: []status.Assignment{{Model: opus, Family: "opus", Account: "work", Pinned: true, PinnedAt: now, Reason: "pinned", AssignedAt: now, LastSeen: now}},
		Account:     status.Account{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}, Sessions: 1},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("PinSession() =\n%+v, %v\nwant the session as it stands, pinned to side, its next request yet to move it\n%+v", got, err, want)
	}

	got, err = client.UnpinSession(t.Context(), sessionID)
	want.Pin, want.Assignments[0].PinnedAt = "", time.Time{}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("UnpinSession() =\n%+v, %v\nwant the session as it stands, with no pin of its own\n%+v", got, err, want)
	}
}

func TestClientPinSessionRefuses(t *testing.T) {
	tests := []struct {
		name    string
		pin     func(ctx context.Context, c *router.Client) error
		wantErr string
		// wantUnknown is set when the router hasn't seen the session.
		wantUnknown bool
	}{
		{
			name: "a session never seen, pinned",
			pin: func(ctx context.Context, c *router.Client) error {
				_, err := c.PinSession(ctx, "nope", "side")
				return err
			},
			wantErr:     "the router hasn't seen session nope",
			wantUnknown: true,
		},
		{
			name: "a session never seen, unpinned",
			pin: func(ctx context.Context, c *router.Client) error {
				_, err := c.UnpinSession(ctx, "nope")
				return err
			},
			wantErr:     "the router hasn't seen session nope",
			wantUnknown: true,
		},
		{
			name: "an account there's none of",
			pin: func(ctx context.Context, c *router.Client) error {
				_, err := c.PinSession(ctx, sessionID, "nope")
				return err
			},
			wantErr: `there's no account "nope": pin work or side`,
		},
		{
			name: "an account without a usable token",
			pin: func(ctx context.Context, c *router.Client) error {
				_, err := c.PinSession(ctx, sessionID, "personal")
				return err
			},
			wantErr: "account personal has no usable token, so nothing can go out on it: " + personalMissing,
		},
		{
			name: "no account",
			pin: func(ctx context.Context, c *router.Client) error {
				_, err := c.PinSession(ctx, sessionID, "")
				return err
			},
			wantErr: `give the account to pin, such as {"account": "work"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRouted(t)
			r.readsAs(workToken, session, week)
			r.readsAs(sideToken, session, week)
			client := router.NewClient(serveControl(t, r.rt))
			r.ask(t, sessionID, opus, "")

			err := tt.pin(t.Context(), client)
			if err == nil || err.Error() != tt.wantErr || errors.Is(err, router.ErrUnknownSession) != tt.wantUnknown {
				t.Errorf("error = %v, want %q, ErrUnknownSession %v", err, tt.wantErr, tt.wantUnknown)
			}
			if got, err := client.Session(t.Context(), sessionID); err != nil || got.Pin != "" {
				t.Errorf("the session is %+v (%v), want it without a pin, as it was", got, err)
			}
		})
	}
}

func TestClientHistory(t *testing.T) {
	up := newUpstream(t, answerWith(http.StatusOK, session, week))
	rt := newRouter(t, up.URL)
	client := router.NewClient(serveControl(t, rt))
	readAll(t, send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
	waitUntil(t, "the request is done", func() bool { return rt.Status().Router.Requests == 1 })

	got, err := client.History(t.Context(), "5h", time.Minute)
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	// Built without Run, the router keeps no history's files, and gives work's
	// session as it read it now, two minutes into its window.
	want := router.History{Window: "5h", Step: "1m0s", Accounts: []router.AccountHistory{
		{ID: "work", Start: session.ResetsAt.Add(-5 * time.Hour), Points: []router.HistoryPoint{{At: now, Utilization: session.Utilization}}},
		{ID: "personal"},
		{ID: "side"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("History() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestClientHistoryRefused(t *testing.T) {
	client := router.NewClient(serveControl(t, newRouter(t, "http://127.0.0.1:1")))

	_, err := client.History(t.Context(), "week", 5*time.Minute)
	if want := `window "week" has no length to read: give one such as 5h or 7d`; err == nil || err.Error() != want || errors.Is(err, router.ErrNoHistory) {
		t.Errorf("History() error = %v, want the router's reason, %q", err, want)
	}
}

func TestClientHistoryOfARouterFromBeforeItGaveOne(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"ok": true}`) })
	serveOn(t, path, mux)

	if _, err := router.NewClient(path).History(t.Context(), "5h", 5*time.Minute); !errors.Is(err, router.ErrNoHistory) {
		t.Errorf("History() error = %v, want ErrNoHistory", err)
	}
}

func TestClientWithoutARouter(t *testing.T) {
	dir := shortTempDir(t)
	stale := filepath.Join(dir, "stale.sock")
	ln, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = ln.Close()
	tests := []struct {
		name string
		path string
	}{
		{name: "no socket", path: filepath.Join(dir, "none.sock")},
		{name: "a socket nothing listens on", path: stale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := router.NewClient(tt.path)
			if _, err := client.Health(t.Context()); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Health() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Status(t.Context()); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Status() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Session(t.Context(), sessionID); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Session() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Sessions(t.Context()); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Sessions() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.PinSession(t.Context(), sessionID, "side"); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("PinSession() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.UnpinSession(t.Context(), sessionID); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("UnpinSession() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"side"}}); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Pin() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Unpin(t.Context(), false); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Unpin() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Refresh(t.Context(), time.Minute); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Refresh() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.History(t.Context(), "5h", 5*time.Minute); !errors.Is(err, router.ErrNotRunning) || errors.Is(err, router.ErrNoHistory) {
				t.Errorf("History() error = %v, want ErrNotRunning, and not ErrNoHistory", err)
			}
			if _, err := client.Stream(t.Context()); !errors.Is(err, router.ErrNotRunning) || errors.Is(err, router.ErrNoStream) {
				t.Errorf("Stream() error = %v, want ErrNotRunning, and not ErrNoStream", err)
			}
		})
	}
}

func TestClientOfASocketThatIsntARouters(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "other.sock")
	serveOn(t, path, http.NotFoundHandler())

	_, err := router.NewClient(path).Health(t.Context())
	if err == nil || errors.Is(err, router.ErrNotRunning) || !strings.Contains(err.Error(), "404") {
		t.Errorf("Health() error = %v, want one saying what answered, and not ErrNotRunning", err)
	}
}

// serveControl serves the router's control API on a socket in a directory of
// the test's own until the test ends, and returns the socket's path.
func serveControl(t *testing.T, rt *router.Router) string {
	t.Helper()
	path := filepath.Join(shortTempDir(t), "control.sock")
	serveOn(t, path, rt.Control())
	return path
}

// serveOn serves h on a unix socket at path until the test ends.
func serveOn(t *testing.T, path string, h http.Handler) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

package router_test

import (
	"context"
	"encoding/json"
	"errors"
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
)

func TestClientHealth(t *testing.T) {
	client := router.NewClient(serveControl(t, newRouter(t, "http://127.0.0.1:1")))

	got, err := client.Health(t.Context())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if want := (router.Health{OK: true, Version: "1.2.3", PID: os.Getpid(), StartedAt: now}); got != want {
		t.Errorf("Health() = %+v, want %+v", got, want)
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
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}, Sessions: 1},
			{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
			{ID: "side", Label: "Side", TokenSet: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Status() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestClientPin(t *testing.T) {
	rt := newRouter(t, "http://127.0.0.1:1")
	client := router.NewClient(serveControl(t, rt))

	doc, err := client.Pin(t.Context(), "side", true)
	want := status.Pin{Account: "side", Since: now, Move: true}
	if err != nil || doc.Pin != want || len(doc.Accounts) != 3 {
		t.Errorf("Pin() = %+v, %v, want the status document, pinned %+v", doc, err, want)
	}
	if got := rt.Status().Pin; got != want {
		t.Errorf("once pinned, the router's pin = %+v, want %+v", got, want)
	}
	doc, err = client.Unpin(t.Context())
	if err != nil || doc.Pin != (status.Pin{}) || len(doc.Accounts) != 3 {
		t.Errorf("Unpin() = %+v, %v, want the status document, without a pin", doc, err)
	}
	if _, err := client.Unpin(t.Context()); err != nil {
		t.Errorf("Unpin() without a pin: %v, want nil", err)
	}
}

func TestClientPinRefusesAnAccountNothingCanGoOutOn(t *testing.T) {
	tests := []struct {
		account string
		wantErr string
	}{
		{account: "nope", wantErr: `there's no account "nope": pin work or side`},
		{account: "personal", wantErr: "account personal has no token, so nothing can go out on it: set CLAUDE_TOKEN_PERSONAL"},
		{account: "", wantErr: `give the account to pin, such as {"account": "work"}`},
	}
	for _, tt := range tests {
		t.Run(tt.account, func(t *testing.T) {
			rt := newRouter(t, "http://127.0.0.1:1")
			client := router.NewClient(serveControl(t, rt))

			if _, err := client.Pin(t.Context(), tt.account, false); err == nil || err.Error() != tt.wantErr {
				t.Errorf("Pin() error = %v, want %q", err, tt.wantErr)
			}
			if got := rt.Status().Pin; got != (status.Pin{}) {
				t.Errorf("the router's pin = %+v, want none", got)
			}
		})
	}
}

func TestPinTakesJSON(t *testing.T) {
	path := serveControl(t, newRouter(t, "http://127.0.0.1:1"))
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		},
	}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://switchboard/pin", strings.NewReader("side"))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	want := `{"error":"give the account to pin as JSON, such as {\"account\": \"work\", \"move\": false}"}` + "\n"
	if body := readAll(t, resp); resp.StatusCode != http.StatusBadRequest || resp.Header.Get("Content-Type") != "application/json" || body != want {
		t.Errorf("POST /pin with a body that isn't JSON answered %d (%s) %s, want 400 (application/json) %s", resp.StatusCode, resp.Header.Get("Content-Type"), body, want)
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
			{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
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
	want := router.Session{
		ID: sessionID,
		Assignments: []router.Assignment{
			{Model: opus, Account: "work", Pinned: true, Reason: "pinned", AssignedAt: later, LastSeen: later},
			{Model: haiku, Account: "side", Pinned: true, Reason: "pinned", AssignedAt: now, LastSeen: now},
		},
		Account: status.Account{ID: "work", Label: "Work", TokenSet: true, FetchedAt: later, Windows: []quota.Window{session, week}, Sessions: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Session() =\n%+v\nwant, the account of the model used last,\n%+v", got, want)
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
			if _, err := client.Pin(t.Context(), "side", false); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Pin() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Unpin(t.Context()); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Unpin() error = %v, want ErrNotRunning", err)
			}
			if _, err := client.Refresh(t.Context(), time.Minute); !errors.Is(err, router.ErrNotRunning) {
				t.Errorf("Refresh() error = %v, want ErrNotRunning", err)
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

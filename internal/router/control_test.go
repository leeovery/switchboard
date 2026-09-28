package router_test

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
	send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))

	got, err := client.Status(t.Context())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	want := status.Document{
		GeneratedAt: now,
		Source:      "router",
		Best:        "work",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}},
			{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
			{ID: "side", Label: "Side", TokenSet: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Status() =\n%+v\nwant\n%+v", got, want)
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

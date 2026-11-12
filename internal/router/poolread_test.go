package router_test

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// askedQueries is a router's control API that answers every question with
// an answer of nothing, and remembers the query each was asked with, by its
// method and path.
type askedQueries struct {
	mu    sync.Mutex
	asked map[string]string
}

func (a *askedQueries) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	a.mu.Lock()
	a.asked[req.Method+" "+req.URL.Path] = req.URL.RawQuery
	a.mu.Unlock()
	switch req.URL.Path {
	case "/sessions":
		_, _ = io.WriteString(w, "[]")
	case "/stream":
	default:
		_, _ = io.WriteString(w, `{"session": "one", "assignments": []}`)
	}
}

// queries returns the query each question was asked with, by its method and
// path.
func (a *askedQueries) queries() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked
}

// askEveryRead has client ask every question of a pool, and the questions of
// one session.
func askEveryRead(t *testing.T, client *router.Client) {
	t.Helper()
	ctx := t.Context()
	for _, ask := range []func() error{
		func() error { _, err := client.Status(ctx); return err },
		func() error { _, err := client.Sessions(ctx); return err },
		func() error { _, err := client.Session(ctx, "one"); return err },
		func() error { _, err := client.History(ctx, "7d", time.Hour); return err },
		func() error { _, err := client.Refresh(ctx, time.Minute); return err },
		func() error { _, err := client.Pin(ctx, router.PinRequest{Accounts: []string{"work"}}); return err },
		func() error { _, err := client.Unpin(ctx, true, router.ByCLI); return err },
		func() error { _, err := client.UnpinSession(ctx, "one", router.ByCLI); return err },
		func() error {
			streamed, cancel := context.WithCancel(ctx)
			defer cancel()
			events, err := client.Stream(streamed)
			for range events {
			}
			return err
		},
	} {
		if err := ask(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAClientInAPoolAsksForItWhereverItReadsAPool(t *testing.T) {
	tests := []struct {
		name string
		pool string
		want map[string]string
	}{
		{
			name: "in a pool",
			pool: "q7mt",
			want: map[string]string{
				"GET /status":              "pool=q7mt",
				"GET /sessions":            "pool=q7mt",
				"GET /sessions/one":        "pool=q7mt",
				"GET /history":             "pool=q7mt&step=1h0m0s&window=7d",
				"POST /refresh":            "pool=q7mt",
				"POST /pin":                "pool=q7mt",
				"DELETE /pin":              "by=cli&force=true&pool=q7mt",
				"DELETE /sessions/one/pin": "by=cli",
				"GET /stream":              "pool=q7mt",
			},
		},
		{
			name: "in none",
			want: map[string]string{
				"GET /status":              "",
				"GET /sessions":            "",
				"GET /sessions/one":        "",
				"GET /history":             "step=1h0m0s&window=7d",
				"POST /refresh":            "",
				"POST /pin":                "",
				"DELETE /pin":              "by=cli&force=true",
				"DELETE /sessions/one/pin": "by=cli",
				"GET /stream":              "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(shortTempDir(t), "control.sock")
			asked := &askedQueries{asked: make(map[string]string)}
			serveOn(t, path, asked)

			askEveryRead(t, router.NewClient(path).InPool(tt.pool))
			if got := asked.queries(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the client asked with the queries\n%v\nwant\n%v", got, tt.want)
			}
		})
	}
}

func TestClientHealthReportsThePoolsTheRouterRoutesFor(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok": true, "version": "0.8.0", "reports_pools": true, "pools": [`+
			`{"id": "q7mt", "label": "Acme", "dirs": ["/work/acme"], "written_dirs": ["~/acme"], "accounts": ["personal"], "primary": "personal"}]}`)
	})
	serveOn(t, path, mux)

	got, err := router.NewClient(path).Health(t.Context())
	want := router.Health{OK: true, Version: "0.8.0", ReportsPools: true, Pools: []router.RoutedPool{
		{ID: "q7mt", Label: "Acme", Dirs: []string{"/work/acme"}, WrittenDirs: []string{"~/acme"}, Accounts: []string{"personal"}, Primary: "personal"},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Health() = %+v, %v; want %+v", got, err, want)
	}
}

func TestClientHealthOfARouterFromBeforePoolsSaysItReportsNone(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok": true, "version": "0.7.0", "strips_own_headers": true}`)
	})
	serveOn(t, path, mux)

	got, err := router.NewClient(path).Health(t.Context())
	if err != nil || got.ReportsPools || got.Pools != nil {
		t.Errorf("Health() = %+v, %v, want it not to report pools, and none", got, err)
	}
}

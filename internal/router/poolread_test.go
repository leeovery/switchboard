package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
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

// pooledControl serves a router's control API on a socket until the test
// ends, that reports the pools given in its health answer, "" for none, as
// one from before pools reports none, and answers GET /sessions of each pool
// with a session of its own, last seen as the pool's place says, and GET
// /status with its document, and returns the socket's path.
func pooledControl(t *testing.T, pools ...string) string {
	t.Helper()
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		h := router.Health{OK: true, ReportsPools: true, Moving: status.Moving{Stage: status.MoveRunning}}
		for _, pool := range pools {
			h.Pools = append(h.Pools, router.RoutedPool{ID: pool, Label: pool, Accounts: []string{}})
		}
		_ = json.NewEncoder(w).Encode(h)
	})
	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, req *http.Request) {
		pool := req.URL.Query().Get(router.PoolQuery)
		seen := now.Add(time.Duration(slices.Index(pools, pool)) * time.Minute)
		_ = json.NewEncoder(w).Encode([]status.Session{{ID: "in-" + pool, Assignments: []status.Assignment{{Account: "work", LastSeen: seen}}}})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(status.Document{PoolID: req.URL.Query().Get(router.PoolQuery)})
	})
	serveOn(t, path, mux)
	return path
}

func TestAClientReadsEveryPoolTheRouterReports(t *testing.T) {
	client := router.NewClient(pooledControl(t, "b2n8", "q7mt")).InPool("z5k2")

	sessions, err := client.SessionsInEveryPool(t.Context())
	var ids []string
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	if want := []string{"in-q7mt", "in-b2n8"}; err != nil || !slices.Equal(ids, want) {
		t.Errorf("SessionsInEveryPool() = %q, %v; want %q, the one seen last first", ids, err, want)
	}
	docs, err := client.StatusInEveryPool(t.Context())
	var of []string
	for _, doc := range docs {
		of = append(of, doc.PoolID)
	}
	if want := []string{"b2n8", "q7mt"}; err != nil || !slices.Equal(of, want) {
		t.Errorf("StatusInEveryPool() is of %q, %v; want %q", of, err, want)
	}
}

func TestAClientOfARouterReportingNoPoolsReadsTheOneThereIs(t *testing.T) {
	client := router.NewClient(pooledControl(t))
	sessions, err := client.SessionsInEveryPool(t.Context())
	if err != nil || len(sessions) != 1 || sessions[0].ID != "in-" {
		t.Errorf("SessionsInEveryPool() = %+v, %v; want the one pool's, asked naming none", sessions, err)
	}
}

func TestAClientTellsTheMoveFromTheHealthAnswer(t *testing.T) {
	moving, err := router.NewClient(pooledControl(t, "b2n8")).Moving(t.Context())
	if err != nil || moving.Stage != status.MoveRunning {
		t.Errorf("Moving() = %+v, %v; want the health answer's", moving, err)
	}
}

func TestAClientTellsTheMoveFromTheDocumentOfARouterFromBeforePools(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok": true}`)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"moving": {"stage": "running"}, "accounts": []}`)
	})
	serveOn(t, path, mux)

	moving, err := router.NewClient(path).Moving(t.Context())
	if err != nil || moving.Stage != status.MoveRunning {
		t.Errorf("Moving() = %+v, %v; want the document's", moving, err)
	}
}

func TestAReadRefusedForWantOfAPoolIsPoolNeeded(t *testing.T) {
	rt := newRouterFrom(t, pooledConfig(now))
	client := router.NewClient(serveControl(t, rt))

	_, err := client.Status(t.Context())
	need, ok := errors.AsType[router.PoolNeeded](err)
	if want := []router.PoolNamed{{ID: "b2n8", Label: "Default"}, {ID: "q7mt", Label: "Acme"}}; !ok || !reflect.DeepEqual(need.Pools, want) || need.Named != "" {
		t.Errorf("Status() error = %#v, want PoolNeeded naming %+v", err, want)
	}
	if router.IsNotRunning(err) {
		t.Errorf("Status() error = %v, which says the router isn't running", err)
	}
	if _, err := client.InPool("z5k2").Sessions(t.Context()); !isNeedFor(err, "z5k2") {
		t.Errorf("Sessions() of a pool not configured error = %#v, want PoolNeeded naming it", err)
	}
}

// isNeedFor reports whether err is a PoolNeeded of the pool named given.
func isNeedFor(err error, named string) bool {
	need, ok := errors.AsType[router.PoolNeeded](err)
	return ok && need.Named == named
}

// pooledConfig is testConfig with work in Default and personal and side in
// Acme, both fenced.
func pooledConfig(now time.Time) router.Config {
	cfg := testConfig("http://127.0.0.1:1")
	cfg.Now = func() time.Time { return now }
	cfg.Pools = config.Pools{
		{ID: "b2n8", Label: "Default", Dirs: []string{"/work/default"}},
		{ID: "q7mt", Label: "Acme", Dirs: []string{"/work/acme"}},
	}
	cfg.Accounts[0].Pool = "b2n8"
	cfg.Accounts[1].Pool, cfg.Accounts[2].Pool = "q7mt", "q7mt"
	return cfg
}

package cli

import (
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestASourceInAPoolReadsTheRoutersDocumentSessionsHistoryAndStreamOfIt(t *testing.T) {
	var mu sync.Mutex
	asked := make(map[string]string)
	client := serveControl(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		asked[req.URL.Path] = req.URL.Query().Get(router.PoolQuery)
		mu.Unlock()
		_, _ = io.WriteString(w, map[string]string{"/sessions": "[]", "/history": "{}", "/status": "{}", "/health": `{"ok": true, "reports_pools": true}`}[req.URL.Path])
	}))

	source := usageSource{ask: true, router: client}.inPool("q7mt")
	if _, _, err := source.Read(t.Context(), status.Read{}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Sessions(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := source.History(t.Context(), "7d", time.Hour); err != nil {
		t.Fatal(err)
	}
	events, err := source.Stream(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	mu.Lock()
	defer mu.Unlock()
	want := map[string]string{"/health": "", "/status": "q7mt", "/sessions": "q7mt", "/history": "q7mt", "/stream": "q7mt"}
	if !maps.Equal(asked, want) {
		t.Errorf("the source asked for the pools %v, by path, want %v", asked, want)
	}
}

func TestASourceWithoutTheRouterHasNoneInAPool(t *testing.T) {
	if source := (usageSource{}).inPool("q7mt"); source.router != nil {
		t.Errorf("inPool() gave a source without the router %+v, want none", source.router)
	}
}

// serveControl serves handler as a router's control API on a socket until
// the test ends, and returns a client of it.
func serveControl(t *testing.T, handler http.Handler) *router.Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ln, err := net.Listen("unix", filepath.Join(dir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return router.NewClient(ln.Addr().String())
}

// A router refusing for want of a pool is never read as one down: the read
// fails with the refusal, giving the router's health as it answered, healthy,
// with the restart it has due.
func TestARouterRefusingForWantOfAPoolIsNeverReadAsOneDown(t *testing.T) {
	refusing := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/health" {
			_, _ = io.WriteString(w, `{"ok": true, "pid": 41, "reports_pools": true, "restart": {"reason": "config changed", "in_flight": 1}}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error": "every pool is fenced", "pools": [{"id": "b2n8", "label": "Default"}, {"id": "q7mt", "label": "Acme"}]}`)
	})
	// A probe would fail the test: the source has nothing to probe with.
	source := usageSource{ask: true, router: serveControl(t, refusing)}

	_, health, err := source.Read(t.Context(), status.Read{Probe: true})
	if need, ok := errors.AsType[router.PoolNeeded](err); !ok || len(need.Pools) != 2 {
		t.Errorf("Read() error = %v, want PoolNeeded naming both pools", err)
	}
	if want := (status.Restart{Reason: status.RestartForConfig, InFlight: 1}); !health.OK || health.PID != 41 || health.Restart != want {
		t.Errorf("Read() gave the router's health %+v, want it as it answered: ok, pid 41, the restart %+v due", health, want)
	}
}

// pooledProbe is a probe of work in Default, unfenced, and personal and side
// in Acme, fenced, all fenced where fenced is set.
func pooledProbe(fenced bool) probeSource {
	pools := config.Pools{{ID: "b2n8", Label: "Default"}, {ID: "q7mt", Label: "Acme", Dirs: []string{"/work/acme"}}}
	if fenced {
		pools[0].Dirs = []string{"/work/default"}
	}
	return probeOf(&config.Config{Accounts: config.Accounts{{ID: "work", Pool: "b2n8"}, {ID: "personal", Pool: "q7mt"}, {ID: "side", Pool: "q7mt"}}, Pools: pools})
}

// probeOf is a probe of the config given, as its pools stand.
func probeOf(cfg *config.Config) probeSource {
	return probeSource{pooled: func() (*config.Config, error) { return cfg, nil }}
}

func TestAProbeInPlaceOfTheRouterReadsOnePoolsAccounts(t *testing.T) {
	tests := []struct {
		name   string
		probe  probeSource
		named  string
		want   []string
		needed bool
	}{
		{name: "no pools: every account", probe: probeOf(&config.Config{Accounts: config.Accounts{{ID: "work"}, {ID: "side"}}}), want: []string{"work", "side"}},
		{name: "none named: the unfenced pool's", probe: pooledProbe(false), want: []string{"work"}},
		{name: "one named", probe: pooledProbe(false), named: "q7mt", want: []string{"personal", "side"}},
		{name: "every pool fenced, none named", probe: pooledProbe(true), needed: true},
		{name: "one not configured named", probe: pooledProbe(false), named: "z5k2", needed: true},
	}
	untold := errors.New("the pools can't be told")
	if _, err := (probeSource{pooled: func() (*config.Config, error) { return nil, untold }}).inPool(""); !errors.Is(err, untold) {
		t.Errorf("inPool() of pools that can't be told fails with %v, want %v, probing none", err, untold)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.probe.inPool(tt.named)
			if _, needs := errors.AsType[router.PoolNeeded](err); needs != tt.needed {
				t.Fatalf("inPool(%q) error = %v, want a pool needed: %v", tt.named, err, tt.needed)
			}
			var ids []string
			for _, a := range got.accounts {
				ids = append(ids, a.ID)
			}
			if !tt.needed && !slices.Equal(ids, tt.want) {
				t.Errorf("inPool(%q) probes %q, want %q", tt.named, ids, tt.want)
			}
		})
	}
}

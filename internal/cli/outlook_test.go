package cli_test

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestStatusWorksOutWhatARouterFromBeforeDoesntGive(t *testing.T) {
	built := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	// As a router from before the status document gave each window's even
	// pace and allowance, the pool and what's coming up gives it.
	given := status.Document{
		GeneratedAt: built, Source: status.SourceRouter, Best: "side", Router: status.Health{Healthy: true},
		Accounts: []status.Account{
			{ID: "work", Label: "Work", Reserve: 0.05, TokenSet: true, FetchedAt: built, Windows: []quota.Window{
				{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: built.Add(3 * time.Hour)},
				{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: built.Add(4 * 24 * time.Hour)},
			}},
			{ID: "personal", Label: "Personal"},
			{ID: "side", Label: "Side", TokenSet: true, FetchedAt: built, Windows: []quota.Window{
				{Key: "5h", Label: "Session", Utilization: 0.2, ResetsAt: built.Add(time.Hour)},
				{Key: "7d", Label: "Week", Utilization: 0.1, ResetsAt: built.Add(6 * 24 * time.Hour)},
			}},
		},
	}
	want := given.WorkedOut(claude.Policy)
	if want.ComingUp == nil || want.Pool.Windows == nil {
		t.Fatalf("WorkedOut() = %+v, want what's coming up and the pool", want)
	}
	tests := []struct {
		name string
		doc  status.Document
	}{
		{name: "from a router from before, which gives none of it", doc: given},
		{name: "from a router that gives it, as it gave it", doc: want},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, "http://127.0.0.1:1", nil)
			serveDocument(t, srv.socket(), tt.doc)
			if got := statusJSON(t, srv.deps); !reflect.DeepEqual(got, want) {
				t.Errorf("switchboard status --json printed\n%+v\nwant the router's document with what's worked out of it\n%+v", got, want)
			}
		})
	}
}

// serveDocument answers health checks on the socket at path until the test
// ends, as a healthy router does, and gives doc as its status document.
func serveDocument(t *testing.T, path string, doc status.Document) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(router.Health{OK: true, Listen: "127.0.0.1:4747"})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(doc)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

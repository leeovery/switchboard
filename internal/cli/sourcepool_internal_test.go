package cli

import (
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestASourceInAPoolReadsTheRoutersDocumentSessionsHistoryAndStreamOfIt(t *testing.T) {
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ln, err := net.Listen("unix", filepath.Join(dir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	asked := make(map[string]string)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		asked[req.URL.Path] = req.URL.Query().Get(router.PoolQuery)
		mu.Unlock()
		_, _ = io.WriteString(w, map[string]string{"/sessions": "[]", "/history": "{}", "/status": "{}", "/health": `{"ok": true}`}[req.URL.Path])
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	source := usageSource{ask: true, router: router.NewClient(ln.Addr().String())}.inPool("q7mt")
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

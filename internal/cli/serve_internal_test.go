package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestTheRouterIsGivenTheFolderToExportInto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[[account]]\nid = \"work\"\n\n[export]\ndir = \"~/Dropbox/Switchboard\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &app{
		Getenv:  func(string) string { return "" },
		Environ: func() []string { return nil },
		HomeDir: func() (string, error) { return "/Users/test", nil },
	}

	cfg, _, err := a.startingFrom(t.TempDir(), path)
	if err != nil {
		t.Fatalf("startingFrom() error = %v", err)
	}
	if want := (config.Export{Dir: "~/Dropbox/Switchboard"}); cfg.Export != want {
		t.Errorf("the router exports as %+v, want %+v", cfg.Export, want)
	}
	if got, err := cfg.Export.Folder(cfg.HomeDir); err != nil || got != "/Users/test/Dropbox/Switchboard" {
		t.Errorf("the router finds the folder %q (%v), want it in the home directory given", got, err)
	}
}

func TestTheRoutersProbesGoOutOnTheTransportItGivesThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	// Nothing listens upstream: a probe sent otherwise than on the transport
	// given fails.
	if err := os.WriteFile(path, []byte("upstream = \"http://127.0.0.1:1\"\n\n[[account]]\nid = \"work\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &app{
		Getenv:        func(string) string { return "" },
		Environ:       func() []string { return nil },
		ClaudeVersion: func() string { return "2.1.300" },
	}
	cfg, _, err := a.startingFrom(t.TempDir(), path)
	if err != nil {
		t.Fatalf("startingFrom() error = %v", err)
	}
	var sent atomic.Int32
	transport := roundTripper(func(r *http.Request) (*http.Response, error) {
		sent.Add(1)
		header := http.Header{}
		header.Set("anthropic-ratelimit-unified-5h-utilization", "0.23")
		header.Set("anthropic-ratelimit-unified-5h-reset", "1790619000")
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: http.NoBody, Request: r}, nil
	})

	if _, err := cfg.Prober.Probe(t.Context(), "test-token-work", transport); err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if n := sent.Load(); n == 0 {
		t.Error("the probe sent nothing on the transport given")
	}
}

// roundTripper is a transport that answers each request as it says.
type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

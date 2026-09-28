package router_test

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
)

const (
	workToken = "test-token-work"
	sideToken = "test-token-side"
	// sessionID is the Claude Code session the tests' requests belong to.
	sessionID = "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"
	opus      = "claude-opus-5-5"
	// messages is a messages request's body, as Claude Code sends one.
	messages = `{"model":"claude-opus-5-5","max_tokens":32000,"messages":[{"role":"user","content":"hello"}],"stream":true}`
)

// now is the time by the router's clock in tests: a Monday, 13:12 UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

var (
	session = quota.Window{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
	week    = quota.Window{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC), Status: quota.StatusAllowedWarning}
)

// testConfig configures a router for three accounts, sending requests to
// upstream: work and side, with tokens, and personal, without one.
func testConfig(upstream string) router.Config {
	env := map[string]string{"CLAUDE_TOKEN_WORK": workToken, "CLAUDE_TOKEN_SIDE": sideToken}
	return router.Config{
		Accounts: []config.Account{
			{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
			{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
			{ID: "side", Label: "Side", TokenEnv: "CLAUDE_TOKEN_SIDE"},
		},
		Getenv:   func(key string) string { return env[key] },
		Upstream: upstream,
		Provider: claude.Provider{},
		Prober:   &fakeProber{},
		Policy:   score.Policy{Shared: claude.SharedWindows, Perishable: claude.PerishableWindow},
		Now:      func() time.Time { return now },
		Version:  "1.2.3",
	}
}

// newRouter builds a router from testConfig.
func newRouter(t *testing.T, upstream string) *router.Router {
	t.Helper()
	rt, err := router.New(testConfig(upstream))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return rt
}

// serveProxy serves the router's proxy until the test ends, and returns its URL.
func serveProxy(t *testing.T, rt *router.Router) string {
	t.Helper()
	srv := httptest.NewServer(rt.Proxy())
	t.Cleanup(srv.Close)
	return srv.URL
}

// received is a request as it reached the upstream.
type received struct {
	method, path, query string
	header              http.Header
	body                []byte
}

// upstream is a fake API: it notes every request that reaches it, then
// answers it as its handler does.
type upstream struct {
	*httptest.Server

	mu       sync.Mutex
	requests []received
}

func newUpstream(t *testing.T, answer http.HandlerFunc) *upstream {
	t.Helper()
	u := &upstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("upstream: read request body: %v", err)
		}
		u.mu.Lock()
		u.requests = append(u.requests, received{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: body})
		u.mu.Unlock()
		answer(w, r)
	}))
	t.Cleanup(u.Close)
	return u
}

// answerOK answers every request with a 200 and a small body.
func answerOK(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, `{"type":"message"}`)
}

// answerWith answers every request with status, reporting windows in its
// headers.
func answerWith(status int, windows ...quota.Window) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for _, win := range windows {
			prefix := "anthropic-ratelimit-unified-" + win.Key + "-"
			w.Header().Set(prefix+"utilization", strconv.FormatFloat(win.Utilization, 'f', -1, 64))
			w.Header().Set(prefix+"reset", strconv.FormatInt(win.ResetsAt.Unix(), 10))
			w.Header().Set(prefix+"status", string(win.Status))
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}
}

// only returns the one request that reached the upstream.
func (u *upstream) only(t *testing.T) received {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.requests) != 1 {
		t.Fatalf("upstream received %d requests, want 1", len(u.requests))
	}
	return u.requests[0]
}

// count returns how many requests have reached the upstream.
func (u *upstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

// claudeCode is the header of a messages request Claude Code sends with token.
func claudeCode(token string) http.Header {
	return http.Header{
		"Authorization":            {"Bearer " + token},
		"Anthropic-Version":        {"2023-06-01"},
		"Anthropic-Beta":           {"oauth-2025-04-20"},
		"Accept-Encoding":          {"gzip, br"},
		"Content-Type":             {"application/json"},
		"User-Agent":               {"claude-cli/2.1.300 (external, cli)"},
		"X-Claude-Code-Session-Id": {sessionID},
	}
}

// with returns h with the header name set to value, or deleted when value is
// empty.
func with(h http.Header, name, value string) http.Header {
	h = h.Clone()
	if value == "" {
		h.Del(name)
	} else {
		h.Set(name, value)
	}
	return h
}

// send sends a request to url with header, and returns the response, which is
// closed when the test ends. Its transport adds no Accept-Encoding of its own,
// so the header is all the proxy is sent.
func send(t *testing.T, method, url string, header http.Header, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(req.Header, header)
	transport := &http.Transport{DisableCompression: true}
	t.Cleanup(transport.CloseIdleConnections)
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// readAll reads the rest of a response's body.
func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}

// checkHeader checks the upstream got exactly the header want, but for the
// Content-Length the transport sets.
func checkHeader(t *testing.T, got received, want http.Header) {
	t.Helper()
	got.header.Del("Content-Length")
	if !reflect.DeepEqual(got.header, want) {
		t.Errorf("upstream got header\n%v\nwant\n%v", sorted(got.header), sorted(want))
	}
}

// sorted shows a header a field to a line, in order, for a readable diff.
func sorted(h http.Header) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(h)) {
		fmt.Fprintf(&b, "%s: %s\n", name, strings.Join(h[name], ", "))
	}
	return b.String()
}

// fakeProber answers each token with its reading, and notes the tokens it's
// given.
type fakeProber struct {
	readings map[string]probeResult

	mu     sync.Mutex
	tokens []string
}

type probeResult struct {
	usage quota.Usage
	err   error
}

func (p *fakeProber) Probe(_ context.Context, token string) (quota.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens = append(p.tokens, token)
	r := p.readings[token]
	return r.usage, r.err
}

// probed returns the tokens the prober was given, sorted.
func (p *fakeProber) probed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Sorted(slices.Values(p.tokens))
}

// shortTempDir returns a directory of the test's own with a path short enough
// to hold a unix socket: t.TempDir's can be too long on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

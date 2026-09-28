package router_test

import (
	"bytes"
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
	// personalToken is personal's token, when a test gives it one.
	personalToken = "test-token-personal"
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
	// fableWeek is the Fable week, which only Fable requests report.
	fableWeek = quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 0.05, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
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

// withPersonalToken gives personal a token, so requests can go out on all
// three accounts.
func withPersonalToken(cfg *router.Config) {
	getenv := cfg.Getenv
	cfg.Getenv = func(key string) string {
		if key == "CLAUDE_TOKEN_PERSONAL" {
			return personalToken
		}
		return getenv(key)
	}
}

// newRouter builds a router from testConfig.
func newRouter(t *testing.T, upstream string) *router.Router {
	t.Helper()
	return newRouterFrom(t, testConfig(upstream))
}

// newRouterFrom builds a router from cfg.
func newRouterFrom(t *testing.T, cfg router.Config) *router.Router {
	t.Helper()
	rt, err := router.New(cfg)
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
		r.Body = io.NopCloser(bytes.NewReader(body))
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
		reportWindows(w.Header(), windows)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}
}

// reportWindows sets the headers that report windows, as the API's do. A
// window without a reset reports none.
func reportWindows(h http.Header, windows []quota.Window) {
	for _, win := range windows {
		prefix := "anthropic-ratelimit-unified-" + win.Key + "-"
		h.Set(prefix+"utilization", strconv.FormatFloat(win.Utilization, 'f', -1, 64))
		if !win.ResetsAt.IsZero() {
			h.Set(prefix+"reset", strconv.FormatInt(win.ResetsAt.Unix(), 10))
		}
		h.Set(prefix+"status", string(win.Status))
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

// accounts returns the account each request that reached the upstream went
// out on, by the token it carried, in the order they came.
func (u *upstream) accounts() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	ids := make([]string, len(u.requests))
	for i, r := range u.requests {
		ids[i] = accountOf(bearerOf(&http.Request{Header: r.header}))
	}
	return ids
}

// bodies returns the body of each request that reached the upstream.
func (u *upstream) bodies() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	bodies := make([]string, len(u.requests))
	for i, r := range u.requests {
		bodies[i] = string(r.body)
	}
	return bodies
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
	mu       sync.Mutex
	readings map[string]probeResult
	tokens   []string
}

type probeResult struct {
	usage quota.Usage
	// models are the models that reported each window, by its key.
	models map[string][]string
	err    error
}

func (p *fakeProber) Probe(_ context.Context, token string) (quota.Probe, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens = append(p.tokens, token)
	r := p.readings[token]
	return quota.Probe{Usage: r.usage, Models: r.models}, r.err
}

// answer has the prober answer token with r from now on.
func (p *fakeProber) answer(token string, r probeResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.readings == nil {
		p.readings = make(map[string]probeResult)
	}
	p.readings[token] = r
}

// probed returns the tokens the prober was given, sorted.
func (p *fakeProber) probed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Sorted(slices.Values(p.tokens))
}

// probes returns how many times the prober was given token.
func (p *fakeProber) probes(token string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(slices.DeleteFunc(slices.Clone(p.tokens), func(t string) bool { return t != token }))
}

// readingEvery is a prober that reads every account as windows.
func readingEvery(windows ...quota.Window) *fakeProber {
	p := &fakeProber{}
	for _, token := range []string{workToken, sideToken, personalToken} {
		p.answer(token, probeResult{usage: quota.Usage{Windows: windows}})
	}
	return p
}

// fakeClock is a clock a test moves as it goes, which the router can read
// from any goroutine.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

func (c *fakeClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// accountsAPI is a fake API that answers each request with the windows the
// test last gave the account whose token it carries, as the real API reports
// an account's usage on every response. A Fable week is reported only on
// Fable requests, as the real API reports it. A test can script an account's
// next answers instead, which it gives first, one a request.
type accountsAPI struct {
	*upstream

	mu      sync.Mutex
	windows map[string][]quota.Window
	scripts map[string][]http.HandlerFunc
}

func newAccountsAPI(t *testing.T) *accountsAPI {
	t.Helper()
	api := &accountsAPI{windows: make(map[string][]quota.Window), scripts: make(map[string][]http.HandlerFunc)}
	api.upstream = newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if answer, ok := api.scripted(bearerOf(r)); ok {
			answer(w, r)
			return
		}
		windows := api.of(bearerOf(r))
		if (claude.Provider{}).Family(modelOf(t, r)) != "fable" {
			windows = slices.DeleteFunc(windows, func(w quota.Window) bool { return w.Key == "7d_oi" })
		}
		answerWith(http.StatusOK, windows...)(w, r)
	})
	return api
}

// script has the API give the next requests on token answers, in order,
// before it goes back to answering with the account's windows.
func (a *accountsAPI) script(token string, answers ...http.HandlerFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.scripts[token] = append(a.scripts[token], answers...)
}

// scripted returns the next answer scripted for token, if one is.
func (a *accountsAPI) scripted(token string) (http.HandlerFunc, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	answers := a.scripts[token]
	if len(answers) == 0 {
		return nil, false
	}
	a.scripts[token] = answers[1:]
	return answers[0], true
}

// modelOf returns the model a request's body asks for.
func modelOf(t *testing.T, r *http.Request) string {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("upstream: read request body: %v", err)
	}
	return (claude.Provider{}).Model(body)
}

// set has the API report windows for the account whose token is token.
func (a *accountsAPI) set(token string, windows ...quota.Window) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.windows[token] = windows
}

func (a *accountsAPI) of(token string) []quota.Window {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.windows[token])
}

// lastAccount returns the account whose token the last request to reach the
// API carried.
func (u *upstream) lastAccount() string {
	accounts := u.accounts()
	return accounts[len(accounts)-1]
}

// accountOf returns the account whose token is token.
func accountOf(token string) string {
	return map[string]string{workToken: "work", sideToken: "side", personalToken: "personal"}[token]
}

func bearerOf(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// limitReached answers with a 429 whose windows report the account's limit
// reached, and message as the error's.
func limitReached(message string, windows ...quota.Window) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		reportWindows(w.Header(), windows)
		w.Header().Set("Anthropic-Ratelimit-Unified-Status", "rejected")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":"rate_limit_error","message":%q}}`, message)
	}
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

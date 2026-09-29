package cli_test

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
)

// workAtStatus is work's status, as status prints it from fakeClaudeAPI's
// probes.
const workAtStatus = `work · Work (primary)
  Session     23%  resets in 4h 58m · Mon 18:10
  Week        93%  resets in 4d 7h · Fri 21:00 · runs out ~Mon 18:01
  Fable week 100%  resets in 5d 11h · Sun 01:10 · exhausted
`

// othersAtStatus is how status prints personal and side from fakeClaudeAPI's
// probes, commands run with deps finding personal's token file missing.
func othersAtStatus(t *testing.T, deps cli.Deps) string {
	t.Helper()
	return `personal · Personal
  token missing: write it to ` + tokenPath(t, deps, "personal") + `

side · Side
  HTTP 401 · Invalid bearer token
`
}

func TestStatusReadsTheRouterWhileItRuns(t *testing.T) {
	srv := routingSetup(t)

	got := run(t, srv.deps, "status")
	want := result{stdout: workAtStatus + "  1 session\n\n" + othersAtStatus(t, srv.deps) + `
sessions
  0b5c6f2e  haiku on work  ·  seen just now

best next: work · Work
from the router: healthy  ·  1 session  ·  pinned to side · Side
`}
	if got != want {
		t.Errorf("switchboard status =\n%+v\nwant\n%+v", got, want)
	}

	doc := statusJSON(t, srv.deps)
	work, _ := doc.Account("work")
	if doc.Source != status.SourceRouter || doc.Pin.Account != "side" || !doc.Router.Healthy || doc.Router.Requests != 1 || doc.Sessions != 1 || work.Sessions != 1 {
		t.Errorf("switchboard status --json printed\n%+v\nwant the router's document: pinned to side, healthy, with work's session", doc)
	}
}

func TestStatusReadsAnUnhealthyRoutersDocument(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	srv := newServeSetup(t, gone.URL, nil)
	srv.start(t)
	for range 5 {
		if code := srv.ask(t); code != http.StatusBadGateway {
			t.Fatalf("a request the router can't send on was answered %d, want 502", code)
		}
	}
	srv.waitForStatus(t, func(doc status.Document) bool { return doc.Router.Requests == 5 })

	got := run(t, srv.deps, "status")
	const reason = "5 of the 5 requests in the last 5 minutes failed"
	wantLast := "from the router: unhealthy, " + reason + "  ·  no sessions  ·  routing automatically\n"
	if got.code != 0 || !strings.HasSuffix(got.stdout, "\n"+wantLast) {
		t.Errorf("switchboard status = %+v, want it to end %q", got, wantLast)
	}
	doc := statusJSON(t, srv.deps)
	if want := (status.Health{Requests: 5, Failures: 5, Reason: reason}); doc.Source != status.SourceRouter || doc.Router != want {
		t.Errorf("switchboard status --json printed source %q and router %+v, want the router's, %+v", doc.Source, doc.Router, want)
	}
}

func TestStatusProbesWithoutTheRouter(t *testing.T) {
	got := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "status")
	if want := "\nprobed directly: the router isn't running\n"; !strings.HasSuffix(got.stdout, want) {
		t.Errorf("switchboard status = %+v, want it to end %q", got, want)
	}
	doc := statusJSON(t, statusDeps(t, fakeClaudeAPI(t), nil))
	if want := (status.Fallback{Router: status.RouterNotRunning}); doc.Source != status.SourceProbe || doc.Fallback != want {
		t.Errorf("switchboard status --json printed source %q and fallback %+v, want probe, %+v", doc.Source, doc.Fallback, want)
	}
}

func TestStatusProbesPastARouterThatDoesntAnswer(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	serveSilently(t, srv.socket())

	got := run(t, srv.deps, "status")
	want := result{stdout: workAtStatus + "\n" + othersAtStatus(t, srv.deps) + `
best next: work · Work
probed directly: the router is unhealthy, no answer within 500ms
`}
	if got != want {
		t.Errorf("switchboard status =\n%+v\nwant\n%+v", got, want)
	}
	doc := statusJSON(t, srv.deps)
	if want := (status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"}); doc.Source != status.SourceProbe || doc.Fallback != want {
		t.Errorf("switchboard status --json printed source %q and fallback %+v, want probe, %+v", doc.Source, doc.Fallback, want)
	}
}

func TestStatusProbesAsAsked(t *testing.T) {
	srv := routingSetup(t)

	got := run(t, srv.deps, "status", "--probe")
	want := result{stdout: workAtStatus + "\n" + othersAtStatus(t, srv.deps) + `
best next: work · Work
probed directly
`}
	if got != want {
		t.Errorf("switchboard status --probe =\n%+v\nwant\n%+v", got, want)
	}
	doc := statusJSON(t, srv.deps, "--probe")
	if doc.Source != status.SourceProbe || doc.Fallback != (status.Fallback{}) {
		t.Errorf("switchboard status --json --probe printed source %q and fallback %+v, want probe, and no fallback", doc.Source, doc.Fallback)
	}
}

func TestUsageReadsTheRouterWhileItRuns(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	// A token for personal, as goldenDeps gives it.
	writeToken(t, srv.deps, "personal", "test-token-personal")
	routing(t, srv)

	got := run(t, srv.deps, "usage")
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("switchboard usage = %+v, want exit status 0 and nothing on stderr", got)
	}
	const golden = "usage-router.golden"
	if *update {
		writeGolden(t, golden, got.stdout)
	}
	if want := readGolden(t, golden); got.stdout != want {
		t.Errorf("switchboard usage printed\n%s\nwant (testdata/%s; run with -update to accept it)\n%s", got.stdout, golden, want)
	}
	probed := run(t, srv.deps, "usage", "--probe")
	if !strings.Contains(probed.stdout, "\n best next: work · Work\n") || strings.Contains(probed.stdout, "router") || strings.Contains(probed.stdout, "pinned") {
		t.Errorf("switchboard usage --probe printed\n%s\nwant the accounts probed, as asked, saying nothing of the router", probed.stdout)
	}
}

func TestUsageWatchReadsTheRouterWhileItRuns(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	stop := srv.start(t)
	srv.waitForProbes(t)
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "usage", "--watch")

	for _, r := range []watch.Read{{Refresh: 30 * time.Minute, Probe: true}, {Probe: true}, {Refresh: 30 * time.Minute}} {
		doc, err := cfg.Source.Read(t.Context(), r)
		if err != nil || doc.Source != status.SourceRouter {
			t.Errorf("Read(%+v) = %+v, %v, want the router's document", r, doc, err)
		}
	}
	if err := cfg.Source.Pin(t.Context(), "side", true); err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if got, want := srv.status(t).Pin, (status.Pin{Account: "side", Since: testNow, Move: true}); got != want {
		t.Errorf("once pinned, the router's pin = %+v, want %+v", got, want)
	}
	if err := cfg.Source.Unpin(t.Context()); err != nil {
		t.Fatalf("Unpin() error = %v", err)
	}
	if got := srv.status(t).Pin; got != (status.Pin{}) {
		t.Errorf("once unpinned, the router's pin = %+v, want none", got)
	}

	stop()
	if _, err := cfg.Source.Read(t.Context(), watch.Read{Refresh: 30 * time.Minute}); !errors.Is(err, watch.ErrNoRouter) {
		t.Errorf("once the router stopped, a question after it failed with %v, want ErrNoRouter", err)
	}
	doc, err := cfg.Source.Read(t.Context(), watch.Read{Probe: true})
	if err != nil || doc.Source != status.SourceProbe || doc.Fallback != (status.Fallback{Router: status.RouterNotRunning}) {
		t.Errorf("once the router stopped, a read that may probe = %+v, %v, want one probed, as the router isn't running", doc, err)
	}
	if err := cfg.Source.Pin(t.Context(), "side", false); err == nil || err.Error() != "the router isn't running: start it with switchboard service install (or switchboard serve)" {
		t.Errorf("once the router stopped, Pin() error = %v, want it to say so", err)
	}
}

func TestUsageWatchProbesAsAsked(t *testing.T) {
	srv := routingSetup(t)
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "usage", "--watch", "--probe")

	doc, err := cfg.Source.Read(t.Context(), watch.Read{Refresh: 30 * time.Minute, Probe: true})
	if err != nil || doc.Source != status.SourceProbe || doc.Fallback != (status.Fallback{}) {
		t.Errorf("Read() = %+v, %v, want one probed, as asked, while the router runs", doc, err)
	}
	if _, err := cfg.Source.Read(t.Context(), watch.Read{Refresh: 30 * time.Minute}); !errors.Is(err, watch.ErrNoRouter) {
		t.Errorf("a question after the router failed with %v, want ErrNoRouter: it's not to be read", err)
	}
}

func TestUsageWatchProbingAsAskedAsksAfterTheRouter(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	stop := srv.start(t)
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "usage", "--watch", "--probe")

	if !cfg.Source.RouterAnswers(t.Context()) {
		t.Error("RouterAnswers() = false while the router runs, want true: it posts the notifications")
	}
	stop()
	if cfg.Source.RouterAnswers(t.Context()) {
		t.Error("RouterAnswers() = true once the router stopped, want false: the dashboard posts its own")
	}
}

// routingSetup starts a router of statusDeps' accounts, as routing does.
func routingSetup(t *testing.T) *serveSetup {
	t.Helper()
	return routing(t, newServeSetup(t, fakeClaudeAPI(t), nil))
}

// routing starts srv's router, once it has probed the accounts, with a
// session on work, and pinned to side, and returns srv.
func routing(t *testing.T, srv *serveSetup) *serveSetup {
	t.Helper()
	srv.start(t)
	srv.waitForProbes(t)
	srv.route(t, "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", "claude-haiku-4-5-20251001")
	if got := run(t, srv.deps, "pin", "side"); got.code != 0 {
		t.Fatalf("switchboard pin side = %+v", got)
	}
	return srv
}

// waitForProbes waits for the router to have probed every account it has a
// token for: to have read its usage, or why it couldn't.
func (s *serveSetup) waitForProbes(t *testing.T) {
	t.Helper()
	s.waitForStatus(t, func(doc status.Document) bool {
		return !slices.ContainsFunc(doc.Accounts, func(a status.Account) bool {
			return a.TokenSet && len(a.Windows) == 0 && a.Error == ""
		})
	})
}

// ask sends the router a messages request on work's token, of no session,
// and returns the status it's answered with.
func (s *serveSetup) ask(t *testing.T) int {
	t.Helper()
	body := `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+s.listen+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token-work")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

// statusJSON runs status --json with args, and returns the document it
// prints.
func statusJSON(t *testing.T, deps cli.Deps, args ...string) status.Document {
	t.Helper()
	got := run(t, deps, append([]string{"status", "--json"}, args...)...)
	var doc status.Document
	if got.code != 0 || json.Unmarshal([]byte(got.stdout), &doc) != nil {
		t.Fatalf("switchboard status --json = %+v, want exit status 0 and a document", got)
	}
	return doc
}

// serveSilently listens on the socket at path until the test ends, and
// answers nothing it's asked.
func serveSilently(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }),
		ReadHeaderTimeout: time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

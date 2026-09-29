package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestServe(t *testing.T) {
	var probed status.Document
	if err := json.Unmarshal([]byte(run(t, statusDeps(t, fakeClaudeAPI(t), nil), "status", "--json", "--probe").stdout), &probed); err != nil {
		t.Fatal(err)
	}
	api := newClaudeAPI(t)
	srv := newServeSetup(t, api.URL, nil)

	stop := srv.start(t)
	doc := srv.waitForStatus(t, func(doc status.Document) bool {
		work, _ := doc.Account("work")
		side, _ := doc.Account("side")
		return len(work.Windows) == 3 && side.Error != ""
	})
	if doc.Source != "router" || doc.Router != (status.Health{Healthy: true}) {
		t.Errorf("source = %q, router = %+v, want router, healthy, having routed nothing", doc.Source, doc.Router)
	}
	doc.Source, doc.Router = probed.Source, probed.Router
	if !reflect.DeepEqual(doc, probed) {
		t.Errorf("once it has probed, the router reports\n%+v\nwant what status --json --probe does, but for its source and health:\n%+v", doc, probed)
	}
	want := []string{
		"test-token-side claude-fable-5", "test-token-side claude-fable-5-1", "test-token-side claude-haiku-4-5-20251001",
		"test-token-work claude-fable-5-1", "test-token-work claude-haiku-4-5-20251001",
	}
	if got := api.questions(); !reflect.DeepEqual(got, want) {
		t.Errorf("at startup the API was asked\n%q\nwant a probe of each account with a token\n%q", got, want)
	}

	if got := stop(); got != (result{}) {
		t.Errorf("switchboard serve = %+v once stopped, want exit status 0 and no output", got)
	}
	if _, err := os.Stat(srv.socket()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket once stopped: %v, want it gone", err)
	}
	log := srv.routerLog(t)
	for _, want := range [][]string{
		{"level=INFO", "msg=start component=process", "role=router", `command="switchboard serve"`},
		{"level=WARN", `msg="account has no token; nothing will go out on it" component=router`, "account=personal"},
		{"level=INFO", "msg=listening component=router", "address=" + srv.listen, "upstream=" + api.URL, "token_set.work=true token_set.personal=false token_set.side=true"},
		{"level=WARN", `msg="probe failed" component=router`, "account=side", `error="HTTP 401 · Invalid bearer token"`},
		{"level=INFO", "msg=stopping component=router"},
		{"level=INFO", "msg=stopped component=router"},
		{"level=INFO", "msg=exit component=process", "status=0"},
	} {
		if !hasLine(log, want...) {
			t.Errorf("router.log reads\n%s\nwant a line with %q", log, want)
		}
	}
	for _, token := range []string{"test-token-work", "test-token-side"} {
		if strings.Contains(log, token) {
			t.Errorf("router.log shows a token:\n%s", log)
		}
	}
}

func TestServeRefusesASecondRouter(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)

	got := run(t, srv.deps, "serve")
	want := result{stderr: "Error: switchboard is already running (pid " + strconv.Itoa(os.Getpid()) + ")\n", code: 1}
	if got != want {
		t.Errorf("a second switchboard serve = %+v, want %+v", got, want)
	}
	if _, err := router.NewClient(srv.socket()).Health(t.Context()); err != nil {
		t.Errorf("the first router's health: %v, want it still answering", err)
	}
}

func TestServeReplacesAStaleSocket(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	if err := os.MkdirAll(filepath.Dir(srv.socket()), 0o700); err != nil {
		t.Fatal(err)
	}
	stale, err := net.Listen("unix", srv.socket())
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = stale.Close()

	stop := srv.start(t)
	if got := stop(); got.code != 0 {
		t.Errorf("switchboard serve = %+v, want exit status 0", got)
	}
	if log := srv.routerLog(t); !hasLine(log, "level=INFO", `msg="removed a stale control socket"`, "path="+srv.socket()) {
		t.Errorf("router.log reads\n%s\nwant the stale socket's removal", log)
	}
}

func TestServeWhenItsAddressIsTaken(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = taken.Close() })
	addr := taken.Addr().String()
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.listen = addr
	srv.writeConfig(t)

	got := run(t, srv.deps, "serve")
	wantErr := "Error: can't listen on " + addr + ", as another program already is: stop it, or set listen in the config to another address"
	if got.code != 1 || !strings.HasPrefix(got.stderr, wantErr) {
		t.Errorf("switchboard serve = %+v, want exit status 1 and an error starting %q", got, wantErr)
	}
	if _, err := os.Stat(srv.socket()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket: %v, want none", err)
	}
}

func TestServeWithoutAnyToken(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"CLAUDE_TOKEN_WORK": "", "CLAUDE_TOKEN_SIDE": " "})

	got := run(t, srv.deps, "serve")
	want := result{
		stderr: "Error: no account has a token, so there's nothing to route to: set CLAUDE_TOKEN_WORK or CLAUDE_TOKEN_PERSONAL or CLAUDE_TOKEN_SIDE\n",
		code:   1,
	}
	if got != want {
		t.Errorf("switchboard serve = %+v, want %+v", got, want)
	}
}

func TestServeLogLevel(t *testing.T) {
	const probedLine = `level=DEBUG msg="probed account" component=router`
	tests := []struct {
		name      string
		args      []string
		wantDebug bool
	}{
		{name: "SWITCHBOARD_LOG_LEVEL's unless given", wantDebug: false},
		{name: "the level given", args: []string{"--log-level", "DEBUG"}, wantDebug: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_LOG_LEVEL": "warn"})

			stop := srv.start(t, tt.args...)
			srv.waitForStatus(t, func(doc status.Document) bool {
				work, _ := doc.Account("work")
				side, _ := doc.Account("side")
				return len(work.Windows) > 0 && side.Error != ""
			})
			stop()
			log := srv.routerLog(t)
			if logged := strings.Contains(log, probedLine); logged != tt.wantDebug {
				t.Errorf("router.log reads\n%s\nwant debug records: %v", log, tt.wantDebug)
			}
			if !strings.Contains(log, `level=WARN msg="probe failed"`) {
				t.Errorf("router.log reads\n%s\nwant warnings at any level", log)
			}
		})
	}
}

func TestServePostsTheNotificationsTheConfigAsksFor(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get("Authorization"); token != "Bearer test-token-work" && token != "Bearer test-token-side" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Anthropic-Ratelimit-Unified-5h-Utilization", "0.2")
		w.Header().Set("Anthropic-Ratelimit-Unified-5h-Reset", "1790619000")
		w.Header().Set("Anthropic-Ratelimit-Unified-7d-Utilization", "0.5")
		w.Header().Set("Anthropic-Ratelimit-Unified-7d-Reset", "1790974800")
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	t.Cleanup(api.Close)
	srv := newServeSetup(t, api.URL, nil)
	srv.extra = "\n[notifications]\nmoves = true\n"
	srv.writeConfig(t)
	notifier := &recordingNotifier{}
	srv.deps.Notifier = notifier
	stop := srv.start(t)

	for _, pin := range []string{"work", "side"} {
		if status := askPinned(t, "http://"+srv.listen, pin); status != http.StatusOK {
			t.Fatalf("a request pinned to %s was answered %d, want 200", pin, status)
		}
	}
	want := []string{"session 0b5c6f2e moved from work · Work to side · Side (pinned)"}
	waitUntil(t, "the move is told of", func() bool { return slices.Equal(notifier.posted(), want) })
	if got := stop(); got.code != 0 {
		t.Errorf("switchboard serve = %+v, want exit status 0", got)
	}
}

// askPinned sends the router at url a messages request of one session, on
// work's token and pinned to pin, and returns the status it's answered with.
func askPinned(t *testing.T, url, pin string) int {
	t.Helper()
	body := `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token-work")
	req.Header.Set("X-Claude-Code-Session-Id", "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e")
	req.Header.Set("X-Switchboard-Account", pin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

// serveSetup is what serve runs with in a test: statusDeps' three accounts,
// a free address to listen on, and a state directory short enough to hold the
// control socket.
type serveSetup struct {
	deps     cli.Deps
	upstream string
	listen   string
	// state is switchboard's state directory, under $XDG_STATE_HOME.
	state  string
	config string
	// extra ends the config file.
	extra string
}

// newServeSetup sets serve up against upstream, with env added to its
// environment.
func newServeSetup(t *testing.T, upstream string, env map[string]string) *serveSetup {
	t.Helper()
	stateHome, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateHome) })
	s := &serveSetup{
		upstream: upstream,
		listen:   freeAddress(t),
		state:    filepath.Join(stateHome, "switchboard"),
		config:   filepath.Join(t.TempDir(), "config.toml"),
	}
	s.writeConfig(t)
	vars := map[string]string{
		"SWITCHBOARD_CONFIG": s.config,
		"XDG_STATE_HOME":     stateHome,
		"CLAUDE_TOKEN_WORK":  "test-token-work",
		"CLAUDE_TOKEN_SIDE":  "test-token-side",
	}
	maps.Copy(vars, env)
	s.deps = testDeps(vars, t.TempDir())
	return s
}

// writeConfig writes the config file serve reads.
func (s *serveSetup) writeConfig(t *testing.T) {
	t.Helper()
	content := fmt.Sprintf(`listen   = %q
upstream = %q

[[account]]
id        = "work"
label     = "Work"
token_env = "CLAUDE_TOKEN_WORK"

[[account]]
id        = "personal"
label     = "Personal"
token_env = "CLAUDE_TOKEN_PERSONAL"

[[account]]
id        = "side"
label     = "Side"
token_env = "CLAUDE_TOKEN_SIDE"
`, s.listen, s.upstream) + s.extra
	if err := os.WriteFile(s.config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (s *serveSetup) socket() string {
	return router.SocketPath(s.state)
}

// routerLog returns what the router's log holds.
func (s *serveSetup) routerLog(t *testing.T) string {
	t.Helper()
	return s.log(t, "router.log")
}

// cliLog returns what the log every other command writes holds.
func (s *serveSetup) cliLog(t *testing.T) string {
	t.Helper()
	return s.log(t, "cli.log")
}

// log returns what the log called name holds.
func (s *serveSetup) log(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.state, "logs", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// start runs switchboard serve with args, returning once its control socket
// answers. It runs until the test calls the stop it returns, which returns
// what serve printed and exited with, or until the test ends.
func (s *serveSetup) start(t *testing.T, args ...string) (stop func() result) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	root := cli.NewRootCommand(s.deps)
	root.SetContext(ctx)
	root.SetArgs(append([]string{"serve"}, args...))
	var stdout, stderr syncBuffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	var code int
	finished := make(chan struct{})
	go func() {
		code = cli.Execute(root)
		close(finished)
	}()
	stop = func() result {
		cancel()
		<-finished
		return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
	}
	t.Cleanup(func() { stop() })
	client := router.NewClient(s.socket())
	waitUntil(t, "switchboard serve answers", func() bool {
		select {
		case <-finished:
			t.Fatalf("switchboard serve = %+v before it answered", stop())
		default:
		}
		_, err := client.Health(ctx)
		return err == nil
	})
	return stop
}

// waitForStatus waits a few seconds at most for the router to report a
// document that done accepts, and returns it.
func (s *serveSetup) waitForStatus(t *testing.T, done func(status.Document) bool) status.Document {
	t.Helper()
	var doc status.Document
	waitUntil(t, "the router reports the status wanted", func() bool {
		var err error
		doc, err = router.NewClient(s.socket()).Status(t.Context())
		return err == nil && done(doc)
	})
	return doc
}

// freeAddress returns a loopback address whose port nothing listens on.
func freeAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

// waitUntil waits a few seconds at most for ready to report true.
func waitUntil(t *testing.T, what string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up waiting until %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

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
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/skill"
	"github.com/leeovery/switchboard/internal/status"
)

func TestServe(t *testing.T) {
	api := newClaudeAPI(t)
	srv := newServeSetup(t, api.URL, nil)
	// Probed from the same state directory, and so the same token files, which
	// the document names.
	var probed status.Document
	probing := statusDeps(t, fakeClaudeAPI(t), map[string]string{"XDG_STATE_HOME": filepath.Dir(srv.state)})
	if err := json.Unmarshal([]byte(run(t, probing, "status", "--json", "--probe").stdout), &probed); err != nil {
		t.Fatal(err)
	}

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
		{"level=WARN", `msg="account has no usable token; nothing will go out on it" component=router`, "account=personal", `error="token missing: write it to ` + tokenPath(t, srv.deps, "personal") + `"`},
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
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	removeToken(t, srv.deps, "work")
	writeToken(t, srv.deps, "side", " ")

	stop := srv.start(t)
	if got := stop(); got != (result{}) {
		t.Errorf("switchboard serve = %+v once stopped, want exit status 0 and no output", got)
	}
	log := srv.routerLog(t)
	for _, want := range [][]string{
		{"level=WARN", `msg="account has no usable token; nothing will go out on it" component=router`, "account=work", `error="token missing: write it to ` + tokenPath(t, srv.deps, "work") + `"`},
		{"level=WARN", `msg="account has no usable token; nothing will go out on it" component=router`, "account=side", `error="token missing: write it to ` + tokenPath(t, srv.deps, "side") + `, which is empty"`},
		{"level=WARN", `msg="no account has a usable token yet; nothing will be routed until one has" component=router`},
	} {
		if !hasLine(log, want...) {
			t.Errorf("router.log reads\n%s\nwant a line with %q", log, want)
		}
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

func TestPrimingAsTheConfigSetsIt(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.extra = "\n[prime]\nday = \"08:00-23:00\"\n"
	srv.writeConfig(t)
	// Work and side have token files, so they're primed at 04:15 and 06:45,
	// and personal has none.
	schedule := "priming 08:00-23:00: work at 04:15 and side at 06:45\n"

	probed := run(t, srv.deps, "status")
	if want := schedule + "next reset: work · Work, Mon 18:10\n"; !strings.Contains(probed.stdout, want) {
		t.Errorf("switchboard status, probing, printed\n%s\nwant\n%s", probed.stdout, want)
	}

	srv.start(t)
	// Work's session runs till 18:10, so it's primed a few seconds after,
	// and side, whose token the API refuses, is primed again five minutes
	// after its probe as the router starts.
	doc := srv.waitForStatus(t, func(doc status.Document) bool {
		work, _ := doc.Account("work")
		side, _ := doc.Account("side")
		return len(work.Windows) == 3 && side.Error != ""
	})
	want := status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{
		{Account: "work", At: "04:15", Next: time.Date(2026, 9, 28, 18, 10, 5, 0, time.UTC)},
		{Account: "side", At: "06:45", Next: testNow.Add(5 * time.Minute)},
	}}
	if !reflect.DeepEqual(doc.Prime, want) {
		t.Errorf("the router's schedule is\n%+v\nwant\n%+v", doc.Prime, want)
	}
	routed := run(t, srv.deps, "status")
	if want := schedule + "next reset: work · Work, Mon 18:10  ·  next prime: side · Side, Mon 13:17\n"; !strings.Contains(routed.stdout, want) {
		t.Errorf("switchboard status, reading the router, printed\n%s\nwant\n%s", routed.stdout, want)
	}
}

func TestServeRestartsItselfWhenTheServiceRunsIt(t *testing.T) {
	tests := []struct {
		name string
		// change changes what serve was started from.
		change     func(t *testing.T, srv *serveSetup)
		wantReason string
	}{
		{
			name: "once its config file makes another valid config",
			change: func(t *testing.T, srv *serveSetup) {
				srv.extra = "\n[notifications]\nmoves = true\n"
				srv.writeConfig(t)
			},
			wantReason: `reason="config changed"`,
		},
		{
			name:       "once an upgrade moves its binary on",
			change:     func(t *testing.T, srv *serveSetup) { srv.upgrade(t) },
			wantReason: "reason=upgraded",
		},
		{
			name:       "once the Mac is taken to another time zone",
			change:     func(t *testing.T, srv *serveSetup) { srv.travel(t) },
			wantReason: `reason="time zone changed"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"XPC_SERVICE_NAME": service.Label})
			srv.watch(t)
			argv := []string{srv.binary, "serve"}
			made := srv.replaceable(t, argv...)
			stop := srv.start(t)

			tt.change(t, srv)
			replaced := waitForReplacement(t, made)
			if got := stop(); got != (result{}) {
				t.Errorf("switchboard serve = %+v as it replaced itself, want no output", got)
			}
			if replaced.path != srv.binary || !slices.Equal(replaced.argv, argv) {
				t.Errorf("serve replaced itself with %s run as %q, want its binary, by the link it was run as, %s, run as it was, %q", replaced.path, replaced.argv, srv.binary, argv)
			}
			env := environMap(replaced.env)
			if env["XPC_SERVICE_NAME"] != service.Label || env["SWITCHBOARD_CONFIG"] != srv.config || env[handover.Variable] == "" {
				t.Errorf("serve replaced itself in the environment %q, want the one it ran in, naming the listeners it handed over", replaced.env)
			}
			waitUntil(t, "the router serve became answers", func() bool {
				h, err := router.NewClient(srv.socket()).Health(t.Context())
				return err == nil && h.StartedAt.Equal(testNow.Add(time.Minute))
			})
			srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
			if got := replaced.stop(); got.code != 0 {
				t.Errorf("the switchboard serve it became = %+v, want exit status 0", got)
			}
			log := srv.routerLog(t)
			for _, want := range [][]string{
				{"level=INFO", "msg=restarting component=router", tt.wantReason},
				{"level=INFO", `msg="replacing itself" component=router`, "path=" + srv.binary, tt.wantReason},
				{"level=INFO", `msg="took up the listener handed over" component=router`, "listener=proxy", "address=" + srv.listen},
				{"level=INFO", `msg="took up the listener handed over" component=router`, "listener=control", "address=" + srv.socket()},
			} {
				if !hasLine(log, want...) {
					t.Errorf("router.log reads\n%s\nwant a line with %q", log, want)
				}
			}
		})
	}
}

func TestServeExitsForLaunchdToStartItAgainWhenItCantReplaceItself(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"XPC_SERVICE_NAME": service.Label})
	srv.watch(t)
	stop := srv.start(t)

	srv.upgrade(t)
	select {
	case <-srv.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("gave up waiting for switchboard serve to restart")
	}
	if got := stop(); got != (result{}) {
		t.Errorf("switchboard serve = %+v as it restarted, want exit status 0 and no output, for launchd to start it again", got)
	}
	if _, err := os.Stat(srv.socket()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket once exited: %v, want it gone", err)
	}
	log := srv.routerLog(t)
	for _, want := range [][]string{
		{"level=WARN", `msg="couldn't replace itself; exiting for launchd to start it again" component=router`, "path=" + srv.binary, `error="no test starts a program"`},
		{"level=INFO", "msg=exit component=process", "status=0"},
	} {
		if !hasLine(log, want...) {
			t.Errorf("router.log reads\n%s\nwant a line with %q", log, want)
		}
	}
}

func TestServeRestartsItselfForAChangeMadeAsItStarts(t *testing.T) {
	tests := []struct {
		name string
		// change changes what serve was started from, once it has read its
		// config, before its router runs.
		change     func(srv *serveSetup) error
		wantReason string
	}{
		{
			name: "its config file making another valid config",
			change: func(srv *serveSetup) error {
				srv.extra = "\n[notifications]\nmoves = true\n"
				return srv.putConfig()
			},
			wantReason: `reason="config changed"`,
		},
		{
			name:       "an upgrade moving its binary on",
			change:     (*serveSetup).moveOn,
			wantReason: "reason=upgraded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"XPC_SERVICE_NAME": service.Label})
			srv.watch(t)
			// Serve asks which launchd job it runs as once it has read its
			// config, as its router is about to start.
			var changed sync.Once
			var err error
			getenv := srv.deps.Getenv
			srv.deps.Getenv = func(key string) string {
				if key == "XPC_SERVICE_NAME" {
					changed.Do(func() { err = tt.change(srv) })
				}
				return getenv(key)
			}
			stop := srv.launch(t)

			select {
			case <-srv.exited:
			case <-time.After(5 * time.Second):
				t.Fatal("gave up waiting for switchboard serve to restart")
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := stop(); got != (result{}) {
				t.Errorf("switchboard serve = %+v as it restarted, want exit status 0 and no output, for launchd to start it again", got)
			}
			if log := srv.routerLog(t); !hasLine(log, "level=INFO", "msg=restarting component=router", tt.wantReason) {
				t.Errorf("router.log reads\n%s\nwant a line with %q", log, tt.wantReason)
			}
		})
	}
}

func TestServeRunByHandLogsThatARestartIsDue(t *testing.T) {
	tests := []struct {
		name string
		// job is the launchd job serve runs as, if any.
		job string
	}{
		{name: "at a terminal"},
		{name: "as another launchd job", job: "application.com.apple.Terminal.1234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			if tt.job != "" {
				env["XPC_SERVICE_NAME"] = tt.job
			}
			srv := newServeSetup(t, fakeClaudeAPI(t), env)
			srv.watch(t)
			stop := srv.start(t)

			srv.upgrade(t)
			waitUntil(t, "the router says a restart is due", func() bool {
				return hasLine(srv.routerLog(t), "level=INFO", `msg="restart due; run switchboard serve again to take it up"`, "reason=upgraded")
			})
			if _, err := router.NewClient(srv.socket()).Health(t.Context()); err != nil {
				t.Errorf("the router's health: %v, want it still answering", err)
			}
			if got := stop(); got.code != 0 {
				t.Errorf("switchboard serve = %+v, want exit status 0", got)
			}
			if log := srv.routerLog(t); hasLine(log, "msg=restarting") {
				t.Errorf("router.log reads\n%s\nwant no restart", log)
			}
		})
	}
}

func TestServeMakesTheTokensDirectoryPrivate(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	dir := filepath.Dir(tokenPath(t, srv.deps, "work"))
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	srv.start(t)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != fs.ModeDir|0o700 {
		t.Errorf("the tokens directory has mode %v, want %v", info.Mode(), fs.ModeDir|0o700)
	}
	if log := srv.routerLog(t); !hasLine(log, "level=INFO", `msg="made the tokens directory private"`, "path="+dir, "was=0755") {
		t.Errorf("router.log reads\n%s\nwant the tokens directory made private", log)
	}
}

func TestServeBringsTheSkillUpToDate(t *testing.T) {
	version := strconv.Itoa(skill.Version())
	tests := []struct {
		name string
		// install installs a copy of the skill at path, when it's given.
		install func(t *testing.T, path string)
		wantLog []string
		// wantCurrent is whether the copy there is this switchboard's
		// afterwards.
		wantCurrent bool
	}{
		{
			name: "an older copy",
			install: func(t *testing.T, path string) {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				older := "---\nname: switchboard\n---\n<!-- switchboard skill version: 0 -->\n"
				if err := os.WriteFile(path, []byte(older), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantLog:     []string{"level=INFO", `msg="brought the skill up to date"`, "was=0", "version=" + version},
			wantCurrent: true,
		},
		{
			name: "the copy this switchboard carries",
			install: func(t *testing.T, path string) {
				if err := skill.Install(path); err != nil {
					t.Fatal(err)
				}
			},
			wantLog:     []string{"level=INFO", `msg="the skill is up to date"`, "version=" + version},
			wantCurrent: true,
		},
		{
			name:    "no copy",
			wantLog: []string{"level=INFO", `msg="no skill installed to bring up to date; setup writes it"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeDir := t.TempDir()
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"CLAUDE_CONFIG_DIR": claudeDir})
			path := filepath.Join(claudeDir, "skills", "switchboard", "SKILL.md")
			if tt.install != nil {
				tt.install(t, path)
			}

			srv.start(t)
			if log := srv.routerLog(t); !hasLine(log, append(tt.wantLog, "path="+path)...) {
				t.Errorf("router.log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
			found, err := skill.Refresh(path)
			if err != nil {
				t.Fatal(err)
			}
			if current := found.Installed && !found.Rewritten; current != tt.wantCurrent {
				t.Errorf("once serve has started, the copy there is %+v, want this switchboard's: %v", found, tt.wantCurrent)
			}
		})
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
	// exited closes once the serve start runs has exited.
	exited <-chan struct{}
	// binary is the switchboard binary serve runs as, once watch has it
	// watched: a link to one version, which upgrade moves on to next.
	binary, next string
	// zone is the time zone's file serve reads, once watch has it watched:
	// a link to one zone's, which travel moves on to nextZone.
	zone, nextZone string
}

// watch has serve run as a binary of the test's, which upgrade moves on to
// another version, in a time zone of the test's, which travel moves on to
// another, and look at what it was started from every few milliseconds.
func (s *serveSetup) watch(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	s.binary, s.next = filepath.Join(dir, "bin", "switchboard"), filepath.Join(dir, "1.1", "switchboard")
	s.zone, s.nextZone = filepath.Join(dir, "localtime"), filepath.Join(dir, "zoneinfo", "Asia", "Tokyo")
	current, currentZone := filepath.Join(dir, "1.0", "switchboard"), filepath.Join(dir, "zoneinfo", "Europe", "London")
	files := map[string]string{current: "switchboard 1.0", s.next: "switchboard 1.1", currentZone: "TZif London", s.nextZone: "TZif Tokyo"}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.binary), 0o700); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{s.binary: current, s.zone: currentZone} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	s.deps.Executable = func() (string, error) { return s.binary, nil }
	s.deps.ZoneFile = s.zone
	s.deps.WatchEvery = 10 * time.Millisecond
}

// upgrade has the binary serve runs as lead to its next version, as an
// upgrade does.
func (s *serveSetup) upgrade(t *testing.T) {
	t.Helper()
	if err := s.moveOn(); err != nil {
		t.Fatal(err)
	}
}

// moveOn is upgrade, from any goroutine.
func (s *serveSetup) moveOn() error {
	return relink(s.binary, s.next)
}

// travel has the time zone's file serve reads lead to another zone's, as
// taking the Mac to another time zone does.
func (s *serveSetup) travel(t *testing.T) {
	t.Helper()
	if err := relink(s.zone, s.nextZone); err != nil {
		t.Fatal(err)
	}
}

// relink has the link at path lead to target, replacing it whole.
func relink(path, target string) error {
	moved := path + ".new"
	if err := os.Symlink(target, moved); err != nil {
		return err
	}
	return os.Rename(moved, path)
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
	vars := map[string]string{"SWITCHBOARD_CONFIG": s.config, "XDG_STATE_HOME": stateHome}
	maps.Copy(vars, env)
	s.deps = testDeps(vars, t.TempDir())
	writeToken(t, s.deps, "work", "test-token-work")
	writeToken(t, s.deps, "side", "test-token-side")
	return s
}

// writeConfig writes the config file serve reads.
func (s *serveSetup) writeConfig(t *testing.T) {
	t.Helper()
	if err := s.putConfig(); err != nil {
		t.Fatal(err)
	}
}

// putConfig is writeConfig, from any goroutine.
func (s *serveSetup) putConfig() error {
	content := fmt.Sprintf("listen   = %q\nupstream = %q\n", s.listen, s.upstream) + threeAccounts + s.extra
	return os.WriteFile(s.config, []byte(content), 0o600)
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

// start runs switchboard serve with args, as launch does, returning once its
// control socket answers.
func (s *serveSetup) start(t *testing.T, args ...string) (stop func() result) {
	t.Helper()
	stop = s.launch(t, args...)
	client := router.NewClient(s.socket())
	waitUntil(t, "switchboard serve answers", func() bool {
		select {
		case <-s.exited:
			t.Fatalf("switchboard serve = %+v before it answered", stop())
		default:
		}
		_, err := client.Health(t.Context())
		return err == nil
	})
	return stop
}

// launch runs switchboard serve with args in the background, until the test
// calls the stop it returns, which returns what serve printed and exited
// with, or until the test ends.
func (s *serveSetup) launch(t *testing.T, args ...string) (stop func() result) {
	t.Helper()
	stop, s.exited = launchWith(t, s.deps, append([]string{"serve"}, args...))
	return stop
}

// launchWith runs the command line args with deps in the background, until
// the test calls the stop it returns, which returns what it printed and
// exited with, or until the test ends. What it returns second closes once
// it has exited.
func launchWith(t *testing.T, deps cli.Deps, args []string) (stop func() result, exited <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	root := cli.NewRootCommand(deps)
	root.SetContext(ctx)
	root.SetArgs(args)
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
	return stop, finished
}

// replacement is serve's replacing itself, as the router it runs restarts:
// what it exec'd, and the stop of the serve that took its place.
type replacement struct {
	path      string
	argv, env []string
	stop      func() result
}

// replaceable has serve run as argv, and replace itself, as the router it
// runs restarts, with a serve the test runs as exec would run it, once the
// serve it replaces has gone: in place, with the command line and the
// environment exec is given, but by a clock a minute on, so it's told from
// the serve it replaced. The replacement is sent once it runs.
func (s *serveSetup) replaceable(t *testing.T, argv ...string) <-chan replacement {
	t.Helper()
	made := make(chan replacement, 1)
	s.deps.Args = argv
	s.deps.Exec = func(path string, argv, env []string) error {
		next := s.deps
		vars := environMap(env)
		next.Getenv = func(key string) string { return vars[key] }
		next.Environ = func() []string { return env }
		next.Args = argv
		next.Now = func() time.Time { return testNow.Add(time.Minute) }
		next.Exec = func(string, []string, []string) error { return errors.New("no test starts a program") }
		replaced := s.exited
		go func() {
			<-replaced
			stop, _ := launchWith(t, next, cli.Args(argv))
			made <- replacement{path: path, argv: argv, env: env, stop: stop}
		}()
		return nil
	}
	return made
}

// waitForReplacement waits a few seconds at most for serve to have replaced
// itself, and returns the replacement.
func waitForReplacement(t *testing.T, made <-chan replacement) replacement {
	t.Helper()
	select {
	case r := <-made:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("gave up waiting for switchboard serve to replace itself")
		return replacement{}
	}
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

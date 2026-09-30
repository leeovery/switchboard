package router_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// watchEvery is how often the routers these tests run look at what they were
// started from.
const watchEvery = 10 * time.Millisecond

// Configs the tests' config file holds: two valid, and one that isn't.
const (
	oneAccount  = "[[account]]\nid = \"work\"\n"
	twoAccounts = "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n"
	notAConfig  = "[[account]]\nid = \"work\"\ncolour = \"red\"\n"
)

func TestASupervisedRouterRestartsItself(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router was started from.
		change     func(t *testing.T, s *selfWatching)
		wantReason string
	}{
		{
			name:       "once its config file makes another valid config",
			change:     func(t *testing.T, s *selfWatching) { writeFile(t, s.config, twoAccounts) },
			wantReason: `reason="config changed"`,
		},
		{
			name:       "once its binary leads to another file, as after an upgrade",
			change:     func(t *testing.T, s *selfWatching) { s.upgrade(t) },
			wantReason: "reason=upgraded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newSelfWatching(t, true)
			r := startRouter(t, s.cfg)

			tt.change(t, s)
			r.waitForExit(t)
			for _, want := range [][]string{
				{"level=INFO", `msg="restart due; restarting once no request is in flight"`, tt.wantReason},
				{"level=INFO", "msg=restarting", tt.wantReason},
				{"level=INFO", "msg=stopping"},
				{"level=INFO", "msg=stopped"},
			} {
				if !log.Has(want...) {
					t.Errorf("log reads\n%s\nwant a line with %q", log, want)
				}
			}
			if _, err := os.Stat(filepath.Join(s.cfg.StateDir, "state.json")); err != nil {
				t.Errorf("state file: %v, want it saved on the way out", err)
			}
		})
	}
}

func TestARouterCarriesOnThroughAConfigChangeThatIsntValid(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, true)
	r := startRouter(t, s.cfg)

	writeFile(t, s.config, notAConfig)
	waitForLine(t, log, "level=WARN", `msg="config change refused; carrying on with the config as it was"`, "path="+s.config)
	time.Sleep(10 * watchEvery)
	r.checkRunning(t)
	if log.Has(`msg="restart due`) {
		t.Errorf("log reads\n%s\nwant no restart due", log)
	}

	writeFile(t, s.config, twoAccounts)
	r.waitForExit(t)
}

func TestARestartWaitsForTheRequestsInFlight(t *testing.T) {
	log := logstest.Capture(t)
	arrived, release := make(chan struct{}), make(chan struct{})
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-release
		answerOK(w, r)
	})
	s := newSelfWatching(t, true)
	s.cfg.Upstream = up.URL
	r := startRouter(t, s.cfg)
	answered := make(chan string, 1)
	go func() { answered <- post("http://" + s.cfg.Listen + "/v1/messages") }()
	<-arrived

	writeFile(t, s.config, twoAccounts)
	waitForLine(t, log, `msg="restart due; restarting once no request is in flight"`)
	time.Sleep(10 * watchEvery)
	r.checkRunning(t)
	close(release)
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	r.waitForExit(t)
}

func TestARouterRunByHandSaysARestartIsDueOnce(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, false)
	r := startRouter(t, s.cfg)

	writeFile(t, s.config, twoAccounts)
	waitForLine(t, log, "level=INFO", `msg="restart due; run switchboard serve again to take it up"`, `reason="config changed"`)
	s.upgrade(t)
	writeFile(t, s.config, oneAccount)
	time.Sleep(10 * watchEvery)
	r.checkRunning(t)
	if n := strings.Count(log.String(), `msg="restart due`); n != 1 {
		t.Errorf("log reads\n%s\nwant the restart due said once, not %d times", log, n)
	}
	if log.Has("msg=restarting") {
		t.Errorf("log reads\n%s\nwant no restart", log)
	}
}

func TestTheRouterTakesUpTokenFilesAsTheyChange(t *testing.T) {
	const renewed = "test-token-work-renewed"
	log := logstest.Capture(t)
	api := newAccountsAPI(t)
	store, files := withTokenFiles(t)
	cfg := runConfig(t, api.URL)
	files(&cfg)
	cfg.WatchEvery = watchEvery
	prober := &fakeProber{}
	cfg.Prober = prober
	// Personal's quota, once it has a token, needs using first, then work's.
	for token, week := range map[string]quota.Window{
		personalToken: weekOf(0.5, 24*time.Hour),
		workToken:     weekOf(0.5, 2*24*time.Hour),
		renewed:       weekOf(0.5, 2*24*time.Hour),
		sideToken:     weekOf(0.5, 5*24*time.Hour),
	} {
		prober.answer(token, probeResult{usage: quota.Usage{Windows: []quota.Window{session, week}}})
		api.set(token, session, week)
	}
	runRouter(t, cfg)
	proxy := "http://" + cfg.Listen
	// ask sends a request of the session given on token, and returns the
	// token it went out on.
	ask := func(token, session string) string {
		t.Helper()
		readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", with(claudeCode(token), claude.SessionHeader, session), strings.NewReader(messages)))
		bearers := api.bearers()
		return bearers[len(bearers)-1]
	}

	writeToken(t, store, "work", renewed)
	waitForLine(t, log, "level=INFO", `msg="token replaced; the one before still counts as the account's"`, "account=work")
	if got := ask(workToken, "before"); got != renewed {
		t.Errorf("a session started on work's token before it was replaced went out on %q, want work's new one", got)
	}
	waitForLine(t, log, "msg=routed", "session=before", "account=work", "status=200")

	writeToken(t, store, "personal", personalToken)
	waitForLine(t, log, "level=INFO", `msg="account has a usable token; requests can go out on it"`, "account=personal")
	if got := ask(sideToken, "new"); got != personalToken {
		t.Errorf("a new session went out on %q, want personal's token: its quota needs using first once it has one", got)
	}

	if _, err := store.Remove("personal"); err != nil {
		t.Fatal(err)
	}
	waitForLine(t, log, "level=INFO", `msg="account has no usable token; nothing will go out on it until it's back"`, "account=personal")
	doc := waitForStatus(t, router.SocketPath(cfg.StateDir), func(doc status.Document) bool {
		personal, _ := doc.Account("personal")
		return !personal.TokenSet
	})
	if personal, _ := doc.Account("personal"); !strings.HasPrefix(personal.Error, "token missing: write it to ") {
		t.Errorf("personal, its token file gone, reads %+v, want why it has no token", personal)
	}
	if got := ask(sideToken, "another"); got != renewed {
		t.Errorf("a new session went out on %q, want work's token: personal has nothing to send on", got)
	}
	if got := ask(sideToken, "new"); got != renewed {
		t.Errorf("the session on personal went out on %q, want work's token: personal can't serve it", got)
	}
	waitForLine(t, log, "msg=routed", "session=new", "account=work", `reason="moved: personal has no room"`)
	checkNoTokenIn(t, log, workToken, renewed, personalToken, sideToken)
}

// selfWatching is what a router that looks after itself is started with: a
// valid config file, and its binary, a link to one version of it.
type selfWatching struct {
	cfg router.Config
	// config is the config file's path, and binary the binary's.
	config, binary string
	// next is the binary's next version, which an upgrade leads its link to.
	next string
}

// newSelfWatching returns what a router, supervised or not, is started with
// to look after itself, looking every watchEvery.
func newSelfWatching(t *testing.T, supervised bool) *selfWatching {
	t.Helper()
	dir := t.TempDir()
	s := &selfWatching{
		cfg:    runConfig(t, "http://127.0.0.1:1"),
		config: filepath.Join(dir, "config.toml"),
		binary: filepath.Join(dir, "bin", "switchboard"),
		next:   filepath.Join(dir, "1.1", "switchboard"),
	}
	writeFile(t, s.config, oneAccount)
	current := filepath.Join(dir, "1.0", "switchboard")
	writeFile(t, current, "switchboard 1.0")
	writeFile(t, s.next, "switchboard 1.1")
	if err := os.MkdirAll(filepath.Dir(s.binary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(current, s.binary); err != nil {
		t.Fatal(err)
	}
	s.cfg.ConfigFile, s.cfg.Binary = router.Watch(s.config), router.Watch(s.binary)
	s.cfg.Supervised = supervised
	s.cfg.WatchEvery = watchEvery
	return s
}

// upgrade has the binary's link lead to its next version, as an upgrade
// does.
func (s *selfWatching) upgrade(t *testing.T) {
	t.Helper()
	moved := s.binary + ".new"
	if err := os.Symlink(s.next, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, s.binary); err != nil {
		t.Fatal(err)
	}
}

// writeFile writes content to the file at path, in place of what it held,
// making its directory first.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// startedRouter is a router Run runs in the background, until the test ends
// or it stops by itself.
type startedRouter struct {
	finished chan struct{}
	err      error
}

// startRouter runs a router with cfg, returning once its control socket
// answers.
func startRouter(t *testing.T, cfg router.Config) *startedRouter {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	r := &startedRouter{finished: make(chan struct{})}
	go func() {
		r.err = router.Run(ctx, cfg)
		close(r.finished)
	}()
	t.Cleanup(func() {
		cancel()
		<-r.finished
	})
	client := router.NewClient(router.SocketPath(cfg.StateDir))
	waitUntil(t, "the router answers", func() bool {
		select {
		case <-r.finished:
			t.Fatalf("Run() = %v before it answered", r.err)
		default:
		}
		_, err := client.Health(ctx)
		return err == nil
	})
	return r
}

// waitForExit waits a few seconds at most for Run to return by itself, as a
// router restarting does, and checks it returned nil.
func (r *startedRouter) waitForExit(t *testing.T) {
	t.Helper()
	select {
	case <-r.finished:
		if r.err != nil {
			t.Errorf("Run() = %v as the router restarted, want nil", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gave up waiting for the router to restart")
	}
}

// checkRunning checks Run hasn't returned.
func (r *startedRouter) checkRunning(t *testing.T) {
	t.Helper()
	select {
	case <-r.finished:
		t.Fatalf("Run() = %v, want the router running", r.err)
	default:
	}
}

// checkNoTokenIn checks the log shows none of the tokens given.
func checkNoTokenIn(t *testing.T, log *logstest.Log, tokens ...string) {
	t.Helper()
	for _, token := range tokens {
		if strings.Contains(log.String(), token) {
			t.Errorf("log reads\n%s\nwhich shows a token", log)
		}
	}
}

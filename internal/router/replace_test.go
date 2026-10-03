package router_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
)

func TestARestartingRouterReplacesItselfInPlace(t *testing.T) {
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
		{
			name:       "once the time zone's file leads to another, as in another time zone",
			change:     func(t *testing.T, s *selfWatching) { s.travel(t) },
			wantReason: `reason="time zone changed"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newSelfWatching(t, true)
			s.cfg.Upstream = newUpstream(t, answerOK).URL
			execs := replacing(&s.cfg)
			r := startRouter(t, s.cfg)
			readAll(t, send(t, http.MethodPost, "http://"+s.cfg.Listen+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))

			tt.change(t, s)
			r.waitForExit(t)
			made := execs.only(t)
			if made.path != s.binary || !slices.Equal(made.argv, s.cfg.Args) {
				t.Errorf("replaced itself with %s run as %q, want its binary, by the path it was started as, %s, run as it was, %q", made.path, made.argv, s.binary, s.cfg.Args)
			}
			handed := handover.Named(made.env)
			if want := append(slices.Clone(s.cfg.Environ), handover.Variable+"="+handed); handed == "" || !slices.Equal(made.env, want) {
				t.Errorf("replaced itself in the environment %q, want the one it started in, naming the listeners it handed over", made.env)
			}
			if !strings.Contains(made.saved, sessionID) {
				t.Errorf("as it replaced itself, the state file held\n%s\nwant the session it routed saved in it", made.saved)
			}
			for _, want := range [][]string{
				{"level=INFO", "msg=restarting", tt.wantReason},
				{"level=INFO", "msg=stopped"},
				{"level=INFO", `msg="replacing itself"`, "path=" + s.binary, tt.wantReason, `listeners="` + handed + `"`},
			} {
				if !log.Has(want...) {
					t.Errorf("log reads\n%s\nwant a line with %q", log, want)
				}
			}

			// A request sent to the proxy before the router it becomes takes
			// the listeners up waits for it, rather than being refused.
			waiting := sendWaiting(t, s.cfg.Listen)
			if _, err := os.Stat(router.SocketPath(s.cfg.StateDir)); err != nil {
				t.Errorf("the control socket between the routers: %v, want it kept for the next", err)
			}
			// The router it becomes finds what it's started from afresh, as
			// switchboard serve does: what this one found would have it
			// restart again at its first look.
			next := s.watched(s.cfg)
			next.Exec, next.Handed = nil, handed
			startRouter(t, next)
			if got := answerOn(t, waiting); got != `200 {"type":"message"}` {
				t.Errorf("the request sent between the routers was answered %q, want 200 and its body", got)
			}
			for _, listener := range []string{"listener=proxy address=" + s.cfg.Listen, "listener=control address=" + router.SocketPath(s.cfg.StateDir)} {
				if !log.Has("level=INFO", `msg="took up the listener handed over"`, listener) {
					t.Errorf("log reads\n%s\nwant the %s taken up", log, listener)
				}
			}
			if log.Has(`msg="removed a stale control socket"`) {
				t.Errorf("log reads\n%s\nwant the control socket taken up, not replaced", log)
			}
		})
	}
}

func TestARouterThatCantReplaceItselfExitsForLaunchdToStartItAgain(t *testing.T) {
	tests := []struct {
		name string
		// unknown has the router not know its binary, and exec is how
		// replacing the process fails.
		unknown   bool
		exec      error
		wantExecs int
		wantLog   []string
	}{
		{
			name:      "its binary not there to exec, try after try",
			exec:      syscall.ENOENT,
			wantExecs: 5,
			wantLog:   []string{"level=WARN", `msg="couldn't replace itself; exiting for launchd to start it again"`, `error="no such file or directory"`},
		},
		{
			name:      "its binary refused",
			exec:      syscall.EACCES,
			wantExecs: 1,
			wantLog:   []string{"level=WARN", `msg="couldn't replace itself; exiting for launchd to start it again"`, `error="permission denied"`},
		},
		{
			name:    "not knowing its binary",
			unknown: true,
			wantLog: []string{"level=WARN", `msg="can't replace itself, not knowing its binary; exiting for launchd to start it again"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newSelfWatching(t, true)
			if tt.unknown {
				s.cfg.Binary = router.Watched{}
			}
			execs := replacing(&s.cfg, tt.exec)
			r := startRouter(t, s.cfg)

			writeFile(t, s.config, twoAccounts)
			r.waitForExit(t)
			if tried := execs.count(); tried != tt.wantExecs {
				t.Errorf("tried to replace itself %d times, want %d", tried, tt.wantExecs)
			}
			if !log.Has(tt.wantLog...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
			if _, err := os.Stat(router.SocketPath(s.cfg.StateDir)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("control socket once exited: %v, want it gone, as a router stopping leaves it", err)
			}
			if conn, err := net.Dial("tcp", s.cfg.Listen); err == nil {
				_ = conn.Close()
				t.Error("the proxy's address takes a connection once the router exited, want nothing left listening")
			}
		})
	}
}

func TestARouterToldToStopAsItFinishesItsRequestsToRestartStopsAtOnce(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, true)
	var arrived <-chan struct{}
	var release func()
	s.cfg.Upstream, arrived, release = holdingUpstream(t)
	execs := replacing(&s.cfg)
	stop, finished := runStoppable(t, s.cfg)
	client := router.NewClient(router.SocketPath(s.cfg.StateDir))
	answered := make(chan string, 1)
	go func() { answered <- post("http://" + s.cfg.Listen + "/v1/messages") }()
	<-arrived

	if _, err := client.Restart(t.Context()); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	waitForLine(t, log, "level=INFO", "msg=stopping")
	stop()
	// With the request still in flight, launchers find the router gone, and
	// its proxy takes nothing more, as at any signal.
	waitUntil(t, "the router stops answering", func() bool {
		_, err := client.Health(t.Context())
		return errors.Is(err, router.ErrNotRunning)
	})
	if conn, err := net.Dial("tcp", s.cfg.Listen); err == nil {
		_ = conn.Close()
		t.Error("the proxy's address takes a connection once the router was told to stop, want it refused")
	}
	release()
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	if err := <-finished; err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
	if made := execs.count(); made != 0 {
		t.Errorf("replaced itself %d times, want none: it was told to stop", made)
	}
	if !log.Has("level=INFO", `msg="told to stop as it restarted; stopping instead"`) {
		t.Errorf("log reads\n%s\nwant the router stopping", log)
	}
}

func TestARouterTriesItsBinaryAgainWhileItIsntThere(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, true)
	execs := replacing(&s.cfg, syscall.ENOENT, syscall.ENOENT, nil)
	r := startRouter(t, s.cfg)

	s.upgrade(t)
	r.waitForExit(t)
	execs.last(t, 3)
	for _, try := range []string{"try=1", "try=2"} {
		if !log.Has("level=INFO", `msg="its binary isn't there; trying again"`, "path="+s.binary, try) {
			t.Errorf("log reads\n%s\nwant a line saying it tries again, %s", log, try)
		}
	}
	if log.Has(`msg="couldn't replace itself`) {
		t.Errorf("log reads\n%s\nwant the router replaced once its binary was there", log)
	}
	if _, err := os.Stat(router.SocketPath(s.cfg.StateDir)); err != nil {
		t.Errorf("the control socket once replaced: %v, want it kept for the router it became", err)
	}
}

func TestARouterToldToStopAsItWaitsForItsBinaryExitsForGood(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, true)
	execs := replacing(&s.cfg, syscall.ENOENT)
	s.cfg.ExecRetry = time.Hour
	stop, finished := runStoppable(t, s.cfg)

	s.upgrade(t)
	waitForLine(t, log, "level=INFO", `msg="its binary isn't there; trying again"`)
	stop()
	select {
	case err := <-finished:
		if err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gave up waiting for the router to exit")
	}
	execs.only(t)
	if !log.Has("level=INFO", `msg="told to stop as it restarted; exiting"`) {
		t.Errorf("log reads\n%s\nwant the router exiting", log)
	}
	if _, err := os.Stat(router.SocketPath(s.cfg.StateDir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket once exited: %v, want it gone", err)
	}
}

func TestARouterSaysWhetherItRestartsInPlace(t *testing.T) {
	tests := []struct {
		name string
		// unknown has the router not know its binary, and noExec gives it
		// nothing to exec with.
		unknown, noExec bool
		want            bool
	}{
		{name: "knowing its binary", want: true},
		{name: "not knowing its binary", unknown: true},
		{name: "with nothing to exec with", noExec: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSelfWatching(t, true)
			replacing(&s.cfg)
			if tt.unknown {
				s.cfg.Binary = router.Watched{}
			}
			if tt.noExec {
				s.cfg.Exec = nil
			}
			r := startRouter(t, s.cfg)

			got, err := router.NewClient(router.SocketPath(s.cfg.StateDir)).Restart(t.Context())
			if err != nil {
				t.Fatalf("Restart() error = %v", err)
			}
			if got.InPlace != tt.want || got.PID != os.Getpid() || !got.StartedAt.Equal(now) {
				t.Errorf("Restart() = %+v, want the router taking the request, restarting in place: %v", got, tt.want)
			}
			r.waitForExit(t)
		})
	}
}

func TestAskedToRestartARouterFinishesItsRequestsThenReplacesItself(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, true)
	var arrived <-chan struct{}
	var release func()
	s.cfg.Upstream, arrived, release = holdingUpstream(t)
	execs := replacing(&s.cfg)
	r := startRouter(t, s.cfg)
	client := router.NewClient(router.SocketPath(s.cfg.StateDir))
	answered := make(chan string, 1)
	go func() { answered <- post("http://" + s.cfg.Listen + "/v1/messages") }()
	<-arrived

	if _, err := client.Restart(t.Context()); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	waitForLine(t, log, "level=INFO", "msg=restarting", `reason="asked to"`)
	time.Sleep(10 * watchEvery)
	r.checkRunning(t)
	if _, err := client.Health(t.Context()); err != nil {
		t.Errorf("the router's health while it finishes its requests: %v, want it answering, for launches to go on sending sessions to it", err)
	}
	if made := execs.count(); made != 0 {
		t.Fatalf("replaced itself %d times with a request still in flight, want none", made)
	}
	release()
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	r.waitForExit(t)
	execs.only(t)
	if !log.Has("level=INFO", `msg="replacing itself"`, `reason="asked to"`) {
		t.Errorf("log reads\n%s\nwant the router replacing itself", log)
	}
}

func TestARouterRefusesToRestartWhenItCant(t *testing.T) {
	tests := []struct {
		name       string
		supervised bool
		// change changes what the router was started from.
		change func(t *testing.T, s *selfWatching)
		want   string
	}{
		{
			name: "run by hand",
			want: "the router was run by hand, with switchboard serve, so nothing would start it again: stop it, and run it again",
		},
		{
			name:       "its config file invalid",
			supervised: true,
			change:     func(t *testing.T, s *selfWatching) { writeFile(t, s.config, notAConfig) },
			want:       "the router's config file doesn't make a valid config, which it couldn't start again from: invalid config ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSelfWatching(t, tt.supervised)
			execs := replacing(&s.cfg)
			r := startRouter(t, s.cfg)
			if tt.change != nil {
				tt.change(t, s)
			}

			_, err := router.NewClient(router.SocketPath(s.cfg.StateDir)).Restart(t.Context())
			if err == nil || errors.Is(err, router.ErrNoRestart) || !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("Restart() error = %v, want it refused: %s", err, tt.want)
			}
			time.Sleep(10 * watchEvery)
			r.checkRunning(t)
			if made := execs.count(); made != 0 {
				t.Errorf("replaced itself %d times, want none", made)
			}
		})
	}
}

func TestClientRestartOfARouterThatCantBeAsked(t *testing.T) {
	tests := []struct {
		name string
		// serve serves the socket at path, if anything does.
		serve func(t *testing.T, path string)
	}{
		{name: "none running"},
		{
			name: "one from before routers restarted when asked",
			serve: func(t *testing.T, path string) {
				mux := http.NewServeMux()
				mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"ok": true}`) })
				serveOn(t, path, mux)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(shortTempDir(t), "control.sock")
			if tt.serve != nil {
				tt.serve(t, path)
			}

			if _, err := router.NewClient(path).Restart(t.Context()); !errors.Is(err, router.ErrNoRestart) {
				t.Errorf("Restart() error = %v, want ErrNoRestart", err)
			}
		})
	}
}

func TestARouterTakesUpTheListenersHandedOverWhereItListens(t *testing.T) {
	log := logstest.Capture(t)
	cfg := runConfig(t, newUpstream(t, answerOK).URL)
	socket := router.SocketPath(cfg.StateDir)
	moved := cfg.Listen
	cfg.Listen = freeAddress(t)
	handed := handOver(t, map[string]string{"proxy": moved, "control": socket})
	cfg.Handed = handed

	runRouter(t, cfg)
	if got := post("http://" + cfg.Listen + "/v1/messages"); !strings.HasPrefix(got, "200 ") {
		t.Errorf("a request where the config says the proxy listens answered %s, want 200", got)
	}
	if conn, err := net.Dial("tcp", moved); err == nil {
		_ = conn.Close()
		t.Errorf("a connection to %s, where the proxy handed over listened, was taken, want it refused: the config moved the proxy", moved)
	}
	for _, want := range [][]string{
		{"level=INFO", `msg="closed a listener handed over, as the router doesn't listen there"`, "listener=proxy", "address=" + moved},
		{"level=INFO", `msg="took up the listener handed over"`, "listener=control", "address=" + socket},
		{"level=INFO", "msg=listening", "address=" + cfg.Listen, "control=" + socket},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
}

func TestARouterListensAfreshForListenersNotHandedOver(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "router.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	tests := []struct {
		name    string
		handed  string
		wantLog string
	}{
		{name: "a value not in the variable's form", handed: "proxy=nine", wantLog: "isn't listeners' names and descriptors"},
		{name: "a descriptor that isn't a listener", handed: "proxy=" + strconv.Itoa(int(file.Fd())), wantLog: "not a socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			cfg := runConfig(t, newUpstream(t, answerOK).URL)
			cfg.Handed = tt.handed

			runRouter(t, cfg)
			if got := post("http://" + cfg.Listen + "/v1/messages"); !strings.HasPrefix(got, "200 ") {
				t.Errorf("a request through the proxy answered %s, want 200", got)
			}
			if !log.Has("level=WARN", `msg="couldn't take up the listeners handed over; listening afresh"`, tt.wantLog) {
				t.Errorf("log reads\n%s\nwant why the listeners handed over were passed over", log)
			}
			if _, err := file.Stat(); err != nil {
				t.Errorf("the file whose descriptor was named: %v, want it left open", err)
			}
		})
	}
}

// holdingUpstream serves an upstream that holds each request until release
// is called, or the test ends, and answers it as answerOK does, and returns
// its URL, with what's closed as the first request arrives.
func holdingUpstream(t *testing.T) (url string, arrived <-chan struct{}, release func()) {
	t.Helper()
	first, released := make(chan struct{}), make(chan struct{})
	var arriving sync.Once
	release = sync.OnceFunc(func() { close(released) })
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		arriving.Do(func() { close(first) })
		<-released
		answerOK(w, r)
	})
	t.Cleanup(release)
	return up.URL, first, release
}

// runStoppable runs a router with cfg, returning once its control socket
// answers, with what stops it, as a signal does, and what Run's answer comes
// on.
func runStoppable(t *testing.T, cfg router.Config) (stop context.CancelFunc, finished <-chan error) {
	t.Helper()
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- router.Run(ctx, cfg) }()
	t.Cleanup(stop)
	client := router.NewClient(router.SocketPath(cfg.StateDir))
	waitUntil(t, "the router answers", func() bool {
		_, err := client.Health(t.Context())
		return err == nil
	})
	return stop, done
}

// execCall is an exec a router made as it replaced itself: what it ran, and
// what the state file held by then.
type execCall struct {
	path      string
	argv, env []string
	saved     string
}

// execs stands in for exec, noting each made, and answering each in turn as
// answers says, and, past its end, as its last does: nil, for none, as an
// exec that replaced the process does, handing over what it was handed.
type execs struct {
	stateFile string
	answers   []error

	mu   sync.Mutex
	made []execCall
}

// replacing has a router run with cfg replace itself through execs answering
// as answers says, run with a command line and an environment of the test's,
// and trying again at once while its binary isn't there.
func replacing(cfg *router.Config, answers ...error) *execs {
	e := &execs{stateFile: filepath.Join(cfg.StateDir, "state.json"), answers: answers}
	cfg.Exec = e.exec
	cfg.Args = []string{"/test/bin/switchboard", "serve"}
	cfg.Environ = []string{"HOME=/home/tester", "XPC_SERVICE_NAME=io.github.leeovery.switchboard"}
	cfg.ExecRetry = time.Millisecond
	return e
}

func (e *execs) exec(path string, argv, env []string) error {
	saved, _ := os.ReadFile(e.stateFile)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.made = append(e.made, execCall{path: path, argv: argv, env: env, saved: string(saved)})
	if len(e.answers) == 0 {
		return nil
	}
	return e.answers[min(len(e.made), len(e.answers))-1]
}

// count is how many execs have been made.
func (e *execs) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.made)
}

// only returns the one exec made.
func (e *execs) only(t *testing.T) execCall {
	t.Helper()
	return e.last(t, 1)
}

// last returns the last of the n execs made.
func (e *execs) last(t *testing.T, n int) execCall {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.made) != n {
		t.Fatalf("tried to replace itself %d times, want %d", len(e.made), n)
	}
	return e.made[n-1]
}

// handOver hands over a listener at each address given, by name, as a router
// replacing itself does, and returns what names them: a unix socket's
// address is a path.
func handOver(t *testing.T, addresses map[string]string) string {
	t.Helper()
	listeners := make(map[string]net.Listener)
	for name, address := range addresses {
		network := "tcp"
		if filepath.IsAbs(address) {
			network = "unix"
			if err := os.MkdirAll(filepath.Dir(address), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		ln, err := net.Listen(network, address)
		if err != nil {
			t.Fatal(err)
		}
		listeners[name] = ln
	}
	held, err := handover.Hold(listeners)
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	defer held.Close()
	for _, ln := range listeners {
		_ = ln.Close()
	}
	recorded := &execs{}
	if err := held.Exec(recorded.exec, "/test/bin/switchboard", nil, nil); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	return handover.Named(recorded.only(t).env)
}

// sendWaiting sends a messages request, on work's token, on a connection of
// its own to the proxy at address, and returns the connection, for the
// answer to be read once a router takes it.
func sendWaiting(t *testing.T, address string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("a connection to the proxy between routers: %v, want it waiting to be accepted", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	req, err := http.NewRequest(http.MethodPost, "http://"+address+"/v1/messages", strings.NewReader(messages))
	if err != nil {
		t.Fatal(err)
	}
	req.Header = claudeCode(workToken)
	if err := req.Write(conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

// answerOn returns the status and body of the answer to the request sent on
// conn, waiting a few seconds at most.
func answerOn(t *testing.T, conn net.Conn) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return err.Error()
	}
	return strconv.Itoa(resp.StatusCode) + " " + readAll(t, resp)
}

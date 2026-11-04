package cli_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/service"
)

// What serve logs as it moves over to the router's copy, and where it can't.
const (
	movingOverLine   = `msg="moving over to the router's own copy of switchboard" component=cli`
	cantMoveOverLine = `msg="can't move over to the router's own copy of switchboard; carrying on from the binary it was started as" component=cli`
)

// addedByHand is a variable added by hand to the plist switchboard wrote
// before the router's copy.
const addedByHand = "<key>HTTPS_PROXY</key>\n\t\t<string>http://proxy.example:8080</string>"

func TestServeRunByAServiceFromBeforeTheCopyMovesOverToIt(t *testing.T) {
	s := newMovingOver(t)
	argv := []string{s.srv.binary, "serve"}
	made := s.srv.replaceable(t, argv...)
	environ := s.srv.deps.Environ()
	// Sent as a claude started meanwhile sends it.
	waiting := s.srv.sendWaiting(t)

	stop := s.srv.launch(t)

	replaced := waitForReplacement(t, made)
	if got := stop(); got != (result{}) {
		t.Errorf("switchboard serve = %+v as it moved over, want exit status 0 and no output", got)
	}
	want := []string{s.routerCopy, "serve", "--from", s.srv.binary}
	if replaced.path != s.routerCopy || !slices.Equal(replaced.argv, want) {
		t.Errorf("serve replaced itself with %s run as %q, want its copy, %s, run as it was started, with --from naming the link it was started as: %q", replaced.path, replaced.argv, s.routerCopy, want)
	}
	if got, want := environMap(replaced.env), environMap(environ); !maps.Equal(got, want) {
		t.Errorf("serve replaced itself in the environment %v, want the one it was started in, naming the listeners handed over to it as they were: %v", got, want)
	}
	if got := answerOn(t, waiting); got != http.StatusOK {
		t.Errorf("the request sent as serve moved over was answered %d, want 200, having waited for the router serve became", got)
	}
	s.checkPlist(t,
		"<string>"+s.routerCopy+"</string>\n\t\t<string>serve</string>\n\t\t<string>--from</string>\n\t\t<string>"+s.srv.binary+"</string>\n\t</array>",
		addedByHand, "<key>ExitTimeOut</key>\n\t<integer>45</integer>", "<key>ProcessType</key>\n\t<string>Interactive</string>")
	if copied, err := os.ReadFile(s.routerCopy); err != nil || string(copied) != "switchboard 1.0" {
		t.Errorf("the router's copy holds %q (%v), want a copy of the version the link leads to", copied, err)
	}
	if s.launchd.calls != nil {
		t.Errorf("ran launchctl %q, want launchd left alone: it reads the plist only as it loads the service", s.launchd.calls)
	}
	if got := replaced.stop(); got.code != 0 {
		t.Errorf("the switchboard serve it became = %+v, want exit status 0", got)
	}
	log := s.srv.routerLog(t)
	if n := strings.Count(log, movingOverLine); n != 1 {
		t.Errorf("router.log reads\n%s\nwant serve moving over once, and the copy it became never again, not %d times", log, n)
	}
	for _, want := range [][]string{
		{"level=INFO", movingOverLine, "copy=" + s.routerCopy, "from=" + s.srv.binary, `listeners="` + s.handed + `"`},
		{"level=INFO", `msg="took up the listener handed over" component=router`, "listener=proxy", "address=" + s.srv.listen},
		{"level=INFO", `msg="took up the listener handed over" component=router`, "listener=control", "address=" + s.srv.socket()},
	} {
		if !hasLine(log, want...) {
			t.Errorf("router.log reads\n%s\nwant a line with %q", log, want)
		}
	}
}

func TestServeThatCantMoveOverCarriesOnFromTheBinaryItWasStartedAs(t *testing.T) {
	tests := []struct {
		name string
		// keep keeps serve from moving over.
		keep func(t *testing.T, s *movingOver)
		// why is what the warning says of why, in part.
		why string
	}{
		{
			name: "the plist can't be read back",
			keep: func(t *testing.T, s *movingOver) { s.putUnreadablePlist(t) },
			why:  "through plutil",
		},
		{
			name: "the copy can't be made",
			keep: func(t *testing.T, s *movingOver) {
				// A file where the copy's directory goes.
				if err := os.MkdirAll(filepath.Dir(filepath.Dir(s.routerCopy)), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Dir(s.routerCopy), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			why: "beside the router's copy",
		},
		{
			name: "the plist can't be written",
			keep: func(t *testing.T, s *movingOver) {
				dir := filepath.Dir(s.plist)
				if err := os.Chmod(dir, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			},
			why: "write the service's plist",
		},
		{
			name: "the copy can't be run",
			keep: func(_ *testing.T, s *movingOver) {
				s.srv.deps.Exec = func(string, []string, []string) error { return errors.New("exec refused") }
			},
			why: "exec refused",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newMovingOver(t)
			tt.keep(t, s)
			was := s.readPlist(t)
			waiting := s.srv.sendWaiting(t)

			stop := s.srv.start(t)

			if got := answerOn(t, waiting); got != http.StatusOK {
				t.Errorf("the request sent as serve started was answered %d, want 200, by the router it runs", got)
			}
			if got := stop(); got != (result{}) {
				t.Errorf("switchboard serve = %+v, want exit status 0 and no output", got)
			}
			if plist := s.readPlist(t); plist != was {
				t.Errorf("the plist reads\n%s\nwant it as it was:\n%s", plist, was)
			}
			if s.launchd.calls != nil {
				t.Errorf("ran launchctl %q, want launchd left alone", s.launchd.calls)
			}
			log := s.srv.routerLog(t)
			warning := []string{"level=WARN", cantMoveOverLine, "binary=" + s.srv.binary, tt.why}
			if n := strings.Count(log, cantMoveOverLine); n != 1 || !hasLine(log, warning...) {
				t.Errorf("router.log reads\n%s\nwant one warning, with %q", log, warning)
			}
			if strings.Contains(log, tokenShaped) {
				t.Errorf("router.log shows something token-shaped:\n%s", log)
			}
			for _, listener := range []string{"listener=proxy", "listener=control"} {
				if !hasLine(log, `msg="took up the listener handed over" component=router`, listener) {
					t.Errorf("router.log reads\n%s\nwant the router serve runs taking up the listener handed over, %s", log, listener)
				}
			}
		})
	}
}

func TestServeMovesOverOnlyAsTheServiceRunsItWithoutFrom(t *testing.T) {
	tests := []struct {
		name string
		// job is the launchd job serve runs as, "" for none, and args what
		// it's run with, each <binary> the link it's started as.
		job  string
		args []string
	}{
		{name: "run by the service with --from", job: service.Label, args: []string{"--from", "<binary>"}},
		{name: "run by hand"},
		{name: "run as another launchd job", job: "application.com.apple.Terminal.1234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newMovingOver(t)
			s.setenv("XPC_SERVICE_NAME", tt.job)
			args := make([]string, len(tt.args))
			for i, arg := range tt.args {
				args[i] = strings.ReplaceAll(arg, "<binary>", s.srv.binary)
			}

			stop := s.srv.start(t, args...)

			if got := stop(); got != (result{}) {
				t.Errorf("switchboard serve = %+v, want exit status 0 and no output", got)
			}
			if plist := s.readPlist(t); plist != s.before {
				t.Errorf("the plist reads\n%s\nwant it as it was:\n%s", plist, s.before)
			}
			if _, err := os.Stat(s.routerCopy); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the router's copy: %v, want none made", err)
			}
			if log := s.srv.routerLog(t); strings.Contains(log, "move over") || strings.Contains(log, "moving over") {
				t.Errorf("router.log reads\n%s\nwant nothing of moving over", log)
			}
		})
	}
}

// movingOver is serve run by launchd as the service from a plist switchboard
// wrote before the router's copy, as the link watch gives it, with the
// listeners handed over by the router it replaced, as after an upgrade.
type movingOver struct {
	*serviceSetup
	// handed names the listeners handed over, as handover.Variable does,
	// and before is what the plist held as it was put in place.
	handed, before string
}

// newMovingOver sets serve up to move over to the router's copy, replacing
// itself as it doesn't unless a test says otherwise.
func newMovingOver(t *testing.T) *movingOver {
	t.Helper()
	s := newServiceSetup(t)
	s.srv.watch(t)
	s.setenv("XPC_SERVICE_NAME", service.Label)
	handed := s.srv.handOver(t)
	s.setenv(handover.Variable, handed)
	before := s.putPlistBeforeTheCopy(t, s.srv.binary)
	s.srv.deps.Args = []string{s.srv.binary, "serve"}
	s.srv.deps.Exec = func(path string, _, _ []string) error {
		t.Errorf("serve replaced itself with %s, want it carrying on as it is", path)
		return errors.New("no test starts a program")
	}
	return &movingOver{serviceSetup: s, handed: handed, before: before}
}

// putPlistBeforeTheCopy puts the plist switchboard wrote before the router's
// copy in the service's place, and returns it: running binary itself with
// serve, in the environment serve runs in, ExitTimeOut and ProcessType among
// its keys, and a variable added to it by hand.
func (s *serviceSetup) putPlistBeforeTheCopy(t *testing.T, binary string) string {
	t.Helper()
	log := service.LogIn(s.srv.state)
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>io.github.leeovery.switchboard</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>XDG_STATE_HOME</key>
		<string>%s</string>
		<key>SWITCHBOARD_CONFIG</key>
		<string>%s</string>
		%s
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ExitTimeOut</key>
	<integer>45</integer>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, binary, filepath.Dir(s.srv.state), s.srv.config, addedByHand, log, log)
	if err := os.MkdirAll(filepath.Dir(s.plist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.plist, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	return plist
}

// readPlist returns what the service's plist holds.
func (s *serviceSetup) readPlist(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(s.plist)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// handOver hands over listeners on the proxy's address and the control
// socket, as a router replacing itself does, and returns what names them, as
// handover.Variable does: they wait, open, for a router to take them up.
func (s *serveSetup) handOver(t *testing.T) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.socket()), 0o700); err != nil {
		t.Fatal(err)
	}
	proxy, err := net.Listen("tcp", s.listen)
	if err != nil {
		t.Fatal(err)
	}
	control, err := net.Listen("unix", s.socket())
	if err != nil {
		t.Fatal(err)
	}
	held, err := handover.Hold(map[string]net.Listener{"proxy": proxy, "control": control})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	_, _ = proxy.Close(), control.Close()
	var named string
	exec := func(_ string, _, env []string) error {
		named = handover.Named(env)
		return nil
	}
	if err := held.Exec(exec, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	return named
}

// sendWaiting sends a messages request through the proxy, on a connection of
// its own, as Claude Code does, and returns the connection, for the answer
// to be read once a router takes the proxy's socket up.
func (s *serveSetup) sendWaiting(t *testing.T) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", s.listen)
	if err != nil {
		t.Fatalf("a connection to the proxy before a router takes it up: %v, want it waiting to be accepted", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := s.messages(t, sessionOne, "claude-haiku-4-5-20251001", "").Write(conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

// answerOn returns the status of the answer to the request sent on conn,
// waiting a few seconds at most.
func answerOn(t *testing.T, conn net.Conn) int {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("the answer to the request sent: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

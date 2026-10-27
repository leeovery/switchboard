package cli_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// putKeptPlist puts a plist in the service's place that runs this
// switchboard's serve with args, its router's state under stateHome, as one
// installed from a shell setting XDG_STATE_HOME to it would.
func (s *serviceSetup) putKeptPlist(t *testing.T, stateHome string, args ...string) {
	t.Helper()
	var program strings.Builder
	for _, arg := range append([]string{s.binary, "serve"}, args...) {
		program.WriteString("<string>" + arg + "</string>")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>io.github.leeovery.switchboard</string>`+
		`<key>ProgramArguments</key><array>%s</array>`+
		`<key>EnvironmentVariables</key><dict><key>XDG_STATE_HOME</key><string>%s</string></dict></dict></plist>
`, program.String(), stateHome)
	if err := os.MkdirAll(filepath.Dir(s.plist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.plist, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
}

// keptRouter serves a router from before routers restarted when asked, at
// pid 100, where the plist keeps its state, under a state home of its own,
// which launchd, once it's signalled the router to stop, starts again at pid
// 4242; and returns that state home.
func (s *serviceSetup) keptRouter(t *testing.T) string {
	t.Helper()
	kept := shortTempDir(t)
	var pid atomic.Int64
	pid.Store(100)
	serveRouter(t, router.SocketPath(filepath.Join(kept, "switchboard")), &pid, nil)
	launchctl := s.srv.deps.Launchctl
	s.srv.deps.Launchctl = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "kill" {
			pid.Store(4242)
		}
		return launchctl(ctx, args...)
	}
	return kept
}

func TestServiceRestartAsksTheRouterWhereThePlistKeepsItsState(t *testing.T) {
	s := newServiceSetup(t)
	s.launchd.loaded = true
	s.putKeptPlist(t, s.keptRouter(t))

	got := run(t, s.srv.deps, "service", "restart")
	want := result{stdout: "the router is finishing its requests in flight, then launchd starts it again\nrestarted\nthe router is up: healthy, pid 4242\n"}
	if got != want {
		t.Errorf("switchboard service restart = %+v, want %+v", got, want)
	}
	if want := [][]string{{"print", target}, {"kill", "SIGTERM", target}}; !reflect.DeepEqual(s.launchd.calls, want) {
		t.Errorf("ran launchctl %q, want %q: the router answering restarted, never kickstarted", s.launchd.calls, want)
	}
}

func TestServiceStatusAsksTheRouterWhereThePlistKeepsItsState(t *testing.T) {
	s := newServiceSetup(t)
	s.launchd.loaded = true
	s.putKeptPlist(t, s.keptRouter(t))

	got := run(t, s.srv.deps, "service", "status")
	if want := (result{stdout: "LaunchAgent  " + s.plist + "\nlaunchd      loaded\nrouter       healthy, pid 100\n"}); got != want {
		t.Errorf("switchboard service status = %+v, want %+v", got, want)
	}
}

func TestServiceRestartReadsABinaryPlistBack(t *testing.T) {
	s := newServiceSetup(t)
	s.launchd.loaded = true
	s.putKeptPlist(t, s.keptRouter(t))
	xml := readFile(t, s.plist)
	// As defaults writes it.
	if err := os.WriteFile(s.plist, []byte("bplist00\xd1\x01\x02"), 0o644); err != nil {
		t.Fatal(err)
	}
	var ran [][]string
	s.srv.deps.Plutil = func(_ context.Context, args ...string) ([]byte, error) {
		ran = append(ran, args)
		return []byte(xml), nil
	}

	got := run(t, s.srv.deps, "service", "restart")
	want := result{stdout: "the router is finishing its requests in flight, then launchd starts it again\nrestarted\nthe router is up: healthy, pid 4242\n"}
	if got != want {
		t.Errorf("switchboard service restart = %+v, want %+v", got, want)
	}
	if want := [][]string{{"-convert", "xml1", "-o", "-", s.plist}}; !reflect.DeepEqual(ran, want) {
		t.Errorf("ran plutil %q, want %q", ran, want)
	}
}

func TestSetupChecksTheRouterCouldStartFromWhatThePlistKeeps(t *testing.T) {
	s := newServiceSetup(t)
	s.launchd.loaded = true
	kept := s.keptRouter(t)
	// From before ProcessType, as a hand edit gave it a log level serve
	// can't parse.
	s.putKeptPlist(t, kept, "--log-level", "loud")
	written := readFile(t, s.plist)
	atATerminal(&s.srv.deps)
	answers := strings.Join([]string{"", "", "", ""}, "\n") + "\n"

	got := runWithInput(t, s.srv.deps, answers, "setup")
	_, step, _ := strings.Cut(got.stdout, "\n3. The service\n")
	step, _, _ = strings.Cut(step, "\n4. ")
	for _, want := range []string{
		"The router uses another state directory",
		"The service's plist differs from the one this switchboard writes, with ",
		"but the router couldn't start installed afresh, so the service is left as it is, its router running: ",
		"It couldn't start as:\n--log-level: ",
	} {
		if !strings.Contains(step, want) {
			t.Errorf("setup's service step printed\n%s\nwant it to hold\n%s", step, want)
		}
	}
	if strings.Contains(step, "Installed ") {
		t.Errorf("setup's service step printed\n%s\nwant the service left as it is", step)
	}
	if got := readFile(t, s.plist); got != written {
		t.Errorf("the plist reads\n%s\nwant it left as it was\n%s", got, written)
	}
	if want := [][]string{{"print", target}}; !reflect.DeepEqual(s.launchd.calls, want) {
		t.Errorf("ran launchctl %q, want %q: the router at the state directory kept found running, and left alone", s.launchd.calls, want)
	}
}

func TestServiceInstallTakesHomeFromTheShell(t *testing.T) {
	tests := []struct {
		name string
		// homeDecides has the shell set no XDG_STATE_HOME, so the home
		// decides where the router keeps its state.
		homeDecides bool
	}{
		{name: "the shell setting XDG_STATE_HOME"},
		{name: "the home deciding the state directory", homeDecides: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServiceSetup(t)
			if tt.homeDecides {
				home := shortTempDir(t)
				s.srv.deps.HomeDir = func() (string, error) { return home, nil }
				s.setenv("XDG_STATE_HOME", "")
				s.srv.state = filepath.Join(home, ".local", "state", "switchboard")
				s.plist = filepath.Join(home, "Library", "LaunchAgents", "io.github.leeovery.switchboard.plist")
			}
			if err := os.MkdirAll(filepath.Dir(s.plist), 0o755); err != nil {
				t.Fatal(err)
			}
			plist := `<plist><dict><key>ProgramArguments</key><array><string>` + s.binary + `</string><string>serve</string></array>` +
				`<key>EnvironmentVariables</key><dict><key>HOME</key><string>` + shortTempDir(t) + `</string>` +
				`<key>HTTPS_PROXY</key><string>http://proxy.example:8080</string></dict></dict></plist>`
			if err := os.WriteFile(s.plist, []byte(plist), 0o644); err != nil {
				t.Fatal(err)
			}

			got := run(t, s.srv.deps, "service", "install")
			want := "installed " + s.plist + "\nthe router is up: healthy, pid " + strconv.Itoa(os.Getpid()) + "\n"
			if got.code != 0 || got.stdout != want {
				t.Errorf("switchboard service install = %+v, want stdout %q: the router up where the shell's home keeps its state", got, want)
			}
			written := readFile(t, s.plist)
			if strings.Contains(written, "<key>HOME</key>") {
				t.Errorf("the plist reads\n%s\nwant the HOME it set dropped, as launchd sets the user's", written)
			}
			if !strings.Contains(written, "<string>"+service.LogIn(s.srv.state)+"</string>") {
				t.Errorf("the plist reads\n%s\nwant launchd's log in %s", written, s.srv.state)
			}
			s.checkPlist(t, "<key>HTTPS_PROXY</key>\n\t\t<string>http://proxy.example:8080</string>")
		})
	}
}

func TestServiceStatusSaysWhenTheRouterAnsweringIsntThePlists(t *testing.T) {
	s := newServiceSetup(t)
	s.launchd.loaded = true
	s.putKeptPlist(t, shortTempDir(t))
	var pid atomic.Int64
	pid.Store(100)
	serveRouter(t, s.srv.socket(), &pid, nil)

	got := run(t, s.srv.deps, "service", "status")
	want := result{stdout: "LaunchAgent  " + s.plist + "\nlaunchd      loaded\nrouter       healthy, pid 100, where this shell keeps its state, not where the plist does\n"}
	if got != want {
		t.Errorf("switchboard service status = %+v, want %+v", got, want)
	}
}

// putUnreadablePlist puts a binary plist in the service's place, which
// plutil fails to read, saying something token-shaped.
func (s *serviceSetup) putUnreadablePlist(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.plist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.plist, []byte("bplist00\xd1\x01\x02"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.srv.deps.Plutil = func(context.Context, ...string) ([]byte, error) {
		return nil, fmt.Errorf("plutil: not a plist, but %s", tokenShaped)
	}
}

func TestServiceCommandsSayWhenThePlistCantBeRead(t *testing.T) {
	const lookedFor = "switchboard: the service's plist can't be read back, so the router is looked for where this shell keeps its state ("
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"service", "status"}, want: lookedFor},
		{args: []string{"service", "restart"}, want: lookedFor},
		{args: []string{"service", "install"}, want: "switchboard: the plist in place couldn't be read back, so any variable added to it by hand is gone ("},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			s := newServiceSetup(t)
			s.launchd.loaded = true
			s.putUnreadablePlist(t)

			got := run(t, s.srv.deps, tt.args...)
			if !strings.Contains(got.stderr, tt.want) || strings.Contains(got.stderr, tokenShaped) {
				t.Errorf("switchboard %s printed on stderr\n%s\nwant it to hold, redacted\n%s", strings.Join(tt.args, " "), got.stderr, tt.want)
			}
		})
	}
}

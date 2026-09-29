package testguard_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// These tests run this test binary again as a child, through the guard as
// every run is, doing what the test names: the guard around the child must
// catch it, and the one around this process stays clean.
const (
	// childDoes names what TestInChild does in the child.
	childDoes = "TESTGUARD_CHILD_DOES"
	// childHome is the child's real home, which the guard moves HOME away from.
	childHome = "TESTGUARD_CHILD_HOME"
	// childElsewhere is where the child's environment puts its real config
	// and state, outside its home, which the guard clears the variables of.
	childElsewhere = "TESTGUARD_CHILD_ELSEWHERE"
)

// seeded are variables a child starts with that the guard must clear.
var seeded = []string{
	"CLAUDE_TOKEN_WORK=test-token-work",
	"CLAUDE_CODE_OAUTH_TOKEN=test-token-oauth",
	"ANTHROPIC_API_KEY=test-key",
	"ANTHROPIC_BASE_URL=http://192.0.2.1",
	"SWITCHBOARD_CONFIG=/nonexistent/config.toml",
	"CLAUDE_CONFIG_DIR=/nonexistent/claude",
	"SWITCHBOARD_LOG_LEVEL=debug",
	"TMUX=/nonexistent/tmux,1,0",
	"TMUX_PANE=%1",
	"TMUX_TMPDIR=/nonexistent/tmux",
	"HTTPS_PROXY=http://127.0.0.1:9",
	"http_proxy=http://127.0.0.1:9",
	"ALL_PROXY=socks5://127.0.0.1:9",
}

// Where switchboard keeps its config, its state, its LaunchAgent and its
// skill, from a home.
var (
	configFile = filepath.Join(".config", "switchboard", "config.toml")
	stateFile  = filepath.Join(".local", "state", "switchboard", "state.json")
	agentFile  = filepath.Join("Library", "LaunchAgents", "io.github.leeovery.switchboard.plist")
	skillFile  = filepath.Join(".claude", "skills", "switchboard", "SKILL.md")
)

// outsideSocket is a control socket outside the temporary directory, where a
// live router's is: nothing's there, but the guard must block the dial all
// the same.
const outsideSocket = "/nonexistent/switchboard/control.sock"

func TestEscapesFailTheRunThoughEveryTestPasses(t *testing.T) {
	tests := []struct {
		does string
		want string
	}{
		{does: "run-a-stub", want: "ran osascript -e return 1"},
		{does: "dial-off-the-machine", want: "blocked dial to 192.0.2.1:80"},
		{does: "dial-a-socket-outside-the-temporary-directory", want: "blocked dial to " + outsideSocket},
		{does: "overwrite-the-real-config", want: "the real ~/.config/switchboard/config.toml was modified"},
		{does: "create-the-real-state", want: "the real ~/.local/state/switchboard appeared"},
		{does: "install-the-real-launch-agent", want: "the real ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist was created"},
		{does: "install-the-real-skill", want: "the real ~/.claude/skills/switchboard/SKILL.md was created"},
	}
	for _, tt := range tests {
		t.Run(tt.does, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, configFile), "listen = \"127.0.0.1:4747\"\n")

			out, err := runChild(t, tt.does, home)
			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
				t.Errorf("child: error = %v, want exit status 1", err)
			}
			passed := slices.Contains(strings.Split(out, "\n"), "PASS")
			if !passed || !strings.Contains(out, "testguard: the tests reached past their isolation") || !strings.Contains(out, tt.want) {
				t.Errorf("child printed\n%s\nwant its test to pass, and the guard to fail the run as %q", out, tt.want)
			}
		})
	}
}

func TestEscapesWhereTheEnvironmentPutsTheConfigStateAndSkill(t *testing.T) {
	tests := []struct {
		does string
		// want is what the guard fails the run as, %s standing for where the
		// environment puts the config and state.
		want string
	}{
		{does: "overwrite-the-config-SWITCHBOARD_CONFIG-names", want: "the real %s/work.toml was modified"},
		{does: "create-the-state-where-XDG_STATE_HOME-puts-it", want: "the real %s/state/switchboard appeared"},
		{does: "dial-the-control-socket-where-XDG_STATE_HOME-puts-it", want: "blocked dial to %s/state/switchboard/control.sock"},
		{does: "install-the-skill-where-CLAUDE_CONFIG_DIR-puts-it", want: "the real %s/claude/skills/switchboard/SKILL.md was created"},
	}
	for _, tt := range tests {
		t.Run(tt.does, func(t *testing.T) {
			elsewhere := t.TempDir()
			writeFile(t, filepath.Join(elsewhere, "work.toml"), "listen = \"127.0.0.1:4747\"\n")

			out, err := runChild(t, tt.does, t.TempDir(),
				"SWITCHBOARD_CONFIG="+filepath.Join(elsewhere, "work.toml"),
				"XDG_STATE_HOME="+filepath.Join(elsewhere, "state"),
				"CLAUDE_CONFIG_DIR="+filepath.Join(elsewhere, "claude"),
				childElsewhere+"="+elsewhere)
			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
				t.Errorf("child: error = %v, want exit status 1", err)
			}
			want := fmt.Sprintf(tt.want, elsewhere)
			passed := slices.Contains(strings.Split(out, "\n"), "PASS")
			if !passed || !strings.Contains(out, "testguard: the tests reached past their isolation") || !strings.Contains(out, want) {
				t.Errorf("child printed\n%s\nwant its test to pass, and the guard to fail the run as %q", out, want)
			}
		})
	}
}

func TestWhatTheGuardLetsBePasses(t *testing.T) {
	for _, does := range []string{"stay-in-isolation", "write-the-real-state-as-a-live-router-does"} {
		t.Run(does, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, stateFile), "{\"version\": 1, \"sessions\": []}\n")

			out, err := runChild(t, does, home)
			if err != nil || strings.Contains(out, "testguard") {
				t.Errorf("child: error = %v, printing\n%s\nwant it to pass, with nothing from the guard", err, out)
			}
		})
	}
}

func TestTheEnvironmentIsIsolated(t *testing.T) {
	out, err := runChild(t, "check-the-environment", t.TempDir(), append(seeded, "TESTGUARD_KEPT=kept")...)
	if err != nil {
		t.Errorf("child: error = %v, printing\n%s", err, out)
	}
}

// TestInChild does what childDoes names, when a test here runs it in a child.
func TestInChild(t *testing.T) {
	does := os.Getenv(childDoes)
	if does == "" {
		t.Skip("runs only in a child")
	}
	realHome := os.Getenv(childHome)
	switch does {
	case "run-a-stub":
		err := exec.Command("osascript", "-e", "return 1").Run()
		if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
			t.Errorf("osascript: error = %v, want the stub's exit status 1", err)
		}
	case "dial-off-the-machine":
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Get("http://192.0.2.1/")
		if err == nil {
			_ = resp.Body.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "testguard: blocked dial to 192.0.2.1:80") {
			t.Errorf("GET http://192.0.2.1/: error = %v, want the dial blocked", err)
		}
	case "dial-a-socket-outside-the-temporary-directory":
		resp, err := overSocket(outsideSocket).Get("http://switchboard/health")
		if err == nil {
			_ = resp.Body.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "testguard: blocked dial to "+outsideSocket) {
			t.Errorf("GET /health over %s: error = %v, want the dial blocked", outsideSocket, err)
		}
	case "overwrite-the-real-config":
		writeFile(t, filepath.Join(realHome, configFile), "listen = \"127.0.0.1:4748\"\nupstream = \"http://127.0.0.1:1\"\n")
	case "create-the-real-state":
		writeFile(t, filepath.Join(realHome, stateFile), "{\"version\": 1, \"sessions\": []}\n")
	case "install-the-real-launch-agent":
		writeFile(t, filepath.Join(realHome, agentFile), "<plist version=\"1.0\"/>\n")
	case "install-the-real-skill":
		writeFile(t, filepath.Join(realHome, skillFile), "---\nname: switchboard\n---\n")
	case "overwrite-the-config-SWITCHBOARD_CONFIG-names":
		writeFile(t, filepath.Join(os.Getenv(childElsewhere), "work.toml"), "listen = \"127.0.0.1:4748\"\nupstream = \"http://127.0.0.1:1\"\n")
	case "create-the-state-where-XDG_STATE_HOME-puts-it":
		writeFile(t, filepath.Join(os.Getenv(childElsewhere), "state", "switchboard", "state.json"), "{\"version\": 1, \"sessions\": []}\n")
	case "install-the-skill-where-CLAUDE_CONFIG_DIR-puts-it":
		writeFile(t, filepath.Join(os.Getenv(childElsewhere), "claude", "skills", "switchboard", "SKILL.md"), "---\nname: switchboard\n---\n")
	case "dial-the-control-socket-where-XDG_STATE_HOME-puts-it":
		// In the temporary directory, as the state directory is here, but a
		// live router's all the same.
		socket := filepath.Join(os.Getenv(childElsewhere), "state", "switchboard", "control.sock")
		resp, err := overSocket(socket).Get("http://switchboard/health")
		if err == nil {
			_ = resp.Body.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "testguard: blocked dial to "+socket) {
			t.Errorf("GET /health over %s: error = %v, want the dial blocked", socket, err)
		}
	case "write-the-real-state-as-a-live-router-does":
		writeLikeARouter(t, filepath.Join(realHome, stateFile))
	case "stay-in-isolation":
		stayInIsolation(t)
	case "check-the-environment":
		checkEnvironment(t, realHome)
	default:
		t.Fatalf("%s=%s names nothing to do", childDoes, does)
	}
}

// stayInIsolation does what the guard allows: it dials loopback and a unix
// socket in the temporary directory, writes a config into the home the guard
// gives it, and writes under a temporary directory.
func stayInIsolation(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	srv := httptest.NewServer(ok)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET %s: %v", srv.URL, err)
	}
	_ = resp.Body.Close()
	socket := serveOnASocket(t, ok)
	resp, err = overSocket(socket).Get("http://switchboard/")
	if err != nil {
		t.Fatalf("GET / over %s: %v", socket, err)
	}
	_ = resp.Body.Close()
	writeFile(t, filepath.Join(os.Getenv("HOME"), configFile), "listen = \"127.0.0.1:4747\"\n")
	writeFile(t, filepath.Join(t.TempDir(), "state.json"), "{}\n")
}

// serveOnASocket serves h on a unix socket in the temporary directory until
// the test ends, and returns the socket's path.
func serveOnASocket(t *testing.T, h http.Handler) string {
	t.Helper()
	// t.TempDir's can be too long for a unix socket on macOS.
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "control.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return path
}

// overSocket returns a client whose every request goes to the unix socket at
// path, as switchboard's router client's do, over a transport cloned from
// http.DefaultTransport, which the guard guards.
func overSocket(path string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dial(ctx, "unix", path)
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

// writeLikeARouter rewrites the state file at state through a temporary file
// renamed over it, and writes to the log beside it, as a live router does.
func writeLikeARouter(t *testing.T, state string) {
	t.Helper()
	writeFile(t, state+".tmp", "{\"version\": 1, \"sessions\": [], \"pin\": {\"account\": \"work\"}}\n")
	if err := os.Rename(state+".tmp", state); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(filepath.Dir(state), "logs", "router.log"), "level=INFO msg=routed account=work\n")
}

// checkEnvironment checks that the guard cleared the seeded variables, kept
// the rest, and moved HOME, XDG's base directories and PATH off the real ones.
// It names a variable that's wrong, never its value.
func checkEnvironment(t *testing.T, realHome string) {
	for _, variable := range seeded {
		name, _, _ := strings.Cut(variable, "=")
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s is set, want it cleared", name)
		}
	}
	if os.Getenv("TESTGUARD_KEPT") != "kept" {
		t.Error("TESTGUARD_KEPT isn't as the child started with it, want it kept")
	}

	home := os.Getenv("HOME")
	if info, err := os.Stat(home); home == realHome || err != nil || !info.IsDir() {
		t.Errorf("HOME is the real home or no directory (%v), want a throwaway one", err)
	}
	for name, want := range map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_STATE_HOME":  filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
		"XDG_DATA_HOME":   filepath.Join(home, ".local", "share"),
	} {
		if os.Getenv(name) != want {
			t.Errorf("%s isn't in the throwaway home", name)
		}
	}

	path := os.Getenv("PATH")
	entries, err := os.ReadDir(path)
	if err != nil || strings.Contains(path, string(os.PathListSeparator)) {
		t.Fatalf("PATH isn't one directory (%v), want the stubs' alone", err)
	}
	var stubs []string
	for _, e := range entries {
		stubs = append(stubs, e.Name())
	}
	if want := []string{"claude", "launchctl", "open", "osascript", "tmux"}; !slices.Equal(stubs, want) {
		t.Errorf("PATH holds %q, want the stubs %q alone", stubs, want)
	}
}

// runChild runs TestInChild in a child of this test binary, doing does, with
// home as its home and env added to the environment, and returns what it
// printed.
func runChild(t *testing.T, does, home string, env ...string) (string, error) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestInChild$", "-test.timeout=1m")
	// Under the race detector, a child that exits 0 would first sleep for its
	// default atexit_sleep_ms: measured, a second a child.
	race := strings.TrimSpace(os.Getenv("GORACE") + " atexit_sleep_ms=0")
	cmd.Env = slices.Concat(os.Environ(), env, []string{"HOME=" + home, "GORACE=" + race, childDoes + "=" + does, childHome + "=" + home})
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// writeFile writes content to a file at path, creating its directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

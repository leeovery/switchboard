package capture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestAPrintoutIsWhatUsagePrintsOfItsRouter(t *testing.T) {
	now := moment(time.UTC)
	tests := []struct {
		name   string
		router source
	}{
		{name: "one account", router: oneAccount(now)},
		{name: "three accounts", router: threeAccounts(now)},
		{name: "eight accounts", router: eightAccounts(now)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := printoutOf(t)
			f.router = tt.router
			usage := usageOf(t, tt.router, now)
			for _, width := range []int{160, 120, 80, 52} {
				if got, want := f.Frame(watch.Size{Width: width}), usage(width); got != want {
					t.Errorf("at %d columns, the printout reads\n%s\nwant what switchboard usage prints of its router\n%s", width, got, want)
				}
			}
		})
	}
}

func TestAPrintoutIsAsTallAsItIsAndPaintsNoBackground(t *testing.T) {
	f := printoutOf(t)
	screen := f
	screen.Printout = false
	if !strings.Contains(screen.Frame(screen.Size), nordCanvas) {
		t.Fatalf("the watch's frame of the same fixture paints no %s either: the check below reads nothing", nordCanvas)
	}
	for _, size := range []watch.Size{{Width: 160, Height: 10}, {Width: 160, Height: 80}} {
		frame := f.Frame(size)
		lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
		if want := f.Frame(watch.Size{Width: 160}); frame != want {
			t.Errorf("on a terminal %d tall, the printout is %d lines, want as many as it has, whatever the height", size.Height, len(lines))
		}
		if !strings.Contains(frame, "38;2;") {
			t.Errorf("the printout\n%q\nwant it in its theme's colours", frame)
		}
		for i, line := range lines {
			if strings.Contains(line, nordCanvas) {
				t.Errorf("line %d paints the canvas, want the terminal's own background left as it is:\n%q", i+1, line)
			}
			if text := ansi.Strip(line); strings.TrimRight(text, " ") != text {
				t.Errorf("line %d reads %q, want it to end at its last glyph, as nothing fills the row", i+1, text)
			}
		}
	}
}

func TestAPrintoutWithoutColourIsBoldAlone(t *testing.T) {
	frame := printoutOf(t).WithoutColour().Frame(watch.Size{Width: 160})

	if !strings.Contains(frame, "\x1b[1m") {
		t.Errorf("without colour, the printout\n%q\nwant its names and percentages bold", frame)
	}
	for _, m := range sgrs.FindAllStringSubmatch(frame, -1) {
		if m[1] != "1" && m[1] != "" && m[1] != "0" {
			t.Errorf("without colour, the printout has %q, want bold alone", m[0])
		}
	}
}

// nordCanvas is Nord's canvas as a background's SGR parameters.
const nordCanvas = "48;2;46;52;64"

// sgrs are the SGR sequences in text, each one's parameters captured.
var sgrs = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// printoutOf is a printout of usage over milestone 5's three accounts, in the
// frames' theme, at the frames' moment, in UTC.
func printoutOf(t *testing.T) Fixture {
	t.Helper()
	now := moment(time.UTC)
	nord, ok := theme.Builtin(theme.DefaultDark)
	if !ok {
		t.Fatal("no built-in nord")
	}
	return Fixture{Name: "usage-3", Size: wide(34), Printout: true, now: now, router: threeAccounts(now), theme: nord}
}

// usageOf is what switchboard usage prints of the router given, by now's
// clock, on a terminal as wide as given: run in process, as its own tests run
// it, its config listing the router's accounts, the router served on the
// control socket of a state directory of its own, and the terminal a
// truecolour one whose background is the frames' canvas.
func usageOf(t *testing.T, r source, now time.Time) func(width int) string {
	t.Helper()
	// t.TempDir's can be too long on macOS for a socket's path.
	stateHome, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateHome) })
	home := t.TempDir()
	configFile := filepath.Join(home, "config.toml")
	var accounts strings.Builder
	for _, a := range r.doc.Accounts {
		fmt.Fprintf(&accounts, "[[account]]\nid = %q\n\n", a.ID)
	}
	if err := os.WriteFile(configFile, []byte(accounts.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"SWITCHBOARD_CONFIG": configFile, "XDG_STATE_HOME": stateHome,
		"TERM": "xterm-256color", "COLORTERM": "truecolor", "CLICOLOR_FORCE": "1",
	}
	deps := cli.Deps{
		Getenv: func(key string) string { return env[key] },
		Environ: func() []string {
			var environ []string
			for key, value := range env {
				environ = append(environ, key+"="+value)
			}
			return environ
		},
		HomeDir:       func() (string, error) { return home, nil },
		Now:           func() time.Time { return now },
		ClaudeVersion: func() string { return "2.1.300" },
		Terminal:      func(io.Writer) bool { return true },
		Background:    func(io.Writer) color.Color { return terminalBackground },
	}
	state, err := config.StateDir(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	serveAsRouter(t, router.SocketPath(state), r)
	return func(width int) string {
		env["COLUMNS"] = strconv.Itoa(width)
		root := cli.NewRootCommand(deps)
		var stdout, stderr bytes.Buffer
		root.SetArgs([]string{"usage"})
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		if code := cli.Execute(root); code != 0 || stderr.Len() > 0 {
			t.Fatalf("switchboard usage exited %d, saying %q", code, stderr.String())
		}
		return stdout.String()
	}
}

// serveAsRouter answers on the control socket at path, until the test ends,
// as a router does that gives what r gives: its health check, its status
// document, the sessions it lists and its windows' history.
func serveAsRouter(t *testing.T, path string, r source) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	answer := func(w http.ResponseWriter, v any, err error) {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, req *http.Request) {
		_, health, err := r.Read(req.Context(), status.Read{})
		answer(w, health, err)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, req *http.Request) {
		doc, _, err := r.Read(req.Context(), status.Read{})
		answer(w, doc, err)
	})
	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, req *http.Request) {
		sessions, err := r.Sessions(req.Context())
		answer(w, sessions, err)
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, req *http.Request) {
		step, err := time.ParseDuration(req.URL.Query().Get("step"))
		if err != nil {
			answer(w, nil, err)
			return
		}
		history, err := r.History(req.Context(), req.URL.Query().Get("window"), step)
		answer(w, history, err)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

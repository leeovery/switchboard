package capture

import (
	"bytes"
	"cmp"
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
)

func TestAPrintoutIsWhatUsagePrintsOfItsRouter(t *testing.T) {
	tests := []struct {
		name, fixture string
		// by is what the config's [usage] by says, "" where it says nothing.
		by   string
		args []string
	}{
		{fixture: "usage-160"},
		{fixture: "usage-120"},
		{fixture: "usage-80"},
		{fixture: "usage-52"},
		{fixture: "usage-5h-160", args: []string{"--5h"}},
		{fixture: "usage-5h-52", args: []string{"--5h"}},
		{fixture: "usage-by-account-160", args: []string{"--by", "account"}},
		{fixture: "usage-by-account-52", args: []string{"--by", "account"}},
		{fixture: "usage-one-account-160"},
		{name: "by account, as the config says", fixture: "usage-by-account-160", by: "account"},
		{name: "--by window beating the config's account", fixture: "usage-160", by: "account", args: []string{"--by", "window"}},
	}
	run := make(map[string]bool)
	for _, tt := range tests {
		run[tt.fixture] = true
		t.Run(cmp.Or(tt.name, tt.fixture), func(t *testing.T) {
			f := fixture(t, tt.fixture)
			if got, want := "\n"+f.Frame(f.Size), usageOf(t, f, tt.by, tt.args...); got != want {
				t.Errorf("the printout reads, after a blank line,\n%s\nwant what switchboard usage %s prints of its router\n%s", got, strings.Join(tt.args, " "), want)
			}
		})
	}
	for _, name := range Names() {
		if f := fixture(t, name); f.Printout && !run[name] {
			t.Errorf("%s is a printout, and nothing holds it to what usage prints", name)
		}
	}
}

func TestAPrintoutIsAsTallAsItIsAndPaintsNoBackground(t *testing.T) {
	f := fixture(t, "usage-160")
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
	frame := fixture(t, "usage-160").WithoutColour().Frame(watch.Size{Width: 160})

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

// usageOf is what switchboard usage prints, given args, of the fixture's
// router, by its clock, on a terminal as wide as its own: run in process, as
// its own tests run it, its config listing the router's accounts and, where
// by isn't "", setting [usage] by to it, the router served on the control
// socket of a state directory of its own, and the terminal a truecolour one
// whose background is the frames' canvas.
func usageOf(t *testing.T, f Fixture, by string, args ...string) string {
	t.Helper()
	// t.TempDir's can be too long on macOS for a socket's path.
	stateHome, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateHome) })
	home := t.TempDir()
	configFile := filepath.Join(home, "config.toml")
	var cfg strings.Builder
	if by != "" {
		fmt.Fprintf(&cfg, "[usage]\nby = %q\n\n", by)
	}
	for _, a := range f.router.doc.Accounts {
		fmt.Fprintf(&cfg, "[[account]]\nid = %q\n\n", a.ID)
	}
	if err := os.WriteFile(configFile, []byte(cfg.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"SWITCHBOARD_CONFIG": configFile, "XDG_STATE_HOME": stateHome, "COLUMNS": strconv.Itoa(f.Size.Width),
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
		Now:           func() time.Time { return f.now },
		ClaudeVersion: func() string { return "2.1.300" },
		Terminal:      func(io.Writer) bool { return true },
		Background:    func(io.Writer) color.Color { return terminalBackground },
	}
	state, err := config.StateDir(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	serveAsRouter(t, router.SocketPath(state), f.router)
	root := cli.NewRootCommand(deps)
	var stdout, stderr bytes.Buffer
	root.SetArgs(append([]string{"usage"}, args...))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if code := cli.Execute(root); code != 0 || stderr.Len() > 0 {
		t.Fatalf("switchboard usage %s exited %d, saying %q", strings.Join(args, " "), code, stderr.String())
	}
	return stdout.String()
}

// serveAsRouter answers on the control socket at path, until the test ends,
// as a router does that gives what r gives: its health check and its status
// document. usage reads nothing else, so anything else asked fails the test.
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
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("switchboard usage asked the router %s %s, want the status document alone", req.Method, req.URL.Path)
		http.NotFound(w, req)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

package cli_test

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/score"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests print")

func TestUsage(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		golden string
	}{
		{name: "at the default width", golden: "usage.golden"},
		{name: "at the width $COLUMNS gives", env: map[string]string{"COLUMNS": "180"}, golden: "usage-wide.golden"},
		{name: "ignoring a $COLUMNS that isn't a width", env: map[string]string{"COLUMNS": "wide"}, golden: "usage.golden"},
		{name: "ignoring a $COLUMNS of zero", env: map[string]string{"COLUMNS": "0"}, golden: "usage.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, goldenDeps(t, tt.env), "usage")
			if got.code != 0 || got.stderr != "" {
				t.Fatalf("switchboard usage = %+v, want exit status 0 and nothing on stderr", got)
			}
			if *update {
				writeGolden(t, tt.golden, got.stdout)
			}
			if want := readGolden(t, tt.golden); got.stdout != want {
				t.Errorf("switchboard usage printed\n%s\nwant (testdata/%s; run with -update to accept it)\n%s", got.stdout, tt.golden, want)
			}
		})
	}
}

func TestUsageRefreshWithoutTheRouterProbesAsUsageDoes(t *testing.T) {
	// usage runs switchboard usage with args, set up as goldenDeps sets it
	// up, and returns how it went and what the API was asked.
	usage := func(args ...string) (result, []string) {
		api := newClaudeAPI(t)
		deps := statusDeps(t, api.URL, nil)
		onTerminal(&deps)
		writeToken(t, deps, "personal", "test-token-personal")
		return run(t, deps, append([]string{"usage"}, args...)...), api.questions()
	}
	_, probes := usage()
	if len(probes) == 0 {
		t.Fatal("switchboard usage asked the API nothing, want every account probed")
	}
	for _, flag := range []string{"--refresh", "-r"} {
		t.Run(flag, func(t *testing.T) {
			got, asked := usage(flag)
			if want := (result{stdout: readGolden(t, "usage.golden")}); got != want {
				t.Errorf("switchboard usage %s =\n%+v\nwant what usage prints (testdata/usage.golden)\n%+v", flag, got, want)
			}
			if !slices.Equal(asked, probes) {
				t.Errorf("switchboard usage %s asked the API %q, want %q, every account probed as usage probes it", flag, asked, probes)
			}
		})
	}
}

func TestUsageColor(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		want     string
		wantNone string
	}{
		{
			name:     "brought down to what the terminal shows",
			env:      map[string]string{"CLICOLOR_FORCE": "1", "TERM": "xterm-256color"},
			want:     "\x1b[38;5;",
			wantNone: "\x1b[38;2;",
		},
		{
			name: "in full where the terminal shows it",
			env:  map[string]string{"CLICOLOR_FORCE": "1", "TERM": "xterm-256color", "COLORTERM": "truecolor"},
			want: "\x1b[38;2;",
		},
		{
			name:     "none under NO_COLOR, but still bold",
			env:      map[string]string{"TTY_FORCE": "1", "TERM": "xterm-256color", "NO_COLOR": "1"},
			want:     "\x1b[1m",
			wantNone: "\x1b[38;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, goldenDeps(t, tt.env), "usage")
			if got.code != 0 {
				t.Fatalf("switchboard usage = %+v, want exit status 0", got)
			}
			if !strings.Contains(got.stdout, tt.want) {
				t.Errorf("switchboard usage printed %q, want it to contain %q", got.stdout, tt.want)
			}
			if tt.wantNone != "" && strings.Contains(got.stdout, tt.wantNone) {
				t.Errorf("switchboard usage printed %q, want no %q", got.stdout, tt.wantNone)
			}
			if stripped, want := ansi.Strip(got.stdout), readGolden(t, "usage.golden"); stripped != want {
				t.Errorf("switchboard usage printed, stripped of its escapes,\n%s\nwant\n%s", stripped, want)
			}
		})
	}
}

func TestUsageWithoutATerminalPrintsWhatStatusJSONPrints(t *testing.T) {
	tests := []struct {
		name string
		// args are given to usage and to status --json alike.
		args []string
	}{
		{name: "as the document stands"},
		{name: "refreshed", args: []string{"--refresh"}},
		{name: "refreshed, with -r for short", args: []string{"-r"}},
		{name: "probed as asked", args: []string{"--probe"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)

			got := run(t, deps, append([]string{"usage"}, tt.args...)...)
			want := run(t, deps, append([]string{"status", "--json"}, tt.args...)...)
			if want.code != 0 || !json.Valid([]byte(want.stdout)) {
				t.Fatalf("switchboard status --json %s = %+v, want exit status 0 and a document", strings.Join(tt.args, " "), want)
			}
			if got != want {
				t.Errorf("switchboard usage %s, without a terminal, =\n%+v\nwant what status --json prints\n%+v", strings.Join(tt.args, " "), got, want)
			}
		})
	}
}

func TestUsageWatch(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantInterval time.Duration
		wantNotify   bool
	}{
		{name: "every 30 minutes unless told", args: []string{"usage", "--watch"}, wantInterval: 30 * time.Minute, wantNotify: true},
		{name: "with -w for short", args: []string{"usage", "-w"}, wantInterval: 30 * time.Minute, wantNotify: true},
		{name: "every interval in minutes", args: []string{"usage", "-w", "15m"}, wantInterval: 15 * time.Minute, wantNotify: true},
		{name: "every interval in hours", args: []string{"usage", "-w", "1h"}, wantInterval: time.Hour, wantNotify: true},
		{name: "every interval in hours and minutes", args: []string{"usage", "-w", "1h30m"}, wantInterval: 90 * time.Minute, wantNotify: true},
		{name: "every bare number of minutes", args: []string{"usage", "-w", "45"}, wantInterval: 45 * time.Minute, wantNotify: true},
		{name: "every fraction of minutes", args: []string{"usage", "-w", "7.5"}, wantInterval: 7*time.Minute + 30*time.Second, wantNotify: true},
		{name: "at the shortest interval", args: []string{"usage", "-w", "5m"}, wantInterval: 5 * time.Minute, wantNotify: true},
		{name: "with the interval before the flag", args: []string{"usage", "45", "-w"}, wantInterval: 45 * time.Minute, wantNotify: true},
		{name: "without notifications", args: []string{"usage", "-w", "--no-notify"}, wantInterval: 30 * time.Minute, wantNotify: false},
		{name: "without notifications, every interval", args: []string{"usage", "--no-notify", "-w", "1h"}, wantInterval: time.Hour, wantNotify: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)
			cfg := recordWatch(t, &deps)

			if got := run(t, deps, tt.args...); got != (result{}) {
				t.Fatalf("switchboard %s = %+v, want exit status 0 and nothing printed", strings.Join(tt.args, " "), got)
			}
			if cfg.Interval != tt.wantInterval {
				t.Errorf("interval = %v, want %v", cfg.Interval, tt.wantInterval)
			}
			switch {
			case tt.wantNotify && cfg.Notifier != deps.Notifier:
				t.Errorf("notifier = %#v, want the command's own", cfg.Notifier)
			case !tt.wantNotify && cfg.Notifier != (notify.Off{}):
				t.Errorf("notifier = %#v, want one that posts nothing", cfg.Notifier)
			}
		})
	}
}

func TestUsageWatchNotifiesAsTheConfigSays(t *testing.T) {
	tests := []struct {
		name  string
		extra string
		want  config.Notifications
	}{
		{name: "as it says when it doesn't", want: config.Notifications{Limits: true, Room: true, Warning: 0.9}},
		{name: "as it says", extra: "\n[notifications]\nroom = false\nwarning = 0.8\n", want: config.Notifications{Limits: true, Warning: 0.8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), nil)
			srv.extra = tt.extra
			srv.writeConfig(t)
			cfg := recordWatch(t, &srv.deps)

			if got := run(t, srv.deps, "usage", "--watch"); got != (result{}) {
				t.Fatalf("switchboard usage --watch = %+v, want exit status 0 and nothing printed", got)
			}
			if cfg.Notifications != tt.want {
				t.Errorf("notifications = %+v, want %+v", cfg.Notifications, tt.want)
			}
		})
	}
}

func TestUsageWatchReadsWhatStatusReads(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"TERM": "xterm-256color"})
	cfg := recordWatch(t, &deps)
	run(t, deps, "usage", "--watch")

	doc, _, err := cfg.Source.Read(t.Context(), watch.Read{Probe: true})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	var fetched strings.Builder
	enc := json.NewEncoder(&fetched)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}
	if want := run(t, deps, "status", "--json").stdout; fetched.String() != want {
		t.Errorf("--watch reads\n%s\nwant what status --json reads\n%s", fetched.String(), want)
	}
	if got := cfg.Now(); !got.Equal(testNow) {
		t.Errorf("clock reads %v, want the command's clock at %v", got, testNow)
	}
	if want := (score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}); !reflect.DeepEqual(cfg.Policy, want) {
		t.Errorf("policy = %+v, want Claude's %+v", cfg.Policy, want)
	}
}

func TestUsageWatchDrawsAtTheEnvironmentsSizeUntilTheTerminalGivesOne(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want watch.Size
	}{
		{name: "80×24 with none given", want: watch.Size{Width: 80, Height: 24}},
		{name: "$COLUMNS × $LINES", env: map[string]string{"COLUMNS": "120", "LINES": "40"}, want: watch.Size{Width: 120, Height: 40}},
		{name: "$COLUMNS alone", env: map[string]string{"COLUMNS": "120"}, want: watch.Size{Width: 120, Height: 24}},
		{name: "$LINES alone", env: map[string]string{"LINES": "40"}, want: watch.Size{Width: 80, Height: 40}},
		{name: "ignoring values that aren't sizes", env: map[string]string{"COLUMNS": "wide", "LINES": "4.5"}, want: watch.Size{Width: 80, Height: 24}},
		{name: "ignoring sizes of zero or less", env: map[string]string{"COLUMNS": "0", "LINES": "-40"}, want: watch.Size{Width: 80, Height: 24}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), tt.env)
			cfg := recordWatch(t, &deps)
			run(t, deps, "usage", "--watch")

			if cfg.Size != tt.want {
				t.Errorf("size = %+v, want %+v", cfg.Size, tt.want)
			}
		})
	}
}

func TestUsageWatchClaimsTheVersionInstalledAtEachRead(t *testing.T) {
	var mu sync.Mutex
	var agents []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		agents = append(agents, r.Header.Get("User-Agent"))
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(api.Close)
	deps := statusDeps(t, api.URL, nil)
	installed := "2.1.300"
	deps.ClaudeVersion = func() string { return installed }
	cfg := recordWatch(t, &deps)
	run(t, deps, "usage", "--watch")

	for _, version := range []string{"2.1.300", "2.1.301"} {
		installed = version
		mu.Lock()
		agents = nil
		mu.Unlock()
		if _, _, err := cfg.Source.Read(t.Context(), watch.Read{Probe: true}); err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		mu.Lock()
		claimed := slices.Compact(slices.Sorted(slices.Values(agents)))
		mu.Unlock()
		if want := []string{"claude-code/" + version}; !slices.Equal(claimed, want) {
			t.Errorf("with %s installed, a read claimed %q, want %q", version, claimed, want)
		}
	}
}

func TestUsageWatchArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "an interval without --watch", args: []string{"usage", "15m"}, wantErr: `unexpected argument "15m": only --watch takes an interval`},
		{name: "two intervals", args: []string{"usage", "-w", "15m", "1h"}, wantErr: "--watch takes one interval, not 2"},
		{name: "an interval that can't be read", args: []string{"usage", "-w", "soon"}, wantErr: `invalid interval "soon": give a duration, such as 15m or 1h, or a number of minutes`},
		{name: "an interval with an unknown unit", args: []string{"usage", "-w", "2d"}, wantErr: `invalid interval "2d": give a duration, such as 15m or 1h, or a number of minutes`},
		{name: "an interval too short", args: []string{"usage", "-w", "4m59s"}, wantErr: "interval 4m59s is too short: the shortest is 5m"},
		{name: "a number of minutes too few", args: []string{"usage", "-w", "2"}, wantErr: "interval 2 is too short: the shortest is 5m"},
		{name: "no interval at all", args: []string{"usage", "-w", "0"}, wantErr: "interval 0 is too short: the shortest is 5m"},
		{name: "a negative interval", args: []string{"usage", "-w", "--", "-10m"}, wantErr: "interval -10m is too short: the shortest is 5m"},
		{name: "refreshing", args: []string{"usage", "-w", "-r"}, wantErr: "--refresh reads once, so it takes no --watch: in a watch, r refreshes"},
		{name: "refreshing, with an interval", args: []string{"usage", "--watch", "15m", "--refresh"}, wantErr: "--refresh reads once, so it takes no --watch: in a watch, r refreshes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)
			deps.Watch = func(context.Context, watch.Config, io.Writer, []string) error {
				t.Error("the dashboard started")
				return nil
			}

			got := run(t, deps, tt.args...)
			if want := "Error: " + tt.wantErr + "\n"; got.code != 1 || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("switchboard %s = %+v, want exit status 1 and an error starting %q", strings.Join(tt.args, " "), got, want)
			}
		})
	}
}

func TestUsageWatchNeedsATerminal(t *testing.T) {
	got := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "usage", "--watch")
	want := result{stderr: "Error: --watch needs a terminal, and stdout isn't one\n", code: 1}
	if got != want {
		t.Errorf("switchboard usage --watch = %+v, want %+v", got, want)
	}
}

// recordWatch has deps' Watch record the config it's given instead of taking
// over the terminal, and returns where it records it.
func recordWatch(t *testing.T, deps *cli.Deps) *watch.Config {
	t.Helper()
	var cfg watch.Config
	deps.Watch = func(_ context.Context, given watch.Config, _ io.Writer, environ []string) error {
		if got, want := slices.Sorted(slices.Values(environ)), slices.Sorted(slices.Values(deps.Environ())); !slices.Equal(got, want) {
			t.Errorf("Watch given the environment %q, want the command's %q", got, want)
		}
		cfg = given
		return nil
	}
	t.Cleanup(func() {
		if cfg.Source == nil {
			t.Error("the dashboard never started")
		}
	})
	return &cfg
}

// goldenDeps are statusDeps on a terminal, so usage draws its dashboard, with
// env added to their environment, but for a token for personal, which
// fakeClaudeAPI refuses: the reason for one missing names the test's own
// state directory, which no golden file can.
func goldenDeps(t *testing.T, env map[string]string) cli.Deps {
	t.Helper()
	deps := statusDeps(t, fakeClaudeAPI(t), env)
	onTerminal(&deps)
	writeToken(t, deps, "personal", "test-token-personal")
	return deps
}

// onTerminal has commands run with deps take their output for a terminal, so
// usage draws its dashboard there.
func onTerminal(deps *cli.Deps) {
	deps.Terminal = func(io.Writer) bool { return true }
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden file (run with -update to create it): %v", err)
	}
	return string(data)
}

func writeGolden(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

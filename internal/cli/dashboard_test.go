package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestDashboard(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantInterval time.Duration
		wantNotify   bool
	}{
		{name: "every 30 minutes unless told", args: []string{"dashboard"}, wantInterval: 30 * time.Minute, wantNotify: true},
		{name: "every interval in minutes", args: []string{"dashboard", "15m"}, wantInterval: 15 * time.Minute, wantNotify: true},
		{name: "every interval in hours", args: []string{"dashboard", "1h"}, wantInterval: time.Hour, wantNotify: true},
		{name: "every interval in hours and minutes", args: []string{"dashboard", "1h30m"}, wantInterval: 90 * time.Minute, wantNotify: true},
		{name: "every bare number of minutes", args: []string{"dashboard", "45"}, wantInterval: 45 * time.Minute, wantNotify: true},
		{name: "every fraction of minutes", args: []string{"dashboard", "7.5"}, wantInterval: 7*time.Minute + 30*time.Second, wantNotify: true},
		{name: "at the shortest interval", args: []string{"dashboard", "5m"}, wantInterval: 5 * time.Minute, wantNotify: true},
		{name: "without notifications", args: []string{"dashboard", "--no-notify"}, wantInterval: 30 * time.Minute, wantNotify: false},
		{name: "without notifications, every interval", args: []string{"dashboard", "--no-notify", "1h"}, wantInterval: time.Hour, wantNotify: false},
		{name: "with the interval before the flag", args: []string{"dashboard", "1h", "--no-notify"}, wantInterval: time.Hour, wantNotify: false},
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

// The dashboard's help tells every page's keys, as the dashboard's own help
// lists them, each page's on a paragraph of its own, each key whole.
func TestDashboardHelpTellsEveryPagesKeys(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "dashboard", "--help")
	help := strings.Join(strings.Fields(got.stdout), " ")
	if got.code != 0 {
		t.Fatalf("switchboard dashboard --help = %+v, want it to succeed", got)
	}
	for _, p := range watch.EveryPagesKeys() {
		told := make([]string, len(p.Keys))
		for i, k := range p.Keys {
			told[i] = k.Key + " " + k.Does
		}
		if want := p.Page + ": " + strings.Join(told, ", "); !strings.Contains(help, want) {
			t.Errorf("switchboard dashboard --help reads\n%s\nwant it to tell %q", got.stdout, want)
		}
	}
}

// Under a page's keys, the dashboard's help says what those do whose words
// don't say it, and that t and p work only where they apply.
func TestDashboardHelpSaysWhatTheKeysWordsDont(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "dashboard", "--help")
	help := strings.Join(strings.Fields(got.stdout), " ")
	tests := []struct{ page, next, says string }{
		{page: "every page", next: "Overview:", says: "t and p work where they apply: t not under NO_COLOR, and p neither with one account"},
		{page: "every page", next: "Overview:", says: "nor with --probe"},
		{page: "every page", next: "Overview:", says: "j, k, PgDn, PgUp and the wheel scroll a view where it doesn't fit: the Overview's USAGE"},
		{page: "every page", next: "Overview:", says: "Runway's lanes, Graphs' page, the Log's table"},
		{page: "every page", next: "Overview:", says: "w, where it's listed as models, shows and hides each model's own window"},
		{page: "Overview", next: "Accounts:", says: "b groups the windows by window or by account"},
		{page: "Overview", next: "Accounts:", says: "↑↓ scrolls USAGE, as j and k do, where it doesn't fit, left to ? as they are"},
		{page: "Accounts", next: "History:", says: "< and > step the period both views count over"},
		{page: "History", next: "Sessions:", says: "< and > step the Year back through each calendar year the ledger holds"},
		{page: "History", next: "Sessions:", says: "Tokens' periods between a day, a week and a month"},
		{page: "History", next: "Sessions:", says: "v cycles what the Year shows, all use, a model version or an account"},
		{page: "History", next: "Sessions:", says: "←→ steps the Year shown for one account between the accounts"},
		{page: "Runway", next: "Graphs:", says: "← steps back to today, then a day at a time, or to this week, then a week at a time, and → on again to the live view"},
		{page: "Graphs", next: "Log:", says: "w, f, o, g and h show and hide WEEK, 5-HOUR, POOL, GLANCE and WHEN YOU WORK"},
		{page: "Log", next: "a session's page:", says: "/ finds a session by its id's start or its directory's name"},
		{page: "a session's page", next: "The Overview's sections", says: "esc goes back where the page was opened from"},
	}
	for _, tt := range tests {
		from := strings.Index(help, " "+tt.page+": ")
		under, _, _ := strings.Cut(help[max(from, 0):], " "+tt.next)
		if from < 0 || !strings.Contains(under, tt.says) {
			t.Errorf("switchboard dashboard --help reads\n%s\nwant it to say %q under %s's keys", got.stdout, tt.says, tt.page)
		}
	}
}

// GRAPHS is gone from the Overview, and its keys with it: the dashboard's
// help names neither it, nor g, which showed it, nor v, which flipped its
// charts, nor the focus that served it, among the Overview's keys.
func TestDashboardHelpNamesNoGRAPHSAmongTheOverviewsKeys(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "dashboard", "--help")
	help := strings.Join(strings.Fields(got.stdout), " ")
	from := strings.Index(help, " Overview: ")
	overview, _, _ := strings.Cut(help[max(from, 0):], " Accounts:")
	for _, gone := range []string{"GRAPHS", "g graphs", "v chart", "burn-", "focus"} {
		if from < 0 || strings.Contains(overview, gone) {
			t.Errorf("switchboard dashboard --help reads, under the Overview's keys,\n%s\nwant no %q", overview, gone)
		}
	}
}

func TestDashboardNotifiesAsTheConfigSays(t *testing.T) {
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

			if got := run(t, srv.deps, "dashboard"); got != (result{}) {
				t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
			}
			if cfg.Notifications != tt.want {
				t.Errorf("notifications = %+v, want %+v", cfg.Notifications, tt.want)
			}
		})
	}
}

func TestDashboardPricesAsTheConfigSays(t *testing.T) {
	tests := []struct {
		name           string
		extra          string
		wantOverridden bool
	}{
		{name: "at the table's prices, where it gives none"},
		{name: "at its own, where it gives any", extra: "\n[prices.plans]\nmax5x = 90\n", wantOverridden: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), nil)
			srv.extra = tt.extra
			srv.writeConfig(t)
			cfg := recordWatch(t, &srv.deps)

			if got := run(t, srv.deps, "dashboard"); got != (result{}) {
				t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
			}
			if cfg.Prices.AsOf != ledger.Pricing.AsOf || cfg.Prices.Overridden != tt.wantOverridden {
				t.Errorf("the prices are as of %s, the config's standing in: %v; want as of %s, the config's: %v",
					cfg.Prices.AsOf, cfg.Prices.Overridden, ledger.Pricing.AsOf, tt.wantOverridden)
			}
		})
	}
}

func TestDashboardReadsWhatStatusReads(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"TERM": "xterm-256color"})
	cfg := recordWatch(t, &deps)
	run(t, deps, "dashboard")

	doc, _, err := cfg.Source.Read(t.Context(), status.Read{Probe: true})
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
		t.Errorf("the dashboard reads\n%s\nwant what status --json reads\n%s", fetched.String(), want)
	}
	if got := cfg.Now(); !got.Equal(testNow) {
		t.Errorf("clock reads %v, want the command's clock at %v", got, testNow)
	}
	if want := (score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}); !reflect.DeepEqual(cfg.Policy, want) {
		t.Errorf("policy = %+v, want Claude's %+v", cfg.Policy, want)
	}
	if cfg.WindowName == nil {
		t.Fatal("the dashboard names no window, want Claude's names")
	}
	for key, want := range map[string]string{"5h": "5-hour", "7d": "Week", "7d_oi": "Fable week"} {
		if got := cfg.WindowName(key); got != want {
			t.Errorf("the dashboard names window %s %q, want %q, as usage names it", key, got, want)
		}
	}
}

func TestDashboardReadsTheLedgerTheReadingsAndTheEventsInTheStateDirectory(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	configure(t, deps)
	state := stateDir(t, deps)
	line := ledger.Line{At: october(7, 9, 0, 0).UTC(), Request: "c1", Kind: ledger.KindMessage, Session: sessionA, Model: "claude-opus-5-5", Account: "work",
		Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 14230, Usage: json.RawMessage(cachedUsage)}
	reading := readings.Reading{At: october(7, 9, 0, 0).UTC(), Account: "work", Key: "5h", Utilization: 0.3, ResetsAt: october(7, 14, 0, 0).UTC(),
		Status: quota.StatusAllowed, Source: readings.FromAnswer}
	event := events.Line{ID: 1, At: october(7, 10, 0, 0).UTC(), Kind: status.EventLimit, Account: "work", Run: october(7, 8, 0, 0).UTC()}
	writeStateFile(t, ledger.Dir(state), "requests-2026-10-07.jsonl", jsonLine(t, line))
	writeStateFile(t, readings.Dir(state), "readings-2026-10-07.jsonl", jsonLine(t, reading))
	writeStateFile(t, ledger.Dir(state), "events-2026-10-07.jsonl", jsonLine(t, event))
	cfg := recordWatch(t, &deps)

	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	today := october(7, 0, 0, 0)
	if lines, _, _ := cfg.Ledger.Today(ledger.Mark{}); len(lines) != 1 || lines[0].Request != "c1" {
		t.Errorf("the ledger's reader reads today's lines as %+v, want the one written", lines)
	}
	if lines := slices.Collect(cfg.Ledger.Session(sessionA)); len(lines) != 1 || lines[0].Request != "c1" {
		t.Errorf("the ledger's reader reads the session's lines as %+v, want the one written", lines)
	}
	if days := cfg.Ledger.Days(today); len(days) != 1 || days[0].Day != "2026-10-07" || days[0].Lines != 1 {
		t.Errorf("the ledger's reader reads the days as %+v, want today's, of the line written", days)
	}
	if got := slices.Collect(cfg.Readings.Between(today, ledgerNow)); len(got) != 1 || !got[0].At.Equal(reading.At) || got[0].Window() != reading.Window() {
		t.Errorf("the readings history's reader reads %+v, want the reading written, %+v", got, reading)
	}
	if got := slices.Collect(cfg.Events.Between(today, ledgerNow)); len(got) != 1 || got[0].ID != 1 || got[0].Kind != status.EventLimit {
		t.Errorf("the events' reader reads %+v, want the event written, %+v", got, event)
	}
}

// History is given what the history command builds its blocks of: the
// configured accounts, whose plans its worth is priced against, the day the
// config starts its weeks on, Claude's week, and Claude's families.
func TestDashboardGivesHistoryTheAccountsTheWeeksAndTheFamilies(t *testing.T) {
	path := writeConfig(t, "week_starts = \"sunday\"\n\n[[account]]\nid = \"work\"\nplan = \"max20x\"\n\n[[account]]\nid = \"side\"\n")
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())
	cfg := recordWatch(t, &deps)

	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	var accounts []string
	for _, a := range cfg.Accounts {
		accounts = append(accounts, a.ID+":"+a.Plan)
	}
	if want := []string{"work:max20x", "side:"}; !slices.Equal(accounts, want) {
		t.Errorf("the dashboard is given the accounts %q, want the config's, %q", accounts, want)
	}
	if cfg.WeekStarts != time.Sunday || cfg.WeekWindow != "7d" {
		t.Errorf("the dashboard's weeks start on %s, its accounts' the window %q; want Sunday, as the config says, and Claude's week, 7d", cfg.WeekStarts, cfg.WeekWindow)
	}
	if cfg.Family == nil {
		t.Fatal("the dashboard reads no model's family, want Claude's")
	}
	if got := cfg.Family("claude-opus-5-5"); got != "opus" {
		t.Errorf("the dashboard reads claude-opus-5-5's family as %q, want opus", got)
	}
}

// The dashboard shows a session's directory under the home it's given, as
// ~; without one, whole.
func TestDashboardGivesTheWatchTheHome(t *testing.T) {
	home := t.TempDir()
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, "[[account]]\nid = \"work\"\n")}, home)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) || cfg.Home != home {
		t.Errorf("switchboard dashboard = %+v, giving the watch the home %q; want %q", got, cfg.Home, home)
	}

	deps.HomeDir = func() (string, error) { return "", errors.New("no home") }
	cfg = recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) || cfg.Home != "" {
		t.Errorf("without a home, switchboard dashboard = %+v, giving the watch the home %q; want none", got, cfg.Home)
	}
}

func TestDashboardReadsNothingWithoutAStateDirectory(t *testing.T) {
	tests := []struct {
		name   string
		noHome bool
	}{
		{name: "where it isn't there yet"},
		{name: "where there's none to find", noHome: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, ledgerConfig)}, t.TempDir())
			deps.Now = func() time.Time { return ledgerNow }
			if tt.noHome {
				deps.HomeDir = func() (string, error) { return "", errors.New("no home directory") }
			}
			cfg := recordWatch(t, &deps)

			if got := run(t, deps, "dashboard"); got != (result{}) {
				t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
			}
			if cfg.Ledger == nil || cfg.Readings == nil || cfg.Events == nil {
				t.Fatalf("the watch is given the readers %#v, %#v and %#v, want one of each", cfg.Ledger, cfg.Readings, cfg.Events)
			}
			from := october(1, 0, 0, 0)
			if lines, _, _ := cfg.Ledger.Today(ledger.Mark{}); len(lines) > 0 {
				t.Errorf("the ledger's reader reads today's lines as %+v, want none", lines)
			}
			if lines := slices.Collect(cfg.Ledger.Session(sessionA)); len(lines) > 0 {
				t.Errorf("the ledger's reader reads the session's lines as %+v, want none", lines)
			}
			if days := cfg.Ledger.Days(from); len(days) != 1 || days[0].Day != "2026-10-07" || days[0].Lines != 0 {
				t.Errorf("the ledger's reader reads the days as %+v, want today's alone, of no requests", days)
			}
			if got := slices.Collect(cfg.Readings.Between(from, ledgerNow)); len(got) > 0 {
				t.Errorf("the readings history's reader reads %+v, want none", got)
			}
			if got := slices.Collect(cfg.Events.Between(from, ledgerNow)); len(got) > 0 {
				t.Errorf("the events' reader reads %+v, want none", got)
			}
			if tt.noHome {
				return
			}
			state := stateDir(t, deps)
			for _, dir := range []string{ledger.Dir(state), readings.Dir(state)} {
				if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("%s is there (%v), want it left as it was, not there", dir, err)
				}
			}
		})
	}
}

// jsonLine returns v as a line of JSON, as the state directory's files hold
// it.
func jsonLine(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func TestDashboardDrawsAtTheEnvironmentsSizeUntilTheTerminalGivesOne(t *testing.T) {
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
			run(t, deps, "dashboard")

			if cfg.Size != tt.want {
				t.Errorf("size = %+v, want %+v", cfg.Size, tt.want)
			}
		})
	}
}

func TestDashboardClaimsTheVersionInstalledAtEachRead(t *testing.T) {
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
	run(t, deps, "dashboard")

	for _, version := range []string{"2.1.300", "2.1.301"} {
		installed = version
		mu.Lock()
		agents = nil
		mu.Unlock()
		if _, _, err := cfg.Source.Read(t.Context(), status.Read{Probe: true}); err != nil {
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

func TestDashboardArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "two intervals", args: []string{"dashboard", "15m", "1h"}, wantErr: "dashboard takes one interval, not 2"},
		{name: "an interval that can't be read", args: []string{"dashboard", "soon"}, wantErr: `invalid interval "soon": give a duration, such as 15m or 1h, or a number of minutes`},
		{name: "an interval with an unknown unit", args: []string{"dashboard", "2d"}, wantErr: `invalid interval "2d": give a duration, such as 15m or 1h, or a number of minutes`},
		{name: "an interval too short", args: []string{"dashboard", "4m59s"}, wantErr: "interval 4m59s is too short: the shortest is 5m"},
		{name: "a number of minutes too few", args: []string{"dashboard", "2"}, wantErr: "interval 2 is too short: the shortest is 5m"},
		{name: "no interval at all", args: []string{"dashboard", "0"}, wantErr: "interval 0 is too short: the shortest is 5m"},
		{name: "a negative interval", args: []string{"dashboard", "--", "-10m"}, wantErr: "interval -10m is too short: the shortest is 5m"},
		{name: "refreshing, which r does on screen", args: []string{"dashboard", "-r"}, wantErr: "unknown shorthand flag: 'r' in -r"},
		{name: "refreshing, in full", args: []string{"dashboard", "--refresh"}, wantErr: "unknown flag: --refresh"},
		{name: "printing JSON, which usage does", args: []string{"dashboard", "--json"}, wantErr: "unknown flag: --json"},
		{name: "printing for a person, which it always does", args: []string{"dashboard", "--pretty"}, wantErr: "unknown flag: --pretty"},
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

func TestDashboardNeedsATerminal(t *testing.T) {
	got := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "dashboard")
	want := result{stderr: "Error: dashboard needs a terminal, and stdout isn't one\n", code: 1}
	if got != want {
		t.Errorf("switchboard dashboard = %+v, want %+v", got, want)
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

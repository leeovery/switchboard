package cli_test

import (
	"fmt"
	"image/color"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// lakeTheme is a theme file of Nord's base tokens, its canvas its own.
const lakeTheme = `text.primary = #ECEFF4
text.secondary = #E5E9F0
text.tertiary = #D8DEE9
text.muted = #939EB2
text.subtle = #73819B
text.faint = #4C566A
text.on-selection = #FFFFFF
accent.primary = #B48EAD
accent.key = #81A1C1
accent.mode = #88C0D0
accent.attention = #EBCB8B
state.positive = #A7C492
state.destructive = #DD8188
canvas = #102030
bg.selection = #434C5E
bg.attention = #3D4046
bg.subtle = #3B4252
border = #4C566A
text.on-attention = #ECEFF4
`

// truecolour is an environment whose terminal shows every colour.
var truecolour = map[string]string{"CLICOLOR_FORCE": "1", "TERM": "xterm-256color", "COLORTERM": "truecolor"}

func TestDashboardIsDrawnInTheThemesChosen(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	writePrefs(t, deps, `{"theme_light": "lake", "theme_dark": "amber"}`)
	writeTheme(t, themesDir(t, deps), "lake", lakeTheme)
	cfg := recordWatch(t, &deps)
	run(t, deps, "dashboard")

	if want := (theme.Choice{Light: "lake", Dark: "amber"}); cfg.Choice != want {
		t.Errorf("choice = %+v, want %+v, as prefs.json keeps it", cfg.Choice, want)
	}
	if cfg.Pair.Light.Slug != "lake" || cfg.Pair.Dark.Slug != "amber" {
		t.Errorf("pair = %s and %s, want lake, from the themes directory, and amber", cfg.Pair.Light.Slug, cfg.Pair.Dark.Slug)
	}
	if cfg.Themes == nil {
		t.Fatal("no themes for the picker, want them")
	}
	if !slices.ContainsFunc(cfg.Themes.List().Entries, func(e theme.Entry) bool { return e.Slug == "lake" && e.Problem == nil }) {
		t.Error("the picker doesn't list lake, from the themes directory")
	}
	if _, err := cfg.Themes.Keep(func(theme.Choice) theme.Choice { return theme.One("exchange") }); err != nil {
		t.Fatalf("Keep() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(stateDir(t, deps), "prefs.json"))
	if err != nil || !strings.Contains(string(data), `"theme": "exchange"`) {
		t.Errorf("prefs.json holds %s, %v; want the choice kept", data, err)
	}
}

func TestTheDashboardsThemesKeepAChoiceAsItStandsNow(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	writePrefs(t, deps, `{"theme_light": "exchange", "view": "accounts"}`)
	cfg := recordWatch(t, &deps)
	run(t, deps, "dashboard")
	writePrefs(t, deps, `{"theme_light": "tokyo-night", "view": "accounts"}`)

	if got, want := cfg.Themes.Chosen(), (theme.Choice{Light: "tokyo-night"}); got != want {
		t.Errorf("Chosen() = %+v, want %+v, as another dashboard kept it since", got, want)
	}
	kept, err := cfg.Themes.Keep(func(c theme.Choice) theme.Choice { return c.WithHalf(true, "amber") })
	if want := (theme.Choice{Light: "tokyo-night", Dark: "amber"}); err != nil || kept != want {
		t.Errorf("Keep() = %+v, %v; want %+v, the half set beside the one kept since", kept, err, want)
	}
	data, err := os.ReadFile(filepath.Join(stateDir(t, deps), "prefs.json"))
	if err != nil || !strings.Contains(string(data), `"theme_light": "tokyo-night"`) || !strings.Contains(string(data), `"view": "accounts"`) {
		t.Errorf("prefs.json holds %s, %v; want the choice kept beside the view", data, err)
	}
}

func TestDashboardOpensOnTheTabKept(t *testing.T) {
	for _, env := range []map[string]string{nil, {"NO_COLOR": "1"}} {
		deps := statusDeps(t, fakeClaudeAPI(t), env)
		writePrefs(t, deps, `{"theme": "amber", "tab": "accounts"}`)
		cfg := recordWatch(t, &deps)
		run(t, deps, "dashboard")

		if cfg.View != dashboard.Accounts {
			t.Errorf("with %v, the dashboard opens on %q, want the tab prefs.json keeps", env, cfg.View)
		}
		if cfg.Prefs == nil {
			t.Fatalf("with %v, the dashboard has nowhere to keep the tab shown", env)
		}
		if err := cfg.Prefs.Update(func(p *theme.Prefs) { p.Tab = "sessions" }); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		data, err := os.ReadFile(filepath.Join(stateDir(t, deps), "prefs.json"))
		if err != nil || !strings.Contains(string(data), `"tab": "sessions"`) || !strings.Contains(string(data), `"theme": "amber"`) {
			t.Errorf("with %v, prefs.json holds %s, %v; want the tab kept beside the theme", env, data, err)
		}
	}
}

func TestDashboardOpensMilestone5sCardViewOnTheOverview(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	writePrefs(t, deps, `{"theme": "amber", "view": "accounts"}`)
	cfg := recordWatch(t, &deps)
	run(t, deps, "dashboard")

	if cfg.View != "" {
		t.Errorf("the dashboard is given %q to open on, want no tab, for the Overview, which shows milestone 5's cards now", cfg.View)
	}
}

func TestDashboardShowsTheOverviewsSectionsKept(t *testing.T) {
	tests := []struct {
		name, prefs string
		want        []string
	}{
		{name: "none kept: those shown at first", prefs: `{"tab": "overview"}`, want: []string{"usage", "routing", "coming", "lately"}},
		{name: "those kept", prefs: `{"sections": ["graphs", "usage"]}`, want: []string{"usage", "graphs"}},
		{name: "none shown, kept as none", prefs: `{"sections": []}`, want: []string{}},
		{name: "one a later switchboard names passed over", prefs: `{"sections": ["usage", "a-later-section"]}`, want: []string{"usage"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)
			writePrefs(t, deps, tt.prefs)
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			if got := cfg.Sections.Names(); !slices.Equal(got, tt.want) {
				t.Errorf("the dashboard opens showing %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDashboardGroupsTheOverviewsWindowsAsKeptElseAsConfigured(t *testing.T) {
	tests := []struct {
		name, prefs string
		// layout is [usage] by in the config, "" for none.
		layout string
		want   dashboard.By
	}{
		{name: "neither saying: by window", prefs: `{}`, want: dashboard.ByWindow},
		{name: "as the config says", prefs: `{}`, layout: "account", want: dashboard.ByAccount},
		{name: "as kept", prefs: `{"by": "account"}`, want: dashboard.ByAccount},
		{name: "as kept, over the config", prefs: `{"by": "window"}`, layout: "account", want: dashboard.ByWindow},
		{name: "one kept it doesn't know: as the config says", prefs: `{"by": "a-later-grouping"}`, layout: "account", want: dashboard.ByAccount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := fakeClaudeAPI(t)
			text := fmt.Sprintf("upstream = %q\n", upstream) + threeAccounts
			if tt.layout != "" {
				text += "\n[usage]\nby = \"" + tt.layout + "\"\n"
			}
			deps := statusDeps(t, upstream, map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, text)})
			writePrefs(t, deps, tt.prefs)
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			if cfg.By != tt.want {
				t.Errorf("the dashboard groups the Overview's windows by %v, want %v", cfg.By, tt.want)
			}
		})
	}
}

// History opens on what prefs.json keeps of its views, the Year's account
// among those the config gives; none kept, as it opens at first.
func TestDashboardShowsHistoryAsKept(t *testing.T) {
	kept := `{"history": {"view": "tokens", "year": "side", "weeks": "columns", "tokens": {"period": "month", "split": "model"}}}`
	tests := []struct {
		name, prefs string
		want        dashboard.HistoryPage
	}{
		{name: "none kept", prefs: `{}`},
		{
			name: "as kept", prefs: kept,
			want: dashboard.HistoryPage{
				Shown: dashboard.HistoryTokens, YearAsk: views.YearAsk{Account: "side"}, WeeksShape: dashboard.WeeksColumns,
				TokensAsk: views.TokensAsk{Period: views.TokensByMonth}, TokensSplit: dashboard.TokensByModel,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := fakeClaudeAPI(t)
			deps := statusDeps(t, upstream, map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, fmt.Sprintf("upstream = %q\n", upstream)+threeAccounts)})
			writePrefs(t, deps, tt.prefs)
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			got := cfg.History
			if got.Shown != tt.want.Shown || got.YearAsk != tt.want.YearAsk || got.WeeksShape != tt.want.WeeksShape ||
				got.TokensAsk != tt.want.TokensAsk || got.TokensSplit != tt.want.TokensSplit {
				t.Errorf("the dashboard opens History on %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Accounts opens on the sub-tab and the period prefs.json keeps; none kept,
// as it opens at first: Compare, this week.
func TestDashboardShowsAccountsAsKept(t *testing.T) {
	tests := []struct {
		name, prefs string
		wantView    dashboard.AccountsView
		wantPeriod  views.AccountsPeriod
	}{
		{name: "none kept", prefs: `{}`},
		{name: "as kept", prefs: `{"accounts": {"view": "detail", "period": "12-weeks"}}`, wantView: dashboard.AccountsDetail, wantPeriod: views.AccountsTwelveWeeks},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)
			writePrefs(t, deps, tt.prefs)
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			if got := cfg.AccountsShown; got.Shown != tt.wantView || got.Period != tt.wantPeriod {
				t.Errorf("the dashboard opens Accounts on %+v, want %d over %q", got, tt.wantView, tt.wantPeriod)
			}
		})
	}
}

// The Log shows the columns prefs.json keeps shown; none kept, those shown
// at first.
func TestDashboardShowsTheLogsColumnsAsKept(t *testing.T) {
	tests := []struct {
		name, prefs string
		want        dashboard.LogTicks
	}{
		{name: "none kept", prefs: `{}`},
		{name: "as kept", prefs: `{"log": {"columns": ["session", "worth"]}}`, want: dashboard.LogKept(theme.LogPrefs{Columns: []string{"session", "worth"}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)
			writePrefs(t, deps, tt.prefs)
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			if cfg.Log != tt.want {
				t.Errorf("the dashboard shows the Log's columns %+v, want %+v", cfg.Log, tt.want)
			}
		})
	}
}

// The Log's oldest day is as [ledger] keep says: 400 days where it doesn't
// say, and forever as dayfile.Forever.
func TestDashboardKeepsTheLedgerAsTheConfigSays(t *testing.T) {
	tests := []struct {
		name, ledger string
		want         time.Duration
	}{
		{name: "not said", want: config.DefaultLedgerKeep},
		{name: "30 days", ledger: "\n[ledger]\nkeep = \"30d\"\n", want: 30 * 24 * time.Hour},
		{name: "forever", ledger: "\n[ledger]\nkeep = \"forever\"\n", want: dayfile.Forever},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := fakeClaudeAPI(t)
			deps := statusDeps(t, upstream, map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, fmt.Sprintf("upstream = %q\n", upstream)+threeAccounts+tt.ledger)})
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			if cfg.LedgerKeep != tt.want {
				t.Errorf("the dashboard keeps the ledger %v, want %v", cfg.LedgerKeep, tt.want)
			}
		})
	}
}

func TestDashboardFindsThemesWhereSwitchboardThemesDirSays(t *testing.T) {
	dotfiles := t.TempDir()
	deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_THEMES_DIR": dotfiles})
	writeTheme(t, dotfiles, "lake", lakeTheme)
	writePrefs(t, deps, `{"theme": "lake"}`)
	cfg := recordWatch(t, &deps)
	run(t, deps, "dashboard")

	if cfg.Pair.Dark.Slug != "lake" {
		t.Errorf("drawn in %s, want lake, from $SWITCHBOARD_THEMES_DIR", cfg.Pair.Dark.Slug)
	}
}

func TestDashboardUnderNoColourHasNoThemes(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"NO_COLOR": "yes"})
	cfg := recordWatch(t, &deps)
	run(t, deps, "dashboard")

	if cfg.Themes != nil {
		t.Error("under NO_COLOR, the dashboard has themes, want none: no colour, and t doing nothing")
	}
}

func TestACorruptPreferencesFileLeavesTheDefaults(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	writePrefs(t, deps, `{"theme": `)
	cfg := recordWatch(t, &deps)
	run(t, deps, "dashboard")

	if cfg.Choice != (theme.Choice{}) || cfg.Pair.Dark.Slug != theme.DefaultDark {
		t.Errorf("choice = %+v, drawn in %s on a dark terminal; want the defaults", cfg.Choice, cfg.Pair.Dark.Slug)
	}
	matches, _ := filepath.Glob(filepath.Join(stateDir(t, deps), "prefs.json.corrupt-*"))
	if len(matches) != 1 {
		t.Errorf("set aside: %q, want the corrupt file set aside once", matches)
	}
}

func TestUsageIsPrintedInTheThemeForTheTerminalsBackground(t *testing.T) {
	tests := []struct {
		name string
		// prefs is what prefs.json holds, "" for none.
		prefs      string
		background color.Color
		// want is the theme's text.primary, as a foreground's SGR, and
		// canvas its canvas, as a background's.
		want, canvas string
	}{
		{name: "a dark terminal: the dark default, nord", background: color.Black, want: "38;2;236;239;244", canvas: "48;2;46;52;64"},
		{name: "a terminal that doesn't say: nord", want: "38;2;236;239;244", canvas: "48;2;46;52;64"},
		{name: "a light terminal: the light default, tokyo-night-day", background: color.White, want: "38;2;46;60;100", canvas: "48;2;225;226;231"},
		{name: "one theme chosen, on a terminal as dark", prefs: `{"theme": "amber"}`, background: color.Black, want: "38;2;255;210;122", canvas: "48;2;14;11;6"},
		{
			name: "one dark theme chosen, on a light terminal: the light default", prefs: `{"theme": "amber"}`, background: color.White,
			want: "38;2;46;60;100", canvas: "48;2;225;226;231",
		},
		{
			name: "one light theme chosen, on a terminal that doesn't say: the dark default", prefs: `{"theme": "tokyo-night-day"}`,
			want: "38;2;236;239;244", canvas: "48;2;46;52;64",
		},
		{name: "a pair chosen, on a light terminal", prefs: `{"theme_light": "amber"}`, background: color.White, want: "38;2;255;210;122", canvas: "48;2;14;11;6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := goldenDeps(t, truecolour)
			if tt.prefs != "" {
				writePrefs(t, deps, tt.prefs)
			}
			deps.Background = func(io.Writer) color.Color { return tt.background }

			got := run(t, deps, "usage")

			if got.code != 0 || !strings.Contains(got.stdout, tt.want) {
				t.Errorf("switchboard usage = %+v, want it in the colour %q", got, tt.want)
			}
			if strings.Contains(got.stdout, tt.canvas) {
				t.Errorf("switchboard usage paints the theme's canvas, want it printed on the terminal's own background")
			}
			for i, row := range strings.Split(ansi.Strip(got.stdout), "\n") {
				if strings.TrimRight(row, " ") != row {
					t.Errorf("switchboard usage printed row %d as %q, want it to end at its last glyph", i+1, row)
				}
			}
		})
	}
}

func TestUsageUnderNoColourAsksTheTerminalNothing(t *testing.T) {
	env := map[string]string{"NO_COLOR": "yes", "TTY_FORCE": "1", "TERM": "xterm-256color"}
	deps := goldenDeps(t, env)
	deps.Background = func(io.Writer) color.Color {
		t.Error("under NO_COLOR, usage asked the terminal its background")
		return nil
	}

	got := run(t, deps, "usage")

	if got.code != 0 || strings.Contains(got.stdout, "\x1b[38;") || !strings.Contains(got.stdout, "\x1b[1m") {
		t.Errorf("switchboard usage under NO_COLOR = %+v, want no colour, but bold", got)
	}
}

func TestUsageOnATerminalThatShowsNoColourIsPrintedAsUnderNoColour(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "a dumb terminal", env: map[string]string{"TTY_FORCE": "1", "TERM": "dumb"}},
		{name: "a terminal that names no kind", env: map[string]string{"TTY_FORCE": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			noColour := maps.Clone(tt.env)
			noColour["NO_COLOR"] = "1"

			got := run(t, goldenDeps(t, tt.env), "usage")
			if want := run(t, goldenDeps(t, noColour), "usage"); got.code != 0 || got != want {
				t.Errorf("switchboard usage =\n%+v\nwant what it prints under NO_COLOR\n%+v", got, want)
			}
		})
	}
}

func TestUsageAsksTheTerminalItsBackgroundOnlyWhereItShowsColour(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		asks bool
	}{
		{name: "a terminal of 256 colours", env: map[string]string{"TTY_FORCE": "1", "TERM": "xterm-256color"}, asks: true},
		{name: "a dumb terminal", env: map[string]string{"TTY_FORCE": "1", "TERM": "dumb"}},
		{name: "a terminal that names no kind", env: map[string]string{"TTY_FORCE": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := goldenDeps(t, tt.env)
			asked := false
			deps.Background = func(io.Writer) color.Color {
				asked = true
				return nil
			}

			if got := run(t, deps, "usage"); got.code != 0 {
				t.Fatalf("switchboard usage = %+v", got)
			}
			if asked != tt.asks {
				t.Errorf("the terminal was asked its background: %v, want %v", asked, tt.asks)
			}
		})
	}
}

func TestTheTerminalsBackgroundIsntAskedOfAnythingButATerminal(t *testing.T) {
	var out strings.Builder
	if got := cli.TerminalBackground(&out); got != nil || out.Len() > 0 {
		t.Errorf("TerminalBackground() of no terminal = %v, writing %q; want nothing asked", got, out.String())
	}
}

// writePrefs writes the preferences file commands run with deps find,
// holding content.
func writePrefs(t *testing.T, deps cli.Deps, content string) {
	t.Helper()
	path := filepath.Join(stateDir(t, deps), "prefs.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeTheme writes a theme file named for slug in dir, holding content.
func writeTheme(t *testing.T, dir, slug, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug+".theme"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// themesDir is where commands run with deps find the themes.
func themesDir(t *testing.T, deps cli.Deps) string {
	t.Helper()
	dir, err := config.ThemesDir(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

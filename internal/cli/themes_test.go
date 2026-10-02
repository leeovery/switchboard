package cli_test

import (
	"image/color"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/theme"
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

func TestUsageWatchIsDrawnInTheThemesChosen(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	writePrefs(t, deps, `{"theme_light": "lake", "theme_dark": "amber"}`)
	writeTheme(t, themesDir(t, deps), "lake", lakeTheme)
	cfg := recordWatch(t, &deps)
	run(t, deps, "usage", "--watch")

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
	if err := cfg.Themes.Keep(theme.One("exchange")); err != nil {
		t.Fatalf("Keep() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(stateDir(t, deps), "prefs.json"))
	if err != nil || !strings.Contains(string(data), `"theme": "exchange"`) {
		t.Errorf("prefs.json holds %s, %v; want the choice kept", data, err)
	}
}

func TestUsageWatchFindsThemesWhereSwitchboardThemesDirSays(t *testing.T) {
	dotfiles := t.TempDir()
	deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_THEMES_DIR": dotfiles})
	writeTheme(t, dotfiles, "lake", lakeTheme)
	writePrefs(t, deps, `{"theme": "lake"}`)
	cfg := recordWatch(t, &deps)
	run(t, deps, "usage", "--watch")

	if cfg.Pair.Dark.Slug != "lake" {
		t.Errorf("drawn in %s, want lake, from $SWITCHBOARD_THEMES_DIR", cfg.Pair.Dark.Slug)
	}
}

func TestUsageWatchUnderNoColourHasNoThemes(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"NO_COLOR": "yes"})
	cfg := recordWatch(t, &deps)
	run(t, deps, "usage", "--watch")

	if cfg.Themes != nil {
		t.Error("under NO_COLOR, the dashboard has themes, want none: no colour, and t doing nothing")
	}
}

func TestACorruptPreferencesFileLeavesTheDefaults(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	writePrefs(t, deps, `{"theme": `)
	cfg := recordWatch(t, &deps)
	run(t, deps, "usage", "--watch")

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
		// want is the theme's text.primary, as a foreground's SGR.
		want string
	}{
		{name: "a dark terminal: the dark default, nord", background: color.Black, want: "38;2;236;239;244"},
		{name: "a terminal that doesn't say: nord", want: "38;2;236;239;244"},
		{name: "a light terminal: the light default, tokyo-night-day", background: color.White, want: "38;2;46;60;100"},
		{name: "one theme chosen, whatever the background", prefs: `{"theme": "amber"}`, background: color.White, want: "38;2;255;210;122"},
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
			if strings.Contains(got.stdout, "48;2;") {
				t.Errorf("switchboard usage paints a background, want it printed on the terminal's own")
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

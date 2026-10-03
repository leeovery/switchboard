package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/capture"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

func TestPrintsAFixturesFrameAsTextAtItsSize(t *testing.T) {
	out := capturing(t, "--fixture", "accounts-3", "--print")

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 34 {
		t.Errorf("printed %d lines, want the fixture's 34", len(lines))
	}
	if !strings.Contains(out, "SWITCHBOARD") {
		t.Errorf("printed\n%s\nwant the dashboard", out)
	}
	if strings.ContainsRune(out, ansi.ESC) {
		t.Errorf("printed escape codes:\n%q\nwant text alone", out)
	}
	for i, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("line %d, %q, ends in spaces, want them trimmed", i+1, line)
		}
	}
}

func TestPrintsItsColoursWithAnsi(t *testing.T) {
	out := capturing(t, "--fixture", "accounts-3", "--print", "--ansi")

	if !strings.ContainsRune(out, ansi.ESC) {
		t.Errorf("printed\n%s\nwant it in colour", out)
	}
	if text := capturing(t, "--fixture", "accounts-3", "--print"); plain(out) != text {
		t.Errorf("in colour, the frame reads\n%s\nwant it to read as it does without\n%s", plain(out), text)
	}
}

func TestPrintsAtTheSizeGiven(t *testing.T) {
	out := capturing(t, "--fixture", "accounts-3", "--print", "--size", "100x20")

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 20 {
		t.Errorf("printed %d lines, want 20", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 100 {
			t.Errorf("line %d is %d cells wide, want 100 at most", i+1, width)
		}
	}
}

func TestRefusesWhatTheCommandLineCantMean(t *testing.T) {
	available := "(available: " + strings.Join(capture.Names(), ", ") + ")"
	scenarios := "(available: " + strings.Join(capture.ScenarioNames(), ", ") + ")"
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no fixture", args: []string{"--print"}, want: "--fixture: name a fixture " + available},
		{name: "an unknown fixture", args: []string{"--fixture", "accounts-2", "--print"}, want: `--fixture: unknown fixture "accounts-2" ` + available},
		{name: "a size full screen", args: []string{"--fixture", "accounts-3", "--size", "80x24"}, want: "--size takes --print: full screen, the terminal gives the size"},
		{name: "colours full screen", args: []string{"--fixture", "accounts-3", "--ansi"}, want: "--ansi takes --print: full screen, the frame is drawn in colour"},
		{name: "an argument", args: []string{"--fixture", "accounts-3", "--print", "now"}, want: `unexpected argument "now"`},
		{name: "full screen without a terminal", args: []string{"--fixture", "accounts-3"}, want: "full screen, it needs a terminal: --print draws without one"},
		{name: "an unknown scenario", args: []string{"--scenario", "accounts-3"}, want: `--scenario: unknown scenario "accounts-3" ` + scenarios},
		{name: "a fixture and a scenario", args: []string{"--fixture", "accounts-3", "--scenario", "routing"}, want: "--scenario and --fixture: name one"},
		{name: "a scenario printed", args: []string{"--scenario", "routing", "--print"}, want: "--print takes a fixture: a scenario plays in time, full screen"},
		{name: "a scenario's size", args: []string{"--scenario", "routing", "--size", "80x24"}, want: "--size takes --print: full screen, the terminal gives the size"},
		{name: "a scenario in a theme there isn't", args: []string{"--scenario", "routing", "--theme", "solarized"}, want: `--theme "solarized" names no built-in theme (built in: amber, exchange, nord, terminal, tokyo-night, tokyo-night-day), nor a .theme file`},
		{name: "a scenario without a terminal", args: []string{"--scenario", "routing"}, want: "a scenario plays full screen: it needs a terminal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(tt.args, &stdout, &stderr, noEnv)
			if err == nil || err.Error() != tt.want {
				t.Errorf("run(%q) error = %v, want %s", tt.args, err, tt.want)
			}
			if stdout.Len() > 0 {
				t.Errorf("run(%q) printed %q, want nothing", tt.args, stdout.String())
			}
		})
	}
}

func TestReadsASizeGivenAsWxH(t *testing.T) {
	tests := []struct {
		given string
		want  watch.Size
		fails bool
	}{
		{given: "160x34", want: watch.Size{Width: 160, Height: 34}},
		{given: "52x36", want: watch.Size{Width: 52, Height: 36}},
		{given: "160", fails: true},
		{given: "160×34", fails: true},
		{given: "x34", fails: true},
		{given: "160x", fails: true},
		{given: "0x34", fails: true},
		{given: "160x-1", fails: true},
		{given: "wide x tall", fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.given, func(t *testing.T) {
			got, err := parseSize(tt.given)
			if tt.fails {
				if want := `--size "` + tt.given + `": give it as WxH, such as 160x34`; err == nil || err.Error() != want {
					t.Errorf("parseSize(%q) error = %v, want %s", tt.given, err, want)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("parseSize(%q) = %+v, %v, want %+v", tt.given, got, err, tt.want)
			}
		})
	}
}

func TestDrawsInTheThemeGiven(t *testing.T) {
	lake := filepath.Join(t.TempDir(), "Lake Draft.theme")
	if err := os.WriteFile(lake, []byte(strings.ReplaceAll(nordFile, "#2E3440", "#102030")), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		// want is the canvas the frame is painted on, as a background's SGR.
		want string
	}{
		{name: "nord, the frames', unless given", want: "48;2;46;52;64"},
		{name: "a built-in", args: []string{"--theme", "amber"}, want: "48;2;14;11;6"},
		{name: "a theme's file, whatever it's named", args: []string{"--theme", lake}, want: "48;2;16;32;48"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := capturing(t, append([]string{"--fixture", "accounts-3", "--print", "--ansi"}, tt.args...)...)
			for i, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				if !strings.Contains(line, tt.want) {
					t.Fatalf("line %d is off the canvas %s: %q", i+1, tt.want, line)
				}
			}
		})
	}
}

func TestRefusesAThemeThatDoesntLoad(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "broken.theme")
	if err := os.WriteFile(broken, []byte("canvas = #2E3440\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, theme, want string
	}{
		{name: "no such built-in", theme: "solarized", want: `--theme "solarized" names no built-in theme (built in: amber, exchange, nord, terminal, tokyo-night, tokyo-night-day), nor a .theme file`},
		{name: "a broken file", theme: broken, want: `--theme "` + broken + `" doesn't load: missing tokens: missing text.primary`},
		{name: "no such file", theme: filepath.Join(t.TempDir(), "gone.theme"), want: "doesn't load: unreadable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run([]string{"--fixture", "accounts-3", "--print", "--theme", tt.theme}, &stdout, &stderr, noEnv)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("run() error = %v, want one saying %s", err, tt.want)
			}
			if stdout.Len() > 0 {
				t.Errorf("run() printed %q, want nothing drawn in a theme it wasn't given", stdout.String())
			}
		})
	}
}

func TestDrawsWithoutColourUnderNoColour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	noColour := func(key string) string { return map[string]string{"NO_COLOR": "1"}[key] }
	if err := run([]string{"--fixture", "accounts-3", "--print", "--ansi"}, &stdout, &stderr, noColour); err != nil {
		t.Fatal(err)
	}

	if out := stdout.String(); strings.Contains(out, "38;") || strings.Contains(out, "48;") || !strings.Contains(out, "\x1b[1m") {
		t.Errorf("under NO_COLOR, printed\n%q\nwant no colour, nor canvas, but bold", out)
	}
}

func TestDrawsThePickerOverTheView(t *testing.T) {
	out := capturing(t, "--fixture", "accounts-3-themes", "--print")

	for _, want := range []string{"│ Themes", "│ ▌ exchange", "│   nord                     ●", "SWITCHBOARD"} {
		if !strings.Contains(out, want) {
			t.Errorf("printed\n%s\nwant %q", out, want)
		}
	}
}

// nordFile is Nord's theme file, as Portal's tokens give it.
const nordFile = `text.primary = #ECEFF4
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
canvas = #2E3440
bg.selection = #434C5E
bg.attention = #3D4046
bg.subtle = #3B4252
border = #4C566A
text.on-attention = #ECEFF4
`

// capturing runs the command with args, returning what it prints.
func capturing(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := run(args, &stdout, &stderr, noEnv); err != nil {
		t.Fatalf("run(%q): %v\n%s", args, err, stderr.String())
	}
	return stdout.String()
}

// noEnv is an environment without a variable set.
func noEnv(string) string { return "" }

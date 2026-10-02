package main

import (
	"bytes"
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
	if !strings.Contains(out, "Switchboard") {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(tt.args, &stdout, &stderr)
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

// capturing runs the command with args, returning what it prints.
func capturing(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := run(args, &stdout, &stderr); err != nil {
		t.Fatalf("run(%q): %v\n%s", args, err, stderr.String())
	}
	return stdout.String()
}

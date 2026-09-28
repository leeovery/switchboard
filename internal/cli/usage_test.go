package cli_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests print")

func TestUsage(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		golden string
	}{
		{name: "at the default width", golden: "usage.golden"},
		{name: "at the width $COLUMNS gives", env: map[string]string{"COLUMNS": "150"}, golden: "usage-wide.golden"},
		{name: "ignoring a $COLUMNS that isn't a width", env: map[string]string{"COLUMNS": "wide"}, golden: "usage.golden"},
		{name: "ignoring a $COLUMNS of zero", env: map[string]string{"COLUMNS": "0"}, golden: "usage.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, statusDeps(t, fakeClaudeAPI(t), tt.env), "usage")
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
			got := run(t, statusDeps(t, fakeClaudeAPI(t), tt.env), "usage")
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

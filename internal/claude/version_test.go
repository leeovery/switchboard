package claude

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestInstalledCLIVersion(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing", "claude")
	directory := filepath.Join(dir, "directory", "claude")
	first := filepath.Join(dir, "first", "claude")
	second := filepath.Join(dir, "second", "claude")
	for _, path := range []string{directory, filepath.Dir(first), filepath.Dir(second)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, nil, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	const onPath = "/elsewhere/bin/claude"

	tests := []struct {
		name      string
		paths     []string
		pathHasIt bool
		output    string
		outputErr error
		want      string
		wantErr   bool
		wantRun   string
	}{
		{
			name:    "from the first install path holding a file",
			paths:   []string{missing, directory, first, second},
			output:  "2.1.290 (Claude Code)\n",
			want:    "2.1.290",
			wantRun: first,
		},
		{
			name:      "from PATH when no install path holds a file",
			paths:     []string{missing, directory},
			pathHasIt: true,
			output:    "2.1.290 (Claude Code)\n",
			want:      "2.1.290",
			wantRun:   onPath,
		},
		{
			name:    "the first version in the output",
			paths:   []string{first},
			output:  "claude 10.20.30-beta.1 (build 4.5.6)\n",
			want:    "10.20.30",
			wantRun: first,
		},
		{
			name:    "none without a CLI",
			paths:   []string{missing},
			wantErr: true,
		},
		{
			name:      "none when the CLI fails",
			paths:     []string{first},
			output:    "2.1.290 (Claude Code)\n",
			outputErr: errors.New("signal: killed"),
			wantErr:   true,
			wantRun:   first,
		},
		{
			name:    "none when the output has no version",
			paths:   []string{first},
			output:  "unknown option '--version'\n",
			wantErr: true,
			wantRun: first,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ran string
			cli := installedCLI{
				paths: tt.paths,
				lookPath: func(file string) (string, error) {
					if file != "claude" {
						t.Errorf("lookPath(%q), want lookPath(%q)", file, "claude")
					}
					if !tt.pathHasIt {
						return "", exec.ErrNotFound
					}
					return onPath, nil
				},
				output: func(ctx context.Context, path string, args ...string) ([]byte, error) {
					ran = path
					if !slices.Equal(args, []string{"--version"}) {
						t.Errorf("ran %s with %q, want --version", path, args)
					}
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > versionTimeout {
						t.Errorf("ran %s without a deadline within %v", path, versionTimeout)
					}
					return []byte(tt.output), tt.outputErr
				},
			}

			got, err := cli.version(t.Context())
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("version() = %q, %v; want %q, and an error %v", got, err, tt.want, tt.wantErr)
			}
			if ran != tt.wantRun {
				t.Errorf("ran %q, want %q", ran, tt.wantRun)
			}
		})
	}
}

func TestVersionCache(t *testing.T) {
	failed := errors.New("signal: killed")
	// step is a call for the version some time after the first, and what the
	// CLI answers if it's asked.
	type step struct {
		after    time.Duration
		answer   string
		fails    bool
		want     string
		wantAsks bool
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "asks the CLI once the version is an hour old",
			steps: []step{
				{after: 0, answer: "2.1.290", want: "2.1.290", wantAsks: true},
				{after: 59 * time.Minute, want: "2.1.290", wantAsks: false},
				{after: time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
				{after: time.Hour + 59*time.Minute, want: "2.1.300", wantAsks: false},
				{after: 2 * time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
			},
		},
		{
			name: "keeps the version the CLI last gave while it doesn't answer, and asks again an hour on",
			steps: []step{
				{after: 0, answer: "2.1.290", want: "2.1.290", wantAsks: true},
				{after: time.Hour, fails: true, want: "2.1.290", wantAsks: true},
				{after: time.Hour + 30*time.Minute, want: "2.1.290", wantAsks: false},
				{after: 2 * time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
			},
		},
		{
			name: "the floor until the CLI gives a version",
			steps: []step{
				{after: 0, fails: true, want: fallbackVersion, wantAsks: true},
				{after: 30 * time.Minute, want: fallbackVersion, wantAsks: false},
				{after: time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
			var now time.Time
			var current step
			asked := false
			cache := &versionCache{
				ask: func() (string, error) {
					asked = true
					if current.fails {
						return "", failed
					}
					return current.answer, nil
				},
				now: func() time.Time { return now },
			}

			for _, s := range tt.steps {
				now, current, asked = start.Add(s.after), s, false
				if got := cache.get(); got != s.want || asked != s.wantAsks {
					t.Errorf("%v on, get() = %q, asking the CLI %v; want %q, asking it %v", s.after, got, asked, s.want, s.wantAsks)
				}
			}
		})
	}
}

func TestInstallPaths(t *testing.T) {
	tests := []struct {
		name string
		home string
		want []string
	}{
		{
			name: "home directory known",
			home: "/home/tester",
			want: []string{
				"/home/tester/.local/bin/claude",
				"/opt/homebrew/bin/claude",
				"/usr/local/bin/claude",
				"/home/tester/.claude/local/claude",
			},
		},
		{
			name: "home directory unknown",
			home: "",
			want: []string{"/opt/homebrew/bin/claude", "/usr/local/bin/claude"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installPaths(tt.home); !slices.Equal(got, tt.want) {
				t.Errorf("installPaths(%q) = %q, want %q", tt.home, got, tt.want)
			}
		})
	}
}

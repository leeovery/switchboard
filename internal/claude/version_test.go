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
			name:  "the floor without a CLI",
			paths: []string{missing},
			want:  fallbackVersion,
		},
		{
			name:      "the floor when the CLI fails",
			paths:     []string{first},
			output:    "2.1.290 (Claude Code)\n",
			outputErr: errors.New("signal: killed"),
			want:      fallbackVersion,
			wantRun:   first,
		},
		{
			name:    "the floor when the output has no version",
			paths:   []string{first},
			output:  "unknown option '--version'\n",
			want:    fallbackVersion,
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

			if got := cli.version(t.Context()); got != tt.want {
				t.Errorf("version() = %q, want %q", got, tt.want)
			}
			if ran != tt.wantRun {
				t.Errorf("ran %q, want %q", ran, tt.wantRun)
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

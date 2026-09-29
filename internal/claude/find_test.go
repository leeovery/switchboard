package claude_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestFind(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing", "claude")
	directory := filepath.Join(dir, "directory", "claude")
	notRunnable := filepath.Join(dir, "not-runnable", "claude")
	first := filepath.Join(dir, "first", "claude")
	second := filepath.Join(dir, "second", "claude")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{notRunnable: 0o600, first: 0o700, second: 0o700} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, mode); err != nil {
			t.Fatal(err)
		}
	}
	const onPath = "/opt/tools/bin/claude"
	tests := []struct {
		name     string
		onPath   bool
		installs []string
		want     string
		wantErr  string
	}{
		{name: "on PATH, before any install path", onPath: true, installs: []string{first}, want: onPath},
		{name: "at the first install path holding a program, off PATH", installs: []string{missing, directory, notRunnable, first, second}, want: first},
		{
			name:     "nowhere",
			installs: []string{missing, directory, notRunnable},
			wantErr:  "can't find claude: it isn't on PATH, nor at " + missing + ", " + directory + ", " + notRunnable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookPath := func(file string) (string, error) {
				if !tt.onPath || file != "claude" {
					return "", exec.ErrNotFound
				}
				return onPath, nil
			}

			got, err := claude.Find(lookPath, tt.installs)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("Find() = %q, %v; want the error %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("Find() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestInstalledVersionAsksTheClaudeFindFinds(t *testing.T) {
	// stub writes a claude at path that says it's version.
	stub := func(path, version string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho '"+version+" (Claude Code)'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	stub(filepath.Join(home, ".local", "bin", "claude"), "2.1.301")
	onPath := filepath.Join(t.TempDir(), "claude")
	stub(onPath, "2.1.302")
	homeDir := func() (string, error) { return home, nil }
	tests := []struct {
		name     string
		lookPath func(string) (string, error)
		want     string
	}{
		{name: "on PATH", lookPath: func(string) (string, error) { return onPath, nil }, want: "2.1.302"},
		{name: "in the home given, off PATH", lookPath: func(string) (string, error) { return "", exec.ErrNotFound }, want: "2.1.301"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.InstalledVersion(tt.lookPath, homeDir)(); got != tt.want {
				t.Errorf("InstalledVersion()() = %q, want %q", got, tt.want)
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
			if got := claude.InstallPaths(tt.home); !slices.Equal(got, tt.want) {
				t.Errorf("InstallPaths(%q) = %q, want %q", tt.home, got, tt.want)
			}
		})
	}
}

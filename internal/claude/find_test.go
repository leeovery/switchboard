package claude_test

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
)

func TestFind(t *testing.T) {
	root := t.TempDir()
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	// switchboard, linked as Homebrew links it, and its claude link, which
	// leads to it through that link; and a longer chain to it.
	binary := claudetest.Program(t, at("Cellar", "switchboard", "bin", "switchboard"))
	brewLink := claudetest.Link(t, filepath.Join("..", "..", "Cellar", "switchboard", "bin", "switchboard"), at("brew", "bin", "switchboard"))
	claudeLink := claudetest.Link(t, brewLink, at("links", "claude"))
	claudetest.Link(t, claudeLink, at("chain", "claude"))
	hardLink := at("hard", "claude")
	if err := os.MkdirAll(filepath.Dir(hardLink), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(binary, hardLink); err != nil {
		t.Fatal(err)
	}
	// Claude Code, as a program, and linked to its version as its native
	// installer links it.
	claudeCode := claudetest.Program(t, at("real", "claude"))
	native := claudetest.Link(t, claudetest.Program(t, at("share", "claude", "versions", "2.1.300")), at("native", "claude"))
	// What isn't a program.
	missing := at("missing", "claude")
	directory := at("directory", "claude")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	notRunnable := at("not-runnable", "claude")
	if err := os.MkdirAll(filepath.Dir(notRunnable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notRunnable, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Relative directories on PATH lead here, where there's a claude.
	t.Chdir(at("real"))

	tests := []struct {
		name string
		// path lists the directories on PATH.
		path     []string
		installs []string
		// self is switchboard's own executable, as os.Executable gives it:
		// binary unless it's given.
		self    string
		selfErr error
		want    string
		wantErr string
	}{
		{name: "on PATH, before any install path", path: []string{at("real")}, installs: []string{native}, want: claudeCode},
		{name: "on PATH, by the link its installer made", path: []string{at("native")}, want: native},
		{
			name: "on PATH, past what isn't a program",
			path: []string{at("missing"), at("directory"), at("not-runnable"), at("real")},
			want: claudeCode,
		},
		{name: "on PATH, past switchboard's claude link ahead of it", path: []string{at("links"), at("real")}, want: claudeCode},
		{name: "on PATH, past a chain of links to switchboard", path: []string{at("chain"), at("real")}, want: claudeCode},
		{name: "on PATH, past a hard link to switchboard", path: []string{at("hard"), at("real")}, want: claudeCode},
		{
			name: "on PATH, past switchboard's claude link, switchboard reached through a link",
			path: []string{at("links"), at("real")},
			self: brewLink,
			want: claudeCode,
		},
		{
			name: "on PATH, past switchboard's claude link, switchboard run by it",
			path: []string{at("links"), at("real")},
			self: claudeLink,
			want: claudeCode,
		},
		{name: "past the relative directories on PATH", path: []string{"", "."}, installs: []string{native}, want: native},
		{
			name:     "at the first install path holding a program, off PATH",
			installs: []string{missing, directory, notRunnable, native, claudeCode},
			want:     native,
		},
		{name: "at an install path, past switchboard's claude link", installs: []string{claudeLink, native}, want: native},
		{
			name:     "nowhere, switchboard's own claude link never taken for it",
			path:     []string{at("links")},
			installs: []string{missing, claudeLink},
			wantErr:  "can't find claude past switchboard's own link: it isn't on PATH, nor at " + missing + ", " + claudeLink,
		},
		{
			name:    "without switchboard's own executable",
			path:    []string{at("real")},
			selfErr: errors.New("no path for the executable"),
			wantErr: "can't find claude: can't tell it from switchboard's own link: no path for the executable",
		},
		{
			name:    "with switchboard's own executable gone",
			path:    []string{at("real")},
			self:    missing,
			wantErr: "can't find claude: can't tell it from switchboard's own link: stat " + missing + ": no such file or directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executable := func() (string, error) { return cmp.Or(tt.self, binary), tt.selfErr }

			got, err := claude.Find(strings.Join(tt.path, string(filepath.ListSeparator)), tt.installs, executable)
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
	// stub writes a program at path that says it's version.
	stub := func(path, version string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho '"+version+" (Claude Code)'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	home := t.TempDir()
	stub(filepath.Join(home, ".local", "bin", "claude"), "2.1.301")
	onPath := filepath.Dir(stub(filepath.Join(t.TempDir(), "claude"), "2.1.302"))
	// switchboard, which would say it's a version no claude is, were it ever
	// asked, and its claude link.
	switchboard := stub(filepath.Join(t.TempDir(), "switchboard"), "9.9.9")
	links := filepath.Dir(claudetest.Link(t, switchboard, filepath.Join(t.TempDir(), "claude")))
	homeDir := func() (string, error) { return home, nil }
	executable := func() (string, error) { return switchboard, nil }
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "on PATH", path: onPath, want: "2.1.302"},
		{name: "on PATH, past switchboard's claude link", path: links + string(filepath.ListSeparator) + onPath, want: "2.1.302"},
		{name: "in the home given, off PATH", want: "2.1.301"},
		{name: "in the home given, past switchboard's claude link, the only one on PATH", path: links, want: "2.1.301"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == "PATH" {
					return tt.path
				}
				return ""
			}

			if got := claude.InstalledVersion(getenv, homeDir, executable)(); got != tt.want {
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

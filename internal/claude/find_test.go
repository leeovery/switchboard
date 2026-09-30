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
	// Another build of switchboard, as the test's own program is taken to be
	// one, and a link to that one.
	thisBuild, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	otherBuild := claudetest.Build(t, at("builds", "claude"))
	claudetest.Link(t, thisBuild, at("this-build", "claude"))
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
		{name: "on PATH, past another build of switchboard ahead of it", path: []string{at("builds"), at("real")}, self: thisBuild, want: claudeCode},
		{
			// Each build would otherwise start the other, which would start it,
			// and so on forever.
			name: "on PATH, past two builds of switchboard, each with a claude link, from one",
			path: []string{at("builds"), at("this-build"), at("real")},
			self: thisBuild,
			want: claudeCode,
		},
		{
			name: "on PATH, past two builds of switchboard, each with a claude link, from the other",
			path: []string{at("this-build"), at("builds"), at("real")},
			self: otherBuild,
			want: claudeCode,
		},
		{name: "at an install path, past another build of switchboard", installs: []string{otherBuild, native}, self: thisBuild, want: native},
		{
			name:    "nowhere, another build of switchboard never taken for it",
			path:    []string{at("builds")},
			self:    thisBuild,
			wantErr: "can't find claude past another switchboard, at " + otherBuild + ": it isn't on PATH, nor at ",
		},
		{
			name: "on PATH, a Go program, when switchboard isn't one to compare it with",
			path: []string{at("builds"), at("real")},
			want: otherBuild,
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

func TestFindAfter(t *testing.T) {
	root := t.TempDir()
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	switchboard := claudetest.Program(t, at("switchboard"))
	claudetest.Link(t, switchboard, at("links", "claude"))
	// A wrapper named claude that runs switchboard, which starts it again.
	wrapper := claudetest.Program(t, at("wrapper", "claude"))
	claudeCode := claudetest.Program(t, at("real", "claude"))
	native := claudetest.Program(t, at("native", "claude"))
	tests := []struct {
		name string
		// after is the claude that led back to switchboard.
		after string
		// path lists the directories on PATH.
		path     []string
		installs []string
		want     string
		wantErr  string
	}{
		{name: "nothing past, as Find", path: []string{at("wrapper"), at("real")}, want: wrapper},
		{name: "on PATH, past it", after: wrapper, path: []string{at("wrapper"), at("real")}, want: claudeCode},
		{name: "at an install path, past it on PATH", after: wrapper, path: []string{at("wrapper")}, installs: []string{native}, want: native},
		{
			name:  "on PATH, past it, its directory twice on PATH",
			after: wrapper,
			path:  []string{at("wrapper"), at("wrapper"), at("real")},
			want:  claudeCode,
		},
		{
			name:     "on PATH, past it, as it's an install path too",
			after:    wrapper,
			path:     []string{at("wrapper"), at("real")},
			installs: []string{native, wrapper},
			want:     claudeCode,
		},
		{name: "past it at an install path", after: wrapper, installs: []string{wrapper, native}, want: native},
		{
			name:  "everywhere, when it's nowhere to look, as when PATH has changed",
			after: wrapper,
			path:  []string{at("real"), at("native")},
			want:  claudeCode,
		},
		{
			name:     "nowhere, nothing past it",
			after:    wrapper,
			path:     []string{at("real"), at("wrapper")},
			installs: []string{at("missing", "claude")},
			wantErr:  "can't find claude past " + wrapper + ", which leads back to switchboard: it isn't on PATH, nor at " + at("missing", "claude"),
		},
		{
			name:    "nowhere, past it and switchboard's own link",
			after:   wrapper,
			path:    []string{at("wrapper"), at("links")},
			wantErr: "can't find claude past " + wrapper + ", which leads back to switchboard: it isn't on PATH, nor at ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executable := func() (string, error) { return switchboard, nil }

			got, err := claude.FindAfter(tt.after, strings.Join(tt.path, string(filepath.ListSeparator)), tt.installs, executable)
			if got != tt.want || errorText(err) != tt.wantErr {
				t.Errorf("FindAfter() = %q, %v; want %q, and the error %q", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestThroughSwitchboard(t *testing.T) {
	root := t.TempDir()
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	// switchboard, run by the link Homebrew makes, and its claude link.
	claudetest.Program(t, at("Cellar", "switchboard", "bin", "switchboard"))
	brewLink := claudetest.Link(t, filepath.Join("..", "..", "Cellar", "switchboard", "bin", "switchboard"), at("brew", "bin", "switchboard"))
	claudetest.Link(t, brewLink, at("links", "claude"))
	claudetest.Program(t, at("real", "claude"))
	claudetest.Link(t, at("gone", "switchboard"), at("dangling", "claude"))
	if err := os.MkdirAll(at("directory", "claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(at("not-runnable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(at("not-runnable", "claude"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Another build of switchboard, as the test's own program is taken to be
	// one.
	thisBuild, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	claudetest.Build(t, at("builds", "claude"))
	tests := []struct {
		name string
		// path lists the directories on PATH.
		path []string
		// self is switchboard's own executable, as os.Executable gives it:
		// its Homebrew link unless it's given.
		self    string
		selfErr error
		want    bool
		wantErr string
	}{
		{name: "switchboard's claude link first", path: []string{at("links"), at("real")}, want: true},
		{name: "another build of switchboard first", path: []string{at("builds"), at("real")}, self: thisBuild, want: true},
		{
			name: "past what isn't a program, as a shell passes it",
			path: []string{at("dangling"), at("directory"), at("not-runnable"), at("links"), at("real")},
			want: true,
		},
		{name: "Claude Code first", path: []string{at("real"), at("links")}},
		{name: "no claude on PATH", path: []string{at("brew", "bin")}},
		{name: "past relative directories, as claude.Find passes them", path: []string{".", at("real")}},
		{
			name:    "without switchboard's own executable, no telling",
			path:    []string{at("links")},
			selfErr: errors.New("no path for the executable"),
			wantErr: "no path for the executable",
		},
	}
	// A relative directory on PATH leads here, where switchboard's link is.
	t.Chdir(at("links"))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executable := func() (string, error) { return cmp.Or(tt.self, brewLink), tt.selfErr }

			got, err := claude.ThroughSwitchboard(strings.Join(tt.path, string(filepath.ListSeparator)), executable)
			if got != tt.want || errorText(err) != tt.wantErr {
				t.Errorf("ThroughSwitchboard() = %v, %v; want %v, and the error %q", got, err, tt.want, tt.wantErr)
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

			if got := claude.InstalledVersionOfStandIn(getenv, homeDir, executable)(); got != tt.want {
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

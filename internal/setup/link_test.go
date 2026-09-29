package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
)

func TestTheClaudeLinkStep(t *testing.T) {
	const bin = "<root>/home/.local/share/switchboard/bin"
	const linked = "Linked " + bin + "/claude to <root>/brew/bin/switchboard.\n"
	const through = "claude on PATH goes through switchboard, from " + bin + ".\n"
	// addLine is what's said to put switchboard's bin directory on PATH:
	// why, and the line to add, the directory as it's written there.
	addLine := func(why, written string) string {
		return "claude doesn't go through switchboard yet: " + why + ". Add this line to your shell's startup file, such as ~/.zshrc, " +
			"after anything else there that changes PATH, so it stays ahead of <root>/claude-code/bin:\n" +
			"export PATH=\"" + written + ":$PATH\"\n" +
			"Running setup again, in a new terminal, checks it.\n"
	}
	notOnPath := addLine(bin+" isn't on PATH", "$HOME/.local/share/switchboard/bin")
	const leftAlone = bin + "/claude is there already, and isn't a link, so it's left alone, and claude isn't linked: move it aside, then run setup again.\n"
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's claude
		// link and its bin directory's being on PATH, before setup runs.
		lay  func(t *testing.T, w *world)
		want string
		// wantLinked is set when setup leaves the link leading to switchboard:
		// every other part of the world is as it was.
		wantLinked bool
	}{
		{
			name:       "made, its directory on PATH ahead of claude",
			lay:        func(_ *testing.T, w *world) { w.putBinOnPath() },
			want:       linked + through,
			wantLinked: true,
		},
		{
			name:       "made, its directory not on PATH: the line that puts it there",
			want:       linked + notOnPath,
			wantLinked: true,
		},
		{
			name:       "made, its directory on PATH behind claude: the line that puts it ahead",
			lay:        func(_ *testing.T, w *world) { w.path = append(w.path, w.bin) },
			want:       linked + addLine(bin+" is on PATH, but after <root>/claude-code/bin, where Claude Code is", "$HOME/.local/share/switchboard/bin"),
			wantLinked: true,
		},
		{
			name: "there already, its directory ahead of claude: nothing written",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude"))
				w.putBinOnPath()
			},
			want:       bin + "/claude leads to switchboard, at <root>/brew/bin/switchboard.\n" + through,
			wantLinked: true,
		},
		{
			name: "there already, its directory ahead of claude by a link to it",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude"))
				w.path = append([]string{claudetest.Link(t, w.bin, filepath.Join(w.root, "linked-bin"))}, w.path...)
			},
			want:       bin + "/claude leads to switchboard, at <root>/brew/bin/switchboard.\n" + through,
			wantLinked: true,
		},
		{
			name: "repaired, leading to another switchboard",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "go", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
				w.putBinOnPath()
			},
			want:       bin + "/claude led to <root>/go/bin/switchboard: it leads to switchboard, at <root>/brew/bin/switchboard, now.\n" + through,
			wantLinked: true,
		},
		{
			name: "repaired, leading to this switchboard's version rather than the path it was run by",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "Cellar", "switchboard", "1.2.3", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
				w.putBinOnPath()
			},
			want:       bin + "/claude led to <root>/Cellar/switchboard/1.2.3/bin/switchboard: it leads to switchboard, at <root>/brew/bin/switchboard, now.\n" + through,
			wantLinked: true,
		},
		{
			name: "not, a file in its place, left alone",
			lay:  func(t *testing.T, w *world) { writeFile(t, filepath.Join(w.bin, "claude"), "#!/bin/sh\n", 0o700) },
			want: leftAlone,
		},
		{
			name: "not, a directory in its place, left alone",
			lay:  func(t *testing.T, w *world) { mkdir(t, filepath.Join(w.bin, "claude")) },
			want: leftAlone,
		},
		{
			name: "made, a relative directory on PATH naming its directory counting for nothing, as claude.Find passes one over",
			lay: func(t *testing.T, w *world) {
				t.Chdir(filepath.Dir(w.bin))
				w.path = append([]string{"bin"}, w.path...)
			},
			want:       linked + notOnPath,
			wantLinked: true,
		},
		{
			name: "made, where switchboard's link elsewhere on PATH ahead of claude counts for nothing",
			lay: func(t *testing.T, w *world) {
				elsewhere := filepath.Join(w.root, "elsewhere")
				claudetest.Link(t, w.switchboard, filepath.Join(elsewhere, "claude"))
				w.path = append([]string{elsewhere}, w.path...)
			},
			want:       linked + notOnPath,
			wantLinked: true,
		},
		{
			name: "made, its directory on PATH and claude off it",
			lay: func(_ *testing.T, w *world) {
				w.path = []string{w.bin}
				w.installPaths = []string{w.realClaude}
			},
			want:       linked + through,
			wantLinked: true,
		},
		{
			name: "made, without a claude",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.realClaude); err != nil {
					t.Fatal(err)
				}
				w.installPaths = []string{filepath.Join(w.home, ".local", "bin", "claude")}
			},
			want: linked + "There's no Claude Code for claude to start: can't find claude: it isn't on PATH, nor at <root>/home/.local/bin/claude. " +
				"Install it, then run setup again.\n",
			wantLinked: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if err := os.Remove(filepath.Join(w.bin, "claude")); err != nil {
				t.Fatal(err)
			}
			w.path = []string{filepath.Dir(w.realClaude)}
			if tt.lay != nil {
				tt.lay(t, w)
			}
			before := w.snapshot(t)

			shown := w.runs(t, "")
			if got := section(t, shown, "4. The claude link"); got != tt.want {
				t.Errorf("the claude link step showed\n%s\nwant\n%s", got, tt.want)
			}
			if tt.wantLinked {
				checkLinked(t, w)
				before[filepath.Join(w.bin, "claude")] = "a link to " + w.switchboard
			}
			w.checkUnchanged(t, before)
		})
	}
}

func TestTheLineThatPutsTheBinDirectoryOnPath(t *testing.T) {
	tests := []struct {
		name string
		// bin is switchboard's bin directory, from the world's root.
		bin  string
		want string
	}{
		{name: "in the home, written from $HOME", bin: "home/.local/share/switchboard/bin", want: `export PATH="$HOME/.local/share/switchboard/bin:$PATH"`},
		{name: "outside the home, written whole", bin: "data/switchboard/bin", want: `export PATH="<root>/data/switchboard/bin:$PATH"`},
		{name: "in the home, what a shell reads specially escaped", bin: `home/it's "$x"/switchboard/bin`, want: `export PATH="$HOME/it's \"\$x\"/switchboard/bin:$PATH"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.bin = filepath.Join(w.root, filepath.FromSlash(tt.bin))
			w.path = []string{filepath.Dir(w.realClaude)}

			shown := w.runs(t, "")
			if !strings.Contains(section(t, shown, "4. The claude link"), "\n"+tt.want+"\n") {
				t.Errorf("the claude link step showed\n%s\nwant it to hold the line\n%s", section(t, shown, "4. The claude link"), tt.want)
			}
			checkLinked(t, w)
		})
	}
}

func TestSetupStopsWhenItCantMakeTheBinDirectory(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.RemoveAll(w.bin); err != nil {
		t.Fatal(err)
	}
	lockedDir(t, filepath.Dir(w.bin))

	shown, err := w.run(t, "")
	if err == nil || !strings.HasPrefix(err.Error(), "make switchboard's bin directory: ") {
		t.Errorf("Run() error = %v, want it to say it couldn't make switchboard's bin directory", err)
	}
	if strings.Contains(shown, "5. The skill") {
		t.Errorf("the terminal showed\n%s\nwant setup stopped at the claude link", shown)
	}
}

// checkLinked checks switchboard's claude link leads to switchboard by the
// path it was run by, which an upgrade moves on.
func checkLinked(t *testing.T, w *world) {
	t.Helper()
	link := filepath.Join(w.bin, "claude")
	if target, err := os.Readlink(link); err != nil || target != w.switchboard {
		t.Errorf("%s leads to %q (%v), want %s, the path switchboard was run by", link, target, err, w.switchboard)
	}
}

// lockedDir makes dir a directory the user can't write to, and restores it
// once the test ends, so its removal can take what's in it.
func lockedDir(t *testing.T, dir string) {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("root can write to any directory")
	}
	mkdir(t, dir)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

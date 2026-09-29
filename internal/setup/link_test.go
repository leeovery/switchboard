package setup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
)

func TestTheClaudeLinkStep(t *testing.T) {
	const question = "Link <root>/bin/claude to switchboard, ahead of <root>/claude-code/bin/claude on PATH, so every claude goes through switchboard? [Y/n] "
	const linked = "Linked <root>/bin/claude to <root>/brew/bin/switchboard: every claude goes through switchboard.\n"
	const leftAlone = "<root>/bin/claude is there already, and isn't switchboard, so it's left alone.\n" +
		"There's nowhere else to put the link: move aside what's named claude, or add another directory of yours to PATH ahead of <root>/claude-code/bin, then run setup again.\n"
	const nowhere = "There's nowhere to put the link: no directory of yours on PATH ahead of <root>/claude-code/bin, where claude is, can be written to. " +
		"Add one to PATH ahead of it, then run setup again.\n"
	const notLinked = "Not linked: claude starts Claude Code without switchboard. Run setup again to link it.\n"
	tests := []struct {
		name string
		// lay changes the world, which is done but for the claude link,
		// before setup runs.
		lay     func(t *testing.T, w *world)
		answers []string
		want    string
		// wantLinks are the directories setup leaves a claude link to
		// switchboard in: every other part of the world is as it was.
		wantLinks []string
	}{
		{
			name:      "in the first directory on PATH, ahead of claude, that the user can write to",
			answers:   []string{"", ""},
			want:      question + "\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name:      "once the user says yes, asked again until they say yes or no",
			answers:   []string{"", "maybe", "y"},
			want:      question + "maybe\nAnswer y or n.\n" + question + "y\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name:    "not, the user saying no",
			answers: []string{"", "n"},
			want:    question + "n\n" + notLinked,
		},
		{
			name: "in the next directory the user can write to, the user saying no to the first",
			lay: func(t *testing.T, w *world) {
				first := filepath.Join(w.root, "first")
				mkdir(t, first)
				w.path = append([]string{first}, w.path...)
			},
			answers: []string{"", "n", ""},
			want: "Link <root>/first/claude to switchboard, ahead of <root>/claude-code/bin/claude on PATH, so every claude goes through switchboard? [Y/n] n\n" +
				question + "\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name:    "offered once in a directory, however many times PATH names it",
			lay:     func(_ *testing.T, w *world) { w.path = append([]string{w.bin + "/"}, w.path...) },
			answers: []string{"", "n"},
			want:    question + "n\n" + notLinked,
		},
		{
			name: "past directories the user can't write to, relative ones, and what isn't a directory",
			lay: func(t *testing.T, w *world) {
				locked := filepath.Join(w.root, "locked")
				lockedDir(t, locked)
				mkdir(t, filepath.Join(w.root, "relative"))
				t.Chdir(w.root)
				file := filepath.Join(w.root, "file")
				writeFile(t, file, "", 0o600)
				w.path = append([]string{"relative", locked, filepath.Join(w.root, "missing"), file}, w.path...)
			},
			answers:   []string{"", ""},
			want:      question + "\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name: "past a directory where something else is named claude, which is left alone",
			lay: func(t *testing.T, w *world) {
				first := filepath.Join(w.root, "first")
				claudetest.Link(t, filepath.Join(w.root, "gone", "claude"), filepath.Join(first, "claude"))
				w.path = append([]string{first}, w.path...)
			},
			answers:   []string{"", ""},
			want:      "<root>/first/claude is there already, and isn't switchboard, so it's left alone.\n" + question + "\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name:      "not again, switchboard's claude link there already",
			lay:       func(t *testing.T, w *world) { claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude")) },
			answers:   []string{""},
			want:      "claude goes through switchboard already: <root>/bin/claude leads to it, ahead of <root>/claude-code/bin/claude.\n",
			wantLinks: []string{"bin"},
		},
		{
			name: "not again, switchboard's claude link further along PATH than a directory the user can write to, but ahead of claude",
			lay: func(t *testing.T, w *world) {
				ahead := filepath.Join(w.root, "ahead")
				mkdir(t, ahead)
				claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude"))
				w.path = append([]string{ahead}, w.path...)
			},
			answers:   []string{""},
			want:      "claude goes through switchboard already: <root>/bin/claude leads to it, ahead of <root>/claude-code/bin/claude.\n",
			wantLinks: []string{"bin"},
		},
		{
			name: "not again, switchboard there by a hard link, as by a copy of it",
			lay: func(t *testing.T, w *world) {
				target, err := filepath.EvalSymlinks(w.switchboard)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, filepath.Join(w.bin, "claude")); err != nil {
					t.Fatal(err)
				}
			},
			answers: []string{""},
			want:    "claude goes through switchboard already: <root>/bin/claude leads to it, ahead of <root>/claude-code/bin/claude.\n",
		},
		{
			name: "ahead of claude, switchboard's claude link behind it counting for nothing",
			lay: func(t *testing.T, w *world) {
				behind := filepath.Join(w.root, "behind")
				claudetest.Link(t, w.switchboard, filepath.Join(behind, "claude"))
				w.path = append(w.path, behind)
			},
			answers:   []string{"", ""},
			want:      question + "\n" + linked,
			wantLinks: []string{"behind", "bin"},
		},
		{
			name: "not, a link named claude leading nowhere there, left alone",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "gone", "switchboard"), filepath.Join(w.bin, "claude"))
			},
			answers: []string{""},
			want:    leftAlone,
		},
		{
			name:    "not, a file named claude there that can't be run, left alone",
			lay:     func(t *testing.T, w *world) { writeFile(t, filepath.Join(w.bin, "claude"), "notes\n", 0o600) },
			answers: []string{""},
			want:    leftAlone,
		},
		{
			name:    "not, a directory named claude there, left alone",
			lay:     func(t *testing.T, w *world) { mkdir(t, filepath.Join(w.bin, "claude")) },
			answers: []string{""},
			want:    leftAlone,
		},
		{
			name:    "nowhere, no directory ahead of claude the user can write to",
			lay:     func(t *testing.T, w *world) { lockedDir(t, w.bin) },
			answers: []string{""},
			want:    nowhere,
		},
		{
			name: "past Homebrew's kegs, reached through their opt links, and its casks",
			lay: func(t *testing.T, w *world) {
				homebrew := filepath.Join(w.root, "homebrew")
				cask := filepath.Join(homebrew, "Caskroom", "tool", "1.0", "bin")
				mkdir(t, cask)
				w.path = append([]string{keg(t, homebrew, "coreutils"), cask}, w.path...)
			},
			answers:   []string{"", ""},
			want:      question + "\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name: "past an app bundle, whose signature a link would break",
			lay: func(t *testing.T, w *world) {
				macOS := filepath.Join(w.root, "Applications", "Terminal Emulator.app", "Contents", "MacOS")
				mkdir(t, macOS)
				w.path = append([]string{macOS}, w.path...)
			},
			answers:   []string{"", ""},
			want:      question + "\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name: "past package managers' trees, and npm's global bin",
			lay: func(t *testing.T, w *world) {
				vendor := filepath.Join(w.home, ".composer", "vendor", "bin")
				modules := filepath.Join(w.root, "project", "node_modules", ".bin")
				mkdir(t, vendor)
				mkdir(t, modules)
				w.path = append([]string{vendor, modules, npmGlobal(t, filepath.Join(w.home, ".npm-global"))}, w.path...)
			},
			answers:   []string{"", ""},
			want:      question + "\n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name: "nowhere, but for directories other programs keep",
			lay: func(t *testing.T, w *world) {
				lockedDir(t, w.bin)
				macOS := filepath.Join(w.root, "Applications", "Tool.app", "Contents", "MacOS")
				mkdir(t, macOS)
				w.path = append([]string{keg(t, filepath.Join(w.root, "homebrew"), "coreutils"), macOS}, w.path...)
			},
			answers: []string{""},
			want:    nowhere,
		},
		{
			name: "in Homebrew's bin first, then the user's own, on a PATH laid out as a Homebrew user's",
			lay: func(t *testing.T, w *world) {
				homebrew := filepath.Join(w.root, "homebrew")
				gnubin := keg(t, homebrew, "coreutils")
				// Homebrew's own prefix, where npm installs too, unless told
				// elsewhere, as it's told here.
				mkdir(t, filepath.Join(homebrew, "lib", "node_modules"))
				mkdir(t, filepath.Join(homebrew, "bin"))
				vendor := filepath.Join(w.home, ".composer", "vendor", "bin")
				mkdir(t, vendor)
				dotfiles := filepath.Join(w.root, "dotfiles", "bin")
				mkdir(t, dotfiles)
				w.path = []string{
					gnubin, npmGlobal(t, filepath.Join(w.home, ".npm-global")), vendor,
					filepath.Join(homebrew, "bin"), dotfiles, filepath.Dir(w.realClaude),
				}
			},
			answers: []string{"", "n", ""},
			want: "Link <root>/homebrew/bin/claude to switchboard, ahead of <root>/claude-code/bin/claude on PATH, so every claude goes through switchboard? [Y/n] n\n" +
				"Link <root>/dotfiles/bin/claude to switchboard, ahead of <root>/claude-code/bin/claude on PATH, so every claude goes through switchboard? [Y/n] \n" +
				"Linked <root>/dotfiles/bin/claude to <root>/brew/bin/switchboard: every claude goes through switchboard.\n",
			wantLinks: []string{"dotfiles/bin"},
		},
		{
			name: "in the first directory on PATH the user can write to, claude found off PATH",
			lay: func(t *testing.T, w *world) {
				w.path = []string{w.bin}
				w.installPaths = []string{filepath.Join(w.home, ".local", "bin", "claude"), w.realClaude}
			},
			answers:   []string{"", ""},
			want:      "Link <root>/bin/claude to switchboard, so every claude goes through switchboard? [Y/n] \n" + linked,
			wantLinks: []string{"bin"},
		},
		{
			name: "nowhere, claude found off PATH, and no directory on PATH the user can write to",
			lay: func(t *testing.T, w *world) {
				lockedDir(t, w.bin)
				w.path = []string{w.bin}
				w.installPaths = []string{w.realClaude}
			},
			answers: []string{""},
			want: "There's nowhere to put the link: claude is at <root>/claude-code/bin/claude, off PATH, and no directory of yours on PATH can be written to. " +
				"Add one to PATH, then run setup again.\n",
		},
		{
			name: "nowhere, without a claude",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.realClaude); err != nil {
					t.Fatal(err)
				}
				w.installPaths = []string{filepath.Join(w.home, ".local", "bin", "claude")}
			},
			answers: []string{""},
			want: "There's no claude to go ahead of: can't find claude: it isn't on PATH, nor at <root>/home/.local/bin/claude. " +
				"Install Claude Code, then run setup again.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if err := os.Remove(filepath.Join(w.bin, "claude")); err != nil {
				t.Fatal(err)
			}
			if tt.lay != nil {
				tt.lay(t, w)
			}
			before := w.snapshot(t)

			shown := w.runs(t, tt.answers...)
			if got := section(t, shown, "4. The claude link"); got != tt.want {
				t.Errorf("the claude link step showed\n%s\nwant\n%s", got, tt.want)
			}
			for _, dir := range tt.wantLinks {
				link := filepath.Join(w.root, dir, "claude")
				if target, err := os.Readlink(link); err != nil || target != w.switchboard {
					t.Errorf("%s leads to %q (%v), want %s, the path switchboard was run by", link, target, err, w.switchboard)
				}
				before[link] = "a link to " + w.switchboard
			}
			w.checkUnchanged(t, before)
		})
	}
}

// keg makes a Homebrew keg of tool's in the Homebrew prefix given, with a
// bin, and the opt link Homebrew reaches it through, and returns the bin as
// PATH reaches it, through that link.
func keg(t *testing.T, prefix, tool string) string {
	t.Helper()
	mkdir(t, filepath.Join(prefix, "Cellar", tool, "1.0", "bin"))
	claudetest.Link(t, filepath.Join("..", "Cellar", tool, "1.0"), filepath.Join(prefix, "opt", tool))
	return filepath.Join(prefix, "opt", tool, "bin")
}

// npmGlobal makes npm's global prefix, as npm's config can name it, with its
// bin beside lib/node_modules, and returns the bin.
func npmGlobal(t *testing.T, prefix string) string {
	t.Helper()
	mkdir(t, filepath.Join(prefix, "lib", "node_modules"))
	mkdir(t, filepath.Join(prefix, "bin"))
	return filepath.Join(prefix, "bin")
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

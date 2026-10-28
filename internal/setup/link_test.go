package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/shell"
)

// What the claude link step says of switchboard's files for the shells, and
// of the login shell running the link, in a world that's done.
const (
	filesCurrent = "switchboard's files for the shells are up to date, in ~/.local/share/switchboard/shell.\n"
	goesThrough  = "claude goes through switchboard in a new terminal: zsh runs ~/.local/share/switchboard/bin/claude.\n"
)

func TestTheClaudeLinkStep(t *testing.T) {
	const bin = "<root>/home/.local/share/switchboard/bin"
	const linked = "Linked " + bin + "/claude to <root>/brew/bin/switchboard.\n"
	const leftAlone = bin + "/claude is there already, and isn't a link, so it's left alone, and claude isn't linked: move it aside, then run setup again.\n"
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's claude
		// link, before setup runs.
		lay  func(t *testing.T, w *world)
		want string
		// wantLinked is set when setup leaves the link leading to switchboard:
		// every other part of the world is as it was.
		wantLinked bool
	}{
		{
			name:       "made",
			want:       linked + filesCurrent + goesThrough,
			wantLinked: true,
		},
		{
			name:       "there already: nothing written",
			lay:        func(t *testing.T, w *world) { claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude")) },
			want:       bin + "/claude leads to switchboard, at <root>/brew/bin/switchboard.\n" + filesCurrent + goesThrough,
			wantLinked: true,
		},
		{
			name: "repaired, leading to another switchboard",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "go", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
			},
			want:       bin + "/claude led to <root>/go/bin/switchboard: it leads to switchboard, at <root>/brew/bin/switchboard, now.\n" + filesCurrent + goesThrough,
			wantLinked: true,
		},
		{
			name: "repaired, leading to this switchboard's version rather than the path it was run by",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "Cellar", "switchboard", "1.2.3", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
			},
			want:       bin + "/claude led to <root>/Cellar/switchboard/1.2.3/bin/switchboard: it leads to switchboard, at <root>/brew/bin/switchboard, now.\n" + filesCurrent + goesThrough,
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
			name: "made, claude off PATH, where its installer puts it",
			lay: func(_ *testing.T, w *world) {
				w.path = nil
				w.installPaths = []string{w.realClaude}
			},
			want:       linked + filesCurrent + goesThrough,
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

func TestTheClaudeLinkStepWritesTheShellFiles(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay  func(t *testing.T, w *world)
		want string
	}{
		{name: "each this switchboard's already", want: filesCurrent},
		{
			name: "one another switchboard's: brought up to date",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.shellDir, "init.zsh"), "# an older switchboard's\n", 0o644)
			},
			want: "Brought switchboard's files for the shells up to date, in ~/.local/share/switchboard/shell: init.zsh.\n",
		},
		{
			name: "two not there: written",
			lay: func(t *testing.T, w *world) {
				for _, name := range []string{"init.bash", "init.fish"} {
					if err := os.Remove(filepath.Join(w.shellDir, name)); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: "Wrote switchboard's files for the shells, in ~/.local/share/switchboard/shell: init.bash and init.fish.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}

			shown := w.runs(t, "")
			want := "<root>/home/.local/share/switchboard/bin/claude leads to switchboard, at <root>/brew/bin/switchboard.\n" + tt.want + goesThrough
			if got := section(t, shown, "4. The claude link"); got != want {
				t.Errorf("the claude link step showed\n%s\nwant\n%s", got, want)
			}
			w.checkShellFiles(t)
		})
	}
}

func TestTheClaudeLinkStepHasTheLoginShellRunTheLink(t *testing.T) {
	const otherClaude = "another claude, <root>/claude-code/bin/claude"
	const notYet = "claude doesn't go through switchboard in a new terminal yet: zsh runs " + otherClaude + "."
	const zshLine = `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`
	const offer = " This line, at the end of ~/.zshrc, has claude go through switchboard: it reads switchboard's file for zsh, which puts ~/.local/share/switchboard/bin first on PATH, and loads switchboard's completions.\n" +
		zshLine + "\nAdd it? [Y/n] "
	byHand := func(line string) string {
		return "To have claude go through switchboard by hand, add this line to the end of your shell's startup file, after anything there that changes PATH:\n" + line + "\n" +
			"Or, where you keep your PATH another way, put this directory first on it:\n~/.local/share/switchboard/bin\n"
	}
	const left = "Check again? [Y/n] n\nLeft as it is: until it's done, claude in a new terminal doesn't go through switchboard. Running setup again checks it.\n"
	other := shell.Answer{Kind: shell.Other, Claude: "<root>/claude-code/bin/claude"}
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's line in
		// .zshrc, before setup runs.
		lay func(t *testing.T, w *world)
		// answers are the claude link step's.
		answers []string
		want    string
		// wantFile is the startup file, from the home, and wantText what it
		// reads, <line> standing for switchboard's line.
		wantFile, wantText string
	}{
		{
			name:     "offered, and added: it runs the link",
			answers:  []string{""},
			want:     notYet + offer + "\nAdded it to ~/.zshrc.\n" + goesThrough,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name:     "offered, and added to the end of a .zshrc without a final newline",
			lay:      func(t *testing.T, w *world) { writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi", 0o644) },
			answers:  []string{"y"},
			want:     notYet + offer + "y\nAdded it to ~/.zshrc.\n" + goesThrough,
			wantFile: ".zshrc", wantText: "export EDITOR=vi\n<line>\n",
		},
		{
			name: "offered, and added through a .zshrc kept with the user's dotfiles",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.root, "dotfiles", "zshrc"), "export EDITOR=vi\n", 0o644)
				claudetest.Link(t, filepath.Join(w.root, "dotfiles", "zshrc"), filepath.Join(w.home, ".zshrc"))
			},
			answers:  []string{""},
			want:     notYet + offer + "\nAdded it to ~/.zshrc.\n" + goesThrough,
			wantFile: ".zshrc", wantText: "export EDITOR=vi\n<line>\n",
		},
		{
			name:     "offered, in $ZDOTDIR",
			lay:      func(_ *testing.T, w *world) { w.env["ZDOTDIR"] = filepath.Join(w.home, ".config", "zsh") },
			answers:  []string{""},
			want:     notYet + strings.ReplaceAll(offer, "~/.zshrc", "~/.config/zsh/.zshrc") + "\nAdded it to ~/.config/zsh/.zshrc.\n" + goesThrough,
			wantFile: ".config/zsh/.zshrc", wantText: "<line>\n",
		},
		{
			name: "bash, offered for the .profile it reads, never a .bash_profile beside it",
			lay: func(t *testing.T, w *world) {
				w.env["SHELL"] = "/opt/homebrew/bin/bash"
				writeFile(t, filepath.Join(w.home, ".profile"), "export EDITOR=vi\n", 0o644)
			},
			answers: []string{""},
			want: "claude doesn't go through switchboard in a new terminal yet: bash runs " + otherClaude + "." +
				" This line, at the end of ~/.profile, has claude go through switchboard: it reads switchboard's file for bash, which puts ~/.local/share/switchboard/bin first on PATH, and loads switchboard's completions.\n" +
				strings.ReplaceAll(zshLine, "init.zsh", "init.bash") + "\nAdd it? [Y/n] \nAdded it to ~/.profile.\n" +
				"claude goes through switchboard in a new terminal: bash runs ~/.local/share/switchboard/bin/claude.\n",
			wantFile: ".profile", wantText: "export EDITOR=vi\n<line>\n",
		},
		{
			name: "offered, declined: by hand, asked again until it runs the link",
			lay: func(_ *testing.T, w *world) {
				w.login.answers = []shell.Answer{other, {Kind: shell.None}, {Kind: shell.Link}}
			},
			answers: []string{"n", "", ""},
			want: notYet + offer + "n\n" + byHand(zshLine) +
				"Check again? [Y/n] \nclaude doesn't go through switchboard in a new terminal yet: zsh finds no claude.\n" +
				"Check again? [Y/n] \n" + goesThrough,
			wantFile: ".zshrc",
		},
		{
			name:     "offered, declined, and left: setup goes on",
			answers:  []string{"n", "n"},
			want:     notYet + offer + "n\n" + byHand(zshLine) + left,
			wantFile: ".zshrc",
		},
		{
			name:     "added, but not taking effect: something after it changes PATH",
			lay:      func(_ *testing.T, w *world) { w.login.overridden = true },
			answers:  []string{"", "n"},
			want:     notYet + offer + "\nAdded it to ~/.zshrc.\nswitchboard's line is in ~/.zshrc, but zsh runs " + otherClaude + ", so something after the line changes PATH.\n" + byHand(zshLine) + left,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, but not taking effect: something after it changes PATH",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.overridden = true
			},
			answers:  []string{"n"},
			want:     "switchboard's line is in ~/.zshrc, but zsh runs " + otherClaude + ", so something after the line changes PATH.\n" + byHand(zshLine) + left,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, the shell not answering at first",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.answers = []shell.Answer{{Why: "it didn't answer within 15s"}}
			},
			answers: []string{""},
			want: "switchboard's line is in ~/.zshrc, but setup can't ask zsh whether it takes effect: it didn't answer within 15s.\n" +
				byHand(zshLine) + "Check again? [Y/n] \n" + goesThrough,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "can't be added, as .zshrc is a link that leads nowhere: by hand",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "gone"), filepath.Join(w.home, ".zshrc"))
			},
			answers: []string{"", "n"},
			want:    notYet + offer + "\nSetup can't add it: <root>/home/.zshrc is a link to <root>/gone, which isn't there.\n" + byHand(zshLine) + left,
		},
		{
			name:    "another shell: the directory alone, by hand",
			lay:     func(_ *testing.T, w *world) { w.env["SHELL"] = "/bin/tcsh" },
			answers: []string{"n"},
			want: "claude doesn't go through switchboard in a new terminal yet: tcsh runs " + otherClaude + ".\n" +
				"To have claude go through switchboard, put this directory first on PATH, wherever your shell sets it:\n~/.local/share/switchboard/bin\n" + left,
		},
		{
			name: "no $SHELL: the directory alone, by hand",
			lay: func(_ *testing.T, w *world) {
				delete(w.env, "SHELL")
				w.login.answers = []shell.Answer{{Why: "$SHELL isn't set"}}
			},
			answers: []string{"n"},
			want: "Setup can't ask your login shell which claude it runs: $SHELL isn't set.\n" +
				"To have claude go through switchboard, put this directory first on PATH, wherever your shell sets it:\n~/.local/share/switchboard/bin\n" + left,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if err := os.Remove(filepath.Join(w.home, ".zshrc")); err != nil {
				t.Fatal(err)
			}
			if tt.lay != nil {
				tt.lay(t, w)
			}

			shown := w.runs(t, append([]string{""}, tt.answers...)...) // no more accounts, then the step's answers
			want := "<root>/home/.local/share/switchboard/bin/claude leads to switchboard, at <root>/brew/bin/switchboard.\n" + filesCurrent + tt.want
			if got := section(t, shown, "4. The claude link"); got != want {
				t.Errorf("the claude link step showed\n%s\nwant\n%s", got, want)
			}
			if tt.wantFile != "" {
				file := filepath.Join(w.home, filepath.FromSlash(tt.wantFile))
				text, _ := os.ReadFile(file)
				if want := strings.ReplaceAll(tt.wantText, "<line>", w.line()); string(text) != want {
					t.Errorf("%s reads %q, want %q", file, text, want)
				}
			}
			for _, link := range w.login.asked {
				if link != filepath.Join(w.bin, "claude") {
					t.Errorf("the login shell was asked whether it runs %s, want switchboard's claude link", link)
				}
			}
			if !strings.Contains(shown, "\n5. The skill\n") {
				t.Errorf("the terminal showed\n%s\nwant setup gone on to the skill", shown)
			}
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

// checkShellFiles checks switchboard's files for the shells are each this
// switchboard's.
func (w *world) checkShellFiles(t *testing.T) {
	t.Helper()
	files := w.shellFiles()
	for _, sh := range shell.Shells {
		completion, err := files.Completion(sh)
		if err != nil {
			t.Fatal(err)
		}
		want, err := shell.Init(sh, w.bin, completion)
		if err != nil {
			t.Fatal(err)
		}
		path := shell.InitFile(w.shellDir, sh)
		if got, err := os.ReadFile(path); err != nil || string(got) != string(want) {
			t.Errorf("%s reads\n%s(%v)\nwant this switchboard's\n%s", path, got, err, want)
		}
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

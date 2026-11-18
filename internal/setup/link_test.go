package setup_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/shell"
)

// What the claude link page does as the user answers it, after Link: add
// switchboard's line, as the offer card's enter does, asking the login shell
// again at once where it's added; or ask again, as r does.
const (
	addIt    = "add"
	askAgain = "again"
)

// settled is what the claude link step finds, as Link finds it, once the
// user has answered its page as actions say, in turn.
func (w *world) settled(t *testing.T, actions ...string) setup.LinkFound {
	t.Helper()
	s := w.setup(t)
	found := w.link(t)
	for _, action := range actions {
		var err error
		switch action {
		case addIt:
			if found, err = s.AddLine(found); err == nil && found.Shell.Situation == setup.LineAdded {
				found, err = s.AskAgain(t.Context(), found)
			}
		case askAgain:
			found, err = s.AskAgain(t.Context(), found)
		}
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	return found
}

func TestTheClaudeLinkPageHasTheLoginShellRunTheLink(t *testing.T) {
	const runsOwn = "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed"
	const through = "runs switchboard's claude: asked just now, as a new terminal starts it"
	const aliasFix = ": removing the alias, or pointing it at claude, puts it right"
	const npx = "zsh's claude is alias claude='npx @anthropic-ai/claude-code', which runs something other than switchboard's claude" + aliasFix
	const cantCheck = " · setup can't check it for this shell: put switchboard's bin directory first on PATH by hand"
	other := shell.Answer{Kind: shell.Other, Claude: "<root>/claude-code/bin/claude"}
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's line in
		// .zshrc, before the step.
		lay func(t *testing.T, w *world)
		// actions are the user's on the claude link page, in turn.
		actions []string
		want    setup.Situation
		// wantSaid is the shell's row, its detail <root> for the world's.
		wantSaid string
		// wantFile is the startup file, from the home, and wantText what it
		// reads, <line> standing for switchboard's line.
		wantFile, wantText string
		// unmade is a startup file, from the home, never made.
		unmade string
	}{
		{
			name:     "offered, and added: it runs the link",
			actions:  []string{addIt},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name:     "offered, and added to the end of a .zshrc without a final newline",
			lay:      func(t *testing.T, w *world) { writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi", 0o644) },
			actions:  []string{addIt},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".zshrc", wantText: "export EDITOR=vi\n<line>\n",
		},
		{
			name: "offered, and added through a .zshrc kept with the user's dotfiles",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.root, "dotfiles", "zshrc"), "export EDITOR=vi\n", 0o644)
				claudetest.Link(t, filepath.Join(w.root, "dotfiles", "zshrc"), filepath.Join(w.home, ".zshrc"))
			},
			actions:  []string{addIt},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".zshrc", wantText: "export EDITOR=vi\n<line>\n",
		},
		{
			name: "zsh says .zshrc is in a ZDOTDIR without one, but the home has one: none written",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi\n", 0o644)
				w.login.configDir = filepath.Join(w.home, ".config", "zsh")
			},
			want: setup.RivalFile,
			wantSaid: runsOwn + " · it says it reads ~/.config/zsh/.zshrc, which isn't there, but ~/.zshrc is: " +
				"setup can't tell which a new terminal reads, so it adds no line",
			wantFile: ".zshrc", wantText: "export EDITOR=vi\n",
			unmade: ".config/zsh/.zshrc",
		},
		{
			name:     "offered, and added in $ZDOTDIR, as zsh says",
			lay:      func(_ *testing.T, w *world) { w.env["ZDOTDIR"] = filepath.Join(w.home, ".config", "zsh") },
			actions:  []string{addIt},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".config/zsh/.zshrc", wantText: "<line>\n",
		},
		{
			name: "offered where the shell says it reads .zshrc, as its own ZDOTDIR, which setup's environment lacks",
			lay: func(_ *testing.T, w *world) {
				w.env["ZDOTDIR"] = filepath.Join(w.home, "elsewhere")
				w.login.configDir = filepath.Join(w.home, ".config", "zsh")
			},
			actions:  []string{addIt},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".config/zsh/.zshrc", wantText: "<line>\n",
		},
		{
			name: "bash, offered for the .profile it reads, never a .bash_profile beside it",
			lay: func(t *testing.T, w *world) {
				w.env["SHELL"] = "/opt/homebrew/bin/bash"
				writeFile(t, filepath.Join(w.home, ".profile"), "export EDITOR=vi\n", 0o644)
			},
			actions:  []string{addIt},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".profile", wantText: "export EDITOR=vi\n<line>\n",
			unmade: ".bash_profile",
		},
		{
			name: "offered, left: asked again until it runs the link, as the directory's put on PATH by hand",
			lay: func(_ *testing.T, w *world) {
				w.login.answers = []shell.Answer{other, {Kind: shell.None}, {Kind: shell.Link}}
			},
			actions:  []string{askAgain, askAgain},
			want:     setup.Through,
			wantSaid: through,
			unmade:   ".zshrc",
		},
		{
			name:     "offered, and left",
			want:     setup.OfferLine,
			wantSaid: runsOwn,
			unmade:   ".zshrc",
		},
		{
			name:     "added, but not taking effect: something read after it changes PATH",
			lay:      func(_ *testing.T, w *world) { w.login.overridden = true },
			actions:  []string{addIt},
			want:     setup.NotTaking,
			wantSaid: runsOwn + " · switchboard's line is in ~/.zshrc, but something read after ~/.zshrc changes PATH",
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, last, but not taking effect: something read after .zshrc changes PATH",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi\n"+w.line()+"\n\n# the end\n", 0o644)
				w.login.overridden = true
			},
			want:     setup.NotTaking,
			wantSaid: runsOwn + " · switchboard's line is in ~/.zshrc, but something read after ~/.zshrc changes PATH",
			wantFile: ".zshrc", wantText: "export EDITOR=vi\n<line>\n\n# the end\n",
		},
		{
			name: "there already, but not taking effect: something after it changes PATH",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), w.line()+"\nexport PATH=/opt/claude-code/bin:$PATH\n", 0o644)
				w.login.overridden = true
			},
			want:     setup.NotTaking,
			wantSaid: runsOwn + " · switchboard's line is in ~/.zshrc, but something after the line changes PATH",
			wantFile: ".zshrc", wantText: "<line>\nexport PATH=/opt/claude-code/bin:$PATH\n",
		},
		{
			name: "there already, the shell not answering at first, asked again",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.answers = []shell.Answer{{Why: "it didn't answer within 15s"}}
			},
			actions:  []string{askAgain},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, reading another switchboard shell directory's file",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), shell.Line(shell.Zsh, filepath.Join(w.root, "old", "switchboard", "shell", "init.zsh"), w.home)+"\n", 0o644)
			},
			want: setup.OtherLine,
			wantSaid: runsOwn + " · switchboard's line in ~/.zshrc reads <root>/old/switchboard/shell/init.zsh, " +
				"not this switchboard's file for zsh, ~/.local/share/switchboard/shell/init.zsh",
		},
		{
			name: "there already, and run, an alias to the name in front: said to go through switchboard",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude='claude --verbose'", Runs: shell.ByName}
			},
			want:     setup.Through,
			wantSaid: through + " · zsh's claude is alias claude='claude --verbose', which runs claude by name, so goes through switchboard too",
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, PATH's the link, but an alias in front runs another claude by its path: the alias to put right",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude=~/.claude/local/claude", Runs: shell.APath, Path: filepath.Join(w.home, ".claude", "local", "claude")}
			},
			want:     setup.InFront,
			wantSaid: "zsh's claude is alias claude=~/.claude/local/claude, which runs ~/.claude/local/claude, not switchboard's claude" + aliasFix,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, PATH's the link, but an alias in front runs something else: the alias to put right, until it is",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				npx := shell.Front{Text: "alias claude='npx @anthropic-ai/claude-code'", Runs: shell.Else}
				w.login.answers = []shell.Answer{{Kind: shell.Link, Front: npx}, {Kind: shell.Link}}
			},
			actions:  []string{askAgain},
			want:     setup.Through,
			wantSaid: through,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "offered, an alias in front of another claude running something else: added, and the alias still to put right",
			lay: func(_ *testing.T, w *world) {
				w.login.front = shell.Front{Text: "alias claude='npx @anthropic-ai/claude-code'", Runs: shell.Else}
			},
			actions:  []string{addIt},
			want:     setup.InFront,
			wantSaid: npx,
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "PATH's another claude, but an alias in front runs the link by its path: done",
			lay: func(_ *testing.T, w *world) {
				w.login.front = shell.Front{Text: "alias claude=~/.local/share/switchboard/bin/claude", Runs: shell.TheLink}
			},
			want:     setup.Through,
			wantSaid: through + " · zsh's claude is alias claude=~/.local/share/switchboard/bin/claude, which runs switchboard's claude link by its path",
			unmade:   ".zshrc",
		},
		{
			name: "there already, PATH's the link, but an alias in front sets PATH for claude: the alias to put right",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude='PATH=/opt/real/bin claude'", Runs: shell.SetsPath}
			},
			want: setup.InFront,
			wantSaid: "zsh's claude is alias claude='PATH=/opt/real/bin claude', which sets PATH for claude, so setup can't tell which claude it runs: " +
				"taking that PATH setting out of the alias puts it right",
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, and run, a function in front, never blamed on PATH",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "function claude", Runs: shell.Unknown}
			},
			want:     setup.Through,
			wantSaid: through + " · zsh's claude is function claude, which setup doesn't read: it goes through switchboard where it runs claude by name",
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "there already, and run, a wrapper alias in front, which setup doesn't read",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude='caffeinate -i claude'", Runs: shell.Unknown}
			},
			want:     setup.Through,
			wantSaid: through + " · zsh's claude is alias claude='caffeinate -i claude', which setup doesn't read: it goes through switchboard where it runs claude by name",
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "offered, an alias to the name in front of another claude: added, and run",
			lay: func(_ *testing.T, w *world) {
				w.login.front = shell.Front{Text: "alias claude='claude --verbose'", Runs: shell.ByName}
			},
			actions:  []string{addIt},
			want:     setup.Through,
			wantSaid: through + " · zsh's claude is alias claude='claude --verbose', which runs claude by name, so goes through switchboard too",
			wantFile: ".zshrc", wantText: "<line>\n",
		},
		{
			name: "the shell not answering, and no line: none written, even where setup's own ZDOTDIR is a terminal's",
			lay: func(_ *testing.T, w *world) {
				w.env["ZDOTDIR"] = filepath.Join(w.root, "editor-terminal")
				w.login.answers = []shell.Answer{{Why: "it didn't answer within 15s"}}
			},
			want:     setup.WhichFile,
			wantSaid: "setup can't ask it which claude it runs: it didn't answer within 15s · setup can't tell which startup file it reads, so it adds no line",
			unmade:   ".zshrc",
		},
		{
			name: "can't be added, as .zshrc is a link that leads nowhere",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "gone"), filepath.Join(w.home, ".zshrc"))
			},
			actions:  []string{addIt},
			want:     setup.CantAdd,
			wantSaid: "setup can't add its line to ~/.zshrc: <root>/home/.zshrc is a link to <root>/gone, which isn't there",
		},
		{
			name:     "another shell, not asked: the directory alone",
			lay:      func(_ *testing.T, w *world) { w.env["SHELL"] = "/bin/tcsh" },
			want:     setup.UnknownShell,
			wantSaid: "setup can't ask it which claude it runs: switchboard asks zsh, bash and fish alone" + cantCheck,
			unmade:   ".zshrc",
		},
		{
			name:     "no $SHELL: the directory alone",
			lay:      func(_ *testing.T, w *world) { w.env["SHELL"] = "" },
			want:     setup.UnknownShell,
			wantSaid: "setup can't ask it which claude it runs: $SHELL isn't set" + cantCheck,
			unmade:   ".zshrc",
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

			found := w.settled(t, tt.actions...)
			if found.Shell.Situation != tt.want {
				t.Errorf("the claude link step left the shell %v, want %v", found.Shell.Situation, tt.want)
			}
			if got := w.rows([]setup.Said{found.Shell.Said})[0].detail; got != tt.wantSaid {
				t.Errorf("the claude link step said of the shell\n%s\nwant\n%s", got, tt.wantSaid)
			}
			if tt.wantFile != "" {
				file := filepath.Join(w.home, filepath.FromSlash(tt.wantFile))
				text, _ := os.ReadFile(file)
				if want := strings.ReplaceAll(tt.wantText, "<line>", w.line()); string(text) != want {
					t.Errorf("%s reads %q, want %q", file, text, want)
				}
			}
			if tt.unmade != "" {
				file := filepath.Join(w.home, filepath.FromSlash(tt.unmade))
				if _, err := os.Lstat(file); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("%s: %v, want it never made", file, err)
				}
			}
			for _, link := range w.login.asked {
				if link != filepath.Join(w.bin, "claude") {
					t.Errorf("the login shell was asked whether it runs %s, want switchboard's claude link", link)
				}
			}
		})
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
	files, err := w.shellFiles().Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Errorf("switchboard's files for the shells are %+v, want four", files)
	}
	for _, f := range files {
		if f.State != shell.Current {
			t.Errorf("%s: %v, want it this switchboard's", f.Path, f.State)
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

package setup_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/shell"
)

// row is a Said as setup's screen draws it.
type row struct {
	mark          setup.Mark
	label, detail string
}

// rows are said as the screen draws them, the world's root written <root>.
func (w *world) rows(said []setup.Said) []row {
	var out []row
	for _, s := range w.rootedSaid(said) {
		out = append(out, row{s.Mark, s.Label, s.Detail})
	}
	return out
}

// link takes the claude link step as Setup.Link does, failing the test where
// it fails.
func (w *world) link(t *testing.T) setup.LinkFound {
	t.Helper()
	found, err := w.setup(t).Link(t.Context())
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	return found
}

// binByHand is switchboard's bin directory in the world, written from $HOME,
// as setup offers it to put first on PATH by hand.
const binByHand = "$HOME/.local/share/switchboard/bin"

// checkRemedy checks what found's remedy puts in place is the world's own:
// switchboard's line for the user's shell, or switchboard's bin directory.
func (w *world) checkRemedy(t *testing.T, found setup.LinkFound) {
	t.Helper()
	r := found.Shell.Remedy()
	for i, part := range r.Parts {
		if part.Put != "" && part.Put != w.line() && part.Put != binByHand {
			t.Errorf("the remedy's part %d puts %q in place, want switchboard's line, %q, or its bin directory, %q", i, part.Put, w.line(), binByHand)
		}
	}
	if r.Line != "" && r.Line != w.line() {
		t.Errorf("the remedy copies the line %q, want switchboard's, %q", r.Line, w.line())
	}
	if r.Dir != "" && r.Dir != binByHand {
		t.Errorf("the remedy copies the directory %q, want switchboard's bin directory, %q", r.Dir, binByHand)
	}
}

func TestLinkSaysTheLinkOnItsRow(t *testing.T) {
	const leads = "~/.local/share/switchboard/bin/claude → switchboard"
	const leftAlone = "not made: ~/.local/share/switchboard/bin/claude is there already, and isn't a link, so it's left alone: move it aside, then run setup again"
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's claude
		// link, before the step.
		lay  func(t *testing.T, w *world)
		want row
		// made is what the step did to the link, and led where a link it
		// repaired led.
		made setup.LinkMade
		led  string
		// wantShell is the shell's row, where the step stops before it asks
		// the shell.
		wantShell *row
	}{
		{name: "made", want: row{setup.Done, "Link", leads}, made: setup.LinkCreated},
		{
			name: "there already: nothing written",
			lay:  func(t *testing.T, w *world) { claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude")) },
			want: row{setup.Done, "Link", leads},
			made: setup.LinkInPlace,
		},
		{
			name: "repaired, leading to another switchboard",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.home, "go", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
			},
			want: row{setup.Done, "Link", leads + " · repaired: it led to ~/go/bin/switchboard"},
			made: setup.LinkRepaired,
			led:  "~/go/bin/switchboard",
		},
		{
			name: "repaired, leading to this switchboard's version rather than the path it was run by",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.root, "Cellar", "switchboard", "1.2.3", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
			},
			want: row{setup.Done, "Link", leads + " · repaired: it led to <root>/Cellar/switchboard/1.2.3/bin/switchboard"},
			made: setup.LinkRepaired,
			led:  "<root>/Cellar/switchboard/1.2.3/bin/switchboard",
		},
		{
			name:      "blocked by a file in its place, left alone: the shell not asked",
			lay:       func(t *testing.T, w *world) { writeFile(t, filepath.Join(w.bin, "claude"), "#!/bin/sh\n", 0o700) },
			want:      row{setup.Warning, "Link", leftAlone},
			wantShell: &row{setup.Warning, "Your shell", "not asked, as the claude link isn't made"},
		},
		{
			name:      "blocked by a directory in its place, left alone: the shell not asked",
			lay:       func(t *testing.T, w *world) { mkdir(t, filepath.Join(w.bin, "claude")) },
			want:      row{setup.Warning, "Link", leftAlone},
			wantShell: &row{setup.Warning, "Your shell", "not asked, as the claude link isn't made"},
		},
		{
			name: "made, claude off PATH, where its installer puts it",
			lay: func(_ *testing.T, w *world) {
				w.path = nil
				w.installPaths = []string{w.realClaude}
			},
			want: row{setup.Done, "Link", leads},
			made: setup.LinkCreated,
		},
		{
			name: "made, but no Claude Code: the shell not asked",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.realClaude); err != nil {
					t.Fatal(err)
				}
				w.installPaths = []string{filepath.Join(w.home, ".local", "bin", "claude")}
			},
			want: row{setup.Done, "Link", leads},
			made: setup.LinkCreated,
			wantShell: &row{setup.Warning, "Your shell", "not asked, as there's no Claude Code for claude to start: " +
				"can't find claude: it isn't on PATH, nor at <root>/home/.local/bin/claude: install it, then run setup again"},
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

			found := w.link(t)
			if led := strings.ReplaceAll(found.Led, w.root, "<root>"); found.Made != tt.made || led != tt.led {
				t.Errorf("Link() did %v to the link, which led to %q, want %v, %q", found.Made, led, tt.made, tt.led)
			}
			if got, want := w.rows(found.Said), []row{tt.want}; !reflect.DeepEqual(got, want) {
				t.Errorf("Link() said\n%#v\nwant\n%#v", got, want)
			}
			if tt.made != setup.LinkNotMade {
				checkLinked(t, w)
				before[filepath.Join(w.bin, "claude")] = "a link to " + w.switchboard
			}
			w.checkUnchanged(t, before)
			w.checkRemedy(t, found)
			if tt.wantShell == nil {
				return
			}
			shell := found.Shell
			if got := w.rows([]setup.Said{shell.Said})[0]; shell.Situation != setup.Unreached || got != *tt.wantShell {
				t.Errorf("Link() left the shell %v, saying %#v; want it unreached, saying %#v", shell.Situation, got, *tt.wantShell)
			}
			if len(w.login.asked) > 0 {
				t.Errorf("the login shell was asked %d times, want never", len(w.login.asked))
			}
		})
	}
}

func TestLinkWritesTheShellFiles(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done, before the step.
		lay func(t *testing.T, w *world)
	}{
		{name: "each this switchboard's already"},
		{
			name: "one another switchboard's: brought up to date",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.shellDir, "init.zsh"), "# an older switchboard's\n", 0o644)
			},
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}

			found := w.link(t)
			if want := []row{{setup.Done, "Link", "~/.local/share/switchboard/bin/claude → switchboard"}}; !reflect.DeepEqual(w.rows(found.Said), want) {
				t.Errorf("Link() said\n%#v\nwant the link's row alone\n%#v", w.rows(found.Said), want)
			}
			w.checkShellFiles(t)
		})
	}
}

func TestLinkFailsWhereItCantMakeTheBinDirectory(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.RemoveAll(w.bin); err != nil {
		t.Fatal(err)
	}
	lockedDir(t, filepath.Dir(w.bin))

	_, err := w.setup(t).Link(t.Context())
	if err == nil || !strings.HasPrefix(err.Error(), "make switchboard's bin directory: ") {
		t.Errorf("Link() error = %v, want it to say it couldn't make switchboard's bin directory", err)
	}
	if len(w.login.asked) > 0 {
		t.Errorf("the login shell was asked %d times, want never", len(w.login.asked))
	}
}

func TestLinkFindsWhereTheLoginShellLeavesIt(t *testing.T) {
	const ownClaude = "Claude Code's own claude, <root>/claude-code/bin/claude"
	const runsOwn = "runs " + ownClaude + ", so its sessions aren't routed"
	const didntAnswer = "it didn't answer within 15s"
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's line in
		// .zshrc, before the step.
		lay  func(t *testing.T, w *world)
		want setup.Situation
		// wantRow is the shell's row; wantFile its startup file, from the
		// home, where it names one, and wantLine set where it names
		// switchboard's line.
		wantRow  row
		wantFile string
		wantLine bool
		wantLast bool
		// wantUnasked is set where the shell couldn't be asked.
		wantUnasked bool
	}{
		{
			name:    "through",
			lay:     func(t *testing.T, w *world) { w.addLine(t) },
			want:    setup.Through,
			wantRow: row{setup.Done, "Your shell", "runs switchboard's claude: asked just now, as a new terminal starts it"},
		},
		{
			name: "through, an alias in front running claude by name",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude='claude --verbose'", Runs: shell.ByName}
			},
			want: setup.Through,
			wantRow: row{setup.Done, "Your shell", "runs switchboard's claude: asked just now, as a new terminal starts it · " +
				"zsh's claude is alias claude='claude --verbose', which runs claude by name, so goes through switchboard too"},
		},
		{
			name: "a shell setup doesn't ask",
			lay:  func(_ *testing.T, w *world) { w.env["SHELL"] = "/bin/tcsh" },
			want: setup.UnknownShell,
			wantRow: row{setup.Warning, "Your shell", "setup can't ask it which claude it runs: switchboard asks zsh, bash and fish alone · " +
				"setup can't check it for this shell: put switchboard's bin directory first on PATH by hand"},
			wantUnasked: true,
		},
		{
			name: "the link on PATH, an alias in front running another claude by its path, from the home",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude=~/.claude/local/claude", Runs: shell.APath, Path: filepath.Join(w.home, ".claude", "local", "claude")}
			},
			want: setup.InFront,
			wantRow: row{setup.Warning, "Your shell", "zsh's claude is alias claude=~/.claude/local/claude, which runs ~/.claude/local/claude, not switchboard's claude: " +
				"removing the alias, or pointing it at claude, puts it right"},
		},
		{
			name:     "a startup file setup can't look in",
			lay:      func(t *testing.T, w *world) { mkdir(t, filepath.Join(w.home, ".zshrc")) },
			want:     setup.CantLook,
			wantRow:  row{setup.Warning, "Your shell", runsOwn + " · setup can't look for its line in ~/.zshrc: read <root>/home/.zshrc: read <root>/home/.zshrc: is a directory"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name:     "the shell not answering, and no line: which file it reads untold",
			lay:      func(_ *testing.T, w *world) { w.login.answers = []shell.Answer{{Why: didntAnswer}} },
			want:     setup.WhichFile,
			wantRow:  row{setup.Warning, "Your shell", "setup can't ask it which claude it runs: " + didntAnswer + " · setup can't tell which startup file it reads, so it adds no line"},
			wantFile: ".zshrc", wantLine: true, wantUnasked: true,
		},
		{
			name: "the file it says it reads not there, but another is",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi\n", 0o644)
				w.login.configDir = filepath.Join(w.home, ".config", "zsh")
			},
			want: setup.RivalFile,
			wantRow: row{setup.Warning, "Your shell", runsOwn + " · it says it reads ~/.config/zsh/.zshrc, which isn't there, but ~/.zshrc is: " +
				"setup can't tell which a new terminal reads, so it adds no line"},
			wantFile: ".config/zsh/.zshrc", wantLine: true,
		},
		{
			name:     "no line, Claude Code's own claude run: offered",
			want:     setup.OfferLine,
			wantRow:  row{setup.Warning, "Your shell", runsOwn},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name: "no line, another claude run: offered",
			lay: func(_ *testing.T, w *world) {
				w.login.answers = []shell.Answer{{Kind: shell.Other, Claude: filepath.Join(w.home, ".claude", "local", "claude")}}
			},
			want:     setup.OfferLine,
			wantRow:  row{setup.Warning, "Your shell", "runs another claude, ~/.claude/local/claude, so its sessions aren't routed"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name: "no line, Claude Code's own claude run, by a path written untidily: offered, named so",
			lay: func(_ *testing.T, w *world) {
				w.login.answers = []shell.Answer{{Kind: shell.Other, Claude: filepath.Dir(w.realClaude) + "//claude"}}
			},
			want:     setup.OfferLine,
			wantRow:  row{setup.Warning, "Your shell", "runs Claude Code's own claude, <root>/claude-code/bin//claude, so its sessions aren't routed"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name: "no line, Claude Code's own claude run, found where its installer puts it, written untidily: offered, named so",
			lay: func(_ *testing.T, w *world) {
				w.path = nil
				w.installPaths = []string{filepath.Dir(w.realClaude) + "/../bin/claude"}
			},
			want:     setup.OfferLine,
			wantRow:  row{setup.Warning, "Your shell", runsOwn},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name:     "no line, and no claude on PATH: offered",
			lay:      func(_ *testing.T, w *world) { w.login.answers = []shell.Answer{{Kind: shell.None}} },
			want:     setup.OfferLine,
			wantRow:  row{setup.Warning, "Your shell", "finds no claude, so claude doesn't go through switchboard"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name: "a line reading another switchboard shell directory's file",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), shell.Line(shell.Zsh, filepath.Join(w.root, "old", "switchboard", "shell", "init.zsh"), w.home)+"\n", 0o644)
			},
			want: setup.OtherLine,
			wantRow: row{setup.Warning, "Your shell", runsOwn + " · switchboard's line in ~/.zshrc reads <root>/old/switchboard/shell/init.zsh, " +
				"not this switchboard's file for zsh, ~/.local/share/switchboard/shell/init.zsh"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name: "the line there, last, not taking effect",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.overridden = true
			},
			want:     setup.NotTaking,
			wantRow:  row{setup.Warning, "Your shell", runsOwn + " · switchboard's line is in ~/.zshrc, but something read after ~/.zshrc changes PATH"},
			wantFile: ".zshrc", wantLine: true, wantLast: true,
		},
		{
			name: "the line there, something after it changing PATH",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), w.line()+"\nexport PATH=/opt/claude-code/bin:$PATH\n", 0o644)
				w.login.overridden = true
			},
			want:     setup.NotTaking,
			wantRow:  row{setup.Warning, "Your shell", runsOwn + " · switchboard's line is in ~/.zshrc, but something after the line changes PATH"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name: "the line there, the shell not answering",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.answers = []shell.Answer{{Why: didntAnswer}}
			},
			want:     setup.NotTaking,
			wantRow:  row{setup.Warning, "Your shell", "switchboard's line is in ~/.zshrc, but setup can't ask it whether it takes effect: " + didntAnswer},
			wantFile: ".zshrc", wantLine: true, wantLast: true, wantUnasked: true,
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

			found := w.link(t)
			got := found.Shell
			if got.Situation != tt.want {
				t.Errorf("Link() left the shell %v, want %v", got.Situation, tt.want)
			}
			w.checkRemedy(t, found)
			if got.Unasked != tt.wantUnasked {
				t.Errorf("Link() left the shell unasked: %v, want %v", got.Unasked, tt.wantUnasked)
			}
			if rows := w.rows([]setup.Said{got.Said}); rows[0] != tt.wantRow {
				t.Errorf("Link() said of the shell\n%#v\nwant\n%#v", rows[0], tt.wantRow)
			}
			w.checkLineFound(t, got, tt.wantFile, tt.wantLine)
			if got.Last != tt.wantLast {
				t.Errorf("Link() found the line last: %v, want %v", got.Last, tt.wantLast)
			}
		})
	}
}

// checkLineFound checks found names file, from the home, as its startup
// file, written from ~ as Where, "" for none, and switchboard's line where
// line is set.
func (w *world) checkLineFound(t *testing.T, found setup.ShellFound, file string, line bool) {
	t.Helper()
	wantFile, wantWhere := "", ""
	if file != "" {
		wantFile, wantWhere = filepath.Join(w.home, filepath.FromSlash(file)), "~/"+file
	}
	if found.File != wantFile || found.Where != wantWhere {
		t.Errorf("found the startup file %q, written %q; want %q, written %q", found.File, found.Where, wantFile, wantWhere)
	}
	if wantLine := w.line(); line && found.Line != wantLine || !line && found.Line != "" {
		t.Errorf("found the line %q, want it: %v", found.Line, line)
	}
}

// offered is what the claude link step finds in a world that's done but
// for switchboard's line in .zshrc, which it offers.
func offered(t *testing.T) (*world, setup.LinkFound) {
	t.Helper()
	w := newWorld(t)
	w.done(t)
	if err := os.Remove(filepath.Join(w.home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	found := w.link(t)
	if found.Shell.Situation != setup.OfferLine {
		t.Fatalf("Link() left the shell %v, want the line offered", found.Shell.Situation)
	}
	return w, found
}

func TestAddLineAddsTheLineOffered(t *testing.T) {
	w, found := offered(t)

	added, err := w.setup(t).AddLine(found)
	if err != nil {
		t.Fatalf("AddLine() error = %v", err)
	}
	want := setup.Said{Mark: setup.Done, Label: "Your shell", Detail: "switchboard's line added to the end of ~/.zshrc"}
	if added.Shell.Situation != setup.LineAdded || added.Shell.Said != want || !added.Shell.Last {
		t.Errorf("AddLine() left the shell %+v, want the line added, last, saying %+v", added.Shell, want)
	}
	w.checkRemedy(t, added)
	if !reflect.DeepEqual(added.Said, found.Said) {
		t.Errorf("AddLine() said %+v of the link, want what Link said, %+v", added.Said, found.Said)
	}
	if text, err := os.ReadFile(found.Shell.File); err != nil || string(text) != w.line()+"\n" {
		t.Errorf("%s reads %q (%v), want switchboard's line", found.Shell.File, text, err)
	}
}

func TestAddLineAddsTheLineOnce(t *testing.T) {
	tests := []struct {
		name string
		// text is what .zshrc reads once the user has put a line there since
		// the step offered one, <line> standing for switchboard's.
		text     string
		want     setup.Situation
		wantSaid setup.Said
		wantLast bool
	}{
		{
			name:     "this switchboard's, last: found there",
			text:     "export EDITOR=vi\n<line>\n",
			want:     setup.LineAdded,
			wantSaid: setup.Said{Mark: setup.Done, Label: "Your shell", Detail: "switchboard's line is in ~/.zshrc already"},
			wantLast: true,
		},
		{
			name:     "this switchboard's, not last: found there",
			text:     "<line>\nexport PATH=/opt/claude-code/bin:$PATH\n",
			want:     setup.LineAdded,
			wantSaid: setup.Said{Mark: setup.Done, Label: "Your shell", Detail: "switchboard's line is in ~/.zshrc already"},
		},
		{
			name: "another shell directory's: the line to put in its place",
			text: "<old>\n",
			want: setup.OtherLine,
			wantSaid: setup.Said{
				Mark:  setup.Warning,
				Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed · " +
					"switchboard's line in ~/.zshrc reads <root>/old/switchboard/shell/init.zsh, not this switchboard's file for zsh, ~/.local/share/switchboard/shell/init.zsh",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, found := offered(t)
			text := strings.ReplaceAll(tt.text, "<line>", w.line())
			text = strings.ReplaceAll(text, "<old>", shell.Line(shell.Zsh, filepath.Join(w.root, "old", "switchboard", "shell", "init.zsh"), w.home))
			writeFile(t, found.Shell.File, text, 0o644)

			added, err := w.setup(t).AddLine(found)
			if err != nil {
				t.Fatalf("AddLine() error = %v", err)
			}
			if said := w.rootedSaid([]setup.Said{added.Shell.Said})[0]; added.Shell.Situation != tt.want || said != tt.wantSaid || added.Shell.Last != tt.wantLast {
				t.Errorf("AddLine() left the shell %v, saying %+v, last: %v; want %v, saying %+v, last: %v",
					added.Shell.Situation, said, added.Shell.Last, tt.want, tt.wantSaid, tt.wantLast)
			}
			w.checkRemedy(t, added)
			if now, err := os.ReadFile(found.Shell.File); err != nil || string(now) != text {
				t.Errorf("%s reads %q (%v), want it as the user left it, %q", found.Shell.File, now, err, text)
			}
		})
	}
}

func TestAddLineThatCantBeAddedLeavesItByHand(t *testing.T) {
	w, found := offered(t)
	claudetest.Link(t, filepath.Join(w.root, "gone"), found.Shell.File)

	added, err := w.setup(t).AddLine(found)
	if err != nil {
		t.Fatalf("AddLine() error = %v", err)
	}
	got := w.rootedSaid([]setup.Said{added.Shell.Said})[0]
	want := setup.Said{
		Mark:   setup.Warning,
		Label:  "Your shell",
		Detail: "setup can't add its line to ~/.zshrc: <root>/home/.zshrc is a link to <root>/gone, which isn't there",
	}
	if added.Shell.Situation != setup.CantAdd || got != want {
		t.Errorf("AddLine() left the shell %v, saying %+v; want it not added, saying %+v", added.Shell.Situation, got, want)
	}
	if !reflect.DeepEqual(added.Shell.Remedy(), found.Shell.Remedy()) {
		t.Errorf("AddLine() left %+v to put right by hand, want the offer's, %+v", added.Shell.Remedy(), found.Shell.Remedy())
	}
	w.checkRemedy(t, added)
}

func TestAddLineThatCantLookInTheFileLeavesItByHand(t *testing.T) {
	w, found := offered(t)
	mkdir(t, found.Shell.File)
	before := w.snapshot(t)

	added, err := w.setup(t).AddLine(found)
	if err != nil {
		t.Fatalf("AddLine() error = %v", err)
	}
	got := w.rootedSaid([]setup.Said{added.Shell.Said})[0]
	want := setup.Said{
		Mark:   setup.Warning,
		Label:  "Your shell",
		Detail: "setup can't add its line to ~/.zshrc: read <root>/home/.zshrc: read <root>/home/.zshrc: is a directory",
	}
	if added.Shell.Situation != setup.CantAdd || got != want {
		t.Errorf("AddLine() left the shell %v, saying %+v; want it not added, saying %+v", added.Shell.Situation, got, want)
	}
	w.checkUnchanged(t, before)
}

func TestAddLineRefusesWhereNoLineIsOffered(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	found := w.link(t)
	before := w.snapshot(t)

	if _, err := w.setup(t).AddLine(found); err == nil {
		t.Errorf("AddLine() of a shell left %v added a line, want it to fail", found.Shell.Situation)
	}
	w.checkUnchanged(t, before)
}

func TestAskAgainSortsTheSituationAfresh(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's line in
		// .zshrc, before the step; meanwhile after it; then again before the
		// shell is asked again, after add, where it's set, adds the line as
		// AddLine does.
		lay, meanwhile, again func(t *testing.T, w *world)
		add                   bool
		was, want             setup.Situation
		// wantSaid is the shell's row, and wantFile its startup file, from
		// the home, "" for none.
		wantSaid setup.Said
		wantFile string
		wantLast bool
		// wantFront is what an alias in front runs, as the step words it.
		wantFront string
	}{
		{
			name: "offered, the line added, and run",
			add:  true,
			was:  setup.OfferLine, want: setup.Through,
			wantSaid: setup.Said{
				Mark: setup.Done, Label: "Your shell", Detail: "runs switchboard's claude: asked just now, as a new terminal starts it",
			},
		},
		{
			name:  "offered, the line added, not taking effect: in the file it was added to, last",
			add:   true,
			again: func(_ *testing.T, w *world) { w.login.overridden = true },
			was:   setup.OfferLine, want: setup.NotTaking,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed · switchboard's line is in ~/.zshrc, but something read after ~/.zshrc changes PATH",
			},
			wantFile: ".zshrc", wantLast: true,
		},
		{
			name:  "offered, the line added, the shell not answering: not taking effect, as setup can't tell",
			add:   true,
			again: func(_ *testing.T, w *world) { w.login.answers = []shell.Answer{{Why: "it didn't answer within 15s"}} },
			was:   setup.OfferLine, want: setup.NotTaking,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "switchboard's line is in ~/.zshrc, but setup can't ask it whether it takes effect: it didn't answer within 15s",
			},
			wantFile: ".zshrc", wantLast: true,
		},
		{
			name: "offered, the line added, the link on PATH, an alias in front running something else",
			add:  true,
			again: func(_ *testing.T, w *world) {
				w.login.front = shell.Front{Text: "alias claude='npx @anthropic-ai/claude-code'", Runs: shell.Else}
			},
			was: setup.OfferLine, want: setup.InFront,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "zsh's claude is alias claude='npx @anthropic-ai/claude-code', which runs something other than switchboard's claude: removing the alias, or pointing it at claude, puts it right",
			},
			wantFront: "zsh's claude is alias claude='npx @anthropic-ai/claude-code', which runs something other than switchboard's claude: removing the alias, or pointing it at claude, puts it right",
		},
		{
			name: "offered, the line found there as it was to be added, not last, not taking effect: not last",
			meanwhile: func(t *testing.T, w *world) {
				writeFile(t, w.startupFile(), w.line()+"\nexport PATH=/opt/claude-code/bin:$PATH\n", 0o644)
			},
			add:   true,
			again: func(_ *testing.T, w *world) { w.login.overridden = true },
			was:   setup.OfferLine, want: setup.NotTaking,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed · switchboard's line is in ~/.zshrc, but something after the line changes PATH",
			},
			wantFile: ".zshrc",
		},
		{
			name: "which file it reads untold, the shell answering now: offered",
			lay:  func(_ *testing.T, w *world) { w.login.answers = []shell.Answer{{Why: "it didn't answer within 15s"}} },
			was:  setup.WhichFile, want: setup.OfferLine,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed",
			},
			wantFile: ".zshrc",
		},
		{
			name: "the file it says it reads not there, but another is, the shell saying it reads that one now: offered",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi\n", 0o644)
				w.login.configDir = filepath.Join(w.home, ".config", "zsh")
			},
			again: func(_ *testing.T, w *world) { w.login.configDir = "" },
			was:   setup.RivalFile, want: setup.OfferLine,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed",
			},
			wantFile: ".zshrc",
		},
		{
			name: "a line reading another shell directory's file, put in its place, not taking effect",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), shell.Line(shell.Zsh, filepath.Join(w.root, "old", "switchboard", "shell", "init.zsh"), w.home)+"\n", 0o644)
			},
			again: func(t *testing.T, w *world) {
				writeFile(t, w.startupFile(), w.line()+"\nexport PATH=/opt/claude-code/bin:$PATH\n", 0o644)
				w.login.overridden = true
			},
			was: setup.OtherLine, want: setup.NotTaking,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed · switchboard's line is in ~/.zshrc, but something after the line changes PATH",
			},
			wantFile: ".zshrc",
		},
		{
			name:  "offered, the line added by hand, and run",
			again: func(t *testing.T, w *world) { w.addLine(t) },
			was:   setup.OfferLine, want: setup.Through,
			wantSaid: setup.Said{
				Mark: setup.Done, Label: "Your shell", Detail: "runs switchboard's claude: asked just now, as a new terminal starts it",
			},
		},
		{
			name: "offered, the line added by hand, not taking effect: the line there, no longer offered",
			again: func(t *testing.T, w *world) {
				writeFile(t, w.startupFile(), w.line()+"\nexport PATH=/opt/claude-code/bin:$PATH\n", 0o644)
				w.login.overridden = true
			},
			was: setup.OfferLine, want: setup.NotTaking,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed · switchboard's line is in ~/.zshrc, but something after the line changes PATH",
			},
			wantFile: ".zshrc",
		},
		{
			name: "offered, the line added, not taking effect, an alias in front running another claude: the alias said",
			add:  true,
			again: func(_ *testing.T, w *world) {
				w.login.overridden = true
				w.login.front = shell.Front{Text: "alias claude=~/.claude/local/claude", Runs: shell.APath, Path: filepath.Join(w.home, ".claude", "local", "claude")}
			},
			was: setup.OfferLine, want: setup.NotTaking,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed · zsh's claude is alias claude=~/.claude/local/claude, " +
					"which runs ~/.claude/local/claude, not switchboard's claude: removing the alias, or pointing it at claude, puts it right · " +
					"switchboard's line is in ~/.zshrc, but something read after ~/.zshrc changes PATH",
			},
			wantFile: ".zshrc", wantLast: true,
			wantFront: "zsh's claude is alias claude=~/.claude/local/claude, which runs ~/.claude/local/claude, not switchboard's claude: " +
				"removing the alias, or pointing it at claude, puts it right",
		},
		{
			name: "offered, another claude run now: still offered, saying so",
			again: func(_ *testing.T, w *world) {
				w.login.answers = []shell.Answer{{Kind: shell.Other, Claude: filepath.Join(w.home, ".claude", "local", "claude")}}
			},
			was: setup.OfferLine, want: setup.OfferLine,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs another claude, ~/.claude/local/claude, so its sessions aren't routed",
			},
			wantFile: ".zshrc",
		},
		{
			name: "an alias in front, taken out, but another claude on PATH: the line there, not taking effect",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude='npx @anthropic-ai/claude-code'", Runs: shell.Else}
			},
			again: func(_ *testing.T, w *world) {
				w.login.front = shell.Front{}
				w.login.overridden = true
			},
			was: setup.InFront, want: setup.NotTaking,
			wantSaid: setup.Said{
				Mark: setup.Warning, Label: "Your shell",
				Detail: "runs Claude Code's own claude, <root>/claude-code/bin/claude, so its sessions aren't routed · switchboard's line is in ~/.zshrc, but something read after ~/.zshrc changes PATH",
			},
			wantFile: ".zshrc", wantLast: true,
		},
		{
			name: "not taking effect, put right: run",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.overridden = true
			},
			again: func(_ *testing.T, w *world) { w.login.overridden = false },
			was:   setup.NotTaking, want: setup.Through,
			wantSaid: setup.Said{
				Mark: setup.Done, Label: "Your shell", Detail: "runs switchboard's claude: asked just now, as a new terminal starts it",
			},
		},
		{
			name:  "a startup file setup can't look in, the directory put on PATH by hand: run",
			lay:   func(t *testing.T, w *world) { mkdir(t, filepath.Join(w.home, ".zshrc")) },
			again: func(_ *testing.T, w *world) { w.login.answers = []shell.Answer{{Kind: shell.Link}} },
			was:   setup.CantLook, want: setup.Through,
			wantSaid: setup.Said{
				Mark: setup.Done, Label: "Your shell", Detail: "runs switchboard's claude: asked just now, as a new terminal starts it",
			},
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
			found := w.link(t)
			if found.Shell.Situation != tt.was {
				t.Fatalf("Link() left the shell %v, want %v", found.Shell.Situation, tt.was)
			}
			if tt.meanwhile != nil {
				tt.meanwhile(t, w)
			}
			if tt.add {
				var err error
				if found, err = w.setup(t).AddLine(found); err != nil {
					t.Fatal(err)
				}
			}
			if tt.again != nil {
				tt.again(t, w)
			}

			got, err := w.setup(t).AskAgain(t.Context(), found)
			if err != nil {
				t.Fatalf("AskAgain() error = %v", err)
			}
			if got.Shell.Situation != tt.want {
				t.Errorf("AskAgain() left the shell %v, want %v", got.Shell.Situation, tt.want)
			}
			if said := w.rootedSaid([]setup.Said{got.Shell.Said})[0]; said != tt.wantSaid {
				t.Errorf("AskAgain() said of the shell\n%+v\nwant\n%+v", said, tt.wantSaid)
			}
			if got.Shell.Last != tt.wantLast {
				t.Errorf("AskAgain() found the line last: %v, want %v", got.Shell.Last, tt.wantLast)
			}
			if got.Shell.Front != tt.wantFront {
				t.Errorf("AskAgain() found in front %q, want %q", got.Shell.Front, tt.wantFront)
			}
			w.checkLineFound(t, got.Shell, tt.wantFile, tt.wantFile != "")
			if got.Shell.Dir != binByHand {
				t.Errorf("AskAgain() found the bin directory to put first %q, want %q", got.Shell.Dir, binByHand)
			}
			if !reflect.DeepEqual(got.Said, found.Said) {
				t.Errorf("AskAgain() said %+v of the link, want what Link said, %+v", got.Said, found.Said)
			}
			w.checkRemedy(t, got)
		})
	}
}

func TestAskAgainUnreachedAsksNothing(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.Remove(filepath.Join(w.bin, "claude")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(w.bin, "claude"), "#!/bin/sh\n", 0o700)
	found := w.link(t)

	got, err := w.setup(t).AskAgain(t.Context(), found)
	if err != nil || !reflect.DeepEqual(got, found) {
		t.Errorf("AskAgain() = %+v, %v; want what Link found, %+v", got, err, found)
	}
	if len(w.login.asked) > 0 {
		t.Errorf("the login shell was asked %d times, want never", len(w.login.asked))
	}
}

func TestAskAgainInterrupted(t *testing.T) {
	w, found := offered(t)
	w.login.interrupted = true

	if _, err := w.setup(t).AskAgain(t.Context(), found); !errors.Is(err, accounts.ErrInterrupted) {
		t.Errorf("AskAgain() error = %v, want it interrupted", err)
	}
}

func TestLinkInterrupted(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.login.interrupted = true

	found, err := w.setup(t).Link(t.Context())
	if !errors.Is(err, accounts.ErrInterrupted) {
		t.Errorf("Link() error = %v, want it interrupted", err)
	}
	if want := []row{{setup.Done, "Link", "~/.local/share/switchboard/bin/claude → switchboard"}}; !reflect.DeepEqual(w.rows(found.Said), want) {
		t.Errorf("Link() said %#v before it was interrupted, want the link, %#v", w.rows(found.Said), want)
	}
	if !reflect.DeepEqual(found.Shell, setup.ShellFound{}) {
		t.Errorf("Link() left the shell %+v, interrupted, want the zero ShellFound", found.Shell)
	}
	if len(w.login.asked) != 1 {
		t.Errorf("the login shell was asked %d times, want once", len(w.login.asked))
	}
}

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

// row is a Said as setup's screen draws it, without the line walk's
// sentence, which the line walk's tests hold.
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
// it fails, or says anything at the terminal: it asks nothing.
func (w *world) link(t *testing.T) setup.LinkFound {
	t.Helper()
	u := &user{}
	found, err := w.setup(t, u).Link(t.Context())
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if shown := u.shown.String(); shown != "" {
		t.Errorf("Link() showed %q at the terminal, want nothing", shown)
	}
	return found
}

func TestLinkSaysTheLinkAndTheShellFiles(t *testing.T) {
	const leads = "~/.local/share/switchboard/bin/claude → switchboard"
	const filesUpToDate = "up to date · ~/.local/share/switchboard/shell"
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's claude
		// link, before the step.
		lay           func(t *testing.T, w *world)
		want          []row
		wantUnreached bool
	}{
		{
			name: "made",
			want: []row{{setup.Done, "Link", leads}, {setup.Done, "Shell files", filesUpToDate}},
		},
		{
			name: "there already",
			lay:  func(t *testing.T, w *world) { claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude")) },
			want: []row{{setup.Done, "Link", leads}, {setup.Done, "Shell files", filesUpToDate}},
		},
		{
			name: "repaired",
			lay: func(t *testing.T, w *world) {
				claudetest.Link(t, filepath.Join(w.home, "go", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
			},
			want: []row{{setup.Done, "Link", leads + " · repaired: it led to ~/go/bin/switchboard"}, {setup.Done, "Shell files", filesUpToDate}},
		},
		{
			name:          "blocked by a file in its place: nothing more",
			lay:           func(t *testing.T, w *world) { writeFile(t, filepath.Join(w.bin, "claude"), "#!/bin/sh\n", 0o700) },
			want:          []row{{setup.Warning, "Link", "not made: ~/.local/share/switchboard/bin/claude is there already, and isn't a link, so it's left alone: move it aside, then run setup again"}},
			wantUnreached: true,
		},
		{
			name: "no Claude Code: no shell files",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.realClaude); err != nil {
					t.Fatal(err)
				}
				w.installPaths = []string{filepath.Join(w.home, ".local", "bin", "claude")}
			},
			want: []row{
				{setup.Done, "Link", leads},
				{setup.Warning, "Claude Code", "none for claude to start: can't find claude: it isn't on PATH, nor at <root>/home/.local/bin/claude: install it, then run setup again"},
			},
			wantUnreached: true,
		},
		{
			name: "shell files written and brought up to date",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.shellDir, "init.zsh"), "# an older switchboard's\n", 0o644)
				for _, name := range []string{"init.bash", "init.fish"} {
					if err := os.Remove(filepath.Join(w.shellDir, name)); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: []row{
				{setup.Done, "Link", leads},
				{setup.Done, "Shell files", "wrote init.bash and init.fish · brought init.zsh up to date · ~/.local/share/switchboard/shell"},
			},
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

			found := w.link(t)
			if got := w.rows(found.Said); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Link() said\n%#v\nwant\n%#v", got, tt.want)
			}
			if unreached := found.Shell.Situation == setup.Unreached; unreached != tt.wantUnreached {
				t.Errorf("Link() left the shell %v, want it unreached: %v", found.Shell.Situation, tt.wantUnreached)
			}
			if tt.wantUnreached && len(w.login.asked) > 0 {
				t.Errorf("the login shell was asked %d times, want never", len(w.login.asked))
			}
		})
	}
}

func TestLinkFindsWhereTheLoginShellLeavesIt(t *testing.T) {
	const runsOther = "runs another claude, <root>/claude-code/bin/claude, so its sessions aren't routed"
	const didntAnswer = "it didn't answer within 15s"
	tests := []struct {
		name string
		// lay changes the world, which is done but for switchboard's line in
		// .zshrc, before the step.
		lay  func(t *testing.T, w *world)
		want setup.Situation
		// wantRow is the shell's row, wantFile its startup file, from the
		// home, where it names one, and wantLine set where it names
		// switchboard's line.
		wantRow  row
		wantFile string
		wantLine bool
		wantLast bool
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
		},
		{
			name: "the link on PATH, an alias in front running another claude",
			lay: func(t *testing.T, w *world) {
				w.addLine(t)
				w.login.front = shell.Front{Text: "alias claude=~/.claude/local/claude", Runs: shell.APath, Path: filepath.Join(w.home, ".claude", "local", "claude")}
			},
			want: setup.InFront,
			wantRow: row{setup.Warning, "Your shell", "zsh's claude is alias claude=~/.claude/local/claude, which runs <root>/home/.claude/local/claude, not switchboard's claude: " +
				"removing the alias, or pointing it at claude, puts it right"},
		},
		{
			name:     "a startup file setup can't look in",
			lay:      func(t *testing.T, w *world) { mkdir(t, filepath.Join(w.home, ".zshrc")) },
			want:     setup.CantLook,
			wantRow:  row{setup.Warning, "Your shell", runsOther + " · setup can't look for its line in ~/.zshrc: read <root>/home/.zshrc: read <root>/home/.zshrc: is a directory"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name:     "the shell not answering, and no line: which file it reads untold",
			lay:      func(_ *testing.T, w *world) { w.login.answers = []shell.Answer{{Why: didntAnswer}} },
			want:     setup.WhichFile,
			wantRow:  row{setup.Warning, "Your shell", "setup can't ask it which claude it runs: " + didntAnswer + " · setup can't tell which startup file it reads, so it adds no line"},
			wantFile: ".zshrc", wantLine: true,
		},
		{
			name: "the file it says it reads not there, but another is",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi\n", 0o644)
				w.login.configDir = filepath.Join(w.home, ".config", "zsh")
			},
			want: setup.RivalFile,
			wantRow: row{setup.Warning, "Your shell", runsOther + " · it says it reads ~/.config/zsh/.zshrc, which isn't there, but ~/.zshrc is: " +
				"setup can't tell which a new terminal reads, so it adds no line"},
			wantFile: ".config/zsh/.zshrc", wantLine: true,
		},
		{
			name:     "no line: offered",
			want:     setup.OfferLine,
			wantRow:  row{setup.Warning, "Your shell", runsOther},
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
			wantRow: row{setup.Warning, "Your shell", runsOther + " · switchboard's line in ~/.zshrc reads <root>/old/switchboard/shell/init.zsh, " +
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
			wantRow:  row{setup.Warning, "Your shell", runsOther + " · switchboard's line is in ~/.zshrc, but something read after ~/.zshrc changes PATH"},
			wantFile: ".zshrc", wantLine: true, wantLast: true,
		},
		{
			name: "the line there, something after it changing PATH",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), w.line()+"\nexport PATH=/opt/claude-code/bin:$PATH\n", 0o644)
				w.login.overridden = true
			},
			want:     setup.NotTaking,
			wantRow:  row{setup.Warning, "Your shell", runsOther + " · switchboard's line is in ~/.zshrc, but something after the line changes PATH"},
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
			wantFile: ".zshrc", wantLine: true, wantLast: true,
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

			got := w.link(t).Shell
			if got.Situation != tt.want {
				t.Errorf("Link() left the shell %v, want %v", got.Situation, tt.want)
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
// file, "" for none, and switchboard's line where line is set.
func (w *world) checkLineFound(t *testing.T, found setup.ShellFound, file string, line bool) {
	t.Helper()
	wantFile := ""
	if file != "" {
		wantFile = filepath.Join(w.home, filepath.FromSlash(file))
	}
	if found.File != wantFile {
		t.Errorf("found the startup file %q, want %q", found.File, wantFile)
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

	added, err := w.setup(t, &user{}).AddLine(found)
	if err != nil {
		t.Fatalf("AddLine() error = %v", err)
	}
	want := setup.Said{Mark: setup.Done, Label: "Your shell", Detail: "switchboard's line added to the end of ~/.zshrc", Sentence: "Added it to ~/.zshrc."}
	if added.Shell.Situation != setup.LineAdded || added.Shell.Said != want || added.Shell.ByHand != nil {
		t.Errorf("AddLine() left the shell %+v, want the line added, saying %+v, with nothing by hand", added.Shell, want)
	}
	if !reflect.DeepEqual(added.Said, found.Said) {
		t.Errorf("AddLine() said %+v of the link, want what Link said, %+v", added.Said, found.Said)
	}
	if text, err := os.ReadFile(found.Shell.File); err != nil || string(text) != w.line()+"\n" {
		t.Errorf("%s reads %q (%v), want switchboard's line", found.Shell.File, text, err)
	}
}

func TestAddLineThatCantBeAddedLeavesItByHand(t *testing.T) {
	w, found := offered(t)
	claudetest.Link(t, filepath.Join(w.root, "gone"), found.Shell.File)

	added, err := w.setup(t, &user{}).AddLine(found)
	if err != nil {
		t.Fatalf("AddLine() error = %v", err)
	}
	got := w.rootedSaid([]setup.Said{added.Shell.Said})[0]
	want := setup.Said{
		Mark:     setup.Warning,
		Label:    "Your shell",
		Detail:   "setup can't add its line to ~/.zshrc: <root>/home/.zshrc is a link to <root>/gone, which isn't there",
		Sentence: "Setup can't add it: <root>/home/.zshrc is a link to <root>/gone, which isn't there.",
	}
	if added.Shell.Situation != setup.CantAdd || got != want {
		t.Errorf("AddLine() left the shell %v, saying %+v; want it not added, saying %+v", added.Shell.Situation, got, want)
	}
	if !reflect.DeepEqual(added.Shell.ByHand, found.Shell.ByHand) {
		t.Errorf("AddLine() left %q by hand, want the offer's, %q", added.Shell.ByHand, found.Shell.ByHand)
	}
}

func TestAddLineRefusesWhereNoLineIsOffered(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	found := w.link(t)
	before := w.snapshot(t)

	if _, err := w.setup(t, &user{}).AddLine(found); err == nil {
		t.Errorf("AddLine() of a shell left %v added a line, want it to fail", found.Shell.Situation)
	}
	w.checkUnchanged(t, before)
}

func TestAskAgain(t *testing.T) {
	notYet := setup.Said{
		Mark:     setup.Warning,
		Label:    "Your shell",
		Detail:   "finds no claude, so claude doesn't go through switchboard",
		Sentence: "claude doesn't go through switchboard in a new terminal yet: zsh finds no claude.",
	}
	tests := []struct {
		name string
		// add is set where switchboard's line is added before the shell is
		// asked again.
		add bool
		// lay changes the world before the shell is asked again.
		lay  func(w *world)
		want setup.Situation
		// wantSaid is the shell's row, where the test holds it.
		wantSaid *setup.Said
		wantLast bool
	}{
		{
			name: "the line added, and run",
			add:  true,
			want: setup.Through,
		},
		{
			name:     "the line added, not taking effect",
			add:      true,
			lay:      func(w *world) { w.login.overridden = true },
			want:     setup.NotTaking,
			wantLast: true,
		},
		{
			name: "the line added, the shell not answering: not taking effect, as setup can't tell",
			add:  true,
			lay:  func(w *world) { w.login.answers = []shell.Answer{{Why: "it didn't answer within 15s"}} },
			want: setup.NotTaking,
			wantSaid: &setup.Said{
				Mark:     setup.Warning,
				Label:    "Your shell",
				Detail:   "switchboard's line is in ~/.zshrc, but setup can't ask it whether it takes effect: it didn't answer within 15s",
				Sentence: "switchboard's line is in ~/.zshrc, but setup can't ask zsh whether it takes effect: it didn't answer within 15s.",
			},
			wantLast: true,
		},
		{
			name: "the line added, the link on PATH, an alias in front running something else",
			add:  true,
			lay: func(w *world) {
				w.login.front = shell.Front{Text: "alias claude='npx @anthropic-ai/claude-code'", Runs: shell.Else}
			},
			want: setup.InFront,
		},
		{
			name: "the line put in by hand, and run",
			lay: func(w *world) {
				w.login.answers = []shell.Answer{{Kind: shell.Link}}
			},
			want: setup.Through,
		},
		{
			name:     "not yet: the offer kept, saying what the shell runs now",
			lay:      func(w *world) { w.login.answers = []shell.Answer{{Kind: shell.None}} },
			want:     setup.OfferLine,
			wantSaid: &notYet,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, found := offered(t)
			if tt.add {
				var err error
				if found, err = w.setup(t, &user{}).AddLine(found); err != nil {
					t.Fatal(err)
				}
			}
			if tt.lay != nil {
				tt.lay(w)
			}

			u := &user{}
			got, err := w.setup(t, u).AskAgain(t.Context(), found)
			if err != nil {
				t.Fatalf("AskAgain() error = %v", err)
			}
			if shown := u.shown.String(); shown != "" {
				t.Errorf("AskAgain() showed %q at the terminal, want nothing", shown)
			}
			if got.Shell.Situation != tt.want {
				t.Errorf("AskAgain() left the shell %v, want %v", got.Shell.Situation, tt.want)
			}
			if said := w.rootedSaid([]setup.Said{got.Shell.Said})[0]; tt.wantSaid != nil && said != *tt.wantSaid {
				t.Errorf("AskAgain() said of the shell\n%+v\nwant\n%+v", said, *tt.wantSaid)
			}
			if got.Shell.Last != tt.wantLast {
				t.Errorf("AskAgain() found the line last: %v, want %v", got.Shell.Last, tt.wantLast)
			}
			w.checkLineFound(t, got.Shell, fileOf(tt.want), tt.want != setup.Through && tt.want != setup.InFront)
			if !reflect.DeepEqual(got.Said, found.Said) {
				t.Errorf("AskAgain() said %+v of the link, want what Link said, %+v", got.Said, found.Said)
			}
		})
	}
}

// fileOf is the startup file, from the home, a shell left as situation
// names in a world asked again: .zshrc, but where it names none.
func fileOf(situation setup.Situation) string {
	if situation == setup.Through || situation == setup.InFront {
		return ""
	}
	return ".zshrc"
}

func TestAskAgainInterrupted(t *testing.T) {
	w, found := offered(t)
	w.login.interrupted = true

	if _, err := w.setup(t, &user{}).AskAgain(t.Context(), found); !errors.Is(err, accounts.ErrInterrupted) {
		t.Errorf("AskAgain() error = %v, want it interrupted", err)
	}
}

func TestLinkInterrupted(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.login.interrupted = true

	found, err := w.setup(t, &user{}).Link(t.Context())
	if !errors.Is(err, accounts.ErrInterrupted) {
		t.Errorf("Link() error = %v, want it interrupted", err)
	}
	if labels := labelsOf(found.Said); labels != "Link, Shell files" {
		t.Errorf("Link() said %s before it was interrupted, want the link and the shell files", labels)
	}
}

// labelsOf are said's labels, in turn.
func labelsOf(said []setup.Said) string {
	var labels []string
	for _, s := range said {
		labels = append(labels, s.Label)
	}
	return strings.Join(labels, ", ")
}

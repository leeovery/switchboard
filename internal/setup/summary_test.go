package setup_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/shell"
)

// The summary's lines, in the steps' order.
const (
	accountsLine = iota
	primingLine
	serviceLine
	linkLine
	skillLine
)

// lineOf is the summary's line at, every step passed through, failing the
// test unless there's a line a step before Done, named as setup names it.
func lineOf(t *testing.T, s setup.Summary, at int) setup.StepLine {
	t.Helper()
	s.At = setup.StepDone
	lines := s.Lines()
	steps := setup.Steps()[:setup.StepDone]
	if len(lines) != len(steps) {
		t.Fatalf("the summary has %d lines, want one a step, %d", len(lines), len(steps))
	}
	for i, l := range lines {
		if l.Label != steps[i].Title() {
			t.Errorf("line %d is labelled %q, want %q", i, l.Label, steps[i].Title())
		}
	}
	return lines[at]
}

// checkStepLine checks got is marked mark, says detail, and has add under it,
// with no sentence of the line walk's.
func checkStepLine(t *testing.T, got setup.StepLine, mark setup.Mark, detail, add string) {
	t.Helper()
	if got.Mark != mark || got.Detail != detail || got.Add != add || got.Sentence != "" {
		t.Errorf("the line is %+v\nwant marked %v, saying %q, adding %q, with no sentence", got, mark, detail, add)
	}
}

func TestEveryStepToldNothingIsPassedThrough(t *testing.T) {
	for i := range setup.StepDone {
		got := lineOf(t, setup.Summary{}, int(i))
		if got.Mark != setup.Done || got.Detail != "" || got.Add != "" {
			t.Errorf("line %d, %s, is %+v, want ✓ and its name alone", i, got.Label, got)
		}
	}
}

// summedLines are the lines, each its label, mark and detail, in turn.
func summedLines(lines []setup.StepLine) []string {
	var got []string
	for _, l := range lines {
		got = append(got, l.Label+" "+[]string{"✓", "!", "✗"}[l.Mark]+" "+l.Detail)
	}
	return got
}

func TestTheSummaryAsTheWalkLeavesIt(t *testing.T) {
	day := config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}
	review := setup.Review{Accounts: []setup.Reviewed{{ID: "work", Primary: true, Token: setup.TokenStanding{State: setup.TokenWorking}}}}
	serviced := setup.Serviced{Said: []setup.Said{{Mark: setup.Done, Label: "Installed"}}, Router: &router.Health{OK: true}}
	failedService := setup.Serviced{Said: []setup.Said{{Mark: setup.Failed, Label: "Not set up", Detail: "the router couldn't start"}}}
	skill := setup.Said{Mark: setup.Done, Label: "Up to date"}
	failedSkill := setup.Said{Mark: setup.Failed, Label: "Not installed", Detail: "mkdir ~/.claude: not a directory"}
	const (
		accounts = "Accounts ✓ 1, working · primary: work"
		priming  = "Priming ✓ on, 08:00–23:00"
		service  = "Service ✓ installed · router healthy"
	)
	tests := []struct {
		name    string
		summary setup.Summary
		want    []string
	}{
		{name: "left at Accounts: Accounts alone", summary: setup.Summary{At: setup.StepAccounts, Review: &review},
			want: []string{accounts}},
		{name: "left at Priming, its day read: up to Priming", summary: setup.Summary{At: setup.StepPriming, Review: &review, Day: &day},
			want: []string{accounts, priming}},
		{name: "left at Priming before its day is read: Priming has no line", summary: setup.Summary{At: setup.StepPriming, Review: &review},
			want: []string{accounts}},
		{name: "Priming failed: why", summary: setup.Summary{At: setup.StepPriming, Failure: "read the config: it doesn't parse", Review: &review},
			want: []string{accounts, "Priming ✗ read the config: it doesn't parse"}},
		{name: "left while the service step runs: no line for it", summary: setup.Summary{At: setup.StepService, Review: &review, Day: &day},
			want: []string{accounts, priming}},
		{name: "the service step failed: its failed row", summary: setup.Summary{At: setup.StepService, Failure: "the router couldn't start", Review: &review, Day: &day, Service: &failedService},
			want: []string{accounts, priming, "Service ✗ not set up: the router couldn't start"}},
		{name: "left at the claude link, passed through to it", summary: setup.Summary{At: setup.StepLink, Review: &review, Day: &day, Service: &serviced},
			want: []string{accounts, priming, service}},
		{name: "left at Skill, done", summary: setup.Summary{At: setup.StepSkill, Review: &review, Day: &day, Service: &serviced, Skill: &skill},
			want: []string{accounts, priming, service, "claude link ✓ ", "Skill ✓ up to date"}},
		{name: "the skill step failed", summary: setup.Summary{At: setup.StepSkill, Failure: "mkdir ~/.claude: not a directory", Review: &review, Day: &day, Service: &serviced, Skill: &failedSkill},
			want: []string{accounts, priming, service, "claude link ✓ ", "Skill ✗ not installed: mkdir ~/.claude: not a directory"}},
		{name: "a failure told by nothing of the step's: why", summary: setup.Summary{At: setup.StepSkill, Failure: "the skill couldn't be read", Review: &review, Day: &day, Service: &serviced},
			want: []string{accounts, priming, service, "claude link ✓ ", "Skill ✗ the skill couldn't be read"}},
		{name: "at Done: every step", summary: setup.Summary{At: setup.StepDone, Review: &review, Day: &day, Service: &serviced, Skill: &skill},
			want: []string{accounts, priming, service, "claude link ✓ ", "Skill ✓ up to date"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summedLines(tt.summary.Lines()); !slices.Equal(got, tt.want) {
				t.Errorf("the summary reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestTheAccountsLine(t *testing.T) {
	working := setup.TokenStanding{State: setup.TokenWorking}
	refused := setup.TokenStanding{State: setup.TokenNotTaken}
	tests := []struct {
		name   string
		review setup.Review
		mark   setup.Mark
		want   string
	}{
		{name: "all working, the primary's reserve said", review: setup.Review{Accounts: []setup.Reviewed{
			{ID: "work", Primary: true, Reserve: 0.05, Token: working}, {ID: "personal", Token: working}, {ID: "side", Token: working},
		}}, want: "3, all working · primary: work, with 5% reserved for the Claude apps"},
		{name: "a primary reserving nothing", review: setup.Review{Accounts: []setup.Reviewed{
			{ID: "work", Token: working}, {ID: "personal", Primary: true, Token: working},
		}}, want: "2, all working · primary: personal"},
		{name: "one working", review: setup.Review{Accounts: []setup.Reviewed{{ID: "work", Primary: true, Token: working}}},
			want: "1, working · primary: work"},
		{name: "some not working: a warning", review: setup.Review{Accounts: []setup.Reviewed{
			{ID: "work", Primary: true, Token: working}, {ID: "personal", Token: refused}, {ID: "side"},
		}}, mark: setup.Warning, want: "3, 1 working · primary: work"},
		{name: "none primary, of two: a warning", review: setup.Review{Unmarked: true, Accounts: []setup.Reviewed{
			{ID: "work", Token: working}, {ID: "personal", Token: working},
		}}, mark: setup.Warning, want: "2, all working · none is the primary"},
		{name: "none: a warning", review: setup.Review{}, mark: setup.Warning, want: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkStepLine(t, lineOf(t, setup.Summary{Review: &tt.review}, accountsLine), tt.mark, tt.want, "")
		})
	}
}

func TestTheAccountsLineReadsEachTokenAsChecked(t *testing.T) {
	read := setup.TokenStanding{State: setup.TokenRead}
	review := setup.Review{Accounts: []setup.Reviewed{
		{ID: "work", Primary: true, Reserve: 0.05, Token: read}, {ID: "personal", Token: read}, {ID: "side", Token: read},
	}}
	review = review.Checked("work", setup.Reviewed{Token: setup.TokenStanding{State: setup.TokenWorking}}, nil)
	review = review.Checked("personal", setup.Reviewed{Token: setup.TokenStanding{State: setup.TokenNotTaken}}, nil)
	review = review.Checked("side", setup.Reviewed{}, errors.New("dial the API: connection refused"))
	status := setup.StatusLine(setup.RouterStanding{}, &review)[1].Words
	if status != "3 accounts, 2 working" {
		t.Fatalf("the status line counts %q, want 3 accounts, 2 working", status)
	}
	want := "3, 2 working · primary: work, with 5% reserved for the Claude apps"
	checkStepLine(t, lineOf(t, setup.Summary{Review: &review}, accountsLine), setup.Warning, want, "")
}

func TestTheAccountsLineCountsAsTheStatusLineDoes(t *testing.T) {
	for _, state := range []setup.TokenState{setup.TokenRead, setup.TokenWorking, setup.TokenNotTaken} {
		review := setup.Review{Accounts: []setup.Reviewed{
			{ID: "work", Primary: true, Token: setup.TokenStanding{State: setup.TokenWorking}},
			{ID: "personal", Token: setup.TokenStanding{State: state}},
		}}
		status := setup.StatusLine(setup.RouterStanding{}, &review)[1].Words
		counted, _, _ := strings.Cut(lineOf(t, setup.Summary{Review: &review}, accountsLine).Detail, " · ")
		if want := strings.Replace(status, "2 accounts", "2", 1); counted != want {
			t.Errorf("with a token %v, the accounts' line counts %q, want it as the status line, %q, counts, %q", state, counted, status, want)
		}
	}
}

func TestThePrimingLine(t *testing.T) {
	tests := []struct {
		name string
		day  config.Day
		want string
	}{
		{name: "on, over its day", day: config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}, want: "on, 08:00–23:00"},
		{name: "off", want: "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkStepLine(t, lineOf(t, setup.Summary{Day: &tt.day}, primingLine), setup.Done, tt.want, "")
		})
	}
}

func TestTheServiceLine(t *testing.T) {
	installed := setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/Library/LaunchAgents/io.github.leeovery.switchboard.plist", Sentence: "Installed it."}
	updated := setup.Said{Mark: setup.Warning, Label: "Updated", Detail: "reinstalled: its plist had no ProcessType"}
	up := setup.Said{Mark: setup.Done, Label: "Router", Detail: "healthy · 127.0.0.1:4747"}
	restarted := setup.Said{Mark: setup.Warning, Label: "Restarted", Detail: "the router didn't answer"}
	healthy := &router.Health{OK: true}
	tests := []struct {
		name     string
		serviced setup.Serviced
		mark     setup.Mark
		want     string
	}{
		{name: "installed, its router healthy", serviced: setup.Serviced{Said: []setup.Said{installed, up}, Router: healthy},
			want: "installed · router healthy"},
		{name: "installed afresh: installed and updated", serviced: setup.Serviced{Said: []setup.Said{updated, installed, up}, Router: healthy},
			want: "installed and updated · router healthy"},
		{name: "in place, its router up", serviced: setup.Serviced{Said: []setup.Said{up}, Router: healthy},
			want: "installed · router healthy"},
		{name: "its router restarted: news, not a warning", serviced: setup.Serviced{Said: []setup.Said{restarted, up}, Router: healthy},
			want: "installed · router healthy"},
		{name: "its router unhealthy: a warning", serviced: setup.Serviced{
			Said:   []setup.Said{{Mark: setup.Warning, Label: "Router", Detail: "unhealthy: no account can take a request"}},
			Router: &router.Health{Reason: "no account can take a request"},
		}, mark: setup.Warning, want: "installed · router unhealthy: no account can take a request"},
		{name: "its router's reason made safe to show", serviced: setup.Serviced{
			Router: &router.Health{Reason: "no account\ncan take a request\x1b[2J"},
		}, mark: setup.Warning, want: "installed · router unhealthy: " + prose.Clean("no account\ncan take a request\x1b[2J")},
		{name: "something left to do: a warning, saying it", serviced: setup.Serviced{
			Said: []setup.Said{installed, {Mark: setup.Warning, Label: "Warning", Detail: "side has no usable token"}, up}, Router: healthy,
		}, mark: setup.Warning, want: "installed · router healthy · side has no usable token"},
		{name: "left as it is: a warning, no router seen", serviced: setup.Serviced{
			Said: []setup.Said{{Mark: setup.Warning, Label: "Left as it is", Detail: "which the service runs can't be told"}},
		}, mark: setup.Warning, want: "installed · which the service runs can't be told"},
		{name: "failed: why, as its failed row says", serviced: setup.Serviced{
			Said: []setup.Said{installed, {Mark: setup.Failed, Label: "Not set up", Detail: "the router couldn't start"}},
		}, mark: setup.Failed, want: "not set up: the router couldn't start"},
		{name: "failed on a row of its own", serviced: setup.Serviced{
			Said: []setup.Said{{Mark: setup.Failed, Label: "Router run by hand", Detail: "a router run some other way answers"}},
		}, mark: setup.Failed, want: "router run by hand: a router run some other way answers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkStepLine(t, lineOf(t, setup.Summary{Service: &tt.serviced}, serviceLine), tt.mark, tt.want, "")
		})
	}
}

func TestTheSkillLine(t *testing.T) {
	const at = "~/.claude/skills/switchboard/SKILL.md"
	tests := []struct {
		name string
		said setup.Said
		want string
	}{
		{name: "installed", said: setup.Said{Mark: setup.Done, Label: "Installed", Detail: at}, want: "installed"},
		{name: "updated", said: setup.Said{Mark: setup.Done, Label: "Updated", Detail: at}, want: "updated"},
		{name: "up to date", said: setup.Said{Mark: setup.Done, Label: "Up to date", Detail: at}, want: "up to date"},
		{name: "failed: why", said: setup.Said{Mark: setup.Failed, Label: "Not installed", Detail: "mkdir ~/.claude: not a directory"}, want: "not installed: mkdir ~/.claude: not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkStepLine(t, lineOf(t, setup.Summary{Skill: &tt.said}, skillLine), tt.said.Mark, tt.want, "")
		})
	}
}

// unlinked takes switchboard's claude link out of a world, for the step to
// create.
func unlinked(t *testing.T, w *world) {
	t.Helper()
	if err := os.Remove(filepath.Join(w.bin, "claude")); err != nil {
		t.Fatal(err)
	}
}

// misled has switchboard's claude link in a world lead to another
// switchboard, ~/go/bin/switchboard, for the step to repair.
func misled(t *testing.T, w *world) {
	t.Helper()
	unlinked(t, w)
	claudetest.Link(t, filepath.Join(w.home, "go", "bin", "switchboard"), filepath.Join(w.bin, "claude"))
}

func TestTheClaudeLinksLineAsTheStepFindsIt(t *testing.T) {
	tests := []struct {
		name string
		// lay changes a world that's done, before the step.
		lay  func(t *testing.T, w *world)
		mark setup.Mark
		want string
		// add is what's to be put in place by hand, <line> standing for
		// switchboard's line.
		add string
	}{
		{name: "done, the link in place, as on a run again", mark: setup.Done, want: "your shell runs it"},
		{name: "done, the link created", lay: unlinked, mark: setup.Done, want: "created · your shell runs it"},
		{name: "done, the link repaired: where it led", lay: misled, mark: setup.Done, want: "repaired: it led to ~/go/bin/switchboard · your shell runs the claude link"},
		{
			name: "not first on PATH: the line to add, naming the file",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(filepath.Join(w.home, ".zshrc")); err != nil {
					t.Fatal(err)
				}
			},
			mark: setup.Warning, want: "it isn't first on your PATH. Add this line to ~/.zshrc:", add: "<line>",
		},
		{
			name: "not first on PATH, the link created",
			lay: func(t *testing.T, w *world) {
				unlinked(t, w)
				if err := os.Remove(filepath.Join(w.home, ".zshrc")); err != nil {
					t.Fatal(err)
				}
			},
			mark: setup.Warning, want: "created, but it isn't first on your PATH. Add this line to ~/.zshrc:", add: "<line>",
		},
		{
			name: "not first on PATH, the link repaired",
			lay: func(t *testing.T, w *world) {
				misled(t, w)
				if err := os.Remove(filepath.Join(w.home, ".zshrc")); err != nil {
					t.Fatal(err)
				}
			},
			mark: setup.Warning, want: "repaired: it led to ~/go/bin/switchboard, but the claude link isn't first on your PATH. Add this line to ~/.zshrc:", add: "<line>",
		},
		{
			name: "not made: why, as the link's row says",
			lay: func(t *testing.T, w *world) {
				link := filepath.Join(w.bin, "claude")
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				writeFile(t, link, "#!/bin/sh\n", 0o700)
			},
			mark: setup.Warning, want: "not made: ~/.local/share/switchboard/bin/claude is there already, and isn't a link, so it's left alone: move it aside, then run setup again",
		},
		{
			name: "no Claude Code for it to start",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.realClaude); err != nil {
					t.Fatal(err)
				}
				w.installPaths = []string{filepath.Join(w.home, ".local", "bin", "claude")}
			},
			mark: setup.Warning, want: "there's no Claude Code for it to start: install Claude Code, then run setup again",
		},
		{
			name: "not first on PATH, an alias in front running another claude: both said",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(filepath.Join(w.home, ".zshrc")); err != nil {
					t.Fatal(err)
				}
				w.login.front = shell.Front{Text: "alias claude=~/.claude/local/claude", Runs: shell.APath, Path: filepath.Join(w.home, ".claude", "local", "claude")}
			},
			mark: setup.Warning,
			want: "zsh's claude is alias claude=~/.claude/local/claude, which runs ~/.claude/local/claude, not switchboard's claude: " +
				"removing the alias, or pointing it at claude, puts it right; and the claude link isn't first on your PATH. Add this line to ~/.zshrc:",
			add: "<line>",
		},
		{
			name: "the file it says it reads not there, but another is: both named",
			lay: func(t *testing.T, w *world) {
				writeFile(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi\n", 0o644)
				w.login.configDir = filepath.Join(w.home, ".config", "zsh")
			},
			mark: setup.Warning, want: "it isn't first on your PATH. Add this line to the end of whichever of ~/.config/zsh/.zshrc and ~/.zshrc a new terminal reads:", add: "<line>",
		},
		{
			name: "a shell setup doesn't ask: the bin directory to put first",
			lay:  func(_ *testing.T, w *world) { w.env["SHELL"] = "/bin/tcsh" },
			mark: setup.Warning, want: "setup can't check it for your shell. Put this directory first on your PATH:", add: binByHand,
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
			got := lineOf(t, setup.Summary{Link: &found}, linkLine)
			checkStepLine(t, got, tt.mark, tt.want, strings.ReplaceAll(tt.add, "<line>", w.line()))
			if want := tt.add == binByHand; got.Directory != want {
				t.Errorf("the line to copy is a directory: %v, want %v", got.Directory, want)
			}
		})
	}
}

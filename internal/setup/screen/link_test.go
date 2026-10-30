package screen

import (
	"context"
	"errors"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// shellLine is switchboard's line for zsh, which the claude link step leaves
// to add by hand, and binDir switchboard's bin directory, to put first on
// PATH by hand.
const (
	shellLine = `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`
	binDir    = "$HOME/.local/share/switchboard/bin"
)

// The claude link step's outcomes, as setup says them.
var (
	linkRow       = setup.Said{Mark: setup.Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}
	shellThrough  = setup.Said{Mark: setup.Done, Label: "Your shell", Detail: "runs switchboard's claude: asked just now, as a new terminal starts it"}
	hookInstalled = setup.Said{Mark: setup.Done, Label: "Session hook", Detail: "Claude Code says so when a session doesn't start through switchboard · ~/.claude/settings.json"}
	// linkThrough is the link made, and the shell running it.
	linkThrough = setup.LinkFound{Made: setup.LinkCreated, Said: []setup.Said{linkRow}, Shell: setup.ShellFound{Situation: setup.Through, Said: shellThrough}}
	// linkLeft is the link made, and zsh running Claude Code's own claude,
	// switchboard's line offered for ~/.zshrc.
	linkLeft = situated(setup.OfferLine, "runs Claude Code's own claude, ~/.local/bin/claude, so its sessions aren't routed")
	// errNoLink is why the claude link step fails, where a test has it.
	errNoLink = errors.New("make switchboard's bin directory: mkdir ~/.local/share/switchboard: permission denied")
)

// situated is the link made, and the shell leaving the situation given,
// its row saying detail, with the line and the directory a shell setup
// asks would leave.
func situated(situation setup.Situation, detail string) setup.LinkFound {
	found := setup.LinkFound{
		Made: setup.LinkCreated,
		Said: []setup.Said{linkRow},
		Shell: setup.ShellFound{
			Situation: situation,
			Said:      setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: detail},
			File:      "/home/user/.zshrc", Where: "~/.zshrc", Line: shellLine, Dir: binDir,
		},
	}
	switch situation {
	case setup.RivalFile:
		found.Shell.Rival = "~/.zprofile"
	case setup.InFront:
		found.Shell.File, found.Shell.Where, found.Shell.Line = "", "", ""
		found.Shell.Front = "zsh's claude is alias claude=npx, which runs something other than switchboard's claude: removing the alias, or pointing it at claude, puts it right"
	case setup.UnknownShell:
		found.Shell.File, found.Shell.Where, found.Shell.Line = "", "", ""
	}
	return found
}

// linkHarness is a harness opening on the claude link step, which finds
// found, started.
func linkHarness(t *testing.T, found setup.LinkFound) *harness {
	t.Helper()
	h := newHarness(t, dashboard.SetupLink)
	h.steps.found = found
	h.start()
	return h
}

// fact is a fact's row as the page draws it, without colour: its mark, its
// label and what's said of it, in their columns.
func fact(said setup.Said) string {
	marks := map[setup.Mark]string{setup.Done: "✓", setup.Warning: "!", setup.Failed: "✗"}
	return "  " + marks[said.Mark] + "  " + said.Label + strings.Repeat(" ", 21-len([]rune(said.Label))) + said.Detail
}

// keyLine is the key line, 160 cells wide: left at its left, and right
// ending at its last column.
func keyLine(left, right string) string {
	return left + strings.Repeat(" ", 160-len([]rune(left))-len([]rune(right))) + right
}

// wantRows checks the screen's rows from the heading's down: each in want
// reads as it says, and every other is blank.
func wantRows(t *testing.T, rows []string, want map[int]string) {
	t.Helper()
	for y, w := range want {
		if rows[y] != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], w)
		}
	}
	for y := 5; y < len(rows); y++ {
		if _, said := want[y]; !said && rows[y] != "" {
			t.Errorf("row %d reads %q, want it blank", y, rows[y])
		}
	}
}

// The page's own words, as frames 11 and 12 have them, and setup's that it
// shows to put the link right by hand.
const (
	linkPurpose = "  The claude command starts Claude Code through switchboard, so every session is routed."
	asking      = "  ●∙∙  Asking your shell, as a new terminal starts it, which claude it runs"
	pressR      = "  Then press r: setup asks your shell again, as a new terminal would start it."
	addTheLine  = "  Add this line to your shell's startup file yourself, after anything else there that changes PATH:"
	orTheDir    = "  Or, where you keep your PATH another way, put this directory first on it:"
	byHandRight = "enter leave it for now   q quit"
	skillRight  = "enter next: Skill   q quit"
)

func TestTheClaudeLinkStepRunsLinkOnceAsItsReached(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.start()
	if h.steps.links != 0 {
		t.Fatalf("Link ran %d times at the Service, want none till its step is reached", h.steps.links)
	}
	h.pressAndDeliver("enter")
	h.deliver(tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, key := range []string{"j", "k", "pgdown", "c", "d"} {
		h.pressAndDeliver(key)
	}
	if h.steps.links != 1 {
		t.Errorf("Link ran %d times, want once, as its step is reached", h.steps.links)
	}
}

func TestWhileTheShellIsAskedThePageSaysSo(t *testing.T) {
	h := newHarness(t, dashboard.SetupLink)
	rows := h.rows()
	wantRows(t, rows, map[int]string{
		6:  linkPurpose,
		8:  asking,
		36: keyLine("", "q quit"),
	})
	if _, spins := h.model.page().SpinDelay(); !spins {
		t.Error("the page's spinner doesn't move while the shell is asked")
	}
}

func TestWhileTheShellIsAskedAgainThePageSaysSoUnderTheLinksRow(t *testing.T) {
	h := linkHarness(t, linkLeft)
	h.steps.again = []setup.LinkFound{linkThrough}
	pending := h.press("r")
	wantRows(t, h.rows(), map[int]string{
		6:  linkPurpose,
		8:  fact(linkRow),
		10: asking,
		36: keyLine("", "q quit"),
	})
	if _, spins := h.model.page().SpinDelay(); !spins {
		t.Error("the page's spinner doesn't move while the shell is asked again")
	}
	for _, key := range []string{"r", "c", "d", "enter"} {
		if msgs := h.press(key); len(msgs) != 0 {
			t.Errorf("%s while the shell is asked sends %v, want nothing", key, msgs)
		}
	}
	if asked := len(h.steps.askedOf); asked != 1 {
		t.Errorf("the shell was asked again %d times, want once: r does nothing while it's asked", asked)
	}
	h.deliver(pending...)
	if got := h.rows()[10]; got != fact(shellThrough) {
		t.Errorf("once the shell answers, row 10 reads %q, want %q", got, fact(shellThrough))
	}
}

func TestTheShellRunningSwitchboardsClaudeIsDoneOnceTheSessionHookIsInstalled(t *testing.T) {
	h := newHarness(t, dashboard.SetupLink)
	hooking := h.update(linkedMsg{found: linkThrough})
	wantRows(t, h.rows(), map[int]string{
		6:  linkPurpose,
		8:  fact(linkRow),
		10: fact(shellThrough),
		36: keyLine("", "q quit"),
	})
	h.deliver(hooking...)
	wantRows(t, h.rows(), map[int]string{
		6:  linkPurpose,
		8:  fact(linkRow),
		10: fact(shellThrough),
		12: fact(hookInstalled),
		36: keyLine("", skillRight),
	})
	if h.steps.hooks != 1 {
		t.Errorf("the session hook was installed %d times, want once", h.steps.hooks)
	}
}

func TestByHandTheShellsRowSaysTheSituationInSetupsWords(t *testing.T) {
	const (
		line = "      " + shellLine
		dir  = "      " + binDir
	)
	added := map[int]string{13: addTheLine, 15: line, 17: orTheDir, 19: dir, 21: pressR,
		36: keyLine("c copy the line   d copy the directory   r check again", byHandRight)}
	againAlone := map[int]string{13: pressR, 36: keyLine("r check again", byHandRight)}
	tests := []struct {
		name      string
		situation setup.Situation
		want      map[int]string
	}{
		{name: "an alias in front: asking again alone", situation: setup.InFront, want: againAlone},
		{name: "a file setup can't look in: the line added", situation: setup.CantLook, want: added},
		{name: "which file: the line added", situation: setup.WhichFile, want: added},
		{name: "a rival file: the line added", situation: setup.RivalFile, want: added},
		{name: "the line offered: the line added", situation: setup.OfferLine, want: added},
		{name: "a line it couldn't add: the line added", situation: setup.CantAdd, want: added},
		{name: "another switchboard's line: the line put in its place", situation: setup.OtherLine, want: map[int]string{
			13: "  Put this line in its place:", 15: line, 17: orTheDir, 19: dir, 21: pressR,
			36: keyLine("c copy the line   d copy the directory   r check again", byHandRight),
		}},
		{name: "a line not taking effect: moved, no line to copy", situation: setup.NotTaking, want: map[int]string{
			13: "  Move switchboard's line to the end of ~/.zshrc, or what changes PATH after it to before it.",
			15: orTheDir, 17: dir, 19: pressR,
			36: keyLine("d copy the directory   r check again", byHandRight),
		}},
		{name: "a line added: asking again alone", situation: setup.LineAdded, want: againAlone},
		{name: "a shell setup doesn't ask: the directory alone, said once", situation: setup.UnknownShell, want: map[int]string{
			13: "  Put this directory first on PATH, wherever your shell sets it:", 15: dir,
			36: keyLine("d copy the directory", byHandRight),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := situated(tt.situation, "setup's words for "+tt.name)
			h := linkHarness(t, found)
			want := map[int]string{6: linkPurpose, 8: fact(linkRow), 10: fact(found.Shell.Said)}
			maps.Copy(want, tt.want)
			wantRows(t, h.rows(), want)

			for range 3 {
				h.pressAndDeliver("enter")
			}
			lines := h.model.page().Lines()
			setups := setup.Summary{At: setup.StepDone, Link: &found}.Lines()
			if got, want := lines[dashboard.SetupLink], setups[dashboard.SetupLink]; got != want {
				t.Errorf("Done's claude link line is %+v, want setup's, %+v", got, want)
			}
		})
	}
}

func TestUnreachedTheStepEndsAtOnceTheSessionHookInstalled(t *testing.T) {
	tests := []struct {
		name  string
		found setup.LinkFound
	}{
		{name: "the link blocked", found: setup.LinkFound{
			Said:  []setup.Said{{Mark: setup.Warning, Label: "Link", Detail: "not made: ~/.local/share/switchboard/bin/claude isn't a link, so it's left alone: move it aside"}},
			Shell: setup.ShellFound{Situation: setup.Unreached, Said: setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "not asked, as the claude link isn't made"}},
		}},
		{name: "no Claude Code", found: setup.LinkFound{
			Made:  setup.LinkCreated,
			Said:  []setup.Said{linkRow},
			Shell: setup.ShellFound{Situation: setup.Unreached, Said: setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "not asked, as there's no Claude Code for claude to start: install it, then run setup again"}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupLink)
			h.steps.found = tt.found
			h.start()
			if h.steps.hooks != 1 {
				t.Errorf("the session hook was installed %d times as Link answered, before any key, want once", h.steps.hooks)
			}
			wantRows(t, h.rows(), map[int]string{
				6:  linkPurpose,
				8:  fact(tt.found.Said[0]),
				10: fact(tt.found.Shell.Said),
				12: fact(hookInstalled),
				36: keyLine("", skillRight),
			})
			for _, key := range []string{"c", "d", "r"} {
				if msgs := h.press(key); len(msgs) != 0 {
					t.Errorf("%s sends %v, want nothing", key, msgs)
				}
			}
			h.pressAndDeliver("enter")
			if got := h.model.page().At; got != dashboard.SetupSkill {
				t.Errorf("enter reaches %s, want the Skill", got.Title())
			}
			if h.steps.hooks != 1 || len(h.steps.askedOf) != 0 {
				t.Errorf("the session hook was installed %d times, the shell asked again %d; want once, and never", h.steps.hooks, len(h.steps.askedOf))
			}
		})
	}
}

func TestRAsksTheShellAgain(t *testing.T) {
	inFront := situated(setup.InFront, "zsh's claude is alias claude=npx, which runs something other than switchboard's claude")
	tests := []struct {
		name  string
		again setup.LinkFound
		want  map[int]string
	}{
		{name: "switchboard's claude: done", again: linkThrough, want: map[int]string{
			8: fact(linkRow), 10: fact(shellThrough), 12: fact(hookInstalled), 36: keyLine("", skillRight),
		}},
		{name: "anything else: by hand, as it now says", again: inFront, want: map[int]string{
			8: fact(linkRow), 10: fact(inFront.Shell.Said), 13: pressR, 36: keyLine("r check again", byHandRight),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := linkHarness(t, linkLeft)
			h.steps.again = []setup.LinkFound{tt.again}
			h.pressAndDeliver("r")
			tt.want[6] = linkPurpose
			wantRows(t, h.rows(), tt.want)
			if asked := h.steps.askedOf; len(asked) != 1 || asked[0].Shell.Situation != setup.OfferLine {
				t.Errorf("AskAgain was given %+v, want what Link found", asked)
			}
			if h.steps.links != 1 {
				t.Errorf("Link ran %d times, want once: r asks again", h.steps.links)
			}
		})
	}
}

func TestCAndDCopyTheLineAndTheDirectoryWhole(t *testing.T) {
	tests := []struct {
		name         string
		found        setup.LinkFound
		c, d         string
		checkedAgain bool
	}{
		{name: "the line offered: c the line, its spacing kept, d the directory", found: linkLeft, c: shellLine, d: binDir, checkedAgain: true},
		{name: "a shell setup doesn't ask: d the directory alone, no r", found: situated(setup.UnknownShell, "setup can't check it for this shell"), d: binDir},
		{name: "an alias in front: neither", found: situated(setup.InFront, "zsh's claude is alias claude=npx"), checkedAgain: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := linkHarness(t, tt.found)
			for key, want := range map[string]string{"c": tt.c, "d": tt.d} {
				msgs := h.press(key)
				switch {
				case want == "" && len(msgs) != 0:
					t.Errorf("%s sends %v, want nothing", key, msgs)
				case want != "" && (len(msgs) != 1 || !clipboard(msgs, want)):
					t.Errorf("%s sends %v, want %q copied alone", key, msgs, want)
				}
			}
			h.pressAndDeliver("r")
			if got := len(h.steps.askedOf) == 1; got != tt.checkedAgain {
				t.Errorf("r asked the shell again: %t, want %t", got, tt.checkedAgain)
			}
		})
	}
}

func TestEnterLeavesTheClaudeLinkStepInstallingTheSessionHookOnceBeforeTheSkill(t *testing.T) {
	tests := []struct {
		name string
		// found is what Link finds, again what AskAgain finds, in turn, and
		// keys are pressed before the enters that leave the step.
		found setup.LinkFound
		again []setup.LinkFound
		keys  []string
		// enters are the enters that reach the Skill: one where the step has
		// ended, two where enter ends it first.
		enters int
	}{
		{name: "done", found: linkThrough, enters: 1},
		{name: "by hand, left for now", found: linkLeft, enters: 2},
		{name: "by hand, then asked again: done", found: linkLeft, again: []setup.LinkFound{linkThrough}, keys: []string{"r"}, enters: 1},
		{name: "by hand, asked again: by hand still", found: linkLeft, again: []setup.LinkFound{linkLeft}, keys: []string{"r", "r"}, enters: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := linkHarness(t, tt.found)
			h.steps.again = tt.again
			for _, key := range tt.keys {
				h.pressAndDeliver(key)
			}
			for i := range tt.enters {
				if h.steps.skills != 0 {
					t.Errorf("the skill was written before enter %d, want it only once the step's left", i+1)
				}
				h.pressAndDeliver("enter")
			}
			if got := h.model.page().At; got != dashboard.SetupSkill {
				t.Errorf("enter reaches %s, want the Skill", got.Title())
			}
			if h.steps.hooks != 1 || h.steps.skills != 1 {
				t.Errorf("the session hook was installed %d times, the skill written %d; want each once", h.steps.hooks, h.steps.skills)
			}
			h.pressAndDeliver("enter")
			if h.steps.hooks != 1 {
				t.Errorf("the session hook was installed %d times, want once", h.steps.hooks)
			}
			if got := h.model.page().At; got != dashboard.SetupDone {
				t.Errorf("enter at the Skill reaches %s, want Done", got.Title())
			}
		})
	}
}

func TestLeavingByHandEndsTheStepTheSessionHooksRowUnderTheShells(t *testing.T) {
	notInstalled := setup.Said{Mark: setup.Warning, Label: "Session hook", Detail: "setup carries on without it, so a session that doesn't go through switchboard won't say so: settings.json doesn't parse"}
	for _, hook := range []setup.Said{hookInstalled, notInstalled} {
		t.Run(hook.Detail, func(t *testing.T) {
			h := linkHarness(t, linkLeft)
			h.steps.hookSaid = hook
			hooking := h.press("enter")
			wantRows(t, h.rows(), map[int]string{
				6:  linkPurpose,
				8:  fact(linkRow),
				10: fact(linkLeft.Shell.Said),
				36: keyLine("", "q quit"),
			})
			for _, key := range []string{"c", "d", "r", "enter"} {
				if msgs := h.press(key); len(msgs) != 0 {
					t.Errorf("%s while the session hook installs sends %v, want nothing", key, msgs)
				}
			}
			h.deliver(hooking...)
			wantRows(t, h.rows(), map[int]string{
				6:  linkPurpose,
				8:  fact(linkRow),
				10: fact(linkLeft.Shell.Said),
				12: fact(hook),
				36: keyLine("", skillRight),
			})
		})
	}
}

func TestLinkFailingFailsTheStepWordingNothingOfWhatItFound(t *testing.T) {
	tests := []struct {
		name  string
		found setup.LinkFound
	}{
		{name: "before the link is made", found: setup.LinkFound{}},
		{name: "after the link is made: no Claude Code isn't said", found: setup.LinkFound{Made: setup.LinkCreated, Said: []setup.Said{linkRow}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupLink)
			h.steps.found, h.steps.linkErr = tt.found, errNoLink
			h.start()
			wantRows(t, h.rows(), map[int]string{
				6:  linkPurpose,
				8:  fact(setup.Said{Mark: setup.Failed, Label: "Not set up", Detail: errNoLink.Error()}),
				36: keyLine("", "q quit"),
			})
			if h.model.link != nil {
				t.Errorf("the screen keeps %+v as what the step found, want nothing", h.model.link)
			}
			if got := h.model.walk.left(); got != (Left{Ending: Failed, Err: errNoLink}) {
				t.Errorf("setup is left %+v, want failing at the claude link", got)
			}
			got := h.model.summary().Lines()[dashboard.SetupLink]
			if got.Mark != setup.Failed || got.Detail != errNoLink.Error() {
				t.Errorf("the summary's claude link line is %+v, want it failed, saying why", got)
			}
			h.pressAndDeliver("enter")
			if h.model.page().At != dashboard.SetupLink || h.steps.hooks != 0 {
				t.Errorf("enter at the failed step reached %s, the hook installed %d times; want nothing done", h.model.page().At.Title(), h.steps.hooks)
			}
		})
	}
}

func TestAskingAgainFailingFailsTheStep(t *testing.T) {
	h := linkHarness(t, linkLeft)
	h.steps.againErr = errNoLink
	h.pressAndDeliver("r")
	wantRows(t, h.rows(), map[int]string{
		6:  linkPurpose,
		8:  fact(linkRow),
		10: fact(setup.Said{Mark: setup.Failed, Label: "Not set up", Detail: errNoLink.Error()}),
		36: keyLine("", "q quit"),
	})
}

func TestAnInterruptAsTheShellIsAskedLeavesSetupAsQDoes(t *testing.T) {
	tests := []struct {
		name string
		// interrupt has the asking interrupted, Link's or AskAgain's.
		interrupt func(h *harness) []tea.Msg
	}{
		{name: "as Link asks", interrupt: func(h *harness) []tea.Msg {
			h.steps.linkErr = accounts.ErrInterrupted
			return h.run(h.model.Init())
		}},
		{name: "as AskAgain asks", interrupt: func(h *harness) []tea.Msg {
			h.start()
			h.steps.againErr = accounts.ErrInterrupted
			return h.press("r")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupLink)
			h.steps.found = linkLeft
			var quit bool
			for _, msg := range tt.interrupt(h) {
				quit = quits(h.update(msg)) || quit
			}
			if !quit {
				t.Fatal("the interrupt didn't have setup leave")
			}
			if got := h.model.walk.left(); got != (Left{Ending: Paused}) {
				t.Errorf("setup is left %+v, want paused, as q leaves it", got)
			}
		})
	}
}

// linkTillCancelled is setup's steps whose Link says it's started, then
// holds on until its context is cancelled, saying so, and ends only once
// it's let go, saying it has; or where interrupt is set, ends once it's
// closed, interrupted, as at an interrupt from outside the terminal.
type linkTillCancelled struct {
	*fakeSteps
	started, cancelled, release, ended, interrupt chan struct{}
}

func (s *linkTillCancelled) Link(ctx context.Context) (setup.LinkFound, error) {
	close(s.started)
	defer close(s.ended)
	if s.interrupt != nil {
		<-s.interrupt
		return setup.LinkFound{}, accounts.ErrInterrupted
	}
	<-ctx.Done()
	close(s.cancelled)
	<-s.release
	return setup.LinkFound{}, ctx.Err()
}

// hookTillReleased is setup's steps whose SessionHook says it's started,
// and ends only once it's let go, saying it has.
type hookTillReleased struct {
	*fakeSteps
	started, release, ended chan struct{}
}

func (s *hookTillReleased) SessionHook() setup.Said {
	close(s.started)
	<-s.release
	defer close(s.ended)
	return s.fakeSteps.SessionHook()
}

func TestLeavingAsTheSessionHookInstallsWaitsForIt(t *testing.T) {
	tests := []struct {
		name  string
		found setup.LinkFound
		// leave is the key that ends the step, where one must.
		leave string
		quit  string
	}{
		{name: "the shell running switchboard's claude, q", found: linkThrough, quit: "q"},
		{name: "the shell running switchboard's claude, ctrl+c", found: linkThrough, quit: "\x03"},
		{name: "left by hand, q", found: linkLeft, leave: "\r", quit: "q"},
		{name: "left by hand, ctrl+c", found: linkLeft, leave: "\r", quit: "\x03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := &hookTillReleased{fakeSteps: newFakeSteps(), started: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{})}
			steps.found = tt.found
			linked := make(chan struct{})
			var once sync.Once
			term := terminal{
				keys: tt.leave, ready: linked,
				then: tt.quit, thenReady: steps.started,
				saw: func(msg tea.Msg) {
					if _, ok := msg.(linkedMsg); ok {
						once.Do(func() { close(linked) })
					}
				},
			}
			returned := make(chan Left, 1)
			go func() {
				left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupLink}, term)
				if err != nil {
					t.Errorf("run() error = %v, want none", err)
				}
				returned <- left
			}()
			select {
			case <-steps.started:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, the session hook hadn't begun installing")
			}
			select {
			case <-returned:
				t.Fatal("run() returned while the session hook installed, want it to wait for it to end")
			case <-time.After(overlap):
			}
			close(steps.release)
			left := <-returned
			select {
			case <-steps.ended:
			default:
				t.Error("run() returned before the session hook was installed")
			}
			if left != (Left{Ending: Paused}) {
				t.Errorf("run() left setup %+v, want it paused", left)
			}
			if steps.skills != 0 {
				t.Errorf("the skill was written %d times, want none: setup left at the claude link", steps.skills)
			}
		})
	}
}

func TestLeavingAsTheShellIsAskedCancelsTheAskingAndWaitsForIt(t *testing.T) {
	for _, key := range []string{"q", "\x03"} {
		t.Run(strings.ReplaceAll(key, "\x03", "ctrl+c"), func(t *testing.T) {
			steps := &linkTillCancelled{
				fakeSteps: newFakeSteps(),
				started:   make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{}),
			}
			returned := make(chan Left, 1)
			go func() {
				left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupLink}, terminal{keys: key, ready: steps.started})
				if err != nil {
					t.Errorf("run() error = %v, want none", err)
				}
				returned <- left
			}()
			<-steps.cancelled
			select {
			case <-returned:
				t.Fatal("run() returned while Link ran on, want it to wait for it to end")
			case <-time.After(overlap):
			}
			close(steps.release)
			left := <-returned
			select {
			case <-steps.ended:
			default:
				t.Error("run() returned before Link ended")
			}
			if left != (Left{Ending: Paused}) {
				t.Errorf("run() left setup %+v, want it paused", left)
			}
		})
	}
}

func TestAnInterruptFromOutsideAsTheShellIsAskedLeavesSetupPaused(t *testing.T) {
	steps := &linkTillCancelled{
		fakeSteps: newFakeSteps(),
		started:   make(chan struct{}), ended: make(chan struct{}), interrupt: make(chan struct{}),
	}
	returned := make(chan Left, 1)
	go func() {
		left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupLink}, terminal{})
		if err != nil {
			t.Errorf("run() error = %v, want none", err)
		}
		returned <- left
	}()
	<-steps.started
	close(steps.interrupt)
	select {
	case left := <-returned:
		if left != (Left{Ending: Paused}) {
			t.Errorf("run() left setup %+v, want it paused, as q leaves it", left)
		}
	case <-time.After(time.Minute):
		t.Fatal("a minute after the interrupt, setup's screen hadn't left")
	}
}

package screen

import (
	"errors"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// shellLine is the line the claude link step leaves to add by hand.
const shellLine = `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`

// The claude link step's outcomes, as setup finds them.
var (
	linkLeft = &setup.LinkFound{
		Made:  setup.LinkCreated,
		Said:  []setup.Said{{Mark: setup.Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}},
		Shell: setup.ShellFound{Situation: setup.OfferLine, Line: shellLine, Where: "~/.zshrc"},
	}
	linkDone = &setup.LinkFound{
		Made:  setup.LinkCreated,
		Said:  []setup.Said{{Mark: setup.Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}},
		Shell: setup.ShellFound{Situation: setup.Through},
	}
)

// walkedToDone is a harness given link as what the claude link step found,
// opening on Priming, its day day, and walked through each step to Done.
func walkedToDone(t *testing.T, link *setup.LinkFound, day config.Day) *harness {
	t.Helper()
	h := newHarness(t, dashboard.SetupPriming)
	cfg := h.config(dashboard.SetupPriming)
	cfg.Link = link
	h.steps.day = day
	h.model = New(t.Context(), cfg)
	t.Cleanup(h.model.jobs.wait)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.start()
	for range dashboard.SetupDone - dashboard.SetupPriming {
		h.pressAndDeliver("enter")
	}
	if got := h.model.page().At; got != dashboard.SetupDone {
		t.Fatalf("walking on from Priming reaches %s, want Done", got.Title())
	}
	return h
}

// clipboard reports whether msgs set the terminal's clipboard to text, by
// OSC 52.
func clipboard(msgs []tea.Msg, text string) bool {
	return slices.Contains(msgs, tea.SetClipboard(text)())
}

func TestDoneSaysEachStepsLineAsTheWalkLeftIt(t *testing.T) {
	h := walkedToDone(t, linkLeft, config.Day{Start: 8 * time.Hour, End: 23 * time.Hour})
	rows := h.rows()
	for y, want := range map[int]string{
		6:  "  ✓  Accounts             3, all working · primary: work",
		8:  "  ✓  Priming              on, 08:00–23:00",
		10: "  ✓  Service              installed · router healthy",
		12: "  !  claude link          created, but it isn't first on your PATH. Add this line to ~/.zshrc:",
		13: "                          " + shellLine,
		15: "  ✓  Skill                installed",
	} {
		if rows[y] != want {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], want)
		}
	}
}

func TestDonesAccountsLineReadsTheReviewTheAccountsPageChecked(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	for range dashboard.SetupDone {
		h.pressAndDeliver("enter")
	}
	page := h.model.page()
	if page.At != dashboard.SetupDone {
		t.Fatalf("walking on from Accounts reaches %s, want Done", page.At.Title())
	}
	if got, want := page.Status[1].Words, "3 accounts, 2 working"; got != want {
		t.Errorf("the status line counts %q, want %q", got, want)
	}
	got := page.Lines()[0]
	if want := "3, 2 working · primary: work, with 5% reserved for the Claude apps"; got.Mark != setup.Warning || got.Detail != want {
		t.Errorf("Done's Accounts line is %+v\nwant a warning, saying %q, as the status line counts", got.Said, want)
	}
}

func TestDoneSaysTheDayAsItWasSet(t *testing.T) {
	tests := []struct {
		name, typed, want string
	}{
		{name: "a day typed", typed: "10:00-18:00", want: "on, 10:00–18:00"},
		{name: "the field cleared: off", want: "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupPriming)
			h.steps.day = config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}
			h.start()
			h.pressAndDeliver("end")
			for range len("08:00-23:00") {
				h.pressAndDeliver("backspace")
			}
			h.typeIn(tt.typed)
			for range dashboard.SetupDone - dashboard.SetupPriming {
				h.pressAndDeliver("enter")
			}
			if got, want := h.rows()[8], "  ✓  Priming              "+tt.want; got != want {
				t.Errorf("Done's Priming row reads %q, want %q", got, want)
			}
		})
	}
}

func TestCCopiesTheLineOnlyWhereTheLinkIsntDone(t *testing.T) {
	tests := []struct {
		name string
		link *setup.LinkFound
		copy bool
	}{
		{name: "not first on PATH: the line copied", link: linkLeft, copy: true},
		{name: "done: nothing", link: linkDone},
		{name: "passed through: nothing", link: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := walkedToDone(t, tt.link, config.Day{})
			msgs := h.press("c")
			if got := clipboard(msgs, shellLine); got != tt.copy {
				t.Errorf("c at Done sends %v: the line copied %v, want %v", msgs, got, tt.copy)
			}
			if tt.copy && len(msgs) != 1 {
				t.Errorf("c at Done sends %v, want the copy alone", msgs)
			}
			if !tt.copy && len(msgs) != 0 {
				t.Errorf("c at Done sends %v, want nothing", msgs)
			}
		})
	}
}

func TestCCopiesNothingBeforeDone(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	cfg := h.config(dashboard.SetupSkill)
	cfg.Link = linkLeft
	h.model = New(t.Context(), cfg)
	t.Cleanup(h.model.jobs.wait)
	h.start()
	if msgs := h.press("c"); len(msgs) != 0 {
		t.Errorf("c at Skill sends %v, want nothing", msgs)
	}
}

func TestEnterAndQAtDone(t *testing.T) {
	tests := []struct {
		key  string
		want Left
	}{
		{key: "enter", want: Left{Ending: ToDashboard}},
		{key: "q", want: Left{Ending: Finished}},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			h := walkedToDone(t, linkLeft, config.Day{})
			if !quits(h.press(tt.key)) {
				t.Fatalf("%s at Done didn't quit", tt.key)
			}
			if got := h.model.walk.left(); got != tt.want {
				t.Errorf("%s at Done left setup %+v, want %+v", tt.key, got, tt.want)
			}
		})
	}
}

func TestTheSummaryIsWhereTheWalkStands(t *testing.T) {
	const (
		accounts = "Accounts 3, all working · primary: work"
		priming  = "Priming on, 08:00–23:00"
		service  = "Service installed · router healthy"
	)
	tests := []struct {
		name string
		// fail has a step fail; enters are pressed from Accounts, the
		// Priming field edited as edit has it before the enter there.
		fail   func(*fakeSteps)
		edit   func(*harness)
		enters int
		want   []string
	}{
		{name: "at Accounts", want: []string{accounts}},
		{name: "at Priming", enters: 1, want: []string{accounts, priming}},
		{name: "Priming failed", fail: func(s *fakeSteps) { s.primingErr = errNoConfig }, enters: 1, want: []string{accounts, "Priming " + errNoConfig.Error()}},
		{name: "Priming failed, its error saying nothing once made safe: failed still", fail: func(s *fakeSteps) { s.primingErr = errors.New("\x1b \x07\n") }, enters: 1,
			want: []string{accounts, "Priming failed"}},
		{name: "Priming's day not set", fail: func(s *fakeSteps) { s.setErr = errUnwritable }, edit: func(h *harness) { h.clear() }, enters: 2,
			want: []string{accounts, "Priming " + errUnwritable.Error()}},
		{name: "at Service", enters: 2, want: []string{accounts, priming, service}},
		{name: "Service failed", fail: func(s *fakeSteps) {
			s.serviced, s.serviceErr = setup.Serviced{Said: []setup.Said{routerUnstarted}}, errNoService
		}, enters: 2, want: []string{accounts, priming, "Service router: " + routerUnstarted.Detail}},
		{name: "at the claude link, which has told nothing", enters: 3, want: []string{accounts, priming, service}},
		{name: "at Skill", enters: 4, want: []string{accounts, priming, service, "claude link ", "Skill installed"}},
		{name: "Skill failed", fail: func(s *fakeSteps) { s.said, s.skillErr = notWritten, errNoSkill }, enters: 4,
			want: []string{accounts, priming, service, "claude link ", "Skill not installed: " + errNoSkill.Error()}},
		{name: "at Done", enters: 5, want: []string{accounts, priming, service, "claude link ", "Skill installed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupAccounts)
			h.steps.day = config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}
			if tt.fail != nil {
				tt.fail(h.steps)
			}
			h.start()
			for i := range tt.enters {
				if i == 1 && tt.edit != nil {
					tt.edit(h)
				}
				h.pressAndDeliver("enter")
			}
			var got []string
			for _, l := range h.model.summary().Lines() {
				got = append(got, l.Label+" "+l.Detail)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the summary reads\n%q\nwant\n%q", got, tt.want)
			}
			if s := h.model.summary(); s.At == setup.StepPriming && s.Failure != "" && h.model.priming.failure != s.Failure {
				t.Errorf("the Priming page says it failed with %q, the summary %q", h.model.priming.failure, s.Failure)
			}
		})
	}
}

func TestTheLinkFoundIsMadeSafeToShowAsItEntersTheScreen(t *testing.T) {
	h := newHarness(t, dashboard.SetupDone)
	cfg := h.config(dashboard.SetupDone)
	cfg.Link = &setup.LinkFound{
		Made: setup.LinkRepaired, Led: "~/old\x1b[2J/switchboard",
		Said: []setup.Said{{Mark: setup.Done, Label: "Link", Detail: "~/bin/claude\x1b[2J → switchboard"}},
		Shell: setup.ShellFound{
			Situation: setup.OfferLine, Said: setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "zsh's claude is alias\nclaude=x\x1b[31m"},
			File: "/home/user/zsh\x1b[2J/.zshrc", Where: "~/zsh\x1b[2J/.zshrc", Rival: "~/.zshrc\x07", Front: "zsh's claude is\talias\x1b[31m",
			Line: "if [ -r \"$HOME/x\x1b[2J\" ]; then . \"$HOME/x\x1b[2J\"; fi  # switchboard", Dir: "$HOME/bin\x1b[2J",
		},
	}
	h.model = New(t.Context(), cfg)
	t.Cleanup(h.model.jobs.wait)
	given, got := cfg.Link.Shell, h.model.link.Shell
	for _, tt := range []struct{ what, got, want string }{
		{what: "the link's row", got: h.model.link.Said[0].Detail, want: prose.Clean(cfg.Link.Said[0].Detail)},
		{what: "where the link led", got: h.model.link.Led, want: prose.Clean(cfg.Link.Led)},
		{what: "the shell's row", got: got.Said.Detail, want: prose.Clean(given.Said.Detail)},
		{what: "the startup file shown", got: got.Where, want: prose.Clean(given.Where)},
		{what: "the other startup file", got: got.Rival, want: prose.Clean(given.Rival)},
		{what: "the alias in front", got: got.Front, want: prose.Clean(given.Front)},
		{what: "the line to copy, its spacing kept", got: got.Line, want: prose.Visible(given.Line)},
		{what: "the directory to copy", got: got.Dir, want: prose.Visible(given.Dir)},
		{what: "the startup file written to, left as it is", got: got.File, want: given.File},
	} {
		if tt.got != tt.want {
			t.Errorf("%s reads %q, want %q", tt.what, tt.got, tt.want)
		}
	}
	if cfg.Link.Said[0].Detail != "~/bin/claude\x1b[2J → switchboard" {
		t.Errorf("the link given was changed to %q, want it left as it was", cfg.Link.Said[0].Detail)
	}
}

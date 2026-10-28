package screen

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// Setup is the steps the screen calls.
var _ Steps = (*setup.Setup)(nil)

// The rows of the screen at 160×37.
const (
	titleRow  = 0
	statusRow = 2
	factRow   = 8
	keyRow    = 36
)

func TestTheWalkGoesForwardAStepAtATimeEachPassedTicked(t *testing.T) {
	h := newHarness(t, dashboard.SetupAccounts)
	h.start()
	steps := dashboard.SetupSteps()
	for i, step := range steps {
		if got := h.model.page(); got.Step != step || !slices.Equal(got.Passed, steps[:i]) {
			t.Fatalf("at %s, the screen shows %s, %v passed; want %s, %v passed", step.Title(), got.Step.Title(), got.Passed, step.Title(), steps[:i])
		}
		if step == dashboard.SetupDone {
			break
		}
		msgs := h.press("enter")
		if quits(msgs) {
			t.Fatalf("enter at %s quit, want it to move on", step.Title())
		}
		h.deliver(msgs...)
	}
	if h.steps.skills != 1 {
		t.Errorf("Skill ran %d times, want once, as the walk reached its step", h.steps.skills)
	}
	title := h.rows()[titleRow]
	if want := " SWITCHBOARD   setup     ✓ Accounts   ✓ Priming   ✓ Service   ✓ claude link   ✓ Skill   Done "; !strings.HasPrefix(title, want) {
		t.Errorf("at Done, the title row reads\n%q\nwant it to start\n%q", title, want)
	}
}

func TestTheScreenOpensOnALaterStepThoseBeforeItPassed(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.start()
	got := h.model.page()
	if want := dashboard.SetupSteps()[:dashboard.SetupSkill]; got.Step != dashboard.SetupSkill || !slices.Equal(got.Passed, want) {
		t.Errorf("the screen opens on %s, %v passed; want Skill, %v passed", got.Step.Title(), got.Passed, want)
	}
	if h.steps.skills != 1 || got.Skill == nil || *got.Skill != installed {
		t.Errorf("opening on Skill ran it %d times, saying %v; want once, saying %+v", h.steps.skills, got.Skill, installed)
	}
}

func TestEnterWaitsForTheStepShowingToBeDone(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	var skilled []tea.Msg
	for _, msg := range h.run(h.model.Init()) {
		if _, ok := msg.(skilledMsg); ok {
			skilled = append(skilled, msg)
			continue
		}
		h.deliver(msg)
	}

	h.pressAndDeliver("enter")
	if got := h.model.page().Step; got != dashboard.SetupSkill {
		t.Fatalf("enter before Skill has answered shows %s, want Skill", got.Title())
	}
	h.deliver(skilled...)
	h.pressAndDeliver("enter")
	if got := h.model.page().Step; got != dashboard.SetupDone {
		t.Errorf("enter once Skill has answered shows %s, want Done", got.Title())
	}
}

func TestLeaving(t *testing.T) {
	tests := []struct {
		name string
		// at is the step the screen opens on; failing has Skill fail; keys are
		// pressed in turn, the last leaving.
		at      dashboard.SetupStep
		failing bool
		keys    []string
		want    Left
	}{
		{name: "q mid-way: paused", at: dashboard.SetupPriming, keys: []string{"q"}, want: Left{Ending: Paused}},
		{name: "ctrl+c mid-way: paused", at: dashboard.SetupService, keys: []string{"ctrl+c"}, want: Left{Ending: Paused}},
		{name: "q at Skill, done: paused", at: dashboard.SetupSkill, keys: []string{"q"}, want: Left{Ending: Paused}},
		{name: "q at Done: finished, without the dashboard", at: dashboard.SetupSkill, keys: []string{"enter", "q"}, want: Left{Ending: Finished}},
		{name: "enter at Done: to the dashboard", at: dashboard.SetupSkill, keys: []string{"enter", "enter"}, want: Left{Ending: ToDashboard}},
		{name: "q at a step that failed: failing, carrying its error", at: dashboard.SetupSkill, failing: true, keys: []string{"q"}, want: Left{Ending: Failed, Err: errNoSkill}},
		{name: "ctrl+c at a step that failed: failing", at: dashboard.SetupSkill, failing: true, keys: []string{"ctrl+c"}, want: Left{Ending: Failed, Err: errNoSkill}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.at)
			if tt.failing {
				h.steps.said, h.steps.skillErr = notWritten, errNoSkill
			}
			h.start()
			last := len(tt.keys) - 1
			for _, key := range tt.keys[:last] {
				h.pressAndDeliver(key)
			}
			if !quits(h.press(tt.keys[last])) {
				t.Fatalf("%s didn't quit", tt.keys[last])
			}
			if got := h.model.walk.left(); got != tt.want {
				t.Errorf("setup was left %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTheSkillStepSaysWhatItDid(t *testing.T) {
	tests := []struct {
		name string
		said setup.Said
		err  error
		// fact is what the fact's row says from its mark on, and keys what the
		// key line's right says.
		fact, keys string
	}{
		{name: "installed", said: installed, fact: factRowOf("✓", installed), keys: "enter finish   q quit"},
		{name: "updated", said: updated, fact: factRowOf("✓", updated), keys: "enter finish   q quit"},
		{name: "up to date", said: upToDate, fact: factRowOf("✓", upToDate), keys: "enter finish   q quit"},
		{name: "failed: why, and q quit alone", said: notWritten, err: errNoSkill, fact: factRowOf("✗", notWritten), keys: "q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupSkill)
			h.steps.said, h.steps.skillErr = tt.said, tt.err
			h.start()

			rows := h.rows()
			if got, want := rows[factRow], tt.fact; got != want {
				t.Errorf("the fact's row reads\n%q\nwant\n%q", got, want)
			}
			if got := rows[keyRow]; strings.TrimLeft(got, " ") != tt.keys {
				t.Errorf("the key line reads %q, want %q at its right alone", got, tt.keys)
			}
		})
	}
}

// factRowOf is said's row, as a page draws it: from column 2, mark; from
// column 5, its label; from column 26, its detail.
func factRowOf(mark string, said setup.Said) string {
	return fmt.Sprintf("  %-3s%-21s%s", mark, said.Label, said.Detail)
}

func TestAStepThatFailedStopsSetupThere(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.steps.said, h.steps.skillErr = notWritten, errNoSkill
	h.start()

	if msgs := h.press("enter"); quits(msgs) || len(msgs) > 0 {
		t.Errorf("enter at a step that failed sends %v, want nothing", msgs)
	}
	if got := h.model.page(); got.Step != dashboard.SetupSkill || len(got.Passed) != int(dashboard.SetupSkill) {
		t.Errorf("enter at a step that failed shows %s, %v passed; want it where it stopped", got.Step.Title(), got.Passed)
	}
}

func TestADetailIsMadeSafeToShowAsItArrives(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.steps.said = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "\x1b[2J~/.claude/skills\t switchboard\n"}
	h.start()
	if got, want := h.model.skill.Detail, "[2J~/.claude/skills switchboard"; got != want {
		t.Errorf("the detail held reads %q, want %q: its control characters spaces, each run of spaces one", got, want)
	}
}

func TestTheStatusLineIsBlankTillStandingAnswers(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	var standing []tea.Msg
	for _, msg := range h.run(h.model.Init()) {
		if _, ok := msg.(standingMsg); ok {
			standing = append(standing, msg)
			continue
		}
		h.deliver(msg)
	}
	if got := h.rows()[statusRow]; got != "" {
		t.Errorf("before Standing answers, the status line reads %q, want it blank", got)
	}

	h.deliver(standing...)
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, all working"; got != want {
		t.Errorf("once Standing answers, the status line reads %q, want %q", got, want)
	}
}

func TestTheStatusLineIsAskedAgain(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.start()
	h.steps.standing = setup.RouterStanding{State: setup.RouterDown}
	h.steps.review.Accounts = h.steps.review.Accounts[:1]

	var cmd tea.Cmd
	h.model, cmd = h.model.askAgain()
	h.deliver(h.run(cmd)...)
	if got, want := h.rows()[statusRow], "○ router not running yet  ·  1 account, working"; got != want {
		t.Errorf("asked again, the status line reads %q, want %q", got, want)
	}
	if h.steps.standings != 2 || h.steps.reviews != 2 {
		t.Errorf("Standing was asked %d times and Review %d, want each twice", h.steps.standings, h.steps.reviews)
	}
}

func TestAnAnswerToAnEarlierAskIsDropped(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.start()
	h.model, _ = h.model.askAgain()
	h.model, _ = h.model.askAgain()
	now := setup.RouterStanding{State: setup.RouterUnhealthy, Reason: "no account can take a request"}
	h.deliver(standingMsg{ask: 3, standing: now}, reviewMsg{ask: 3})
	h.deliver(standingMsg{ask: 2, standing: setup.RouterStanding{State: setup.RouterHealthy}}, reviewMsg{ask: 2, review: h.steps.review})

	if got, want := h.rows()[statusRow], "● router up, unhealthy: no account can take a request"; got != want {
		t.Errorf("the status line reads %q, want %q, the latest ask's answers", got, want)
	}
}

func TestStandingFailingAsItsContextEndsIsDropped(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.deliver(standingMsg{ask: 1, err: fmt.Errorf("ask the router: %w", context.Canceled)})
	if got := h.model.status.line(); got != nil {
		t.Errorf("the status line holds %v, want nothing", got)
	}
}

// tallSkill is a skill step's fact of 200 words of six cells, whose page at
// setup's least, 95×25, scrolls 6 rows, 16 a page.
func tallSkill() setup.Said {
	words := make([]string, 200)
	for i := range words {
		words[i] = fmt.Sprintf("w%04d", i+1)
	}
	return setup.Said{Mark: setup.Done, Label: "Installed", Detail: strings.Join(words, " ")}
}

func TestThePageScrollsNoFurtherThanItsEnds(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.steps.said = tallSkill()
	h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
	h.start()
	if got, want := h.model.page().Scrolling(95, 25), (dashboard.Scrolling{Most: 6, Page: 16}); got != want {
		t.Fatalf("the page scrolls %+v, want %+v", got, want)
	}
	steps := []struct {
		name string
		do   func()
		want int
	}{
		{name: "k at the top", do: func() { h.press("k") }, want: 0},
		{name: "j", do: func() { h.press("j") }, want: 1},
		{name: "the wheel down", do: func() { h.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown}) }, want: 4},
		{name: "PgDn, past the end", do: func() { h.press("pgdown") }, want: 6},
		{name: "j at the end", do: func() { h.press("j") }, want: 6},
		{name: "the wheel up", do: func() { h.update(tea.MouseWheelMsg{Button: tea.MouseWheelUp}) }, want: 3},
		{name: "PgUp, past the top", do: func() { h.press("pgup") }, want: 0},
	}
	for _, step := range steps {
		step.do()
		if h.model.scroll != step.want {
			t.Errorf("after %s, the page is scrolled %d, want %d", step.name, h.model.scroll, step.want)
		}
	}
}

func TestBelowSetupsLeastTheScrollIsKept(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.steps.said = tallSkill()
	h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
	h.start()
	h.press("j")
	h.press("j")

	h.update(tea.WindowSizeMsg{Width: 94, Height: 25})
	h.press("k")
	if h.model.scroll != 2 {
		t.Errorf("k below the least leaves the page scrolled %d, want it kept at 2", h.model.scroll)
	}
	h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
	h.press("j")
	if h.model.scroll != 3 {
		t.Errorf("j at the least again scrolls the page to %d, want 3, on from where it was kept", h.model.scroll)
	}
}

func TestReachingAStepShowsItsPageFromTheTop(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.steps.said = tallSkill()
	h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
	h.start()
	h.press("j")
	h.pressAndDeliver("enter")
	if h.model.scroll != 0 {
		t.Errorf("Done's page is scrolled %d, want it from the top", h.model.scroll)
	}
}

func TestTheClockMovesOnEachSecond(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.clock.now = start.Add(400 * time.Millisecond)
	h.start()
	if len(h.timers) != 1 || h.timers[0] != (timer{delay: 650 * time.Millisecond, msg: tickMsg{}}) {
		t.Fatalf("starting arms %+v, want a tick just past the next second", h.timers)
	}

	h.clock.now = start.Add(time.Minute + time.Second)
	h.deliver(tickMsg{})
	if len(h.timers) != 2 || h.timers[1] != (timer{delay: time.Second + tickSlack, msg: tickMsg{}}) {
		t.Errorf("a tick arms %+v, want the next, just past the next second", h.timers[1:])
	}
	if got := h.rows()[titleRow]; !strings.HasSuffix(got, "Tue 27 Oct  17:43:01") {
		t.Errorf("the title row reads %q, want the clock at the time the tick read", got)
	}
}

func TestTheScreenTakesTheTerminalsSize(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	if msgs := h.update(tea.WindowSizeMsg{Width: 120, Height: 30}); len(msgs) > 0 || h.model.size != (Size{Width: 120, Height: 30}) {
		t.Errorf("a resize to 120×30 has the screen at %+v, sending %v; want it at 120×30, sending nothing", h.model.size, msgs)
	}
	msgs := h.update(tea.WindowSizeMsg{Width: 0, Height: 28})
	if h.model.size != (Size{Width: 120, Height: 28}) {
		t.Errorf("a width given as zero has the screen at %+v, want its width kept, 120×28", h.model.size)
	}
	if want := (tea.WindowSizeMsg{Width: 120, Height: 28}); len(msgs) != 1 || msgs[0] != want {
		t.Errorf("a width given as zero sends %v, want the size drawn at passed on, %v", msgs, want)
	}
	if rows := h.rows(); len(rows) != 28 || len([]rune(rows[0])) > 120 {
		t.Errorf("the screen is drawn %d rows tall, its title row %q; want 28 rows, 120 cells wide", len(rows), rows[0])
	}
}

func TestInColourTheScreenIsBlankTillTheTerminalSaysItsBackground(t *testing.T) {
	pair := nord(t)
	h := newHarness(t, dashboard.SetupSkill)
	cfg := h.config(dashboard.SetupSkill)
	cfg.Pair = pair
	h.model = New(t.Context(), cfg)
	t.Cleanup(h.model.jobs.wait)
	h.start()
	if got := h.model.View(); got.Content != "" || got.BackgroundColor != nil {
		t.Fatalf("before the terminal says its background, the screen draws %q on %v, want it blank", got.Content, got.BackgroundColor)
	}
	if !slices.ContainsFunc(h.timers, func(tm timer) bool { return tm == (timer{delay: theme.AnswerWithin, msg: backdrop.Unanswered{}}) }) {
		t.Errorf("starting in colour arms %+v, want the terminal given its time to answer", h.timers)
	}

	h.deliver(backdrop.Unanswered{})
	got := h.model.View()
	if got.Content == "" {
		t.Error("once the terminal has had its time, the screen is blank, want it drawn")
	}
	if want := pair.Dark.Colour(theme.Canvas); got.BackgroundColor != want {
		t.Errorf("the terminal's background is set to %v, want nord's canvas, %v", got.BackgroundColor, want)
	}
}

func TestWithoutColourTheScreenIsDrawnAtOnce(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.start()
	got := h.model.View()
	if got.Content == "" || got.BackgroundColor != nil {
		t.Errorf("without colour, the screen draws %q on %v, want it drawn, no background set", got.Content, got.BackgroundColor)
	}
	if strings.Contains(got.Content, "\x1b[3") || strings.Contains(got.Content, "\x1b[4") {
		t.Errorf("without colour, the screen draws %q, want no colour in it", got.Content)
	}
}

package screen

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// start is when every test's clock starts: a Tuesday, 17:42 an hour east of
// UTC.
var start = time.Date(2026, 10, 27, 17, 42, 0, 0, time.FixedZone("UTC+1", 60*60))

// The skill step's outcomes, as setup says them.
var (
	installed  = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	updated    = setup.Said{Mark: setup.Done, Label: "Updated", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	upToDate   = setup.Said{Mark: setup.Done, Label: "Up to date", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	errNoSkill = errors.New("create the skill's directory: mkdir ~/.claude: not a directory")
	notWritten = setup.Said{Mark: setup.Failed, Label: "Not installed", Detail: errNoSkill.Error()}
	// errNoConfig is why Review fails, where a test has it.
	errNoConfig = errors.New("read the config: the config doesn't parse")
)

// harness drives a model as Bubble Tea would, on a clock the test moves. It
// runs the commands the model returns, and holds each timer the model arms
// until the test fires it.
type harness struct {
	t      *testing.T
	model  Model
	clock  *fakeClock
	steps  *fakeSteps
	timers []timer
}

// timer is one the model armed: msg, due after delay.
type timer struct {
	delay time.Duration
	msg   tea.Msg
}

// newHarness is a model opening on step, without colour, its steps answering
// as fakeSteps does, in a terminal 160 cells wide and 37 tall. It hasn't
// started.
func newHarness(t *testing.T, step dashboard.SetupStep) *harness {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{now: start}, steps: newFakeSteps()}
	h.model = New(t.Context(), h.config(step))
	t.Cleanup(h.model.jobs.wait)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	return h
}

// config is the screen's config the harness's models are made of, opening
// on step.
func (h *harness) config(step dashboard.SetupStep) Config {
	return Config{Steps: h.steps, Step: step, Now: h.clock.Now, After: h.arm, Size: Size{Width: 80, Height: 24}}
}

// arm records a timer instead of starting one.
func (h *harness) arm(d time.Duration, msg tea.Msg) tea.Cmd {
	h.timers = append(h.timers, timer{delay: d, msg: msg})
	return nil
}

// start starts the model, and delivers what its first commands send back.
func (h *harness) start() {
	h.t.Helper()
	h.deliver(h.run(h.model.Init())...)
}

// update gives the model msg, and returns what the commands it returns send
// back, undelivered.
func (h *harness) update(msg tea.Msg) []tea.Msg {
	h.t.Helper()
	next, cmd := h.model.Update(msg)
	h.model = next.(Model)
	return h.run(cmd)
}

// deliver gives the model msgs, and whatever they send back, until they're
// all delivered.
func (h *harness) deliver(msgs ...tea.Msg) {
	h.t.Helper()
	for len(msgs) > 0 {
		msgs = append(msgs[1:], h.update(msgs[0])...)
	}
}

// run runs cmd, and every command in a batch it gives, returning what they
// send back.
func (h *harness) run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, h.run(c)...)
		}
		return msgs
	default:
		return []tea.Msg{msg}
	}
}

// press presses key, returning what the model sends back, undelivered.
func (h *harness) press(key string) []tea.Msg {
	h.t.Helper()
	switch key {
	case "ctrl+c":
		return h.update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	case "enter":
		return h.update(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "pgdown":
		return h.update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	case "pgup":
		return h.update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	return h.update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}

// pressAndDeliver presses key, and delivers what the model sends back.
func (h *harness) pressAndDeliver(key string) {
	h.t.Helper()
	h.deliver(h.press(key)...)
}

// rows are the screen as it's drawn, without colour, each row's text.
func (h *harness) rows() []string {
	h.t.Helper()
	content := h.model.View().Content
	rows := strings.Split(ansi.Strip(content), "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	return rows
}

// quits reports whether msgs have Bubble Tea quit.
func quits(msgs []tea.Msg) bool {
	for _, msg := range msgs {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

// fakeSteps answers as setup's steps do: Standing with standing, Review with
// review, failing with reviewErr where it's set, and Skill with said,
// failing with skillErr where it's set; each counted as it's asked.
type fakeSteps struct {
	standing  setup.RouterStanding
	review    setup.Review
	reviewErr error
	said      setup.Said
	skillErr  error
	standings int
	reviews   int
	skills    int
}

// newFakeSteps is the steps of a router up and healthy, three accounts
// working, and a skill installed.
func newFakeSteps() *fakeSteps {
	working := setup.TokenStanding{State: setup.TokenWorking}
	return &fakeSteps{
		standing: setup.RouterStanding{State: setup.RouterHealthy},
		review: setup.Review{At: start, Accounts: []setup.Reviewed{
			{ID: "work", Label: "Work", Primary: true, Token: working},
			{ID: "personal", Label: "Personal", Token: working},
			{ID: "side", Label: "Side", Token: working},
		}},
		said: installed,
	}
}

func (s *fakeSteps) Standing(context.Context) (setup.RouterStanding, error) {
	s.standings++
	return s.standing, nil
}

func (s *fakeSteps) Review() (setup.Review, error) {
	s.reviews++
	if s.reviewErr != nil {
		return setup.Review{}, s.reviewErr
	}
	return s.review, nil
}

func (s *fakeSteps) Skill() (setup.Said, error) {
	s.skills++
	return s.said, s.skillErr
}

// nord is the pair of the nord theme alone, which paints its canvas.
func nord(t *testing.T) *theme.Pair {
	t.Helper()
	for _, b := range theme.Builtins() {
		if b.Slug == "nord" {
			return &theme.Pair{Light: b, Dark: b}
		}
	}
	t.Fatal("no built-in theme nord")
	return nil
}

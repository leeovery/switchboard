package capture

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setup/screen"
	"github.com/leeovery/switchboard/internal/theme"
)

// setupScreen is a fixture of setup's screen, named after the frame it
// draws, at that frame's size: opening on its step, every step before it
// passed through, its steps answering as steps has them at the moment
// drawn. It's drawn over no world, at the frames' moment.
type setupScreen struct {
	name  string
	size  watch.Size
	step  dashboard.SetupStep
	steps func(now time.Time) setupSteps
}

// setupScreens are setup's screen's fixtures: the Priming page, its day
// read, typed in, as frame 7 has it; and the Skill page, the skill
// installed, as frame 9 has it.
var setupScreens = []setupScreen{
	{name: "setup-priming-160", size: wide(37), step: dashboard.SetupPriming, steps: accountsPassed},
	{name: "setup-skill-160", size: wide(37), step: dashboard.SetupSkill, steps: everyStepPassed},
}

// setupShown is setup's screen as a fixture draws it: the step it opens on,
// and what its steps answer.
type setupShown struct {
	step  dashboard.SetupStep
	steps setupSteps
}

// called is the fixture's name.
func (s setupScreen) called() string {
	return s.name
}

// made is the fixture at the frames' moment, in the frames' theme: it reads
// none of the worlds.
func (s setupScreen) made(func() accountWorlds) Fixture {
	now := localMoment()
	return Fixture{Name: s.name, Size: s.size, now: now, theme: framesTheme(), setup: &setupShown{step: s.step, steps: s.steps(now)}}
}

// settleSetup builds setup's screen of the fixture, through screen.New, on a
// terminal of the given size, and brings it to the fixture's moment: its
// clock standing there, it asks for the status line's parts and runs its
// step's action, each answered, then the terminal says what its background
// is. Its timers never fire, so nothing moves from there but what a key
// does.
func (f Fixture) settleSetup(size watch.Size) screen.Model {
	cfg := screen.Config{
		Steps: f.setup.steps,
		Step:  f.setup.step,
		Now:   func() time.Time { return f.now },
		After: held,
		Size:  screen.Size{Width: size.Width, Height: size.Height},
	}
	if !f.colourless {
		cfg.Pair = &theme.Pair{Light: f.theme, Dark: f.theme}
	}
	m := screen.New(context.Background(), cfg)
	m = deliver(m, run(m.Init())...)
	if !f.colourless {
		m = deliver(m, tea.BackgroundColorMsg{Color: terminalBackground})
	}
	return m
}

// setupSteps are setup's steps as a capture fakes them, answering at once
// as given: they read, run and write nothing.
type setupSteps struct {
	standing setup.RouterStanding
	review   setup.Review
	day      config.Day
	skill    setup.Said
}

// accountsPassed are the steps as they stand once the Accounts have passed,
// at now: no router running yet; three accounts, work the primary, each
// token checked and working, as the Accounts page leaves them; and priming
// over 08:00-23:00.
func accountsPassed(now time.Time) setupSteps {
	working := setup.TokenStanding{State: setup.TokenWorking}
	return setupSteps{
		standing: setup.RouterStanding{State: setup.RouterDown},
		review: setup.Review{At: now, Accounts: []setup.Reviewed{
			{ID: "work", Label: "Work", Primary: true, Token: working},
			{ID: "personal", Label: "Personal", Token: working},
			{ID: "side", Label: "Side", Token: working},
		}},
		day: config.Day{Start: 8 * time.Hour, End: 23 * time.Hour},
	}
}

// everyStepPassed are the steps as they stand once each before the Skill
// has passed, at now: as the Accounts left them, but for a router up and
// healthy; and the skill installed, written from the home as setup writes
// it.
func everyStepPassed(now time.Time) setupSteps {
	steps := accountsPassed(now)
	steps.standing = setup.RouterStanding{State: setup.RouterHealthy}
	steps.skill = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	return steps
}

// Standing is the router as given.
func (s setupSteps) Standing(context.Context) (setup.RouterStanding, error) {
	return s.standing, nil
}

// Review is the accounts as given.
func (s setupSteps) Review() (setup.Review, error) {
	return s.review, nil
}

// Priming is the day as given.
func (s setupSteps) Priming() (config.Day, error) {
	return s.day, nil
}

// SetDay parses the day as setup does, writing nothing.
func (s setupSteps) SetDay(day string) (config.Day, error) {
	return config.ParseDay(day)
}

// Skill is what the skill step did, as given.
func (s setupSteps) Skill() (setup.Said, error) {
	return s.skill, nil
}

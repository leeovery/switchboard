package capture

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setup/screen"
	"github.com/leeovery/switchboard/internal/shell"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/tokens"
)

// setupScreen is a fixture of setup's screen, named after the frame it
// draws, at that frame's size: opening on its step, every step before it
// passed through, its steps answering as steps has them at the moment
// drawn, then its keys pressed. It's drawn over no world, at the frames'
// moment.
type setupScreen struct {
	name  string
	size  watch.Size
	step  dashboard.SetupStep
	steps func(now time.Time) setupSteps
	// keys are pressed once the screen has opened, each step's action
	// answered before the next.
	keys []tea.KeyPressMsg
}

// setupScreens are setup's screen's fixtures: the Accounts page run again,
// each token checked, side chosen, as frame 3 has it; the Priming page, its
// day read, typed in, as frame 7 has it; the Service page, the service
// installed again and its router healthy, as frame 4 has it; the Skill
// page, the skill installed, as frame 9 has it; and Done, every step walked
// through from Priming, the claude link made but not first on PATH, as
// frame 5 has it.
var setupScreens = []setupScreen{
	{name: "setup-review-160", size: wide(37), step: dashboard.SetupAccounts, steps: reviewed, keys: keysOf("j", "j")},
	{name: "setup-priming-160", size: wide(37), step: dashboard.SetupPriming, steps: accountsPassed},
	{name: "setup-service-160", size: wide(37), step: dashboard.SetupService, steps: everyStepPassed},
	{name: "setup-skill-160", size: wide(37), step: dashboard.SetupSkill, steps: everyStepPassed},
	{name: "setup-done-160", size: wide(37), step: dashboard.SetupPriming, steps: linkLeft, keys: repeated(enterKey, 4)},
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

// setupMoment is when setup's screen's fixtures are drawn, in the machine's
// own zone: the moment of setup's signed-off frames, which their clock
// shows and their tokens' years count from.
func setupMoment() time.Time {
	return time.Date(2026, time.October, 27, 17, 42, 0, 0, time.Local)
}

// made is the fixture at setupMoment, in the frames' theme: it reads none of
// the worlds.
func (s setupScreen) made(func() accountWorlds) Fixture {
	now := setupMoment()
	return Fixture{Name: s.name, Size: s.size, now: now, theme: framesTheme(), keys: s.keys, setup: &setupShown{step: s.step, steps: s.steps(now)}}
}

// settleSetup builds setup's screen of the fixture, through screen.New, on a
// terminal of the given size, and brings it to the fixture's moment: its
// clock standing there, it asks for the status line's parts, checks each
// account's token and runs its step's action, each answered, then the
// terminal says what its background is, and the fixture's keys are pressed.
// Its timers never fire, so nothing moves from there but what a key does.
func (f Fixture) settleSetup(size watch.Size) screen.Model {
	cfg := screen.Config{
		Steps: f.setup.steps,
		Step:  f.setup.step,
		Now:   func() time.Time { return f.now },
		After: held,
		Size:  screen.Size{Width: size.Width, Height: size.Height},
		Link:  f.setup.steps.link,
	}
	if !f.colourless {
		cfg.Pair = &theme.Pair{Light: f.theme, Dark: f.theme}
	}
	m := screen.New(context.Background(), cfg)
	m = deliver(m, run(m.Init())...)
	if !f.colourless {
		m = deliver(m, tea.BackgroundColorMsg{Color: terminalBackground})
	}
	for _, key := range f.keys {
		m = deliver(m, tea.Msg(key))
	}
	return m
}

// setupSteps are setup's steps as a capture fakes them, answering at once
// as given: they read, run and write nothing. Review reads each token, and
// ReviewOf checks it as checked has it. link is what the claude link step
// found, which the screen is given, as it doesn't take the step itself yet.
type setupSteps struct {
	standing setup.RouterStanding
	review   setup.Review
	checked  map[string]setup.TokenStanding
	day      config.Day
	service  setup.Serviced
	skill    setup.Said
	link     *setup.LinkFound
}

// read is a token read but not yet checked, its year as given.
func read(year status.TokenYear) setup.TokenStanding {
	return setup.TokenStanding{State: setup.TokenRead, Year: year}
}

// reviewed are the steps as they stand on a run again, at now, as frame 3
// has them: a router up and healthy; work, the primary, on Max 20x, 5%
// reserved, its token working, its year unknown; personal, its plan not
// set, its token working, 21 days left of its year; and side, on Pro, its
// token refused.
func reviewed(now time.Time) setupSteps {
	until := now.AddDate(0, 0, 21)
	year := status.TokenYear{Taken: until.AddDate(-1, 0, 0), GoodUntil: until}
	return setupSteps{
		standing: setup.RouterStanding{State: setup.RouterHealthy},
		review: setup.Review{At: now, Accounts: []setup.Reviewed{
			{ID: "work", Label: "Work", Plan: "max20x", Reserve: 0.05, Primary: true, Token: read(status.TokenYear{})},
			{ID: "personal", Label: "Personal", Token: read(year)},
			{ID: "side", Label: "Side", Plan: "pro", Token: read(status.TokenYear{})},
		}},
		checked: map[string]setup.TokenStanding{
			"work":     {State: setup.TokenWorking},
			"personal": {State: setup.TokenWorking, Year: year},
			"side":     {State: setup.TokenNotTaken},
		},
	}
}

// accountsPassed are the steps as they stand once the Accounts have passed,
// at now: no router running yet; three accounts, work the primary with 5%
// reserved, each token working once checked; and priming over 08:00-23:00.
func accountsPassed(now time.Time) setupSteps {
	working := setup.TokenStanding{State: setup.TokenWorking}
	return setupSteps{
		standing: setup.RouterStanding{State: setup.RouterDown},
		review: setup.Review{At: now, Accounts: []setup.Reviewed{
			{ID: "work", Label: "Work", Primary: true, Reserve: 0.05, Token: read(status.TokenYear{})},
			{ID: "personal", Label: "Personal", Token: read(status.TokenYear{})},
			{ID: "side", Label: "Side", Token: read(status.TokenYear{})},
		}},
		checked: map[string]setup.TokenStanding{"work": working, "personal": working, "side": working},
		day:     config.Day{Start: 8 * time.Hour, End: 23 * time.Hour},
	}
}

// everyStepPassed are the steps as they stand once each before the Skill
// has passed, at now: as the Accounts left them, but for a router up and
// healthy; the service installed again, its plist out of date, and its
// router healthy, as frame 4 says it in the words setup says; and the skill
// installed; each place written from the home as setup writes it.
func everyStepPassed(now time.Time) setupSteps {
	steps := accountsPassed(now)
	steps.standing = setup.RouterStanding{State: setup.RouterHealthy}
	steps.service = setup.Serviced{
		Said: []setup.Said{
			{Mark: setup.Done, Label: "Installed", Detail: "~/Library/LaunchAgents/io.github.leeovery.switchboard.plist"},
			{Mark: setup.Warning, Label: "Updated", Detail: "reinstalled: its plist had no ProcessType · kept its --log-level"},
			{Mark: setup.Done, Label: "Router", Detail: "healthy · 127.0.0.1:4747 · v0.2.10"},
		},
		Router: &router.Health{OK: true, Listen: "127.0.0.1:4747", Version: "0.2.10"},
	}
	steps.skill = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	return steps
}

// linkLeft are the steps as they stand once each has passed, at now: as
// everyStepPassed has them, but for the skill up to date; and the claude
// link made, but zsh running Claude Code's own claude, the line setup
// offered left to add to ~/.zshrc by hand, as frame 5 has it.
func linkLeft(now time.Time) setupSteps {
	steps := everyStepPassed(now)
	steps.skill = setup.Said{Mark: setup.Done, Label: "Up to date", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	const home = "/home/user"
	steps.link = &setup.LinkFound{
		Made: setup.LinkCreated,
		Said: []setup.Said{{Mark: setup.Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}},
		Shell: setup.ShellFound{
			Situation: setup.OfferLine,
			Said:      setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "runs Claude Code's own claude, ~/.local/bin/claude, so its sessions aren't routed"},
			File:      home + "/.zshrc",
			Where:     "~/.zshrc",
			Line:      shell.Line(shell.Zsh, shell.InitFile(home+"/.local/share/switchboard/shell", shell.Zsh), home),
		},
	}
	return steps
}

// Standing is the router as given.
func (s setupSteps) Standing(context.Context) (setup.RouterStanding, error) {
	return s.standing, nil
}

// Review is the accounts as given, each token read, as testToken has it.
func (s setupSteps) Review() (setup.Review, error) {
	review := s.review
	review.Accounts = slices.Clone(review.Accounts)
	for i, a := range review.Accounts {
		review.Accounts[i].Token = a.Token.Of(testToken(a.ID))
	}
	return review, nil
}

// ReviewOf is the account's row as given, its token, as testToken has it,
// as checked has it.
func (s setupSteps) ReviewOf(_ context.Context, id string) (setup.Reviewed, error) {
	i := slices.IndexFunc(s.review.Accounts, func(a setup.Reviewed) bool { return a.ID == id })
	if i < 0 {
		return setup.Reviewed{}, config.NotConfigured(id)
	}
	row := s.review.Accounts[i]
	row.Token = s.checked[id].Of(testToken(id))
	return row, nil
}

// testToken is the token the account's file holds, as capture fakes it: a
// placeholder of its own, never shown.
func testToken(id string) tokens.Token {
	token, _ := tokens.Parse("test-token-" + id)
	return token
}

// Priming is the day as given.
func (s setupSteps) Priming() (config.Day, error) {
	return s.day, nil
}

// SetDay parses the day as setup does, writing nothing.
func (s setupSteps) SetDay(day string) (config.Day, error) {
	return config.ParseDay(day)
}

// Service is what the service step did, as given.
func (s setupSteps) Service(context.Context) (setup.Serviced, error) {
	return s.service, nil
}

// Skill is what the skill step did, as given.
func (s setupSteps) Skill() (setup.Said, error) {
	return s.skill, nil
}

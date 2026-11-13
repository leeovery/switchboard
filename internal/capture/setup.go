package capture

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setup/screen"
	"github.com/leeovery/switchboard/internal/setuptoken"
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
	// answered before the next, waited before the moment drawn, as a card
	// counts how long it's waited.
	keys   []tea.KeyPressMsg
	waited time.Duration
	// card is the title of the card the keys leave open over the page: ""
	// where they leave none; and cardKeys are its keys, as its keys row
	// reads them.
	card, cardKeys string
}

// setupScreens are setup's screen's fixtures: the Accounts page on a first
// run, no router running and no account, its add card open as it opens at
// once, work's ID and Label typed and Ultra chosen, then tab round to
// Label again, as frame 1 has it; the Accounts page run again, each token
// checked, side chosen, as frame 3 has it; the same, personal
// chosen, its edit card open, its Plan chosen and moved to Max, as frame
// 6 has it; the Priming page, its day read, typed in, as frame 7 has it;
// the Service page, the service installed again and its router healthy, as
// frame 4 has it; the claude link page, zsh running Claude Code's own
// claude, the card offering to add the line to ~/.zshrc open, as frame 8 has
// it, or the offer declined with n, the line shown by hand, as frame 11 has
// it, or zsh running switchboard's, the session hook installed, as frame 12
// has it; the Skill page, the skill installed, as frame 9 has it; Done,
// every step walked through from Priming, the claude link made but not
// first on PATH, the offer declined, as frame 5 has it, or the same, every
// account on Ultra, Priming skipped as it's reached; and on a first run,
// work added, its token made, then personal, its token being made in a
// private Firefox window, setup-token waiting on its approval for 21
// seconds, as frame 2 has it, or stopped with an error, as frame 10 has it;
// and from there, p pressed, the card pasting personal's token, a token
// typed in, as no frame has it, laid out as frame 2; and on a first run,
// work, personal and side added, each token made, none the primary, then p
// on work's row, as a user must, and enter, the primary's reserve card open,
// 5% typed, as frame 13 has it; and the run again with a second pool, Acme,
// its add card adding Acme 3 to Acme, as the pools' frame 5 has it, or Client
// 1 to a new pool, Client, its new pool card open, as frame 6 has it, or
// Acme 2's edit card open, its Pool chosen, as frame 7 has it.
var setupScreens = []setupScreen{
	{name: "setup-add-first-160", size: wide(37), step: dashboard.SetupAccounts, steps: firstRun, keys: slices.Concat(typingWork(), []tea.KeyPressMsg{tabKey}), card: "ADD AN ACCOUNT", cardKeys: "tab next   enter create token   esc cancel"},
	{name: "setup-token-waiting-160", size: wide(37), step: dashboard.SetupAccounts, steps: makingPersonal(nil), keys: addingPersonal(), waited: 21 * time.Second, card: "NEW TOKEN · PERSONAL", cardKeys: "o reopen window   p paste a token   esc cancel"},
	{name: "setup-token-failed-160", size: wide(37), step: dashboard.SetupAccounts, steps: makingPersonal(&personalNotMade), keys: addingPersonal(), card: "NEW TOKEN · PERSONAL", cardKeys: "enter try again   p paste a token   esc cancel"},
	{name: "setup-token-paste-160", size: wide(37), step: dashboard.SetupAccounts, steps: makingPersonal(&personalNotMade), keys: slices.Concat(addingPersonal(), keysOf("p"), keysOf(strings.Split("test-token-personal", "")...)), card: "PASTE A TOKEN · PERSONAL", cardKeys: "enter save   esc cancel"},
	{name: "setup-review-160", size: wide(37), step: dashboard.SetupAccounts, steps: reviewed, keys: keysOf("j", "j")},
	{name: "setup-edit-160", size: wide(37), step: dashboard.SetupAccounts, steps: reviewed, keys: slices.Concat(keysOf("j", "e"), []tea.KeyPressMsg{tabKey, leftKey, leftKey}), card: "EDIT PERSONAL", cardKeys: "tab next   ←→ select   enter save   esc cancel"},
	{name: "setup-add-160", size: wide(37), step: dashboard.SetupAccounts, steps: pooledReview, keys: addingAcme3(), card: "ADD AN ACCOUNT", cardKeys: "tab next   enter create token   esc cancel"},
	{name: "setup-new-pool-160", size: wide(37), step: dashboard.SetupAccounts, steps: pooledReview, keys: makingClient(), card: "NEW POOL", cardKeys: "tab next   enter create pool   esc back"},
	{name: "setup-edit-pool-160", size: wide(37), step: dashboard.SetupAccounts, steps: pooledReview, keys: slices.Concat(repeated(downKey, 4), keysOf("e"), repeated(tabKey, 2)), card: "EDIT ACME 2", cardKeys: "tab next   ←→ select   enter save   esc cancel"},
	{name: "setup-reserve-160", size: wide(37), step: dashboard.SetupAccounts, steps: addingThree, keys: slices.Concat(addedThree(), repeated(upKey, 2), keysOf("p"), []tea.KeyPressMsg{enterKey}, keysOf("5", "%")), card: "RESERVE · WORK", cardKeys: "enter save   esc no reserve"},
	{name: "setup-priming-160", size: wide(37), step: dashboard.SetupPriming, steps: accountsPassed},
	{name: "setup-service-160", size: wide(37), step: dashboard.SetupService, steps: everyStepPassed},
	{name: "setup-skill-160", size: wide(37), step: dashboard.SetupSkill, steps: everyStepPassed},
	{name: "setup-link-offer-160", size: wide(37), step: dashboard.SetupLink, steps: linkLeft, card: "ADD A LINE TO ~/.zshrc", cardKeys: "enter add it   n I'll add it myself"},
	{name: "setup-link-by-hand-160", size: wide(37), step: dashboard.SetupLink, steps: linkLeft, keys: keysOf("n")},
	{name: "setup-link-done-160", size: wide(37), step: dashboard.SetupLink, steps: linkThrough},
	{name: "setup-done-160", size: wide(37), step: dashboard.SetupPriming, steps: linkLeft, keys: slices.Concat(repeated(enterKey, 2), keysOf("n"), repeated(enterKey, 3))},
	{name: "setup-priming-skipped-160", size: wide(37), step: dashboard.SetupPriming, steps: everyAccountUltra, keys: slices.Concat(repeated(enterKey, 1), keysOf("n"), repeated(enterKey, 3))},
}

// setupShown is setup's screen as a fixture draws it: the step it opens on;
// what its steps answer; how long before its moment its keys are pressed;
// and the title of the card left open over its page, "" for none, and its
// keys.
type setupShown struct {
	step           dashboard.SetupStep
	steps          setupSteps
	waited         time.Duration
	card, cardKeys string
}

// typingWork are the keys that type work into the add card, as frame 1 has
// it: its Label, then Ultra chosen.
func typingWork() []tea.KeyPressMsg {
	return slices.Concat(keysOf("W", "o", "r", "k"), []tea.KeyPressMsg{tabKey}, repeated(rightKey, 2))
}

// addingAcme3 are the keys that open the add card over the review by pool,
// from + add an account, type Acme 3 into it, choose Ultra, and choose Acme
// as its pool, as the pools' frame 5 has it.
func addingAcme3() []tea.KeyPressMsg {
	return slices.Concat(
		repeated(downKey, 5), []tea.KeyPressMsg{enterKey},
		keysOf(strings.Split("Acme 3", "")...), []tea.KeyPressMsg{tabKey}, repeated(rightKey, 2),
		[]tea.KeyPressMsg{tabKey, rightKey},
	)
}

// makingClient are the keys that open the add card over the review by pool,
// type Client 1 into it, choose Ultra and new pool, and open the new pool
// card on enter, Client typed as its label and ~/Code/client as its
// directory, as the pools' frame 6 has it.
func makingClient() []tea.KeyPressMsg {
	return slices.Concat(
		repeated(downKey, 5), []tea.KeyPressMsg{enterKey},
		keysOf(strings.Split("Client 1", "")...), []tea.KeyPressMsg{tabKey}, repeated(rightKey, 2),
		[]tea.KeyPressMsg{tabKey}, repeated(rightKey, 2), []tea.KeyPressMsg{enterKey},
		keysOf(strings.Split("Client", "")...), []tea.KeyPressMsg{tabKey},
		keysOf(strings.Split("~/Code/client", "")...),
	)
}

// addingPersonal are the keys that add work on a first run, as typingWork
// types it, its token made, then add personal, its Label typed, from + add
// an account, to have its token made.
func addingPersonal() []tea.KeyPressMsg {
	return slices.Concat(
		typingWork(), []tea.KeyPressMsg{enterKey},
		[]tea.KeyPressMsg{downKey, enterKey},
		keysOf(strings.Split("Personal", "")...), []tea.KeyPressMsg{enterKey},
	)
}

// addedThree are the keys that add work on a first run, as typingWork types
// it, then personal on Max and side on Pro, from + add an account, each
// token made, leaving side's row chosen.
func addedThree() []tea.KeyPressMsg {
	adding := func(label string, rights int) []tea.KeyPressMsg {
		return slices.Concat(
			[]tea.KeyPressMsg{downKey, enterKey},
			keysOf(strings.Split(label, "")...),
			[]tea.KeyPressMsg{tabKey}, repeated(rightKey, rights), []tea.KeyPressMsg{enterKey},
		)
	}
	return slices.Concat(typingWork(), []tea.KeyPressMsg{enterKey}, adding("Personal", 1), adding("Side", 0))
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
	return Fixture{Name: s.name, Size: s.size, now: now, theme: framesTheme(), keys: s.keys, setup: &setupShown{step: s.step, steps: s.steps(now), waited: s.waited, card: s.card, cardKeys: s.cardKeys}}
}

// settleSetup builds setup's screen of the fixture, through screen.New, on a
// terminal of the given size, and brings it to the fixture's moment: its
// clock standing as long before it as its keys wait, it asks for the status
// line's parts, checks each account's token and runs its step's action,
// each answered, then the terminal says what its background is, and the
// fixture's keys are pressed, each token's making heard as far as it tells,
// as told counts it; then the clock reads the moment, which the screen is
// drawn at. Its timers never fire, so nothing moves from there but what a
// key does.
func (f Fixture) settleSetup(size watch.Size) screen.Model {
	steps := f.setup.steps
	steps.saved = &setupSaved{}
	untold := steps.told()
	clock := &clock{now: ago(f.now, f.setup.waited)}
	cfg := screen.Config{
		Steps: steps,
		Step:  f.setup.step,
		Now:   clock.read,
		After: held,
		Await: func(wait tea.Cmd) tea.Cmd {
			if untold == 0 {
				return nil
			}
			untold--
			return wait
		},
		Size: screen.Size{Width: size.Width, Height: size.Height},
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
	clock.now = f.now
	return m
}

// setupSteps are setup's steps as a capture fakes them, answering at once
// as given: they read, run and write nothing. Review reads each account, as
// the config marks it, and each token, and ReviewOf checks it as checked
// has it, each account saved among them.
// makings are how each account's token is made, by the name the token's
// cards give it: its label, where it's added. link is what the claude link
// step finds, asked first or again, and hook what the session hook's
// install says.
type setupSteps struct {
	standing setup.RouterStanding
	review   setup.Review
	checked  map[string]setup.TokenStanding
	day      config.Day
	service  setup.Serviced
	skill    setup.Said
	link     setup.LinkFound
	hook     setup.Said
	makings  map[string]setupMaking
	// saved are the accounts added once their tokens are saved, a settle's
	// own, as settleSetup gives it.
	saved *setupSaved
}

// setupMaking is how a capture makes an account's token: it reports each of
// steps in turn, then ends as ends has it; or with no end, holds there, as
// setup-token waits on the approval.
type setupMaking struct {
	steps []setup.Making
	ends  *setup.Tokened
}

// told is how many things the steps' makings tell the screen as they're
// made: each step, and how each ends, where it does, which settleSetup
// hears in turn, a making's steps before how it ends, as the screen hears
// them, then holds its listening, as a making held tells nothing more.
func (s setupSteps) told() int {
	n := 0
	for _, making := range s.makings {
		n += len(making.steps)
		if making.ends != nil {
			n++
		}
	}
	return n
}

// setupSaved are the accounts a capture's steps have added, their tokens
// made and saved, each a year from then, as setup saves it; and the account
// marked the primary since, in every other's place: "" where none is.
type setupSaved struct {
	mu       sync.Mutex
	accounts []setup.Reviewed
	primary  string
}

// add adds the account f adds, under the id given, its token saved at the
// time given, marked the primary only where f asks it to be, as config adds
// it.
func (s *setupSaved) add(f setup.TokenFor, id string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := f.Account
	s.accounts = append(s.accounts, setup.Reviewed{ID: id, Label: a.Label, Plan: a.Plan, Pool: a.Pool, Token: read(status.TokenYearFrom(at))})
	if a.Primary {
		s.primary = id
	}
}

// mark marks the account the primary, in every other's place.
func (s *setupSaved) mark(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.primary = id
}

// over is the accounts given, then those added, each marked the primary as
// the config writes it: the one marked since, where one is, else as given.
// There are none added where the steps have none saved, as outside a
// settle.
func (s *setupSaved) over(given []setup.Reviewed) []setup.Reviewed {
	if s == nil {
		return slices.Clone(given)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	accounts := slices.Concat(given, s.accounts)
	if s.primary != "" {
		for i := range accounts {
			accounts[i].Primary = accounts[i].ID == s.primary
		}
	}
	return accounts
}

// makingPersonal are the steps of a first run, at now, as frames 2 and 10
// have them: no router running, and no account, at first; work's token made
// in the browser as it is, and saved; then personal's, its sign-in address
// opened in a private Firefox window, the browser signed in to work,
// setup-token waiting on its approval, then ending as ends has it, or with
// no end, held there.
func makingPersonal(ends *setup.Tokened) func(time.Time) setupSteps {
	return func(now time.Time) setupSteps {
		steps := firstRun(now)
		steps.makings = map[string]setupMaking{
			"Work": {
				steps: []setup.Making{
					{Step: setup.AddressReady, Opened: setuptoken.OpenedInBrowser, For: "Work"},
					{Step: setup.AwaitingApproval}, {Step: setup.TokenMade}, {Step: setup.CheckingToken},
				},
				ends: &setup.Tokened{Result: setup.TokenSaved},
			},
			"Personal": {
				steps: []setup.Making{
					{Step: setup.AddressReady, Opened: setuptoken.OpenedPrivately, Browser: "Firefox", For: "Personal", SignedIn: "Work"},
					{Step: setup.AwaitingApproval},
				},
				ends: ends,
			},
		}
		return steps
	}
}

// addingThree are the steps of a first run, at now, as frame 13 has them:
// no router running, and no account, at first; then work's token, personal's
// and side's, each made in the browser as it is, and saved.
func addingThree(now time.Time) setupSteps {
	steps := firstRun(now)
	steps.makings = map[string]setupMaking{}
	for _, label := range []string{"Work", "Personal", "Side"} {
		steps.makings[label] = setupMaking{
			steps: []setup.Making{
				{Step: setup.AddressReady, Opened: setuptoken.OpenedInBrowser, For: label},
				{Step: setup.AwaitingApproval}, {Step: setup.TokenMade}, {Step: setup.CheckingToken},
			},
			ends: &setup.Tokened{Result: setup.TokenSaved},
		}
	}
	return steps
}

// personalNotMade is setup-token stopping with an error making personal's
// token, as frame 10 has it, what it showed last set in from its screen's
// edge, as setup-token sets it.
var personalNotMade = setup.Tokened{
	Result: setup.TokenNotMade,
	Why:    "setup-token showed an error",
	Shown:  []string{" OAuth error: Request failed with status code 400", " Press Enter to retry."},
}

// read is a token read but not yet checked, its year as given.
func read(year status.TokenYear) setup.TokenStanding {
	return setup.TokenStanding{State: setup.TokenRead, Year: year}
}

// firstRun are the steps as they stand on a first run, at now, as frame 1
// has them: no router running, and no account.
func firstRun(now time.Time) setupSteps {
	return setupSteps{standing: setup.RouterStanding{State: setup.RouterDown}, review: setup.Review{At: now}}
}

// reviewed are the steps as they stand on a run again, at now, as frame 3
// has them: a router up and healthy; work, the primary, on Ultra, 5%
// reserved, its token working, its year unknown; personal, its plan not
// set, its token working, 21 days left of its year; and side, on Pro, its
// token refused.
func reviewed(now time.Time) setupSteps {
	until := now.AddDate(0, 0, 21)
	year := status.TokenYear{Taken: until.AddDate(-1, 0, 0), GoodUntil: until}
	return setupSteps{
		standing: setup.RouterStanding{State: setup.RouterHealthy},
		review: setup.Review{At: now, Accounts: []setup.Reviewed{
			{ID: "work", Label: "Work", Plan: config.PlanUltra, Reserve: 0.05, Primary: true, Token: read(status.TokenYear{})},
			{ID: "personal", Label: "Personal", Token: read(year)},
			{ID: "side", Label: "Side", Plan: config.PlanPro, Token: read(status.TokenYear{})},
		}},
		checked: map[string]setup.TokenStanding{
			"work":     {State: setup.TokenWorking},
			"personal": {State: setup.TokenWorking, Year: year},
			"side":     {State: setup.TokenNotTaken},
		},
	}
}

// pooledReview are the steps as reviewed has them, at now, but for a second
// pool, as the pools' frames 5 to 7 have it: Default, unfenced, with work,
// personal and side; and Acme, fenced to ~/Code/acme, with Acme 1, its
// primary, and Acme 2, each on Ultra, each token working, its year unknown.
func pooledReview(now time.Time) setupSteps {
	steps := reviewed(now)
	const home = "/home/user"
	steps.review.Pools = config.Pools{
		{ID: "b2n8", Label: "Default"},
		{ID: "q7mt", Label: "Acme", Dirs: []string{home + "/Code/acme"}, WrittenDirs: []string{"~/Code/acme"}},
	}
	accounts := slices.Clone(steps.review.Accounts)
	for i := range accounts {
		accounts[i].Pool = "b2n8"
	}
	steps.review.Accounts = append(accounts,
		setup.Reviewed{ID: "acme1", Label: "Acme 1", Plan: config.PlanUltra, Primary: true, Pool: "q7mt", Token: read(status.TokenYear{})},
		setup.Reviewed{ID: "acme2", Label: "Acme 2", Plan: config.PlanUltra, Pool: "q7mt", Token: read(status.TokenYear{})},
	)
	steps.checked = maps.Clone(steps.checked)
	steps.checked["acme1"] = setup.TokenStanding{State: setup.TokenWorking}
	steps.checked["acme2"] = setup.TokenStanding{State: setup.TokenWorking}
	return steps
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
// router healthy, as frame 4 says it, each row as setup words it; and the
// skill installed; each place written from the home as setup writes it.
func everyStepPassed(now time.Time) setupSteps {
	steps := accountsPassed(now)
	steps.standing = setup.RouterStanding{State: setup.RouterHealthy}
	health := router.Health{OK: true, Listen: "127.0.0.1:4747", Version: "0.2.10"}
	steps.service = setup.Serviced{
		Said: []setup.Said{
			setup.ServiceInstalled(plistFromHome()),
			setup.Reinstalled(setup.PlistHad([]string{"no ProcessType"}), "--log-level"),
			setup.RouterUp(health),
		},
		Router: &health,
	}
	steps.skill = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	return steps
}

// plistFromHome is where the service's plist goes, from the home, as setup
// writes it: the service's own path, its home ~.
func plistFromHome() string {
	svc, err := service.New(service.Config{GOOS: "darwin", Home: "~"})
	if err != nil {
		panic(err)
	}
	return svc.Plist()
}

// linkLeft are the steps as they stand once each has passed, at now: as
// everyStepPassed has them, but for the skill up to date; and the claude
// link made, but zsh running Claude Code's own claude, setup offering to
// add its line to ~/.zshrc, as frame 8 has it, shown by hand once declined,
// as frames 5 and 11 have it, and the session hook installed once the
// step's left.
func linkLeft(now time.Time) setupSteps {
	steps := everyStepPassed(now)
	steps.skill = setup.Said{Mark: setup.Done, Label: "Up to date", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	const home = "/home/user"
	steps.link = setup.LinkFound{
		Made: setup.LinkCreated,
		Said: []setup.Said{linkMade},
		Shell: setup.ShellFound{
			Situation: setup.OfferLine,
			Said:      setup.Said{Mark: setup.Warning, Label: "Your shell", Detail: "runs Claude Code's own claude, ~/.local/bin/claude, so its sessions aren't routed"},
			File:      home + "/.zshrc",
			Where:     "~/.zshrc",
			Line:      shell.Line(shell.Zsh, shell.InitFile(home+"/.local/share/switchboard/shell", shell.Zsh), home),
			Dir:       "$HOME/.local/share/switchboard/bin",
		},
	}
	steps.hook = hookInstalled
	return steps
}

// everyAccountUltra are the steps as linkLeft has them, at now, but for
// every account on Ultra, so the Priming step's skipped, its day set still.
func everyAccountUltra(now time.Time) setupSteps {
	steps := linkLeft(now)
	steps.review.Accounts = slices.Clone(steps.review.Accounts)
	for i := range steps.review.Accounts {
		steps.review.Accounts[i].Plan = config.PlanUltra
	}
	return steps
}

// linkMade is the claude link's row, made in switchboard's bin directory,
// as setup says it.
var linkMade = setup.Said{Mark: setup.Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}

// linkThrough are the steps as they stand once the Service has passed, at
// now: as everyStepPassed has them; and the claude link made, zsh running
// it, and the session hook installed, as frame 12 has it, in the words setup
// says.
func linkThrough(now time.Time) setupSteps {
	steps := everyStepPassed(now)
	steps.link = setup.LinkFound{
		Made: setup.LinkCreated,
		Said: []setup.Said{linkMade},
		Shell: setup.ShellFound{
			Situation: setup.Through,
			Said:      setup.Said{Mark: setup.Done, Label: "Your shell", Detail: "runs switchboard's claude: asked just now, as a new terminal starts it"},
		},
	}
	steps.hook = hookInstalled
	return steps
}

// hookInstalled is the session hook installed, as setup says it.
var hookInstalled = setup.Said{Mark: setup.Done, Label: "Session hook", Detail: "Claude Code says so when a session doesn't start through switchboard · ~/.claude/settings.json"}

// Standing is the router as given.
func (s setupSteps) Standing(context.Context) (setup.RouterStanding, error) {
	return s.standing, nil
}

// Review is the accounts as given, then those saved, each marked the
// primary as saved has it, as setup resolves them, each token read, as
// testToken has it.
func (s setupSteps) Review() (setup.Review, error) {
	review := s.review
	review.Accounts = s.saved.over(review.Accounts)
	review = review.Resolved()
	for i, a := range review.Accounts {
		review.Accounts[i].Token = a.Token.Of(testToken(a.ID))
	}
	return review, nil
}

// ReviewOf is the account's row as Review has it, its token, as testToken
// has it: one given as checked has it, the zero standing where it has none,
// so a fixture that leaves one out shows it; one saved, working.
func (s setupSteps) ReviewOf(_ context.Context, id string) (setup.Reviewed, error) {
	review, _ := s.Review()
	i := slices.IndexFunc(review.Accounts, func(a setup.Reviewed) bool { return a.ID == id })
	if i < 0 {
		return setup.Reviewed{}, config.NotConfigured(id)
	}
	row := review.Accounts[i]
	checked := s.checked[id]
	if i >= len(s.review.Accounts) {
		checked = setup.TokenStanding{State: setup.TokenWorking, Year: row.Token.Year}
	}
	row.Token = checked.Of(testToken(id))
	return row, nil
}

// testToken is the token the account's file holds, as capture fakes it: a
// placeholder of its own, never shown.
func testToken(id string) tokens.Token {
	token, _ := tokens.Parse("test-token-" + id)
	return token
}

// Priming is the day as given, the step skipped as setup skips it for the
// accounts as Review has them.
func (s setupSteps) Priming() (setup.Primed, error) {
	review, err := s.Review()
	if err != nil {
		return setup.Primed{}, err
	}
	return setup.Primed{Day: s.day, Skipped: review.SkipsPriming()}, nil
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

// Link is what the claude link step found, as given: it makes no link,
// writes no file and asks no shell.
func (s setupSteps) Link(context.Context) (setup.LinkFound, error) {
	return s.link, nil
}

// AskAgain is what the claude link step found, as given, as no fixture asks
// the shell again.
func (s setupSteps) AskAgain(context.Context, setup.LinkFound) (setup.LinkFound, error) {
	return s.link, nil
}

// AddLine adds nothing, as no fixture adds the line: it gives found back as
// it is, writing no startup file.
func (s setupSteps) AddLine(found setup.LinkFound) (setup.LinkFound, error) {
	return found, nil
}

// SessionHook is what the session hook's install said, as given: it writes
// no settings.
func (s setupSteps) SessionHook() setup.Said {
	return s.hook
}

// CheckAdding gives the account as setup does, its plan read as
// config.ParsePlan reads it, in the pool chosen, or with the new pool made
// for it, refusing nothing.
func (s setupSteps) CheckAdding(adding setup.Adding) (accounts.Adding, error) {
	plan, err := config.ParsePlan(adding.Plan)
	account := config.NewAccount{Label: adding.Label, Plan: plan, Pool: adding.Pool}
	return accounts.Adding{Account: account, NewPool: adding.NewPool}, err
}

// CheckPool refuses no pool, as no fixture's is refused.
func (s setupSteps) CheckPool(accounts.NewPool) error {
	return nil
}

// MakeToken makes the account's token as makings has it: it reports each
// step in turn, then ends as given, an account added once its token's
// saved, its id its label in lower case, as the fixtures' ids are. One with
// no end holds there, as setup-token waits on the approval: settleSetup's
// listening is held from then, so it ends at once, cancelled, never heard,
// leaving nothing running.
func (s setupSteps) MakeToken(_ context.Context, f setup.TokenFor, report func(setup.Making)) (setup.Tokened, error) {
	making := s.makings[f.Name()]
	for _, at := range making.steps {
		report(at)
	}
	if making.ends == nil {
		return setup.Tokened{Result: setup.TokenCancelled}, nil
	}
	ended := *making.ends
	if ended.Result == setup.TokenSaved && f.Add {
		ended.ID = strings.ToLower(f.Account.Label)
		s.saved.add(f, ended.ID, s.review.At)
	}
	return ended, nil
}

// Answer saves nothing, as no fixture asks which account a token is for: it
// answers as a save does whose context ends first.
func (s setupSteps) Answer(context.Context, *setup.Asked, accounts.Answer) (setup.Tokened, error) {
	return setup.Tokened{Result: setup.TokenCancelled}, nil
}

// PasteToken saves nothing, as no fixture saves a pasted token: it answers
// as a paste does whose context ends before it's saved.
func (s setupSteps) PasteToken(context.Context, setup.TokenFor, string) (setup.Tokened, error) {
	return setup.Tokened{Result: setup.TokenCancelled}, nil
}

// Reopen opens nothing, as a capture never opens a browser.
func (s setupSteps) Reopen(context.Context, setup.Making) error {
	return nil
}

// EditAccount marks the account the primary where the edit makes it so, as
// p does; it changes nothing else, as no fixture saves another edit.
func (s setupSteps) EditAccount(id string, edit setup.Edit) (accounts.Edited, error) {
	if edit.Primary && s.saved != nil {
		s.saved.mark(id)
	}
	return accounts.Edited{}, nil
}

// RemoveAccount removes nothing, as no fixture removes an account.
func (s setupSteps) RemoveAccount(string) (accounts.Removed, error) {
	return accounts.Removed{}, nil
}

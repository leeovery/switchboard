// Package screen runs setup's full screen, a page a step in the dashboard's
// chrome, as dashboard.Setup draws it: it walks the steps forward, each
// doing what it does as it's reached, until the user leaves, or a step
// fails. It holds the screen's state alone, and calls setup through the
// Steps it declares, its long work on goroutines it owns, so tests drive it
// as a terminal would.
package screen

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// logger notes what the screen does: a full screen leaves nowhere else to
// say what went wrong.
var logger = logs.For("setup")

// Steps are what the screen has setup do.
type Steps interface {
	// Standing is whether a router answers, for the status line, failing
	// only with ctx's error once ctx is done.
	Standing(ctx context.Context) (setup.RouterStanding, error)
	// Review is every configured account, read but unchecked, for the
	// status line and the Accounts page.
	Review() (setup.Review, error)
	// ReviewOf is the account's row, its token checked with the API, failing
	// with ctx's error once ctx is done.
	ReviewOf(ctx context.Context, id string) (setup.Reviewed, error)
	// Priming is the day priming spreads the 5-hour windows' resets over:
	// the zero Day where priming is off.
	Priming() (config.Day, error)
	// SetDay sets the day priming spreads the resets over, "" turning
	// priming off, writing nothing where it doesn't change.
	SetDay(day string) (config.Day, error)
	// Service takes the service step, installing the service, or starting
	// its router, where it must, and says what it found and did, and the
	// router's health: on a failure, it says so too, as the screen draws it.
	// It stops once ctx is done.
	Service(ctx context.Context) (setup.Serviced, error)
	// Skill writes the skill, or brings it up to date, and says which: on a
	// failure, it says so too, as the screen draws it.
	Skill() (setup.Said, error)
	// CheckAdding checks the account adding gives, as it would be added,
	// writing nothing, failing with a setup.Refused where config refuses a
	// value.
	CheckAdding(adding setup.Adding) (config.NewAccount, error)
	// EditAccount changes the account id as edit says, failing with a
	// setup.Refused where config refuses a value.
	EditAccount(id string, edit setup.Edit) (accounts.Edited, error)
	// RemoveAccount removes the account id, failing with a setup.Refused
	// where it's the only one.
	RemoveAccount(id string) (accounts.Removed, error)
	// MakeToken makes the token f says with claude setup-token, and saves it,
	// telling report each step as it comes, so report must never wait on it.
	// The end of ctx stops it, saving nothing, until setup-token says it has
	// made the token; from then, it's saved however ctx ends, or, unread,
	// given back to copy.
	MakeToken(ctx context.Context, f setup.TokenFor, report func(setup.Making)) (setup.Tokened, error)
	// Reopen opens the sign-in address at its AddressReady again, the way it
	// opened first.
	Reopen(ctx context.Context, at setup.Making) error
	// PasteToken saves the token pasted, as f says, as MakeToken saves a
	// token made, saving nothing where what's pasted is no token, or the
	// end of ctx comes before it's saved.
	PasteToken(ctx context.Context, f setup.TokenFor, pasted string) (setup.Tokened, error)
	// Link makes switchboard's claude link, or repairs it, writes
	// switchboard's files for the shells, and asks the user's login shell
	// which claude it runs, saying what it found, but never with its error,
	// failing with accounts.ErrInterrupted at an interrupt as it asks. It
	// stops once ctx is done.
	Link(ctx context.Context) (setup.LinkFound, error)
	// AskAgain asks the user's login shell again which claude it runs, found
	// being what the claude link step found before, and says what it finds
	// now, failing as Link does.
	AskAgain(ctx context.Context, found setup.LinkFound) (setup.LinkFound, error)
	// AddLine adds switchboard's line, which found offers, to the end of the
	// startup file it offers it for, once, and says what that leaves: the
	// line added, the shell to ask again; a line there reading another
	// switchboard's file; or, where it can't be added, how to add it by
	// hand. It fails, changing nothing, where found offers no line.
	AddLine(found setup.LinkFound) (setup.LinkFound, error)
	// SessionHook installs the session hook, or puts it right, and says
	// which it did, never failing: a hook it can't install, it says so.
	SessionHook() setup.Said
}

// Config is what the screen is given.
type Config struct {
	Steps Steps
	// Step is the step the screen opens on, the steps before it passed:
	// zero, Accounts, for the first.
	Step dashboard.SetupStep
	// Now reads the wall clock: nil reads the system's.
	Now func() time.Time
	// After delivers msg once d has passed on the clock, for the ticks that
	// move the clock on: nil takes a real timer.
	After func(d time.Duration, msg tea.Msg) tea.Cmd
	// Await has wait run, the command that waits for what a token's making
	// tells next, which blocks until it tells something: nil runs it as any
	// command is run. A test holds it, to hear the making's steps in turn.
	Await func(wait tea.Cmd) tea.Cmd
	// Pair is the themes the screen is drawn in, by the terminal's
	// background: nil draws it without colour, as NO_COLOR asks.
	Pair *theme.Pair
	// Size is what to draw at until the terminal gives its size, and in place
	// of a width or height it gives as zero, which means it doesn't know.
	Size Size
}

// Size is a terminal's size in cells.
type Size struct {
	Width, Height int
}

// or is s with each dimension it has as zero, which means it isn't known,
// taken from known.
func (s Size) or(known Size) Size {
	return Size{Width: cmp.Or(s.Width, known.Width), Height: cmp.Or(s.Height, known.Height)}
}

// Model is the screen's state, as Bubble Tea runs it. Build one with New.
type Model struct {
	steps Steps
	now   func() time.Time
	after func(d time.Duration, msg tea.Msg) tea.Cmd
	await func(wait tea.Cmd) tea.Cmd
	jobs  *jobs

	// size is what the screen is drawn at: the terminal's, as far as it's
	// known.
	size Size

	// coloured is set where the screen is drawn in colour, in pair, showing
	// the theme in force once the terminal has said what its background is,
	// or had its time to.
	coloured bool
	pair     theme.Pair
	showing  theme.Theme
	backdrop backdrop.Backdrop

	walk   walk
	status status
	// firstRun is set where the Accounts step first read no account.
	firstRun bool
	// chosen is the Accounts page's row chosen: an account's, by its place
	// in the review, or past the last, + add an account.
	chosen int
	// add, edit, remove, token and paste are the cards open over the
	// Accounts page, the card adding an account, the chosen account's edit
	// card, the card asking before it's removed, the token card, or the card
	// pasting a token: nil where none is.
	add    *addCard
	edit   *editCard
	remove *removeCard
	token  *tokenCard
	paste  *pasteCard
	// makings numbers the tokens' makings begun, each the token card's; and
	// interrupted is set once one ends with an interrupt held as its token
	// was saved, which the screen leaves at.
	makings     int
	interrupted bool
	// marking counts the accounts p is making the primary, not yet answered;
	// and rereading is set once a change to the accounts is made, till
	// they're read again, as settling has them, the row chosen then moved to
	// the account madeFor names, where it's set, as a token was saved for it.
	marking   int
	rereading bool
	madeFor   string
	// accountsFailed is what the Accounts step failed at, where it did:
	// reading the accounts, the zero, unless a change to them, or the check
	// of an account to add, failed.
	accountsFailed dashboard.AccountsFailed
	// priming is the Priming page as it stands: nil until its day's read.
	priming *primingForm
	// service is what the service step told, and its router's health: nil
	// until it has.
	service *setup.Serviced
	// found is what the claude link step found, as setup gave it, for setup
	// to be given again; link is the same, made safe to show; both nil until
	// it has found anything. offer is the card offering to add switchboard's
	// line, open over the page: nil where none is. linkEnded is set once the
	// step has ended, its session hook installing, and hook is what the
	// install said: nil until it has.
	found     *setup.LinkFound
	link      *setup.LinkFound
	offer     *offerCard
	linkEnded bool
	hook      *setup.Said
	// skill is what the skill step said it did: nil until it has.
	skill *setup.Said
	// scroll is how many rows the page under its heading is scrolled down.
	scroll int
	// spins numbers the spinners started, a step's as it's reached, and the
	// token card's as a making begins, each move armed for the last: a move
	// armed for one before is dropped, so no spinner moves on two chains of
	// moves.
	spins int
}

// New returns the screen of cfg, opening on cfg's step, its jobs run under
// ctx: once ctx is done, no outcome of theirs is taken in.
func New(ctx context.Context, cfg Config) Model {
	m := Model{steps: cfg.Steps, now: cfg.Now, after: cfg.After, await: cfg.Await, jobs: newJobs(ctx), size: cfg.Size, walk: openingOn(cfg.Step)}
	if m.now == nil {
		m.now = time.Now
	}
	if m.after == nil {
		m.after = after
	}
	if m.await == nil {
		m.await = func(wait tea.Cmd) tea.Cmd { return wait }
	}
	if cfg.Pair != nil {
		m.coloured, m.pair = true, *cfg.Pair
	}
	m.status = m.status.asking()
	return m
}

// after delivers msg once d has passed, on a real timer.
func after(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// Init starts the clock's ticks, asks for the status line's parts, and runs
// the action of the step the screen opens on, its spinner moving while it
// runs; and in colour, asks the terminal what its background is, the screen
// staying blank till it says or has had its time to.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.armTick(), m.ask(), m.action(), m.armSpin()}
	if m.coloured {
		cmds = append(cmds, backdrop.Ask(m.after))
	}
	return tea.Batch(cmds...)
}

// Update takes in a message: a key, a paste, the wheel, a resize, the
// terminal's background, a tick, or an outcome of setup's.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.pressed(msg)
	case tea.PasteMsg:
		return m.pasted(msg.Content), nil
	case tea.MouseWheelMsg:
		return m.wheeled(msg), nil
	case tea.WindowSizeMsg:
		return m.resized(msg)
	case tea.BackgroundColorMsg:
		return m.settled(m.backdrop.Answered(msg.Color, m.pair)), nil
	case backdrop.Unanswered:
		return m.settled(m.backdrop.NoAnswer()), nil
	case tickMsg:
		return m, m.armTick()
	case spinMsg:
		if msg.spins != m.spins {
			return m, nil
		}
		return m, m.armSpin()
	case standingMsg:
		m.status = m.status.standingAnswered(msg)
		return m, nil
	case reviewMsg:
		return m.reviewed(msg).check()
	case checkedMsg:
		m.status = m.status.checkAnswered(msg)
		return m.check()
	case primedMsg:
		return m.primed(msg), nil
	case daySetMsg:
		return m.daySet(msg)
	case servicedMsg:
		return m.serviced(msg)
	case linkedMsg:
		return m.linked(msg)
	case addedMsg:
		return m.added(msg)
	case hookedMsg:
		return m.hooked(msg), nil
	case skilledMsg:
		return m.skilled(msg), nil
	case addCheckedMsg:
		return m.addChecked(msg)
	case editedMsg:
		return m.editSaved(msg)
	case removedMsg:
		return m.removed(msg)
	case primaryMsg:
		return m.marked(msg)
	case makingMsg:
		return m.madeStep(msg)
	case tokenedMsg:
		return m.tokened(msg)
	case reopenedMsg:
		return m.reopened(msg), nil
	case pastedMsg:
		return m.pasteSaved(msg)
	}
	return m, nil
}

// pressed acts on a key: while a card is open, as cardPressed has it;
// while a field is typed in, as typedIn has it; else q and ctrl+c leave,
// enter moves on, as entered has it, tab types in the Priming page's field,
// c and d copy what the page offers at them, where it offers anything, r
// asks the shell again at the claude link, where checkable has it, the
// Accounts page's keys choose its rows, as choosing has them, and act on
// the account chosen, as accountKey has them, and the rest scroll the page,
// as dashboard.ScrollKey has them.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.carded():
		return m.cardPressed(key)
	case m.typing():
		return m.typedIn(key)
	}
	switch k := key.String(); k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		return m.entered()
	case "tab":
		return m.focused(true), nil
	case "c", "d":
		return m, copied(m.page().Copies(k))
	default:
		if k == "r" && m.checkable() {
			return m.checkingAgain()
		}
		if by, ok := m.choosing(k); ok {
			return m.chose(by), nil
		}
		if a, ok := m.chosenAccount(); ok && slices.Contains(accountKeys, k) {
			return m.accountKey(k, a)
		}
		if rows, pages, ok := dashboard.ScrollKey(k); ok {
			return m.scrolled(rows, pages), nil
		}
	}
	return m, nil
}

// resized takes in the size the terminal gives, keeping the size drawn at for
// a width or height it gives as zero.
func (m Model) resized(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	given := Size{Width: msg.Width, Height: msg.Height}
	m.size = given.or(m.size)
	m = m.chosenInView()
	if m.size == given {
		return m, nil
	}
	// Bubble Tea cuts every frame to the size in the last size message it
	// passed on, so the model passes on the size it draws at, as the
	// dashboard's watch does.
	size := tea.WindowSizeMsg{Width: m.size.Width, Height: m.size.Height}
	return m, func() tea.Msg { return size }
}

// tickMsg wakes the screen to move its clock on.
type tickMsg struct{}

// armTick arms the clock's next tick, as dashboard.TickDelay has it. Ticks
// never come as one long timer, as a timer's clock stops while a Mac
// sleeps: each reads the wall clock instead.
func (m Model) armTick() tea.Cmd {
	return m.after(dashboard.TickDelay(m.now()), tickMsg{})
}

// spinMsg wakes the screen to move the spinner of the page showing on, armed
// for the step reached as spins numbers it.
type spinMsg struct {
	spins int
}

// armSpin arms the spinner's next move, as Setup.SpinDelay has it, where
// the page showing has one.
func (m Model) armSpin() tea.Cmd {
	if d, spins := m.page().SpinDelay(); spins {
		return m.after(d, spinMsg{spins: m.spins})
	}
	return nil
}

// View draws the screen full, as dashboard.Setup draws it, at the clock's
// time: in colour, once the terminal has said what its background is, or
// had its time to, its canvas painted on every cell and set as the
// terminal's background too, as Background says. The mouse is on while it
// runs, for the wheel.
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.coloured && !m.backdrop.Settled() {
		return v
	}
	look := m.look()
	v.SetContent(strings.Join(m.page().Draw(m.size.Width, m.size.Height, look), "\n"))
	v.BackgroundColor = m.backdrop.Background(look)
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// page is the screen as dashboard.Setup draws it, at the clock's time.
func (m Model) page() dashboard.Setup {
	s := dashboard.Setup{
		Now: m.now(), Summary: m.summary(), Passed: m.walk.passed, Status: m.status.line(),
		UnderWay: !m.walk.done, Scroll: m.scroll, Linking: dashboard.SetupLinkPage{Asking: m.asking(), Ended: m.linkEnded, Hook: m.hook, Offer: m.offer.drawn()},
	}
	if m.walk.step == dashboard.SetupAccounts {
		s.Accounts = m.accountsPage()
	}
	if m.priming != nil {
		s.Priming = m.priming.page()
	}
	return s
}

// failure is why a step failed, as err says, made safe to show: "failed"
// where that leaves nothing, as it must still read as a failure.
func failure(err error) string {
	return cmp.Or(prose.Clean(err.Error()), "failed")
}

// summary is how setup stands, as the walk leaves it: the step it stands
// at, why that step failed, made safe to show, where it did, and what each
// step told.
func (m Model) summary() setup.Summary {
	s := setup.Summary{At: m.walk.step, Review: m.status.review, Service: m.service, Link: m.link, Skill: m.skill}
	if m.walk.failed != nil {
		s.Failure = failure(m.walk.failed)
	}
	if m.priming != nil {
		s.Day = m.priming.primed()
	}
	return s
}

// copied is the command that copies text to the terminal's clipboard, by
// OSC 52: none where there's no text.
func copied(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return tea.SetClipboard(text)
}

// Backdrop is the terminal's background as the screen has it.
func (m Model) Backdrop() backdrop.Backdrop {
	return m.backdrop
}

// look is how the screen is drawn: in the theme in force, full screen, or
// without colour.
func (m Model) look() dashboard.Look {
	if !m.coloured {
		return dashboard.NoColour()
	}
	return dashboard.Screen(m.showing)
}

// settled has the screen drawn, from now on, as b says the terminal's
// background is, in the theme in force, should afresh say so.
func (m Model) settled(b backdrop.Backdrop, afresh bool) Model {
	m.backdrop = b
	if afresh {
		m.backdrop, m.showing = b.DrawInForce(m.pair)
	}
	return m
}

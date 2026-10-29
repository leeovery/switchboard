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
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/logs"
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
	// status line.
	Review() (setup.Review, error)
	// Priming is the day priming spreads the 5-hour windows' resets over:
	// the zero Day where priming is off.
	Priming() (config.Day, error)
	// SetDay sets the day priming spreads the resets over, "" turning
	// priming off, writing nothing where it doesn't change.
	SetDay(day string) (config.Day, error)
	// Skill writes the skill, or brings it up to date, and says which: on a
	// failure, it says so too, as the screen draws it.
	Skill() (setup.Said, error)
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
	// priming is the Priming page as it stands: nil until its day's read.
	priming *primingForm
	// skill is what the skill step said it did: nil until it has.
	skill *setup.Said
	// scroll is how many rows the page under its heading is scrolled down.
	scroll int
}

// New returns the screen of cfg, opening on cfg's step, its jobs run under
// ctx: once ctx is done, no outcome of theirs is taken in.
func New(ctx context.Context, cfg Config) Model {
	m := Model{steps: cfg.Steps, now: cfg.Now, after: cfg.After, jobs: newJobs(ctx), size: cfg.Size, walk: openingOn(cfg.Step)}
	if m.now == nil {
		m.now = time.Now
	}
	if m.after == nil {
		m.after = after
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
// the action of the step the screen opens on; and in colour, asks the
// terminal what its background is, the screen staying blank till it says or
// has had its time to.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.armTick(), m.ask(), m.action()}
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
	case standingMsg:
		m.status = m.status.standingAnswered(msg)
		return m, nil
	case reviewMsg:
		m.status = m.status.reviewAnswered(msg)
		return m, nil
	case primedMsg:
		return m.primed(msg), nil
	case daySetMsg:
		return m.daySet(msg)
	case skilledMsg:
		return m.skilled(msg), nil
	}
	return m, nil
}

// pressed acts on a key: while a field is typed in, as typedIn has it;
// else q and ctrl+c leave, enter moves on, as entered has it, tab types in
// the Priming page's field, and the rest scroll the page, as
// dashboard.ScrollKey has them.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.typing() {
		return m.typedIn(key)
	}
	switch k := key.String(); k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		return m.entered()
	case "tab":
		return m.focused(true), nil
	default:
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
		Now: m.now(), Step: m.walk.step, Passed: m.walk.passed, Status: m.status.line(),
		UnderWay: !m.walk.done, Failed: m.walk.failed != nil, Skill: m.skill, Scroll: m.scroll,
	}
	if m.priming != nil {
		s.Priming = m.priming.page()
	}
	return s
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

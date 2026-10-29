package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/setup"
)

// SetupStep is a step of setup, as its tabs name them, in their order.
type SetupStep int

const (
	SetupAccounts SetupStep = iota
	SetupPriming
	SetupService
	SetupLink
	SetupSkill
	SetupDone
)

// SetupSteps are setup's steps, in their order.
func SetupSteps() []SetupStep {
	return []SetupStep{SetupAccounts, SetupPriming, SetupService, SetupLink, SetupSkill, SetupDone}
}

// Title is the step's name, as its tab has it.
func (s SetupStep) Title() string {
	switch s {
	case SetupAccounts:
		return "Accounts"
	case SetupPriming:
		return "Priming"
	case SetupService:
		return "Service"
	case SetupLink:
		return "claude link"
	case SetupSkill:
		return "Skill"
	}
	return "Done"
}

// numbered reports whether the step's tab carries its number: every step's
// but Done's, which isn't one to take.
func (s SetupStep) numbered() bool {
	return s != SetupDone
}

// Setup is setup's screen in the dashboard's chrome, a page a step: the
// title row, its steps as tabs; the status line; the page of the step
// showing; and the key line.
type Setup struct {
	// Now is the time the clock shows.
	Now time.Time
	// Step is the step showing, and Passed the steps passed through, each
	// ticked, whatever it left for later.
	Step   SetupStep
	Passed []SetupStep
	// Status are the status line's parts, in order, as setup decides them.
	Status []setup.Part
	// UnderWay is set while the step showing does what it does, and Failed
	// where it failed, which stops setup there, its page saying why: either
	// way, its key line's right has q quit alone.
	UnderWay, Failed bool
	// Priming is the Priming step's page: nil until its day is read.
	Priming *SetupPrimingPage
	// Skill is what the skill step did: nil until it's done it.
	Skill *setup.Said
	// Scroll is how many rows the page under its heading is scrolled down.
	Scroll int
}

// The page's rows, as setupRows lays them out.
const (
	setupTitleRow = iota
	setupStatusRow
	setupBodyRows
	setupKeyRow
)

// leastSetupBody is the fewest rows a step's page has, from its heading,
// which with the chrome makes setup 25 rows at the least. A page taller
// than the rows it's given scrolls, as setupPage.draw pages it.
const leastSetupBody = 19

// tooSmall is all setup says on a terminal smaller than its least.
const tooSmall = "Make the terminal larger."

// Draw draws the screen width cells wide and height tall, in look: a string
// for each of its rows.
func (s Setup) Draw(width, height int, look Look) []string {
	c, _ := s.page(width, height)
	return c.rows(look)
}

// Scrolling is how far the page of the step showing scrolls in a screen
// width cells wide and height tall, scrolled as the screen has it: none on a
// screen smaller than setup's least.
func (s Setup) Scrolling(width, height int) Scrolling {
	_, scrolling := s.page(width, height)
	return scrolling
}

// page draws the screen on a canvas width cells wide and height tall, and
// says how far its page scrolls: its title row; its status line; the page
// of the step showing, scrolled Scroll rows down, in the rows between the
// status line and the key line; and the key line, the last row. Where the
// title row's own pieces don't fit across it, or its rows down it, as
// setupRows lays them, it's tooSmall alone.
func (s Setup) page(width, height int) (*canvas, Scrolling) {
	c := newCanvas(width, height)
	wide := s.title(c, width)
	rows, tall := setupRows(height)
	if !wide || !tall {
		c = newCanvas(width, height)
		c.line(0, 0, line{{tooSmall, secondaryInk}}.fit(width))
		return c, Scrolling{}
	}
	s.statusLine(c, rows[setupStatusRow].At, width)
	shown := s.shown()
	if s.UnderWay || s.Failed {
		shown.onward = []Key{Quit}
	}
	body := rows[setupBodyRows]
	scrolling := shown.draw(c, body.At, body.Size, width, s.Scroll)
	shown.keyLine(c, rows[setupKeyRow].At, width)
	return c, scrolling
}

// setupRows lays the screen's rows out down height rows: the title row; a
// row on, the status line; a row on, the step's page, at least
// leastSetupBody rows, taking what's spare; and a row on, the key line, the
// last. It reports false where they don't fit.
func setupRows(height int) ([]flex.Span, bool) {
	one := flex.Part{Least: 1, Natural: 1, Gap: 1}
	return flex.Lay(height, []flex.Part{
		setupTitleRow:  {Least: 1, Natural: 1},
		setupStatusRow: one,
		setupBodyRows:  {Least: leastSetupBody, Natural: leastSetupBody, Grows: true, Gap: 1},
		setupKeyRow:    one,
	}...)
}

// passed reports whether the step's been passed through.
func (s Setup) passed(step SetupStep) bool {
	return slices.Contains(s.Passed, step)
}

// shown is the page of the step showing: the Priming's, as primingPage has
// it; the Skill's, as skillPage has it; else, until it's drawn, its name in
// capitals alone.
func (s Setup) shown() setupPage {
	switch s.Step {
	case SetupPriming:
		return s.primingPage()
	case SetupSkill:
		return s.skillPage()
	}
	return setupPage{name: strings.ToUpper(s.Step.Title())}
}

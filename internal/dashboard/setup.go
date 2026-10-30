package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/setup"
)

// SetupStep is a step of setup, as setup names them, its tab named by its
// Title.
type SetupStep = setup.Step

// Setup's steps, in their order.
const (
	SetupAccounts = setup.StepAccounts
	SetupPriming  = setup.StepPriming
	SetupService  = setup.StepService
	SetupLink     = setup.StepLink
	SetupSkill    = setup.StepSkill
	SetupDone     = setup.StepDone
)

// SetupSteps are setup's steps, in their order.
func SetupSteps() []SetupStep {
	return setup.Steps()
}

// stepNumbered reports whether the step's tab carries its number: every step's
// but Done's, which isn't one to take.
func stepNumbered(step SetupStep) bool {
	return step != SetupDone
}

// Setup is setup's screen in the dashboard's chrome, a page a step: the
// title row, its steps as tabs; the status line; the page of the step
// showing; and the key line.
type Setup struct {
	// Now is the time the clock shows.
	Now time.Time
	// Summary is how setup stands: At, the step showing; Failure, why it
	// failed, where it did, which stops setup there, its page saying why;
	// and what each step told, which its page draws, and Done a line a step.
	setup.Summary
	// Passed are the steps passed through, each ticked, whatever it left for
	// later.
	Passed []SetupStep
	// Status are the status line's parts, in order, as setup decides them.
	Status []setup.Part
	// UnderWay is set while the step showing does what it does: then, as
	// where it failed, its key line's right has q quit alone.
	UnderWay bool
	// Accounts is what the Accounts step's page has of its own: nil on every
	// other step.
	Accounts *SetupAccountsPage
	// Priming is the Priming step's page: nil until its day is read.
	Priming *SetupPrimingPage
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
	c, _ := s.page(width, height, look)
	return c.rows(look)
}

// Scrolling is how far the page of the step showing scrolls in a screen
// width cells wide and height tall, scrolled as the screen has it: none on a
// screen smaller than setup's least.
func (s Setup) Scrolling(width, height int) Scrolling {
	_, scrolling := s.page(width, height, Look{})
	return scrolling
}

// ScrolledToChosen is how far the page of the step showing is scrolled to
// keep its row chosen in view in a screen width cells wide and height tall:
// from Scroll, no further than it must go; Scroll where the page has no row
// chosen, or none of it shows, as on a screen smaller than setup's least.
func (s Setup) ScrolledToChosen(width, height int) int {
	if s.Scrolling(width, height).Page == 0 {
		return s.Scroll
	}
	rows, _ := setupRows(height)
	return s.shown(Look{}).revealing(rows[setupBodyRows].Size, width, s.Scroll)
}

// SpinDelay is how long from the screen's time until the spinner of the
// page showing, or of the card open over it, moves on, as spinDelay has it.
// It reports false where neither shows one.
func (s Setup) SpinDelay() (time.Duration, bool) {
	if s.shown(Look{}).underWay == nil && !s.cardSpins() {
		return 0, false
	}
	return spinDelay(s.Now), true
}

// page draws the screen on a canvas width cells wide and height tall, in
// look, and says how far its page scrolls: its title row; its status line;
// the page of the step showing, scrolled Scroll rows down, in the rows
// between the status line and the key line; and the key line, the last row,
// but where a card is open over the page: then the card, its keys its own,
// and the key line empty. Where the title row's own pieces don't fit across
// it, or its rows down it, as setupRows lays them, it's tooSmall alone.
func (s Setup) page(width, height int, look Look) (*canvas, Scrolling) {
	c := newCanvas(width, height)
	wide := s.title(c, width)
	rows, tall := setupRows(height)
	if !wide || !tall {
		c = newCanvas(width, height)
		c.line(0, 0, line{{tooSmall, secondaryInk}}.fit(width))
		return c, Scrolling{}
	}
	s.statusLine(c, rows[setupStatusRow].At, width)
	shown := s.shown(look)
	if s.UnderWay || s.Failure != "" {
		shown.onward = []Key{Quit}
	}
	body := rows[setupBodyRows]
	scrolling := shown.draw(c, body.At, body.Size, width, s.Scroll)
	if card := s.card(width); card != nil {
		card.draw(c)
		return c, scrolling
	}
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

// shown is the page of the step showing, drawn in look: the Accounts', as
// accountsPage has it; the Priming's, as primingPage has it; the Service's,
// as servicePage has it; the Skill's, as skillPage has it; Done's, as
// donePage has it; else, until it's drawn, its name in capitals alone.
func (s Setup) shown(look Look) setupPage {
	switch s.At {
	case SetupAccounts:
		return s.accountsPage(look)
	case SetupPriming:
		return s.primingPage()
	case SetupService:
		return s.servicePage()
	case SetupSkill:
		return s.skillPage()
	case SetupDone:
		return s.donePage()
	}
	return setupPage{name: strings.ToUpper(s.At.Title())}
}

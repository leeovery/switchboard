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
	Status []SetupPart
	// Skill is what the skill step did: nil until it's done it.
	Skill *setup.Said
}

// SetupStanding is how a part of setup's status line stands, which marks it
// and inks its words.
type SetupStanding int

const (
	// SetupTold is a part told alone, unmarked, its words muted.
	SetupTold SetupStanding = iota
	// SetupUp is a part that's up, as a router that answers.
	SetupUp
	// SetupDown is a part that's not up yet, as a router not running.
	SetupDown
)

// SetupPart is a part of setup's status line: how it stands, and its words.
type SetupPart struct {
	Standing SetupStanding
	Words    string
}

// The page's rows, as setupRows lays them out.
const (
	setupTitleRow = iota
	setupStatusRow
	setupBodyRows
	setupKeyRow
)

// leastSetupBody is the fewest rows a step's page has, from its heading:
// Done's, from its heading to its last line, as the tallest page that
// doesn't scroll. With the chrome, setup takes 25 rows at the least.
const leastSetupBody = 19

// tooSmall is all setup says on a terminal smaller than its least.
const tooSmall = "Make the terminal larger."

// Draw draws the screen width cells wide and height tall, in look: a string
// for each of its rows.
func (s Setup) Draw(width, height int, look Look) []string {
	return s.page(width, height).rows(look)
}

// page draws the screen on a canvas width cells wide and height tall: its
// title row; its status line; the page of the step showing, in the rows
// between the status line and the key line; and the key line, the last row.
// Where the title row's own pieces don't fit across it, or its rows down it,
// as setupRows lays them, it's tooSmall alone.
func (s Setup) page(width, height int) *canvas {
	c := newCanvas(width, height)
	wide := s.title(c, width)
	rows, tall := setupRows(height)
	if !wide || !tall {
		c = newCanvas(width, height)
		c.line(0, 0, line{{tooSmall, secondaryInk}}.fit(width))
		return c
	}
	s.statusLine(c, rows[setupStatusRow].At, width)
	shown := s.shown()
	body := rows[setupBodyRows]
	shown.draw(c.within(bounds{y: body.At, width: width, rows: body.Size}), body.At, width)
	shown.keyLine(c, rows[setupKeyRow].At, width)
	return c
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

// shown is the page of the step showing: the Skill's, as skillPage has it;
// else, until it's drawn, its name in capitals alone.
func (s Setup) shown() setupPage {
	if s.Step == SetupSkill {
		return s.skillPage()
	}
	return setupPage{name: strings.ToUpper(s.Step.Title())}
}

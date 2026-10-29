package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/setup"
)

// What Done says: its heading, and the heading over what's next; then the
// commands to run next, each with what it does.
const (
	doneHeading = "SWITCHBOARD IS SET UP"
	nextHeading = "NEXT"
)

var nextCommands = []command{
	{name: "claude", does: "start Claude Code as usual: its sessions route through switchboard"},
	{name: "switchboard", does: "open the dashboard: every account's windows, sessions and history"},
}

// The keys that copy what the page offers: a line, or a directory.
var (
	copyLine      = Key{Key: "c", Does: "copy the line"}
	copyDirectory = Key{Key: "c", Does: "copy the directory"}
)

// command is a command Done says to run next, and what it does.
type command struct {
	name, does string
}

// donePage is the Done page: SWITCHBOARD IS SET UP, then a line a step, as
// setup sums them up; then NEXT, and the commands to run next. Its keys copy
// what the claude link step leaves to put in place by hand, where there's
// anything, named as what it is, and its onward key opens the dashboard.
func (s Setup) donePage() setupPage {
	p := setupPage{name: doneHeading, steps: s.Lines(), next: nextCommands, onward: onward("open the dashboard")}
	switch l, offers := s.offered(); {
	case !offers:
	case l.Directory:
		p.keys = []Key{copyDirectory}
	default:
		p.keys = []Key{copyLine}
	}
	return p
}

// Copies is what the page showing offers to copy: at Done, what the claude
// link step leaves to put in place by hand, as its line has it; "" where it
// offers nothing.
func (s Setup) Copies() string {
	l, _ := s.offered()
	return l.Add
}

// offered is the step's line whose Add the page showing offers to copy, and
// whether there's one: at Done alone.
func (s Setup) offered() (setup.StepLine, bool) {
	if s.At != SetupDone {
		return setup.StepLine{}, false
	}
	for _, l := range s.Lines() {
		if l.Add != "" {
			return l, true
		}
	}
	return setup.StepLine{}, false
}

// stepLine draws a step's line along row y, width cells wide: its row, as
// fact draws it, then under it, from the detail's column, its line to add
// by hand, in accent.mode, wrapped at the column's width, words wider than
// it broken, never cut, as it's to be copied whole. It returns the row
// after the last it drew on.
func stepLine(c *canvas, y, width int, l setup.StepLine) int {
	y = fact(c, y, width, l.Said)
	if l.Add == "" {
		return y
	}
	detail := factParts(width)[factDetail]
	for part := range strings.SplitSeq(ansi.Wrap(l.Add, detail.Size, ""), "\n") {
		c.line(detail.At, y, line{{part, accentInk}})
		y++
	}
	return y
}

// The parts of a command's row, as commandParts lays them out.
const (
	commandIndent = iota
	commandName
	commandDoes
)

// commandParts are a command's row's parts, across width cells: setupIndent
// cells, its name, cut to end a cell before factDetailAt, and what it does,
// from there, taking the rest.
func commandParts(width int) []flex.Span {
	name := factDetailAt - setupIndent - 1
	spans, _ := flex.Lay(width, []flex.Part{
		commandIndent: {Least: setupIndent, Natural: setupIndent},
		commandName:   {Least: name, Natural: name},
		commandDoes:   {Least: 1, Grows: true, Gap: 1},
	}...)
	return spans
}

// draw draws the command along row y, width cells wide, as commandParts lays
// it out: its name bold in accent.mode, and what it does in text.secondary,
// wrapped onto the rows under it. It returns the row after the last it drew
// on.
func (cmd command) draw(c *canvas, y, width int) int {
	spans := commandParts(width)
	name, does := spans[commandName], spans[commandDoes]
	c.line(name.At, y, line{{cmd.name, nameInk}}.fit(name.Size))
	return wrapAlong(c, does, y, line{{cmd.does, secondaryInk}})
}

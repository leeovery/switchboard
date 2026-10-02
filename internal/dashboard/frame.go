package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// The widths the frame's arrangement changes at: from slotsInRowFrom
// columns, the heading's four slots sit in a row, and from phoneUnder in two
// rows of two; under it, the frame is a phone's.
const (
	slotsInRowFrom = 150
	phoneUnder     = 100
)

// Frame is the dashboard full screen, as a watch draws it: the title row,
// the heading, the view shown and the footer, drawn from the document, the
// clock, and what the watch knows besides.
type Frame struct {
	// Width and Height are the terminal's, in cells.
	Width, Height int
	Look          Look
	// Views are the views there are, in the order tab moves through them,
	// each a tab, and View the one shown.
	Views []View
	View  View
	// Lost is when the router stopped answering, while its last document
	// stays on screen: zero otherwise.
	Lost time.Time
	// Outdated is set when the router is from before it told of events and
	// gave its history: RECENT asks for it to be restarted.
	Outdated bool
	// Fresh are the events looks saw newly, by id, each as far as the
	// highlight on its row has faded, from 0, just seen, to 1, gone.
	Fresh map[int]float64
	// History is how the accounts' windows have been used, for the charts.
	History History
	// Keys are the keys that work, the most used first: the footer lists as
	// many as fit.
	Keys []Key
	// Note says what the last key did, or why it couldn't, while that's news:
	// the footer says it in place of the keys.
	Note string
	// Status says how reading goes, at the footer's right: when the document
	// was read, as "read 4s ago".
	Status string
	// Policy is the provider's say in which windows every model shares, which
	// a session's 5-hour window is, and which its week.
	Policy score.Policy
}

// Draw draws the frame of doc at now: a string for each of the terminal's
// rows, each, where the look paints its canvas, the terminal's whole width.
// Countdowns run from now, and times show in now's time zone.
func (f Frame) Draw(doc status.Document, now time.Time) []string {
	c := newCanvas(f.Width, f.Height)
	top := f.title(c, now) + 1
	top = f.heading(c, doc, now, top) + 1
	footer := f.Height - 1
	if f.View == Accounts {
		f.accounts(c, doc, now, top, footer-1)
	}
	f.footer(c, footer)
	return c.rows(f.Look)
}

// phone reports whether the frame is a phone's: under phoneUnder columns.
func (f Frame) phone() bool {
	return f.Width < phoneUnder
}

// edge is the column the frame's content ends before at the right, a margin
// short of the terminal's.
func (f Frame) edge() int {
	return f.Width - margin
}

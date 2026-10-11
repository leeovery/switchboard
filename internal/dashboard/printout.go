package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// By is how the printout groups the windows: by window, each window's
// accounts under it, or by account, each account's windows under it.
type By int

const (
	ByWindow By = iota
	ByAccount
)

// Printout is every account's windows as usage prints them into the
// scrollback, once, from the status document alone.
type Printout struct {
	// Width is the terminal's, in cells.
	Width int
	Look  Look
	By    By
	// Windows are the windows' keys to print, in Claude's order; none prints
	// every one.
	Windows []string
}

// Draw draws the printout of doc at now: a string for each of its rows, as
// tall as it is, from the header to the key line, with no blank row before or
// after.
func (p Printout) Draw(doc status.Document, now time.Time) []string {
	c := growing(p.Width)
	c.line(0, 0, slices.Concat(pill, line{spaces(2), {doc.GeneratedAt.In(now.Location()).Format("Mon 2 Jan  15:04"), titleInk}}))
	return c.rows(p.Look)
}

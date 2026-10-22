package dashboard

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// By is how the printout groups the windows: by window, each window's
// accounts under it, or by account, each account's windows under it.
type By int

const (
	ByWindow By = iota
	ByAccount
)

// headerGap is the fewest cells between when the document was read and the
// status, on the printout's header's one line.
const headerGap = 2

// readApart are the cells between the badge and the day it read at, and
// between the day and the time.
const readApart = 2

// headerParts are the badge's line and the status full, as the header lays
// them out on one line, the status ending at the last column. The printout
// is printed once, never redrawn under the reader's eye, so its form
// follows what it says as read: the badge's line, its day counted at the
// widest a day is; and the status in full as read, every part whole, its
// free text too, a restart's reason or a probe's.
func headerParts(full line) []flex.Part {
	badge := pill.width() + readApart + prose.DayWidth() + readApart + prose.TimeOfDayWidth()
	return []flex.Part{{Least: badge, Natural: badge}, {Least: full.width(), Natural: full.width(), Gap: headerGap}}
}

// Printout is every account's windows as usage prints them into the
// scrollback, once, from the status document alone.
type Printout struct {
	// Width is the terminal's, in cells.
	Width int
	Look  Look
	By    By
	// Windows are the windows' keys to print, each once, in the order of care;
	// none prints every one, each model's own among them, as the moment's
	// Listed has them.
	Windows []string
	// Name names a window by its key, as "5-hour" or "Fable week"; where
	// it's nil, a window is named by its label in the document.
	Name func(key string) string
}

// Draw draws the printout of the moment m, a document as a policy judges it
// at a time: a string for each of its rows, as tall as it is, from the
// header to the key line, with no blank row before or after. Under the
// header, after a blank line, is the usage block, then a blank line and its
// key line.
func (p Printout) Draw(m status.Moment) []string {
	c := growing(p.Width)
	y := p.header(c, m) + 2
	block := newUsageBlock(m, p.Width, p.Look, p.By, usageWindows(p.windows(m), p.namer(m)))
	block.key(c, block.draw(c, y)+1)
	return c.rows(p.Look)
}

// windows are the keys of the windows the printout of m prints: its Windows,
// else every one m lists, each model's own among them.
func (p Printout) windows(m status.Moment) []string {
	if len(p.Windows) > 0 {
		return p.Windows
	}
	return m.Listed(true)
}

// namer names m's windows by their keys as the printout names them: as its
// Name has it, else by their labels, as labelsOf has them.
func (p Printout) namer(m status.Moment) func(key string) string {
	if p.Name != nil {
		return p.Name
	}
	return labelsOf(m)
}

// header draws the printout's header of the moment m, and returns its last
// row: the badge and when its document was read, then the status line, its
// mode shown only while the router has a global pin. It's one line where
// headerParts fit, the status in full ending at the last column, so it
// never says less than on two; else two, the status from column 0 after a
// blank line, shortening to fit, as a phone's does. The badge's line leaves
// off the date, as a phone's title row does, then the badge, before it's
// cut.
func (p Printout) header(c *canvas, m status.Moment) int {
	at := prose.TimeOfDay(m.Now, m.ReadAt)
	gap, clock := line{spaces(readApart)}, line{{at, titleInk}}
	c.line(0, 0, firstFit(p.Width,
		slices.Concat(pill, gap, line{{prose.Day(m.ReadAt.In(m.Now.Location())) + strings.Repeat(" ", readApart) + at, titleInk}}),
		slices.Concat(pill, gap, clock),
		clock))
	s := newStatusLine(m)
	if full := s.forms()[0]; fitsBeside(p.Width, full) {
		c.right(p.Width, 0, full)
		return 0
	}
	c.line(0, 2, s.fit(p.Width))
	return 2
}

// fitsBeside reports whether the header's parts, the status full, fit on
// one line width cells wide, as flex lays them.
func fitsBeside(width int, full line) bool {
	_, fits := flex.Lay(width, headerParts(full)...)
	return fits
}

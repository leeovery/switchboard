package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// By is how the printout groups the windows: by window, each window's
// accounts under it, or by account, each account's windows under it.
type By int

const (
	ByWindow By = iota
	ByAccount
)

// The printout's header: the fewest columns it takes one line at, narrower
// taking two; and on one line, the fewest cells between when the document was
// read and the status.
const (
	oneLineHeader = 120
	headerGap     = 2
)

// Printout is every account's windows as usage prints them into the
// scrollback, once, from the status document alone.
type Printout struct {
	// Width is the terminal's, in cells.
	Width int
	Look  Look
	By    By
	// Windows are the windows' keys to print, each once, in Claude's order;
	// none prints every one.
	Windows []string
	// Policy is the provider's say in which windows every model shares, which
	// print whatever is read, so every account keeps its rows before any is:
	// the policy the moment printed is judged by. A zero Policy names no
	// window, and prints only the windows read.
	Policy score.Policy
	// Name names a window by its key, as "5-hour" or "Fable week"; where
	// it's nil, a window is named by its label in the document.
	Name func(key string) string
}

// Draw draws the printout of the moment m, a document as its Policy judges
// it at a time: a string for each of its rows, as tall as it is, from the
// header to the key line, with no blank row before or after. Under the
// header, after a blank line, or by account two, is the usage block, then a
// blank line and its key line.
func (p Printout) Draw(m status.Moment) []string {
	c := growing(p.Width)
	y := p.header(c, m) + 2
	if p.By == ByAccount {
		y++
	}
	block := newUsageBlock(m, p.Width, p.Look, p.By, usageWindows(m, p.Windows, p.Policy.Shared, p.namer(m)))
	block.key(c, block.draw(c, y)+1)
	return c.rows(p.Look)
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
// mode shown only while the router has a global pin. From 120 columns it's
// one line, the status ending at the last column; narrower, two, the status
// from column 0 after a blank line. The status shortens to fit, as a phone's
// does; the badge's line leaves off the date, as a phone's title row does,
// then the badge, before it's cut.
func (p Printout) header(c *canvas, m status.Moment) int {
	at := m.ReadAt.In(m.Now.Location())
	gap, clock := line{spaces(2)}, line{{at.Format("15:04"), titleInk}}
	end := c.line(0, 0, firstFit(p.Width,
		slices.Concat(pill, gap, line{{at.Format("Mon 2 Jan  15:04"), titleInk}}),
		slices.Concat(pill, gap, clock),
		clock))
	s := newStatusLine(m)
	if p.Width >= oneLineHeader {
		c.right(p.Width, 0, s.fit(p.Width-end-headerGap))
		return 0
	}
	c.line(0, 2, s.fit(p.Width))
	return 2
}

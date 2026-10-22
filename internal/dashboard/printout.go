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

// widestSessions is the most sessions the header's natural counts: two
// figures' worth.
const widestSessions = 99

// headerParts are the badge's line and the status, at their naturals, as
// the header lays them out on one line of the moment m, the status taking
// the rest, to the last column. Each natural is of what's configured, never
// what's read, so a header of any reading takes one form: the badge's line
// with the widest day and time there are; and the status at the widest the
// configuration allows, as headerStatus has it.
func headerParts(m status.Moment) []flex.Part {
	badge := pill.width() + readApart + prose.DayWidth() + readApart + prose.TimeOfDayWidth()
	status := headerStatus(m).forms()[0].width()
	return []flex.Part{{Least: badge, Natural: badge}, {Least: status, Natural: status, Gap: headerGap}}
}

// headerStatus is the status line at the widest m's configuration allows:
// unhealthy; a restart due, its reason, free text, at its least, cut to
// nothing; pinned to every account configured; widestSessions sessions; and
// new sessions going to the widest account configured, or to none.
func headerStatus(m status.Moment) statusLine {
	ids := make([]string, len(m.Accounts))
	going := []line{nothingRead, noRoom}
	for i, a := range m.Accounts {
		ids[i] = a.ID
		going = append(going, goingTo(m, a.ID))
	}
	return statusLine{
		state:       unhealthy,
		restart:     restartSaid(prose.Ellipsis),
		mode:        pinnedSaid(m, ids),
		sessions:    line{{prose.SessionCount(widestSessions), mutedInk}},
		destination: slices.MaxFunc(going, func(a, b line) int { return a.width() - b.width() }),
	}
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
// headerParts fit, the status ending at the last column, every part of it
// kept, only its free text cut, as statusBeside has it; else two, the
// status from column 0 after a blank line, shortening to fit, as a phone's
// does. The badge's line leaves off the date, as a phone's title row does,
// then the badge, before it's cut.
func (p Printout) header(c *canvas, m status.Moment) int {
	at := prose.TimeOfDay(m.Now, m.ReadAt)
	gap, clock := line{spaces(readApart)}, line{{at, titleInk}}
	end := c.line(0, 0, firstFit(p.Width,
		slices.Concat(pill, gap, line{{prose.Day(m.ReadAt.In(m.Now.Location())) + strings.Repeat(" ", readApart) + at, titleInk}}),
		slices.Concat(pill, gap, clock),
		clock))
	if _, oneLine := flex.Lay(p.Width, headerParts(m)...); oneLine {
		c.right(p.Width, 0, statusBeside(m, p.Width-end-headerGap))
		return 0
	}
	c.line(0, 2, newStatusLine(m).fit(p.Width))
	return 2
}

// statusBeside is the status line of the moment m in full in room cells, its
// free text cut as far as it must be; and where even that doesn't fit, as
// fit shortens it.
func statusBeside(m status.Moment, room int) line {
	for cells := room; cells > 0; cells-- {
		if l := statusCut(m, cells).forms()[0]; l.width() <= room {
			return l
		}
	}
	return newStatusLine(m).fit(room)
}

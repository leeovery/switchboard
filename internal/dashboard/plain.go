package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// plainAt is the column the plain list's rows of sessions start at, under
// their account's panel.
const plainAt = margin + 1

// plain is the Sessions view laid out as a plain list, from its top: each
// account's panel, in the accounts' order, width cells wide, with its
// windows' bars where bars is set, and its sessions under it, a blank row
// between each; and LOG under them all. It's content rows tall.
type plain struct {
	width   int
	bars    bool
	panels  []linePanel
	seats   [][]seated
	log     logBlock
	content int
}

// layOutPlain lays the Sessions view of doc at now out as a plain list: each
// panel as wide as the lines are, but never wider than the frame, with bars
// from barsFrom columns; under it a row for each of its sessions' models,
// the request stream's moves among them, and a note under each, as a card's
// back has them, or where it has none, why, in as many rows as that takes,
// or a row saying so; then LOG, logGap blank rows under the last, with as
// many lines as it has, logLines at most.
func (f Frame) layOutPlain(doc status.Document, now time.Time) plain {
	l := plain{bars: f.Width >= barsFrom, width: linesNarrow}
	if l.bars {
		l.width = linesWide
	}
	l.width = min(l.width, f.edge()-margin)
	shown, last, listing := shownWindows(doc, now, f.Policy), -2, f.listing()
	for _, a := range doc.Accounts {
		p := f.linePanelOf(doc, a, now, shown)
		p.top, p.rows, p.boxed = last+2, len(shown), true
		seats := seatedOn(listing, a.ID)
		l.panels, l.seats = append(l.panels, p), append(l.seats, seats)
		last = p.bottom() + f.underPanel(doc, p, seats, now)
	}
	if lines := f.logCount(doc, now); lines > 0 {
		at := last + logGap + 1
		l.log = logBlock{row: at, lines: lines, end: f.edge()}
		last = at + lines
	}
	l.content = last + 1
	return l
}

// underPanel is how many rows go under the panel p of doc at now, whose
// seats are those given: two for each, its row and its note; else as many
// as unseatedUnder takes.
func (f Frame) underPanel(doc status.Document, p linePanel, seats []seated, now time.Time) int {
	if len(seats) > 0 {
		return 2 * len(seats)
	}
	return len(f.unseatedUnder(doc, p, now))
}

// unseatedUnder says why the panel p of doc at now has no sessions under it,
// as unseatedBy says it, on two lines at most, or that it has none; but
// nothing while probing, as LOG says why there are none.
func (f Frame) unseatedUnder(doc status.Document, p linePanel, now time.Time) []line {
	if doc.Source != status.SourceRouter {
		return nil
	}
	width := f.edge() - plainAt - seatLead
	if lines := flow(f.unseatedBy(doc, p.account.ID, now), []int{width, width}, " "); len(lines) > 0 {
		return lines
	}
	return []line{{{"no sessions", dimInk}}}
}

// plainList draws the Sessions view of doc at now from row top as a plain
// list, laid out as layOutPlain has it, scrolled where it doesn't fit, a
// scrollbar beside it; then the line over the footer, as overFooter has it.
func (f Frame) plainList(c *canvas, doc status.Document, now time.Time, top int) {
	l := f.layOutPlain(doc, now)
	view := max(f.Height-2-top, 0)
	offset := min(max(f.Scroll, 0), max(l.content-view, 0))
	area := newCanvas(f.Width, l.content)
	for i, p := range l.panels {
		f.drawPanel(area, doc, p, margin, l.width, l.bars, now)
		f.plainSeats(area, doc, p, l.seats[i], now)
	}
	f.drawLog(area, doc, l.log, now)
	c.paste(area, offset, top, min(view, l.content))
	f.overFooter(c, doc, now, top, l.content, view, offset, l.hidden)
}

// plainSeats draws the seats under the panel p of doc at now, as a card's
// back lists them: a row each, and its note under it; or where there are
// none, as unseatedUnder says.
func (f Frame) plainSeats(c *canvas, doc status.Document, p linePanel, seats []seated, now time.Time) {
	y, width := p.bottom()+1, f.edge()-plainAt
	if len(seats) == 0 {
		for i, l := range f.unseatedUnder(doc, p, now) {
			c.line(plainAt+seatLead, y+i, l)
		}
		return
	}
	column := modelColumnOf(seats)
	for i, s := range seats {
		c.line(plainAt, y+2*i, f.seatLine(s, column, false, now).fit(width))
		c.line(plainAt+noteIndent, y+2*i+1, line{{"╰ " + note(doc, p.account.ID, s, now), mutedInk}}.fit(width-noteIndent))
	}
}

// hidden counts the accounts whose panels the plain list's view, view rows
// shown from the row offset, leaves out of view, wholly or partly: those
// above, and those below.
func (l plain) hidden(offset, view int) (above, below int) {
	for _, p := range l.panels {
		if p.top < offset {
			above++
		}
		if p.bottom() >= offset+view {
			below++
		}
	}
	return above, below
}

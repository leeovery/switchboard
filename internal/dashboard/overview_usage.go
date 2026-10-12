package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The cells of USAGE by account from 150 columns, beside ROUTING: its bars
// are besideBar cells with a name column of matrixNames, a cell fewer for
// each cell the column is wider; ROUTING's blocks start besideCells after
// its words, and its heading ends routingApart before ROUTING's.
const (
	besideBar    = 50
	besideCells  = 4
	routingApart = 3
)

// usageSection is the Overview's USAGE of doc at now: usage's block, as
// overviewBlock has it. At its least it's the block as usage lays it out at
// the frame's width, which from 150 columns, by window, is the compact
// matrix, 2a + 7 rows for a accounts and the pool; at its most, from 150
// columns, by window, the big bars, its windows stacked as sections at the
// full width; otherwise its least. USAGE never folds: given fewer rows than
// its least, it scrolls within them.
func (f Frame) usageSection(doc status.Document, now time.Time) section {
	block := f.overviewBlock(doc, now)
	return section{
		Section: SectionUsage, least: sectionHead + block.height(), most: sectionHead + block.enlarged().height(),
		description: func() line { return f.roomLeft(doc) },
		draw: func(c *canvas, body bounds) {
			block.fitted(body.rows, f.Scroll).drawIn(c, body)
		},
		targets: func(body bounds) []Target {
			return block.fitted(body.rows, f.Scroll).targets(body, f.usageAcross(block))
		},
	}
}

// overviewBlock is usage's block of doc at now as the Overview's USAGE draws
// it, grouped as the frame's By has it: as usage prints it, but for what the
// Overview alone has. By account from 150 columns, its bars leave room for
// ROUTING beside them. While GRAPHS shows, the matrix picks out the account
// the focus is on. Its key line names the marks the accounts carry. An
// account out of the pool is faint, as overviewGauge draws its windows and
// overviewNames its name.
func (f Frame) overviewBlock(doc status.Document, now time.Time) usageBlock {
	names := overviewNames(doc)
	b := blockOf(doc, now, f.Width, f.Look, f.By, nil, names)
	b.marks = keyMarks(names, f.phone())
	if f.By == ByAccount && f.Width >= matrixWidth {
		b.accountBar = besideBar - (b.names - matrixNames)
	}
	if f.Sections.Has(SectionGraphs) {
		b.focus = f.Focus
	}
	for i, n := range names {
		row := b.rows[i]
		for j, g := range row.gauges {
			row.gauges[j] = f.overviewGauge(g, Ref{Account: row.id, Window: b.windows[j].key}, n.out)
		}
	}
	return b
}

// overviewNames are the names of doc's accounts as the Overview names them
// wherever it does: as namesOf has them, but faint, number, label and marks,
// where an account is out of the pool.
func overviewNames(doc status.Document) []accountName {
	names := namesOf(doc)
	for i := range names {
		names[i].faint = names[i].out
	}
	return names
}

// overviewGauge is g, the window ref names, as the Overview draws it: where
// its account is out of the pool, its bar faded as a stale reading's is, the
// two fades compounding where it's stale too, and its percentage and words
// faint; its bar filled as far as it has eased, while it eases after a read;
// and, where its words say it runs out, those words cued on bg.attention,
// fading as its account's highlight does, while its change is picked out.
func (f Frame) overviewGauge(g gauge, ref Ref, out bool) gauge {
	if out {
		g.bar.out, g.words = true, g.words.faint()
	}
	if used, ok := f.Eased[ref]; ok {
		g.bar.eased = &used
	}
	if fade, ok := f.Changed[ref.Account]; ok && g.runsOut {
		g.cue = hue{token: theme.BgAttention, fade: fade}
	}
	return g
}

// keyMarks are the marks the accounts named carry, as USAGE's key line names
// them after the cap's: ◆ on the primary, and ▲ on the account new sessions
// go to, which a phone words short.
func keyMarks(names []accountName, phone bool) []line {
	var marks []line
	if slices.ContainsFunc(names, func(n accountName) bool { return n.primary }) {
		marks = append(marks, line{{"◆", primaryInk}, {" primary", mutedInk}})
	}
	if slices.ContainsFunc(names, func(n accountName) bool { return n.best }) {
		goes := " new sessions go here"
		if phone {
			goes = " new"
		}
		marks = append(marks, line{{"▲", accentInk}, {goes, mutedInk}})
	}
	return marks
}

// roomLeft is USAGE's description of doc: with one account, which has no
// pool, the room it has left in its 5-hour window and its week, each summed
// as the pool's room is, as in "room left  5h 0.4 of 1  ·  week 0.6 of 1";
// none on a phone, nor with more accounts, whose pool's row says the room.
func (f Frame) roomLeft(doc status.Document) line {
	if len(doc.Accounts) != 1 || f.phone() {
		return nil
	}
	var parts []line
	for _, w := range []struct{ key, word string }{{f.Policy.Started, "5h"}, {sharedWeek(f.Policy), "week"}} {
		if room, ok := doc.RoomOf(doc.Accounts[0], w.key, f.Policy); ok {
			parts = append(parts, line{{w.word + " ", dimInk}, {tenths(room), strongInk}, {" of 1", dimInk}})
		}
	}
	if parts == nil {
		return nil
	}
	return slices.Concat(line{{"room left  ", faintInk}}, joined(span{"  ·  ", faintInk}, parts...))
}

// usageAcross is the cells USAGE takes across the frame, its block b: by
// account from 150 columns, with ROUTING beside it, to where its heading
// ends, routingApart before ROUTING's; else the frame's width.
func (f Frame) usageAcross(b usageBlock) int {
	if b.accountBar > 0 && f.Sections.Has(SectionRouting) {
		return b.routingAt() - routingApart
	}
	return f.Width
}

// routingAt is the column ROUTING's blocks start at beside the block's, by
// account from 150 columns: hx = n + 10 + bar + 7 + 22 + 4, its words'
// cells and besideCells after them, 107 with a name column of 16.
func (b usageBlock) routingAt() int {
	return b.names + windowLabelCells + b.accountBar + afterBar + fullCells + besideCells
}

// enlarged is the block with its big bars, by window from 150 columns: its
// windows stacked as sections at the full width. Otherwise it's as it is.
func (b usageBlock) enlarged() usageBlock {
	if b.by == ByWindow && b.width >= matrixWidth {
		b.big = true
	}
	return b
}

// fitted is the block as USAGE draws it in rows rows, scrolled offset
// accounts' rows down: with its big bars, where they fit; else as it's laid
// out, scrolled within the rows where it doesn't fit.
func (b usageBlock) fitted(rows, offset int) usageBlock {
	if big := b.enlarged(); big.height() <= rows {
		return big
	}
	return b.scrolled(rows, offset)
}

// scrolling is how far the block scrolls in rows rows, an account's rows at a
// time: no further than shows the last account, a page being as many as
// show between ▲ and ▼; nowhere where it fits.
func (b usageBlock) scrolling(rows int) Scrolling {
	accounts, _ := b.split()
	n := len(accounts)
	if n < 2 || b.height() <= rows {
		return Scrolling{}
	}
	last := 1
	for last < n-1 && b.shows(n-last-1, last+1).height() <= rows {
		last++
	}
	return Scrolling{Most: n - last, Page: b.shownFrom(rows, 1)}
}

// scrolled is the block in rows rows, scrolled offset accounts' rows down, no
// further than scrolling says it goes, showing as many as fit, at least one.
func (b usageBlock) scrolled(rows, offset int) usageBlock {
	s := b.scrolling(rows)
	if s.Most == 0 {
		return b
	}
	first := min(max(offset, 0), s.Most)
	return b.shows(first, b.shownFrom(rows, first))
}

// shownFrom is how many accounts' rows show in rows rows, from the one at
// first: as many as fit, at least one.
func (b usageBlock) shownFrom(rows, first int) int {
	accounts, _ := b.split()
	shown := 1
	for shown < len(accounts)-first && b.shows(first, shown+1).height() <= rows {
		shown++
	}
	return shown
}

// shows is the block with above accounts' rows out of view above, shown in
// view, and the rest below.
func (b usageBlock) shows(above, shown int) usageBlock {
	accounts, _ := b.split()
	b.above, b.below = above, len(accounts)-above-shown
	return b
}

// drawIn draws the block in body, its key line after a blank row, cut to
// body's rows.
func (b usageBlock) drawIn(c *canvas, body bounds) {
	part := newCanvas(c.width, body.rows)
	b.key(part, b.draw(part, 0)+1)
	c.paste(part, 0, body.y, body.rows)
}

// placed is where the block draws each of its rows in view, the accounts'
// and the pool's, counted from its first row.
func (b usageBlock) placed() []rowAt {
	return b.lay(growing(b.width), 0).placed
}

// targets are the block's click targets as it's drawn in body: each row of
// an account's, width cells across, which moves the focus to it.
func (b usageBlock) targets(body bounds, width int) []Target {
	var targets []Target
	for _, p := range b.placed() {
		if p.id == "" {
			continue
		}
		for y := p.y; y < min(p.y+p.rows, body.rows); y++ {
			targets = append(targets, Target{X: body.x, Y: body.y + y, Width: width, On: OnAccount, Account: p.id})
		}
	}
	return targets
}

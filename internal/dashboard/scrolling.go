package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

var (
	// thumbInk is the part of a scrollbar its view shows, and railInk its
	// track.
	thumbInk = ink{token: theme.TextMuted}
	railInk  = ink{token: theme.TextFaint}
)

// Scrolling is how far a frame's view scrolls: Most, the furthest, in rows,
// or where it scrolls a thing at a time, as ScrollsRows says it doesn't, in
// those things, zero where it fits; and Page, how many of them show at once,
// as the frame has it scrolled.
type Scrolling struct {
	Most, Page int
}

// byRows is how far content rows tall scrolls in a view rows tall: as far
// as it runs past it, the view's rows at once.
func byRows(content, rows int) Scrolling {
	return Scrolling{Most: max(content-rows, 0), Page: rows}
}

// clamped is offset as far as what scrolls as s says goes: none before the
// first, and no further than Most.
func (s Scrolling) clamped(offset int) int {
	return min(max(offset, 0), s.Most)
}

// ScrollsRows reports whether the view shown scrolls a row at a time, as
// Runway, History's Year and a session's page's turns do, rather
// than a thing at a time: an account, as the Overview's USAGE does, and
// History's Weeks and Days, or a period, as its Tokens does.
func (f Frame) ScrollsRows() bool {
	if f.Session != nil {
		return true
	}
	switch f.View {
	case Overview:
		return false
	case History:
		return f.History.Shown == HistoryYear
	}
	return true
}

// Revealing is how far the Overview's USAGE of doc at now is scrolled to show
// the account with the given id: as far as the frame has it scrolled, where
// that shows it, else no further than it must go to. Scrolled down, a page
// may show fewer than the one it left, as USAGE's does once a ▲ row comes
// over it, so it goes on an account at a time until the account shows, the
// page laid out once for every account it goes on.
func (f Frame) Revealing(doc status.Document, now time.Time, id string) int {
	f = f.At(doc, now)
	m := f.Moment(doc, now)
	i := slices.IndexFunc(m.Accounts, func(a status.AccountNow) bool { return a.ID == id })
	scrolling := f.scrollingAt(doc, now)
	at := scrolling(f.Scroll).revealing(f.Scroll, i)
	for s := scrolling(at); at < s.Most && i >= at+s.Page; s = scrolling(at) {
		at++
	}
	return at
}

// revealing is how far what's scrolled at, and scrolls as s says, is
// scrolled to show its i-th account: no further than it must go to, and no
// further than it goes. An i of none of them leaves it where it is.
func (s Scrolling) revealing(at, i int) int {
	at = s.clamped(at)
	switch {
	case i < 0:
		return at
	case i < at:
		return i
	case i >= at+s.Page:
		return min(i-s.Page+1, s.Most)
	}
	return at
}

// Scrolling is how far the frame's view of doc scrolls at now, scrolled as
// the frame has it, as scrollingAt says.
func (f Frame) Scrolling(doc status.Document, now time.Time) Scrolling {
	return f.At(doc, now).scrollingAt(doc, now)(f.Scroll)
}

// scrollingAt is how far the frame's view of doc scrolls at now, scrolled to
// each offset given: a session's page's turns, where one is open, a row a
// turn and its turn open's rows under its own, as turnsRows lays them out,
// in the rows its layout gives them; History's, of the ledger's days alone,
// as historyScrolling says; the Overview's, as overviewScrolling says, laid
// out once for every offset; Accounts', as accountsScrolling says; and
// Runway, where its lanes don't fit, as runwayRows counts them, alike at
// every offset. A frame too short for the view, as cramped says, never
// scrolls, nor one of a document of no accounts, but History and a
// session's page.
func (f Frame) scrollingAt(doc status.Document, now time.Time) func(offset int) Scrolling {
	m := f.Moment(doc, now)
	var s Scrolling
	switch top := f.top(); {
	case f.cramped():
	case f.Session != nil:
		l := f.sessionLayout(len(f.Session.Page.Accounts))
		s = byRows(len(f.Session.turnsRows(l)), l.turns)
	case f.View == History:
		return f.historyScrolling(top)
	case len(m.Accounts) == 0:
	case f.View == Overview:
		return f.overviewScrolling(doc, now)
	case f.View == Accounts:
		return f.accountsScrolling(doc, now)
	case f.View == Runway:
		content, view := f.runwayRows(len(m.Accounts), top)
		s = Scrolling{Most: content - view, Page: view}
	}
	return func(int) Scrolling { return s }
}

// outForm is a form of the line saying what's out of view where a view
// scrolls: what each count leads with in place of its mark, ▲ or ▼, and the
// space after it, as "+" in "+2 more", none where it leads with its mark;
// the thing it counts, as "account", none where a count stands alone, as "▼
// 3 more"; the words after a count of those above, and of those below, as "
// above" and " below", none where it doesn't say where they are; the gap
// between its parts; the keys that scroll the view, as "j/k", none where it
// doesn't say, and what they do; and its inks.
type outForm struct {
	lead            string
	noun            string
	above, below    string
	gap, keys, does string
	inks            outInks
}

// outInks are the inks of a line saying what's out of view: its marks, ▲ and
// ▼, and the space after each; its counts and where they are; the gaps
// between its parts; its keys; and what they do.
type outInks struct{ mark, space, count, gap, keys, does ink }

var (
	// keyedOut is how History inks its scroll lines, as the design gives
	// them: the marks in accent.key, the words in text.muted, the gaps in
	// text.faint, and the keys bold accent.key.
	keyedOut = outInks{mark: keyMarkInk, space: mutedInk, count: mutedInk, gap: faintInk, keys: keyInk, does: mutedInk}
	// subtleOut is how a scroll line among the rows it scrolls is inked, as
	// USAGE's and the routing card's are: all in text.subtle.
	subtleOut = outInks{mark: dimInk, space: dimInk, count: dimInk, gap: dimInk, keys: dimInk, does: dimInk}
)

// rowsOut is how Runway says what's out of view until its stage draws it as
// the design does: "▼ 2 more accounts below · j/k or wheel to scroll", the
// marks, and the space after each, bold accent.key, the counts in
// text.muted and the rest in text.subtle, cut short as words where it
// doesn't fit, as line.fit cuts it.
var rowsOut = outForm{
	noun: "account", above: " above", below: " below", gap: " · ", keys: "j/k", does: " or wheel to scroll",
	inks: outInks{mark: keyInk, space: keyInk, count: mutedInk, gap: dimInk, keys: dimInk, does: dimInk},
}

// outOfView says what's out of view where a view scrolls, above and below,
// in the longest of forms, each shorter than the one before, that fits width
// cells; where none does, as the last says one count alone, below where any
// are, as what's below is what a line cut short loses, else above, cut to
// width cells: its count a figure, drawn whole with its mark, and where that
// doesn't fit, the mark alone, as a count cut short would say fewer. It says
// nothing where nothing is out of view.
func outOfView(above, below, width int, forms ...outForm) line {
	if above+below == 0 {
		return nil
	}
	for _, form := range forms {
		if said := form.said(above, below); said.width() <= width {
			return said
		}
	}
	last := forms[len(forms)-1]
	one := last.parts(0, below)
	if below == 0 {
		one = last.parts(above, 0)
	}
	if mark := one[0].said[0]; partsWidth(one[:2]) > width {
		return line{mark}.fit(width)
	}
	return fitParts(width, one...)
}

// said is what's out of view, above and below, as the form says it, whole:
// each run of one ink a span.
func (o outForm) said(above, below int) line {
	var said line
	for _, p := range o.parts(above, below) {
		said = append(said, p.drawn()...)
	}
	return said.inked()
}

// parts are what's out of view, above and below, as the form says it: each
// count where any are out of view there, after its mark and a space, or the
// form's lead, its count a figure, then where they are; then the keys that scroll the
// view, and what they do; each part gap apart.
func (o outForm) parts(above, below int) []part {
	return joinedParts(span{o.gap, o.inks.gap}, o.counted("▲", above, o.above), o.counted("▼", below, o.below), o.scrolled())
}

// counted is n out of view, after the mark given and a space, or what the
// form leads a count with instead, as the form counts them, where says where
// they are: none where n is.
func (o outForm) counted(mark string, n int, where string) []part {
	if n == 0 {
		return nil
	}
	k := o.inks
	lead := line{{mark, k.mark}, {" ", k.space}}
	if o.lead != "" {
		lead = line{{o.lead, k.mark}}
	}
	counted := slices.Concat([]part{wordsPart(lead)}, moreParts(n, o.noun, k.count))
	if where != "" {
		counted = append(counted, wordsIn(where, k.count))
	}
	return counted
}

// scrolled is the keys that scroll the view and what they do, as the form
// says them: none where it names no keys.
func (o outForm) scrolled() []part {
	if o.keys == "" {
		return nil
	}
	scrolled := []part{wordsIn(o.keys, o.inks.keys)}
	if o.does != "" {
		scrolled = append(scrolled, wordsIn(o.does, o.inks.does))
	}
	return scrolled
}

// moreParts count n more of the thing noun names, as "4 more accounts", or
// n more alone, as "3 more", where it names none, in the ink given: the
// count a figure.
func moreParts(n int, noun string, k ink) []part {
	if noun == "" {
		return []part{figureIn(prose.Grouped(n), k), wordsIn(" more", k)}
	}
	return countedParts(n, "more "+noun, k)
}

// scrollbar draws a scrollbar down column x from row y, rows tall, for
// content rows that scroll, shown from the row offset: ┃ the part shown,
// as long as the share it shows, on a │ track, the whole of it.
func scrollbar(c *canvas, x, y, rows, content, offset int) {
	if rows < 1 || content <= rows {
		return
	}
	thumb := max(1, rows*rows/content)
	most := content - rows
	start := (offset*(rows-thumb) + most/2) / most
	for i := range rows {
		if i >= start && i < start+thumb {
			c.text(x, y+i, "┃", thumbInk)
		} else {
			c.text(x, y+i, "│", railInk)
		}
	}
}

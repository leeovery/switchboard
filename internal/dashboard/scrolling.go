package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// Scrolling is how far a frame's view scrolls: Most, the furthest, in rows,
// or where it scrolls a thing at a time, as ScrollsRows says it doesn't, in
// those things, zero where it fits; Page, how many of them show at once, as
// the frame has it scrolled, and how far a page down goes; and Back, how far
// a page up goes, where that's other than Page, as Paged has them.
type Scrolling struct {
	Most, Page, Back int
}

// byRows is how far content rows tall scrolls in a view rows tall: as far
// as it runs past it, the view's rows at once.
func byRows(content, rows int) Scrolling {
	return Scrolling{Most: max(content-rows, 0), Page: rows}
}

// Paged is how far what's scrolled at, and scrolls as s says, is scrolled a
// page on, down where pages is more than zero, up where less: Page down, and
// Back up, or Page where Back is zero, as it is wherever a page up goes as
// far as a page down; no further either way than it goes.
func (s Scrolling) Paged(at, pages int) int {
	page := s.Page
	if pages < 0 && s.Back > 0 {
		page = s.Back
	}
	return s.clamped(s.clamped(at) + pages*page)
}

// clamped is offset as far as what scrolls as s says goes: none before the
// first, and no further than Most.
func (s Scrolling) clamped(offset int) int {
	return min(max(offset, 0), s.Most)
}

// leastPaged is the fewest rows a view shown a row at a time keeps over a
// foot it can do without, as USAGE's key line and Tokens' footnote: a row of
// it, and the row saying what's out of view.
const leastPaged = 2

// headed is a heading's row and the row of the first thing under it, which
// a view scrolled a row at a time shows together, or the heading not at all.
type headed struct {
	at, first int
}

// keptHeading is a heading's row, and the row after the last of those under
// it, which a view scrolled a row at a time keeps on its page's first row
// while it's scrolled past the heading into them, as USAGE keeps its
// windows'.
type keptHeading struct {
	at, end int
}

// rowsPaged is a view's rows laid out whole, content of them, shown a row at
// a time in rows rows: a page of them, as many as fit over its foot rows,
// foot of them, held on its last rows with a blank row over them, where it
// keeps any; but a page ending between a heading and the first row under it,
// as heads holds them in the order of their rows, ends over the heading,
// unless the heading is its first row. Where says has it, the row after the
// page says how many rows are out of view above it and below: the blank row
// over the foot, or where there's none, the last row, the page a row shorter
// where it scrolls, so long as that leaves it a row. A page scrolled past a
// heading of kept, in the order of their rows, into the rows under it, shows
// the heading on its first row in place of the row there, as keeps has it.
type rowsPaged struct {
	rows, foot, content int
	heads               []headed
	kept                []keptHeading
	says                bool
}

// page is how many rows show at once, at most.
func (p rowsPaged) page() int {
	page := max(p.rows-p.foot-min(p.foot, 1), 0)
	if p.says && p.foot == 0 && p.content > page && page > 1 {
		page--
	}
	return page
}

// shown are the rows shown scrolled offset rows down, no further than shows
// the last: from the first, up to to.
func (p rowsPaged) shown(offset int) (from, to int) {
	from = byRows(p.content, p.page()).clamped(offset)
	return from, p.end(from)
}

// end is the row after the last shown from row from: a page on, or the
// content's end, but over a heading whose first row that leaves out, where
// it has one.
func (p rowsPaged) end(from int) int {
	to := min(from+p.page(), p.content)
	for _, h := range slices.Backward(p.heads) {
		if from < h.at && h.at < to && h.first >= to && h.first < p.content {
			to = h.at
		}
	}
	return to
}

// keeps is the row of the heading of kept the page shown from row from up
// to to shows on its first row: the one whose rows that row and the next are
// among, below the heading's own, so a row of its own shows under it; none,
// false, where it's no such row, or the page has fewer than two rows under it
// to show, as a heading and its first row there need.
func (p rowsPaged) keeps(from, to int) (int, bool) {
	if to-from < 3 {
		return 0, false
	}
	for _, k := range p.kept {
		if k.at < from && from+1 < k.end {
			return k.at, true
		}
	}
	return 0, false
}

// top is the first of the content's rows the page shown from row from up to
// to shows: from, or the row after, where a heading kept takes its place.
func (p rowsPaged) top(from, to int) int {
	if _, ok := p.keeps(from, to); ok {
		return from + 1
	}
	return from
}

// pagedWhole is how a view drawn whole on c pages in rows rows, a row at a
// time, each heading marked on it shown with its first row.
func pagedWhole(c *canvas, rows int) rowsPaged {
	return rowsPaged{rows: rows, content: c.height(), heads: c.headings()}
}

// scrolling is how far the rows scroll, a row at a time, scrolled offset
// down: as far as shows the last; a page down going as far as down has it,
// and a page up as far as up has it, so paging either way passes over no
// row.
func (p rowsPaged) scrolling(offset int) Scrolling {
	from, to := p.shown(offset)
	return pagedFrom(max(p.content-p.page(), 0), from, p.down(from, to), p.up(from, to))
}

// pagedFrom is how far what's shown from from, and scrolls no further than
// most, scrolls: a page down to down, and a page up to up, Back where that's
// other than a page down's length up.
func pagedFrom(most, from, down, up int) Scrolling {
	s := Scrolling{Most: most, Page: down - from}
	if up != max(from-s.Page, 0) {
		s.Back = from - up
	}
	return s
}

// pageDown is where a page down from the page shown from from starts, next
// being the first row, or thing, it didn't show: as far down as shows next,
// no further than next, as shows has it; next where none after from does.
func pageDown(from, next int, shows func(at, r int) bool) int {
	for at := next; at > from; at-- {
		if shows(at, next) {
			return at
		}
	}
	return next
}

// pageUp is where a page up from the page shown from from starts, above
// being the row, or thing, above the first it showed: as far up as shows
// above, as shows has it, so the page ends at above, the search bounded to
// least on; a step up where none before from does.
func pageUp(least, from, above int, shows func(at, r int) bool) int {
	for at := max(least, 0); at < from; at++ {
		if shows(at, above) {
			return at
		}
	}
	return max(from-1, 0)
}

// thingsPaged is how far things shown whole, a thing at a time, scroll,
// scrolled offset of them down, no further than most: a page down starting
// at the first the page didn't show, and a page up ending at the one above
// the first it showed, as pageDown and pageUp have them, shown(at) of them
// showing scrolled to the at-th.
func thingsPaged(most, offset int, shown func(at int) int) Scrolling {
	from := min(max(offset, 0), most)
	shows := func(at, i int) bool {
		at = min(at, most)
		return at <= i && i < at+shown(at)
	}
	return pagedFrom(most, from, pageDown(from, from+shown(from), shows), pageUp(0, from, from-1, shows))
}

// shows reports whether the page scrolled offset down shows the content's
// row r: not where a heading kept takes its place.
func (p rowsPaged) shows(offset, r int) bool {
	from, to := p.shown(offset)
	return p.top(from, to) <= r && r < to
}

// down is where a page down from the page shown from row from up to to
// starts: at the first row it didn't show, or as little short of it as
// shows it, where a heading kept would take its place, as pageDown has it; a
// page on where the content ends within it.
func (p rowsPaged) down(from, to int) int {
	if to >= p.content {
		return from + p.page()
	}
	return pageDown(from, to, p.shows)
}

// up is where a page up from the page shown from row from up to to starts:
// as far up as shows the row above the first it showed, a row a heading
// kept takes counting as not shown, as pageUp has it; a row up where none
// does.
func (p rowsPaged) up(from, to int) int {
	above := p.top(from, to) - 1
	return pageUp(above-p.page()+1, from, above, p.shows)
}

// paste draws the rows of whole, the view's rows laid out whole, shown
// scrolled offset down, from row y, and under them, where it says, how many
// are out of view, as said has it.
func (p rowsPaged) paste(c, whole *canvas, y, offset, width int, forms ...outForm) {
	from, to := p.shown(offset)
	c.paste(whole, from, y, to-from)
	p.said(c, y, from, to, width, forms...)
}

// said draws, where the rows say it and any from from up to to are out of
// view, how many are, above and below, as outOfView says it in width cells,
// on the row after the page, the view's first row being y.
func (p rowsPaged) said(c *canvas, y, from, to, width int, forms ...outForm) {
	page := p.page()
	if above, below := from, p.content-to; p.says && above+below > 0 && page > 0 && page < p.rows-p.foot {
		c.line(0, y+page, outOfView(above, below, width, forms...))
	}
}

// revealing is how far the rows, scrolled offset down, are scrolled to show
// one of the spans given, in the order of their rows: where they are, while
// any shows; else no further than it must go to, down till the first below
// the page shows, or up till the last above it is the first the page shows,
// under a heading kept where one is, whichever is nearer.
func (p rowsPaged) revealing(offset int, spans []rowAt) int {
	from, to := p.shown(offset)
	top := p.top(from, to)
	if slices.ContainsFunc(spans, func(s rowAt) bool { return s.y < to && s.y+s.rows > top }) {
		return from
	}
	up, down := -1, -1
	for _, s := range spans {
		if s.y < top {
			up = s.y
		}
	}
	for row := up; up > 0 && !p.shows(up, row); {
		up--
	}
	if i := slices.IndexFunc(spans, func(s rowAt) bool { return s.y >= to }); i >= 0 {
		most := max(p.content-p.page(), 0)
		for down = from; down < most && !p.shows(down, spans[i].y); {
			down++
		}
	}
	switch {
	case up < 0 && down < 0:
		return from
	case up < 0 || down >= 0 && down-from <= from-up:
		return down
	}
	return up
}

// ScrollsRows reports whether the view shown of doc at now scrolls a row at a
// time, as the Overview's USAGE, History's Year, its Weeks and Days where
// they scroll whole and Tokens where a period doesn't fit, Sessions' List and
// a session's page's turns do, rather than a thing at a time: an account, as
// History's Weeks and Days, Sessions' Switchboard and Runway's lanes do, or a
// period, as Tokens does.
func (f Frame) ScrollsRows(doc status.Document, now time.Time) bool {
	switch f.Shown() {
	case Runway:
		return false
	case History:
		return f.historyScrollsRows()
	case Sessions:
		return f.SessionsTab.Shown == SessionsList || f.sessionsForm(doc, now).byRows
	}
	return true
}

// Revealing is how far the Overview's USAGE of doc at now is scrolled to show
// the account with the given id, as its revealing has it, in the rows it's
// laid out in: none where it doesn't scroll, as on a frame too short for the
// view.
func (f Frame) Revealing(doc status.Document, now time.Time, id string) int {
	f = f.At(doc, now)
	usage := usageOf(f.laidOut(doc, now, f.top()))
	if f.cramped() || usage.revealing == nil {
		return 0
	}
	return usage.revealing(usage.body.rows, f.Scroll, id)
}

// revealing is how far what's scrolled at, and scrolls as s says, is
// scrolled to show its i-th account, as Keeping keeps it in view.
func (s Scrolling) revealing(at, i int) int {
	return s.Keeping(at, i, 0)
}

// Keeping is how far what's scrolled at, and scrolls as s says, is scrolled
// to keep its rows from i to i+under in view, row i where they don't all
// fit: no further than it must go to, and no further than it goes. An i of
// none of them, under 0, leaves it where it is, as does a view of no rows.
func (s Scrolling) Keeping(at, i, under int) int {
	at = s.clamped(at)
	switch {
	case i < 0 || s.Page <= 0:
		return at
	case i < at:
		return i
	case i+under >= at+s.Page:
		return s.clamped(min(i+under-s.Page+1, i))
	}
	return at
}

// Scrolling is how far the frame's view of doc scrolls at now, scrolled as
// the frame has it, as scrollingAt says.
func (f Frame) Scrolling(doc status.Document, now time.Time) Scrolling {
	return f.At(doc, now).scrollingAt(doc, now)(f.Scroll)
}

// KeysScroll reports whether j and k, PgUp and PgDn and the wheel scroll
// anything of the frame's view of doc at now: with a session's turn open, its
// requests, as RequestsScrolling has it, which they scroll in its turns'
// place; else the view, as Scrolling has it.
func (f Frame) KeysScroll(doc status.Document, now time.Time) bool {
	if f.Shown() == SessionPage && f.Session.Open != 0 {
		return f.RequestsScrolling().Most > 0
	}
	return f.Scrolling(doc, now).Most > 0
}

// TurnsScrolling is how far the turns of the frame's session's page scroll:
// a row a turn and its turn open's rows under its own, as turnsRows lays
// them out, in the rows its layout gives them, of the frame's size and the
// page alone; none without a page, or where the frame is too short for its
// turns, as cramped says.
func (f Frame) TurnsScrolling() Scrolling {
	if f.Session == nil {
		return Scrolling{}
	}
	return f.Session.turnsScrolling(f.pageLayout())
}

// turnsScrolling is how far the turns of the page s shows scroll in the
// layout l, as TurnsScrolling has it.
func (s *SessionShown) turnsScrolling(l sessionLayout) Scrolling {
	if l.chrome {
		return Scrolling{}
	}
	return byRows(len(s.turnsRows(l)), l.turns)
}

// scrollingAt is how far the frame's view of doc scrolls at now, scrolled to
// each offset given: a session's page's turns, where one is open, as
// TurnsScrolling has them; History's, of the ledger's days alone, as
// historyScrolling says; Sessions', as sessionsScrolling says; the
// Overview's, as overviewScrolling says, laid out once for every offset;
// Accounts', as accountsScrolling says; and Runway's, as runwayScrolling
// says. A frame too short
// for the view, as cramped says, never scrolls, nor one of a document of no
// accounts, but History, Sessions, whose List lists the ledger's sessions
// whatever the document holds, and a session's page.
func (f Frame) scrollingAt(doc status.Document, now time.Time) func(offset int) Scrolling {
	shown := f.Shown()
	if shown == SessionPage {
		s := f.TurnsScrolling()
		return func(int) Scrolling { return s }
	}
	m := f.Moment(doc, now)
	var s Scrolling
	switch top := f.top(); {
	case f.cramped():
	case shown == History:
		return f.historyScrolling(top)
	case shown == Sessions:
		return f.sessionsScrolling(doc, now)
	case len(m.Accounts) == 0:
	case shown == Overview:
		return f.overviewScrolling(doc, now)
	case shown == Accounts:
		return f.accountsScrolling(doc, now)
	case shown == Runway:
		return f.runwayScrolling(doc, now)
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

// outOfView says what's out of view where a view scrolls, above and below,
// in the first that fits width cells of: both counts with the keys, in each
// of forms, each shorter than the one before; both counts without them; one
// count alone, below where any are, as what's below is what a line cut short
// loses, else above, with the keys, then without. Where none fits, it's the
// last of those cut to width cells: its count a figure, drawn whole with its
// mark, and where that doesn't fit, the mark alone, as a count cut short
// would say fewer. It says nothing where nothing is out of view.
func outOfView(above, below, width int, forms ...outForm) line {
	if above+below == 0 {
		return nil
	}
	alone := [2]int{0, below}
	if below == 0 {
		alone = [2]int{above, 0}
	}
	tried := slices.Clip(forms)
	for _, form := range forms {
		tried = append(tried, form.keyless())
	}
	for _, n := range [][2]int{{above, below}, alone} {
		for _, form := range tried {
			if said := form.said(n[0], n[1]); said.width() <= width {
				return said
			}
		}
	}
	one := tried[len(tried)-1].parts(alone[0], alone[1])
	if mark := one[0].said[0]; partsWidth(one[:2]) > width {
		return line{mark}.fit(width)
	}
	return fitParts(width, one...)
}

// keyless is the form without its keys and what they do.
func (o outForm) keyless() outForm {
	o.keys, o.does = "", ""
	return o
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

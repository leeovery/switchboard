package dashboard

import (
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// The cells of USAGE by account from 150 columns, beside ROUTING: its bars
// are besideBar cells with a name column of matrixNames, a cell fewer for
// each cell the column is wider, and ROUTING's blocks start besideCells after
// its words.
const (
	besideBar   = 50
	besideCells = 4
)

// usageSection is the Overview's USAGE of doc at now: usage's block, as
// overviewBlock has it. At its least it's the block as usage lays it out at
// the frame's width, which from 150 columns, by window, is the compact
// matrix, 2a + 7 rows for a accounts and the pool; at its most, from 150
// columns, by window, the big bars, its windows stacked as sections at the
// full width; by account with GRAPHS' charts beside its blocks, the charts at
// 5 rows, every one, and between, the stacked step, as stackedBeside has it;
// otherwise its least. Short of its least, it gives way in steps, as giving
// has them, the planner taking each as a step of its least, as leaner has
// them, but never folds: given fewer rows than its form takes, it scrolls
// within them, as shown shows it. By account from 150 columns, GRAPHS'
// charts or ROUTING are drawn beside its blocks, from besideAt, as
// besideUsage has them, and follow them as they scroll; a click along an
// account's rows, its own or what's beside them, moves the focus to it.
func (f Frame) usageSection(doc status.Document, now time.Time) section {
	views := f.overviewBlock(doc, now).views()
	beside := f.besideUsage(doc, now)
	s := section{
		Section: SectionUsage, least: sectionHead + views.all, most: sectionHead + views.big,
		between:     views.between(),
		leaner:      views.leanerRows(),
		showing:     sectionHead + views.showsFrom(),
		description: func() line { return f.roomLeft(doc, now) },
		keyed:       true,
	}
	if beside != nil {
		s.aside = views.block.besideAt()
	}
	s.leaned = func(given giving) section { return f.usageIn(s, views.given(given), beside) }
	return f.usageIn(s, views, beside)
}

// usageIn is USAGE as s has it, its block drawn in the form of the views
// given, what's beside its blocks drawn beside them: its heading's rows as
// the form's headed has them, and its body, its targets and how it scrolls
// the form's.
func (f Frame) usageIn(s section, form usageViews, beside func(*canvas, blocksBeside)) section {
	s.heads = form.block.headed()
	s.draw = func(c *canvas, body bounds) {
		shown := form.shown(body.rows, f.Scroll)
		shown.draw(c, body)
		if beside != nil {
			beside(c.within(shown.rowsIn(body)), shown.beside(s.aside, body))
		}
	}
	s.targets = func(body bounds) []Target {
		return form.shown(body.rows, f.Scroll).targets(body)
	}
	s.scrolling, s.rowsAtATime, s.revealing = form.scrolling, form.whole, form.revealing
	return s
}

// overviewBlock is usage's block of doc at now as the Overview's USAGE draws
// it, grouped as the frame's By has it: as usage prints it, but for what the
// Overview alone has. By account from 150 columns, its bars leave room for
// GRAPHS' charts or ROUTING beside them, its blocks a row further apart for
// the charts' axes, where they're the charts. While GRAPHS shows, the matrix
// picks out the account the focus is on. Its key line names the marks the
// accounts carry. An account out of the pool at now is faint, as
// overviewGauge draws its windows and overviewNames its name.
func (f Frame) overviewBlock(doc status.Document, now time.Time) usageBlock {
	m := f.Moment(doc, now)
	names := overviewNames(m)
	b := blockOf(m, f.Width, f.Look, f.By, f.overviewWindows(doc, now), names)
	b.marks = keyMarks(names)
	if f.By == ByAccount && f.Width >= matrixWidth {
		b.accountBar = besideBar - (b.names - matrixNames)
	}
	b.charted = f.graphsBeside()
	if f.Sections.Has(SectionGraphs) {
		b.focus = f.Focus
	}
	for i, row := range b.rows {
		out := i < len(names) && names[i].out
		for j, g := range row.gauges {
			row.gauges[j] = f.overviewGauge(g, out)
		}
	}
	return b
}

// overviewWindows are the windows of doc at now the Overview shows, in USAGE
// and GRAPHS, as listedWindows has them.
func (f Frame) overviewWindows(doc status.Document, now time.Time) []shownWindow {
	return f.listedWindows(f.Moment(doc, now))
}

// listedWindows are the windows of m the live views list, as m's Listed has
// them while the frame's Models shows each model's own or not, each named as
// windowNamer names it.
func (f Frame) listedWindows(m status.Moment) []shownWindow {
	return usageWindows(m.Listed(f.Models), f.windowNamer(m))
}

// windowNamer names m's windows by their keys as the Overview names them
// wherever it does: as the frame's WindowName has it, else as m names them,
// as windowNamed has them.
func (f Frame) windowNamer(m status.Moment) func(key string) string {
	if f.WindowName != nil {
		return f.WindowName
	}
	return func(key string) string { return windowNamed(m, key).Name }
}

// overviewNames are the names of m's accounts as the Overview names them
// wherever it does: as namesOf has them, but faint, number, label and marks,
// where an account is out of the pool.
func overviewNames(m status.Moment) []accountName {
	names := namesOf(m)
	for i := range names {
		names[i].faint = names[i].out
	}
	return names
}

// overviewNamed is the account of m with the given id as the Overview names
// it in a line, in k: its label, as names, overviewNames' of m, have it,
// faint where it's out of the pool; where m lacks it, its id, cleaned.
func overviewNamed(m status.Moment, names []accountName, id string, k ink) span {
	if i := place(m, id); i > 0 {
		return span{names[i-1].label, names[i-1].inked(k)}
	}
	return span{prose.Clean(id), k}
}

// overviewGauge is g, an account's window or the pool's, as the Overview
// draws it: where its account is out of the pool, its bar faded as a stale
// reading's is, the two fades compounding where it's stale too, and its
// percentage and words faint; its bar filled as far as it has eased, while
// it eases after a read; and its words cued while their cue for turning to
// out ~ fades back, as the frame's TurnedOut says, where they say it still.
func (f Frame) overviewGauge(g gauge, out bool) gauge {
	if out {
		g.bar.out, g.words = true, g.words.faint()
	}
	if used, ok := f.Eased[g.of]; ok {
		g.bar.eased = &used
	}
	if fade, ok := f.TurnedOut[g.of]; ok && g.runsOut {
		g.cue = &fade
	}
	return g
}

// keyMarks are the marks the accounts named carry, as USAGE's key line names
// them after the cap's, in their forms, the widest first: ◆ on the primary,
// and ▲ on the account new sessions go to, worded as each of bestWords.
func keyMarks(names []accountName) [][]line {
	primary := slices.ContainsFunc(names, func(n accountName) bool { return n.primary })
	best := slices.ContainsFunc(names, func(n accountName) bool { return n.best })
	forms := make([][]line, len(bestWords))
	for i, goes := range bestWords {
		if primary {
			forms[i] = append(forms[i], line{{"◆", primaryInk}, {" primary", mutedInk}})
		}
		if best {
			forms[i] = append(forms[i], line{{"▲", accentInk}, {goes, mutedInk}})
		}
	}
	return forms
}

// bestWords are the key line's words after ▲, the widest first.
var bestWords = []string{" new sessions go here", " new"}

// roomLeft is USAGE's description of doc at now: with one account, which has
// no pool, the room it has left in its week and its 5-hour window at now,
// where it has them, in the order m lists windows, as its CompareWindows has
// it, as their Room has it, and numbered as its window's room, as in
// "room left  week 0.6 of 1  ·  5h 0.4 of 1"; none with more accounts, whose
// pool's row says the room. Each window's room is an item, as a key's parts
// are, a blank span before the · after the first, so where the heading has
// no room for them all, they give way whole from the last, as fitItems cuts
// them.
func (f Frame) roomLeft(doc status.Document, now time.Time) line {
	m := f.Moment(doc, now)
	if len(m.Accounts) != 1 {
		return nil
	}
	a := m.Accounts[0]
	type room struct {
		window status.WindowNow
		word   string
	}
	var rooms []room
	if session, ok := a.Window(m.Policy().Started); ok {
		rooms = append(rooms, room{session, "5h"})
	}
	if week, ok := sharedWeek(a); ok {
		rooms = append(rooms, room{week, "week"})
	}
	slices.SortFunc(rooms, func(a, b room) int { return m.CompareWindows(a.window.Key, b.window.Key) })
	var parts []line
	for i, r := range rooms {
		left := span{prose.Tenths(r.window.Room), strongInk}.numbered(Number{Kind: NumberRoom, Account: a.ID, Of: r.window.Key})
		part := line{{r.word + " ", dimInk}, left, {" of 1", dimInk}}
		if i > 0 {
			part = slices.Concat(line{{"·  ", faintInk}}, part)
		}
		parts = append(parts, part)
	}
	if parts == nil {
		return nil
	}
	return slices.Concat(line{{"room left  ", faintInk}}, joined(span{"  ", faintInk}, parts...))
}

// besideAt is the column what's beside the block's blocks starts at, by
// account from 150 columns, GRAPHS' charts or ROUTING's lines: hx = n + 10 +
// bar + 7 + 22 + 4, its words' cells and besideCells after them, 107 with a
// name column of 16.
func (b usageBlock) besideAt() int {
	return b.names + windowLabelCells + b.accountBar + afterBar + fullCells + besideCells
}

// enlarged is the block at its most, where it grows, as enlarges says: with
// its big bars; or by account, GRAPHS' charts beside its blocks every one, at
// leastChart rows, as besideTall has them. Otherwise it's as it is.
func (b usageBlock) enlarged() usageBlock {
	switch {
	case b.charted:
		b.beside = besideTall
	case b.enlarges():
		b.big = true
	}
	return b
}

// stackedBeside is the block by account a step past its least, GRAPHS'
// charts beside its blocks every one, at leastBeside rows, as besideStacked
// has them; or where none sit there, as it is.
func (b usageBlock) stackedBeside() usageBlock {
	if b.charted {
		b.beside = besideStacked
	}
	return b
}

// enlarges reports whether the block grows: by window from 150 columns, to
// its big bars, its windows stacked as sections at the full width; and by
// account, where GRAPHS' charts sit beside its blocks, apart for them.
func (b usageBlock) enlarges() bool {
	return b.by == ByWindow && b.width >= matrixWidth || b.charted
}

// usageViews are the block's views, as USAGE shows it in however many rows
// it's given, scrolled however far: the block; the rows it takes with its big
// bars, and as it's laid out; how many accounts it has; and, worked out only
// once it scrolls, the rows each account in view adds, and each mark, ▲ or
// ▼, as of counts them, and the block laid out whole, as paged pages it.
// Where it's short of rows, its leaner views are the block as it gives way, a
// step at a time, as giving has them.
type usageViews struct {
	block             usageBlock
	big, stacked, all int
	accounts          int
	steps             func() (account, mark int)
	laid              func() laid
	leaner            []usageViews
}

// views are the block's views, laid out once as it is, and once more with
// its big bars where it has them, as enlarges says; by account, with GRAPHS'
// charts beside its blocks, stacked, as stackedBeside has them; and once as
// it is at each step it gives way.
func (b usageBlock) views() usageViews {
	v := b.viewsAsLaid()
	if b.enlarges() {
		v.big = b.enlarged().height()
	}
	if b.charted {
		v.stacked = b.stackedBeside().height()
	}
	for given := givenApart; given <= givenKey; given++ {
		lean := b
		lean.given = given
		v.leaner = append(v.leaner, lean.viewsAsLaid())
	}
	return v
}

// viewsAsLaid are the block's views as it's laid out, and in no other form.
func (b usageBlock) viewsAsLaid() usageViews {
	accounts, _ := b.split()
	all := b.height()
	return usageViews{
		block: b, big: all, stacked: all, all: all, accounts: len(accounts),
		steps: sync.OnceValues(func() (int, int) { return b.steps(all) }),
		laid:  sync.OnceValue(func() laid { return b.lay(growing(b.width), 0) }),
	}
}

// given are the block's views as far as given has it give way: as it is, or
// the leaner view of that step.
func (v usageViews) given(given giving) usageViews {
	if given == givenNone {
		return v
	}
	return v.leaner[given-1]
}

// headed are the rows USAGE's heading takes over the block in its form: its
// own and the blank row under it; but its own alone where the block has
// given way the blank rows under its windows' headings, as the blank under
// USAGE's goes with them.
func (b usageBlock) headed() int {
	if b.given >= givenHeads {
		return 1
	}
	return sectionHead
}

// showsFrom is the fewest rows the block shows an account in, as it's laid
// out, scrolled to its top: an account's row among its targets.
func (v usageViews) showsFrom() int {
	for rows := 1; rows < v.all; rows++ {
		if len(v.shown(rows, 0).targets(bounds{rows: rows})) > 0 {
			return rows
		}
	}
	return v.all
}

// leanerRows are the rows USAGE takes, its heading's among them, as headed
// has them, at each step its block gives way, the planner stepping it short
// of its least as its rows run short.
func (v usageViews) leanerRows() []int {
	rows := make([]int, len(v.leaner))
	for i, lean := range v.leaner {
		rows[i] = lean.block.headed() + lean.all
	}
	return rows
}

// between are the rows, its heading's among them, USAGE grows to whole past
// its least short of its most: its blocks by account with the charts beside
// them stacked, where that's a step of its own.
func (v usageViews) between() []int {
	if v.all < v.stacked && v.stacked < v.big {
		return []int{sectionHead + v.stacked}
	}
	return nil
}

// usageDrawn is USAGE's block of doc at now as USAGE draws it in the rows the
// Overview gives it, as fitted has it, scrolled as the frame is.
func (f Frame) usageDrawn(doc status.Document, now time.Time) usageBlock {
	views := f.overviewBlock(doc, now).views()
	for _, u := range f.allotted(doc, now) {
		if u.ledBy(SectionUsage) && !u.fold {
			form := views.given(u.lean)
			return form.fitted(u.rows-form.block.headed(), f.Scroll)
		}
	}
	return views.block
}

// steps are the rows the block, all rows tall with every account in view,
// takes for each account in view, and for each mark, ▲ or ▼: worked out from
// two more layouts of it at most, as every account's rows are alike, and ▲
// and ▼ take a row each in each window's section: all but the last account
// in view, and all but the last two. With two accounts, a view of one has a
// mark, so the account's rows count the mark's.
func (b usageBlock) steps(all int) (account, mark int) {
	accounts, _ := b.split()
	n := len(accounts)
	if n < 2 {
		return 0, 0
	}
	oneBelow := b.shows(0, n-1).height()
	account = all - oneBelow
	if n > 2 {
		account = oneBelow - b.shows(0, n-2).height()
		mark = oneBelow + account - all
	}
	return account, mark
}

// of is how many rows the block takes, as height has them, with above
// accounts' rows out of view above, shown in view, and the rest below.
func (v usageViews) of(above, shown int) int {
	account, mark := v.steps()
	marks := 0
	if above > 0 {
		marks++
	}
	if v.accounts-above-shown > 0 {
		marks++
	}
	return v.all - (v.accounts-shown)*account + marks*mark
}

// whole reports whether the block, in rows rows, is shown whole, a row at a
// time, as paged pages it: where it doesn't fit, nor does it scrolled an
// account at a time, an account a window in view, as leastPage has it.
func (v usageViews) whole(rows int) bool {
	return v.all > rows && v.leastPage() > rows
}

// leastPage is the most rows the block takes scrolled an account at a time,
// an account in view: past the first, ▲ over it and ▼ under it, where it has
// three accounts or more; ▼ alone with two; and with one, every row.
func (v usageViews) leastPage() int {
	if v.accounts < 2 {
		return v.all
	}
	return v.of(min(1, v.accounts-2), 1)
}

// paged is how the block, laid out whole, pages in rows rows: its key line
// held on their last rows, a blank row over it, but where that would leave
// fewer than leastPaged rows over it; each window's heading, and each
// block's name on a row of its own, shown with the first row under it, as
// lay has them.
func (v usageViews) paged(rows int) rowsPaged {
	l := v.laid()
	foot := max(v.all-l.end-1, 0)
	if rows-foot < leastPaged {
		foot = 0
	}
	return rowsPaged{rows: rows, foot: foot, content: l.end, heads: l.heads, says: true}
}

// usageShown is USAGE's block as it's shown in a body's rows: scrolled an
// account at a time, as fitted lays it out, its rows from the first to the
// body's last; or shown whole, its rows from from up to to, as paged pages
// them.
type usageShown struct {
	block    usageBlock
	paged    rowsPaged
	whole    bool
	from, to int
}

// shown is the block as USAGE shows it in rows rows, scrolled offset down:
// whole, a row at a time, where whole says so; else as fitted lays it out.
func (v usageViews) shown(rows, offset int) usageShown {
	if !v.whole(rows) {
		return usageShown{block: v.fitted(rows, offset), to: rows}
	}
	p := v.paged(rows)
	from, to := p.shown(offset)
	return usageShown{block: v.block, paged: p, whole: true, from: from, to: to}
}

// fitted is the block as USAGE draws it in rows rows, scrolled offset
// accounts' rows down: at its most, as enlarged has it, where that's past its
// least and fits; else stacked beside, where that's a step of its own, as
// between has it, and fits; else as it's laid out, scrolled within the rows
// where it doesn't fit. Only the rows given choose, never what it holds.
func (v usageViews) fitted(rows, offset int) usageBlock {
	switch {
	case v.all < v.big && v.big <= rows:
		return v.block.enlarged()
	case v.all < v.stacked && v.stacked <= rows:
		return v.block.stackedBeside()
	}
	return v.scrolled(rows, offset)
}

// scrolling is how far the block scrolls in rows rows, scrolled offset down:
// shown whole, a row at a time, as paged has it; else an account's rows at a
// time, no further than shows the last account, a page being as many as show
// at the offset, taken no further than it goes: more at the top or the foot,
// with no ▲ or ▼ by them, than between; nowhere where it fits.
func (v usageViews) scrolling(rows, offset int) Scrolling {
	switch {
	case v.whole(rows):
		return v.paged(rows).scrolling(offset)
	case v.all <= rows:
		return Scrolling{}
	}
	n, last := v.accounts, 1
	for last < n-1 && v.of(n-last-1, last+1) <= rows {
		last++
	}
	most := n - last
	return Scrolling{Most: most, Page: v.shownFrom(rows, min(max(offset, 0), most))}
}

// revealing is how far the block, in rows rows, scrolled offset down, is
// scrolled to show the account with the given id: shown whole, as paged
// reveals one of its rows; else as far as it's scrolled, where that shows
// it, else no further than it must go to. Scrolled down, a page may show
// fewer than the one it left, once a ▲ row comes over it, so it goes on an
// account at a time until the account shows.
func (v usageViews) revealing(rows, offset int, id string) int {
	if v.whole(rows) {
		var spans []rowAt
		for _, p := range v.block.placed() {
			if p.id == id {
				spans = append(spans, p)
			}
		}
		return v.paged(rows).revealing(offset, spans)
	}
	accounts, _ := v.block.split()
	i := slices.IndexFunc(accounts, func(r usageRow) bool { return r.id == id })
	at := v.scrolling(rows, offset).revealing(offset, i)
	for s := v.scrolling(rows, at); at < s.Most && i >= at+s.Page; s = v.scrolling(rows, at) {
		at++
	}
	return at
}

// scrolled is the block in rows rows, scrolled offset accounts' rows down, no
// further than scrolling says it goes, showing as many as fit, at least one,
// the rows they leave spare under them, so its pool and its key line keep
// their rows at the foot however far it's scrolled.
func (v usageViews) scrolled(rows, offset int) usageBlock {
	s := v.scrolling(rows, offset)
	if s.Most == 0 {
		return v.block
	}
	above := min(max(offset, 0), s.Most)
	shown := v.block.shows(above, s.Page)
	shown.spare = max(rows-v.of(above, s.Page), 0)
	return shown
}

// shownFrom is how many accounts' rows show in rows rows, from the one at
// first: as many as fit, at least one.
func (v usageViews) shownFrom(rows, first int) int {
	shown := 1
	for shown < v.accounts-first && v.of(first, shown+1) <= rows {
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

// usageRowsOut is the form of the line saying how many of USAGE's rows are
// out of view, where it's shown whole: "▲ 3 more above · ▼ 9 more below", in
// text.subtle, as its accounts' are.
var usageRowsOut = outForm{above: " above", below: " below", gap: " · ", inks: subtleOut}

// draw draws the block in body, its rows shown in those rowsIn gives them:
// shown whole, its key line on the body's last rows, where it keeps it, and
// on the row after the rows shown, how many are out of view; else its key
// line after a blank row, cut to the body's rows.
func (s usageShown) draw(c *canvas, body bounds) {
	in := c.within(s.rowsIn(body))
	end := s.block.draw(in, body.y-s.from)
	if !s.whole {
		s.block.key(in, end+1)
		return
	}
	if s.paged.foot > 0 {
		s.block.key(c, body.y+body.rows-s.paged.foot)
	}
	s.paged.said(c, body.y, s.from, s.to, s.block.width, usageRowsOut)
}

// rowsIn are the rows of body the block's rows shown are drawn in, from its
// first.
func (s usageShown) rowsIn(body bounds) bounds {
	body.rows = s.to - s.from
	return body
}

// placed is where the block draws each of its rows in view, the accounts'
// and the pool's, counted from its first row.
func (b usageBlock) placed() []rowAt {
	return b.lay(growing(b.width), 0).placed
}

// beside is where the block draws its rows shown in body, for ROUTING or
// GRAPHS' charts to be drawn beside them from column x: where accounts are
// out of view below, what's beside the last shown is cut at the row after
// its last, where ▼ and how many sit.
func (s usageShown) beside(x int, body bounds) blocksBeside {
	at := blocksBeside{x: x, under: s.block.under(), across: s.block.besideCharts()}
	for _, p := range s.block.placed() {
		p.y += body.y - s.from
		at.blocks = append(at.blocks, p)
		if p.id != "" && s.block.below > 0 {
			at.cut = p.y + p.rows
		}
	}
	return at
}

// targets are the block's click targets as it's shown in body: each row of
// an account's shown, across the body, which moves the focus to it.
func (s usageShown) targets(body bounds) []Target {
	var targets []Target
	for _, p := range s.block.placed() {
		if p.id == "" {
			continue
		}
		for y := max(p.y, s.from); y < min(p.y+p.rows, s.to); y++ {
			targets = append(targets, Target{X: body.x, Y: body.y + y - s.from, Width: body.width, On: OnAccount, Account: p.id})
		}
	}
	return targets
}

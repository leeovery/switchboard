package dashboard

import (
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// The cells of USAGE by account from besideWidth columns, beside ROUTING or
// GRAPHS' charts: its bars are besideBar cells with a name column of
// matrixNames, a cell fewer for each cell the column is wider, and what's
// beside its blocks starts besideCells after its words.
const (
	besideWidth = 150
	besideBar   = 50
	besideCells = 4
	// matrixNames is the name column the Overview's cells past it are worked
	// out for, besideBar's and GRAPHS' charts': a wider one takes its cells
	// from them.
	matrixNames = 14
)

// usageSection is the Overview's USAGE of doc at now: usage's block, as
// overviewBlock has it. At its least it's the block as usage lays it out at
// the frame's width, which by window, where its windows sit side by side, is
// the compact matrix, 2a + 7 rows for a accounts and the pool; at its most,
// there, the big bars, its windows stacked as sections at the full width; by
// account with GRAPHS' charts beside its blocks, the charts at 5 rows, every
// one, and between, the stacked step, as stackedBeside has it; otherwise its
// least. Short of its least, it gives way in steps, as giving has them, the
// planner taking each as a step of its least, as leaner has them, but never
// folds: given fewer rows than its form takes, it scrolls within them whole,
// a row at a time, as shown shows it. By account from besideWidth columns,
// GRAPHS' charts or ROUTING are drawn beside its blocks, from besideAt, as
// besideUsage has them, and follow them as they scroll; a click along an
// account's rows, its own or what's beside them, moves the focus to it.
func (f Frame) usageSection(doc status.Document, now time.Time) section {
	views := f.overviewBlock(doc, now).views()
	beside := f.besideUsage(doc, now)
	s := section{
		Section: SectionUsage, least: sectionHead + views.all, most: sectionHead + views.big,
		between:     views.between(),
		leaner:      views.leanerRows(),
		accounts:    func(given giving, rows int) int { return views.given(given).accountsIn(rows) },
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
	s.scrolling, s.revealing = form.scrolling, form.revealing
	s.scrollForm = form.scrollForm
	return s
}

// overviewBlock is usage's block of doc at now as the Overview's USAGE draws
// it, grouped as the frame's By has it: as usage prints it, but for what the
// Overview alone has. By account from besideWidth columns, its bars leave
// room for GRAPHS' charts or ROUTING beside them, its blocks a row further
// apart for the charts' axes, where they're the charts. While GRAPHS shows,
// the matrix picks out the account the focus is on. Its key line names the
// marks the accounts carry, its form judged by those they may. An account
// out of the pool at now is faint, as overviewGauge draws its windows and
// overviewNames its name.
func (f Frame) overviewBlock(doc status.Document, now time.Time) usageBlock {
	m := f.Moment(doc, now)
	names := overviewNames(m)
	b := blockOf(m, f.Width, f.Look, f.By, f.overviewWindows(doc, now), names)
	b.marks, b.markable = keyMarks(names), markable(names)
	if f.By == ByAccount && f.Width >= besideWidth {
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
	return marksNamed(slices.ContainsFunc(names, isPrimary), slices.ContainsFunc(names, func(n accountName) bool { return n.best }))
}

// markable are the marks the accounts named may carry at any read, as
// keyMarks words them: ◆ where one is the primary, and ▲, as one may be
// where new sessions go whether one is now or not.
func markable(names []accountName) [][]line {
	return marksNamed(slices.ContainsFunc(names, isPrimary), true)
}

// isPrimary reports whether the name is the primary's.
func isPrimary(n accountName) bool {
	return n.primary
}

// marksNamed are the marks the key line names, in their forms, the widest
// first: ◆ where primary is set, and ▲ where best is, worded as each of
// bestWords.
func marksNamed(primary, best bool) [][]line {
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
// account from besideWidth columns, GRAPHS' charts or ROUTING's lines: hx =
// n + 10 + bar + 7 + 22 + 4, its words' cells and besideCells after them, 107
// with a name column of 16.
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

// enlarges reports whether the block grows: by window, where its windows sit
// side by side, to its big bars, its windows stacked as sections at the full
// width; and by account, where GRAPHS' charts sit beside its blocks, apart
// for them.
func (b usageBlock) enlarges() bool {
	return b.sideBySide() || b.charted
}

// usageViews are the block's views, as USAGE shows it in however many rows
// it's given, scrolled however far: the block; the rows it takes with its big
// bars, by account with GRAPHS' charts beside its blocks stacked, and as it's
// laid out; and, worked out only once it's asked for, the block laid out
// whole and drawn so, as paged pages it. Where it's short of rows, its
// leaner views are the block as it gives way, a step at a time, as giving
// has them.
type usageViews struct {
	block             usageBlock
	big, stacked, all int
	whole             func() (laid, *canvas)
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
	all := b.height()
	return usageViews{
		block: b, big: all, stacked: all, all: all,
		whole: sync.OnceValues(func() (laid, *canvas) {
			c := growing(b.width)
			return b.lay(c, 0), c
		}),
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

// accountsIn is how many accounts the block shows in rows rows, its
// heading's among them, as headed has them, scrolled to its top: those with
// a row among its targets.
func (v usageViews) accountsIn(rows int) int {
	body := rows - v.block.headed()
	if body <= 0 {
		return 0
	}
	var ids []string
	for _, t := range v.shown(body, 0).targets(bounds{rows: body}) {
		ids = append(ids, t.Account)
	}
	slices.Sort(ids)
	return len(slices.Compact(ids))
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
// Overview gives it, in the form fitted has it.
func (f Frame) usageDrawn(doc status.Document, now time.Time) usageBlock {
	views := f.overviewBlock(doc, now).views()
	for _, u := range f.allotted(doc, now) {
		if u.ledBy(SectionUsage) && !u.fold {
			form := views.given(u.lean)
			return form.fitted(u.rows - form.block.headed())
		}
	}
	return views.block
}

// paged is how the block, laid out whole, its key line among its rows,
// pages in rows rows, a row at a time: each window's heading, and each
// block's name on a row of its own, shown with the first row under it, and
// each window's heading kept on the page's first row while its rows scroll
// under it, as lay has them.
func (v usageViews) paged(rows int) rowsPaged {
	l, _ := v.whole()
	return rowsPaged{rows: rows, content: v.all, heads: l.heads, kept: l.kept, says: true}
}

// usageShown is USAGE's block as it's shown in a body's rows: its rows from
// from up to to, as paged pages them where it scrolls, the block laid out
// whole with them, which a heading kept is taken from; its rows from the
// first to the body's last where it fits.
type usageShown struct {
	block    usageBlock
	whole    *canvas
	paged    rowsPaged
	from, to int
}

// usageForm is the form USAGE's rows are laid out in where it scrolls, which
// an offset into them is read in: grouped by window or by account; how far
// it has given way; as a matrix, or by account with each block's name on a
// row of its own, or neither; and how many rows it takes over its key line.
// Those under them, the key line's, are its last, and as many as the marks
// it names have, so a read can change them without changing what any offset
// into the rows over them shows. The zero usageForm where it fits, and
// doesn't scroll.
type usageForm struct {
	by            By
	given         giving
	matrix, above bool
	over          int
}

// scrollForm is the form the block's rows are laid out in, shown in rows
// rows.
func (v usageViews) scrollForm(rows int) usageForm {
	if v.all <= rows {
		return usageForm{}
	}
	l, _ := v.whole()
	_, _, above := v.block.labels()
	return usageForm{by: v.block.by, given: v.block.given, matrix: v.block.matrixed(), above: above, over: l.end}
}

// shown is the block as USAGE shows it in rows rows, scrolled offset rows
// down: in the form fitted has it, where that fits; else whole, a row at a
// time, as paged pages it.
func (v usageViews) shown(rows, offset int) usageShown {
	if v.all <= rows {
		return usageShown{block: v.fitted(rows), to: rows}
	}
	p := v.paged(rows)
	from, to := p.shown(offset)
	_, whole := v.whole()
	return usageShown{block: v.block, whole: whole, paged: p, from: from, to: to}
}

// fitted is the block as USAGE draws it in rows rows: at its most, as
// enlarged has it, where that's past its least and fits; else stacked
// beside, where that's a step of its own, as between has it, and fits; else
// as it's laid out. Only the rows given choose, never what it holds.
func (v usageViews) fitted(rows int) usageBlock {
	switch {
	case v.all < v.big && v.big <= rows:
		return v.block.enlarged()
	case v.all < v.stacked && v.stacked <= rows:
		return v.block.stackedBeside()
	}
	return v.block
}

// scrolling is how far the block scrolls in rows rows, scrolled offset down:
// a row at a time, as paged has it; nowhere where it fits.
func (v usageViews) scrolling(rows, offset int) Scrolling {
	if v.all <= rows {
		return Scrolling{}
	}
	return v.paged(rows).scrolling(offset)
}

// revealing is how far the block, in rows rows, scrolled offset down, is
// scrolled to show the account with the given id, as paged reveals one of
// its rows: nowhere where it fits.
func (v usageViews) revealing(rows, offset int, id string) int {
	if v.all <= rows {
		return 0
	}
	var spans []rowAt
	l, _ := v.whole()
	for _, p := range l.placed {
		if p.id == id {
			spans = append(spans, p)
		}
	}
	return v.paged(rows).revealing(offset, spans)
}

// usageRowsOut is the form of the line saying how many of USAGE's rows are
// out of view, where it scrolls: "▲ 3 more above · ▼ 9 more below", in
// text.subtle.
var usageRowsOut = outForm{above: " above", below: " below", gap: " · ", inks: subtleOut}

// draw draws the block in body, its rows shown in those rowsIn gives them,
// its key line after a blank row among them; the heading paged keeps on the
// body's first row, where it keeps one; and on the row after the rows shown,
// how many are out of view, where it scrolls.
func (s usageShown) draw(c *canvas, body bounds) {
	in := c.within(s.rowsIn(body))
	end := s.block.draw(in, body.y-s.from)
	s.block.key(in, end+1)
	if at, ok := s.paged.keeps(s.from, s.to); ok {
		c.within(bounds{x: body.x, y: body.y, width: body.width, rows: 1}).paste(s.whole, at, body.y, 1)
	}
	s.paged.said(c, body.y, s.from, s.to, s.block.width, usageRowsOut)
}

// rowsIn are the rows of body the block's rows shown are drawn in: from the
// first, but under a heading kept on it.
func (s usageShown) rowsIn(body bounds) bounds {
	top := s.paged.top(s.from, s.to)
	body.y += top - s.from
	body.rows = s.to - top
	return body
}

// placed is where the block draws each of its rows, the accounts' and the
// pool's, counted from its first row.
func (b usageBlock) placed() []rowAt {
	return b.lay(growing(b.width), 0).placed
}

// beside is where the block draws its rows shown in body, for ROUTING or
// GRAPHS' charts to be drawn beside them from column x.
func (s usageShown) beside(x int, body bounds) blocksBeside {
	at := blocksBeside{x: x, under: s.block.under(), across: s.block.besideCharts()}
	for _, p := range s.block.placed() {
		p.y += body.y - s.from
		at.blocks = append(at.blocks, p)
	}
	return at
}

// targets are the block's click targets as it's shown in body: each row of
// an account's shown, across the body, which moves the focus to it, but a
// row a heading kept takes.
func (s usageShown) targets(body bounds) []Target {
	var targets []Target
	top := s.paged.top(s.from, s.to)
	for _, p := range s.block.placed() {
		if p.id == "" {
			continue
		}
		for y := max(p.y, top); y < min(p.y+p.rows, s.to); y++ {
			targets = append(targets, Target{X: body.x, Y: body.y + y - s.from, Width: body.width, On: OnAccount, Account: p.id})
		}
	}
	return targets
}

package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The cells of the usage block's columns.
const (
	// groupGap are the cells after a matrix group's words, before the next.
	groupGap = 3
	// leastUsageBar is the fewest cells a bar of the block takes: where a row's
	// parts don't fit at their leasts with it, its windows don't sit side by
	// side.
	leastUsageBar = 12
	// mostMatrixBar is the most cells a matrix's bar is.
	mostMatrixBar = 90
	// afterBar are the cells after a bar, before its words: a cell, the
	// percentage and two cells.
	afterBar = 1 + percentCells + 2
	// afterWords is the cell a row leaves after its words, at the edge.
	afterWords = 1
	// windowLabelCells are the cells of a window's label in a block by
	// account, wide; narrow, the label starts a cell in and has a cell more.
	windowLabelCells = 10
	// nameGap are the cells between the names and the bars, and markCells a
	// mark's, a space and its glyph, as ▲ is kept on every account.
	nameGap   = 2
	markCells = 2
)

// usageBlock is every account's windows, and the pool's, as usage prints them
// and the Overview's USAGE draws them, width cells wide, in a look, grouped by
// window or by account, with its key line.
type usageBlock struct {
	width int
	look  Look
	by    By
	// windows are those shown, in the order of care.
	windows []shownWindow
	// rows are the accounts', in the config's order, then the pool's, where
	// there's one.
	rows []usageRow
	// names is the name column's cells.
	names int
	// laid is whether its words are as its rows' parts lay them out, in full
	// wherever they fit their cells, as usage's and USAGE's are; else, as the
	// views that draw its gauges in rows of their own have them, as pick
	// picks them by the width.
	laid bool
	// The rest are the Overview's alone, the printout's being their zero
	// values: big stacks its windows where they'd sit side by side, as its
	// big bars; accountBar is its bars' cells by account, where it sets them;
	// focus is the account whose number and name the matrix picks out; marks
	// are the marks the key line names after the cap's, in their forms, the
	// widest first, and markable those its accounts may carry at any read,
	// which its key line's form is judged by; charted is whether GRAPHS'
	// charts sit beside its blocks by account, and beside the form they take
	// there; and given is how far it has given way to rows short.
	big        bool
	accountBar int
	focus      string
	marks      [][]line
	markable   [][]line
	charted    bool
	beside     besideForm
	given      giving
}

// giving is how far USAGE's block has given way where the Overview's rows
// run short, each step a form of the one before: not at all; its accounts a
// row apart, the blank rows between them gone; the blank rows under its
// windows' headings gone too, as the blank under USAGE's heading is; and its
// key gone too, with the blank row over it.
type giving int

const (
	givenNone giving = iota
	givenApart
	givenHeads
	givenKey
)

// shownWindow is a window the block shows: its key, and its name, as in
// "5-hour" or "Fable week".
type shownWindow struct {
	key, name string
}

// usageRow is a row of the block, an account's or the pool's: its account's
// id, "" for the pool's; its name; a gauge for each window shown; and its
// account's cap as the key line names it, as "work at 95%", "" where it has
// none.
type usageRow struct {
	id     string
	name   line
	gauges []gauge
	pool   bool
	cap    string
}

// gauge is a window of an account, or the pool's, as a row draws it: the
// window it is, by its account's id, "" for the pool's, and its key, or with
// all set, all accounts' window, which is never the pool's, by its key; its
// bar; its words, and whether they say it runs out; and, while the Overview
// cues its words, how far their cue has faded, nil where they aren't cued.
type gauge struct {
	of      Ref
	all     bool
	bar     usageBar
	words   usageWords
	runsOut bool
	cue     *float64
}

// newUsageBlock is the usage block of the moment m, width cells wide, drawn
// in look, grouped by, of the windows given: its accounts' windows and its
// pool as they stand at m, its accounts named as namesOf has them.
func newUsageBlock(m status.Moment, width int, look Look, by By, windows []shownWindow) usageBlock {
	return blockOf(m, width, look, by, windows, namesOf(m))
}

// blockOf is the usage block of m, as newUsageBlock has it, its accounts
// named as names has them.
func blockOf(m status.Moment, width int, look Look, by By, windows []shownWindow, names []accountName) usageBlock {
	b := usageBlock{width: width, look: look, by: by, windows: windows, laid: true}
	b.names = nameColumn(names, width)
	for i, a := range m.Accounts {
		row := usageRow{id: a.ID, name: names[i].line(b.names)}
		if a.Reserve > 0 {
			row.cap = name(a) + " at " + prose.Percent(a.Cap)
		}
		for _, w := range b.windows {
			row.gauges = append(row.gauges, accountGauge(a, w.key, m.Now))
		}
		b.rows = append(b.rows, row)
	}
	if row, ok := poolRow(m.Pool(), names, b.windows); ok {
		b.rows = append(b.rows, row)
	}
	return b
}

// usageWindows are the windows with the keys given, in their order, each
// named as name names it, cleaned.
func usageWindows(keys []string, name func(key string) string) []shownWindow {
	shown := make([]shownWindow, 0, len(keys))
	for _, key := range keys {
		shown = append(shown, shownWindow{key: key, name: prose.Clean(name(key))})
	}
	return shown
}

// accountGauge is account a's window with the given key, as it stands at
// now, as its row draws it: empty, and nothing on it, where the account has
// no usable token or nothing is read of the window; else filled to its use,
// with its cap's mark and even pace's, and faded where it's stale.
func accountGauge(a status.AccountNow, key string, now time.Time) gauge {
	w, _ := a.Window(key)
	bar := usageBar{used: w.Used, capAt: a.Cap, pace: w.Pace, stale: w.Kind == status.WindowStale}
	if drawsEmpty(w.Kind) {
		bar = usageBar{empty: true}
	}
	return gauge{of: Ref{Account: a.ID, Window: key}, bar: bar, words: wordsOf(w, now), runsOut: w.RunsOut}
}

// drawsEmpty reports whether a window of the given kind draws an empty bar:
// where its account has no usable token or nothing is read of it.
func drawsEmpty(kind status.WindowKind) bool {
	return kind == status.WindowTokenless || kind == status.WindowUnread
}

// poolRow is pool's row under the rows of accounts named as names has them,
// of the windows shown, its Σ in their number column, reporting false where
// there's no pool, as with one account: each window's bar filled to its
// accounts' mean use, with their even pace and no cap's mark, empty where no
// account's use is counted, and its words its room, the first it has words
// for saying it's pinned, where it is.
func poolRow(pool status.PoolNow, names []accountName, windows []shownWindow) (usageRow, bool) {
	if len(names) < 2 || len(pool.Accounts) == 0 {
		return usageRow{}, false
	}
	row := usageRow{name: poolNamed(names), pool: true}
	pinned := pool.Pinned
	for _, w := range windows {
		j := slices.IndexFunc(pool.Windows, func(p status.PoolWindowNow) bool { return p.Key == w.key })
		if j < 0 {
			row.gauges = append(row.gauges, gauge{bar: usageBar{empty: true}})
			continue
		}
		p := pool.Windows[j]
		bar := usageBar{empty: true}
		if p.Used != nil {
			bar = usageBar{used: *p.Used, capAt: 1, pace: p.Pace}
		}
		row.gauges = append(row.gauges, gauge{of: Ref{Window: w.key}, bar: bar, words: poolWords(p.Room, len(pool.Accounts), pinned)})
		pinned = false
	}
	return row, true
}

// poolNamed is the pool's name as its row heads it under the rows of
// accounts named as names has them: Σ in their numbers' column, in
// text.subtle, then pool, in text.secondary.
func poolNamed(names []accountName) line {
	return line{poolSigma(names), {" pool", noteInk}}
}

// poolSigma is the pool's Σ, under the rows of accounts named as names has
// them: in their numbers' column, right-aligned, in text.subtle.
func poolSigma(names []accountName) span {
	return span{strings.Repeat(" ", len(names[0].number)-1) + "Σ", dimInk}
}

// caps is the key line's cap part: ╎ and the cap of each account whose bars,
// bar cells long, draw its mark, in the config's order, as in "cap: work at
// 95%, side at 90%"; nil where none does, as where every cap is off its bars.
func (b usageBlock) caps(bar int) line {
	return b.capsOf(func(r usageRow) bool {
		return slices.ContainsFunc(r.gauges, func(g gauge) bool { _, ok := g.bar.capCell(bar); return ok })
	})
}

// everyCap is the key line's cap part as wide as it may be: the cap of every
// account that has one, whether its bars draw its mark or not.
func (b usageBlock) everyCap() line {
	return b.capsOf(func(usageRow) bool { return true })
}

// capsOf is the key line's cap part, naming the cap of each account that has
// one and that drawn says its bars draw; nil where there's none.
func (b usageBlock) capsOf(drawn func(usageRow) bool) line {
	var caps []string
	for _, r := range b.rows {
		if r.cap != "" && drawn(r) {
			caps = append(caps, r.cap)
		}
	}
	if len(caps) == 0 {
		return nil
	}
	return line{{capMark, reserveInk}, {" cap: " + strings.Join(caps, ", "), mutedInk}}
}

// accountName is an account's name as a row of the block heads it: its
// number, its place in the config, right-aligned, faint where it's out of the
// pool; its label; and its marks, ◆ on the primary and ▲ on the account new
// sessions go to. A faint one, as the Overview names an account out of the
// pool, has its label and marks faint too. An unrouted one is named where
// nothing is routed, as on Accounts, so it never carries ▲.
type accountName struct {
	number   string
	label    string
	primary  bool
	best     bool
	out      bool
	faint    bool
	unrouted bool
}

// namesOf are the names of m's accounts, in the config's order, numbered
// right-aligned once there are ten, ▲ on the one new sessions go to, and
// each out of the pool where it is.
func namesOf(m status.Moment) []accountName {
	digits := len(strconv.Itoa(len(m.Accounts)))
	names := make([]accountName, len(m.Accounts))
	for i, a := range m.Accounts {
		names[i] = accountName{
			number: fmt.Sprintf("%*d", digits, i+1), label: name(a), primary: a.Primary, best: a.Next, out: a.OutOfPool,
		}
	}
	return names
}

// cells are the cells the name takes in the name column, its label whole:
// its number, its label and its marks, as headCells counts them.
func (n accountName) cells(label int) int {
	return n.headCells() + 1 + label
}

// headCells are the cells the name's number and marks take, ▲'s kept on an
// account without it, but where the name is unrouted.
func (n accountName) headCells() int {
	cells := len(n.number)
	if !n.unrouted {
		cells += markCells
	}
	if n.primary {
		cells += markCells
	}
	return cells
}

// line is the name, in a name column column cells wide, its label cut short
// to keep its number, its marks and ▲'s cells.
func (n accountName) line(column int) line {
	return n.labelIn(column - nameGap - n.cells(0))
}

// fitted is the name in the cells given, nameGap of them kept after it, its
// label cut short to keep its number and its marks, as labelIn cuts it, and
// no cells kept for a ▲ it hasn't.
func (n accountName) fitted(cells int) line {
	return n.labelIn(n.labelRoom(cells))
}

// said is the name as a heading says it, in the cells given, as fitted
// keeps them, its label cut only where they run short, never at a label's
// most, as a heading names the account whole wherever it fits.
func (n accountName) said(cells int) line {
	return n.labelled(prose.Cut(n.label, n.labelRoom(cells)))
}

// labelRoom are the cells the name's label has of the cells given: those
// past nameGap, its number and the space after it, and its marks.
func (n accountName) labelRoom(cells int) int {
	return cells - nameGap - len(n.number) - 1 - n.marks().width()
}

// labelIn is the name, its label cut to the cells given as labelledIn cuts
// it, so never past a label's most.
func (n accountName) labelIn(cells int) line {
	return n.labelled(labelledIn(n.label, cells))
}

// labelled is the name, its label as given: its label alone, unnumbered,
// where it has no number, as an account the config no longer has.
func (n accountName) labelled(text string) line {
	label := span{text, n.inked(titleInk)}
	if n.number == "" {
		return append(line{label}, n.marks()...)
	}
	label.text = " " + label.text
	return append(line{n.numbered(), label}, n.marks()...)
}

// numbered is the name's number, in text.subtle, or text.faint where it's
// out of the pool.
func (n accountName) numbered() span {
	if n.out {
		return span{n.number, faintInk}
	}
	return span{n.number, dimInk}
}

// marks are the marks after the name's label, each after a space: ◆ on the
// primary, in text.tertiary, and ▲ on the account new sessions go to, in
// accent.mode; each faint where the name is.
func (n accountName) marks() line {
	primary, best := primaryInk, accentInk
	if n.faint {
		primary, best = faintInk, faintInk
	}
	var l line
	if n.primary {
		l = append(l, span{" ◆", primary})
	}
	if n.best {
		l = append(l, span{" ▲", best})
	}
	return l
}

// inked is k as the name's label is drawn in it: in text.faint, k's weight
// kept, where the name is faint.
func (n accountName) inked(k ink) ink {
	if n.faint {
		k.token = theme.TextFaint
	}
	return k
}

// namesPart is the name column of the names given as flex lays it, a column
// of names, each with its number, its marks and nameGap, as accountsColumn
// has it. It counts what's configured, never what's shown, as cells keeps
// ▲'s cells on every account.
func namesPart(names []accountName) flex.Part {
	return accountsColumn(nameGap, names)
}

// nameColumn is the cells of the name column of a block width columns wide,
// and of ROUTING's, SESSIONS' and GRAPHS' under it: the names' column as
// namesPart has it, laid out in a row of a window, as layGauges lays one, so
// it narrows toward its least only once the bar is at its least and the
// words are short.
func nameColumn(names []accountName, width int) int {
	row, _ := layGauges(width, namesPart(names), 1, afterBar+afterWords, 0)
	return row.names
}

// The parts of a row of the block, as layGauges lays them: its name column;
// every window's bar; the cells it keeps beside them, after each bar, between
// its windows and at its edge; and every window's words.
const (
	gaugeNames = iota
	gaugeBars
	gaugeKept
	gaugeWords
)

// The turns a row's parts narrow in: its words, from full toward short, then
// its names, toward their least.
const (
	usageWordsTurn = 1 + iota
	usageNamesTurn
)

// usageCells are the cells of a row's parts, as layGauges lays them: its name
// column's, and each window's bar's and words'.
type usageCells struct {
	names, bar, words int
}

// layGauges are the cells of a row of the block's parts, windows windows
// across width columns, after a name column as names has it, with kept cells
// beside its bars and words: each window's bar at least leastUsageBar, taking
// what's left, up to most where it's more than 0; and its words at their
// natural, fullCells, narrowing toward the short form's, shortCells, in the
// first turn, before the names narrow toward their least. So the bars only
// grow with the width. It reports false where the parts don't fit at their
// leasts, the bars then taking what the row leaves them, none where it
// leaves none.
func layGauges(width int, names flex.Part, windows, kept, most int) (usageCells, bool) {
	names.Narrows = usageNamesTurn
	s, ok := flex.Lay(width, []flex.Part{
		gaugeNames: names,
		gaugeBars:  {Least: windows * leastUsageBar, Most: windows * most, Grows: true},
		gaugeKept:  {Least: kept, Natural: kept},
		gaugeWords: {Least: windows * shortCells, Natural: windows * fullCells, Narrows: usageWordsTurn},
	}...)
	bars := s[gaugeBars].Size
	if !ok {
		bars = max(bars-(s[gaugeWords].At+s[gaugeWords].Size-width), 0)
	}
	return usageCells{names: s[gaugeNames].Size, bar: bars / windows, words: s[gaugeWords].Size / windows}, ok
}

// cells are the cells of the block's rows' parts, as its layout has them:
// by account, after its labels; as a matrix, its windows side by side; else
// stacked, a window's row at a time.
func (b usageBlock) cells() usageCells {
	switch {
	case b.by == ByAccount:
		labelAt, cells, _ := b.labels()
		row, _ := layGauges(b.width, flex.Part{}, 1, labelAt+cells+afterBar+afterWords, 0)
		return row
	case b.matrixed():
		row, _ := b.matrixCells()
		return row
	}
	row, _ := layGauges(b.width, b.column(), 1, afterBar+afterWords, 0)
	return row
}

// matrixCells are the cells of a matrix's row's parts, every window side by
// side after the name column, each group's words groupGap cells before the
// next's bar, the last's a cell before the edge, as a stacked row's are; and
// whether they fit at their leasts.
func (b usageBlock) matrixCells() (usageCells, bool) {
	k := len(b.windows)
	if k == 0 {
		return usageCells{}, false
	}
	return layGauges(b.width, b.column(), k, k*afterBar+(k-1)*groupGap+afterWords, mostMatrixBar)
}

// column is the block's name column as a part of its rows: as wide as it is.
func (b usageBlock) column() flex.Part {
	return flex.Part{Least: b.names, Natural: b.names}
}

// wordCells are the cells the words take of a block by its width, as the
// views that draw its gauges in rows of their own have them, as widestWords
// counts them: in full from fullWords columns, else short.
func (b usageBlock) wordCells() int {
	if b.width >= fullWords {
		return fullCells
	}
	return shortCells
}

// draw draws the block's rows from row y, as lay lays them out, and returns
// the row after the last.
func (b usageBlock) draw(c *canvas, y int) int {
	return b.lay(c, y).end
}

// laid is the block's rows as drawn: the row after the last; where each row
// is; its heads, each window's heading, a block's name on a row of its own
// and the pool's rule, over the first row under each, in the order of their
// rows; and its windows' headings, each kept over the rows under it as they
// scroll, to the end of its window's.
type laid struct {
	end    int
	placed []rowAt
	heads  []headed
	kept   []keptHeading
}

// rowAt is where the block drew a row: its account's id, "" for the pool's,
// and the rows it takes from row y.
type rowAt struct {
	id      string
	y, rows int
}

// lay draws the block's rows from row y, as its grouping and its width lay
// them out: by account; else as a matrix; else its windows stacked.
func (b usageBlock) lay(c *canvas, y int) laid {
	var l laid
	switch {
	case b.by == ByAccount:
		l = b.byAccount(c, y)
	case b.matrixed():
		l = b.matrix(c, y)
	default:
		l = b.stacked(c, y)
	}
	slices.SortFunc(l.heads, func(a, b headed) int { return a.at - b.at })
	return l
}

// matrixed reports whether the block draws as a matrix, its windows side by
// side, as sideBySide has them, but for the Overview's big bars.
func (b usageBlock) matrixed() bool {
	return b.sideBySide() && !b.big
}

// sideBySide reports whether the block's windows, by window, sit side by
// side: wherever every part of the matrix's row fits at its least, as
// matrixCells lays it out. It follows the width, the names configured and
// the windows shown, never what they say.
func (b usageBlock) sideBySide() bool {
	_, fits := b.matrixCells()
	return b.by == ByWindow && fits
}

// bar is the cells of the block's bars, as its layout has them, none where
// the width leaves none: as cells has them, but by account where the
// Overview sets them.
func (b usageBlock) bar() int {
	if b.by == ByAccount && b.accountBar > 0 {
		return b.accountBar
	}
	return b.cells().bar
}

// matrix draws the block's windows side by side, a group each from the name
// column, its heading over it, and a row an account, the pool last, from row
// y, the account the focus is on picked out, where the block has one.
func (b usageBlock) matrix(c *canvas, y int) laid {
	row := b.cells()
	bar, group := row.bar, row.bar+afterBar+row.words+groupGap
	for i, w := range b.windows {
		c.line(b.names+i*group, y, line{{strings.ToUpper(w.name), dimInk}})
	}
	first := y + b.underHeading()
	l := b.each(c, first, 2, func(r usageRow, y int) int {
		c.line(0, y, r.name)
		if b.focus != "" && r.id == b.focus {
			c.surface(0, y, b.focusCells(), hue{token: theme.BgSelection})
		}
		for i, g := range r.gauges {
			next := b.names + (i+1)*group
			b.gauge(c, g, next-group, y, bar, next-groupGap)
		}
		return y
	})
	l.heads = append(l.heads, headed{at: y, first: first})
	l.kept = []keptHeading{{at: y, end: l.end}}
	return l
}

// underHeading is how far a window's first row is from its heading: a row
// on from the blank under it, or where the block has given way its
// headings' blank rows, the next.
func (b usageBlock) underHeading() int {
	if b.given >= givenHeads {
		return 1
	}
	return 2
}

// focusCells are the cells of the focused account's row the Overview's
// matrix picks out: its whole name column, its number, name and marks, as
// nameColumn fits it to the names, but the gap before the bars.
func (b usageBlock) focusCells() int {
	return b.names - nameGap
}

// stacked draws the block's windows one under another, each a section, its
// heading, then a row an account, the pool last, from row y, a blank line
// between sections, as before the key line.
func (b usageBlock) stacked(c *canvas, y int) laid {
	bar := b.bar()
	all := laid{end: y}
	for i, w := range b.windows {
		if i > 0 {
			y++
		}
		c.line(0, y, line{{strings.ToUpper(w.name), dimInk}})
		first := y + b.underHeading()
		section := b.each(c, first, 2, func(r usageRow, y int) int {
			c.line(1, y, r.name)
			b.gauge(c, r.gauges[i], b.names, y, bar, b.width-afterWords)
			return y
		})
		all.placed = append(all.placed, section.placed...)
		all.heads = append(all.heads, headed{at: y, first: first})
		all.heads = append(all.heads, section.heads...)
		all.kept = append(all.kept, keptHeading{at: y, end: section.end})
		y, all.end = section.end, section.end
	}
	return all
}

// each draws the block's rows from row y, each as draw draws it from the row
// given, returning its last, a row's first apart rows on from the last of
// the row before, or a row fewer where the block has given way the blank
// rows between its accounts. The pool's row comes last, apart rows on, its
// rule in the row above it.
func (b usageBlock) each(c *canvas, y, apart int, draw func(r usageRow, y int) int) laid {
	accounts, pool := b.split()
	between := apart
	if b.given >= givenApart {
		between--
	}
	var l laid
	last, next := y, y
	for _, r := range accounts {
		last = draw(r, next)
		l.placed = append(l.placed, rowAt{id: r.id, y: next, rows: last - next + 1})
		next = last + between
	}
	next = last + apart
	if pool != nil {
		c.line(0, next-1, line{{rule(b.width), borderInk}})
		last = draw(*pool, next)
		l.placed = append(l.placed, rowAt{id: pool.id, y: next, rows: last - next + 1})
		l.heads = append(l.heads, headed{at: next - 1, first: next})
	}
	l.end = last + 1
	return l
}

// split is the block's rows: the accounts', and the pool's, nil where it has
// none.
func (b usageBlock) split() ([]usageRow, *usageRow) {
	if n := len(b.rows); n > 0 && b.rows[n-1].pool {
		return b.rows[:n-1], &b.rows[n-1]
	}
	return b.rows, nil
}

// byAccount draws a block an account from row y, each window a row of it,
// the pool's last, as far apart as blocksApart has them, the pool's rule in
// the last blank line before it: the name beside its first window's row and
// each label in the name column, where they fit, as labels says; else the
// name on a line of its own above, and the labels a cell in. Its bars take
// what the block's width leaves them, and its words end a cell before the
// edge, but where the Overview sets its bars: then its words end besideCells
// before besideAt, the gap before ROUTING's blocks beside them. After the
// last block come the rows under it what's beside it takes, as under has
// them, the first of them the blank line before the key line.
func (b usageBlock) byAccount(c *canvas, y int) laid {
	labelAt, cells, above := b.labels()
	bar, end := b.bar(), b.width-afterWords
	if b.accountBar > 0 {
		end = b.besideAt() - besideCells
	}
	l := b.each(c, y, b.blocksApart(), func(r usageRow, y int) int {
		c.line(0, y, r.name)
		for j, g := range r.gauges {
			at := y + b.windowRow(j)
			c.line(labelAt, at, line{{prose.Cut(status.Short(b.windows[j].name), cells-1), dimInk}})
			b.gauge(c, g, labelAt+cells, at, bar, end)
		}
		return y + b.blockRows() - 1
	})
	if above {
		for _, p := range l.placed {
			l.heads = append(l.heads, headed{at: p.y, first: p.y + 1})
		}
	}
	l.end += max(b.under()-1, 0)
	return l
}

// blocksApart is how far a block by account's first row is from the last
// of the block before: a blank line between them, as before the key line,
// after the rows under it what's beside it takes, as under has them.
func (b usageBlock) blocksApart() int {
	return b.under() + 2
}

// under are the rows under a block by account's last that what's beside it
// takes: none; but where GRAPHS' charts sit beside them, from the block's
// first row, the axis row under it at the least, or as many as give each row
// of charts, as the block's form lays them out, the rows of chart its form
// has.
func (b usageBlock) under() int {
	if !b.charted {
		return 0
	}
	rows := b.besideCharts().rowsFor(len(b.windows))
	return max(rows*(b.beside.chartRows()+chartEnds), b.blockRows()+1) - b.blockRows()
}

// besideCharts are how GRAPHS' charts lie across beside the block's blocks by
// account, from besideAt to the frame's edge, as its form lays its windows'
// out: every block's in the same columns.
func (b usageBlock) besideCharts() chartsAcross {
	return b.beside.across(len(b.windows), b.width-margin-b.besideAt())
}

// windowApart are the rows from one window's row of a block by account to
// the next's.
const windowApart = 2

// windowRow is the row of window j in a block by account, from its first:
// after the name's line, where it has one of its own, as labels says, a
// window's every other row.
func (b usageBlock) windowRow(j int) int {
	row := windowApart * j
	if _, _, above := b.labels(); above {
		row++
	}
	return row
}

// blockRows are the rows a block by account takes, from its first to its
// last window's, as windowRow places them.
func (b usageBlock) blockRows() int {
	return b.windowRow(len(b.windows)-1) + 1
}

// labels are the column a block by account's window labels start at, the
// cells they take, and whether its name is on a line of its own above them:
// the name column and windowLabelCells, the name beside the first, wherever
// the row's parts fit at their leasts with the name column beside them, as
// layGauges lays them; else a cell in, and a cell more, the name above.
func (b usageBlock) labels() (at, cells int, above bool) {
	if _, fits := layGauges(b.width, b.column(), 1, windowLabelCells+afterBar+afterWords, 0); fits {
		return b.names, windowLabelCells, false
	}
	return 1, windowLabelCells + 1, true
}

// gauge draws g from x along row y: its bar, bar cells long, marked as its
// window's; a cell; its percentage, right-aligned in its cells; two cells;
// and its words, ending before column end, in the form wordsIn has them,
// picked out while they're cued, as cue picks them out.
func (b usageBlock) gauge(c *canvas, g gauge, x, y, bar, end int) {
	g.bar.draw(c, b.look, x, y, bar)
	c.barred(x, y, bar, g.of)
	at := x + bar + 1
	c.right(at+percentCells, y, g.percentage())
	words := at + percentCells + 2
	if drawn := c.line(words, y, b.wordsIn(g, end-words)); g.cue != nil {
		cue(c, b.look, words, y, drawn-words, *g.cue)
	}
}

// wordsIn are g's words as the block draws them in room cells: where its
// words are laid, in full wherever they fit them, as fitted has them; else
// as said has them by the block's width.
func (b usageBlock) wordsIn(g gauge, room int) line {
	if b.laid {
		return g.led(g.words.fitted(room))
	}
	return g.said(b.width, room)
}

// percentage is g's percentage, as its bar has it, numbered as its window's
// use where it shows one.
func (g gauge) percentage() line {
	shown := g.bar.percentage()
	if g.bar.empty {
		return shown
	}
	return shown.numbered(g.number(NumberUsed))
}

// said are g's words as a view drawing usage's gauges in rows of its own
// draws them width columns wide, in room cells, as pick has them, led as led
// has them.
func (g gauge) said(width, room int) line {
	return g.led(g.words.pick(width, room))
}

// led is said, g's words in a form, the number they lead with, where they
// lead with one, numbered as their window's.
func (g gauge) led(said line) line {
	if g.words.leads == 0 || len(said) == 0 {
		return said
	}
	return slices.Concat(line{said[0].numbered(g.number(g.words.leads))}, said[1:])
}

// number is g's window's number of the kind given: all accounts' use and
// room their own, as all accounts is never the pool.
func (g gauge) number(kind NumberKind) Number {
	switch {
	case g.all && kind == NumberUsed:
		kind = NumberAllUsed
	case g.all && kind == NumberRoom:
		kind = NumberAllRoom
	}
	return Number{Kind: kind, Account: g.of.Account, Of: g.of.Window}
}

// key draws the block's key line at row y, saying what its marks mean: one
// line from the name column, the cap's part after even pace's, where it fits
// there, as oneLineKey judges it; else two lines from column 0, the cap's
// part on the second. The marks the Overview names follow the cap's part,
// each part a gap after the one before, in the first of their forms
// that fits the line, as keyForms and firstFit have them. Where the block
// has given way its key, it draws none.
func (b usageBlock) key(c *canvas, y int) {
	if b.given >= givenKey {
		return
	}
	pace, forms := b.keyParts(b.bar())
	if b.oneLineKey() {
		c.line(b.names, y, firstFit(b.width-b.names, keyForms(forms, pace)...))
		return
	}
	c.line(0, y, pace.fit(b.width))
	if second := keyForms(forms); second[0] != nil {
		c.line(0, y+1, firstFit(b.width, second...))
	}
}

// oneLineKey reports whether the block's key line is one line, from the name
// column: where it fits there at its widest, the cap of every account that
// has one among its parts, and the marks its accounts may carry, in their
// widest form, its gaps at their least; so it's one line or two whatever the
// bars draw and wherever ▲ is, and one line at a width is one line wider.
func (b usageBlock) oneLineKey() bool {
	pace, forms := keyPartsOf(b.everyCap(), b.markable)
	widest := keyForms(forms[:1], pace)
	return widest[len(widest)-1].width() <= b.width-b.names
}

// keyForms are a key line in each of the forms of its parts, the widest
// first, the parts led by those given: each form with its parts keyApart
// cells apart, then leastKeyApart, as the gaps narrow before the parts take
// a narrower form. So the first form that fits a line never narrows as the
// line widens.
func keyForms(forms [][]line, lead ...line) []line {
	var lines []line
	for _, parts := range forms {
		for _, gap := range []int{keyApart, leastKeyApart} {
			lines = append(lines, joined(span{strings.Repeat(" ", gap), mutedInk}, slices.Concat(lead, parts)...))
		}
	}
	return lines
}

// keyParts are the parts of the block's key line, its bars bar cells long:
// even pace's; then, in each of the forms of the marks the Overview names,
// the widest first, the cap's, where its bars draw one, and those marks.
func (b usageBlock) keyParts(bar int) (pace line, forms [][]line) {
	return keyPartsOf(b.caps(bar), b.marks)
}

// keyPartsOf are the parts of a key line: even pace's; then, in each of the
// forms of the marks given, the widest first, the cap part given and those
// marks.
func keyPartsOf(caps line, marks [][]line) (pace line, forms [][]line) {
	pace = line{{paceMark, paceInk}, {" even pace: where you would be now, used evenly", mutedInk}}
	if len(marks) == 0 {
		return pace, [][]line{{caps}}
	}
	for _, m := range marks {
		forms = append(forms, slices.Concat([]line{caps}, m))
	}
	return pace, forms
}

// keyApart is the cells between the parts of a key line where they fit, and
// leastKeyApart the fewest, where they don't.
const (
	keyApart      = 4
	leastKeyApart = 3
)

// height is how many rows the block takes as it's laid out, its key line
// after a blank row, where it keeps it.
func (b usageBlock) height() int {
	c := growing(b.width)
	end := b.draw(c, 0)
	b.key(c, end+1)
	return max(c.height(), end)
}

package dashboard

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The widths the usage block's layouts change at, in columns.
const (
	// matrixWidth is the fewest a block by window draws as a matrix at, its
	// windows side by side; narrower, they stack.
	matrixWidth = 150
	// wideAccounts is the fewest a block by account draws an account's name
	// beside its first bar at; narrower, the name has a line of its own.
	wideAccounts = 110
	// roomyNames is the fewest the name column may be its widest at.
	roomyNames = 80
)

// The cells of the usage block's columns.
const (
	// groupCells are a matrix group's cells beyond its bar: a cell, the
	// percentage, two cells, the words and three cells before the next.
	groupCells = 32
	// leastMatrixBar is the fewest cells a matrix's bars may be; shorter,
	// the windows stack.
	leastMatrixBar = 12
	// mostMatrixBar is the most cells a matrix's bar is.
	mostMatrixBar = 90
	// matrixNames is the name column a matrix's bars are worked out for:
	// a wider one takes its cells from the bars.
	matrixNames = 14
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
	// focusCells are the cells of the focused account's number and name the
	// Overview's matrix picks out.
	focusCells = 12
)

// usageBlock is every account's windows, and the pool's, as usage prints them
// and the Overview's USAGE draws them, width cells wide, in a look, grouped by
// window or by account, with its key line.
type usageBlock struct {
	width int
	look  Look
	by    By
	// windows are those shown, in Claude's order.
	windows []shownWindow
	// rows are the accounts', in the config's order, then the pool's, where
	// there's one.
	rows []usageRow
	// names is the name column's cells, and caps the key line's cap part:
	// nil where no account has a cap.
	names int
	caps  line
	// The rest are the Overview's alone, the printout's being their zero
	// values: big stacks its windows from 150 columns, as its big bars;
	// accountBar is its bars' cells by account, where it sets them; focus is
	// the account whose number and name the matrix picks out; marks are the
	// marks the key line names after the cap's; and above and below are how
	// many accounts' rows are scrolled out of view above those shown, and
	// below.
	big          bool
	accountBar   int
	focus        string
	marks        []line
	above, below int
}

// shownWindow is a window the block shows: its key, and its name, as in
// "5-hour" or "Fable week".
type shownWindow struct {
	key, name string
}

// usageRow is a row of the block, an account's or the pool's: its account's
// id, "" for the pool's; its name; and a gauge for each window shown.
type usageRow struct {
	id     string
	name   line
	gauges []gauge
	pool   bool
}

// gauge is a window of an account, or the pool's, as a row draws it: the
// window it is, by its account's id, "" for the pool's, and its key; its bar;
// its words, and whether they say it runs out; and, while the Overview cues
// its words, how far their cue has faded, nil where they aren't cued.
type gauge struct {
	of      Ref
	bar     usageBar
	words   usageWords
	runsOut bool
	cue     *float64
}

// newUsageBlock is the usage block of doc at now, width cells wide, drawn in
// look, grouped by, of the windows with the keys given, every window any
// account reports where none is.
func newUsageBlock(doc status.Document, now time.Time, width int, look Look, by By, keys []string) usageBlock {
	return blockOf(doc, now, width, look, by, keys, namesOf(doc))
}

// blockOf is the usage block of doc at now, as newUsageBlock has it, its
// accounts named as names has them.
func blockOf(doc status.Document, now time.Time, width int, look Look, by By, keys []string, names []accountName) usageBlock {
	b := usageBlock{width: width, look: look, by: by, windows: usageWindows(doc, keys), caps: capsOf(doc)}
	b.names = nameColumn(names, width)
	for i, a := range doc.Accounts {
		row := usageRow{id: a.ID, name: names[i].line(b.names)}
		for _, w := range b.windows {
			st := doc.WindowStateOf(a, w.key, now)
			row.gauges = append(row.gauges, accountGauge(a, w.key, st, now))
		}
		b.rows = append(b.rows, row)
	}
	if pool, ok := poolRow(doc, b.windows, len(strconv.Itoa(len(doc.Accounts)))); ok {
		b.rows = append(b.rows, pool)
	}
	return b
}

// usageWindows are the windows with the keys given, or with none, every
// window any of doc's accounts reports, in Claude's order, each named as
// nameOfWindow names it.
func usageWindows(doc status.Document, keys []string) []shownWindow {
	labels := make(map[string]string)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			labels[w.Key] = cmp.Or(labels[w.Key], w.Label)
		}
	}
	keys = slices.Clone(keys)
	if len(keys) == 0 {
		keys = slices.Collect(maps.Keys(labels))
	}
	slices.SortFunc(keys, quota.CompareKeys)
	shown := make([]shownWindow, 0, len(keys))
	for _, key := range slices.Compact(keys) {
		shown = append(shown, shownWindow{key: key, name: nameOfWindow(key, labels[key])})
	}
	return shown
}

// nameOfWindow names a window by its key and label: one of whole hours,
// under a day, by them, as "5-hour", and any other by its label, cleaned, as
// "Week" or "Fable week", or its key where it has none.
func nameOfWindow(key, label string) string {
	if length, ok := quota.Length(key); ok && length%time.Hour == 0 && length < day {
		return fmt.Sprintf("%d-hour", length/time.Hour)
	}
	return status.Clean(cmp.Or(label, key))
}

// accountGauge is account a's window with the given key, standing as st says
// at now, as its row draws it: empty, and nothing on it, where the account
// has no usable token or nothing is read of the window; else filled to its
// use, with its cap's mark and even pace's, and faded where it's stale.
func accountGauge(a status.Account, key string, st status.WindowState, now time.Time) gauge {
	w, _ := a.Window(key)
	bar := usageBar{used: st.Used, capAt: 1 - a.Reserve, pace: w.Pace, stale: st.Kind == status.WindowStale}
	if st.Kind == status.WindowTokenless || st.Kind == status.WindowUnread {
		bar = usageBar{empty: true}
	}
	return gauge{of: Ref{Account: a.ID, Window: key}, bar: bar, words: wordsOf(st, w.Allowance, now), runsOut: st.Kind == status.WindowRunsOut}
}

// poolRow is the pool's row of doc, of the windows shown, its Σ in a column
// digits wide, reporting false where doc has no pool, as with one account:
// each window's bar filled to what the pool has used, with its accounts' even
// pace and no cap's mark, and its words its room, the first window's saying
// it's pinned, where it is.
func poolRow(doc status.Document, windows []shownWindow, digits int) (usageRow, bool) {
	if len(doc.Accounts) < 2 || len(doc.Pool.Accounts) == 0 {
		return usageRow{}, false
	}
	row := usageRow{name: line{{strings.Repeat(" ", digits-1) + "Σ", dimInk}, {" pool", noteInk}}, pool: true}
	for i, w := range windows {
		j := slices.IndexFunc(doc.Pool.Windows, func(p status.PoolWindow) bool { return p.Key == w.key })
		if j < 0 {
			row.gauges = append(row.gauges, gauge{bar: usageBar{empty: true}})
			continue
		}
		p := doc.Pool.Windows[j]
		row.gauges = append(row.gauges, gauge{
			of:    Ref{Window: w.key},
			bar:   usageBar{used: p.Used, capAt: 1, pace: p.Pace},
			words: poolWords(p.Room, len(doc.Pool.Accounts), doc.Pool.Pinned && i == 0),
		})
	}
	return row, true
}

// capsOf is the key line's cap part of doc: ╎ and each account with a cap,
// in the config's order, where it is, as in "cap: work at 95%, side at 90%";
// nil where none has one.
func capsOf(doc status.Document) line {
	var caps []string
	for _, a := range doc.Accounts {
		if a.Reserve > 0 {
			caps = append(caps, fmt.Sprintf("%s at %d%%", name(a), wholePercent(1-a.Reserve)))
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
// pool, has its label and marks faint too.
type accountName struct {
	number  string
	label   string
	primary bool
	best    bool
	out     bool
	faint   bool
}

// namesOf are the names of doc's accounts, in the config's order, numbered
// right-aligned once there are ten.
func namesOf(doc status.Document) []accountName {
	digits := len(strconv.Itoa(len(doc.Accounts)))
	best := bestOf(doc)
	names := make([]accountName, len(doc.Accounts))
	for i, a := range doc.Accounts {
		names[i] = accountName{
			number: fmt.Sprintf("%*d", digits, i+1), label: name(a), primary: a.Primary, best: a.ID == best, out: outOfPool(doc, a.ID),
		}
	}
	return names
}

// cells are the cells the name takes in the name column, its label whole:
// its number, its label and its marks, ▲'s kept on an account without it.
func (n accountName) cells(label int) int {
	cells := len(n.number) + 1 + label + markCells
	if n.primary {
		cells += markCells
	}
	return cells
}

// line is the name, in a name column column cells wide, its label cut short
// to keep its number, its marks and ▲'s cells.
func (n accountName) line(column int) line {
	label := truncate(n.label, column-nameGap-n.cells(0))
	number, primary, best := dimInk, primaryInk, accentInk
	if n.out {
		number = faintInk
	}
	if n.faint {
		primary, best = faintInk, faintInk
	}
	l := line{{n.number, number}, {" " + label, n.inked(titleInk)}}
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

// nameColumn is the cells of the name column of a block width columns wide:
// the widest of the names, and a gap, at least 14 cells, or 12 under 80
// columns, and at most 24, or 16 under 80.
func nameColumn(names []accountName, width int) int {
	least, most := 14, 24
	if width < roomyNames {
		least, most = 12, 16
	}
	widest := 0
	for _, n := range names {
		widest = max(widest, n.cells(ansi.StringWidth(n.label)))
	}
	return min(max(widest+nameGap, least), most)
}

// wordCells are the cells the block's words take.
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

// laid is the block's rows as drawn: the row after the last, and where each
// row is.
type laid struct {
	end    int
	placed []rowAt
}

// rowAt is where the block drew a row: its account's id, "" for the pool's,
// and the rows it takes from row y.
type rowAt struct {
	id      string
	y, rows int
}

// lay draws the block's rows from row y, as its grouping and its width lay
// them out: by account; else as a matrix, from 150 columns, where its bars
// would be 12 cells or more, but for the Overview's big bars; else its
// windows stacked.
func (b usageBlock) lay(c *canvas, y int) laid {
	switch {
	case b.by == ByAccount:
		return b.byAccount(c, y)
	case b.width >= matrixWidth && b.matrixBar() >= leastMatrixBar && !b.big:
		return b.matrix(c, y)
	default:
		return b.stacked(c, y)
	}
}

// matrixBar is the cells of a matrix's bars, for its windows side by side:
// min(90, ⌊(w − 14 − 32k + 3) ÷ k⌋) − ⌈(n − 14) ÷ k⌉ for k windows, the
// name column n.
func (b usageBlock) matrixBar() int {
	k := len(b.windows)
	if k == 0 {
		return 0
	}
	room := b.width - matrixNames - groupCells*k + 3
	return min(mostMatrixBar, room/k) - (b.names-matrixNames+k-1)/k
}

// matrix draws the block's windows side by side, a group each from the name
// column, its heading over it, and a row an account, the pool last, from row
// y, the account the focus is on picked out, where the block has one.
func (b usageBlock) matrix(c *canvas, y int) laid {
	bar := b.matrixBar()
	group := bar + groupCells
	for i, w := range b.windows {
		c.line(b.names+i*group, y, line{{strings.ToUpper(w.name), dimInk}})
	}
	return b.each(c, 0, y+2, 2, func(r usageRow, y int) int {
		c.line(0, y, r.name)
		if b.focus != "" && r.id == b.focus {
			c.surface(0, y, focusCells, hue{token: theme.BgSelection})
		}
		for i, g := range r.gauges {
			b.gauge(c, g, b.names+i*group, y, bar)
		}
		return y
	})
}

// stacked draws the block's windows one under another, each a section, its
// heading, then a row an account, the pool last, from row y, two blank lines
// between sections.
func (b usageBlock) stacked(c *canvas, y int) laid {
	bar := b.width - b.names - afterBar - b.wordCells() - afterWords
	all := laid{end: y}
	for i, w := range b.windows {
		if i > 0 {
			y += 2
		}
		c.line(0, y, line{{strings.ToUpper(w.name), dimInk}})
		section := b.each(c, 1, y+2, 2, func(r usageRow, y int) int {
			c.line(1, y, r.name)
			b.gauge(c, r.gauges[i], b.names, y, bar)
			return y
		})
		all.placed = append(all.placed, section.placed...)
		y, all.end = section.end, section.end
	}
	return all
}

// each draws the rows in view from row y, each as draw draws it from the row
// given, returning its last, a row's first apart rows on from the last of
// the row before. Where accounts' rows are scrolled out of view, ▲ and how
// many are above comes over the first row shown, and ▼ and how many below
// under the last, from column x. The pool's row comes last, its rule in the
// row above it.
func (b usageBlock) each(c *canvas, x, y, apart int, draw func(r usageRow, y int) int) laid {
	accounts, pool := b.split()
	var l laid
	if b.above > 0 {
		c.line(x, y, outOfSight("▲", b.above))
		y++
	}
	last, next := y, y
	for _, r := range accounts[b.above : len(accounts)-b.below] {
		last = draw(r, next)
		l.placed = append(l.placed, rowAt{id: r.id, y: next, rows: last - next + 1})
		next = last + apart
	}
	if b.below > 0 {
		last++
		c.line(x, last, outOfSight("▼", b.below))
		next = last + apart
	}
	if pool != nil {
		c.line(0, next-1, line{{rule(b.width), borderInk}})
		last = draw(*pool, next)
		l.placed = append(l.placed, rowAt{id: pool.id, y: next, rows: last - next + 1})
	}
	l.end = last + 1
	return l
}

// outOfSight says how many accounts' rows are scrolled out of view, by the
// mark given, as in "▼ 4 more accounts".
func outOfSight(mark string, n int) line {
	return line{{mark + " " + moreAccounts(n), dimInk}}
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
// the pool's last, two blank lines between blocks, the pool's rule in the
// second: wide, from 110 columns, the name beside its first window's row and
// each label in the name column; else the name on a line of its own above,
// and the labels a cell in. Its bars take what the block's width leaves
// them, but where the Overview sets them.
func (b usageBlock) byAccount(c *canvas, y int) laid {
	labelAt, cells := b.names, windowLabelCells
	if b.width < wideAccounts {
		labelAt, cells = 1, windowLabelCells+1
	}
	bar := b.width - labelAt - cells - afterBar - b.wordCells() - afterWords
	if b.accountBar > 0 {
		bar = b.accountBar
	}
	return b.each(c, 0, y, 3, func(r usageRow, y int) int {
		c.line(0, y, r.name)
		if b.width < wideAccounts {
			y++
		}
		for j, g := range r.gauges {
			if j > 0 {
				y += 2
			}
			c.line(labelAt, y, line{{truncate(status.Short(b.windows[j].name), cells-1), dimInk}})
			b.gauge(c, g, labelAt+cells, y, bar)
		}
		return y
	})
}

// gauge draws g from x along row y: its bar, bar cells long, marked as its
// window's; a cell; its percentage, right-aligned in its cells; two cells;
// and its words, in the form the block's width has them, picked out while
// they're cued, as cue picks them out.
func (b usageBlock) gauge(c *canvas, g gauge, x, y, bar int) {
	bar = max(bar, 0)
	g.bar.draw(c, b.look, x, y, bar)
	c.barred(x, y, bar, g.of)
	at := x + bar + 1
	c.right(at+percentCells, y, g.percentage())
	words := at + percentCells + 2
	if end := c.line(words, y, g.said(b.width)); g.cue != nil {
		cue(c, b.look, words, y, end-words, *g.cue)
	}
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

// said are g's words as a block width columns wide draws them, as pick has
// them, the number they lead with, where they lead with one, numbered as
// their window's.
func (g gauge) said(width int) line {
	said := g.words.pick(width)
	if g.words.leads == 0 || len(said) == 0 {
		return said
	}
	return slices.Concat(line{said[0].numbered(g.number(g.words.leads))}, said[1:])
}

// number is g's window's number of the kind given.
func (g gauge) number(kind NumberKind) Number {
	return Number{Kind: kind, Account: g.of.Account, Of: g.of.Window}
}

// key draws the block's key line at row y, saying what its marks mean: from
// 150 columns, one line from the name column, the cap's part after even
// pace's; narrower, two lines from column 0, the cap's part on the second.
// The marks the Overview names follow the cap's part, each part keyApart
// cells after the one before.
func (b usageBlock) key(c *canvas, y int) {
	pace := line{{paceMark, paceInk}, {" even pace: where you would be now, used evenly", mutedInk}}
	apart := span{strings.Repeat(" ", keyApart(b.width)), mutedInk}
	after := slices.Concat([]line{b.caps}, b.marks)
	if b.width >= matrixWidth {
		c.line(b.names, y, joined(apart, slices.Concat([]line{pace}, after)...).fit(b.width-b.names))
		return
	}
	c.line(0, y, pace.fit(b.width))
	if second := joined(apart, after...); second != nil {
		c.line(0, y+1, second.fit(b.width))
	}
}

// keyApart is the cells between the parts of a key line width columns wide:
// four, or on a phone three.
func keyApart(width int) int {
	if width < phoneUnder {
		return 3
	}
	return 4
}

// height is how many rows the block takes as it's laid out, its key line
// after a blank row.
func (b usageBlock) height() int {
	c := growing(b.width)
	b.key(c, b.draw(c, 0)+1)
	return c.height()
}

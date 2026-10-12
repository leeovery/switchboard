package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
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
	// groupGap are the cells after a matrix group's words, before the next.
	groupGap = 3
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
	// names is the name column's cells.
	names int
}

// shownWindow is a window the block shows: its key, and its name, as in
// "5-hour" or "Fable week".
type shownWindow struct {
	key, name string
}

// usageRow is a row of the block, an account's or the pool's: its name, a
// gauge for each window shown, and its account's cap as the key line names
// it, as "work at 95%", "" where it has none.
type usageRow struct {
	name   line
	gauges []gauge
	pool   bool
	cap    string
}

// gauge is a window of an account, or the pool's, as a row draws it: its bar,
// and its words.
type gauge struct {
	bar   usageBar
	words usageWords
}

// newUsageBlock is the usage block of doc at now, width cells wide, drawn in
// look, grouped by, of the windows given.
func newUsageBlock(doc status.Document, now time.Time, width int, look Look, by By, windows []shownWindow) usageBlock {
	b := usageBlock{width: width, look: look, by: by, windows: windows}
	digits := len(strconv.Itoa(len(doc.Accounts)))
	names := namesOf(doc, digits)
	b.names = nameColumn(names, width)
	for i, a := range doc.Accounts {
		row := usageRow{name: names[i].line(b.names)}
		if a.Reserve > 0 {
			row.cap = name(a) + " at " + status.Percent(1-a.Reserve)
		}
		states := doc.WindowStatesOf(a, now)
		for _, w := range b.windows {
			row.gauges = append(row.gauges, accountGauge(a, w.key, states.Of(w.key), now))
		}
		b.rows = append(b.rows, row)
	}
	if pool, ok := poolRow(doc, b.windows, digits); ok {
		b.rows = append(b.rows, pool)
	}
	return b
}

// UsageWindows are the keys of the windows usage shows of doc where none is
// given: those every model shares, as shared has them, and every other any
// account reports, so every account keeps its rows before any is read; in
// Claude's order.
func UsageWindows(doc status.Document, shared []string) []string {
	keys := slices.Clone(shared)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			keys = append(keys, w.Key)
		}
	}
	return slices.Compact(slices.SortedFunc(slices.Values(keys), quota.CompareKeys))
}

// usageWindows are the windows of doc with the keys given, in their order,
// which is Claude's, or with none, those UsageWindows has, each named as name
// names it, cleaned, or where name is nil, by its label as labelOf has it.
func usageWindows(doc status.Document, keys, shared []string, name func(key string) string) []shownWindow {
	if len(keys) == 0 {
		keys = UsageWindows(doc, shared)
	}
	if name == nil {
		name = func(key string) string { return labelOf(doc, key) }
	}
	shown := make([]shownWindow, 0, len(keys))
	for _, key := range keys {
		shown = append(shown, shownWindow{key: key, name: status.Clean(name(key))})
	}
	return shown
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
	return gauge{bar: bar, words: wordsOf(st, w.Allowance, now)}
}

// poolRow is the pool's row of doc, of the windows shown, its Σ in a column
// digits wide, reporting false where doc has no pool, as with one account:
// each window's bar filled to what the pool has used, with its accounts' even
// pace and no cap's mark, and its words its room, the first it has words for
// saying it's pinned, where it is.
func poolRow(doc status.Document, windows []shownWindow, digits int) (usageRow, bool) {
	if len(doc.Accounts) < 2 || len(doc.Pool.Accounts) == 0 {
		return usageRow{}, false
	}
	row := usageRow{name: line{{strings.Repeat(" ", digits-1) + "Σ", dimInk}, {" pool", noteInk}}, pool: true}
	pinned := doc.Pool.Pinned
	for _, w := range windows {
		j := slices.IndexFunc(doc.Pool.Windows, func(p status.PoolWindow) bool { return p.Key == w.key })
		if j < 0 {
			row.gauges = append(row.gauges, gauge{bar: usageBar{empty: true}})
			continue
		}
		p := doc.Pool.Windows[j]
		row.gauges = append(row.gauges, gauge{
			bar:   usageBar{used: p.Used, capAt: 1, pace: p.Pace},
			words: poolWords(p.Room, len(doc.Pool.Accounts), pinned),
		})
		pinned = false
	}
	return row, true
}

// caps is the key line's cap part: ╎ and the cap of each account whose bars,
// bar cells long, draw its mark, in the config's order, as in "cap: work at
// 95%, side at 90%"; nil where none does, as where every cap is off its bars.
func (b usageBlock) caps(bar int) line {
	var caps []string
	for _, r := range b.rows {
		if r.cap != "" && slices.ContainsFunc(r.gauges, func(g gauge) bool { _, ok := g.bar.capCell(bar); return ok }) {
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
// sessions go to.
type accountName struct {
	number    string
	numberInk ink
	label     string
	primary   bool
	best      bool
}

// namesOf are the names of doc's accounts, in the config's order, numbered
// right-aligned in a column digits cells wide.
func namesOf(doc status.Document, digits int) []accountName {
	best := bestOf(doc)
	names := make([]accountName, len(doc.Accounts))
	for i, a := range doc.Accounts {
		k := dimInk
		if len(doc.Pool.Accounts) > 0 && !slices.Contains(doc.Pool.Accounts, a.ID) {
			k = faintInk
		}
		names[i] = accountName{
			number: fmt.Sprintf("%*d", digits, i+1), numberInk: k, label: name(a), primary: a.Primary, best: a.ID == best,
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
	l := line{{n.number, n.numberInk}, {" " + label, titleInk}}
	if n.primary {
		l = append(l, span{" ◆", primaryInk})
	}
	if n.best {
		l = append(l, span{" ▲", accentInk})
	}
	return l
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

// draw draws the block's rows from row y, as its grouping and its width lay
// them out, and returns the row after the last: by account; else as a
// matrix; else its windows stacked.
func (b usageBlock) draw(c *canvas, y int) int {
	switch {
	case b.by == ByAccount:
		return b.byAccount(c, y)
	case b.matrixed():
		return b.matrix(c, y)
	default:
		return b.stacked(c, y)
	}
}

// matrixed reports whether the block, by window, draws as a matrix: from 150
// columns, where its bars would be 12 cells or more.
func (b usageBlock) matrixed() bool {
	return b.width >= matrixWidth && b.matrixBar() >= leastMatrixBar
}

// bar is the cells of the block's bars, as its layout has them, none where
// the width leaves none.
func (b usageBlock) bar() int {
	switch {
	case b.by == ByAccount:
		labelAt, cells := b.labels()
		return max(b.width-labelAt-cells-afterBar-b.wordCells()-afterWords, 0)
	case b.matrixed():
		return b.matrixBar()
	default:
		return max(b.width-b.names-afterBar-b.wordCells()-afterWords, 0)
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
	room := b.width - matrixNames - groupCells*k + groupGap
	return min(mostMatrixBar, room/k) - (b.names-matrixNames+k-1)/k
}

// matrix draws the block's windows side by side, a group each from the name
// column, its heading over it, and a row an account, the pool last, from row
// y, and returns the row after the last.
func (b usageBlock) matrix(c *canvas, y int) int {
	bar := b.bar()
	group := bar + groupCells
	for i, w := range b.windows {
		c.line(b.names+i*group, y, line{{strings.ToUpper(w.name), dimInk}})
	}
	return b.rowsFrom(c, y+2, func(r usageRow, y int) {
		c.line(0, y, r.name)
		for i, g := range r.gauges {
			next := b.names + (i+1)*group
			b.gauge(c, g, next-group, y, bar, next-groupGap)
		}
	})
}

// stacked draws the block's windows one under another, each a section, its
// heading, then a row an account, the pool last, from row y, two blank lines
// between sections, and returns the row after the last.
func (b usageBlock) stacked(c *canvas, y int) int {
	bar := b.bar()
	for i, w := range b.windows {
		if i > 0 {
			y += 2
		}
		c.line(0, y, line{{strings.ToUpper(w.name), dimInk}})
		y = b.rowsFrom(c, y+2, func(r usageRow, y int) {
			c.line(1, y, r.name)
			b.gauge(c, r.gauges[i], b.names, y, bar, b.width-afterWords)
		})
	}
	return y
}

// rowsFrom draws each of the block's rows, as draw draws one, from row y, a
// blank line under each, the pool's rule in the one above it, and returns
// the row after the last.
func (b usageBlock) rowsFrom(c *canvas, y int, draw func(r usageRow, y int)) int {
	for i, r := range b.rows {
		if i > 0 {
			y += 2
		}
		if r.pool {
			c.line(0, y-1, line{{rule(b.width), borderInk}})
		}
		draw(r, y)
	}
	return y + 1
}

// byAccount draws a block an account from row y, each window a row of it,
// the pool's last, two blank lines between blocks, the pool's rule in the
// second, and returns the row after the last: wide, from 110 columns, the
// name beside its first window's row and each label in the name column; else
// the name on a line of its own above, and the labels a cell in.
func (b usageBlock) byAccount(c *canvas, y int) int {
	labelAt, cells := b.labels()
	bar := b.bar()
	for i, r := range b.rows {
		if i > 0 {
			y += 3
		}
		if r.pool {
			c.line(0, y-1, line{{rule(b.width), borderInk}})
		}
		c.line(0, y, r.name)
		if b.width < wideAccounts {
			y++
		}
		for j, g := range r.gauges {
			if j > 0 {
				y += 2
			}
			c.line(labelAt, y, line{{truncate(status.Short(b.windows[j].name), cells-1), dimInk}})
			b.gauge(c, g, labelAt+cells, y, bar, b.width-afterWords)
		}
	}
	return y + 1
}

// labels are the column a block by account's window labels start at, and the
// cells they take: wide, from 110 columns, the name column and 10 cells;
// else a cell in, and a cell more.
func (b usageBlock) labels() (at, cells int) {
	if b.width < wideAccounts {
		return 1, windowLabelCells + 1
	}
	return b.names, windowLabelCells
}

// gauge draws g from x along row y: its bar, bar cells long; a cell; its
// percentage, right-aligned in its cells; two cells; and its words, in the
// form the block's width has them, ending before column end.
func (b usageBlock) gauge(c *canvas, g gauge, x, y, bar, end int) {
	g.bar.draw(c, b.look, x, y, bar)
	at := x + bar + 1
	c.right(at+percentCells, y, g.bar.percentage())
	words := at + percentCells + 2
	c.line(words, y, g.words.pick(b.width, end-words))
}

// key draws the block's key line at row y, saying what its marks mean: from
// 150 columns, one line from the name column, the cap's part after even
// pace's; narrower, two lines from column 0, the cap's part on the second.
func (b usageBlock) key(c *canvas, y int) {
	pace := line{{paceMark, paceInk}, {" even pace: where you would be now, used evenly", mutedInk}}
	caps := b.caps(b.bar())
	if b.width >= matrixWidth {
		c.line(b.names, y, joined(span{"    ", mutedInk}, pace, caps).fit(b.width-b.names))
		return
	}
	c.line(0, y, pace.fit(b.width))
	if caps != nil {
		c.line(0, y+1, caps.fit(b.width))
	}
}

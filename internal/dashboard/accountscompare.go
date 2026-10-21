package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Compare's columns and rows, in cells.
const (
	// compareNameLeast and compareNameMost are the fewest and the most cells
	// a column of names or labels takes, its widest whole between them.
	compareNameLeast, compareNameMost = 8, 24
	// labelIndent is where the groups' rows' labels and BY MODEL's ● start,
	// and versionAt where a version's name does; labelsGap are the cells
	// between the labels' column and the first account's.
	labelIndent = 2
	versionAt   = labelIndent + 2
	labelsGap   = 2
	// leastColumn are the fewest cells a column takes, as many accounts
	// showing as fit at it, less all accounts'.
	leastColumn = 19
	// barMost are the most cells a bar beside words takes, so none reads as
	// a rule; and shareSpare the cells a column keeps beside BY MODEL's.
	barMost    = 40
	shareSpare = 4
	// compareHeads are the rows the heads take, held however far the rest
	// scrolls: the names, the states, and a blank row.
	compareHeads = 3
	// compareByModel is BY MODEL's row under the heads, a blank row after WEEK
	// USED's; and groupApart is how far each group's heading is from the
	// row before it, a blank row between.
	compareByModel = 2
	groupApart     = 2
)

// compareLayout is where Compare's parts sit across a frame, as flex lays
// them: the labels' column's cells; each column's span, the accounts' shown,
// from the first, then all accounts'; and the cells of BY MODEL's bars, and
// of WEEK USED's, which ends with them, its percentage a cell on.
type compareLayout struct {
	labels int
	slots  []flex.Span
	share  int
	week   int
}

// shown is how many accounts the layout shows a column of, beside all
// accounts'.
func (l compareLayout) shown() int {
	return len(l.slots) - 1
}

// compareLaid is Compare laid out across the frame: the labels' column at its
// natural width, then, labelsGap on, a column an account, and all accounts'
// last, each at least leastCompared cells. Where they don't all fit, the
// accounts' columns give way from the last, the first never; those left
// share the rest alike, the odd cell all accounts'. The bars are as
// compareBars has them in the narrowest column.
func (f Frame) compareLaid() compareLayout {
	n, least, labels := len(f.Accounts.Accounts), f.leastCompared(), f.compareLabels()
	parts := []flex.Part{{Least: labels, Natural: labels}}
	for i := range n {
		slot := flex.Part{Least: least, Natural: least, Grows: true, Leaves: n - i}
		if i == 0 {
			slot.Leaves, slot.Gap = 0, labelsGap
		}
		parts = append(parts, slot)
	}
	parts = append(parts, flex.Part{Least: least, Natural: least, Grows: true, Odd: true})
	spans, _ := flex.Lay(f.Width, parts...)
	l := compareLayout{labels: spans[0].Size}
	for _, s := range spans[1:] {
		if !s.Left {
			l.slots = append(l.slots, s)
		}
	}
	l.share, l.week = compareBars(slices.MinFunc(l.slots, func(a, b flex.Span) int { return a.Size - b.Size }).Size)
	return l
}

// compareBars are the cells of BY MODEL's bars and of WEEK USED's in a
// column the cells given: BY MODEL's the column less shareSpare, at most
// barMost; WEEK USED's ending with it, a cell and its percentage after.
func compareBars(column int) (share, week int) {
	bars, _ := flex.Lay(column, flex.Part{Grows: true, Most: barMost}, flex.Part{Least: shareSpare, Natural: shareSpare})
	weekly, _ := flex.Lay(bars[0].Size, flex.Part{Grows: true}, flex.Part{Least: percentCells, Natural: percentCells, Gap: 1})
	return bars[0].Size, weekly[0].Size
}

// compareLabels are the cells of Compare's labels' column: its widest label
// whole, at most compareNameMost. Its labels are WEEK USED's, BY MODEL's
// heading, each version's name, and the groups' names and their rows'
// labels, from where each is drawn.
func (f Frame) compareLabels() int {
	widest := max(weekLabel(f.Accounts.Period).width(), groupHead("BY MODEL", "share of use", false).width())
	if f.Accounts.Page != nil {
		for _, v := range f.Accounts.Page.Versions {
			widest = max(widest, versionAt+ansi.StringWidth(prose.Clean(v.Name)))
		}
	}
	for _, g := range (compared{f: f}).groups() {
		widest = max(widest, ansi.StringWidth(g.name))
		for _, r := range g.rows {
			widest = max(widest, labelIndent+ansi.StringWidth(r.label))
		}
	}
	return min(widest, compareNameMost)
}

// leastCompared are the fewest cells a column of Compare takes: leastColumn,
// or, where it's more, the widest account's head and a cell: its number and
// a space, then its name whole, at most compareNameMost; so no account's
// name is cut where another's column would show it whole.
func (f Frame) leastCompared() int {
	widest := 0
	for i, a := range f.Accounts.Accounts {
		number := strconv.Itoa(i + 1)
		named := accountName{label: labelled(a.Label, a.ID), primary: a.Primary}
		widest = max(widest, len(number)+1+min(wholeName(named), compareNameMost))
	}
	return max(leastColumn, widest+1)
}

// wholeName are the cells the name takes unnumbered, its label whole and its
// marks: a name's natural width.
func wholeName(n accountName) int {
	return ansi.StringWidth(n.label) + n.marks().width()
}

// weekLabel is WEEK USED's label, and now after it where the period is
// another than this week, as the week is always as it stands.
func weekLabel(period views.AccountsPeriod) line {
	label := line{{"WEEK USED", dimInk}}
	if period != views.AccountsThisWeek {
		label = append(label, span{"  now", faintInk})
	}
	return label
}

// compareOut is the line saying how many of Compare's rows are out of view,
// and how to scroll to them: "▼ 3 more below · j/k".
var compareOut = outForm{above: " above", below: " below", gap: " · ", keys: "j/k", inks: keyedOut}

// compared is Compare as a frame lays it out of a moment: the accounts its
// columns show, from the first shown, then all accounts'; the key of the
// accounts' own week, as the frame names it, which WEEK USED shows; and,
// where it's laid out to draw wide, as comparing lays it out, where its
// parts sit, and its rows drawn whole, once, for every look to share.
type compared struct {
	f     Frame
	m     status.Moment
	now   time.Time
	page  *views.AccountsPage
	first int
	shown int
	week  string
	laid  compareLayout
	rows  func() *canvas
}

// compareColumn is a column of Compare: from column x, its values cut to
// width cells and its heads to head cells, its column's less a cell, but
// all accounts' values, to the frame's edge; of an account, by its id and
// place in the config, as it stands, where the moment has it, and its row of
// the period's page; or, of all accounts, every account's.
type compareColumn struct {
	x, width, head int
	all            bool
	id             string
	place          int
	account        status.AccountNow
	known          bool
	row            views.AccountsRow
}

// compareForm is Compare as it's drawn wide, of doc at now: its heads,
// held, then its rows, scrolled as the frame has them, as compared lays them
// out.
func (f Frame) compareForm(doc status.Document, now time.Time) accountsForm {
	if len(f.Accounts.Accounts) == 0 {
		return accountsForm{}
	}
	cp := f.comparing(doc, now)
	return accountsForm{draw: cp.draw, scrolling: cp.scrolling, scrolls: cp.scrolls, targets: cp.targets, filled: cp.filled}
}

// comparing is Compare of doc at now, laid out as the frame has it, its rows
// drawn whole once, the first time a look asks.
func (f Frame) comparing(doc status.Document, now time.Time) compared {
	m, laid := f.Moment(doc, now), f.compareLaid()
	cp := compared{
		f: f, m: m, now: now, page: f.Accounts.Page, first: f.compareFirst(), shown: laid.shown(),
		week: f.WeekWindow, laid: laid,
	}
	cp.rows = sync.OnceValue(func() *canvas { return cp.body(cp.columns()) })
	return cp
}

// compareShows is how many accounts Compare shows a column of, beside all
// accounts', as compareLaid lays them out.
func (f Frame) compareShows() int {
	return f.compareLaid().shown()
}

// compareFirst is the place among the accounts, from 0, of the first Compare
// shows: as far along as the frame has it, but no further than leaves a
// column of each shown.
func (f Frame) compareFirst() int {
	return min(max(f.Accounts.First, 0), len(f.Accounts.Accounts)-f.compareShows())
}

// CompareFirst is the place among the accounts, from 0, of the first Compare
// shows, its columns scrolled to show the account with the id given: as far
// along as the frame has them, where that shows it, else no further than
// they must go to. All accounts, "", always shows, as does an account the
// config doesn't hold, so neither moves them.
func (f Frame) CompareFirst(id string) int {
	n, shown := len(f.Accounts.Accounts), f.compareShows()
	i := slices.IndexFunc(f.Accounts.Accounts, func(a config.Account) bool { return a.ID == id })
	return Scrolling{Most: max(n-shown, 0), Page: shown}.revealing(f.Accounts.First, i)
}

// compareState is what Compare's heading says after [ ] where it shows fewer
// accounts than there are: which it shows, as "accounts 1–6 of 10", that ←→
// moves along them, and that all accounts stays put; and, a clause shorter,
// for less room, without the last. The heading's room at 100 columns holds
// the shorter form, so none shorter is needed. It says nothing on a phone,
// whose Compare shows every account a row.
func (f Frame) compareState() []line {
	n, shown := len(f.Accounts.Accounts), f.compareShows()
	if f.phone() || shown >= n {
		return nil
	}
	first := f.compareFirst()
	moves := line{
		{fmt.Sprintf("accounts %d–%d of %d", first+1, first+shown, n), faintInk},
		{"  ·  ", faintInk}, {"←→", keyInk}, {" moves along them", faintInk},
	}
	return []line{slices.Concat(moves, line{{"; all accounts stays put", faintInk}}), moves}
}

// columns are Compare's columns: each account's shown, from the first, then
// all accounts', each in its place where it's laid out to draw wide, all
// accounts' values running to the frame's edge.
func (cp compared) columns() []compareColumn {
	columns := make([]compareColumn, 0, cp.shown+1)
	for i := range cp.shown {
		columns = append(columns, cp.column(cp.first+i))
	}
	columns = append(columns, cp.allColumn())
	for i, s := range cp.laid.slots {
		columns[i].x, columns[i].head, columns[i].width = s.At, s.Size-1, s.Size-1
	}
	if last := len(cp.laid.slots) - 1; last >= 0 {
		columns[last].width = cp.f.Width - columns[last].x
	}
	return columns
}

// column is the column of the configured account at the place given among
// them, from 0, wherever it's shown: as the moment has it, else as the
// config names it, and its row of the period's page.
func (cp compared) column(i int) compareColumn {
	configured := cp.f.Accounts.Accounts[i]
	col := compareColumn{id: configured.ID, place: i + 1}
	col.account, col.known = cp.m.Account(configured.ID)
	if !col.known {
		col.account = status.AccountNow{ID: configured.ID, Label: configured.Label, Primary: configured.Primary}
	}
	if cp.page != nil {
		if j := slices.IndexFunc(cp.page.Rows, func(r views.AccountsRow) bool { return r.Totals.Account == configured.ID }); j >= 0 {
			col.row = cp.page.Rows[j]
		}
	}
	return col
}

// allColumn is all accounts' column.
func (cp compared) allColumn() compareColumn {
	all := compareColumn{all: true}
	if cp.page != nil {
		all.row = cp.page.All
	}
	return all
}

// ink is the ink of the column's values: all accounts' bold.
func (col compareColumn) ink() ink {
	if col.all {
		return strongInk
	}
	return secondaryInk
}

// draw draws Compare from row top: its heads, held, then its rows, as far
// down as the frame has them scrolled, as many as the view has room for
// under the heads, and over the key line, where they don't all show, how
// many are out of view.
func (cp compared) draw(c *canvas, top int) {
	cp.heads(c, top, cp.columns())
	body := cp.rows()
	paged := pagedWhole(body, cp.bodyRows())
	from, to := paged.shown(cp.f.Scroll)
	c.paste(body, from, top+compareHeads, to-from)
	c.line(0, cp.f.Height-underView, outOfView(from, paged.content-to, cp.f.edge(), compareOut))
}

// bodyRows are the rows Compare's rows show in, under its heads.
func (cp compared) bodyRows() int {
	return max(cp.f.viewRows()-compareHeads, 0)
}

// scrolling is how far Compare's rows scroll under its heads, a row at a
// time, scrolled offset down, as pagedWhole pages them.
func (cp compared) scrolling(offset int) Scrolling {
	return pagedWhole(cp.rows(), cp.bodyRows()).scrolling(offset)
}

// scrolls is what of Compare scrolls: its rows under its heads, across the
// frame.
func (cp compared) scrolls() (x, y, width, rows int) {
	return 0, cp.f.top() + compareHeads, cp.f.Width, cp.bodyRows()
}

// targets are Compare's heads, each column's, a click on which moves the
// focus to it: its name and its state, each across its heads' cells.
func (cp compared) targets() []Target {
	top := cp.f.top()
	var targets []Target
	for _, col := range cp.columns() {
		for y := range compareHeads - 1 {
			targets = append(targets, Target{X: col.x, Y: top + y, Width: col.head, On: OnColumn, Account: col.id})
		}
	}
	return targets
}

// filled are how much of each of WEEK USED's bars Compare draws it fills, as
// Filled has them: every configured account's, shown or scrolled out of
// view, as the Overview's are, so a column scrolled into view while its bar
// eases eases from where it stood, by its id and its week's key; and all
// accounts', by the key alone; none drawn empty.
func (cp compared) filled() map[Ref]float64 {
	filled := make(map[Ref]float64)
	columns := []compareColumn{cp.allColumn()}
	for i := range cp.f.Accounts.Accounts {
		columns = append(columns, cp.column(i))
	}
	for _, col := range columns {
		if g := cp.weekGauge(col); !g.bar.empty {
			filled[g.of] = g.bar.used
		}
	}
	return filled
}

// heads draws the columns' heads from row top: each account's number, its
// name and its marks, as usage names an account, but for ▲, as Accounts
// shows no routing, and under them its state in a word; all accounts', 0 and
// its name, and its accounts' states counted; the focused column's on
// bg.selection, and over it, a state cued as it changes, a warning showing
// through the focus.
func (cp compared) heads(c *canvas, top int, columns []compareColumn) {
	s := cp.f.Accounts
	for _, col := range columns {
		c.line(col.x, top, cp.named(col, col.head))
		stands := cp.stands(col, col.head)
		c.line(col.x, top+1, stands)
		if s.Focused && s.Focus == col.id {
			for y := range compareHeads - 1 {
				c.surface(col.x, top+y, col.head, hue{token: theme.BgSelection})
			}
		}
		cp.f.cueState(c, col.id, col.x, top+1, stands.width())
	}
}

// named is the column's head in the cells given: an account's number, its
// name, cut short to keep its number and marks, and its marks but ▲; all
// accounts', 0 and its name.
func (cp compared) named(col compareColumn, cells int) line {
	if col.all {
		return line{{"0", dimInk}, {" all accounts", strongInk}}.fit(cells)
	}
	named := accountName{number: strconv.Itoa(col.place), label: name(col.account), primary: col.account.Primary}
	return named.labelIn(cells - len(named.number) - 1 - named.marks().width())
}

// stands is how the column's account stands, in the cells given: its state
// in a word, as stateWord says it, as StateInAWord has it; all accounts',
// every configured account's states counted, as CountStatesOf counts them.
func (cp compared) stands(col compareColumn, cells int) line {
	if col.all {
		return countedStates(cp.m.CountStatesOf(cp.f.Accounts.Accounts.IDs()), NumberAccountStates, cells)
	}
	return stateWord(cp.m.StateInAWord(col.id)).fit(cells)
}

// stateWord is an account's state in a word, after its glyph, as the
// Overview colours a state: open's glyph in state.positive and its word in
// text.secondary; any other's both in its colour, as stateInk has it.
func stateWord(s status.AccountState) line {
	word := stateInk(s)
	if s == status.StateOpen {
		word = secondaryInk
	}
	return line{stateMark(s), {" " + s.Word(), word}}.inked()
}

// stateMark is an account's state's glyph, as stateWord colours it, and
// Detail's list draws it alone.
func stateMark(s status.AccountState) span {
	if s == status.StateOpen {
		return span{s.Glyph(), positiveInk}
	}
	return span{s.Glyph(), stateInk(s)}
}

// body is Compare's rows under its heads, drawn on a canvas of their own:
// WEEK USED; then, once the period's page is read, BY MODEL and its
// versions, ACTIVITY, LIMITS and VALUE, a blank row before each.
func (cp compared) body(columns []compareColumn) *canvas {
	if cp.page == nil {
		c := newCanvas(cp.f.Width, 1)
		cp.weekUsed(c, 0, columns)
		return c
	}
	groups := cp.groups()
	rows := compareByModel + 1 + len(cp.page.Versions)
	for _, g := range groups {
		rows += groupApart + len(g.rows)
	}
	c := newCanvas(cp.f.Width, rows)
	cp.weekUsed(c, 0, columns)
	y := cp.byModel(c, compareByModel, columns)
	for _, g := range groups {
		y += groupApart
		c.line(0, y, line{{g.name, dimInk}})
		c.heading(y, y+1)
		for _, r := range g.rows {
			y++
			c.line(labelIndent, y, line{{r.label, secondaryInk}})
			for _, col := range columns {
				c.line(col.x, y, fitParts(col.width, periodNumbered(r.said(col), col.id, r.label)...))
			}
		}
	}
	return c
}

// weekUsed draws WEEK USED along row y: its label, as weekLabel has it; and
// each column's week, as weekGauge has it, its bar as the layout has it, a
// cell, and its percentage, right-aligned in 4, ending with BY MODEL's bar.
func (cp compared) weekUsed(c *canvas, y int, columns []compareColumn) {
	c.line(0, y, weekLabel(cp.f.Accounts.Period))
	bar := cp.laid.week
	for _, col := range columns {
		g := cp.weekGauge(col)
		g.bar.draw(c, cp.f.Look, col.x, y, bar)
		c.barred(col.x, y, bar, g.of)
		c.right(col.x+bar+1+percentCells, y, g.percentage())
	}
}

// weekGauge is the column's week as WEEK USED draws it, as far as its bar
// has eased: an account's as usage draws its window, its cap and even pace
// marked, faded where it's stale, and empty where the moment doesn't have
// the account; all accounts', as allGauge has it.
func (cp compared) weekGauge(col compareColumn) gauge {
	g := gauge{of: Ref{Account: col.id, Window: cp.week}, bar: usageBar{empty: true}}
	switch {
	case col.all:
		g = allGauge(cp.m, cp.week)
	case col.known:
		g = accountGauge(col.account, cp.week, cp.now)
	}
	if used, ok := cp.f.Eased[g.of]; ok {
		g.bar.eased = &used
	}
	return g
}

// byModel draws BY MODEL from row y, and returns the row its last version is
// on: its heading, and on it each column's share of use as a stacked bar, as
// the layout has it, each version's part in its shade; then a row a version
// any account used in the period, in the version table's order, its ● in its
// shade and its name, cut to the labels' column, and each column's share of
// use of it, right-aligned in 4.
func (cp compared) byModel(c *canvas, y int, columns []compareColumn) int {
	c.line(0, y, groupHead("BY MODEL", "share of use", false))
	versions := cp.page.Versions
	hues := versionHues(versions)
	for _, col := range columns {
		stackedBar(c, cp.f.Look, col.x, y, splitCells(shareAmounts(col.row, versions), cp.laid.share), hues)
	}
	for i, v := range versions {
		y++
		c.text(labelIndent, y, "●", hues[i].ink())
		c.line(versionAt, y, line{{prose.Cut(v.Name, cp.laid.labels-versionAt), secondaryInk}})
		for _, col := range columns {
			var share *float64
			if s, ok := shareOf(col.row, v); ok {
				share = s.Share
			}
			c.rightFit(col.x+percentCells, col.x+col.width, y, periodNumbered(shareParts(share, col.ink()), col.id, shareNamed(v))...)
		}
	}
	return y
}

// shareOf is the row's share of use of the version given, reporting false
// where it used none of it.
func shareOf(row views.AccountsRow, v ledger.Version) (views.ModelShare, bool) {
	i := slices.IndexFunc(row.Totals.ByModel, func(s views.ModelShare) bool { return slices.Contains(v.IDs, s.Model) })
	if i < 0 {
		return views.ModelShare{}, false
	}
	return row.Totals.ByModel[i], true
}

// stackedBar draws a bar from x along row y, its parts runs cells long, one
// after another, each in its hue: where the look blends its colours, a space
// on it; else a full block in it.
func stackedBar(c *canvas, look Look, x, y int, runs []int, hues []hue) {
	for i, n := range runs {
		glyph, k := "█", hues[i].ink()
		if look.blends() {
			glyph, k = " ", ink{on: hues[i]}
		}
		c.text(x, y, strings.Repeat(glyph, n), k)
		x += n
	}
}

// compareGroup is a group of Compare's rows, as ACTIVITY: its name, and its
// rows.
type compareGroup struct {
	name string
	rows []compareRow
}

// compareRow is a row of Compare's groups: its label, and what it says of
// each column.
type compareRow struct {
	label string
	said  func(col compareColumn) []part
}

// groups are Compare's groups under BY MODEL: ACTIVITY, LIMITS and VALUE, as
// Accounts' words say each figure. A period with nothing recorded, as on a
// fresh install, says a dash in each of its own figures, as period has it;
// left at its last reset, the plan and extra usage are none of them. Its
// names and labels read no page, so they're known before it is.
func (cp compared) groups() []compareGroup {
	names := cp.f.windowNamer(cp.m)
	return []compareGroup{
		{name: "ACTIVITY", rows: []compareRow{
			{"requests", cp.period(func(col compareColumn) []part {
				return []part{figureIn(prose.Grouped(col.row.Totals.Requests), col.ink())}
			})},
			{"sessions served", cp.period(func(col compareColumn) []part {
				return []part{figureIn(prose.Grouped(col.row.Totals.Sessions), col.ink())}
			})},
			{"sessions moved", cp.period(func(col compareColumn) []part { return movedParts(col.row.Totals, col.ink()) })},
		}},
		{name: "LIMITS", rows: []compareRow{
			{"limits hit", cp.period(func(col compareColumn) []part { return limitsParts(col.row.Totals, names, col.ink()) })},
			{"at cap or limit", cp.period(func(col compareColumn) []part { return atCapParts(col.row.Totals, col.ink()) })},
			{"left at its last reset", func(col compareColumn) []part { return leftParts(col.row, col.ink()) }},
		}},
		{name: "VALUE", rows: []compareRow{
			{"plan", func(col compareColumn) []part { return planParts(col.row, col.ink()) }},
			{"extra usage", cp.extra},
			{"worth at API prices", cp.period(func(col compareColumn) []part { return worthParts(col.row, col.ink()) })},
			{"against its price", cp.period(func(col compareColumn) []part { return againstParts(*cp.page, col.row, col.width) })},
		}},
	}
}

// period is what a row of Compare's groups says of each column, as said says
// it, as periodParts has a row of the period.
func (cp compared) period(said func(col compareColumn) []part) func(col compareColumn) []part {
	return func(col compareColumn) []part { return periodParts(*cp.page, said(col), col.ink()) }
}

// extra is the column's extra usage, as it stands: an account's, as
// extraParts says it, or every configured account's, as extraOfAll does, as
// ExtraOf takes them together.
func (cp compared) extra(col compareColumn) []part {
	if col.all {
		return extraOfAll(cp.m.ExtraOf(cp.f.Accounts.Accounts.IDs()), col.ink())
	}
	if !col.known {
		return noneOf(noFigure.text, col.ink())
	}
	return extraParts(col.account, col.ink())
}

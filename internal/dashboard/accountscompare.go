package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Compare's columns and rows, in cells.
const (
	// compareLabels are the labels' columns, 0 to 25: the first column
	// starts after them.
	compareLabels = 26
	// leastColumn are the fewest cells a column takes, as many accounts
	// showing as fit at it, less all accounts'.
	leastColumn = 19
	// weekBar and shareBar are WEEK USED's bar and BY MODEL's at their
	// widest; a column keeps weekSpare and shareSpare cells beside them.
	weekBar, weekSpare   = 18, 8
	shareBar, shareSpare = 24, 4
	// compareHeads are the rows the heads take, held however far the rest
	// scrolls: the names, the states, and a blank row.
	compareHeads = 3
	// compareByModel is BY MODEL's row under the heads, a blank row after WEEK
	// USED's; and groupApart is how far each group's heading is from the
	// row before it, a blank row between.
	compareByModel = 2
	groupApart     = 2
)

// compareOut is the line saying how many of Compare's rows are out of view,
// and how to scroll to them: "▼ 3 more below · j/k".
var compareOut = outForm{above: " above", below: " below", gap: " · ", keys: "j/k", inks: keyedOut}

// compared is Compare as a frame lays it out of a moment: the accounts its
// columns show, from the first shown, then all accounts'; each colw cells
// wide; and the key of the accounts' own week, as the frame names it, which
// WEEK USED shows.
type compared struct {
	f     Frame
	m     status.Moment
	now   time.Time
	page  *views.AccountsPage
	first int
	shown int
	colw  int
	week  string
}

// compareColumn is a column of Compare: from column x, its values cut to
// width cells, its heads' width cells less a cell; of an account, by its id
// and place in the config, as it stands, where the moment has it, and its
// row of the period's page; or, of all accounts, every account's.
type compareColumn struct {
	x, width int
	all      bool
	id       string
	place    int
	account  status.AccountNow
	known    bool
	row      views.AccountsRow
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

// comparing is Compare of doc at now, laid out as the frame has it.
func (f Frame) comparing(doc status.Document, now time.Time) compared {
	m := f.Moment(doc, now)
	shown := f.compareShows()
	return compared{
		f: f, m: m, now: now, page: f.Accounts.Page, first: f.compareFirst(), shown: shown,
		colw: (f.Width - compareLabels) / (shown + 1), week: f.WeekWindow,
	}
}

// compareShows is how many accounts Compare shows a column of, beside all
// accounts': as many as fit leastColumn cells each, at least one, and no
// more than there are.
func (f Frame) compareShows() int {
	return min(max(1, (f.Width-compareLabels)/leastColumn-1), len(f.Accounts.Accounts))
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
// all accounts', the last running to the frame's edge.
func (cp compared) columns() []compareColumn {
	columns := make([]compareColumn, 0, cp.shown+1)
	for i := range cp.shown {
		col := cp.column(cp.first + i)
		col.x = compareLabels + i*cp.colw
		columns = append(columns, col)
	}
	return append(columns, cp.allColumn(compareLabels+cp.shown*cp.colw))
}

// column is the column of the configured account at the place given among
// them, from 0, wherever it's shown: as the moment has it, else as the
// config names it, and its row of the period's page.
func (cp compared) column(i int) compareColumn {
	configured := cp.f.Accounts.Accounts[i]
	col := compareColumn{width: cp.colw - 1, id: configured.ID, place: i + 1}
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

// allColumn is all accounts' column, from column x to the frame's edge.
func (cp compared) allColumn(x int) compareColumn {
	all := compareColumn{x: x, width: cp.f.Width - x, all: true}
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
	columns := cp.columns()
	cp.heads(c, top, columns)
	body := cp.body(columns)
	content, rows := body.height(), cp.bodyRows()
	offset := byRows(content, rows).clamped(cp.f.Scroll)
	c.paste(body, offset, top+compareHeads, rows)
	if content > rows {
		c.line(0, cp.f.Height-underView, outOfView(offset, content-offset-rows, cp.f.edge(), compareOut))
	}
}

// bodyRows are the rows Compare's rows show in, under its heads.
func (cp compared) bodyRows() int {
	return max(cp.f.viewRows()-compareHeads, 0)
}

// scrolling is how far Compare's rows scroll under its heads, a row at a
// time, at every offset alike.
func (cp compared) scrolling(int) Scrolling {
	return byRows(cp.body(cp.columns()).height(), cp.bodyRows())
}

// scrolls is what of Compare scrolls: its rows under its heads, across the
// frame.
func (cp compared) scrolls() (x, y, width, rows int) {
	return 0, cp.f.top() + compareHeads, cp.f.Width, cp.bodyRows()
}

// targets are Compare's heads, each column's, a click on which moves the
// focus to it: its name and its state, each across the column less a cell.
func (cp compared) targets() []Target {
	top := cp.f.top()
	var targets []Target
	for _, col := range cp.columns() {
		for y := range compareHeads - 1 {
			targets = append(targets, Target{X: col.x, Y: top + y, Width: cp.colw - 1, On: OnColumn, Account: col.id})
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
	columns := []compareColumn{cp.allColumn(0)}
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
	s, head := cp.f.Accounts, cp.colw-1
	for _, col := range columns {
		c.line(col.x, top, cp.named(col, head))
		stands := cp.stands(col, head)
		c.line(col.x, top+1, stands)
		if s.Focused && s.Focus == col.id {
			for y := range compareHeads - 1 {
				c.surface(col.x, top+y, head, hue{token: theme.BgSelection})
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
	if s == status.StateOpen {
		return line{{s.Glyph(), positiveInk}, {" " + s.Word(), secondaryInk}}
	}
	return line{{s.Glyph() + " " + s.Word(), stateInk(s)}}
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
		for _, r := range g.rows {
			y++
			c.line(2, y, line{{r.label, secondaryInk}})
			for _, col := range columns {
				c.line(col.x, y, fitParts(col.width, r.said(col)...))
			}
		}
	}
	return c
}

// weekUsed draws WEEK USED along row y: its label, and now after it, where
// the period is another than this week, as the week is always as it stands;
// and each column's week, as weekGauge has it, its bar min(18, colw − 8)
// cells, a cell, and its percentage, right-aligned in 4.
func (cp compared) weekUsed(c *canvas, y int, columns []compareColumn) {
	label := line{{"WEEK USED", dimInk}}
	if cp.f.Accounts.Period != views.AccountsThisWeek {
		label = append(label, span{"  now", faintInk})
	}
	c.line(0, y, label)
	bar := min(weekBar, cp.colw-weekSpare)
	for _, col := range columns {
		g := cp.weekGauge(col)
		g.bar.draw(c, cp.f.Look, col.x, y, bar)
		c.barred(col.x, y, bar, g.of)
		percentage := g.percentage()
		if col.all && !g.bar.empty {
			percentage = g.bar.percentage().numbered(Number{Kind: NumberAllUsed, Of: cp.week})
		}
		c.right(col.x+bar+1+percentCells, y, percentage)
	}
}

// weekGauge is the column's week as WEEK USED draws it, as far as its bar
// has eased: an account's as usage draws its window, its cap and even pace
// marked, faded where it's stale, and empty where the moment doesn't have
// the account; all accounts', as every account's week together, the mean of
// their use and of their paces, with no cap, faded where all are stale, and
// empty where none's use is known.
func (cp compared) weekGauge(col compareColumn) gauge {
	g := gauge{of: Ref{Account: col.id, Window: cp.week}, bar: usageBar{empty: true}}
	switch {
	case col.all:
		all := cp.m.All().Windows
		if i := slices.IndexFunc(all, func(w status.AllWindowNow) bool { return w.Key == cp.week }); i >= 0 {
			g.bar = usageBar{used: all[i].Used, capAt: 1, pace: all[i].Pace, stale: all[i].Stale}
		}
	case col.known:
		g = accountGauge(col.account, cp.week, cp.now)
	}
	if used, ok := cp.f.Eased[g.of]; ok {
		g.bar.eased = &used
	}
	return g
}

// byModel draws BY MODEL from row y, and returns the row its last version is
// on: its heading, and on it each column's share of use as a stacked bar,
// min(24, colw − 4) cells, each version's part in its shade; then a row a
// version any account used in the period, in the version table's order, its
// ● in its shade and its name, and each column's share of use of it,
// right-aligned in 4.
func (cp compared) byModel(c *canvas, y int, columns []compareColumn) int {
	c.line(0, y, groupHead("BY MODEL", "share of use", false))
	versions := cp.page.Versions
	hues := make([]hue, len(versions))
	for i, v := range versions {
		hues[i] = versionHue(v)
	}
	bar := min(shareBar, cp.colw-shareSpare)
	for _, col := range columns {
		amounts := make([]ledger.Picodollars, len(versions))
		for i, v := range versions {
			if s, ok := shareOf(col.row, v); ok {
				amounts[i] = amountOf(s.Worth)
			}
		}
		stackedBar(c, cp.f.Look, col.x, y, splitCells(amounts, bar), hues)
	}
	for i, v := range versions {
		y++
		c.text(2, y, "●", hues[i].ink())
		c.line(4, y, line{{prose.Cut(v.Name, compareLabels-5), secondaryInk}})
		for _, col := range columns {
			var share *float64
			if s, ok := shareOf(col.row, v); ok {
				share = s.Share
			}
			c.right(col.x+percentCells, y, fitParts(percentCells, shareParts(share, col.ink())...))
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
// fresh install, says a dash for each of its figures, as period has it.
func (cp compared) groups() []compareGroup {
	page := *cp.page
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
			{"plan", func(col compareColumn) []part { return planParts(page, col.row, col.ink()) }},
			{"extra usage", cp.extra},
			{"worth at API prices", cp.period(func(col compareColumn) []part { return worthParts(col.row, col.ink()) })},
			{"against its price", cp.period(func(col compareColumn) []part { return againstParts(page, col.row) })},
		}},
	}
}

// period is what a row of the period's figures says of each column, as said
// says it, as periodParts has a figure of the period.
func (cp compared) period(said func(col compareColumn) []part) func(col compareColumn) []part {
	return func(col compareColumn) []part { return periodParts(*cp.page, said(col), col.ink()) }
}

// extra is the column's extra usage, as it stands: an account's, as
// extraParts says it, or every account's, as extraOfAll does.
func (cp compared) extra(col compareColumn) []part {
	if col.all {
		return extraOfAll(cp.m.Accounts, col.ink())
	}
	if !col.known {
		return noneOf(noFigure.text, col.ink())
	}
	return extraParts(col.account, col.ink())
}

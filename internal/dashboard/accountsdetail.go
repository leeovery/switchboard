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
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Detail's list's columns: its rows from listAt, each account's name in the
// name column nameColumn fits to the names, and after it its state's dot or
// square; then, as many cells past the dot as given, the end of the chosen
// row's light, from column 0; the list's │; and the panel beside it.
const (
	listAt       = 1
	litPastDot   = 2
	rulePastDot  = 5
	panelPastDot = 9
)

// Detail's list's rows: an account's listApart rows from the one before,
// and ↑↓ choose chooseUnder rows under the last shown.
const (
	listApart   = 2
	chooseUnder = 3
)

// Detail's panel's columns, in cells into it: where BY MODEL, BY DAY and
// WEEKS start, cx; and where LATELY starts beside SESSIONS.
const (
	panelCX      = 12
	latelyBeside = 48
)

// Detail's panel's rows, from its top: the head; USAGE's heading, its
// windows from the row after, windowsApart rows apart, and THIS WEEK's
// title on its row, its thisWeekRows items under it; BY MODEL, byModelGap
// rows under the lower of the two; and the parts under BY MODEL, each the
// rows given under it.
const (
	usageAt      = 2
	windowsApart = 2
	thisWeekRows = 5
	byModelGap   = 2
	ruleUnder    = 3
	byDayUnder   = 5
	weeksUnder   = 8
	verdictUnder = 10
	latelyUnder  = 12
)

// THIS WEEK's labels' cells and its figures' fewest, ending at the weeks'
// edge, and the cells before it past USAGE's words; and how far after its
// name the head says how the account stands.
const (
	thisWeekLabels  = 10
	thisWeekFigures = 10
	thisWeekApart   = 1
	headApart       = 3
)

// wideBarLeast are the fewest cells USAGE's bars take wide, two fewer than
// the stack's, so the wide form keeps four weeks at 108 columns beside the
// widest extra use THIS WEEK says of three accounts.
const wideBarLeast = 16

// SESSIONS: as many rows as sessionRows show at once, the one selected
// picked out across sessionLit cells from a cell before its ▸; and its
// enter line, enterUnder rows under its heading, two cells in.
const (
	sessionRows = 4
	sessionLit  = 43
	enterUnder  = 6
	enterIn     = 2
)

// The wide form's weeks: six at most, the last of them this one; their gap
// at its widest, narrowing a cell at a time to its least before a week
// leaves; and their days' cells at least, growing where six weeks at the
// widest gap have room for wider.
const (
	sixWeeks      = 6
	weeksGapMost  = 4
	weeksGapLeast = 2
	dayCellsLeast = 2
)

// The wide form's parts across a frame, in the order detailLaid lays them:
// the list's, its indent, its name column and the cells from it to the
// panel; then the panel's headings' column, to cx; and its weeks, two parts,
// to their natural, and past it.
const (
	detailIndent = iota
	detailNames
	detailToPanel
	detailHeadings
	detailWeeks
	detailWeeksPast
)

// The turns the wide form narrows in: the list's names first, toward their
// least, then the weeks, toward theirs.
const (
	namesNarrowed = 1 + iota
	weeksNarrowed
)

// listNamesMost are the most cells the list's name column takes.
const listNamesMost = 24

// detailLayout is Detail laid out wide, or on a phone, as stackLayout: with
// its list at its left, its name column names cells wide, the panel beside
// it, or without, the panel alone, across the frame; how far into the panel
// BY MODEL, BY DAY and WEEKS start; and its days' cells, the cells between
// its weeks, and, wide, how many weeks it shows.
type detailLayout struct {
	list        bool
	names       int
	panel, into int
	dw, gap     int
	weeks       int
	thisWeek    int
}

// detailLayout is Detail's wide layout, as detailLaid lays it.
func (f Frame) detailLayout() detailLayout {
	l, _ := f.detailLaid()
	return l
}

// detailLaid is Detail's wide layout, as flex lays its parts across the
// frame, and whether their leasts fit. The parts: the list, where there's
// more than one account, its name column from leastNames to listNamesMost;
// the panel's headings' column; and its weeks, at their natural, six at the
// widest gap, at least weeksLeast, and past it, growing. The list gives way
// first: its names narrow toward their least before the weeks narrow, and
// past the weeks' natural, they take their most before the weeks grow (the
// owner's call, 15 October 2026). The names are laid at their most whatever
// the labels, so the panel's layout follows the width alone; the list then
// takes what its names need of that, and of what the weeks, as wideWeeks
// lays them, leave of their room, and the panel sits after it.
func (f Frame) detailLaid() (detailLayout, bool) {
	l := detailLayout{into: panelCX, thisWeek: f.thisWeekCells()}
	natural := detailLayout{dw: dayCellsLeast, gap: weeksGapMost, weeks: sixWeeks}.weeksCells()
	parts := []flex.Part{
		detailIndent:    {},
		detailNames:     {},
		detailToPanel:   {},
		detailHeadings:  {Least: panelCX, Natural: panelCX},
		detailWeeks:     {Least: weeksLeast(usageWide(f.Width) + l.thisWeek), Natural: natural, Narrows: weeksNarrowed},
		detailWeeksPast: {Grows: true},
	}
	if !f.Accounts.Alone() {
		l.list = true
		parts[detailIndent] = flex.Part{Least: listAt, Natural: listAt}
		parts[detailNames] = flex.Part{Least: leastNames, Natural: listNamesMost, Narrows: namesNarrowed}
		parts[detailToPanel] = flex.Part{Least: panelPastDot, Natural: panelPastDot}
	}
	s, ok := flex.Lay(f.edge(), parts...)
	room := s[detailWeeks].Size + s[detailWeeksPast].Size
	l = l.wideWeeks(room)
	if l.list {
		l.names = min(f.listNames(), s[detailNames].Size+room-l.weeksCells())
		l.panel = l.dotAt() + panelPastDot
	}
	return l, ok
}

// usageWide are the cells USAGE's row takes wide, its parts at their leasts,
// on a frame width cells wide, its words in their form at their widest, and
// the cell after it before THIS WEEK.
func usageWide(width int) int {
	return detailUsageLeast(wideBarLeast, usageBlock{width: width}.widestWords()) + thisWeekApart
}

// thisWeekCells are the cells of THIS WEEK's column: its labels, and its
// figures' widest form that the configured accounts bound, at least
// thisWeekFigures: all accounts' extra use, as extraShort says it on for one
// fewer than every account, "on for 2 of 3". The same for every choice in
// the list, so nothing moves as the choice does. The requests, the plans'
// summed price, the plan's name from the price table and the worth have no
// such bound, and are cut at the weeks' edge.
func (f Frame) thisWeekCells() int {
	figures, n := thisWeekFigures, len(f.Accounts.Accounts)
	if n > 1 {
		figures = max(figures, partsWidth(extraShort(status.AllExtra{State: status.ExtraOn, On: n - 1, Of: n})))
	}
	return thisWeekLabels + figures
}

// weeksLeast are the fewest cells the wide form's weeks take: the least room
// whose weeks, as wideWeeks lays them, end reach cells past the panel's
// left, as USAGE's row and THIS WEEK, ending at the weeks' edge, need.
func weeksLeast(reach int) int {
	room := max(reach-panelCX, 0)
	for (detailLayout{}).wideWeeks(room).weeksCells() < reach-panelCX {
		room++
	}
	return room
}

// wideWeeks is the layout with its weeks laid out wide in room cells: its
// days dayCellsLeast cells, or as many more as six weeks at the widest gap
// have room for; then as many weeks as fit, six at most, as latestFitting
// lays them at the least gap; and their gap the widest from weeksGapMost
// they still fit at.
func (l detailLayout) wideWeeks(room int) detailLayout {
	l.dw = max(dayCellsLeast, (room-(sixWeeks-1)*weeksGapMost)/(7*sixWeeks))
	l.weeks = len(latestFitting(room, sixWeeks, 7*l.dw, weeksGapLeast))
	l.gap = weeksGapMost
	for l.gap > weeksGapLeast && len(latestFitting(room, l.weeks, 7*l.dw, l.gap)) < l.weeks {
		l.gap--
	}
	return l
}

// listNames is the cells of the name column Detail's list fits its accounts'
// names to, as nameColumn fits USAGE's: the widest, number, label and ◆, and
// a gap, within its least and its most.
func (f Frame) listNames() int {
	configured := f.Accounts.Accounts
	digits := len(strconv.Itoa(len(configured)))
	names := make([]accountName, len(configured))
	for i, a := range configured {
		names[i] = listName(status.AccountNow{ID: a.ID, Label: a.Label, Primary: a.Primary, Place: i + 1}, digits)
	}
	return nameColumn(names, f.Width)
}

// dotAt is the column of each account's state's dot or square in the list.
func (l detailLayout) dotAt() int {
	return listAt + l.names
}

// lit is how many cells of the list's chosen row are lit, from column 0.
func (l detailLayout) lit() int {
	return l.dotAt() + litPastDot
}

// ruleAt is the column of the list's │.
func (l detailLayout) ruleAt() int {
	return l.dotAt() + rulePastDot
}

// cx is the column BY MODEL, BY DAY and WEEKS start at.
func (l detailLayout) cx() int {
	return l.panel + l.into
}

// step is how far apart the weeks are: a week's days and the gap after.
func (l detailLayout) step() int {
	return 7*l.dw + l.gap
}

// weeksCells are the cells the weeks shown wide take, from the first's first
// day to the last's last.
func (l detailLayout) weeksCells() int {
	return max(l.weeks*l.step()-l.gap, 0)
}

// edge is the column the weeks shown wide end before, as BY MODEL's bar, the
// rule and THIS WEEK do.
func (l detailLayout) edge() int {
	return l.cx() + l.weeksCells()
}

// shownWeeks are where the weeks shown wide lie, from cx, as weeksIn lays
// them.
func (l detailLayout) shownWeeks() []flex.Span {
	return l.weeksIn(l.weeksCells(), l.weeks)
}

// thisWeekAt is the column THIS WEEK starts at, its figures ending at the
// weeks' edge.
func (l detailLayout) thisWeekAt() int {
	return l.edge() - l.thisWeek
}

// weekBar is the cells of a week's peak's bar in WEEKS: a week's step, less
// the gap after it and the peak's cells.
func (l detailLayout) weekBar() int {
	return l.step() - l.gap - 5
}

// detailForm is Detail as it's drawn wide, of doc at now, where its wide
// form's leasts fit the frame; narrower, as a phone's body draws it, under
// the wide chrome, as detailPhoneBody has it.
func (f Frame) detailForm(doc status.Document, now time.Time) accountsForm {
	l, wide := f.detailLaid()
	if !wide {
		return f.detailPhoneBody(doc, now)
	}
	d := f.detailDrawn(doc, now, l)
	return accountsForm{draw: d.draw, scrolling: d.scrolling, scrolls: d.scrolls, targets: d.targets, filled: d.filled}
}

// detailDrawnIn is the layout Detail is drawn in on the frame: a phone's
// stack's, where it's stacked, else its wide layout.
func (f Frame) detailDrawnIn() detailLayout {
	if l, wide := f.detailLaid(); wide {
		return l
	}
	return stackLayout
}

// detailDrawn is Detail of a moment as it's drawn wide: the frame and its
// layout; the account shown, by its id, "" for all accounts, as
// configuredNow has it, and whether the moment has it; its row of the
// period's page, where the page is read and holds one; the sessions running
// on it; and its panel drawn whole, once, for every look to share.
type detailDrawn struct {
	f       Frame
	m       status.Moment
	l       detailLayout
	id      string
	account status.AccountNow
	known   bool
	page    *views.AccountsPage
	row     views.AccountsRow
	running []views.Running
	drawn   func() *canvas
}

// detailDrawn is Detail of doc at now laid out as l: the account the frame's
// Accounts shows, else all accounts, where it shows none the config has; its
// panel drawn once, the first time a look asks.
func (f Frame) detailDrawn(doc status.Document, now time.Time, l detailLayout) detailDrawn {
	d := detailDrawn{f: f, m: f.Moment(doc, now), l: l}
	configured := f.Accounts.Accounts
	if i := slices.IndexFunc(configured, func(a config.Account) bool { return a.ID == f.Accounts.Account() }); i >= 0 {
		d.id = configured[i].ID
		d.account, d.known = configuredNow(d.m, configured[i], i+1)
		d.running = views.RunningOn(f.running(doc, now), d.id)
	}
	d.page, d.row = pageRowOf(f.Accounts.Page, d.id)
	d.drawn = sync.OnceValue(d.panel)
	return d
}

// configuredNow is the configured account a, at the place given among them,
// as the moment m has it, reporting false where m lacks it, as one the
// config has just gained: then as the config names and marks it, nothing
// read of it.
func configuredNow(m status.Moment, a config.Account, place int) (status.AccountNow, bool) {
	now, ok := m.Account(a.ID)
	if !ok {
		now = status.AccountNow{ID: a.ID, Label: a.Label, Primary: a.Primary}
	}
	now.Place = place
	return now, ok
}

// pageRowOf is the page's row of the account with the given id, "" for all
// accounts', and the page: neither where the page isn't read, or holds no
// row of the account.
func pageRowOf(page *views.AccountsPage, id string) (*views.AccountsPage, views.AccountsRow) {
	switch {
	case page == nil:
		return nil, views.AccountsRow{}
	case id == "":
		return page, page.All
	}
	i := slices.IndexFunc(page.Rows, func(r views.AccountsRow) bool { return r.Totals.Account == id })
	if i < 0 {
		return nil, views.AccountsRow{}
	}
	return page, page.Rows[i]
}

// listOut are the forms of the line saying how many of Detail's list's
// accounts are out of view, in the list's cells: "▼ 4 more accounts", and
// for less room, the counts alone, "▲ 2 more · ▼ 2 more".
var listOut = []outForm{{noun: "account", gap: " · ", inks: keyedOut}, {gap: " · ", inks: keyedOut}}

// draw draws Detail from row top: its panel, as far down as it's scrolled,
// and its list beside it, held; and over the key line, where they don't all
// show, how many of the panel's rows are out of view, from its column, as
// Compare's line says it, and under the list, how many of its accounts.
func (d detailDrawn) draw(c *canvas, top int) {
	panel := d.drawn()
	rows, y := d.f.viewRows(), d.f.Height-underView
	paged := pagedWhole(panel, rows)
	from, to := paged.shown(d.f.Scroll)
	c.paste(panel, from, top, to-from)
	c.line(d.l.panel, y, outOfView(from, paged.content-to, d.f.edge()-d.l.panel, compareOut))
	if !d.l.list {
		return
	}
	d.list(c.within(bounds{y: top, width: d.l.ruleAt() + 1, rows: rows}), top, rows)
	first, shown := d.listShown()
	c.line(listAt, y, outOfView(first, len(d.f.Accounts.Accounts)-first-shown, d.l.ruleAt()-listAt, listOut...))
}

// scrolling is how far Detail's panel scrolls, by rows, scrolled offset
// down, as pagedWhole pages it.
func (d detailDrawn) scrolling(offset int) Scrolling {
	return pagedWhole(d.drawn(), d.f.viewRows()).scrolling(offset)
}

// scrolls is what of Detail scrolls: its panel, from its column to the
// frame's edge, the list beside it held.
func (d detailDrawn) scrolls() (x, y, width, rows int) {
	return d.l.panel, d.f.top(), d.f.Width - d.l.panel, d.f.viewRows()
}

// windows are the windows USAGE shows: those the Overview lists, used or
// not, as listedWindows has them.
func (d detailDrawn) windows() []shownWindow {
	return d.f.listedWindows(d.m)
}

// windowNames are the cells of the widest name USAGE gives a window, of
// every window listed, shown or not, so its row doesn't move as the windows
// shown change.
func (d detailDrawn) windowNames() int {
	names := 0
	for _, w := range usageWindows(d.m.Listed(true), d.f.windowNamer(d.m)) {
		names = max(names, ansi.StringWidth(status.Short(w.name)))
	}
	return names
}

// byModelAt is the row of the panel BY MODEL is on: byModelGap rows under
// USAGE's last window, or THIS WEEK's last item, whichever is lower.
func (d detailDrawn) byModelAt() int {
	return max(usageAt+len(d.windows())*windowsApart-1, usageAt+thisWeekRows) + byModelGap
}

// versionLines are BY MODEL's versions with their shares, over the rows
// under its bar, as versionsOver has them across the weeks' width; none
// where the period's page isn't read.
func (d detailDrawn) versionLines() []line {
	if d.page == nil {
		return nil
	}
	return versionsOver(versionsUsed(d.row, d.page.Versions), d.l.weeksCells())
}

// underModels is the row of the panel the parts under BY MODEL count their
// rows from: BY MODEL's, moved down a row for each row its versions take
// past the first.
func (d detailDrawn) underModels() int {
	return d.byModelAt() + max(len(d.versionLines())-1, 0)
}

// latelyAt is the row of the panel LATELY's heading is on, or SESSIONS'
// where sessions run on the account, LATELY beside it.
func (d detailDrawn) latelyAt() int {
	return d.underModels() + latelyUnder
}

// panel is Detail's panel drawn whole, from row 0 of a canvas the frame's
// width: its head; USAGE, and THIS WEEK beside it; BY MODEL, the rule, BY
// DAY, WEEKS and the verdict, where the period's page is read, the rule and
// what's under it moved down a row for each row BY MODEL's versions take
// past the first; and LATELY, beside SESSIONS where sessions run on the
// account, its heading on SESSIONS' row and its lines under it, their words
// cut at the frame's edge.
func (d detailDrawn) panel() *canvas {
	c := growing(d.f.Width)
	d.head(c, 0)
	d.usage(c, usageAt)
	if at := d.underModels(); d.page != nil {
		d.thisWeek(c, usageAt)
		d.byModel(c, d.byModelAt())
		d.rule(c, at+ruleUnder)
		c.heading(at+ruleUnder, at+byDayUnder)
		weeks := d.l.shownWeeks()
		d.byDay(c, at+byDayUnder, weeks)
		d.weeks(c, at+weeksUnder, weeks)
		d.verdict(c, at+verdictUnder)
	}
	y, x := d.latelyAt(), d.l.cx()
	if len(d.running) == 0 {
		c.line(d.l.panel, y, line{{"LATELY", dimInk}})
		d.lately(c, y, x, d.f.edge()-x)
		return c
	}
	d.sessions(c, y)
	x = d.l.panel + latelyBeside
	c.line(x, y, line{{"LATELY", dimInk}})
	d.lately(c, y+1, x, d.f.edge()-x)
	return c
}

// head draws the panel's head along row y: the account's number, name and
// marks, or 0 all accounts, then headApart cells on, how it stands, as
// stands has it.
func (d detailDrawn) head(c *canvas, y int) {
	x := d.l.panel
	if d.id == "" {
		x = c.line(x, y, allAccountsName(1)) + headApart
	} else {
		x = c.line(x, y, listName(d.account, 1).fitted(d.f.edge()-x)) + headApart
	}
	d.stands(c, x, y, false)
}

// stands draws how the account stands along row y from x, short where short
// is: as its Headed says, in ROUTING's glyph and colours, picked out while
// its state in a word is newly changed, as cueState has it, or not read, as
// stateWord says it, where the moment lacks it; or every configured
// account's state counted, as CountStatesOf counts them.
func (d detailDrawn) stands(c *canvas, x, y int, short bool) {
	switch {
	case d.id == "":
		c.line(x, y, countedStates(d.m.CountStatesOf(d.f.Accounts.Accounts.IDs()), NumberAccountStates, d.f.edge()-x))
	case !d.known:
		c.line(x, y, stateWord(status.StateNotRead).fit(d.f.edge()-x))
	default:
		end := c.line(x, y, standingWords(d.account.Headed(), short, d.f.edge()-x))
		d.f.cueState(c, d.id, x, y, end-x)
	}
}

// usage draws USAGE from row y, its heading, then every window under it,
// windowsApart rows apart, each named in text.subtle, then as usage draws a
// window, laid out as detailUsageLaid lays its row from the panel's left to
// thisWeekApart cells short of THIS WEEK: its names as wide as windowNames
// has them, and its words the cells of their form at their widest, ending
// there.
func (d detailDrawn) usage(c *canvas, y int) {
	c.line(d.l.panel, y, line{{"USAGE", dimInk}})
	c.heading(y, y+1)
	b := usageBlock{width: d.f.Width, look: d.f.Look}
	end := d.l.thisWeekAt() - thisWeekApart
	l, _ := detailUsageLaid(end-d.l.panel, wideBarLeast, d.windowNames(), b.widestWords())
	for i, w := range d.windows() {
		row := y + 1 + i*windowsApart
		c.line(d.l.panel, row, line{{prose.Cut(status.Short(w.name), l.names), dimInk}})
		b.gauge(c, d.gauge(w.key), d.l.panel+l.at, row, l.bar, end)
	}
}

// gauge is the window with the given key as USAGE draws it, eased and cued
// as the Overview's: the account's, as usage draws it, empty where the
// moment lacks it; or all accounts', as allGauge has it.
func (d detailDrawn) gauge(key string) gauge {
	switch {
	case d.id == "":
		return d.f.overviewGauge(allGauge(d.m, key), false)
	case !d.known:
		return gauge{of: Ref{Account: d.id, Window: key}, bar: usageBar{empty: true}}
	}
	return d.f.overviewGauge(accountGauge(d.account, key, d.m.Now), false)
}

// allGauge is all accounts' window with the given key, as Accounts draws it,
// of the moment m: every account's window together, as Moment.All has it,
// its bar with no cap, faded where every account it counts is stale, and its
// words its room, to each account's limit, of how many accounts it counts;
// empty where no account's use of it is known.
func allGauge(m status.Moment, key string) gauge {
	g := gauge{of: Ref{Window: key}, all: true, bar: usageBar{empty: true}}
	all := m.All().Windows
	if i := slices.IndexFunc(all, func(w status.AllWindowNow) bool { return w.Key == key }); i >= 0 {
		w := all[i]
		g.bar = usageBar{used: w.Used, capAt: 1, pace: w.Pace, stale: w.Stale}
		g.words = poolWords(w.Room, w.Of, false)
	}
	return g
}

// thisWeek draws THIS WEEK from row y, at its column: its title, naming the
// period, then each item, its label thisWeekLabels cells wide in
// text.subtle, and its figure cut at the weeks' edge, as fitParts cuts it,
// in the cells thisWeekCells gives them.
func (d detailDrawn) thisWeek(c *canvas, y int) {
	x := d.l.thisWeekAt()
	c.line(x, y, line{{d.page.Period.Title(), dimInk}})
	cells := d.l.edge() - x - thisWeekLabels
	for i, item := range d.thisWeekItems(cells) {
		c.line(x, y+1+i, line{{padded(item.label, thisWeekLabels), dimInk}})
		c.line(x+thisWeekLabels, y+1+i, fitParts(cells, periodNumbered(item.parts, d.id, item.label)...))
	}
}

// thisWeekItem is an item of THIS WEEK: its label, and its figure.
type thisWeekItem struct {
	label string
	parts []part
}

// thisWeekItems are THIS WEEK's items, as the words of Accounts say them, in
// text.secondary: the requests; the limits, by window, or all accounts' how
// many; the plan, by its name, or all accounts' summed; the worth, with its
// ratio, in the cells given; and the extra usage of the accounts shown. Each
// of the period's figures is as periodParts says it, a dash where the period
// records nothing; the plan and extra use are none of them.
func (d detailDrawn) thisWeekItems(cells int) []thisWeekItem {
	totals, page := d.row.Totals, *d.page
	return []thisWeekItem{
		{"requests", periodParts(page, []part{figureIn(prose.Grouped(totals.Requests), secondaryInk)}, secondaryInk)},
		{"limits", periodParts(page, limitsParts(totals, d.f.windowNamer(d.m), secondaryInk), secondaryInk)},
		{"plan", planNamed(d.row, secondaryInk)},
		{"worth", periodParts(page, worthAgainst(d.row, secondaryInk, cells), secondaryInk)},
		{"extra use", extraShort(d.extra())},
	}
}

// extra is the extra usage of what Detail shows, as ExtraOf takes it: the
// account's, or every configured account's together.
func (d detailDrawn) extra() status.AllExtra {
	if d.id == "" {
		return d.m.ExtraOf(d.f.Accounts.Accounts.IDs())
	}
	return d.m.ExtraOf([]string{d.id})
}

// byModel draws BY MODEL along row y: its heading, the period's share of use
// of each version it used as a stacked bar, from cx to the weeks' edge, a
// part a version in its shade, as Compare's; and from the row under it, the
// versions with their shares, as versionLines has them.
func (d detailDrawn) byModel(c *canvas, y int) {
	c.line(d.l.panel, y, line{{"BY MODEL", dimInk}})
	versions := d.page.Versions
	stackedBar(c, d.f.Look, d.l.cx(), y, splitCells(shareAmounts(d.row, versions), d.l.weeksCells()), versionHues(versions))
	for i, l := range d.versionLines() {
		c.line(d.l.cx(), y+1+i, l)
	}
}

// rule draws the rule heading the weeks shown along row y, from the panel's
// column to the weeks' edge: ┄┄ in border, the weeks counted, "the last four
// weeks", in text.faint, and ┄ in border to the edge.
func (d detailDrawn) rule(c *canvas, y int) {
	head := line{{"┄┄ ", borderInk}, {"the last " + prose.Number(d.l.weeks) + " weeks", faintInk}, {" ", borderInk}}
	rest := d.l.edge() - d.l.panel - head.width()
	c.line(d.l.panel, y, append(head, span{strings.Repeat("┄", max(rest, 0)), borderInk}))
}

// latelyLines are how many lines LATELY tells.
const latelyLines = 3

// lately draws LATELY's lines from x along row y on, as accountLatelyLines
// has them, their words cut to the cells given, each cut short of the
// frame's edge and picked out while it's fresh; or why it has none.
func (d detailDrawn) lately(c *canvas, y, x, words int) {
	sentences := views.AccountLately(d.m, d.id, latelyLines)
	if len(sentences) == 0 {
		d.f.quietly(c, d.m, x, y, d.f.edge())
		return
	}
	for i, l := range accountLatelyLines(d.m, sentences, words) {
		end := c.line(x, y+i, l.fit(d.f.edge()-x))
		d.f.freshen(c, sentences[i].ID, x, y+i, end)
	}
}

// accountLatelyLines are Detail's LATELY lines of the sentences given, at the
// moment m: each led as latelyLeads has it; then its words in full, on a
// phone as wide, in text.muted, each account named as the dashboard names
// it, cut to the cells given.
func accountLatelyLines(m status.Moment, sentences []views.Sentence, words int) []line {
	lines := latelyLeads(m, sentences)
	for i, s := range sentences {
		var told line
		for _, p := range slices.Concat(s.Full.Subject, s.Full.Words, s.Full.Object, s.Full.Rest) {
			text := p.Text
			if p.Kind == status.PartAccount {
				text = named(m, p.Text)
			}
			told = append(told, span{text, mutedInk})
		}
		lines[i] = append(lines[i], told.fit(words)...)
	}
	return lines
}

// sessions draws SESSIONS from row y: its heading, as panelHeading has it
// while SESSIONS holds the focus or not, and now, in text.faint; from the
// row under it, the sessions running on the account, as many as sessionRows
// show, scrolled with the one selected, as sessionsScrolled has them, the
// last row saying how many more there are where more are below; and
// enterUnder rows under the heading, two cells in, what enter opens.
func (d detailDrawn) sessions(c *canvas, y int) {
	x := d.l.panel
	c.line(x, y, line{panelHeading("SESSIONS", d.f.Accounts.sessionsFocused()), {"  now", faintInk}})
	c.heading(y, y+1)
	selected := d.selected()
	first, shown := sessionsScrolled(len(d.running), selected)
	for i, r := range d.running[first : first+shown] {
		d.session(c, r, x, y+1+i, first+i == selected)
	}
	if more := len(d.running) - first - shown; more > 0 {
		c.line(x+enterIn, y+1+shown, outOfView(0, more, sessionLit, runningOut))
	}
	opens := d.running[selected].Session
	c.line(x+enterIn, y+enterUnder, line{{"enter", keyInk}, {" opens " + status.ShownID(opens) + "'s page", mutedInk}})
}

// selected is the place among the sessions running of the one selected, as
// the frame's Accounts' Selection has it.
func (d detailDrawn) selected() int {
	ids := make([]string, len(d.running))
	for i, r := range d.running {
		ids[i] = r.Session
	}
	return max(slices.Index(ids, d.f.Accounts.Selection(ids)), 0)
}

// sessionsScrolled are the first of n sessions SESSIONS shows, the one with the
// index selected among them, and how many it shows: every one where they
// fit its rows; else scrolled no further than it takes to show the one
// selected, a row left at the foot for how many more there are while more
// are below.
func sessionsScrolled(n, selected int) (first, shown int) {
	if n <= sessionRows {
		return 0, n
	}
	first = min(max(selected-(sessionRows-2), 0), n-sessionRows)
	if first == n-sessionRows {
		return first, sessionRows
	}
	return first, sessionRows - 1
}

// session draws the session running r along row y from x: ▸, bold
// accent.key, where it's selected; ● in state.positive while it's busy, or
// ○ in text.subtle; its id, bold, text.primary while busy, else text.muted;
// two cells on, its model's family, padded, in text.secondary; what it's
// doing, padded, bold accent.mode while busy, else text.subtle, as asKnown
// inks it as last known; and when it was last seen, right-aligned, in
// text.subtle. The one selected is picked out from a cell before it,
// sessionLit cells, as chosenRow picks it out while SESSIONS holds the
// focus or not.
func (d detailDrawn) session(c *canvas, r views.Running, x, y int, selected bool) {
	now := d.m.Now
	busy := r.Activity.Busy(now)
	dot, id, doing := span{"○", dimInk}, span{status.ShownID(r.Session), ink{token: theme.TextMuted, bold: true}}, dimInk
	if busy {
		dot, id.ink, doing = span{"●", positiveInk}, titleInk, nameInk
	}
	seen := runningSeen(r.Activity.LastSeen, now)
	c.line(x+2, y, line{
		dot, spaces(1), id, spaces(2),
		{padded(prose.Cut(r.Family, 8), 8), secondaryInk},
		{padded(r.Activity.Short(), 9), asKnown(r.Activity, doing)},
		{fmt.Sprintf("%3s", seen), dimInk},
	})
	if selected {
		c.line(x, y, line{{"▸", keyInk}})
		chosenRow(c, x-1, y, sessionLit, d.f.Accounts.sessionsFocused())
	}
}

// panelHeading is the heading of a part of Detail that holds a choice, as
// SESSIONS does: bold text.primary while it holds the focus, else
// text.subtle, as every heading of Accounts is.
func panelHeading(name string, focused bool) span {
	if focused {
		return span{name, titleInk}
	}
	return span{name, dimInk}
}

// chosenRow picks out the chosen row of a part of Detail that holds a
// choice, its list or SESSIONS, across width cells from x along row y: on
// bg.selection while its part holds the focus; else bold, its colours kept,
// so it still reads as chosen while the other's row is lit.
func chosenRow(c *canvas, x, y, width int, focused bool) {
	if focused {
		c.surface(x, y, width, hue{token: theme.BgSelection})
		return
	}
	c.emboldened(x, y, width)
}

// list draws Detail's list from row top, rows tall, as listShown scrolls it:
// 0 all accounts, then each configured account shown, listApart rows apart,
// as configuredNow has it, its number, name and marks, as listName has them,
// in the name column, and after it its state's glyph, as StateInAWord and
// stateMark have it, cued over the row's light while it's newly changed, as
// cueState has it; the chosen row picked out, as listRow has it; ↑↓ choose
// chooseUnder rows under the last; and the │ beside them, to the panel's
// foot.
func (d detailDrawn) list(c *canvas, top, rows int) {
	configured := d.f.Accounts.Accounts
	digits := len(strconv.Itoa(len(configured)))
	d.listRow(c, top, allAccountsName(digits), d.id == "")
	first, shown := d.listShown()
	y := top
	for i, a := range configured[first : first+shown] {
		y += listApart
		now, _ := configuredNow(d.m, a, first+i+1)
		c.line(d.l.dotAt(), y, line{stateMark(d.m.StateInAWord(a.ID))})
		d.listRow(c, y, listName(now, digits).fitted(d.l.names), a.ID == d.id)
		d.f.cueState(c, a.ID, d.l.dotAt(), y, 1)
	}
	c.line(listAt, y+chooseUnder, line{{"↑↓", keyInk}, {" choose", dimInk}})
	for row := range min(d.ruleRows(), rows) {
		c.line(d.l.ruleAt(), top+row, line{{"│", borderInk}})
	}
}

// listRow draws a row of the list along row y, named as given, picked out
// where it's the one chosen, over what's drawn of it already, as chosenRow
// picks it out while the list holds the focus or not; and while it doesn't,
// ▸ in bold accent.key before it, in the cell before its number, as SESSIONS
// marks its chosen row, as bold alone barely shows on a bold name.
func (d detailDrawn) listRow(c *canvas, y int, name line, chosen bool) {
	c.line(listAt, y, name)
	if !chosen {
		return
	}
	focused := !d.f.Accounts.sessionsFocused()
	if !focused {
		c.line(listAt-1, y, line{{"▸", keyInk}})
	}
	chosenRow(c, 0, y, d.l.lit(), focused)
}

// ruleRows are the rows the list's │ runs down: to the row over SESSIONS'
// enter line, or with sessions, to it.
func (d detailDrawn) ruleRows() int {
	rows := d.latelyAt() + enterUnder
	if len(d.running) > 0 {
		return rows + 1
	}
	return rows
}

// listShown are the first of the configured accounts the list shows, by
// their place among them from 0, scrolled to show the one shown, as
// DetailListScroll has it, and how many it shows in the view's rows: every
// one where they fit.
func (d detailDrawn) listShown() (first, shown int) {
	return d.f.DetailListScroll(d.id), listScrolling(len(d.f.Accounts.Accounts), d.f.viewRows()).Page
}

// listScrolling is how far Detail's list of n accounts scrolls in rows rows,
// by accounts: as many show as fit under all accounts' row with ↑↓ choose
// under the last, at least one.
func listScrolling(n, rows int) Scrolling {
	page := min(max((rows-1-chooseUnder)/listApart, 1), n)
	return Scrolling{Most: n - page, Page: page}
}

// DetailListScroll is how far Detail's list is scrolled, by accounts, to show
// the account with the id given: as far as the frame's ListScroll has it,
// where that shows it, else no further than it must go to. All accounts, "",
// always shows, held at the list's top, as does an account the config
// doesn't hold, so neither moves it.
func (f Frame) DetailListScroll(id string) int {
	configured := f.Accounts.Accounts
	i := slices.IndexFunc(configured, func(a config.Account) bool { return a.ID == id })
	return listScrolling(len(configured), f.viewRows()).revealing(f.Accounts.ListScroll, i)
}

// listName is account a's name as Detail's list and head name it: its
// number, right-aligned in the digits given, its label and ◆ on the primary,
// unrouted, as Accounts shows no routing.
func listName(a status.AccountNow, digits int) accountName {
	return accountName{number: fmt.Sprintf("%*d", digits, a.Place), label: name(a), primary: a.Primary, unrouted: true}
}

// allAccountsName is all accounts as Detail's list and head name it: 0,
// right-aligned in the digits given, in text.subtle, and all accounts, bold
// text.secondary.
func allAccountsName(digits int) line {
	return line{{fmt.Sprintf("%*d", digits, 0), dimInk}, {" all accounts", strongInk}}
}

// targets are the zones of Detail a click acts on: the list's, and
// SESSIONS'.
func (d detailDrawn) targets() []Target {
	return append(d.listTargets(), d.sessionTargets()...)
}

// listTargets are the list's rows shown, where it shows, each across the
// cells it's lit across, which chooses its account, or all accounts, "".
func (d detailDrawn) listTargets() []Target {
	if !d.l.list {
		return nil
	}
	top := d.f.top()
	targets := []Target{{X: 0, Y: top, Width: d.l.lit(), On: OnAccount}}
	first, shown := d.listShown()
	for i, a := range d.f.Accounts.Accounts[first : first+shown] {
		targets = append(targets, Target{X: 0, Y: top + (i+1)*listApart, Width: d.l.lit(), On: OnAccount, Account: a.ID})
	}
	return targets
}

// sessionTargets are the sessions SESSIONS shows, where they show in the
// view as the panel's scrolled, each across the cells it's lit across where
// it's selected, which selects it, and opens the one selected.
func (d detailDrawn) sessionTargets() []Target {
	if len(d.running) == 0 {
		return nil
	}
	top := d.f.top()
	from, to := pagedWhole(d.drawn(), d.f.viewRows()).shown(d.f.Scroll)
	at := d.latelyAt() + 1 - from
	first, shown := sessionsScrolled(len(d.running), d.selected())
	var targets []Target
	for i, r := range d.running[first : first+shown] {
		if y := at + i; y >= 0 && y < to-from {
			targets = append(targets, Target{X: max(d.l.panel-1, 0), Y: top + y, Width: sessionLit, On: OnSession, Session: r.Session})
		}
	}
	return targets
}

// filled are how much of each bar Detail fills, as Filled has them: every
// account's windows, whichever is chosen, so a bar chosen between reads
// eases from where it stood, but none USAGE draws empty; and all accounts'
// windows, by their keys alone.
func (d detailDrawn) filled() map[Ref]float64 {
	filled := windowsFilled(d.m, func(w status.WindowNow) bool { return !drawsEmpty(w.Kind) })
	for _, w := range d.m.All().Windows {
		filled[Ref{Window: w.Key}] = w.Used
	}
	return filled
}

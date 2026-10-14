package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Detail's list's columns: its rows from listAt; the chosen row lit across
// listLit cells from column 0; each account's state's dot or square at
// listDotAt; its │ at listRuleAt; and the panel beside it from listPanelAt.
const (
	listAt      = 1
	listLit     = 17
	listDotAt   = 15
	listRuleAt  = 20
	listPanelAt = 24
)

// Detail's list's rows: an account's listApart rows from the one before,
// and ↑↓ choose chooseUnder rows under the last shown.
const (
	listApart   = 2
	chooseUnder = 3
)

// Detail's panel's columns, in cells into it: where BY MODEL, BY DAY and
// WEEKS start, cx; where USAGE's bars start; and where LATELY starts beside
// SESSIONS, and the cells its lines' words are cut to there.
const (
	panelCX      = 12
	usageBarAt   = 9
	latelyBeside = 48
	latelyWords  = 52
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

// THIS WEEK's labels' cells, and how far after its name the head says how
// the account stands.
const (
	thisWeekLabels = 10
	headApart      = 3
)

// SESSIONS: as many rows as sessionRows show at once, the one selected
// picked out across sessionLit cells from a cell before its ▸; and its
// enter line, enterUnder rows under its heading, two cells in.
const (
	sessionRows = 4
	sessionLit  = 43
	enterUnder  = 6
	enterIn     = 2
)

// sixWeeks are the weeks BY DAY and WEEKS show, the last of them this one.
const sixWeeks = 6

// detailLayout is Detail laid out wide: with its list at its left, the panel
// beside it, or with one account, the panel alone, across the frame; and its
// panel's days' cells, the cells between its weeks, and USAGE's bars' cells.
type detailLayout struct {
	list         bool
	panel        int
	dw, gap, bar int
}

// detailLayout is Detail's wide layout, as its accounts have it: the list and
// the panel beside it, its days 2 cells and its weeks 4 apart, its bars 40
// wide; or with one account, the panel alone from column 0, the stack grown
// into the width, its days 3 cells and its weeks 1 apart, its bars 50 wide.
func (f Frame) detailLayout() detailLayout {
	if f.Accounts.Alone() {
		return detailLayout{dw: 3, gap: 1, bar: 50}
	}
	return detailLayout{list: true, panel: listPanelAt, dw: 2, gap: 4, bar: 40}
}

// cx is the column BY MODEL, BY DAY and WEEKS start at.
func (l detailLayout) cx() int {
	return l.panel + panelCX
}

// step is how far apart the six weeks are: a week's days and the gap after.
func (l detailLayout) step() int {
	return 7*l.dw + l.gap
}

// edge is the column the six weeks end before, as BY MODEL's bar and the
// rule do: the panel's widest fixed rows.
func (l detailLayout) edge() int {
	return l.cx() + sixWeeks*l.step() - l.gap
}

// thisWeekAt is the column THIS WEEK starts at, over the fifth of the six
// weeks.
func (l detailLayout) thisWeekAt() int {
	return l.cx() + 4*l.step()
}

// weekBar is the cells of a week's peak's bar in WEEKS: a week's step, less
// the gap after it and the peak's cells.
func (l detailLayout) weekBar() int {
	return l.step() - l.gap - 5
}

// reach is the width Detail's wide form takes: where its widest fixed rows
// end, and the margin after them.
func (l detailLayout) reach() int {
	return l.edge() + margin
}

// detailForm is Detail as it's drawn wide, of doc at now, where the frame is
// wide enough for its widest fixed rows; narrower, as a phone's body draws
// it, under the wide chrome, as detailPhoneBody has it.
func (f Frame) detailForm(doc status.Document, now time.Time) accountsForm {
	l := f.detailLayout()
	if l.reach() > f.Width {
		return f.detailPhoneBody(doc, now)
	}
	d := f.detailDrawn(doc, now, l)
	return accountsForm{draw: d.draw, scrolling: d.scrolling, scrolls: d.scrolls, targets: d.targets, filled: d.filled}
}

// detailDrawn is Detail of a moment as it's drawn wide: the frame and its
// layout; the account shown, by its id, "" for all accounts, as
// configuredNow has it, and whether the moment has it; its row of the
// period's page, where the page is read and holds one; and the sessions
// running on it.
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
}

// detailDrawn is Detail of doc at now laid out as l: the account the frame's
// Accounts shows, else all accounts, where it shows none the config has.
func (f Frame) detailDrawn(doc status.Document, now time.Time, l detailLayout) detailDrawn {
	d := detailDrawn{f: f, m: f.Moment(doc, now), l: l}
	configured := f.Accounts.Accounts
	if i := slices.IndexFunc(configured, func(a config.Account) bool { return a.ID == f.Accounts.Account() }); i >= 0 {
		d.id = configured[i].ID
		d.account, d.known = configuredNow(d.m, configured[i], i+1)
		d.running = views.RunningOn(f.running(doc, now), d.id)
	}
	d.page, d.row = pageRowOf(f.Accounts.Page, d.id)
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

// draw draws Detail from row top: its panel, as far down as it's scrolled,
// and its list beside it, held.
func (d detailDrawn) draw(c *canvas, top int) {
	panel := d.panel()
	rows := d.f.viewRows()
	c.paste(panel, byRows(panel.height(), rows).clamped(d.f.Scroll), top, rows)
	if d.l.list {
		d.list(c.within(bounds{y: top, width: listRuleAt + 1, rows: rows}), top, rows)
	}
}

// scrolling is how far Detail's panel scrolls, by rows, at any offset: as far
// as it runs past the view's rows.
func (d detailDrawn) scrolling(int) Scrolling {
	return byRows(d.panel().height(), d.f.viewRows())
}

// scrolls is what of Detail scrolls: its panel, from its column to the
// frame's edge, the list beside it held.
func (d detailDrawn) scrolls() (x, y, width, rows int) {
	return d.l.panel, d.f.top(), d.f.Width - d.l.panel, d.f.viewRows()
}

// windows are the windows USAGE shows: every one, used or not, as usage
// shows them, each named as the Overview names it.
func (d detailDrawn) windows() []shownWindow {
	return usageWindows(d.m, nil, d.f.windowNamer(d.m))
}

// byModelAt is the row of the panel BY MODEL is on: byModelGap rows under
// USAGE's last window, or THIS WEEK's last item, whichever is lower.
func (d detailDrawn) byModelAt() int {
	return max(usageAt+len(d.windows())*windowsApart-1, usageAt+thisWeekRows) + byModelGap
}

// latelyAt is the row of the panel LATELY's heading is on, or SESSIONS'
// where sessions run on the account, LATELY beside it.
func (d detailDrawn) latelyAt() int {
	return d.byModelAt() + latelyUnder
}

// panel is Detail's panel drawn whole, from row 0 of a canvas the frame's
// width: its head; USAGE, and THIS WEEK beside it; BY MODEL, the rule, BY
// DAY, WEEKS and the verdict, where the period's page is read; and LATELY,
// beside SESSIONS where sessions run on the account, its heading on
// SESSIONS' row and its lines under it, their words cut to latelyWords.
func (d detailDrawn) panel() *canvas {
	c := growing(d.f.Width)
	d.head(c, 0)
	d.usage(c, usageAt)
	if at := d.byModelAt(); d.page != nil {
		d.thisWeek(c, usageAt)
		d.byModel(c, at)
		d.rule(c, at+ruleUnder)
		d.byDay(c, at+byDayUnder)
		d.weeks(c, at+weeksUnder)
		d.verdict(c, at+verdictUnder)
	}
	y := d.latelyAt()
	if len(d.running) == 0 {
		c.line(d.l.panel, y, line{{"LATELY", dimInk}})
		d.lately(c, y, d.l.cx(), d.f.edge()-d.l.cx())
		return c
	}
	d.sessions(c, y)
	x := d.l.panel + latelyBeside
	c.line(x, y, line{{"LATELY", dimInk}})
	d.lately(c, y+1, x, latelyWords)
	return c
}

// head draws the panel's head along row y: the account's number, name and
// marks, then headApart cells on, how it stands, as its Headed says, in
// ROUTING's glyph and colours, picked out while its state in a word is newly
// changed, as cueState has it, or not read, as stateWord says it, where the
// moment lacks it; or 0 all accounts and every configured account's state
// counted, as CountStatesOf counts them.
func (d detailDrawn) head(c *canvas, y int) {
	x := d.l.panel
	if d.id == "" {
		x = c.line(x, y, allAccountsName(1)) + headApart
		c.line(x, y, countedStates(d.m.CountStatesOf(d.f.Accounts.Accounts.IDs()), NumberAccountStates, d.f.edge()-x))
		return
	}
	x = c.line(x, y, listName(d.account, 1).fitted(d.f.edge()-x)) + headApart
	if !d.known {
		c.line(x, y, stateWord(status.StateNotRead).fit(d.f.edge()-x))
		return
	}
	end := c.line(x, y, standingWords(d.account.Headed(), false, d.f.edge()-x))
	d.f.cueState(c, d.id, x, y, end-x)
}

// usage draws USAGE from row y, its heading, then every window under it,
// windowsApart rows apart, each named in text.subtle, then as usage draws a
// window, its bar usageBarAt cells into the panel, its words ending two
// cells short of THIS WEEK.
func (d detailDrawn) usage(c *canvas, y int) {
	c.line(d.l.panel, y, line{{"USAGE", dimInk}})
	b := usageBlock{width: d.f.Width, look: d.f.Look}
	for i, w := range d.windows() {
		row := y + 1 + i*windowsApart
		c.line(d.l.panel, row, line{{prose.Cut(status.Short(w.name), usageBarAt-1), dimInk}})
		b.gauge(c, d.gauge(w.key), d.l.panel+usageBarAt, row, d.l.bar, d.l.thisWeekAt()-2)
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
// text.subtle, and its figure cut short of the frame's edge.
func (d detailDrawn) thisWeek(c *canvas, y int) {
	x := d.l.thisWeekAt()
	c.line(x, y, line{{d.page.Period.Title(), dimInk}})
	for i, item := range d.thisWeekItems() {
		c.line(x, y+1+i, line{{padded(item.label, thisWeekLabels), dimInk}})
		c.line(x+thisWeekLabels, y+1+i, fitParts(d.f.edge()-x-thisWeekLabels, item.parts...))
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
// ratio; and the extra usage of the accounts shown. The period's figures are
// as periodParts says them, a dash where it records nothing.
func (d detailDrawn) thisWeekItems() []thisWeekItem {
	totals, page := d.row.Totals, *d.page
	return []thisWeekItem{
		{"requests", periodParts(page, []part{figureIn(prose.Grouped(totals.Requests), secondaryInk)}, secondaryInk)},
		{"limits", periodParts(page, limitsParts(totals, d.f.windowNamer(d.m), secondaryInk), secondaryInk)},
		{"plan", planNamed(d.row, secondaryInk)},
		{"worth", periodParts(page, worthAgainst(d.row, secondaryInk), secondaryInk)},
		{"extra use", extraShort(d.shown())},
	}
}

// shown are the accounts Detail shows: the account, or every account.
func (d detailDrawn) shown() []status.AccountNow {
	if d.id == "" {
		return d.m.Accounts
	}
	return []status.AccountNow{d.account}
}

// byModel draws BY MODEL along row y: its heading, the period's share of use
// of each version it used as a stacked bar, from cx to the six weeks' edge,
// a part a version in its shade, as Compare's; and on the row under it, the
// versions with their shares, as versionShares names them in its cells.
func (d detailDrawn) byModel(c *canvas, y int) {
	c.line(d.l.panel, y, line{{"BY MODEL", dimInk}})
	cells, versions := d.l.edge()-d.l.cx(), d.page.Versions
	stackedBar(c, d.f.Look, d.l.cx(), y, splitCells(shareAmounts(d.row, versions), cells), versionHues(versions))
	c.line(d.l.cx(), y+1, versionShares(d.row, versions, cells))
}

// rule draws the rule heading the last six weeks along row y, from the
// panel's column to the six weeks' edge: ┄┄ in border, the last six weeks
// in text.faint, and ┄ in border to the edge.
func (d detailDrawn) rule(c *canvas, y int) {
	head := line{{"┄┄ ", borderInk}, {"the last six weeks", faintInk}, {" ", borderInk}}
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
// moment m: each its time, in a column as wide as the widest, in
// text.subtle; its mark, as LATELY marks it; and its words, in text.muted,
// each account named as the dashboard names it, cut to the cells given.
func accountLatelyLines(m status.Moment, sentences []views.Sentence, words int) []line {
	column := 0
	for _, s := range sentences {
		column = max(column, len(prose.Dated(m.Now, s.At)))
	}
	lines := make([]line, len(sentences))
	for i, s := range sentences {
		var said line
		for _, p := range slices.Concat(s.Full.Subject, s.Full.Words, s.Full.Object, s.Full.Rest) {
			text := p.Text
			if p.Kind == status.PartAccount {
				text = named(m, p.Text)
			}
			said = append(said, span{text, mutedInk})
		}
		lead := line{{padded(prose.Dated(m.Now, s.At), column) + afterTime, dimInk}, eventMark(s), spaces(1)}
		lines[i] = append(lead, said.fit(words)...)
	}
	return lines
}

// sessions draws SESSIONS from row y: its heading, and now, in text.faint;
// from the row under it, the sessions running on the account, as many as
// sessionRows show, scrolled with the one selected, as sessionsScrolled has
// them, the last row saying how many more there are where more are below;
// and enterUnder rows under the heading, two cells in, what enter opens.
func (d detailDrawn) sessions(c *canvas, y int) {
	x := d.l.panel
	c.line(x, y, line{{"SESSIONS", dimInk}, {"  now", faintInk}})
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
// doing, padded, bold accent.mode while busy, else text.subtle; and when it
// was last seen, right-aligned, in text.subtle. The one selected is picked
// out from a cell before it, sessionLit cells.
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
		{padded(r.Activity.Short(), 9), doing},
		{fmt.Sprintf("%3s", seen), dimInk},
	})
	if selected {
		c.line(x, y, line{{"▸", keyInk}})
		c.surface(x-1, y, sessionLit, hue{token: theme.BgSelection})
	}
}

// list draws Detail's list from row top, rows tall, as listShown scrolls it:
// 0 all accounts, then each configured account shown, listApart rows apart,
// as configuredNow has it, its number, name and marks, as listName has them,
// and at listDotAt its state's glyph, as StateInAWord and stateMark have it,
// cued over the row's light while it's newly changed, as cueState has it;
// the chosen row lit; ↑↓ choose chooseUnder rows under the last; and the │
// beside them, to the panel's foot.
func (d detailDrawn) list(c *canvas, top, rows int) {
	configured := d.f.Accounts.Accounts
	digits := len(strconv.Itoa(len(configured)))
	d.listRow(c, top, allAccountsName(digits), d.id == "")
	first, shown := d.listShown()
	y := top
	for i, a := range configured[first : first+shown] {
		y += listApart
		now, _ := configuredNow(d.m, a, first+i+1)
		c.line(listDotAt, y, line{stateMark(d.m.StateInAWord(a.ID))})
		d.listRow(c, y, listName(now, digits).fitted(listDotAt-listAt), a.ID == d.id)
		d.f.cueState(c, a.ID, listDotAt, y, 1)
	}
	c.line(listAt, y+chooseUnder, line{{"↑↓", keyInk}, {" choose", dimInk}})
	for row := range min(d.ruleRows(), rows) {
		c.line(listRuleAt, top+row, line{{"│", borderInk}})
	}
}

// listRow draws a row of the list along row y, named as given, lit where
// it's the one chosen, over what's drawn of it already.
func (d detailDrawn) listRow(c *canvas, y int, name line, chosen bool) {
	c.line(listAt, y, name)
	if chosen {
		c.surface(0, y, listLit, hue{token: theme.BgSelection})
	}
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
// with no ▲, as Accounts shows no routing.
func listName(a status.AccountNow, digits int) accountName {
	return accountName{number: fmt.Sprintf("%*d", digits, a.Place), label: name(a), primary: a.Primary}
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
	targets := []Target{{X: 0, Y: top, Width: listLit, On: OnAccount}}
	first, shown := d.listShown()
	for i, a := range d.f.Accounts.Accounts[first : first+shown] {
		targets = append(targets, Target{X: 0, Y: top + (i+1)*listApart, Width: listLit, On: OnAccount, Account: a.ID})
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
	top, rows := d.f.top(), d.f.viewRows()
	at := d.latelyAt() + 1 - byRows(d.panel().height(), rows).clamped(d.f.Scroll)
	first, shown := sessionsScrolled(len(d.running), d.selected())
	var targets []Target
	for i, r := range d.running[first : first+shown] {
		if y := at + i; y >= 0 && y < rows {
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

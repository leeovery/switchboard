package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Accounts on a phone: Compare turned, a group at a time and an account a
// row, all accounts last; and Detail stacked, each part under its heading,
// as Detail's body is drawn too where the frame is too narrow for its wide
// form. Each is drawn whole, then scrolled a row at a time, as History's
// Year is.

// A phone's Compare's columns: its names from turnedNameAt; WEEK USED's and
// BY MODEL's bars, turnedBar cells, and its groups' first figures, from
// turnedAt; and BY MODEL's largest version at turnedLargestAt.
const (
	turnedNameAt    = 1
	turnedAt        = 13
	turnedBar       = 20
	turnedLargestAt = 35
)

// A phone's Detail's columns: USAGE's bars, stackUsageBar cells, from
// stackUsageAt; THIS WEEK's two columns stackHalf apart, their labels
// stackLabels cells; BY MODEL's bar, stackModelBar cells, BY DAY's last
// stackWeeks weeks, and WEEKS' last stackPeaks peaks, each right-aligned in
// stackPeakCells stackPeakApart apart, from stackAt; and LATELY's words cut
// to stackLatelyWords.
const (
	stackUsageAt     = 9
	stackUsageBar    = 18
	stackHalf        = 26
	stackLabels      = 10
	stackAt          = 10
	stackModelBar    = 40
	stackWeeks       = 3
	stackPeaks       = 4
	stackPeakCells   = 5
	stackPeakApart   = 6
	stackLatelyWords = 44
)

// partsApart is how far a phone's part is from the row before it, a blank
// row between.
const partsApart = 2

// phoneAccounts is Accounts' view shown as a phone draws it, of doc at now:
// Compare turned, or Detail stacked; nothing where no account is configured.
func (f Frame) phoneAccounts(doc status.Document, now time.Time) accountsForm {
	switch {
	case len(f.Accounts.Accounts) == 0:
		return accountsForm{}
	case f.Accounts.View() == AccountsCompare:
		return f.stackForm(func() stack { return f.turned(doc, now).turnedWhole() })
	}
	return f.detailPhoneBody(doc, now)
}

// detailPhoneBody is Detail of doc at now as a phone stacks it: on a phone,
// and as Detail's body where the frame is too narrow for its wide form,
// under the wide chrome.
func (f Frame) detailPhoneBody(doc status.Document, now time.Time) accountsForm {
	return f.stackForm(func() stack { return f.stackedOf(doc, now).whole() })
}

// stack is a phone's view drawn whole: its rows, on a canvas the frame's
// width; the zones of them a click acts on, by their rows on it; and how much
// of each bar it draws it fills, as Filled has them.
type stack struct {
	c       *canvas
	targets []Target
	filled  map[Ref]float64
}

// stackForm is the form of the stack drawn whole as drawn draws it: as many
// of its rows as the view has, from as far down as the frame has it
// scrolled, a row at a time.
func (f Frame) stackForm(drawn func() stack) accountsForm {
	return accountsForm{
		draw:      func(c *canvas, top int) { f.drawStack(c, drawn(), top) },
		scrolling: func(int) Scrolling { return byRows(drawn().c.height(), f.viewRows()) },
		targets:   func() []Target { return f.stackTargets(drawn()) },
		filled:    func() map[Ref]float64 { return drawn().filled },
	}
}

// drawStack draws the stack's rows the view shows from row top, and over the
// key line, where they don't all show, how many are out of view, as
// Compare's line says it.
func (f Frame) drawStack(c *canvas, s stack, top int) {
	content, rows := s.c.height(), f.viewRows()
	offset := byRows(content, rows).clamped(f.Scroll)
	c.paste(s.c, offset, top, rows)
	if content > rows {
		c.line(0, f.Height-underView, outOfView(offset, content-offset-rows, f.edge(), compareOut))
	}
}

// stackTargets are the stack's zones the view shows, at the rows it shows
// them on.
func (f Frame) stackTargets(s stack) []Target {
	top, rows := f.top(), f.viewRows()
	offset := byRows(s.c.height(), rows).clamped(f.Scroll)
	var shown []Target
	for _, t := range s.targets {
		if y := t.Y - offset; y >= 0 && y < rows {
			t.Y = top + y
			shown = append(shown, t)
		}
	}
	return shown
}

// turned is Compare of doc at now as a phone turns it: a column every
// account, none scrolled out of view, then all accounts', as compared reads
// them, each a row.
func (f Frame) turned(doc status.Document, now time.Time) compared {
	m := f.Moment(doc, now)
	return compared{f: f, m: m, now: now, page: f.Accounts.Page, shown: len(f.Accounts.Accounts), week: f.WeekWindow}
}

// turnedWhole is a phone's Compare drawn whole: WEEK USED; then, once the
// period's page is read, BY MODEL, ACTIVITY, LIMITS and VALUE, a blank row
// before each.
func (cp compared) turnedWhole() stack {
	c := growing(cp.f.Width)
	columns := cp.columns()
	y := cp.turnedWeek(c, 0, columns)
	if cp.page != nil {
		y = cp.turnedByModel(c, y+partsApart, columns)
		for _, g := range cp.turnedGroups() {
			y = cp.turnedGroup(c, y+partsApart, g, columns)
		}
	}
	return stack{c: c, filled: cp.filled()}
}

// turnedName is the column's name as a phone's row heads it, from
// turnedNameAt to a cell before turnedAt: an account's label and marks, but
// ▲, as Accounts shows no routing, its label cut short to keep its marks;
// or all, unnumbered, bold text.secondary.
func turnedName(col compareColumn) line {
	if col.all {
		return line{{"all", strongInk}}
	}
	named := accountName{label: name(col.account), primary: col.account.Primary}
	return named.labelIn(turnedAt - turnedNameAt - 1 - named.marks().width())
}

// turnedWeek draws WEEK USED from row y, and returns the row its last column
// is on: its label, and now after it where the period is another than this
// week, as the week is always as it stands; then each column, a blank row
// before it, its week's bar turnedBar cells from turnedAt, as weekGauge has
// it, a cell, and its percentage, right-aligned in 4.
func (cp compared) turnedWeek(c *canvas, y int, columns []compareColumn) int {
	label := line{{"WEEK USED", dimInk}}
	if cp.f.Accounts.Period != views.AccountsThisWeek {
		label = append(label, span{"  now", faintInk})
	}
	c.line(0, y, label)
	for _, col := range columns {
		y += partsApart
		c.line(turnedNameAt, y, turnedName(col))
		g := cp.weekGauge(col)
		g.bar.draw(c, cp.f.Look, turnedAt, y, turnedBar)
		c.barred(turnedAt, y, turnedBar, g.of)
		c.right(turnedAt+turnedBar+1+percentCells, y, g.percentage())
	}
	return y
}

// turnedByModel draws BY MODEL from row y, and returns the row its last
// column is on: its heading; then each column, a blank row before it, its
// share of use as a stacked bar turnedBar cells from turnedAt, as Compare's,
// and at turnedLargestAt the version it used most, as versionsNamed names it.
func (cp compared) turnedByModel(c *canvas, y int, columns []compareColumn) int {
	c.line(0, y, line{{"BY MODEL", dimInk}})
	versions := cp.page.Versions
	hues := versionHues(versions)
	for _, col := range columns {
		y += partsApart
		c.line(turnedNameAt, y, turnedName(col))
		stackedBar(c, cp.f.Look, turnedAt, y, splitCells(shareAmounts(col.row, versions), turnedBar), hues)
		if most, ok := largestVersion(versionsUsed(col.row, versions)); ok {
			c.line(turnedLargestAt, y, fitParts(cp.f.edge()-turnedLargestAt, versionsNamed([]versionUsed{most}, 0)...))
		}
	}
	return y
}

// largestVersion is the version of those used given with the largest share,
// the first of those alike, reporting false where none's share is known.
func largestVersion(used []versionUsed) (versionUsed, bool) {
	var most versionUsed
	for _, u := range used {
		if shareSize(u.share) > shareSize(most.share) {
			most = u
		}
	}
	return most, most.share != nil
}

// turnedGroup is a group of a phone's Compare, as ACTIVITY: its name, and its
// columns.
type turnedGroup struct {
	name    string
	columns []turnedColumn
}

// turnedColumn is a column of a phone's group: from column at, its head, and
// what it says of each of Compare's columns.
type turnedColumn struct {
	at   int
	head string
	said func(col compareColumn) []part
}

// turnedGroups are a phone's Compare's groups under BY MODEL, ACTIVITY,
// LIMITS and VALUE, each column at its place, as Accounts' words say each
// figure in their short forms, a window of hours named by them, as 5h. A
// period with nothing recorded, as on a fresh install, says a dash for each
// of its figures, as period has it.
func (cp compared) turnedGroups() []turnedGroup {
	limits := shortWindows(cp.f.windowNamer(cp.m))
	return []turnedGroup{
		{name: "ACTIVITY", columns: []turnedColumn{
			{turnedAt, "reqs", cp.period(func(col compareColumn) []part {
				return []part{figureIn(prose.Grouped(col.row.Totals.Requests), col.ink())}
			})},
			{23, "sessions", cp.period(func(col compareColumn) []part {
				return []part{figureIn(prose.Grouped(col.row.Totals.Sessions), col.ink())}
			})},
			{35, "moved", cp.period(func(col compareColumn) []part { return movedParts(col.row.Totals, col.ink()) })},
		}},
		{name: "LIMITS", columns: []turnedColumn{
			{turnedAt, "hit", cp.period(func(col compareColumn) []part { return limitsParts(col.row.Totals, limits, col.ink()) })},
			{26, "at cap", cp.period(func(col compareColumn) []part { return atCapParts(col.row.Totals, col.ink()) })},
			{36, "left", func(col compareColumn) []part { return leftShort(col.row, col.ink()) }},
		}},
		{name: "VALUE", columns: []turnedColumn{
			{turnedAt, "plan", func(col compareColumn) []part { return planPriced(col.row, col.ink()) }},
			{26, "worth", cp.period(func(col compareColumn) []part { return worthParts(col.row, col.ink()) })},
			{36, "against it", cp.period(func(col compareColumn) []part { return againstShort(col.row) })},
		}},
	}
}

// turnedGroup draws the group from row y, and returns the row its last
// column is on: its name, and on its row each of its columns' heads, in
// text.subtle; then a row each of Compare's columns, no blank between, each
// figure cut a cell short of the next column, the last's at the frame's
// edge.
func (cp compared) turnedGroup(c *canvas, y int, g turnedGroup, columns []compareColumn) int {
	c.line(0, y, line{{g.name, dimInk}})
	for _, head := range g.columns {
		c.line(head.at, y, line{{head.head, dimInk}})
	}
	for _, col := range columns {
		y++
		c.line(turnedNameAt, y, turnedName(col))
		for i, figure := range g.columns {
			end := cp.f.edge()
			if i+1 < len(g.columns) {
				end = g.columns[i+1].at - 1
			}
			c.line(figure.at, y, fitParts(end-figure.at, figure.said(col)...))
		}
	}
	return y
}

// stacked is a phone's Detail of a moment: Detail as detailDrawn has it,
// laid out as stackLayout, its parts stacked down the frame.
type stacked struct {
	detailDrawn
}

// stackLayout is Detail on a phone: no list; BY MODEL, BY DAY and WEEKS from
// stackAt; and its days swatchCells wide, as History's, its weeks a cell
// apart.
var stackLayout = detailLayout{into: stackAt, dw: swatchCells, gap: 1}

// stackedOf is a phone's Detail of doc at now: of the account the frame's
// Accounts shows, else all accounts, as detailDrawn has it.
func (f Frame) stackedOf(doc status.Document, now time.Time) stacked {
	return stacked{f.detailDrawn(doc, now, stackLayout)}
}

// whole is the phone's Detail drawn whole, each part under its heading, a
// blank row between: the picker, and under it how the account stands, short,
// as stands has it; USAGE; once the period's page is read, THIS WEEK, BY
// MODEL, BY DAY's last stackWeeks weeks, and WEEKS with the verdict under
// it; SESSIONS, as Detail draws it, where sessions run on the account; and
// LATELY, its words cut to stackLatelyWords.
func (s stacked) whole() stack {
	c := growing(s.f.Width)
	s.picker(c, 0)
	s.stands(c, 0, 1, true)
	y := s.usage(c, 1+partsApart)
	if s.page != nil {
		y = s.thisWeek(c, y+partsApart)
		y = s.byModel(c, y+partsApart) + partsApart
		s.byDay(c, y, stackWeeks)
		y = s.weeks(c, y+1+partsApart)
	}
	var targets []Target
	if len(s.running) > 0 {
		y, targets = s.sessions(c, y+partsApart)
	}
	y += partsApart
	c.line(0, y, line{{"LATELY", dimInk}})
	s.lately(c, y+1, 0, stackLatelyWords)
	return stack{c: c, targets: targets, filled: s.filled()}
}

// picker draws the picker along row y: the account shown, or all accounts,
// as named has it, between ◂ and ▸, bold accent.key; and at the right, where
// it fits beside it, ↑↓ an account, or all. With one account, there's no
// other to pick: its name alone.
func (s stacked) picker(c *canvas, y int) {
	if s.f.Accounts.Alone() {
		c.line(0, y, s.named(s.f.edge()))
		return
	}
	shown := slices.Concat(line{{"◂", keyInk}, spaces(1)}, s.named(s.f.edge()-4), line{spaces(1), {"▸", keyInk}})
	c.line(0, y, shown)
	if hint := (line{{"↑↓", keyInk}, {" an account, or all", dimInk}}); shown.width()+1+hint.width() <= s.f.Width {
		c.right(s.f.Width, y, hint)
	}
}

// named is the account shown in the cells given, as Detail's head names it.
func (s stacked) named(cells int) line {
	if s.id == "" {
		return allAccountsName(1).fit(cells)
	}
	return listName(s.account, 1).fitted(cells)
}

// usage draws USAGE from row y, and returns the row its last window is on:
// its heading, then every window, windowsApart rows apart, named in
// text.subtle, then as Detail's gauge has it, its bar stackUsageBar cells
// from stackUsageAt, its words ending at the frame's edge.
func (s stacked) usage(c *canvas, y int) int {
	c.line(0, y, line{{"USAGE", dimInk}})
	b := usageBlock{width: s.f.Width, look: s.f.Look}
	last := y
	for i, w := range s.windows() {
		last = y + 1 + i*windowsApart
		c.line(0, last, line{{prose.Cut(status.Short(w.name), stackUsageAt-1), dimInk}})
		b.gauge(c, s.gauge(w.key), stackUsageAt, last, stackUsageBar, s.f.edge())
	}
	return last
}

// thisWeek draws THIS WEEK from row y, and returns its last row: its title,
// naming the period, then its items in two columns stackHalf apart, a row
// an item, their labels stackLabels cells in text.subtle, each figure cut a
// cell short of the next column, or of the frame's edge.
func (s stacked) thisWeek(c *canvas, y int) int {
	c.line(0, y, line{{s.page.Period.Title(), dimInk}})
	halves := s.thisWeekItems()
	for h, items := range halves {
		x, end := h*stackHalf, s.f.edge()
		if h+1 < len(halves) {
			end = (h+1)*stackHalf - 1
		}
		for i, item := range items {
			c.line(x, y+1+i, line{{padded(item.label, stackLabels), dimInk}})
			c.line(x+stackLabels, y+1+i, fitParts(end-x-stackLabels, item.parts...))
		}
	}
	return y + len(halves[0])
}

// thisWeekItems are THIS WEEK's items in its two columns, as the words of
// Accounts say them, in text.secondary: the requests, the limits, by window,
// or all accounts' how many, and the plan, by its name, or all accounts'
// summed; then the worth, the extra usage of the accounts shown, and the
// worth against the plan's price. The period's figures are as periodParts
// says them.
func (s stacked) thisWeekItems() [][]thisWeekItem {
	totals, page := s.row.Totals, *s.page
	return [][]thisWeekItem{
		{
			{"requests", periodParts(page, []part{figureIn(prose.Grouped(totals.Requests), secondaryInk)}, secondaryInk)},
			{"limits", periodParts(page, limitsParts(totals, s.f.windowNamer(s.m), secondaryInk), secondaryInk)},
			{"plan", planNamed(s.row, secondaryInk)},
		},
		{
			{"worth", periodParts(page, worthParts(s.row, secondaryInk), secondaryInk)},
			{"extra use", extraShort(s.shown())},
			{"against", periodParts(page, againstShort(s.row), secondaryInk)},
		},
	}
}

// byModel draws BY MODEL from row y, and returns its last row: its heading,
// and on its row the period's share of use as a stacked bar stackModelBar
// cells from cx, as Detail's; then the versions it used, each with its
// share, over the rows under it, as versionsOver has them.
func (s stacked) byModel(c *canvas, y int) int {
	c.line(0, y, line{{"BY MODEL", dimInk}})
	versions := s.page.Versions
	stackedBar(c, s.f.Look, s.l.cx(), y, splitCells(shareAmounts(s.row, versions), stackModelBar), versionHues(versions))
	for _, l := range versionsOver(versionsUsed(s.row, versions), s.f.edge()) {
		y++
		c.line(0, y, l)
	}
	return y
}

// versionsOver are the versions used given, in their order, as versionsNamed
// names them, over two rows width cells wide: on the first, as many as fit;
// on the second, the rest, as versionShares has them, the smallest left out
// where they don't all fit. A row with none on it is left out.
func versionsOver(used []versionUsed, width int) []line {
	first := 0
	for first < len(used) && partsWidth(versionsNamed(used[:first+1], 0)) <= width {
		first++
	}
	rows := []line{fitParts(width, versionsNamed(used[:first], 0)...), versionShares(used[first:], width)}
	return slices.DeleteFunc(rows, func(l line) bool { return len(l) == 0 })
}

// weeks draws WEEKS along row y, and returns the verdict's last row: its
// heading; from cx, the last stackPeaks weeks' peaks, as peakPart has them,
// this week's bold text.secondary, each right-aligned in stackPeakCells,
// stackPeakApart apart; and under it the verdict, as verdictShort has it, a
// row each of its parts.
func (s stacked) weeks(c *canvas, y int) int {
	c.line(0, y, line{{"WEEKS", dimInk}})
	peaks := s.row.Peaks[max(len(s.row.Peaks)-stackPeaks, 0):]
	for i, p := range peaks {
		c.rightFit(s.l.cx()+i*stackPeakApart+stackPeakCells, s.f.edge(), y, peakPart(p, strongInk, Number{}))
	}
	for _, parts := range verdictShort(s.row) {
		y++
		c.line(0, y, fitParts(s.f.edge(), parts...))
	}
	return y
}

// sessions draws SESSIONS from row y as Detail draws it, and returns the row
// its enter line is on and the zones of its rows a click acts on: each
// session shown, across the cells it's lit across where it's selected, which
// selects it, and opens the one selected.
func (s stacked) sessions(c *canvas, y int) (int, []Target) {
	s.detailDrawn.sessions(c, y)
	first, shown := sessionsScrolled(len(s.running), s.selected())
	targets := make([]Target, shown)
	for i, r := range s.running[first : first+shown] {
		targets[i] = Target{Y: y + 1 + i, Width: sessionLit, On: OnSession, Session: r.Session}
	}
	return y + enterUnder, targets
}

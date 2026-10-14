package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
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
// stackLabels cells; BY MODEL's bar, stackModelBar cells, BY DAY's weeks,
// stackWeeks of them stackWeekApart apart, and WEEKS' last stackPeaks peaks,
// each right-aligned in stackPeakCells stackPeakApart apart, from stackAt;
// and LATELY's stackLatelyLines lines, their words cut to stackLatelyWords.
const (
	stackUsageAt     = 9
	stackUsageBar    = 18
	stackHalf        = 26
	stackLabels      = 10
	stackAt          = 10
	stackModelBar    = 40
	stackWeeks       = 3
	stackWeekApart   = 7*swatchCells + 1
	stackPeaks       = 4
	stackPeakCells   = 5
	stackPeakApart   = 6
	stackLatelyLines = 3
	stackLatelyWords = 44
)

// SESSIONS on a phone: as many rows as stackSessionRows show at once, each
// from stackSessionAt, ▸ before the one selected, which is picked out across
// stackSessionLit cells from column 0.
const (
	stackSessionRows = 4
	stackSessionAt   = 2
	stackSessionLit  = 43
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
	return compared{f: f, m: m, now: now, page: f.Accounts.Page, shown: len(f.Accounts.Accounts), week: sharedWeekKey(m.Policy())}
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
// share of use as a stacked bar turnedBar cells from turnedAt, as sharesBar
// draws it, and at turnedLargestAt the version it used most, its ● in its
// shade, its name and its share.
func (cp compared) turnedByModel(c *canvas, y int, columns []compareColumn) int {
	c.line(0, y, line{{"BY MODEL", dimInk}})
	for _, col := range columns {
		y += partsApart
		c.line(turnedNameAt, y, turnedName(col))
		sharesBar(c, cp.f.Look, turnedAt, y, turnedBar, cp.page.Versions, col.row)
		if most, ok := largestVersion(cp.page.Versions, col.row); ok {
			c.line(turnedLargestAt, y, versionNamed(most, col.row).fit(cp.f.edge()-turnedLargestAt))
		}
	}
	return y
}

// largestVersion is the version of those given the row used most, by its
// share of use, the first of those alike, reporting false where it used
// none priced.
func largestVersion(versions []ledger.Version, row views.AccountsRow) (ledger.Version, bool) {
	var most ledger.Version
	largest := 0.0
	for _, v := range versions {
		if s, ok := shareOf(row, v); ok && s.Share != nil && *s.Share > largest {
			most, largest = v, *s.Share
		}
	}
	return most, largest > 0
}

// versionNamed names a version the row used, as BY MODEL does with its
// share: its ● in its shade, then its name and its share, as shareParts says
// it, in text.secondary, as "● Opus 5.5 55%".
func versionNamed(v ledger.Version, row views.AccountsRow) line {
	var share *float64
	if s, ok := shareOf(row, v); ok {
		share = s.Share
	}
	named := line{{"●", versionHue(v).ink()}, {" " + prose.Clean(v.Name) + " ", secondaryInk}}
	for _, p := range shareParts(share, secondaryInk) {
		named = append(named, p.drawn()...)
	}
	return named
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

// stacked is a phone's Detail of a moment: the account shown, by its id, ""
// for all accounts, and as the moment has it; its row of the period's page,
// where the page is read and holds one; and the sessions running on it.
type stacked struct {
	f       Frame
	m       status.Moment
	id      string
	account status.AccountNow
	page    *views.AccountsPage
	row     views.AccountsRow
	running []views.Running
}

// stackedOf is a phone's Detail of doc at now: of the account the frame's
// Accounts shows, else all accounts, where the moment lacks it.
func (f Frame) stackedOf(doc status.Document, now time.Time) stacked {
	s := stacked{f: f, m: f.Moment(doc, now)}
	if a, ok := s.m.Account(f.Accounts.Account()); ok {
		s.id, s.account = a.ID, a
		s.running = views.RunningOn(f.running(doc, now), a.ID)
	}
	s.page, s.row = rowOfPage(f.Accounts.Page, s.id)
	return s
}

// rowOfPage is the page's row of the account with the given id, "" for all
// accounts', and the page: neither where the page isn't read, or holds no
// row of the account.
func rowOfPage(page *views.AccountsPage, id string) (*views.AccountsPage, views.AccountsRow) {
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

// whole is the phone's Detail drawn whole, each part under its heading, a
// blank row between: the picker, and under it how the account stands;
// USAGE; once the period's page is read, THIS WEEK, BY MODEL, BY DAY, and
// WEEKS with the verdict under it; SESSIONS, where sessions run on the
// account; and LATELY.
func (s stacked) whole() stack {
	c := growing(s.f.Width)
	s.picker(c, 0)
	s.stands(c, 1)
	y := s.usage(c, 1+partsApart)
	if s.page != nil {
		y = s.thisWeek(c, y+partsApart)
		y = s.byModel(c, y+partsApart)
		y = s.byDay(c, y+partsApart)
		y = s.weeks(c, y+partsApart)
	}
	var targets []Target
	if len(s.running) > 0 {
		y, targets = s.sessions(c, y+partsApart)
	}
	s.lately(c, y+partsApart)
	return stack{c: c, targets: targets, filled: s.filled()}
}

// picker draws the picker along row y: the account shown, its number, name
// and marks, but ▲, or 0 all accounts, between ◂ and ▸, bold accent.key;
// and at the right, where it fits beside it, ↑↓ an account, or all. With one
// account, there's no other to pick: its name alone.
func (s stacked) picker(c *canvas, y int) {
	if s.f.Accounts.alone() {
		c.line(0, y, s.named(s.f.edge()))
		return
	}
	shown := slices.Concat(line{{"◂", keyInk}, spaces(1)}, s.named(s.f.edge()-4), line{spaces(1), {"▸", keyInk}})
	c.line(0, y, shown)
	if hint := (line{{"↑↓", keyInk}, {" an account, or all", dimInk}}); shown.width()+1+hint.width() <= s.f.Width {
		c.right(s.f.Width, y, hint)
	}
}

// named is the account shown in the cells given: its number, its name, cut
// short to keep its number and marks, and its marks but ▲; or 0 all
// accounts.
func (s stacked) named(cells int) line {
	if s.id == "" {
		return line{{"0", dimInk}, {" all accounts", strongInk}}.fit(cells)
	}
	named := accountName{number: strconv.Itoa(s.account.Place), label: name(s.account), primary: s.account.Primary}
	return named.labelIn(cells - len(named.number) - 1 - named.marks().width())
}

// stands draws how the account shown stands along row y: its glyph and
// words, as its Headed says them short, picked out while its state is newly
// changed; or all accounts' states counted.
func (s stacked) stands(c *canvas, y int) {
	width := s.f.edge()
	if s.id == "" {
		c.line(0, y, countedStates(status.CountStates(s.m.Accounts), NumberAccountStates, width))
		return
	}
	end := c.line(0, y, standingWords(s.account.Headed(), true, width))
	s.f.cueStanding(c, s.id, 0, y, end)
}

// usage draws USAGE from row y, and returns the row its last window is on:
// its heading, then every window, used or not, a blank row before each but
// the first, named in text.subtle, then as usage draws a window, its bar
// stackUsageBar cells from stackUsageAt, its words ending at the frame's
// edge.
func (s stacked) usage(c *canvas, y int) int {
	c.line(0, y, line{{"USAGE", dimInk}})
	b := usageBlock{width: s.f.Width, look: s.f.Look}
	last := y
	for i, w := range s.windows() {
		last = y + 1 + i*partsApart
		c.line(0, last, line{{prose.Cut(status.Short(w.name), stackUsageAt-1), dimInk}})
		b.gauge(c, s.gauge(w.key), stackUsageAt, last, stackUsageBar, s.f.edge())
	}
	return last
}

// windows are the windows USAGE shows: every one, used or not, as usage
// shows them, each named as the Overview names it.
func (s stacked) windows() []shownWindow {
	return usageWindows(s.m, nil, s.f.windowNamer(s.m))
}

// gauge is the window with the given key as USAGE draws it: the account's,
// as usage draws it, eased and cued as the Overview's; or all accounts', as
// allGauge has it.
func (s stacked) gauge(key string) gauge {
	if s.id == "" {
		return s.f.overviewGauge(allGauge(s.m, key), false)
	}
	return s.f.overviewGauge(accountGauge(s.account, key, s.m.Now), false)
}

// filled are how much of each of USAGE's bars the phone's Detail draws it
// fills, as Filled has them: none drawn empty.
func (s stacked) filled() map[Ref]float64 {
	filled := make(map[Ref]float64)
	for _, w := range s.windows() {
		if g := s.gauge(w.key); !g.bar.empty {
			filled[g.of] = g.bar.used
		}
	}
	return filled
}

// stackItem is an item of THIS WEEK: its label, and its figure.
type stackItem struct {
	label string
	parts []part
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
// worth against the plan's price. The period's figures are a dash where it
// records nothing, as on a fresh install.
func (s stacked) thisWeekItems() [][]stackItem {
	totals := s.row.Totals
	return [][]stackItem{
		{
			{"requests", s.recorded([]part{figureIn(prose.Grouped(totals.Requests), secondaryInk)})},
			{"limits", s.recorded(limitsParts(totals, s.f.windowNamer(s.m), secondaryInk))},
			{"plan", planNamed(s.row, secondaryInk)},
		},
		{
			{"worth", s.recorded(worthParts(s.row, secondaryInk))},
			{"extra use", extraShort(s.shown())},
			{"against", s.recorded(againstShort(s.row))},
		},
	}
}

// recorded is a figure of the period, as the parts given say it, where it
// records anything; else a dash.
func (s stacked) recorded(parts []part) []part {
	if !s.page.Recorded {
		return noneOf(noFigure.text, secondaryInk)
	}
	return parts
}

// shown are the accounts the phone's Detail shows: the account, or every
// account.
func (s stacked) shown() []status.AccountNow {
	if s.id == "" {
		return s.m.Accounts
	}
	return []status.AccountNow{s.account}
}

// byModel draws BY MODEL from row y, and returns its last row: its heading,
// and on its row the period's share of use as a stacked bar stackModelBar
// cells from stackAt, as sharesBar draws it; then the versions it used, each
// with its share, over the rows under it, as versionsOver has them.
func (s stacked) byModel(c *canvas, y int) int {
	c.line(0, y, line{{"BY MODEL", dimInk}})
	sharesBar(c, s.f.Look, stackAt, y, stackModelBar, s.page.Versions, s.row)
	for _, l := range versionsOver(s.page.Versions, s.row, s.f.edge()) {
		y++
		c.line(0, y, l)
	}
	return y
}

// versionsOver are the versions of those given the row used, in the order
// given, each as versionNamed names it, two cells apart, over two rows
// width cells wide: on the first, as many as fit; on the second, the rest,
// the smallest left out where they don't all fit, as many as it takes for the
// rest, and how many more there are, as "+2 more" in text.subtle, to fit. A
// row with none on it is left out.
func versionsOver(versions []ledger.Version, row views.AccountsRow, width int) []line {
	var used []ledger.Version
	for _, v := range versions {
		if _, ok := shareOf(row, v); ok {
			used = append(used, v)
		}
	}
	first := 0
	for first < len(used) && versionsLine(used[:first+1], row, 0).width() <= width {
		first++
	}
	rows := []line{versionsLine(used[:first], row, 0)}
	rest := used[first:]
	smallest := slices.SortedStableFunc(slices.Values(rest), func(a, b ledger.Version) int {
		return cmp.Compare(shareOrNone(row, a), shareOrNone(row, b))
	})
	for out := range len(rest) + 1 {
		left := smallest[:out]
		kept := slices.DeleteFunc(slices.Clone(rest), func(v ledger.Version) bool {
			return slices.ContainsFunc(left, func(o ledger.Version) bool { return o.Name == v.Name })
		})
		if l := versionsLine(kept, row, out); l.width() <= width {
			rows = append(rows, l)
			break
		}
	}
	return slices.DeleteFunc(rows, func(l line) bool { return l == nil })
}

// shareOrNone is the row's share of use of the version given: none where it
// used none of it, or none of its use is priced.
func shareOrNone(row views.AccountsRow, v ledger.Version) float64 {
	if s, ok := shareOf(row, v); ok && s.Share != nil {
		return *s.Share
	}
	return 0
}

// versionsLine names the versions given, each as versionNamed names it, two
// cells apart, then, where more is any, how many more there are, as "+2
// more", in text.subtle.
func versionsLine(versions []ledger.Version, row views.AccountsRow, more int) line {
	named := make([]line, 0, len(versions)+1)
	for _, v := range versions {
		named = append(named, versionNamed(v, row))
	}
	if more > 0 {
		named = append(named, line{{"+" + strconv.Itoa(more) + " more", dimInk}})
	}
	return joined(spaces(2), named...)
}

// byDay draws BY DAY along row y, and returns the row under it: its heading;
// from stackAt, the last stackWeeks weeks' days, swatchCells a day and the
// weeks stackWeekApart apart, each in Days' shades at its level on the
// account's own scale, a red ▪ where it hit a 5-hour limit, and ·· in
// text.faint for each day still to come; and on the row under them, each
// week's first day, this week's bold.
func (s stacked) byDay(c *canvas, y int) int {
	c.line(0, y, line{{"BY DAY", dimInk}})
	days := s.row.Days
	first, coming := max(len(days)-7*stackWeeks, 0), len(days)-s.page.ToCome
	for i := first; i < len(days); i++ {
		x := stackAt + (i-first)/7*stackWeekApart + (i-first)%7*swatchCells
		if i >= coming {
			c.text(x, y, toCome, faintInk)
			continue
		}
		c.line(x, y, s.f.swatch(dayInk(days[i].Level), days[i].Level, days[i].Limit))
	}
	weeks := s.page.Weeks
	firstWeek := max(len(weeks)-stackWeeks, 0)
	for i, week := range weeks[firstWeek:] {
		c.line(stackAt+i*stackWeekApart, y+1, weekHead(week, firstWeek+i == len(weeks)-1))
	}
	return y + 1
}

// weeks draws WEEKS along row y, and returns the verdict's last row: its
// heading; from stackAt, the last stackPeaks weeks' peaks, as peakPart has
// them, this week's bold text.secondary, each right-aligned in
// stackPeakCells, stackPeakApart apart; and under it the verdict, as
// verdictShort has it, a row each of its parts.
func (s stacked) weeks(c *canvas, y int) int {
	c.line(0, y, line{{"WEEKS", dimInk}})
	peaks := s.row.Peaks[max(len(s.row.Peaks)-stackPeaks, 0):]
	for i, p := range peaks {
		c.rightFit(stackAt+i*stackPeakApart+stackPeakCells, s.f.edge(), y, peakPart(p, strongInk, Number{}))
	}
	for _, parts := range verdictShort(s.row) {
		y++
		c.line(0, y, fitParts(s.f.edge(), parts...))
	}
	return y
}

// sessions draws SESSIONS from row y, and returns its last row and the zones
// of its rows a click acts on: its heading, and now, in text.faint; from the
// row under it, the sessions running on the account, as many as
// stackSessionRows show, scrolled with the one selected, as
// stackSessionsShown has them, the last row saying how many more there are
// where more are below; and two rows under the last, what enter opens.
func (s stacked) sessions(c *canvas, y int) (int, []Target) {
	c.line(0, y, line{{"SESSIONS", dimInk}, {"  now", faintInk}})
	selected := s.selected()
	first, shown := stackSessionsShown(len(s.running), selected)
	var targets []Target
	for i, r := range s.running[first : first+shown] {
		y++
		s.session(c, r, y, first+i == selected)
		targets = append(targets, Target{Y: y, Width: stackSessionLit, On: OnSession, Session: r.Session})
	}
	if more := len(s.running) - first - shown; more > 0 {
		y++
		c.line(stackSessionAt, y, outOfView(0, more, stackSessionLit, runningOut))
	}
	y += partsApart
	opens := status.ShownID(s.running[selected].Session)
	c.line(stackSessionAt, y, line{{"enter", keyInk}, {" opens " + opens + "'s page", mutedInk}})
	return y, targets
}

// selected is the place among the sessions running of the one selected: the
// first, where none is, or the one selected no longer runs there.
func (s stacked) selected() int {
	return max(slices.IndexFunc(s.running, func(r views.Running) bool { return r.Session == s.f.Accounts.Selected }), 0)
}

// stackSessionsShown are the first of n sessions SESSIONS shows, the one
// with the index selected among them, and how many it shows: every one where
// they fit its rows; else scrolled no further than it takes to show the one
// selected, a row left at the foot for how many more there are while more
// are below.
func stackSessionsShown(n, selected int) (first, shown int) {
	if n <= stackSessionRows {
		return 0, n
	}
	first = min(max(selected-(stackSessionRows-2), 0), n-stackSessionRows)
	if first == n-stackSessionRows {
		return first, stackSessionRows
	}
	return first, stackSessionRows - 1
}

// session draws the session running r along row y: ▸, bold accent.key,
// where it's selected; ● in state.positive while it's busy, or ○ in
// text.subtle; its id, bold, text.primary while busy, else text.muted; two
// cells on, its model's family, padded, in text.secondary; what it's doing,
// padded, bold accent.mode while busy, else text.subtle; and when it was
// last seen, right-aligned, in text.subtle. The one selected is picked out.
func (s stacked) session(c *canvas, r views.Running, y int, selected bool) {
	now := s.m.Now
	dot, id, doing := span{"○", dimInk}, span{status.ShownID(r.Session), ink{token: theme.TextMuted, bold: true}}, dimInk
	if r.Activity.Busy(now) {
		dot, id.ink, doing = span{"●", positiveInk}, titleInk, nameInk
	}
	c.line(stackSessionAt, y, line{
		dot, spaces(1), id, spaces(2),
		{padded(prose.Cut(r.Family, 8), 8), secondaryInk},
		{padded(r.Activity.Short(), 9), doing},
		{fmt.Sprintf("%3s", runningSeen(r.Activity.LastSeen, now)), dimInk},
	})
	if selected {
		c.line(0, y, line{{"▸", keyInk}})
		c.surface(0, y, stackSessionLit, hue{token: theme.BgSelection})
	}
}

// lately draws LATELY from row y: its heading, and under it the account's
// last events, newest first, told from its side, or of all accounts, each
// naming its account, as AccountLately tells them: each its time, in a
// column as wide as the widest, in text.subtle; its mark, as LATELY marks
// it; and its words, in text.muted, cut to stackLatelyWords; each picked out
// while it's fresh. Where it has none, why.
func (s stacked) lately(c *canvas, y int) {
	c.line(0, y, line{{"LATELY", dimInk}})
	sentences := views.AccountLately(s.m, s.id, stackLatelyLines)
	if len(sentences) == 0 {
		s.f.quietly(c, s.m, 0, y+1, s.f.edge())
		return
	}
	column := 0
	for _, said := range sentences {
		column = max(column, len(prose.Dated(s.m.Now, said.At)))
	}
	for i, said := range sentences {
		var words line
		for _, p := range slices.Concat(said.Full.Subject, said.Full.Words, said.Full.Object, said.Full.Rest) {
			text := p.Text
			if p.Kind == status.PartAccount {
				text = named(s.m, p.Text)
			}
			words = append(words, span{text, mutedInk})
		}
		lead := line{{padded(prose.Dated(s.m.Now, said.At), column) + afterTime, dimInk}, eventMark(said), spaces(1)}
		end := c.line(0, y+1+i, append(lead, words.fit(stackLatelyWords)...).fit(s.f.edge()))
		s.f.freshen(c, said.ID, 0, y+1+i, end)
	}
}

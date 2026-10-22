package dashboard

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Accounts' narrow forms: Compare turned, a group at a time and an account a
// row, all accounts last, where its wide form doesn't fit (CompareWide); and
// Detail stacked, each part under its heading, where its wide form doesn't
// (detailIn). Each is drawn whole, then scrolled a row at a time, as
// History's Year is.

// Turned Compare's columns: its names from turnedNameAt, turnedApart before
// its bars; and its bars at least turnedBar cells.
const (
	turnedNameAt = 1
	turnedApart  = 2
	turnedBar    = 20
)

// The turned parts across a frame, in the order turnedLaid lays them. The
// figures' column is two parts: turnedFigures, a cell and the widest largest
// version at a share of two digits, and turnedHundred, the cell more a share
// of 100% takes.
const (
	turnedIndent = iota
	turnedNames
	turnedBars
	turnedFigures
	turnedHundred
)

// figuresNarrowed is the turn the figures' column narrows in, toward a cell
// of a version's name, where the names' least and the bars' don't leave it
// the widest version at a two-digit share: under 50 columns with the version
// table's names, and wider where the config prices a model named by a long
// id.
const figuresNarrowed = 1

// turnedLayout is where turned Compare's parts sit across a frame, as flex
// lays them: its names' cells, from turnedNameAt; where its bars start, as
// its groups' first columns do, and their cells; and where its figures'
// column starts, a cell after the bars: WEEK USED's percentages right-aligned
// in its first 4 cells, and BY MODEL's largest versions a cell in, so their
// figures line up.
type turnedLayout struct {
	names, at, bar, figures int
}

// turnedLaid is turned Compare's columns laid out across the frame: its
// names at their widest whole, at least compareNameLeast and at most
// compareNameMost; its bars at least turnedBar, growing to barMost; and its
// figures holding the widest largest version the table can name, as the
// frame's VersionNameCells has it, at a share of two digits, and taking the
// cell more a share of 100% needs last. The names give way before the figures
// do. The figures read no page, so nothing moves as it's read, or as the
// period changes.
func (cp compared) turnedLaid(columns []compareColumn) turnedLayout {
	names := 0
	for _, col := range columns {
		names = max(names, wholeName(turnedNamed(col)))
	}
	figures := largestCells(cp.f.VersionNameCells, 0.55)
	s, _ := flex.Lay(cp.f.edge(), []flex.Part{
		turnedIndent:  {Least: turnedNameAt, Natural: turnedNameAt},
		turnedNames:   {Least: compareNameLeast, Natural: names, Most: compareNameMost},
		turnedBars:    {Least: turnedBar, Natural: turnedBar, Grows: true, Most: barMost, Gap: turnedApart},
		turnedFigures: {Least: 1 + largestCells(1, 0.55), Natural: 1 + figures, Narrows: figuresNarrowed, Gap: 1},
		turnedHundred: {Natural: largestCells(cp.f.VersionNameCells, 1) - figures},
	}...)
	return turnedLayout{names: s[turnedNames].Size, at: s[turnedBars].At, bar: s[turnedBars].Size, figures: s[turnedFigures].At}
}

// largestCells are the cells a largest version takes, as versionsNamed names
// it, of a name the cells given at the share given.
func largestCells(name int, share float64) int {
	v := ledger.Version{Name: strings.Repeat("x", name)}
	return partsWidth(versionsNamed([]versionUsed{{version: v, share: &share}}, 0))
}

// largestIn is the largest version used given, as versionNamed names it, in
// the cells given: where it doesn't fit, its name is cut, never its share,
// which is numbered by the name whole however it's shown.
func largestIn(used versionUsed, cells int) line {
	name := prose.Clean(used.version.Name)
	named := versionNamed(used, name)
	if over := partsWidth(named) - cells; over > 0 {
		named = versionNamed(used, prose.Cut(name, max(ansi.StringWidth(name)-over, 1)))
	}
	return fitParts(cells, named...)
}

// A phone's Detail's columns: USAGE's bars at least stackBarLeast cells;
// THIS WEEK's two columns stackHalf apart, their labels stackLabels cells;
// and from stackAt, BY MODEL's bar, BY DAY's weeks and WEEKS' peaks, the
// peaks each right-aligned in stackPeakCells, stackPeakGap between.
const (
	stackBarLeast  = 18
	stackHalf      = 26
	stackLabels    = 10
	stackAt        = 10
	stackPeakCells = 5
	stackPeakGap   = 1
)

// The parts of Detail's USAGE row, stacked or wide, in the order
// detailUsageRow lays them.
const (
	detailUsageNames = iota
	detailUsageBar
	detailUsageAfter
	detailUsageWords
)

// detailUsage is where Detail's USAGE row's parts sit, stacked or wide: its
// names' cells, and where its bars start and their cells.
type detailUsage struct {
	names, at, bar int
}

// detailUsageRow is Detail's USAGE row, stacked or wide: its bars at least
// bar cells; its windows' names at least least cells, and names at their
// widest; and its words, words cells.
type detailUsageRow struct {
	bar, least, names, words int
}

// laid is the row laid out in room cells, as flex lays it, and whether its
// parts' leasts fit: its windows' names, from their least to their widest,
// at least compareNameLeast and at most compareNameMost; a cell on, its
// bars; then afterBar's cells, their percentages among them; and their
// words. The bars take their least first, then the names are raised toward
// their widest, then the bars grow into what's left, at most barMost, as
// every bar beside words does.
func (r detailUsageRow) laid(room int) (detailUsage, bool) {
	s, ok := flex.Lay(room, r.parts()...)
	return detailUsage{names: s[detailUsageNames].Size, at: s[detailUsageBar].At, bar: s[detailUsageBar].Size}, ok
}

// leastCells are the fewest cells the row takes: its parts at their leasts.
func (r detailUsageRow) leastCells() int {
	s, _ := flex.Lay(0, r.parts()...)
	return s[detailUsageWords].At + s[detailUsageWords].Size
}

// parts are the row's parts, as laid lays them.
func (r detailUsageRow) parts() []flex.Part {
	return []flex.Part{
		detailUsageNames: {Least: max(r.least, compareNameLeast), Natural: r.names, Most: compareNameMost},
		detailUsageBar:   {Least: r.bar, Natural: r.bar, Grows: true, Most: barMost, Gap: 1},
		detailUsageAfter: {Least: afterBar, Natural: afterBar},
		detailUsageWords: {Least: r.words, Natural: r.words},
	}
}

// partsApart is how far a phone's part is from the row before it, a blank
// row between.
const partsApart = 2

// detailPhoneBody is Detail of doc at now stacked, where the frame is too
// narrow for its wide form, as detailIn has it.
func (f Frame) detailPhoneBody(doc status.Document, now time.Time) accountsForm {
	s := f.stackedOf(doc, now)
	return f.stackForm(s.whole, s.filled)
}

// stack is a phone's view drawn whole: its rows, on a canvas the frame's
// width; and the zones of them a click acts on, by their rows on it.
type stack struct {
	c       *canvas
	targets []Target
}

// stackForm is the form of the stack drawn whole as drawn draws it, drawn
// once, the first time a look asks, for every look to share: as many of its
// rows as the view has, from as far down as the frame has it scrolled, a row
// at a time; and how much of each bar it draws it fills, as filled has them
// without drawing it.
func (f Frame) stackForm(drawn func() stack, filled func() map[Ref]float64) accountsForm {
	drawn = sync.OnceValue(drawn)
	return accountsForm{
		draw:      func(c *canvas, top int) { f.drawStack(c, drawn(), top) },
		scrolling: func(offset int) Scrolling { return pagedWhole(drawn().c, f.viewRows()).scrolling(offset) },
		targets:   func() []Target { return f.stackTargets(drawn()) },
		filled:    filled,
	}
}

// drawStack draws the stack's rows the view shows from row top, and over the
// key line, where they don't all show, how many are out of view, as
// Compare's line says it.
func (f Frame) drawStack(c *canvas, s stack, top int) {
	paged := pagedWhole(s.c, f.viewRows())
	from, to := paged.shown(f.Scroll)
	c.paste(s.c, from, top, to-from)
	c.line(0, f.Height-underView, outOfView(from, paged.content-to, f.edge(), compareOut))
}

// stackTargets are the stack's zones the view shows, at the rows it shows
// them on.
func (f Frame) stackTargets(s stack) []Target {
	from, to := pagedWhole(s.c, f.viewRows()).shown(f.Scroll)
	var shown []Target
	for _, t := range s.targets {
		if y := t.Y - from; y >= 0 && y < to-from {
			t.Y = f.top() + y
			shown = append(shown, t)
		}
	}
	return shown
}

// turned is Compare of doc at now turned: a column every account, none
// scrolled out of view, then all accounts', as compared reads them, each a
// row.
func (f Frame) turned(doc status.Document, now time.Time) compared {
	m := f.Moment(doc, now)
	return compared{f: f, m: m, now: now, page: f.Accounts.Page, shown: len(f.Accounts.Accounts), week: f.WeekWindow}
}

// turnedWhole is turned Compare drawn whole, laid out as turnedLaid lays
// it: WEEK USED; then, once the period's page is read, BY MODEL, ACTIVITY,
// LIMITS and VALUE, a blank row before each.
func (cp compared) turnedWhole() stack {
	c := growing(cp.f.Width)
	columns := cp.columns()
	l := cp.turnedLaid(columns)
	y := cp.turnedWeek(c, 0, l, columns)
	if cp.page != nil {
		y = cp.turnedByModel(c, y+partsApart, l, columns)
		for _, g := range cp.turnedGroups() {
			y = cp.turnedGroup(c, y+partsApart, l, g, columns)
		}
	}
	return stack{c: c}
}

// turnedNamed is the column's name as turned Compare's row heads it: an account's
// label and marks, but ▲, as Accounts shows no routing; or all, unnumbered.
func turnedNamed(col compareColumn) accountName {
	if col.all {
		return accountName{label: "all"}
	}
	return accountName{label: name(col.account), primary: col.account.Primary}
}

// turnedName is the column's name, as turnedNamed has it, in the cells
// given, its label cut short to keep its marks; all bold text.secondary.
func turnedName(col compareColumn, cells int) line {
	if col.all {
		return line{{"all", strongInk}}
	}
	named := turnedNamed(col)
	return named.labelIn(cells - named.marks().width())
}

// turnedWeek draws WEEK USED from row y, and returns the row its last column
// is on: its label, as weekLabel has it; then each column, a blank row before
// it, its name, its week's bar, as weekGauge has it, and its percentage, as
// the layout places them.
func (cp compared) turnedWeek(c *canvas, y int, l turnedLayout, columns []compareColumn) int {
	c.line(0, y, weekLabel(cp.f.Accounts.Period))
	c.heading(y, y+partsApart)
	for _, col := range columns {
		y += partsApart
		c.line(turnedNameAt, y, turnedName(col, l.names))
		g := cp.weekGauge(col)
		g.bar.draw(c, cp.f.Look, l.at, y, l.bar)
		c.barred(l.at, y, l.bar, g.of)
		c.right(l.figures+percentCells, y, g.percentage())
	}
	return y
}

// turnedByModel draws BY MODEL from row y, and returns the row its last
// column is on: its heading; then each column, a blank row before it, its
// name, its share of use as a stacked bar, as Compare's, and the version it
// used most, "● Opus 5.5 55%", as largestIn fits it to the frame's edge, as
// the layout places them.
func (cp compared) turnedByModel(c *canvas, y int, l turnedLayout, columns []compareColumn) int {
	c.line(0, y, compareByModelHead(true))
	c.heading(y, y+partsApart)
	versions := cp.page.Versions
	hues := versionHues(versions)
	for _, col := range columns {
		y += partsApart
		c.line(turnedNameAt, y, turnedName(col, l.names))
		stackedBar(c, cp.f.Look, l.at, y, splitCells(shareAmounts(col.row, versions), l.bar), hues)
		if most, ok := largestVersion(versionsUsed(col.row, versions)); ok {
			c.line(l.figures+1, y, largestIn(most, cp.f.edge()-l.figures-1))
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

// turnedGroup is a group of turned Compare, as ACTIVITY: its name, and its
// columns.
type turnedGroup struct {
	name    string
	columns []turnedColumn
}

// turnedColumn is a column of turned Compare's group: its head, and what it
// says of each of Compare's columns.
type turnedColumn struct {
	head string
	said func(col compareColumn) []part
}

// turnedGroups are turned Compare's groups under BY MODEL, ACTIVITY,
// LIMITS and VALUE, as Accounts' words say each figure in their short forms,
// a window of hours named by them, as 5h. A period with nothing recorded, as
// on a fresh install, says a dash in each of its own figures, as period has
// it; left and the plan are none of them.
func (cp compared) turnedGroups() []turnedGroup {
	limits := shortWindows(cp.f.windowNamer(cp.m))
	return []turnedGroup{
		{name: "ACTIVITY", columns: []turnedColumn{
			{"reqs", cp.period(func(col compareColumn) []part {
				return []part{figureIn(prose.Grouped(col.row.Totals.Requests), col.ink())}
			})},
			{"sessions", cp.period(func(col compareColumn) []part {
				return []part{figureIn(prose.Grouped(col.row.Totals.Sessions), col.ink())}
			})},
			{"moved", cp.period(func(col compareColumn) []part { return movedParts(col.row.Totals, col.ink()) })},
		}},
		{name: "LIMITS", columns: []turnedColumn{
			{"hit", cp.period(func(col compareColumn) []part { return limitsParts(col.row.Totals, limits, col.ink()) })},
			{"at cap", cp.period(func(col compareColumn) []part { return atCapParts(col.row.Totals, col.ink()) })},
			{"left", func(col compareColumn) []part { return leftShort(col.row, col.ink()) }},
		}},
		{name: "VALUE", columns: []turnedColumn{
			{"plan", func(col compareColumn) []part { return planPriced(col.row, col.ink()) }},
			{"worth", cp.period(func(col compareColumn) []part { return worthParts(col.row, col.ink()) })},
			{"against it", cp.period(func(col compareColumn) []part { return againstOnPhone(col.row) })},
		}},
	}
}

// turnedGroup draws the group from row y, and returns the row its last
// column is on: its name, and on its row each of its columns' heads, in
// text.subtle; then a row each of Compare's columns, no blank between. Its
// columns share the room from the layout's bars to the frame's edge alike,
// as flex lays them, so every group's line up; each figure is cut a cell
// short of the next column, the last's at the frame's edge.
func (cp compared) turnedGroup(c *canvas, y int, l turnedLayout, g turnedGroup, columns []compareColumn) int {
	shared := make([]flex.Part, len(g.columns))
	for i := range shared {
		shared[i] = flex.Part{Grows: true}
	}
	spans, _ := flex.Lay(cp.f.edge()-l.at, shared...)
	c.line(0, y, line{{g.name, dimInk}})
	c.heading(y, y+1)
	for i, head := range g.columns {
		c.line(l.at+spans[i].At, y, line{{head.head, dimInk}})
	}
	for _, col := range columns {
		y++
		c.line(turnedNameAt, y, turnedName(col, l.names))
		for i, figure := range g.columns {
			at, cells := l.at+spans[i].At, spans[i].Size-1
			if i+1 == len(g.columns) {
				cells = cp.f.edge() - at
			}
			c.line(at, y, fitParts(cells, periodNumbered(figure.said(col), col.id, figure.head)...))
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
var stackLayout = detailLayout{into: stackAt, dw: dayCellsLeast, gap: 1}

// stackedOf is a phone's Detail of doc at now: of the account the frame's
// Accounts shows, else all accounts, as detailDrawn has it.
func (f Frame) stackedOf(doc status.Document, now time.Time) stacked {
	return stacked{f.detailDrawn(doc, now, stackLayout)}
}

// whole is the phone's Detail drawn whole, each part under its heading, a
// blank row between: the picker, and under it how the account stands, short,
// as stands has it; USAGE; once the period's page is read, THIS WEEK, BY
// MODEL, BY DAY, and WEEKS with the verdict under it; SESSIONS, as Detail
// draws it, where sessions run on the account; and LATELY, its words uncut,
// each line cut at the frame's edge.
func (s stacked) whole() stack {
	c := growing(s.f.Width)
	s.picker(c, 0)
	s.stands(c, 0, 1, true)
	y := s.usage(c, 1+partsApart)
	if s.page != nil {
		y = s.thisWeek(c, y+partsApart)
		y = s.byModel(c, y+partsApart) + partsApart
		s.byDay(c, y, s.daysWeeks())
		y = s.weeks(c, y+1+partsApart)
	}
	var targets []Target
	if len(s.running) > 0 {
		y, targets = s.sessions(c, y+partsApart)
	}
	y += partsApart
	c.line(0, y, line{{"LATELY", dimInk}})
	c.heading(y, y+1)
	s.lately(c, y+1, 0, s.f.edge())
	return stack{c: c, targets: targets}
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
// text.subtle, then as Detail's gauge has it, laid out as detailUsageRow
// lays its row across the frame: its names as wide as windowNames has them,
// at least compareNameLeast, and its words the cells of their form at their
// widest, so neither moves as the windows shown or their words change; its
// words ending at the frame's edge.
func (s stacked) usage(c *canvas, y int) int {
	c.line(0, y, line{{"USAGE", dimInk}})
	c.heading(y, y+1)
	b := usageBlock{width: s.f.Width, look: s.f.Look}
	l, _ := detailUsageRow{bar: stackBarLeast, names: s.windowNames(), words: b.widestWords()}.laid(s.f.edge())
	last := y
	for i, w := range s.windows() {
		last = y + 1 + i*windowsApart
		c.line(0, last, line{{prose.Cut(status.Short(w.name), l.names), dimInk}})
		b.gauge(c, s.gauge(w.key), l.at, last, l.bar, s.f.edge())
	}
	return last
}

// thisWeek draws THIS WEEK from row y, and returns its last row: its title,
// naming the period, then its items in two columns stackHalf apart, a row
// an item, their labels stackLabels cells in text.subtle, each figure cut a
// cell short of the next column, or of the frame's edge.
func (s stacked) thisWeek(c *canvas, y int) int {
	c.line(0, y, line{{s.page.Period.Title(), dimInk}})
	c.heading(y, y+1)
	halves := s.thisWeekItems()
	for h, items := range halves {
		x, end := h*stackHalf, s.f.edge()
		if h+1 < len(halves) {
			end = (h+1)*stackHalf - 1
		}
		for i, item := range items {
			c.line(x, y+1+i, line{{padded(item.label, stackLabels), dimInk}})
			c.line(x+stackLabels, y+1+i, fitParts(end-x-stackLabels, periodNumbered(item.parts, s.id, item.label)...))
		}
	}
	return y + len(halves[0])
}

// thisWeekItem is an item of a phone's THIS WEEK: its label, and its figure.
type thisWeekItem struct {
	label string
	parts []part
}

// thisWeekItems are THIS WEEK's items in its two columns, as the words of
// Accounts say them, in text.secondary: the requests, the limits, by window,
// or all accounts' how many, and the plan, by its name, or all accounts'
// summed; then the worth, the extra usage of the accounts shown, and the
// worth against the plan's price. Each of the period's figures is as
// periodParts says it, a dash where the period records nothing; the plan
// and extra use are none of them.
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
			{"extra use", extraShort(s.extra())},
			{"against", periodParts(page, againstOnPhone(s.row), secondaryInk)},
		},
	}
}

// byModel draws BY MODEL from row y, and returns its last row: its heading,
// and on its row the period's share of use as a stacked bar across the
// stretch, as Detail's; then the versions it used, each with its
// share, over the rows under it, as versionsOver has them.
func (s stacked) byModel(c *canvas, y int) int {
	c.line(0, y, line{{"BY MODEL", dimInk}})
	versions := s.page.Versions
	stretch := s.stretch()
	stackedBar(c, s.f.Look, stretch.At, y, splitCells(shareAmounts(s.row, versions), stretch.Size), versionHues(versions))
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

// stretch is where the row runs from cx to the frame's edge, after the
// headings' column, as flex lays it: BY MODEL's bar, and the room BY DAY's
// weeks and WEEKS' peaks lie in; no cells where the frame is narrower.
func (s stacked) stretch() flex.Span {
	spans, _ := flex.Lay(s.f.edge(), flex.Part{Least: s.l.cx(), Natural: s.l.cx()}, flex.Part{Grows: true})
	return spans[1]
}

// daysWeeks are BY DAY's weeks shown, the latest that fit the stretch, as
// weeksIn lays them.
func (s stacked) daysWeeks() []flex.Span {
	return s.l.weeksIn(s.stretch().Size, len(s.page.Weeks))
}

// openHang are the cells an open week's peak says past a closed one's, its
// … of a week still running, which hangs past its peak's cells so its % is
// in line with the others'.
var openHang = ansi.StringWidth(views.Peak(0, true)) - ansi.StringWidth(views.Peak(0, false))

// weeks draws WEEKS along row y, and returns the verdict's last row: its
// heading; across the stretch, keeping openHang cells at its end, the latest
// weeks' peaks that fit, as latestFitting lays them, each as peakPart has
// it, this week's bold text.secondary, right-aligned in stackPeakCells,
// stackPeakGap between, an open week's … past them; and under it the
// verdict, as verdictShort has it, a row each of its parts.
func (s stacked) weeks(c *canvas, y int) int {
	c.line(0, y, line{{"WEEKS", dimInk}})
	stretch := s.stretch()
	spans := latestFitting(stretch.Size-openHang, len(s.row.Peaks), stackPeakCells, stackPeakGap)
	peaks := s.row.Peaks[len(s.row.Peaks)-len(spans):]
	for i, p := range peaks {
		end := stretch.At + spans[i].At + spans[i].Size
		if p.Open && p.Share != nil {
			end += openHang
		}
		c.rightFit(end, s.f.edge(), y, peakPart(p, strongInk, Number{}))
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

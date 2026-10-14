package dashboard

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Where the Year is drawn, as the design lays it out at 160 columns, and on
// a phone.
const (
	// yearGridX is the grid's first column, and the key's; yearPhoneGridX a
	// phone's, and yearPhoneWeeks the latest weeks it shows.
	yearGridX      = 14
	yearPhoneGridX = 4
	yearPhoneWeeks = 22
	// yearNumbersX is the numbers' column, their labels yearLabelCells wide;
	// a phone's are in two columns, the second at yearPhoneSecondX, their
	// labels yearPhoneLabelCells wide, three to a column.
	yearNumbersX        = 126
	yearLabelCells      = 13
	yearPhoneSecondX    = 26
	yearPhoneLabelCells = 10
	yearPhoneFigures    = 3
	// The rows under the view's top the key, the numbers on a phone, and BY
	// MONTH start at, on a wide screen and on a phone.
	yearKeyRow          = 9
	yearByMonthRow      = 11
	yearPhoneNumbersRow = 11
	yearPhoneByMonthRow = 16
	// byMonthNameX is the column a month's name is at, and byMonthBarX its
	// bar's, byMonthBarCells long at most; and on a phone, the last
	// byMonthPhoneMonths months'.
	byMonthNameX         = 2
	byMonthBarX          = 60
	byMonthBarCells      = 30
	byMonthPhoneNameX    = 1
	byMonthPhoneBarX     = 32
	byMonthPhoneBarCells = 18
	byMonthPhoneMonths   = 6
)

// whole is the Year drawn whole on a canvas of its own, from its first row,
// as tall as height lays it out.
func (y yearDrawn) whole() *canvas {
	c := newCanvas(y.Width, y.height())
	y.draw(c, 0)
	return c
}

// draw draws the Year from row top in its form: wide, or as a phone lays it
// out.
func (y yearDrawn) draw(c *canvas, top int) {
	if y.form.phone {
		y.phone(c, top)
		return
	}
	y.wide(c, top)
}

// wide draws the Year as the design lays it out at 160 columns: the grid,
// the key under it, the numbers at its right, each cut at the frame's width,
// BY MONTH and the bars' key.
func (y yearDrawn) wide(c *canvas, top int) {
	y.grid(c, yearGridX, top, y.page.Weeks, 0, dayAbbreviation)
	y.key(c, top+yearKeyRow, keyWide)
	y.heading(c, yearNumbersX, top)
	for i, figure := range y.figures() {
		c.line(yearNumbersX, top+1+i, figure.drawn(figure.label, yearLabelCells, y.Width-yearNumbersX))
	}
	y.byMonth(c, top)
	y.dots(c, byMonthBarX, top+y.dotsRow())
}

// yearHeight is how many rows the Year takes, as it's laid out: none before
// its page is built.
func (f Frame) yearHeight() int {
	y, ok := f.yearDrawn()
	if !ok {
		return 0
	}
	return y.height()
}

// reach is the width the Year's wide form takes, drawn to the frame's width:
// where its widest row ends, the grid's, a number's whose length is bounded,
// at the widest its format draws, so the form never hangs on the data, BY
// MONTH's or the bars' key's. The rest of the numbers, the heading over
// them, which may name an account by its label, and the shades' key are cut
// at the frame's width, so they're left out.
func (y yearDrawn) reach() int {
	ends := []int{y.gridReach(), y.byMonthReach(), byMonthBarX + y.familyDots().width()}
	for _, figure := range y.figures() {
		if figure.widest > 0 {
			ends = append(ends, yearNumbersX+ansi.StringWidth(padded(figure.label, yearLabelCells))+figure.widest)
		}
	}
	return slices.Max(ends)
}

// gridReach is the column the wide grid ends before: its last week's, or
// the name of a month over the weeks.
func (y yearDrawn) gridReach() int {
	end := 0
	if weeks := len(y.page.Weeks); weeks > 0 {
		end = yearGridX + 2*(weeks-1) + 1
	}
	for _, l := range y.page.Months {
		end = max(end, yearGridX+2*l.Week+ansi.StringWidth(monthName(l.Month, false)))
	}
	return end
}

// byMonthReach is the column BY MONTH, wide, ends before: its heading's, its
// heads', the row before a version's months', or a month's, its note after
// its bar, which is byMonthBarCells long at most. A month's figures, before
// its bar, are left out: each is in a column, short of the bars.
func (y yearDrawn) byMonthReach() int {
	end := max(groupHead("BY MONTH", y.byMonthSays(), false).width(), byMonthBarX+ansi.StringWidth(y.barHead()))
	for _, col := range y.monthColumns(byMonthWide) {
		end = max(end, col.x+ansi.StringWidth(col.head))
	}
	if b := y.page.Before; b != nil {
		months, then := beforeSaid(*b)
		end = max(end, byMonthNameX+ansi.StringWidth(months), byMonthWorthX+ansi.StringWidth(then))
	}
	largest := largestWorth(y.page.ByMonth)
	for _, m := range y.page.ByMonth {
		bar := byMonthBarX + y.monthBar(m, largest, byMonthBarCells).width()
		if note := monthNote(m); note != nil {
			bar += 2 + partsWidth(note)
		}
		end = max(end, bar)
	}
	return end
}

// phone draws the Year as a phone has it: the latest weeks that fit, 22 at
// most, under their months' names; the key cut short; the numbers, the
// first six, in two columns of three, the first's cut a cell before the
// second's, as limits of two windows run past it; and BY MONTH, the last six
// months, its heads on its own row.
func (y yearDrawn) phone(c *canvas, top int) {
	weeks := y.page.Weeks
	first := max(len(weeks)-y.phoneWeeks(), 0)
	y.grid(c, yearPhoneGridX, top, weeks[first:], first, dayInitial)
	y.key(c, top+yearKeyRow, keyPhone)
	y.heading(c, 0, top+yearPhoneNumbersRow)
	figures := y.figures()
	for i, figure := range figures[:min(len(figures), 2*yearPhoneFigures)] {
		if row := top + yearPhoneNumbersRow + 1 + i; i < yearPhoneFigures {
			c.line(0, row, figure.drawn(figure.short, yearPhoneLabelCells, min(yearPhoneSecondX-1, y.Width)))
		} else {
			c.line(yearPhoneSecondX, row-yearPhoneFigures, figure.drawn(figure.short, yearPhoneLabelCells, y.Width-yearPhoneSecondX))
		}
	}
	y.byMonth(c, top)
}

// phoneWeeks is how many weeks a phone's grid shows: as many as fit, 22 at
// most, two cells apart from its column.
func (y yearDrawn) phoneWeeks() int {
	return max(min(yearPhoneWeeks, (y.Width-yearPhoneGridX+1)/2), 0)
}

// yearDrawn is the Year as the frame draws it: its page, what it was asked
// to show, and how it's laid out, on a phone or wide, as BY MONTH's form
// gives it.
type yearDrawn struct {
	Frame
	page views.YearPage
	ask  views.YearAsk
	form byMonthForm
}

// yearDrawn is the Year as the frame draws it, wide where its widest row
// fits, else as a phone lays it out, reporting false before its page is
// built.
func (f Frame) yearDrawn() (yearDrawn, bool) {
	page := f.History.Year
	if page == nil {
		return yearDrawn{}, false
	}
	y := yearDrawn{Frame: f, page: *page, ask: f.History.YearAsk, form: byMonthWide}
	if f.phoneForm(y.reach) {
		y.form = byMonthPhone
	}
	return y, true
}

// height is how many rows the Year takes under its top: to BY MONTH's last
// month, and wide, to the bars' key under it, where there are bars to key.
func (y yearDrawn) height() int {
	if y.form.phone || len(y.page.Families) == 0 {
		return y.placed().last() + 1
	}
	return y.dotsRow() + 1
}

// dotsRow is the row under the Year's top the bars' key is on, wide: two
// under BY MONTH's last month.
func (y yearDrawn) dotsRow() int {
	return y.placed().last() + 2
}

// ofVersion reports whether the Year shows a version alone.
func (y yearDrawn) ofVersion() bool {
	return y.page.Version.Name != ""
}

// grid draws the weeks given, those from the page's week first on, a column
// each from column x, two cells apart, their months' names on row top over
// them, the current month's in text.secondary, and the days on the seven
// rows under it, each in its level's shade; the first, third and fifth
// days' names at column 0, as name gives them.
func (y yearDrawn) grid(c *canvas, x, top int, weeks []views.GridWeek, first int, name func(time.Weekday) string) {
	for _, l := range y.page.Months {
		if l.Week < first {
			continue
		}
		k := faintInk
		if l.Current {
			k = secondaryInk
		}
		at := x + 2*(l.Week-first)
		c.line(at, top, line{{monthName(l.Month, false), k}}.fit(y.Width-at))
	}
	if len(weeks) == 0 {
		return
	}
	if start, ok := civilDay(weeks[0].First); ok {
		for _, i := range []int{0, 2, 4} {
			c.line(0, top+1+i, line{{name(start.AddDate(0, 0, i).Weekday()), dimInk}}.fit(y.Width))
		}
	}
	for w, week := range weeks {
		for i, day := range week.Days {
			if !day.LeftOut {
				c.line(x+2*w, top+1+i, line{y.square(day.Level)})
			}
		}
	}
}

// square is a day at its level, as the grid and the key draw it, in its
// shade, or, filtered to a version, in its step of the version's shade.
func (y yearDrawn) square(level int) span {
	k := dayInk(level)
	if y.ofVersion() {
		k = stepInk(versionHue(y.page.Version), level)
	}
	return y.daySquare(k, level)
}

// daySquare is a day at its level as the Year draws it, in its shade's ink,
// k: ■; or, in a look that draws no shades of its own, its level's glyph.
func (f Frame) daySquare(k ink, level int) span {
	if !f.Look.blends() {
		return span{shadeGlyph(level), k}
	}
	return span{"■", k}
}

// keyForm is how the key is laid out: from the column x, its shades gap
// apart, each one's bounds written as number writes a cut, after under for
// the first quarter's and over for the last's, and level 0's as none.
type keyForm struct {
	x                 int
	gap               string
	number            func(int) string
	none, under, over string
}

// The key's forms: wide, from the grid's column, and cut short on a phone.
var (
	keyWide  = keyForm{x: yearGridX, gap: "   ", number: prose.Grouped, none: "none", under: "under ", over: "over "}
	keyPhone = keyForm{gap: " ", number: kilo, none: "0", under: "<", over: ">"}
)

// key draws the shades' key along row row, as the form lays it out: each
// shade's square and its bounds in requests a day, a figure, then what they
// count, cut at the frame's edge.
func (y yearDrawn) key(c *canvas, row int, form keyForm) {
	var parts []part
	for level, bound := range form.bounds(y.page.KeyCuts) {
		if level > 0 {
			parts = append(parts, wordsIn(form.gap, dimInk))
		}
		parts = append(parts, wordsPart(line{y.square(level), {" ", dimInk}}), figureIn(bound, dimInk))
	}
	parts = append(parts, wordsIn("  "+y.unit("requests a day"), faintInk))
	c.line(form.x, row, fitParts(y.Width-form.x, parts...))
}

// bounds are each level's bounds in requests a day, as cuts, the key's, give
// them: level 0's none; the first quarter's under the first cut; the last's
// over the last; and those between from one cut to the next. Without cuts,
// no day is active, and level 0's alone are given.
func (form keyForm) bounds(cuts []int) []string {
	bounds := []string{form.none}
	if len(cuts) == 0 {
		return bounds
	}
	bounds = append(bounds, form.under+form.number(cuts[0]))
	for i := 1; i < len(cuts); i++ {
		bounds = append(bounds, form.number(cuts[i-1])+"–"+form.number(cuts[i]))
	}
	return append(bounds, form.over+form.number(cuts[len(cuts)-1]))
}

// kilo writes n as a phone's key has it: in thousands from a thousand, as
// "1k" or "1.25k".
func kilo(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return strconv.FormatFloat(float64(n)/1000, 'f', -1, 64) + "k"
}

// shown is the use shown alone, as the heading row names it: "" for all
// use.
func (y yearDrawn) shown() string {
	return y.History.yearShown()
}

// unit is what the Year counts, the use shown named before it: as requests
// a day, Opus 5.5 requests a day, or work requests a day.
func (y yearDrawn) unit(what string) string {
	return strings.TrimSpace(y.shown() + " " + what)
}

// heading draws the numbers' heading at x along row row, cut at the frame's
// width: the period, LAST 12 MONTHS or the year, as 2025; or after the use
// shown, OPUS 5.5 · 12 MONTHS or WORK · 2025.
func (y yearDrawn) heading(c *canvas, x, row int) {
	period := "12 MONTHS"
	if y.ask.Year != 0 {
		period = strconv.Itoa(y.ask.Year)
	}
	lead := ""
	switch {
	case y.shown() != "":
		lead = strings.ToUpper(y.shown()) + " · "
	case y.ask.Year == 0:
		lead = "LAST "
	}
	c.line(x, row, fitParts(y.Width-x, wordsIn(lead, dimInk), figureIn(period, dimInk)))
}

// yearFigure is a number beside the grid: its label, and a phone's, what it
// says, and its shorter forms, each leaving off more of it whole, as limits
// of more than one window have; whether a look can change it, and the
// number it is where one can; and where what it is bounds how long it runs,
// as a day, a run of the year's days or a share do, the cells the widest its
// format draws takes: none for a count or a sum of dollars, which runs as
// long as the ledger makes it.
type yearFigure struct {
	label, short string
	value        line
	shorter      []line
	changes      bool
	number       Number
	widest       int
}

// drawn is the figure in width cells, labelled as given, its label padded
// to labelCells: the label words, then what it says, a figure, in the
// longest of its forms that fits, as many windows of limits as fit whole;
// cut as fitParts cuts them.
func (figure yearFigure) drawn(label string, labelCells, width int) line {
	said := figure.value
	for _, form := range figure.shorter {
		if max(labelCells, ansi.StringWidth(label))+said.width() <= width {
			break
		}
		said = form
	}
	return fitParts(width, wordsIn(padded(label, labelCells), dimInk), figurePart(said, figure.number))
}

// figures are the numbers beside the grid, in order: of all use, or an
// account's, its requests, sessions, limits hit, worth, what the plans cost
// and the worth against it, its busiest day and its longest run; of a
// version, its requests, sessions, worth, share of all use's worth, the day
// it was first used and its busiest day. Of a period with no requests, each
// is a dash. Each a look can change, all but what the plans cost, is
// numbered as the Year's, by its label.
func (y yearDrawn) figures() []yearFigure {
	n := y.page.Numbers
	figures := []yearFigure{
		{label: "requests", short: "requests", value: line{{prose.Grouped(n.Requests), secondaryInk}}, changes: true},
		{label: "sessions", short: "sessions", value: line{{prose.Grouped(n.Sessions), secondaryInk}}, changes: true},
	}
	if y.ofVersion() {
		figures = append(figures,
			yearFigure{label: "worth", short: "worth", value: worthSaid(n.Worth, secondaryInk), changes: true},
			yearFigure{label: "of all use", short: "share", value: shareSaid(n.Share, secondaryInk), changes: true, widest: widestShare},
			yearFigure{label: "first used", short: "first", value: daySaid(n.FirstUsed), changes: true, widest: widestDay},
			yearFigure{label: "busiest day", short: "busiest", value: daySaid(n.BusiestDay), changes: true, widest: widestDay},
		)
	} else {
		figures = append(figures,
			y.limitsFigure(n.Limits),
			yearFigure{label: "worth", short: "worth", value: worthSaid(n.Worth, secondaryInk), changes: true},
			yearFigure{label: "plans cost", short: "plans", value: costSaid(n.Cost)},
			yearFigure{label: "against it", short: "against", value: againstSaid(n.Ratio), changes: true},
			yearFigure{label: "busiest day", short: "busiest", value: daySaid(n.BusiestDay), changes: true, widest: widestDay},
			yearFigure{label: "longest run", short: "run", value: runSaid(n.LongestRun), changes: true, widest: widestRun},
		)
	}
	for i, figure := range figures {
		if n.Requests == 0 {
			figures[i].value, figures[i].shorter = line{noFigure}, nil
		}
		if figure.changes {
			figures[i].number = y.number(figure.label)
		}
	}
	return figures
}

// number is the Year's number of the figure named of.
func (y yearDrawn) number(of string) Number {
	return Number{Kind: NumberYear, Account: y.ask.Account, Of: strings.TrimSpace(y.ask.Version + " " + of)}
}

// limitsFigure is the limits hit, by window, in state.destructive, those of
// more than one window joined by a faint ·, as "5-hour ×402 · week ×3": none
// where none was. Its shorter forms leave off its last windows whole, one
// at a time, as history-phone-year-52 draws "5-hour ×402" where both don't
// fit.
func (y yearDrawn) limitsFigure(limits map[string]int) yearFigure {
	figure := yearFigure{label: "limits hit", short: "limits", value: line{{"none", secondaryInk}}, changes: true}
	if len(limits) == 0 {
		return figure
	}
	var forms []line
	for i, key := range slices.SortedFunc(maps.Keys(limits), quota.CompareKeys) {
		var l line
		if i > 0 {
			l = append(slices.Clone(forms[i-1]), span{" · ", faintInk})
		}
		forms = append(forms, append(l, span{y.windowName(key) + " ×" + prose.Grouped(limits[key]), errorInk}))
	}
	slices.Reverse(forms)
	figure.value, figure.shorter = forms[0], forms[1:]
	return figure
}

// windowName names a window by its key as a sentence has it, as the frame's
// WindowName names it, else by its length: "5-hour" or "week".
func (y yearDrawn) windowName(key string) string {
	if y.WindowName != nil {
		return status.InProse(y.WindowName(key))
	}
	return status.InProse(status.NameOfWindow(key, ""))
}

// costSaid is what the plans cost, as worthSaid writes a priced worth: a
// dash where no plan is known.
func costSaid(cost *ledger.Picodollars) line {
	if cost == nil {
		return line{noFigure}
	}
	return line{{cost.GroupedDollars(), secondaryInk}}
}

// daySaid names the day with the given date, as "Tue 22 Sep": a dash for
// none.
func daySaid(date string) line {
	day, ok := civilDay(date)
	if !ok {
		return line{noFigure}
	}
	return line{{day.Format(dayLayout), secondaryInk}}
}

// runSaid says how many days in a row there were, as "58 days": a dash for
// none.
func runSaid(days int) line {
	if days == 0 {
		return line{noFigure}
	}
	return line{{prose.Counted(days, "day"), secondaryInk}}
}

// ordinal is the day of the month as a sentence counts it, as 22nd.
func ordinal(day int) string {
	suffix := "th"
	if day/10 != 1 {
		switch day % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(day) + suffix
}

// byMonthForm is how BY MONTH is laid out: wide, or on a phone, its heads on
// its own row, its last months alone and none of its notes; the row under
// the Year's top it starts at; the column its months' names are at, and its
// bars', the most cells a bar takes, and how many of the last months it
// shows, 0 for all.
type byMonthForm struct {
	phone                 bool
	row                   int
	nameX, barX, barCells int
	months                int
}

// BY MONTH's forms, wide and on a phone.
var (
	byMonthWide  = byMonthForm{row: yearByMonthRow, nameX: byMonthNameX, barX: byMonthBarX, barCells: byMonthBarCells}
	byMonthPhone = byMonthForm{
		phone: true, row: yearPhoneByMonthRow, nameX: byMonthPhoneNameX, barX: byMonthPhoneBarX, barCells: byMonthPhoneBarCells, months: byMonthPhoneMonths,
	}
)

// byMonthPlaced is where BY MONTH's rows are under the Year's top: its
// heads', on a phone its heading's, else the one under it; whether the row
// under them is the one for the months before a version was first used, as
// it is wide where there are any; and the months it shows, oldest first, on
// a phone the last of them alone, a row each from first.
type byMonthPlaced struct {
	heads  int
	before bool
	first  int
	months []views.MonthRow
}

// placed is where BY MONTH's rows are under the Year's top, as its form lays
// them out.
func (y yearDrawn) placed() byMonthPlaced {
	f := y.form
	p := byMonthPlaced{heads: f.row + 1, before: y.page.Before != nil && !f.phone, months: y.page.ByMonth}
	if f.phone {
		p.heads = f.row
	}
	if p.first = p.heads + 1; p.before {
		p.first++
	}
	if f.months > 0 {
		p.months = p.months[max(len(p.months)-f.months, 0):]
	}
	return p
}

// last is the row of the last month shown: where none is, the row over
// where the first would be.
func (p byMonthPlaced) last() int {
	return p.first + len(p.months) - 1
}

// monthColumn is a column of BY MONTH: its head, the column it's at, what it
// says of a month, and whether a look can change it, in the month so far.
type monthColumn struct {
	head    string
	x       int
	of      func(views.MonthRow) line
	changes bool
}

// The columns of BY MONTH's worth, and of what follows it, wide and on a
// phone; the row before a version's months names the version used then
// under its worth.
const (
	byMonthWorthX      = 26
	byMonthThirdX      = 38
	byMonthLimitsX     = 48
	byMonthPhoneWorthX = 12
	byMonthPhoneThirdX = 22
)

// monthColumns are BY MONTH's columns, before its bars: wide, a month's
// requests, worth and against, then its limits, or filtered to a version,
// its share of the month's worth in against's place; on a phone, its worth
// and against, or share.
func (y yearDrawn) monthColumns(f byMonthForm) []monthColumn {
	third := monthColumn{head: "against", of: monthAgainst}
	if y.ofVersion() {
		third = monthColumn{head: "share", of: func(m views.MonthRow) line { return shareSaid(m.Share, secondaryInk) }, changes: true}
	}
	worth := monthColumn{head: "worth", of: func(m views.MonthRow) line { return worthSaid(m.Worth, secondaryInk) }, changes: true}
	if f.phone {
		worth.x, third.x = byMonthPhoneWorthX, byMonthPhoneThirdX
		return []monthColumn{worth, third}
	}
	worth.x, third.x = byMonthWorthX, byMonthThirdX
	columns := []monthColumn{{head: "requests", x: yearGridX, of: monthRequests, changes: true}, worth, third}
	if !y.ofVersion() {
		columns = append(columns, monthColumn{head: "limits", x: byMonthLimitsX, of: monthLimits, changes: true})
	}
	return columns
}

// byMonth draws BY MONTH under the Year's top, where placed puts its rows:
// its heading, wide with what it shows after it; its heads, and over its
// bars, by model, or a version's worth; then a row a month, oldest first,
// the first with its year, wide, and filtered to a version, a row before
// them for the months before it was used.
func (y yearDrawn) byMonth(c *canvas, top int) {
	f, at := y.form, y.placed()
	c.line(0, top+f.row, groupHead("BY MONTH", y.byMonthSays(), f.phone).fit(y.Width))
	columns := y.monthColumns(f)
	for i := range columns {
		c.line(columns[i].x, top+at.heads, line{{columns[i].head, dimInk}}.fit(y.columnCells(columns, i)))
	}
	c.line(f.barX, top+at.heads, line{{y.barHead(), dimInk}}.fit(y.Width-f.barX))
	if at.before {
		monthsBefore(c, top+at.heads+1, *y.page.Before, y.Width)
	}
	largest := largestWorth(y.page.ByMonth)
	for i, m := range at.months {
		y.month(c, top+at.first+i, m, i == 0 && !f.phone && !at.before, columns, f, largest)
	}
}

// barHead is the head over BY MONTH's bars: by model, or a version's worth.
func (y yearDrawn) barHead() string {
	if y.ofVersion() {
		return "worth"
	}
	return "by model"
}

// byMonthSays is what BY MONTH shows, after its heading: the worth and what
// the plans cost, or a version's worth and its share.
func (y yearDrawn) byMonthSays() string {
	if y.ofVersion() {
		return y.page.Version.Name + " only: its worth at API prices, and its share of the month"
	}
	return "worth at API prices; against what the plans cost that month"
}

// month draws a month's row along row: its name, bold for the month so far;
// its columns, those a look can change numbered in the month so far, each a
// figure fitParts fits a cell short of the next, or of the bars; its bar,
// against the largest month's worth; and, wide, two cells after it, so far,
// or from the day the ledger began, or the version was first used.
func (y yearDrawn) month(c *canvas, row int, m views.MonthRow, withYear bool, columns []monthColumn, f byMonthForm, largest ledger.Picodollars) {
	name := secondaryInk
	if m.SoFar {
		name = strongInk
	}
	c.line(f.nameX, row, fitParts(min(columns[0].x-1, y.Width)-f.nameX, figureIn(monthName(m.Month.Month, withYear), name)))
	for i, col := range columns {
		var n Number
		if m.SoFar && col.changes {
			n = y.number(m.Month.Month + " " + col.head)
		}
		c.line(col.x, row, fitParts(y.columnCells(columns, i), figurePart(col.of(m), n)))
	}
	cells := min(f.barCells, y.Width-f.barX)
	if cells < 1 {
		return
	}
	end := c.line(f.barX, row, y.monthBar(m, largest, cells))
	if note := monthNote(m); note != nil && !f.phone {
		c.line(end+2, row, fitParts(y.Width-end-2, note...))
	}
}

// columnCells is how many cells BY MONTH's i-th column of those given has:
// to a cell before the next, or before the bars, and never past the
// frame's width.
func (y yearDrawn) columnCells(columns []monthColumn, i int) int {
	next := y.form.barX
	if i+1 < len(columns) {
		next = columns[i+1].x
	}
	return min(next-1, y.Width) - columns[i].x
}

// monthRequests says a month's requests.
func monthRequests(m views.MonthRow) line {
	return line{{prose.Grouped(m.Requests), secondaryInk}}
}

// monthAgainst says a month's worth against what the plans cost it, as
// againstSaid does: a dash for the month so far.
func monthAgainst(m views.MonthRow) line {
	if m.SoFar {
		return line{noFigure}
	}
	return againstSaid(m.Ratio)
}

// monthLimits says how many limits were hit in a month, in
// state.destructive: a faint 0 for none.
func monthLimits(m views.MonthRow) line {
	n := 0
	for _, hit := range m.Limits {
		n += hit
	}
	if n == 0 {
		return line{{"0", faintInk}}
	}
	return line{{prose.Grouped(n), errorInk}}
}

// monthNote is what follows a part month's bar, in text.faint: from the day
// it's counted from, as from the 16th, the day a figure, or so far; none for
// a whole month.
func monthNote(m views.MonthRow) []part {
	if day, ok := civilDay(m.From); ok {
		return []part{wordsIn("from the ", faintInk), figureIn(ordinal(day.Day()), faintInk)}
	}
	if m.SoFar {
		return []part{wordsIn("so far", faintInk)}
	}
	return nil
}

// monthsBefore draws, along row, the row for the months before a version
// was first used, faint, as beforeSaid says it, cut to width cells: their
// span a figure, short of what follows it.
func monthsBefore(c *canvas, row int, b views.MonthsBefore, width int) {
	months, then := beforeSaid(b)
	c.line(byMonthNameX, row, fitParts(min(byMonthWorthX-1, width)-byMonthNameX, figureIn(months, faintInk)))
	c.line(byMonthWorthX, row, line{{then, faintInk}}.fit(width-byMonthWorthX))
}

// beforeSaid is what the row for the months before a version was first used
// says: their span, as Oct 2025 – May, and the version of its family used
// then, as none: Opus 5 then, or none.
func beforeSaid(b views.MonthsBefore) (months, then string) {
	months = monthName(b.From, true)
	if b.To != b.From {
		months += " – " + monthName(b.To, false)
	}
	then = "none"
	if b.Then != "" {
		then += ": " + b.Then + " then"
	}
	return months, then
}

// monthBar is the month's bar, as many cells of most as its worth is of the
// largest month's, each ▅: in the version's shade, filtered to one, else
// split by family, each in its family's colour.
func (y yearDrawn) monthBar(m views.MonthRow, largest ledger.Picodollars, most int) line {
	cells := barCells(amountOf(m.Worth), largest, most)
	if y.ofVersion() {
		return line{{strings.Repeat("▅", cells), versionHue(y.page.Version).ink()}}
	}
	return familySplit(m.ByFamily, cells)
}

// barCells is how many cells of most a worth of w takes against the largest,
// a half rounded to the even, as the frames have it: none of none, else at
// least one.
func barCells(w, largest ledger.Picodollars, most int) int {
	if w <= 0 || largest <= 0 {
		return 0
	}
	return max(1, int(math.RoundToEven(float64(most)*float64(w)/float64(largest))))
}

// familySplit splits cells cells among the families by their worth, in their
// order, each family's run ending where its share, and those before it, of
// the whole rounds to, so the runs fill the cells: a family worth too little
// for a cell has none.
func familySplit(families []views.FamilyWorth, cells int) line {
	var whole ledger.Picodollars
	for _, f := range families {
		whole += amountOf(f.Worth)
	}
	var bar line
	var sum ledger.Picodollars
	end := 0
	for _, f := range families {
		sum += amountOf(f.Worth)
		next := int(math.RoundToEven(float64(cells) * float64(sum) / float64(whole)))
		if next > end {
			bar = append(bar, span{strings.Repeat("▅", next-end), familyHue(f.Family).ink()})
		}
		end = next
	}
	return bar
}

// largestWorth is the most any of the months was worth.
func largestWorth(months []views.MonthRow) ledger.Picodollars {
	var largest ledger.Picodollars
	for _, m := range months {
		largest = max(largest, amountOf(m.Worth))
	}
	return largest
}

// amountOf is what a worth's priced requests cost: none where none was
// priced.
func amountOf(w views.Worth) ledger.Picodollars {
	if w.Amount == nil {
		return 0
	}
	return *w.Amount
}

// dots draws the bars' key along row from x, cut at the frame's width.
func (y yearDrawn) dots(c *canvas, x, row int) {
	c.line(x, row, y.familyDots().fit(y.Width-x))
}

// familyDots is the bars' key: each family's dot in its colour, and its
// name.
func (y yearDrawn) familyDots() line {
	var l line
	for i, family := range y.page.Families {
		if i > 0 {
			l = append(l, span{"   ", dimInk})
		}
		l = append(l, familyDot(family, dimInk)...)
	}
	return l
}

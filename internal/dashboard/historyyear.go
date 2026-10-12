package dashboard

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
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

// aDollar is a dollar, as the ledger counts money.
const aDollar ledger.Picodollars = 1_000_000_000_000

var (
	// boldPositiveInk is against its price at 1× or more; alertInk under.
	boldPositiveInk = ink{token: theme.StatePositive, bold: true}
	// noFigure is a figure that isn't known, or isn't yet.
	noFigure = span{"—", faintInk}
)

// historyYear draws the Year from row top, once its page is built: on a
// phone, as phone lays it out.
func (f Frame) historyYear(c *canvas, top int) {
	page := f.History.Year
	if page == nil {
		return
	}
	y := yearDrawn{Frame: f, page: *page, ask: f.History.YearAsk}
	if f.phone() {
		y.phone(c, top)
		return
	}
	y.grid(c, yearGridX, top, page.Weeks, 0, dayAbbreviation)
	y.key(c, top+yearKeyRow, keyWide)
	y.heading(c, yearNumbersX, top)
	for i, figure := range y.figures() {
		c.line(yearNumbersX, top+1+i, slices.Concat(line{{padded(figure.label, yearLabelCells), dimInk}}, figure.value))
	}
	last := y.byMonth(c, top+yearByMonthRow, byMonthWide)
	y.dots(c, byMonthBarX, last+2)
}

// phone draws the Year as a phone has it: the latest weeks that fit, under
// their months' names; the key cut short; the numbers, the first six, in two
// columns of three; and BY MONTH, the last six months, its heads on its own
// row.
func (y yearDrawn) phone(c *canvas, top int) {
	weeks := y.page.Weeks
	first := max(len(weeks)-yearPhoneWeeks, 0)
	y.grid(c, yearPhoneGridX, top, weeks[first:], first, dayInitial)
	y.key(c, top+yearKeyRow, keyPhone)
	y.heading(c, 0, top+yearPhoneNumbersRow)
	figures := y.figures()
	for i, figure := range figures[:min(len(figures), 2*yearPhoneFigures)] {
		x, row := 0, top+yearPhoneNumbersRow+1+i
		if i >= yearPhoneFigures {
			x, row = yearPhoneSecondX, row-yearPhoneFigures
		}
		c.line(x, row, slices.Concat(line{{padded(figure.short, yearPhoneLabelCells), dimInk}}, figure.value))
	}
	y.byMonth(c, top+yearPhoneByMonthRow, byMonthPhone)
}

// yearDrawn is the Year as the frame draws it: its page, and what it was
// asked to show.
type yearDrawn struct {
	Frame
	page views.YearPage
	ask  views.YearAsk
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
		c.text(x+2*(l.Week-first), top, monthName(l.Month, false), k)
	}
	if len(weeks) == 0 {
		return
	}
	if start, ok := civilDay(weeks[0].First); ok {
		for _, i := range []int{0, 2, 4} {
			c.text(0, top+1+i, name(start.AddDate(0, 0, i).Weekday()), dimInk)
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

// square is a day at its level, as the grid and the key draw it: ■ in its
// shade, or, filtered to a version, in its step of the version's shade; or,
// in a look that draws no shades of its own, its level's glyph.
func (y yearDrawn) square(level int) span {
	k := dayInk(level)
	if y.ofVersion() {
		k = stepInk(versionHue(y.page.Version), level)
	}
	if !y.Look.blends() {
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
// shade's square and its bounds in requests a day, then what they count, cut
// at the frame's edge.
func (y yearDrawn) key(c *canvas, row int, form keyForm) {
	var l line
	for level, bound := range form.bounds(y.page.KeyCuts) {
		if level > 0 {
			l = append(l, span{form.gap, dimInk})
		}
		l = append(l, y.square(level), span{" " + bound, dimInk})
	}
	l = append(l, span{"  " + y.unit("requests a day"), faintInk})
	c.line(form.x, row, l.fit(y.Width-form.x))
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

// shown is the use shown alone, the version's name or the account's id: ""
// for all use.
func (y yearDrawn) shown() string {
	return cmp.Or(y.ask.Version, y.ask.Account)
}

// unit is what the Year counts, the use shown named before it: as requests
// a day, Opus 5.5 requests a day, or work requests a day.
func (y yearDrawn) unit(what string) string {
	return strings.TrimSpace(y.shown() + " " + what)
}

// heading draws the numbers' heading at x along row row: the period, LAST 12
// MONTHS or the year, as 2025; or after the use shown, OPUS 5.5 · 12 MONTHS
// or WORK · 2025.
func (y yearDrawn) heading(c *canvas, x, row int) {
	period := "12 MONTHS"
	if y.ask.Year != 0 {
		period = strconv.Itoa(y.ask.Year)
	}
	text := period
	switch {
	case y.shown() != "":
		text = strings.ToUpper(y.shown()) + " · " + period
	case y.ask.Year == 0:
		text = "LAST " + period
	}
	c.text(x, row, text, dimInk)
}

// yearFigure is a number beside the grid: its label, and a phone's, what it
// says, and whether a look can change it.
type yearFigure struct {
	label, short string
	value        line
	changes      bool
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
		{"requests", "requests", line{{prose.Grouped(n.Requests), secondaryInk}}, true},
		{"sessions", "sessions", line{{prose.Grouped(n.Sessions), secondaryInk}}, true},
	}
	if y.ofVersion() {
		figures = append(figures,
			yearFigure{"worth", "worth", worthSaid(n.Worth), true},
			yearFigure{"of all use", "share", shareSaid(n.Share), true},
			yearFigure{"first used", "first", daySaid(n.FirstUsed), true},
			yearFigure{"busiest day", "busiest", daySaid(n.BusiestDay), true},
		)
	} else {
		figures = append(figures,
			yearFigure{"limits hit", "limits", y.limitsSaid(n.Limits), true},
			yearFigure{"worth", "worth", worthSaid(n.Worth), true},
			yearFigure{"plans cost", "plans", costSaid(n.Cost), false},
			yearFigure{"against it", "against", againstSaid(n.Ratio), true},
			yearFigure{"busiest day", "busiest", daySaid(n.BusiestDay), true},
			yearFigure{"longest run", "run", runSaid(n.LongestRun), true},
		)
	}
	for i, figure := range figures {
		if n.Requests == 0 {
			figures[i].value = line{noFigure}
		}
		if figure.changes {
			figures[i].value = y.numbered(figure.label, figures[i].value)
		}
	}
	return figures
}

// numbered is the figure l says, named of, numbered as the Year's.
func (y yearDrawn) numbered(of string, l line) line {
	return l.numbered(Number{Kind: NumberYear, Account: y.ask.Account, Of: strings.TrimSpace(y.ask.Version + " " + of)})
}

// limitsSaid says the limits hit, by window, in state.destructive, those of
// more than one window joined by a faint ·, as "5-hour ×402 · week ×3": none
// where none was.
func (y yearDrawn) limitsSaid(limits map[string]int) line {
	if len(limits) == 0 {
		return line{{"none", secondaryInk}}
	}
	var l line
	for i, key := range slices.SortedFunc(maps.Keys(limits), quota.CompareKeys) {
		if i > 0 {
			l = append(l, span{" · ", faintInk})
		}
		l = append(l, span{y.windowName(key) + " ×" + prose.Grouped(limits[key]), errorInk})
	}
	return l
}

// windowName names a window by its key as a sentence has it, as the frame's
// WindowName names it, else by its length: "5-hour" or "week".
func (y yearDrawn) windowName(key string) string {
	if y.WindowName != nil {
		return status.InProse(y.WindowName(key))
	}
	return status.InProse(nameOfWindow(key, ""))
}

// monthLayout lays out a month as views names it, as 2026-10.
const monthLayout = "2006-01"

// monthName is the month, as views names it, as the Year names it: Oct, or
// with its year, Oct 2025.
func monthName(month string, withYear bool) string {
	m, err := time.Parse(monthLayout, month)
	if err != nil {
		return month
	}
	if withYear {
		return m.Format("Jan 2006")
	}
	return m.Format("Jan")
}

// civilDay is the day with the given date, reporting false for one that
// isn't.
func civilDay(date string) (time.Time, bool) {
	day, err := time.Parse(time.DateOnly, date)
	return day, err == nil
}

// dayAbbreviation and dayInitial name a weekday as the grid does, wide and on
// a phone: Mon, M.
func dayAbbreviation(d time.Weekday) string { return d.String()[:3] }
func dayInitial(d time.Weekday) string      { return d.String()[:1] }

// worthSaid is a worth as History writes it, in text.secondary: to the
// dollar, its thousands set apart, as "$27,480"; a faint + after it where
// part of it is unpriced, as "$184+"; unpriced where all of it is; and a
// dash for none.
func worthSaid(w views.Worth) line {
	switch {
	case w.Amount == nil && len(w.Unpriced) > 0:
		return line{{"unpriced", secondaryInk}}
	case w.Amount == nil:
		return line{noFigure}
	case len(w.Unpriced) > 0:
		return line{{dollarsSaid(*w.Amount), secondaryInk}, {"+", faintInk}}
	}
	return line{{dollarsSaid(*w.Amount), secondaryInk}}
}

// costSaid is what the plans cost, as worthSaid writes a priced worth: a
// dash where no plan is known.
func costSaid(cost *ledger.Picodollars) line {
	if cost == nil {
		return line{noFigure}
	}
	return line{{dollarsSaid(*cost), secondaryInk}}
}

// dollarsSaid writes an amount to the dollar, its thousands set apart, as
// "$27,480".
func dollarsSaid(p ledger.Picodollars) string {
	return "$" + prose.Grouped(int((p+aDollar/2)/aDollar))
}

// againstSaid is a worth against its price, as "3.8×", bold, in
// state.positive at 1× or more as it's written, and accent.attention under: a
// dash where there's none.
func againstSaid(ratio *float64) line {
	if ratio == nil {
		return line{noFigure}
	}
	shown := math.Round(*ratio*10) / 10
	k := boldPositiveInk
	if shown < 1 {
		k = alertInk
	}
	return line{{fmt.Sprintf("%.1f×", shown), k}}
}

// shareSaid is a share as a whole percentage, as "21%": a dash where it isn't
// known.
func shareSaid(share *float64) line {
	if share == nil {
		return line{noFigure}
	}
	return line{{status.Percent(*share), secondaryInk}}
}

// daySaid names the day with the given date, as "Tue 22 Sep": a dash for
// none.
func daySaid(date string) line {
	day, ok := civilDay(date)
	if !ok {
		return line{noFigure}
	}
	return line{{day.Format("Mon 2 Jan"), secondaryInk}}
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
// its own row, its last months alone and none of its notes; the column its
// months' names are at, and its bars', the most cells a bar takes, and how
// many of the last months it shows, 0 for all.
type byMonthForm struct {
	phone                 bool
	nameX, barX, barCells int
	months                int
}

// BY MONTH's forms, wide and on a phone.
var (
	byMonthWide  = byMonthForm{nameX: byMonthNameX, barX: byMonthBarX, barCells: byMonthBarCells}
	byMonthPhone = byMonthForm{phone: true, nameX: byMonthPhoneNameX, barX: byMonthPhoneBarX, barCells: byMonthPhoneBarCells, months: byMonthPhoneMonths}
)

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
		third = monthColumn{head: "share", of: func(m views.MonthRow) line { return shareSaid(m.Share) }, changes: true}
	}
	worth := monthColumn{head: "worth", of: func(m views.MonthRow) line { return worthSaid(m.Worth) }, changes: true}
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

// byMonth draws BY MONTH from row row, as the form lays it out, and returns
// the row of its last month: its heading, wide with what it shows after it;
// its heads, and over its bars, by model, or a version's worth; then a row a
// month, oldest first, the first with its year, wide, and filtered to a
// version, a row before them for the months before it was used.
func (y yearDrawn) byMonth(c *canvas, row int, f byMonthForm) int {
	heads := row + 1
	if f.phone {
		heads = row
	}
	c.text(0, row, "BY MONTH", dimInk)
	if !f.phone {
		c.text(len("BY MONTH")+2, row, y.byMonthSays(), faintInk)
	}
	columns := y.monthColumns(f)
	for _, col := range columns {
		c.text(col.x, heads, col.head, dimInk)
	}
	barHead := "by model"
	if y.ofVersion() {
		barHead = "worth"
	}
	c.text(f.barX, heads, barHead, dimInk)
	at, withYear := heads+1, !f.phone
	if b := y.page.Before; b != nil && !f.phone {
		monthsBefore(c, at, *b)
		at, withYear = at+1, false
	}
	rows := y.page.ByMonth
	if f.months > 0 {
		rows = rows[max(len(rows)-f.months, 0):]
	}
	largest := largestWorth(y.page.ByMonth)
	for i, m := range rows {
		y.month(c, at+i, m, withYear && i == 0, columns, f, largest)
	}
	return at + len(rows) - 1
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
// its columns, those a look can change numbered in the month so far; its
// bar, against the largest month's worth; and, wide, two cells after it, so
// far, or from the day the ledger began, or the version was first used.
func (y yearDrawn) month(c *canvas, row int, m views.MonthRow, withYear bool, columns []monthColumn, f byMonthForm, largest ledger.Picodollars) {
	name := secondaryInk
	if m.SoFar {
		name = strongInk
	}
	c.text(f.nameX, row, monthName(m.Month.Month, withYear), name)
	for _, col := range columns {
		said := col.of(m)
		if m.SoFar && col.changes {
			said = y.numbered(m.Month.Month+" "+col.head, said)
		}
		c.line(col.x, row, said)
	}
	end := c.line(f.barX, row, y.monthBar(m, largest, f.barCells))
	if note := monthNote(m); note != "" && !f.phone {
		c.text(end+2, row, note, faintInk)
	}
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

// monthNote is what follows a part month's bar: from the day it's counted
// from, as from the 16th, or so far.
func monthNote(m views.MonthRow) string {
	if day, ok := civilDay(m.From); ok {
		return "from the " + ordinal(day.Day())
	}
	if m.SoFar {
		return "so far"
	}
	return ""
}

// monthsBefore draws, along row, the row for the months before a version
// was first used, faint: their span, as Oct 2025 – May, and the version of
// its family used then, as none: Opus 5 then, or none.
func monthsBefore(c *canvas, row int, b views.MonthsBefore) {
	months := monthName(b.From, true)
	if b.To != b.From {
		months += " – " + monthName(b.To, false)
	}
	c.text(byMonthNameX, row, months, faintInk)
	then := "none"
	if b.Then != "" {
		then += ": " + b.Then + " then"
	}
	c.text(byMonthWorthX, row, then, faintInk)
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

// dots draws the bars' key along row from x: each family's dot in its
// colour, and its name.
func (y yearDrawn) dots(c *canvas, x, row int) {
	var l line
	for i, family := range y.page.Families {
		if i > 0 {
			l = append(l, span{"   ", dimInk})
		}
		l = append(l, familyDot(family, dimInk)...)
	}
	c.line(x, row, l)
}

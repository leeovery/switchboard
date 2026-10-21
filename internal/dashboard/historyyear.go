package dashboard

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The Year's parts, as the design lays them out, wide and on a phone.
const (
	// yearGridX is the grid's first column, and the wide key's; a phone's is
	// yearPhoneGridX. The grid shows yearWeeks at most, two cells a week.
	yearGridX      = 14
	yearPhoneGridX = 4
	yearWeeks      = 53
	// The numbers' labels are yearLabelCells wide, or on a phone
	// yearPhoneLabelCells. Beside the grid, they're yearNumbersGap cells after
	// the whole year's weeks; under the key, in columns of yearFiguresDown,
	// yearColumnsApart cells apart.
	yearLabelCells      = 13
	yearPhoneLabelCells = 10
	yearNumbersGap      = 6
	yearFiguresDown     = 3
	yearColumnsApart    = 4
	// The rows under the view's top the key is on, and the numbers' heading
	// under it, or where they're beside the grid, BY MONTH's.
	yearKeyRow     = 9
	yearNumbersRow = 11
	// byMonthNameX is the column a month's name is at, and byMonthBarX its
	// bar's; a phone's are byMonthPhoneNameX and byMonthPhoneBarX. A bar takes
	// byMonthBarMost cells at most, the house's most for a bar beside words,
	// and a part month's note follows it byMonthNoteGap cells on.
	byMonthNameX      = 2
	byMonthBarX       = 60
	byMonthPhoneNameX = 1
	byMonthPhoneBarX  = 32
	byMonthBarMost    = 40
	byMonthNoteGap    = 2
)

// yearForm is a form the Year is drawn in: wide, as the design lays it out,
// or as a phone lays it out, its words shorter. Its grid's column and its
// days' names; its key's form; its numbers' labels' cells, a phone's labels
// short; and BY MONTH's heads on its heading's row, as a phone has them,
// and its months' names' column and its bars'.
type yearForm struct {
	phone       bool
	gridX       int
	dayName     func(time.Weekday) string
	key         keyForm
	labelCells  int
	nameX, barX int
}

// The Year's forms, wide and on a phone.
var (
	yearWide = yearForm{
		gridX: yearGridX, dayName: prose.Weekday, key: keyWide, labelCells: yearLabelCells,
		nameX: byMonthNameX, barX: byMonthBarX,
	}
	yearPhone = yearForm{
		phone: true, gridX: yearPhoneGridX, dayName: prose.WeekdayInitial, key: keyPhone, labelCells: yearPhoneLabelCells,
		nameX: byMonthPhoneNameX, barX: byMonthPhoneBarX,
	}
)

// label is the figure's label in the form: a phone's short.
func (f yearForm) label(figure yearFigure) string {
	if f.phone {
		return figure.short
	}
	return figure.label
}

// yearLaid is where the Year's parts lie in the frame: how many of the
// latest weeks the grid shows; the numbers' heading's column and row, their
// columns, and whether they're beside the grid; where BY MONTH's rows are;
// and the most cells its bars take.
type yearLaid struct {
	weeks                int
	numbersX, numbersRow int
	columns              []figureColumn
	beside               bool
	byMonth              byMonthPlaced
	bar                  int
}

// figureColumn is a column of the numbers: its figures, a row each from
// row, at column x, in cells cells.
type figureColumn struct {
	figures       []yearFigure
	x, row, cells int
}

// yearDrawn is the Year as the frame draws it: its page, what it was asked
// to show, its form and where its parts lie.
type yearDrawn struct {
	Frame
	page views.YearPage
	ask  views.YearAsk
	form yearForm
	laid yearLaid
}

// yearDrawn is the Year as the frame draws it, wide where the wide form's
// parts fit, else as a phone lays it out, reporting false before its page is
// built.
func (f Frame) yearDrawn() (yearDrawn, bool) {
	page := f.History.Year
	if page == nil {
		return yearDrawn{}, false
	}
	y := yearDrawn{Frame: f, page: *page, ask: f.History.YearAsk, form: yearWide}
	if !y.wideFits() {
		y.form = yearPhone
	}
	return y.laidOut(), true
}

// laidOut is the Year with its parts laid out in the frame, in its form.
func (y yearDrawn) laidOut() yearDrawn {
	y.laid = y.layOut()
	return y
}

// wideFits reports whether the wide form's parts fit the frame, as flex
// lays them: its key whole after the days' names, the widest of its rows
// that never narrow. So the form never hangs on the data, the key is
// reckoned with every cut at reckonedCut, and its unit without the use
// shown, which may name an account by its label; a key wider than that is
// cut at the frame's width. The grid shows the weeks that fit, the numbers'
// columns narrow and leave, and BY MONTH's bars take what its row leaves.
func (y yearDrawn) wideFits() bool {
	key := partsWidth(y.keyParts(keyWide, []int{reckonedCut, reckonedCut, reckonedCut}, "requests a day"))
	_, fits := flex.Lay(y.Width, flex.Part{Least: yearGridX, Natural: yearGridX}, flex.Part{Least: key, Natural: key})
	return fits
}

// barParts are a BY MONTH row's parts across the frame, in the form given:
// its columns to the bars; its bar, a cell at least, which grows to its
// most; and where a note of note cells follows it, the note, which leaves
// where it doesn't fit beside a bar's least.
func barParts(f yearForm, note int) []flex.Part {
	parts := []flex.Part{
		{Least: f.barX, Natural: f.barX},
		{Least: 1, Natural: 1, Most: byMonthBarMost, Grows: true},
	}
	if note > 0 {
		parts = append(parts, flex.Part{Least: note, Natural: note, Gap: byMonthNoteGap, Leaves: 1})
	}
	return parts
}

// layOut lays the Year's parts out in the frame, in its form: the weeks
// that fit; the numbers beside the grid where the whole year and they fit
// there, else under the key, and BY MONTH under them; and BY MONTH's bars.
func (y yearDrawn) layOut() yearLaid {
	l := yearLaid{weeks: y.weeksShown(), bar: y.barCells()}
	if x, beside := y.besideAt(); beside {
		l.numbersX, l.beside = x, true
		l.columns = []figureColumn{{figures: y.figures(), x: x, row: 1, cells: y.Width - x}}
	} else {
		l.numbersRow, l.columns = yearNumbersRow, y.figureColumns()
	}
	l.byMonth = y.placed(l.beside)
	return l
}

// weeksShown is how many of the latest weeks the grid shows, at most all
// the page's: as many as end, with their months' names over them, short of
// the frame's margin.
func (y yearDrawn) weeksShown() int {
	for w := len(y.page.Weeks); w > 0; w-- {
		if y.gridReach(w) <= y.edge() {
			return w
		}
	}
	return 0
}

// gridReach is the column the grid of the latest weeks given ends before:
// its last week's, or the name of a month over them.
func (y yearDrawn) gridReach(weeks int) int {
	x, first := y.form.gridX, len(y.page.Weeks)-weeks
	end := x + 2*(weeks-1) + 1
	for _, l := range y.page.Months {
		if l.Week >= first {
			end = max(end, x+2*(l.Week-first)+ansi.StringWidth(monthName(l.Month, false)))
		}
	}
	return end
}

// besideAt is the column the numbers start at beside the grid, and whether
// they fit there, as flex lays them after the days' names and the whole
// year's weeks: only wide, and only where every figure fits whole, as
// columnPart reckons it, so a wider frame never shows less of them than a
// narrower one does under the key.
func (y yearDrawn) besideAt() (int, bool) {
	if y.form.phone {
		return 0, false
	}
	numbers := y.columnPart(y.figures()).Natural
	s, fits := flex.Lay(y.Width,
		flex.Part{Least: yearGridX, Natural: yearGridX},
		flex.Part{Least: 2 * yearWeeks, Natural: 2 * yearWeeks},
		flex.Part{Least: numbers, Natural: numbers, Gap: yearNumbersGap},
	)
	return s[2].At, fits
}

// figureColumns are the numbers under the key, yearFiguresDown to a column,
// as flex lays them across the frame, each as wide as its widest figure
// whole. Where the room is short, a figure takes its shorter forms before
// any whole figure leaves: the second column narrows to its figures'
// shortest forms; then the columns past it leave, the last first; then the
// first narrows; then the second leaves.
func (y yearDrawn) figureColumns() []figureColumn {
	figures := slices.Collect(slices.Chunk(y.figures(), yearFiguresDown))
	n := len(figures)
	parts := make([]flex.Part, n)
	for i, column := range figures {
		parts[i] = y.columnPart(column)
		switch i {
		case 0:
			parts[i].Narrows = n
		case 1:
			parts[i].Narrows, parts[i].Leaves = 1, n+1
		default:
			parts[i].Leaves = n + 1 - i
		}
		if i > 0 {
			parts[i].Gap = yearColumnsApart
		}
	}
	s, _ := flex.Lay(y.Width, parts...)
	var columns []figureColumn
	for i, column := range figures {
		if !s[i].Left {
			columns = append(columns, figureColumn{figures: column, x: s[i].At, row: yearNumbersRow + 1, cells: min(s[i].Size, y.Width-s[i].At)})
		}
	}
	if n := len(columns); n > 0 {
		columns[n-1].cells = y.Width - columns[n-1].x
	}
	return columns
}

// columnPart is a column of the figures given as a part: least, their
// labels and their shortest forms; natural, their labels and all they say,
// a figure whose length is bounded at the widest its format draws, so the
// arrangement never hangs on it.
func (y yearDrawn) columnPart(figures []yearFigure) flex.Part {
	var p flex.Part
	for _, figure := range figures {
		label := max(y.form.labelCells, ansi.StringWidth(y.form.label(figure)))
		shortest := figure.value
		if n := len(figure.shorter); n > 0 {
			shortest = figure.shorter[n-1]
		}
		p.Least = max(p.Least, label+shortest.width())
		p.Natural = max(p.Natural, label+max(figure.value.width(), figure.widest))
	}
	return p
}

// barCells is the most cells BY MONTH's bars take: what each month's row
// leaves after its columns, at most byMonthBarMost, never past the frame's
// width; and fewer where a part month's bar would leave its note no room
// beside it, so each bar stays its month's share of the largest month's.
func (y yearDrawn) barCells() int {
	most := y.barRoom(nil)
	largest := largestWorth(y.page.ByMonth)
	for _, m := range y.page.ByMonth {
		note := monthNote(m)
		if note == nil {
			continue
		}
		room := y.barRoom(note)
		for most > 0 && barCells(amountOf(m.Worth), largest, most) > room {
			most--
		}
	}
	return most
}

// barRoom is the cells a BY MONTH row leaves its bar, as flex lays it, its
// note after it where it has one, never past the frame's width.
func (y yearDrawn) barRoom(note []part) int {
	s, _ := flex.Lay(y.Width, barParts(y.form, partsWidth(note))...)
	return max(min(s[1].Size, y.Width-y.form.barX), 0)
}

// whole is the Year drawn whole on a canvas of its own, from its first row,
// as tall as height lays it out.
func (y yearDrawn) whole() *canvas {
	c := newCanvas(y.Width, y.height())
	y.draw(c, 0)
	return c
}

// draw draws the Year from row top in its form: the latest weeks that fit
// under their months' names; the key; the numbers, beside the grid or under
// the key; BY MONTH; and the bars' key under it.
func (y yearDrawn) draw(c *canvas, top int) {
	f, weeks := y.form, y.page.Weeks
	first := len(weeks) - y.laid.weeks
	y.grid(c, f.gridX, top, weeks[first:], first, f.dayName)
	y.key(c, top+yearKeyRow, f.key)
	y.numbers(c, top)
	y.byMonth(c, top)
	y.dots(c, f.barX, top+y.dotsRow())
}

// numbers draws the numbers' heading and their columns, where they're laid
// out, each figure labelled as the form labels it.
func (y yearDrawn) numbers(c *canvas, top int) {
	y.heading(c, y.laid.numbersX, top+y.laid.numbersRow)
	for _, column := range y.laid.columns {
		for i, figure := range column.figures {
			c.line(column.x, top+column.row+i, figure.drawn(y.form.label(figure), y.form.labelCells, column.cells))
		}
	}
}

// height is how many rows the Year takes under its top: to BY MONTH's last
// month, and to the bars' key under it, where there are bars to key.
func (y yearDrawn) height() int {
	if dots := len(y.familyDots()); dots > 0 {
		return y.dotsRow() + dots
	}
	return y.laid.byMonth.last() + 1
}

// dotsRow is the row under the Year's top the bars' key is on: two under BY
// MONTH's last month.
func (y yearDrawn) dotsRow() int {
	return y.laid.byMonth.last() + 2
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
	if start, ok := dayfile.Civil(weeks[0].First); ok {
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
	keyPhone = keyForm{gap: " ", number: prose.Thousands, none: "0", under: "<", over: ">"}
)

// key draws the shades' key along row row, as the form lays it out, its unit
// naming the use shown, cut at the frame's edge.
func (y yearDrawn) key(c *canvas, row int, form keyForm) {
	c.line(form.x, row, fitParts(y.Width-form.x, y.keyParts(form, y.page.KeyCuts, y.unit("requests a day"))...))
}

// reckonedCut is a cut of four figures, the widest the wide form's key is
// reckoned at.
const reckonedCut = 9_950

// keyParts are the shades' key as the form lays it out, of the cuts given:
// each shade's square and its bounds in requests a day, a figure, then what
// they count, unit.
func (y yearDrawn) keyParts(form keyForm, cuts []int, unit string) []part {
	var parts []part
	for level, bound := range form.bounds(cuts) {
		if level > 0 {
			parts = append(parts, wordsIn(form.gap, dimInk))
		}
		parts = append(parts, wordsPart(line{y.square(level), {" ", dimInk}}), figureIn(bound, dimInk))
	}
	return append(parts, wordsIn("  "+unit, faintInk))
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

// yearFigure is one of the Year's numbers: its label, and a phone's, what it
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

// figures are the Year's numbers, in order: of all use, or an
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
			againstFigure(n.Against),
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

// againstFigure is the worth against what the plans cost, as againstSaid
// says it, then, where some accounts' plans aren't known, that it's over
// those known, as knownForms says it, in text.subtle: "3.8× · 2 of 3 plans
// known"; its shorter forms that short, then compact, "3.8× · 2/3", then the
// ratio alone.
func againstFigure(a views.Against) yearFigure {
	figure := yearFigure{label: "against it", short: "against", value: againstSaid(a.Ratio), changes: true}
	if a.Ratio == nil {
		return figure
	}
	ratio := figure.value
	var forms []line
	for _, known := range knownForms(a.Plans) {
		forms = append(forms, append(slices.Clone(ratio), span{known, dimInk}))
	}
	if len(forms) > 0 {
		figure.value, figure.shorter = forms[0], append(forms[1:], ratio)
	}
	return figure
}

// number is the Year's number of the figure named of.
func (y yearDrawn) number(of string) Number {
	return Number{Kind: NumberYear, Account: y.ask.Account, Of: strings.TrimSpace(y.ask.Version + " " + of)}
}

// limitsFigure is the limits hit, by window, in state.destructive, those of
// more than one window joined by a faint ·, as "week ×3 · 5-hour ×412": none
// where none was. Its shorter forms leave off its last windows whole, one
// at a time, as "week ×3" where both don't fit.
func (y yearDrawn) limitsFigure(limits map[string]int) yearFigure {
	figure := yearFigure{label: "limits hit", short: "limits", value: line{{"none", secondaryInk}}, changes: true}
	if len(limits) == 0 {
		return figure
	}
	var forms []line
	for i, hit := range views.LimitsHit(limits, y.windowName) {
		var l line
		if i > 0 {
			l = append(slices.Clone(forms[i-1]), span{" · ", faintInk})
		}
		forms = append(forms, append(l, span{hit, errorInk}))
	}
	slices.Reverse(forms)
	figure.value, figure.shorter = forms[0], forms[1:]
	return figure
}

// windowName names a window by its key, as the frame's WindowName names it,
// else as status.NameOfKey does: "5-hour" or "Week".
func (y yearDrawn) windowName(key string) string {
	if y.WindowName != nil {
		return y.WindowName(key)
	}
	return status.NameOfKey(key)
}

// costSaid is what the plans cost, as worthSaid writes a priced worth: a
// dash where no plan is known.
func costSaid(cost *ledger.Picodollars) line {
	if cost == nil {
		return line{noFigure}
	}
	return line{{cost.GroupedDollars(), secondaryInk}}
}

// daySaid names the day with the given date, as views.DateNamed does, as
// "Tue 22 Sep": a dash for none.
func daySaid(date string) line {
	return figureOr(views.DateNamed(date))
}

// runSaid says how many days in a row there were, as views.DaysInARow does,
// as "58 days": a dash for none.
func runSaid(days int) line {
	return figureOr(views.DaysInARow(days))
}

// figureOr is a figure of the year's, said, in text.secondary: a dash where
// it says nothing.
func figureOr(said string) line {
	if said == "" {
		return line{noFigure}
	}
	return line{{said, secondaryInk}}
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

// byMonthPlaced is where BY MONTH's rows are under the Year's top: its
// heading's; its heads', on a phone its heading's, else the one under it;
// whether the row under them is the one for the months before a version was
// first used, as it is wide where there are any; and its months, oldest
// first, a row each from first.
type byMonthPlaced struct {
	heading, heads int
	before         bool
	first          int
	months         []views.MonthRow
}

// placed is where BY MONTH's rows are under the Year's top, as its form lays
// them out: under the key where the numbers are beside the grid, else under
// them.
func (y yearDrawn) placed(beside bool) byMonthPlaced {
	f := y.form
	p := byMonthPlaced{heading: yearNumbersRow, before: y.page.Before != nil && !f.phone, months: y.page.ByMonth}
	if !beside {
		p.heading += yearFiguresDown + 2
	}
	if p.heads = p.heading; !f.phone {
		p.heads++
	}
	if p.first = p.heads + 1; p.before {
		p.first++
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
func (y yearDrawn) monthColumns() []monthColumn {
	third := monthColumn{head: "against", of: monthAgainst}
	if y.ofVersion() {
		third = monthColumn{head: "share", of: func(m views.MonthRow) line { return shareSaid(m.Share, secondaryInk) }, changes: true}
	}
	worth := monthColumn{head: "worth", of: func(m views.MonthRow) line { return worthSaid(m.Worth, secondaryInk) }, changes: true}
	if y.form.phone {
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

// byMonth draws BY MONTH under the Year's top, where it's placed: its
// heading, wide with what it shows after it, as byMonthHead says it; its
// heads, and over its bars, by model, or a version's worth; then a row a
// month, oldest first, the first with its year, and filtered to a version,
// wide, a row before them for the months before it was used.
func (y yearDrawn) byMonth(c *canvas, top int) {
	f, at := y.form, y.laid.byMonth
	c.line(0, top+at.heading, y.byMonthHead(f.phone).fit(y.Width))
	columns := y.monthColumns()
	for i := range columns {
		c.line(columns[i].x, top+at.heads, line{{columns[i].head, dimInk}}.fit(y.columnCells(columns, i)))
	}
	c.line(f.barX, top+at.heads, line{{y.barHead(), dimInk}}.fit(y.Width-f.barX))
	if at.before {
		monthsBefore(c, top+at.heads+1, *y.page.Before, y.Width)
	}
	largest := largestWorth(y.page.ByMonth)
	for i, m := range at.months {
		y.month(c, top+at.first+i, m, i == 0 && !at.before, columns, largest)
	}
}

// barHead is the head over BY MONTH's bars: by model, or a version's worth.
func (y yearDrawn) barHead() string {
	if y.ofVersion() {
		return "worth"
	}
	return "by model"
}

// byMonthHead is BY MONTH's heading, as groupHead says it, and wide, of
// every account's use where some plans aren't known, that each month's
// against is over those known, once for the table, in the longest of
// knownForms' forms that fits the frame, else none: "... the plans cost
// that month · 2 of 3 plans known", "· 2 of 3 plans" or "· 2/3". The form
// is chosen without those words, so it never hangs on the plans.
func (y yearDrawn) byMonthHead(phone bool) line {
	head := groupHead("BY MONTH", y.byMonthSays(), phone)
	if phone || y.ofVersion() {
		return head
	}
	for _, known := range knownForms(y.page.Numbers.Plans) {
		if head.width()+ansi.StringWidth(known) <= y.Width {
			return append(head, span{known, faintInk})
		}
	}
	return head
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
// against the largest month's worth, in the cells laid out for the bars; and
// two cells after it, so far, or from the day the ledger began, or the
// version was first used, in what's left.
func (y yearDrawn) month(c *canvas, row int, m views.MonthRow, withYear bool, columns []monthColumn, largest ledger.Picodollars) {
	f := y.form
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
	if y.laid.bar < 1 {
		return
	}
	end := c.line(f.barX, row, y.monthBar(m, largest, y.laid.bar))
	if note := monthNote(m); note != nil {
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
	n := m.LimitCount()
	if n == 0 {
		return line{{"0", faintInk}}
	}
	return line{{prose.Grouped(n), errorInk}}
}

// monthNote is what follows a part month's bar, in text.faint: from the day
// it's counted from, as from the 16th, the day a figure, or so far; none for
// a whole month.
func monthNote(m views.MonthRow) []part {
	if day, ok := dayfile.Civil(m.From); ok {
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
// order, as splitCells splits them: a family worth too little for a cell has
// none.
func familySplit(families []views.FamilyWorth, cells int) line {
	amounts := make([]ledger.Picodollars, len(families))
	for i, f := range families {
		amounts[i] = amountOf(f.Worth)
	}
	var bar line
	for i, n := range splitCells(amounts, cells) {
		if n > 0 {
			bar = append(bar, span{strings.Repeat("▅", n), familyHue(families[i].Family).ink()})
		}
	}
	return bar
}

// splitCells splits cells cells among parts worth the amounts given, in
// their order, each part's run ending where its share, and those before it,
// of the whole rounds to, a half to the even one, so the runs fill the cells:
// a part worth too little for a cell has none, and none has any where the
// whole is worth nothing.
func splitCells(amounts []ledger.Picodollars, cells int) []int {
	var whole ledger.Picodollars
	for _, a := range amounts {
		whole += a
	}
	runs := make([]int, len(amounts))
	if whole <= 0 {
		return runs
	}
	var sum ledger.Picodollars
	end := 0
	for i, a := range amounts {
		sum += a
		next := int(math.RoundToEven(float64(cells) * float64(sum) / float64(whole)))
		runs[i], end = next-end, next
	}
	return runs
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

// dots draws the bars' key from row row, from x, each of its rows cut at the
// frame's width.
func (y yearDrawn) dots(c *canvas, x, row int) {
	for i, l := range y.familyDots() {
		c.line(x, row+i, l.fit(y.Width-x))
	}
}

// familyDots are the bars' key, from the bars' column: each family's dot in
// its colour, and its name, three cells apart, flowed onto as many rows as
// they take, a family never broken.
func (y yearDrawn) familyDots() []line {
	dots := make([]line, len(y.page.Families))
	for i, family := range y.page.Families {
		dots[i] = familyDot(family, dimInk)
	}
	return wrapped(span{"   ", dimInk}, y.Width-y.form.barX, dots)
}

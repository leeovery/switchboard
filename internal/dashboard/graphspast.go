package dashboard

import (
	"cmp"
	"slices"
)

// modelsStripRow is how many rows under MODELS BY DAY's top its first strip
// is: under the weeks over its days, and a blank row.
const modelsStripRow = 3

// pastBody is the body of YEAR, BY MONTH or MODELS BY DAY, as its page lays
// it out in the frame: the rows it takes, a row at the least, as before its
// page is built; and its headings, each over the first row under it, in rows
// under its top.
type pastBody struct {
	rows  int
	heads []headed
}

// pastBodies are the bodies of YEAR, BY MONTH and MODELS BY DAY, by the
// section: YEAR's days and their numbers, as the Year's days lays them out,
// the numbers' heading over them where they're under the key; BY MONTH's
// rows and the bars' key, its heads over its months; and MODELS BY DAY's
// strips and their key, as modelsLaid lays them out, in the narrow form each
// strip's name over its days. Before its page is built, each is a blank row,
// as is MODELS BY DAY's where nothing is recorded in its eight weeks, but
// for the words saying so.
func (f Frame) pastBodies() map[GraphsSection]pastBody {
	bodies := map[GraphsSection]pastBody{GraphsYear: {rows: 1}, GraphsMonths: {rows: 1}, GraphsModels: {rows: 1}}
	if y, ok := f.graphsYear(); ok {
		year := pastBody{rows: y.daysRows()}
		if !y.laid.beside {
			year.heads = []headed{{at: yearNumbersRow, first: yearNumbersRow + 1}}
		}
		months := y.laid.byMonth
		bodies[GraphsYear] = year
		bodies[GraphsMonths] = pastBody{rows: y.height(), heads: []headed{{at: months.heads, first: months.first}}}
	}
	if l, ok := f.modelsLaid(0); ok {
		models := pastBody{rows: l.keyRow + 1}
		for i := range l.page.ByModel {
			models.heads = append(models.heads, l.named(l.familyRow(i))...)
		}
		bodies[GraphsModels] = models
	}
	return bodies
}

// pastDrawn draws the body of YEAR, BY MONTH or MODELS BY DAY, placed, from
// the left: the Year's days and their numbers; BY MONTH's rows and the bars'
// key under them; or Days' strips of each family and their key, as
// modelsDrawn draws them. Before its page is built, nothing.
func (f Frame) pastDrawn(c *canvas, placed graphsPlaced) {
	top := placed.body(f.Width).y
	if placed.section == GraphsModels {
		f.modelsDrawn(c, top)
		return
	}
	y, ok := f.graphsYear()
	switch {
	case !ok:
	case placed.section == GraphsYear:
		y.days(c, top)
	default:
		y.months(c, top)
		y.dots(c, y.form.barX, top+y.dotsRow())
	}
}

// pastSays is what the heading of BY MONTH or MODELS BY DAY says after its
// name, in cells cells: BY MONTH's, as byMonthSaid has it, once its page is
// built; MODELS BY DAY's, what it shows, as Days' BY MODEL says it, where it
// fits whole; else nothing, as for the rest.
func (f Frame) pastSays(s GraphsSection, cells int) line {
	switch s {
	case GraphsMonths:
		if y, ok := f.graphsYear(); ok {
			return y.byMonthSaid(cells)
		}
	case GraphsModels:
		if said := (line{{byModelSays, faintInk}}); said.width() <= cells {
			return said
		}
	}
	return nil
}

// graphsYear is YEAR's and BY MONTH's Year as Trends draws it, of its page
// shown as Split asks, drawn apart, as yearFor lays it out, reporting false
// before its page is built.
func (f Frame) graphsYear() (yearDrawn, bool) {
	g := f.Graphs
	if g.Year == nil {
		return yearDrawn{}, false
	}
	return f.yearFor(yearDrawn{page: *g.Year, ask: g.Split, alone: g.yearShown(), apart: true}), true
}

// modelsLaid is MODELS BY DAY laid out in the frame from row top: Days'
// first part, of its page without BY ACCOUNT, which Accounts' Detail draws,
// so its names' column is its families' alone; wide where its strips and its
// key fit, as modelsKey has it, else in the narrow form; its key a blank row
// under the last strip. It reports false before the page is built, or where
// nothing is recorded in its eight weeks.
func (f Frame) modelsLaid(top int) (daysLayout, bool) {
	p := f.Graphs.Days
	if p == nil || !p.Recorded() {
		return daysLayout{}, false
	}
	models := *p
	models.ByAccount = nil
	l := f.daysStripped(models, top, !f.daysWideFits(models, f.modelsKey(false)), modelsStripRow)
	l.keyRows, l.keyRow = 1, l.stripRow(l.familyRow(len(models.ByModel)-1))+2
	return l, true
}

// modelsDrawn draws MODELS BY DAY from row top, as modelsLaid lays it out:
// the weeks over the days, a strip a family, and the key; or where nothing
// is recorded in its eight weeks, that it isn't; or before its page is
// built, nothing.
func (f Frame) modelsDrawn(c *canvas, top int) {
	l, ok := f.modelsLaid(top)
	switch {
	case ok:
		f.daysWeeks(c, l)
		f.familyStrips(c, l)
		c.line(0, l.keyRow, f.modelsKey(l.narrow).fit(f.edge()))
	case f.Graphs.Days != nil:
		c.line(0, top, line{{nothingRecorded, dimInk}}.fit(f.edge()))
	}
}

// modelsKey is MODELS BY DAY's key, Days' but for BY ACCOUNT's mark: the
// shades, what they are where it's wide, and the days to come.
func (f Frame) modelsKey(narrow bool) line {
	if narrow {
		return slices.Concat(f.daysShades(), line{{"   ", ink{}}}, daysToCome())
	}
	return slices.Concat(f.daysShadesSaid(), line{{"    ", ink{}}}, daysToCome())
}

// yearShown names what YEAR is shown for alone, as yearShownOf names it.
func (g GraphsShown) yearShown() string {
	return yearShownOf(g.Split, g.Accounts, g.Read)
}

// SplitShown is what v has YEAR show, as the key line names it: all, or
// what it's shown for alone, as yearShown names it.
func (g GraphsShown) SplitShown() string {
	return cmp.Or(g.yearShown(), allUse)
}

// Cycled is what Trends shows with what v has YEAR show cycled on, round,
// as cycledAsk has it of its page. It reports false where nothing changes:
// before its page is built, or with nothing to show alone.
func (g GraphsShown) Cycled() (GraphsShown, bool) {
	if g.Year == nil {
		return g, false
	}
	var changed bool
	g.Split, changed = cycledAsk(g.Split, *g.Year)
	return g, changed
}

// Moved is what Trends shows with YEAR, shown for one account, shown for the
// account by places on from it, among those configured, round, as ← and →
// step, a place each, as movedAsk has it. It reports false elsewhere.
func (g GraphsShown) Moved(by int) (GraphsShown, bool) {
	var moved bool
	g.Split, moved = movedAsk(g.Split, g.Accounts, by)
	return g, moved
}

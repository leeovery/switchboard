package dashboard

import (
	"cmp"
	"slices"

	"github.com/leeovery/switchboard/internal/views"
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

// pastLaid is YEAR, BY MONTH and MODELS BY DAY laid out in the frame, those
// shown alone: the Year YEAR and BY MONTH draw, as graphsYear lays it out,
// and MODELS BY DAY, from row 0, as modelsLaid lays it out, each with
// whether it's laid out, as neither is while hidden or before its page is
// built.
type pastLaid struct {
	year     yearDrawn
	yearOK   bool
	models   daysLayout
	modelsOK bool
}

// pastKey is all that lays YEAR, BY MONTH and MODELS BY DAY out, and all the
// Year and the strips laid out read as they're drawn, so a copy of a frame
// Built laid them out for that differs in none of it shares what it laid:
// the frame's width and its look, which inks the days and squares them; the
// sections shown; the pages; what v has YEAR show, and what it's shown for
// alone, as the accounts' labels and the last 12 months read name it.
type pastKey struct {
	width int
	look  Look
	shown GraphsSections
	year  *views.YearPage
	days  *views.DaysPage
	split views.YearAsk
	alone string
}

// pastKey is what lays the frame's YEAR, BY MONTH and MODELS BY DAY out.
func (f Frame) pastKey() pastKey {
	g := f.Graphs
	return pastKey{width: f.Width, look: f.Look, shown: g.Sections, year: g.Year, days: g.Days, split: g.Split, alone: g.yearShown()}
}

// pastLaid is YEAR, BY MONTH and MODELS BY DAY as the frame lays them out:
// as Built laid them out, where it did for a frame alike, as pastKey has it,
// else afresh, as layPast lays them.
func (f Frame) pastLaid() pastLaid {
	if f.built != nil && f.built.pastOf == f.pastKey() {
		return f.built.past()
	}
	return f.layPast()
}

// layPast lays out YEAR, BY MONTH and MODELS BY DAY, those shown alone.
func (f Frame) layPast() pastLaid {
	var p pastLaid
	if s := f.Graphs.Sections; s.Has(GraphsYear) || s.Has(GraphsMonths) {
		p.year, p.yearOK = f.graphsYear()
	}
	if f.Graphs.Sections.Has(GraphsModels) {
		p.models, p.modelsOK = f.modelsLaid(0)
	}
	return p
}

// pastBodies are the bodies of YEAR, BY MONTH and MODELS BY DAY, by the
// section, as pastLaid lays them out: YEAR's days and their numbers, as the
// Year's days lays them out, the numbers' heading over them where they're
// under the key; BY MONTH's rows and the bars' key, its heads over its
// months; and MODELS BY DAY's strips and their key, in the narrow form each
// strip's name over its days. Before its page is built, each is a blank row,
// as is MODELS BY DAY's where nothing is recorded in its eight weeks, but
// for the words saying so.
func (f Frame) pastBodies() map[GraphsSection]pastBody {
	bodies := map[GraphsSection]pastBody{GraphsYear: {rows: 1}, GraphsMonths: {rows: 1}, GraphsModels: {rows: 1}}
	p := f.pastLaid()
	if y := p.year; p.yearOK {
		year := pastBody{rows: y.daysRows()}
		if !y.laid.beside {
			year.heads = []headed{{at: yearNumbersRow, first: yearNumbersRow + 1}}
		}
		months := y.laid.byMonth
		bodies[GraphsYear] = year
		bodies[GraphsMonths] = pastBody{rows: y.height(), heads: []headed{{at: months.heads, first: months.first}}}
	}
	if l := p.models; p.modelsOK {
		models := pastBody{rows: l.keyRow + 1}
		for i := range l.page.ByModel {
			models.heads = append(models.heads, l.named(l.familyRow(i))...)
		}
		bodies[GraphsModels] = models
	}
	return bodies
}

// pastDrawn draws the body of YEAR, BY MONTH or MODELS BY DAY, placed, from
// the left, as pastLaid lays them out: the Year's days and their numbers;
// BY MONTH's rows and the bars' key under them; or Days' strips of each
// family and their key, as modelsDrawn draws them. Before its page is
// built, nothing.
func (f Frame) pastDrawn(c *canvas, placed graphsPlaced) {
	top, p := placed.body(f.Width).y, f.pastLaid()
	switch y := p.year; {
	case placed.section == GraphsModels:
		f.modelsDrawn(c, p, top)
	case !p.yearOK:
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
		if p := f.pastLaid(); p.yearOK {
			return p.year.byMonthSaid(cells)
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
	return f.yearFor(yearDrawn{page: *g.Year, ask: g.Split, alone: g.yearShown()}), true
}

// modelsLaid is MODELS BY DAY laid out in the frame from row top: wide where
// its strips and its key fit, as modelsKey has it, else in the narrow form;
// its key a blank row under the last strip. It reports false before the page
// is built, or where nothing is recorded in its eight weeks.
func (f Frame) modelsLaid(top int) (daysLayout, bool) {
	p := f.Graphs.Days
	if p == nil || !p.Recorded() {
		return daysLayout{}, false
	}
	l := f.daysStripped(*p, top, !f.daysWideFits(*p, f.modelsKey(false)), modelsStripRow)
	l.keyRows, l.keyRow = 1, l.stripRow(l.familyRow(len(p.ByModel)-1))+2
	return l, true
}

// modelsDrawn draws MODELS BY DAY from row top, as p lays it out: the weeks
// over the days, a strip a family, and the key; or where nothing is recorded
// in its eight weeks, that it isn't; or before its page is built, nothing.
func (f Frame) modelsDrawn(c *canvas, p pastLaid, top int) {
	switch l := p.models.down(top); {
	case p.modelsOK:
		f.daysWeeks(c, l)
		f.familyStrips(c, l)
		c.line(0, l.keyRow, f.modelsKey(l.narrow).fit(f.edge()))
	case f.Graphs.Days != nil:
		c.line(0, top, line{{nothingRecorded, dimInk}}.fit(f.edge()))
	}
}

// down is MODELS BY DAY, laid out from row 0, laid out from row top.
func (l daysLayout) down(top int) daysLayout {
	l.top, l.keyRow = l.top+top, l.keyRow+top
	return l
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

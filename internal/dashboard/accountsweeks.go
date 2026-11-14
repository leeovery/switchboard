package dashboard

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// Detail's weeks section, as many weeks as fit under its rule, and WORTH
// under it: of the account shown, BY DAY, WEEKS and their facts; of all
// accounts, a band of week columns an account, or where their rows don't
// fit, Weeks' table.

// sectionForm is the form Detail's weeks section takes: one account's BY DAY
// and WEEKS, all accounts' bands, or their table.
type sectionForm int

const (
	weeksOfOne sectionForm = iota
	weeksInBands
	weeksInATable
)

// weeksForm is the form the panel's weeks section takes: one account's,
// where one is shown; all accounts' bands, where at least a week of them
// fits across the weeks' edge and their rows, WORTH's and a line of
// LATELY's fit the view; else their table.
func (d detailDrawn) weeksForm() sectionForm {
	switch {
	case d.id != "":
		return weeksOfOne
	case d.bands().shown > 0 && d.latelyFrom(d.bandsRows())+1 <= d.f.viewRows():
		return weeksInBands
	}
	return weeksInATable
}

// weeksShown is how many weeks the weeks section shows, of those the page
// holds: as many as its form fits.
func (d detailDrawn) weeksShown() int {
	shown := d.l.weeks
	switch d.weeksForm() {
	case weeksInBands:
		shown = d.bands().shown
	case weeksInATable:
		shown = d.table().shown
	}
	return min(shown, len(d.page.Weeks))
}

// reach is how many weeks the weeks section reaches over, as reachedOver
// has it of the page's and those shown.
func (d detailDrawn) reach() int {
	return reachedOver(d.page.Reaches, d.weeksShown())
}

// weeksRows are the rows the weeks section takes in its form, a row or a
// band each configured account, as the moment's Configured has them, read
// or not, so what's under it stands where it will once the page is read.
func (d detailDrawn) weeksRows() int {
	switch d.weeksForm() {
	case weeksInBands:
		return d.bandsRows()
	case weeksInATable:
		return tableRows(len(d.accounts))
	}
	return oneWeeksRows
}

// bandsRows are the rows all accounts' bands take: a band each configured
// account and all accounts.
func (d detailDrawn) bandsRows() int {
	return bandHeight * (len(d.accounts) + 1)
}

// weeksAt is the row the weeks section starts at, a blank row under the
// rule.
func (d detailDrawn) weeksAt() int {
	return d.ruleRow() + partsApart
}

// worthAt is the row WORTH starts at, a blank row under the weeks section,
// as weeksRows has it.
func (d detailDrawn) worthAt() int {
	return d.weeksAt() + d.weeksRows() + 1
}

// latelyFrom is the row LATELY starts at, a blank row under WORTH, under
// the weeks section rows rows tall.
func (d detailDrawn) latelyFrom(rows int) int {
	return d.weeksAt() + rows + 1 + detailWorthRows + 1
}

// weeksSection draws the weeks section from row y in its form, as weeksForm
// chooses it.
func (d detailDrawn) weeksSection(c *canvas, y int) {
	switch d.weeksForm() {
	case weeksInBands:
		d.inBands(c, y)
	case weeksInATable:
		d.asATable(c, y, line{{"WEEKS", dimInk}})
	default:
		weeks := d.l.weeksIn(d.l.weeksCells(), d.weeksShown())
		d.byDay(c, y, weeks)
		d.weeks(c, y+weeksUnderDays, weeks)
		d.facts(c, y+factsUnderDays)
	}
}

// weeksNames are the cells of the names' column all accounts' weeks are
// laid out by: all accounts', or the widest of the accounts' names and
// marks, as listName has them, and the gap given, to mostNames at the most.
func (d detailDrawn) weeksNames(gap int) int {
	widest := allAccountsNamed.width()
	for _, a := range d.f.Accounts.Accounts {
		n := accountName{label: labelled(a.Label, a.ID), primary: a.Primary, unrouted: true}
		widest = max(widest, n.cells(ansi.StringWidth(n.label)))
	}
	return min(widest+gap, mostNames)
}

// bands are all accounts' bands laid from the panel's column to the weeks'
// edge, as bandsIn lays them, their names bandNameGap before their columns.
func (d detailDrawn) bands() bandsLayout {
	return bandsIn(d.l.panel, d.weeksNames(bandNameGap), d.l.edge())
}

// tableFrom is all accounts' table laid from the given column to the edge
// given, as tableIn lays it, its names nameGap before its weeks, as Weeks'
// table has them.
func (d detailDrawn) tableFrom(x, end int) weeksTable {
	return tableIn(x, d.weeksNames(nameGap), end)
}

// table is all accounts' table laid from the panel's column to the weeks'
// edge.
func (d detailDrawn) table() weeksTable {
	return d.tableFrom(d.l.panel, d.l.edge())
}

// inBands draws all accounts' bands from row y, all accounts' first, then
// each configured account's, as the moment's Configured has them, and on
// all accounts' peaks' row, where the weeks reached over don't all show,
// how many don't.
func (d detailDrawn) inBands(c *canvas, y int) {
	b := d.bands()
	reach := d.reach()
	b.draw(c, d.f.Look, y, weekBand{name: allAccountsNamed, peaks: d.page.All.Peaks, facts: d.page.All.Facts(reach)})
	b.more(c, y+bandRows, reach)
	for i, a := range d.accounts {
		row, ok := d.rowOf(a.ID)
		if !ok {
			continue
		}
		_, known := d.m.Account(a.ID)
		band := weekBand{name: listName(a).line(b.names), peaks: row.Peaks, facts: row.Facts(reach), capAt: capKept(a, known)}
		b.draw(c, d.f.Look, y+bandHeight*(i+1), band)
	}
}

// asATable draws all accounts' table from row y, laid from the panel's
// column to the weeks' edge, label on its heads' row.
func (d detailDrawn) asATable(c *canvas, y int, label line) {
	d.tableAt(c, d.table(), y, label)
}

// tableAt draws all accounts' table as t lays it from row y, label on its
// heads' row: all accounts' row, then each configured account's, as the
// moment's Configured has them, each its peaks and its facts on the weeks
// reached over.
func (d detailDrawn) tableAt(c *canvas, t weeksTable, y int, label line) {
	reach := reachedOver(d.page.Reaches, min(t.shown, len(d.page.Weeks)))
	all := tableRow{name: allAccountsNamed, peaks: d.page.All.Peaks, facts: d.page.All.Facts(reach)}
	var rows []tableRow
	for _, a := range d.accounts {
		if row, ok := d.rowOf(a.ID); ok {
			rows = append(rows, tableRow{name: listName(a).line(t.names), peaks: row.Peaks, facts: row.Facts(reach)})
		}
	}
	t.draw(c, y, label, d.page.Weeks, reach, all, rows)
}

// rowOf is the period's page's row of the account with the given id,
// reporting false where it holds none.
func (d detailDrawn) rowOf(id string) (views.AccountsRow, bool) {
	page, row := pageRowOf(d.page, id)
	return row, page != nil
}

// weeksIn are where the latest of count weeks that fit in room cells lie,
// from cx, as latestFitting lays them: a week its days, the gap between.
func (l detailLayout) weeksIn(room, count int) []flex.Span {
	return latestFitting(room, count, l.step()-l.gap, l.gap)
}

// byDay draws BY DAY along row y: its heading; from cx, the days of the last
// of its weeks, as many as given, each week where its span lies, a day dw
// cells wide, each in Days' shades at its level on the account's own scale,
// a red ▪ in its middle where it hit a 5-hour limit, and · in text.faint a
// cell of each day still to come; and on the row under them, each week's
// first day, this week's bold, and at the panel's left, where the weeks
// reached over don't all show, how many don't.
func (d detailDrawn) byDay(c *canvas, y int, weeks []flex.Span) {
	c.line(d.l.panel, y, line{{"BY DAY", dimInk}})
	days, coming := d.row.Days, len(d.row.Days)-d.page.ToCome
	first := max(len(days)-7*len(weeks), 0)
	for i := first; i < len(days); i++ {
		x := d.l.cx() + weeks[(i-first)/7].At + (i-first)%7*d.l.dw
		if i >= coming {
			c.line(x, y, dayToCome(d.l.dw))
			continue
		}
		c.line(x, y, d.f.daySwatch(dayInk(days[i].Level), days[i].Level, days[i].Limit, d.l.dw))
	}
	heads := d.page.Weeks[max(len(d.page.Weeks)-len(weeks), 0):]
	for i, week := range heads {
		c.line(d.l.cx()+weeks[i].At, y+1, weekHead(week, i == len(heads)-1))
	}
	c.line(d.l.panel, y+1, moreWeeks(d.page.Reaches-len(weeks)).fit(d.l.into-1))
}

// dayToCome is a day of BY DAY still to come, cells wide: a · a cell, in
// text.faint.
func dayToCome(cells int) line {
	return line{{strings.Repeat("·", cells), faintInk}}
}

// weeks draws WEEKS along row y: its heading; and from cx, at the weeks
// given, the latest peak at the last, each as usage's bar draws a share,
// weekBar cells long, and a cell on, as peakSaid says it.
func (d detailDrawn) weeks(c *canvas, y int, weeks []flex.Span) {
	c.line(d.l.panel, y, line{{"WEEKS", dimInk}})
	cells := d.l.weekBar()
	peaks := d.row.Peaks[max(len(d.row.Peaks)-len(weeks), 0):]
	for i, p := range peaks {
		x := d.l.cx() + weeks[len(weeks)-len(peaks)+i].At
		peakBar(p).draw(c, d.f.Look, x, y, cells)
		c.line(x+cells+1, y, peakSaid(p))
	}
}

// peakBar is a week's peak as WEEKS' bar fills to it: empty where it isn't
// known.
func peakBar(p views.WeekPeak) usageBar {
	if p.Share == nil {
		return usageBar{empty: true}
	}
	return usageBar{used: *p.Share, capAt: 1}
}

// peakSaid says a week's peak as WEEKS does, a whole percentage without its
// sign, this week's so far too: in text.muted, or at its cap bold
// state.destructive; a dash where it isn't known.
func peakSaid(p views.WeekPeak) line {
	if p.Share == nil {
		return line{noFigure}
	}
	peak := prose.WholePercent(*p.Share)
	if p.AtCap {
		return line{{strconv.Itoa(peak), exhaustedInk}}
	}
	return line{{strconv.Itoa(peak), mutedInk}}
}

// facts draws the facts on the account's weeks along row y, from the panel's
// column to the weeks' edge: Weeks' facts on the weeks reached over, as
// detailFacts says them, the fullest form that fits.
func (d detailDrawn) facts(c *canvas, y int) {
	cells := d.l.edge() - d.l.panel
	c.line(d.l.panel, y, fitParts(cells, detailFacts(d.row.Facts(d.reach())).fitting(cells)...))
}

// worth draws WORTH from row y: its heading; from cx to the weeks' edge,
// what the row's requests were worth over the period, detailWorthChart rows
// tall against its own peak, in the finest bucket that fits, as
// worthChart draws it with its axis; and under the axis its peak, as
// peakForms has it, the fullest that fits.
func (d detailDrawn) worth(c *canvas, y int) {
	c.line(d.l.panel, y, line{{"WORTH", dimInk}})
	cells := d.l.weeksCells()
	worthAcross(c, d.f.Look, d.l.cx(), y, cells, d.row.WorthBy)
}

// worthAcross draws a worth chart from x along row y, cells wide, as
// Detail's WORTH draws one: the finest of the charts given that fits,
// detailWorthChart rows tall against its own peak, its axis under it, and
// under that its peak, as peakForms has it, the fullest that fits.
func worthAcross(c *canvas, look Look, x, y, cells int, charts views.WorthCharts) {
	by := charts.Fitting(chartFits(cells))
	worthChart(c, look, x, y, cells, detailWorthChart, by, peakWorth(by))
	c.line(x, y+detailWorthChart+1, fitParts(cells, peakForms(by, false).fitting(cells)...))
}

// rule draws the rule heading the weeks section along row y, from the
// panel's column to the weeks' edge: ┄┄ in border, the weeks shown,
// counted, "the last nine weeks", in text.faint, and ┄ in border to the
// edge.
func (d detailDrawn) rule(c *canvas, y int) {
	head := line{{"┄┄ ", borderInk}, {"the last " + prose.Number(d.weeksShown()) + " weeks", faintInk}, {" ", borderInk}}
	rest := d.l.edge() - d.l.panel - head.width()
	c.line(d.l.panel, y, append(head, span{strings.Repeat("┄", max(rest, 0)), borderInk}))
}

package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The chosen project's figures, in cells and rows: WORTH BY's columns' rows,
// and the rows the section takes, its heading, its columns, its axis and its
// peak; the cells between the wide form's halves; a week's days' columns at
// their least, the cells of each and between two, as the wide form's left
// half holds them whatever the period; and ON ACCOUNTS' bar's least, as it's
// laid beside another part.
const (
	worthRows       = 6
	worthTaken      = 1 + worthRows + 2
	halvesApart     = 4
	weekDays        = 7
	dayColumnLeast  = 8
	dayColumnsApart = 2
	shareBarLeast   = 20
)

// The chosen project's sections, in the order they show and the reverse of
// the order they leave in as rows run short: WORTH BY, BY MODEL, then ON
// ACCOUNTS.
const (
	sectionWorth = iota
	sectionModels
	sectionAccounts
	projectSections
)

// projectForm is how the chosen project's sections lie across the frame:
// stacked, as on a phone; middling, WORTH BY across the width, BY MODEL
// beside ON ACCOUNTS under it, as on an iPad; or wide, WORTH BY in the left
// half, BY MODEL over ON ACCOUNTS in the right, as on a MacBook.
type projectForm int

const (
	projectStacked projectForm = iota
	projectMiddling
	projectWide
)

// projectDrawn is the chosen project as a frame draws it under the list, of
// the moment m at now: its page; WORTH BY in the bucket that fits it as it
// lies, as worthFitting has it; how its sections lie, and where: the left
// half's cells, wide, or the column ON ACCOUNTS starts at, middling; and how
// many of its sections show, from the first, as projectsLaid keeps them.
type projectDrawn struct {
	f     Frame
	m     status.Moment
	now   time.Time
	page  views.ProjectPage
	worth views.WorthBy
	form  projectForm
	split int
	kept  int
}

// projectDrawnOf is the chosen project of the frame f, of the moment m at
// now, laid out across its width, as formOf lays its sections out, WORTH BY
// in the finest bucket that fits the cells it takes as they lie, all of
// them kept.
func projectDrawnOf(f Frame, m status.Moment, now time.Time, page views.ProjectPage) projectDrawn {
	d := projectDrawn{f: f, m: m, now: now, page: page, kept: projectSections}
	d.form, d.split = d.formOf()
	d.worth = d.worthFitting()
	return d
}

// worthFitting is WORTH BY in the finest bucket that fits it as the
// sections lie, in the left half, wide, else across the frame, as chartFits
// has it: of the last 12 weeks, by day where a cell for each day fits, else
// by week; of the last 12 months, by week, else by month.
func (d projectDrawn) worthFitting() views.WorthBy {
	cells := d.f.Width
	if d.form == projectWide {
		cells = d.split
	}
	return d.page.WorthBy.Fitting(chartFits(cells))
}

// chartFits reports whether a chart of as many buckets as given fits the
// cells given, a cell for each, touching, as chartSlots narrows its columns
// to fit.
func chartFits(cells int) func(buckets int) bool {
	return func(buckets int) bool {
		_, fits := flex.Lay(cells, chartPart(buckets, 1, 0))
		return fits
	}
}

// chartPart is the cells a chart of the buckets given takes at the fewest,
// each column the cells given and the cells apart given.
func chartPart(buckets, column, apart int) flex.Part {
	least := max(buckets*(column+apart)-apart, 0)
	return flex.Part{Least: least, Natural: least}
}

// halfLeast is the fewest cells WORTH BY takes in the wide form's left
// half, whatever the period, so the form holds as the dial turns: a week's
// days' columns at dayColumnLeast cells, dayColumnsApart apart; or a cell
// for each bucket of its coarsest, touching, where that's more, as
// chartSlots narrows its columns to fit.
func (d projectDrawn) halfLeast() int {
	coarsest := d.page.WorthBy.Coarsest()
	return max(chartPart(weekDays, dayColumnLeast, dayColumnsApart).Least, chartPart(len(coarsest.Buckets), 1, 0).Least)
}

// formOf is how the sections lie across the frame, as flex lays two halves
// out: wide where WORTH BY at its least, as halfLeast has it, fits the left
// half, halvesApart from the right, and BY MODEL and ON ACCOUNTS whole fit
// the right, the left half's cells given; else middling where BY MODEL and
// ON ACCOUNTS whole fit a half each, the column the right half starts at
// given; else stacked.
func (d projectDrawn) formOf() (projectForm, int) {
	right := max(d.modelsWhole(), d.accountsWhole())
	if left, _, ok := halves(d.f.Width, halvesApart, max(d.halfLeast(), right)); ok {
		return projectWide, left.Size
	}
	if _, half, ok := halves(d.f.Width, 0, right); ok {
		return projectMiddling, half.At
	}
	return projectStacked, 0
}

// halves are two halves of width cells, apart cells between them, each least
// cells at the fewest, the right taking the odd cell, as flex lays them;
// reporting false where they don't fit.
func halves(width, apart, least int) (left, right flex.Span, ok bool) {
	spans, ok := flex.Lay(width, flex.Part{Least: least, Grows: true}, flex.Part{Least: least, Grows: true, Gap: apart, Odd: true})
	return spans[0], spans[1], ok
}

// modelsWhole are the cells BY MODEL takes whole: its versions' rows, as
// versionRow lays them out.
func (d projectDrawn) modelsWhole() int {
	return versionAt + d.versionCells() + 1 + percentCells
}

// versionCells are the cells of the widest name of BY MODEL's versions.
func (d projectDrawn) versionCells() int {
	cells := 0
	for _, v := range d.page.ByModel {
		cells = max(cells, line{{v.Version.Name, secondaryInk}}.width())
	}
	return cells
}

// accountsWhole are the cells ON ACCOUNTS takes whole beside another part:
// its names, as namesCells has them, its bar at shareBarLeast, and its share.
func (d projectDrawn) accountsWhole() int {
	return labelIndent + d.namesCells() + shareBarLeast + listGap + percentCells
}

// namesCells are the cells of ON ACCOUNTS' column of names: the configured
// accounts', with their marks, as every column of names lays them out.
func (d projectDrawn) namesCells() int {
	return accountsColumn(nameGap, namesOf(d.m)).Natural
}

// heights are the rows each section takes, in their order: WORTH BY's
// worthTaken; BY MODEL's and ON ACCOUNTS' heading and a row a version or an
// account.
func (d projectDrawn) heights() [projectSections]int {
	return [projectSections]int{worthTaken, 1 + len(d.page.ByModel), 1 + len(d.page.OnAccounts)}
}

// height is the rows the first kept of the sections take as they lie, a
// blank row between two stacked: wide, WORTH BY's or the right column's,
// whichever's taller; middling, WORTH BY's and the taller of the two beside
// each other under it; stacked, each's.
func (d projectDrawn) height(kept int) int {
	h := d.heights()
	stacked := func(parts ...int) int {
		rows := 0
		for _, n := range parts {
			if n > 0 {
				rows += n + min(rows, 1)
			}
		}
		return rows
	}
	shown := func(section int) int {
		if section < kept {
			return h[section]
		}
		return 0
	}
	switch d.form {
	case projectWide:
		return max(shown(sectionWorth), stacked(shown(sectionModels), shown(sectionAccounts)))
	case projectMiddling:
		return stacked(shown(sectionWorth), max(shown(sectionModels), shown(sectionAccounts)))
	}
	return stacked(shown(sectionWorth), shown(sectionModels), shown(sectionAccounts))
}

// head draws the project's head along row y: its name, as the list names
// it; two cells on, its path, in text.muted, cut from its start as
// status.DirPath cuts it; and at the right, ● and its sessions running, as
// the live column says them, then running, in text.muted, where any run.
func (d projectDrawn) head(c *canvas, y int) {
	var running line
	if live := d.page.Live; live > 0 {
		running = line{{"●", positiveInk}, {" " + prose.Grouped(live), secondaryInk}, {" running", mutedInk}}
		c.right(d.f.Width, y, running)
	}
	x := c.line(0, y, projectNamed(d.page.ProjectRow)) + listGap
	if !d.page.NoDirectory() {
		room := d.f.Width - x - running.width() - listGap
		c.line(x, y, line{{status.DirPath(d.page.Path, "", room), mutedInk}})
	}
}

// numbers are the project's numbers over the period, on as many lines width
// cells wide as they take, each number bold text.primary and its word
// text.muted, " · " between them: its sessions; its requests; its worth at
// API prices, as Rounded says it; its share of every project's worth, where
// it's known; and its tokens, as prose.Millions counts them.
func (d projectDrawn) numbers(width int) []line {
	p := d.page
	parts := []line{
		logCounted(p.ProjectTotals.Sessions, "session", titleInk),
		logCounted(p.Requests, "request", titleInk),
		pricedWorth(p.Worth, titleInk),
	}
	if share, ok := views.ShareSaid(p.Share); ok {
		parts = append(parts, line{{share, titleInk}, {" of all use", mutedInk}})
	}
	parts = append(parts, line{{prose.Millions(p.Tokens.Total), titleInk}, {" tokens", mutedInk}})
	return wrapped(span{" · ", mutedInk}, width, parts)
}

// pricedWorth is a worth at API prices, as Rounded says it, in the ink
// given, " at API prices" after it in text.muted, but where none of it is
// priced.
func pricedWorth(w views.Worth, k ink) line {
	said := line{{w.Rounded(), k}}
	if w.Amount == nil && len(w.Unpriced) > 0 {
		return said
	}
	return append(said, span{" at API prices", mutedInk})
}

// sections draws the sections kept from row y, as they lie, and returns
// the row after the lowest.
func (d projectDrawn) sections(c *canvas, y int) int {
	ends := []int{y}
	at := func(section, x, row, width int) int {
		if section >= d.kept {
			return row
		}
		end := d.section(c, section, x, row, width)
		ends = append(ends, end)
		return end + 1
	}
	switch d.form {
	case projectWide:
		at(sectionWorth, 0, y, d.split)
		right := d.split + halvesApart
		at(sectionAccounts, right, at(sectionModels, right, y, d.f.Width-right), d.f.Width-right)
	case projectMiddling:
		under := at(sectionWorth, 0, y, d.f.Width)
		at(sectionModels, 0, under, d.split)
		at(sectionAccounts, d.split, under, d.f.Width-d.split)
	default:
		at(sectionAccounts, 0, at(sectionModels, 0, at(sectionWorth, 0, y, d.f.Width), d.f.Width), d.f.Width)
	}
	return slices.Max(ends)
}

// section draws the section given from x along row y, width cells wide, and
// returns the row after it.
func (d projectDrawn) section(c *canvas, section, x, y, width int) int {
	switch section {
	case sectionWorth:
		return d.worthBy(c, x, y, width)
	case sectionModels:
		return d.byModel(c, x, y, width)
	}
	return d.onAccounts(c, x, y, width)
}

// worthBy draws WORTH BY from x along row y, width cells wide: its heading,
// naming its bucket; its columns and their axis, as worthChart draws them,
// against its peak's; and its peak, as worthPeak says it. It returns the
// row after it.
func (d projectDrawn) worthBy(c *canvas, x, y, width int) int {
	c.line(x, y, line{{"WORTH BY " + strings.ToUpper(bucketWord[d.worth.Unit]), dimInk}})
	worthChart(c, d.f.Look, x, y+1, width, worthRows, d.worth, peakWorth(d.worth))
	c.line(x, y+1+worthRows+1, worthPeak(d.worth))
	return y + worthTaken
}

// byModel draws BY MODEL from x along row y, width cells wide: its heading;
// then a row a version, as Accounts' BY MODEL heads each, its share of the
// project's worth after the widest name, right-aligned in percentCells, in
// text.secondary. It returns the row after it.
func (d projectDrawn) byModel(c *canvas, x, y, width int) int {
	c.line(x, y, line{{"BY MODEL", dimInk}})
	cells := d.versionCells()
	for i, v := range d.page.ByModel {
		versionHead(c, x, y+1+i, v.Version, cells)
		c.rightFit(x+versionAt+cells+1+percentCells, x+width, y+1+i, shareParts(v.Share, secondaryInk)...)
	}
	return y + 1 + len(d.page.ByModel)
}

// onAccounts draws ON ACCOUNTS from x along row y, width cells wide: its
// heading; then a row an account, labelIndent cells in, its name in the
// column of names, as namesCells has it, with its marks, as the moment has
// them; the bar of its share of the project's worth, in accent.primary, to
// the eighth, as shareEighths has it, taking the cells left; and listGap on,
// its share, right-aligned in percentCells, in text.secondary. It returns the
// row after it.
func (d projectDrawn) onAccounts(c *canvas, x, y, width int) int {
	c.line(x, y, line{{"ON ACCOUNTS", dimInk}})
	names := d.namesCells()
	bar := width - labelIndent - names - listGap - percentCells
	for i, a := range d.page.OnAccounts {
		row, at := y+1+i, x+labelIndent
		c.line(at, row, d.accountNamed(a).line(names))
		eighthsBar(c, d.f.Look, at+names, row, shareEighths(a.Share, bar), []ledger.Picodollars{1}, []hue{worthInk.hue()})
		c.rightFit(at+names+bar+listGap+percentCells, x+width, row, shareParts(a.Share, secondaryInk)...)
	}
	return y + 1 + len(d.page.OnAccounts)
}

// accountNamed is the account a share is of as a column of names names it:
// as the moment names it, ◆ on the primary and ▲ on the one new sessions go
// to; else, as of one no longer configured, by the name the share has.
func (d projectDrawn) accountNamed(a views.AccountShare) accountName {
	if now, ok := d.m.Account(a.Account); ok {
		return namesAt([]status.AccountNow{now})[0]
	}
	return accountName{label: labelled(a.Label, a.Account)}
}

// shareEighths are the eighths of cells cells a share takes, as barUnits
// has them: none where it isn't known.
func shareEighths(share *float64, cells int) int {
	if share == nil {
		return 0
	}
	return barUnits(*share, 1, cells*8)
}

package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Accounts' weeks an account at a time, History's Weeks without its verdict:
// as bands of week columns, Detail's all accounts' where their rows fit and
// turned Compare's WEEKS; or as Weeks' table, Detail's all accounts' where
// they don't, as on a phone.

// A band's rows: its columns bandRows tall, its peaks on the row under them;
// its name's marks a cell before its columns; and its columns, each
// columnCells wide, a week's cells apart, as Weeks' columns lie, its facts
// as far after the last as the next column would start.
const (
	bandRows    = 4
	bandHeight  = bandRows + 1
	bandNameGap = 1
	bandPitch   = weekColumnCells + weeksApart
)

// The weeks' table's columns: its facts', tableApart apart, each headed.
const (
	tableApart  = 2
	limitsHead  = "limits"
	atResetHead = "at a reset"
)

// moreWeeks says how many of the weeks reached over don't fit, at the left
// of those that do, in text.subtle: "‹ 34 more"; nothing where none.
func moreWeeks(n int) line {
	if n <= 0 {
		return nil
	}
	return line{{"‹ " + prose.Grouped(n) + " more", dimInk}}
}

// reachedOver is how many weeks the weeks section reaches over, shown of
// them fitting: as many as reach, the period's own as views.AccountsPage's
// Reaches counts them, or as fit, where more do; those that don't fit
// counted out of view, its facts over them all.
func reachedOver(reach, shown int) int {
	return max(reach, shown)
}

// factsMost are the most weeks a row's facts count, as many as the page
// holds: the count their cells are bound by, every week at its limit and
// its share left whole.
const factsMost = 53

// bandFactsCells are the cells a band's facts take, so the columns hold
// still as the facts change: the widest of either of its rows, as bandFacts
// has them, at factsMost weeks, every one at its limit, and left whole.
func bandFactsCells() int {
	whole := 1.0
	widest := 0
	for _, all := range []bool{false, true} {
		v := views.AccountVerdict{All: all, Weeks: factsMost, Limits: factsMost, LeftAtReset: &whole}
		for _, row := range bandFacts(v, 0) {
			widest = max(widest, partsWidth(row))
		}
	}
	return widest
}

// bandFacts are a band's facts in cells cells, a row each: how many of its
// whole weeks it hit its limit in, or of every account, how many some
// account was short in, as factsLimits says it; and what they left at a
// reset, on average, in the fullest of leftFacts' forms that fits, where
// it's known; cells of none giving the fullest. A dash where no whole week
// is judged.
func bandFacts(v views.AccountVerdict, cells int) [][]part {
	if v.Weeks == 0 {
		return [][]part{{figurePart(line{noFigure}, Number{})}}
	}
	rows := [][]part{factsLimits(v)}
	if left := leftFacts(v); left != nil {
		if cells == 0 {
			return append(rows, left[0])
		}
		rows = append(rows, left.fitting(cells))
	}
	return rows
}

// bandsLayout is where bands of week columns lie across a row: their names
// from x, names cells wide; their columns from the names' end, shown of
// them, a band's pitch apart; and their facts after the last, facts cells.
type bandsLayout struct {
	x, names, shown, facts int
}

// bandsIn are bands laid from x to end, their names names cells wide: as
// many columns as fit between the names and their facts, as bandFactsCells
// has them, none where not one does.
func bandsIn(x, names, end int) bandsLayout {
	b := bandsLayout{x: x, names: names, facts: bandFactsCells()}
	b.shown = max((end-b.at()-b.facts)/bandPitch, 0)
	return b
}

// at is the column the bands' first week's column starts at.
func (b bandsLayout) at() int {
	return b.x + b.names
}

// factsAt is the column the bands' facts start at, where a column after the
// last would.
func (b bandsLayout) factsAt() int {
	return b.at() + b.shown*bandPitch
}

// weekBand is a band as it's drawn: its name; its peaks, the latest of which
// that fit show; Weeks' facts on them; and the cap it keeps a reserve to,
// its share of its week, none where it keeps none.
type weekBand struct {
	name  line
	peaks []views.WeekPeak
	facts views.AccountVerdict
	capAt float64
}

// draw draws the band from row y, as look draws its cells: its name in the
// names' cells; the latest of its peaks that fit, each a column bandRows
// tall, as peakColumn draws it, its peak under it, centred in a week's
// cells, as peakPart inks it, this week's in text.secondary; along it, its
// cap's level, where it keeps one, as capLevelLine draws it, under the
// columns; and at its right, its facts, as bandFacts has them, a row each.
func (b bandsLayout) draw(c *canvas, look Look, y int, band weekBand) {
	c.line(b.x, y, band.name.fit(b.names-1))
	shown := lastPeaks(band.peaks, b.shown)
	floor := y + bandRows - 1
	if band.capAt > 0 && len(shown) > 0 {
		capLevelLine(c, b.at(), floor, bandRows, (len(shown)-1)*bandPitch+columnCells, band.capAt)
	}
	for i, p := range shown {
		x := b.at() + i*bandPitch
		peakColumn(c, look, x, floor, bandRows, columnCells, p)
		said := peakPart(p, secondaryInk, Number{}).drawn()
		c.line(x+(weekColumnCells-said.width())/2, y+bandRows, said)
	}
	for i, row := range bandFacts(band.facts, b.facts) {
		c.line(b.factsAt(), y+i, fitParts(b.facts, row...))
	}
}

// more draws, on row y in the names' cells, how many of the weeks reached
// over don't show, as moreWeeks says it.
func (b bandsLayout) more(c *canvas, y, reach int) {
	c.line(b.x, y, moreWeeks(reach-b.shown).fit(b.names-1))
}

// lastPeaks are the last n of the peaks given, every one where there are
// fewer.
func lastPeaks(peaks []views.WeekPeak, n int) []views.WeekPeak {
	return peaks[max(len(peaks)-n, 0):]
}

// capLevelLine marks a cap's level across a chart of week columns rows tall
// up from row floor, width cells from x, in text.subtle, the columns drawn
// over it: along the row the cap falls in, each a share of its week, the
// top's holding a cap at the top's floor and anything above it.
func capLevelLine(c *canvas, x, floor, rows, width int, capAt float64) {
	row := min(int(capAt*float64(rows)), rows-1)
	c.text(x, floor-row, strings.Repeat(capLevel, width), dimInk)
}

// capKept is the share of its week the account keeps its cap at, where it
// keeps a reserve and the moment has it: none otherwise.
func capKept(a status.AccountNow, known bool) float64 {
	if !known || a.Reserve <= 0 {
		return 0
	}
	return a.Cap
}

// weeksTable is Weeks' table without its verdict: its names from x, names
// cells wide; its weeks from the names' end, shown of them, a band's pitch
// apart, each its first day over its peaks; and after the last, its facts,
// limits cells, tableApart, then left cells, each column headed.
type weeksTable struct {
	x, names, shown, limits, left int
}

// tableIn is the table laid from x to end, its names names cells wide: its
// facts' columns as wide as their heads and their widest, at factsMost
// weeks, so its weeks hold still as the facts change; and as many weeks as
// fit before them.
func tableIn(x, names, end int) weeksTable {
	whole := 1.0
	t := weeksTable{x: x, names: names, limits: ansi.StringWidth(limitsHead), left: ansi.StringWidth(atResetHead)}
	for _, all := range []bool{false, true} {
		v := views.AccountVerdict{All: all, Weeks: factsMost, Limits: factsMost, LeftAtReset: &whole}
		t.limits = max(t.limits, partsWidth(factsLimits(v)))
		t.left = max(t.left, partsWidth(leftFacts(v).shortest()))
	}
	t.shown = max((end-x-names-t.limits-tableApart-t.left)/bandPitch, 0)
	return t
}

// tableRows are the rows the table takes of accounts many accounts: its
// heads', all accounts', a blank, and a row an account.
func tableRows(accounts int) int {
	return 3 + accounts
}

// at is the column the table's week with index i, of those shown, starts at.
func (t weeksTable) at(i int) int {
	return t.x + t.names + i*bandPitch
}

// limitsAt is the column the table's limits start at, after its last week.
func (t weeksTable) limitsAt() int {
	return t.at(t.shown)
}

// leftAt is the column what the table's weeks left starts at.
func (t weeksTable) leftAt() int {
	return t.limitsAt() + t.limits + tableApart
}

// tableRow is a row of the table: its name, its peaks, the latest of which
// that fit show, and Weeks' facts on them.
type tableRow struct {
	name  line
	peaks []views.WeekPeak
	facts views.AccountVerdict
}

// draw draws the table from row y: on its heads' row, label at its left, or
// where weeks reached over don't show, how many, as moreWeeks says it; each
// week's first day, right-aligned in a week's cells, this week's bold, as
// weekHead has it, of the weeks given; and over the facts, their heads, in
// text.faint. Under them all accounts' row, a blank, and a row each account,
// as row draws them.
func (t weeksTable) draw(c *canvas, y int, label line, weeks []string, reach int, all tableRow, accounts []tableRow) {
	if more := moreWeeks(reach - t.shown); more != nil {
		label = more
	}
	c.line(t.x, y, label.fit(t.names-1))
	shown := weeks[max(len(weeks)-t.shown, 0):]
	for i, week := range shown {
		c.right(t.at(i)+weekColumnCells, y, weekHead(week, i == len(shown)-1))
	}
	c.line(t.limitsAt(), y, line{{limitsHead, faintInk}})
	c.line(t.leftAt(), y, line{{atResetHead, faintInk}})
	t.row(c, y+1, all)
	for i, row := range accounts {
		t.row(c, y+3+i, row)
	}
}

// row draws a row of the table along row y: its name; its peaks in the weeks
// shown, each right-aligned in peakCells, as peakPart inks it, this week's
// bold text.secondary, its … hanging past; and its facts, the limits as
// factsLimits says them and what they left in leftFacts' shortest form,
// where it's known, or a dash where no whole week is judged.
func (t weeksTable) row(c *canvas, y int, r tableRow) {
	c.line(t.x, y, r.name.fit(t.names-1))
	for i, p := range lastPeaks(r.peaks, t.shown) {
		end := t.at(i) + peakCells
		if p.Open && p.Share != nil {
			end += openHang
		}
		c.right(end, y, peakPart(p, strongInk, Number{}).drawn())
	}
	if r.facts.Weeks == 0 {
		c.line(t.limitsAt(), y, line{noFigure})
		return
	}
	c.line(t.limitsAt(), y, fitParts(t.limits, factsLimits(r.facts)...))
	if left := leftFacts(r.facts); left != nil {
		c.line(t.leftAt(), y, fitParts(t.left, left.shortest()...))
	}
}

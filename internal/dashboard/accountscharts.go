package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// Compare's charts, in rows: WEEKS' columns, a full week's, and WORTH's;
// and the rows each takes under its heading, its columns, its axis, and its
// facts or its peak.
const (
	weeksChartRows    = 5
	worthChartRows    = 4
	compareWeeksUnder = weeksChartRows + 2
	compareWorthUnder = worthChartRows + 2
)

// What Compare's charts' headings say after their names.
const (
	compareWeeksSays  = "each week at its peak"
	compareMonthsSays = compareWeeksSays + ", a month its fullest"
	compareWorthSays  = "at API prices, by "
	compareOwnScale   = " · its own scale"
)

// capLevel marks the cap's level across WEEKS' chart, on its empty cells.
const capLevel = "┄"

// weeksSection draws WEEKS from row y, and returns its last row: its
// heading, a month's fullest week named at 12 months; then in each column
// its peaks in the reach's columns, weeksChartRows tall at a full week, as
// peakColumn draws them, laid across its head's cells as chartSlots lays
// them, the cap's level across them where its account keeps a reserve, as
// capLine draws it; under them their axis, as reachNames has it; and under
// that its facts, as weeksFacts says them, cut as its figures are.
func (cp compared) weeksSection(c *canvas, y int, columns []compareColumn) int {
	cp.weeksHead(c, y, y+1)
	floor := y + weeksChartRows
	for _, col := range columns {
		slots := chartSlots(col.head, len(cp.page.Reach))
		cp.capLine(c, col, floor, slots)
		for i, p := range col.row.Reached {
			peakColumn(c, cp.f.Look, col.x+slots[i].x, floor, weeksChartRows, slots[i].width, p)
		}
		chartAxis(c, col.x, floor+1, col.head, slots, cp.reachNames(col.row))
		c.line(col.x, floor+2, fitParts(col.width, weeksFacts(col.row.Reaching).fitting(col.width)...))
	}
	return floor + 2
}

// weeksHead draws WEEKS' heading along row y, a month's fullest week named
// at 12 months, the heading of the rows from first.
func (cp compared) weeksHead(c *canvas, y, first int) {
	says := compareWeeksSays
	if cp.page.Period == views.PeriodTwelveMonths {
		says = compareMonthsSays
	}
	c.line(0, y, groupHead("WEEKS", says, false))
	c.heading(y, first)
}

// capLine draws the cap's level across the column's chart, its columns laid
// as slots lay them up from row floor, in text.subtle, where the column's
// account keeps a reserve, as the moment has it: along the row its cap falls
// in, of weeksChartRows, each a fifth of its week, the top's holding a cap
// at the top's floor, 80%, and anything above it. The columns are drawn
// over it.
func (cp compared) capLine(c *canvas, col compareColumn, floor int, slots []chartSlot) {
	capAt := capKept(col.account, !col.all && col.known)
	if capAt <= 0 || len(slots) == 0 {
		return
	}
	last := slots[len(slots)-1]
	capLevelLine(c, col.x, floor, weeksChartRows, last.x+last.width, capAt)
}

// reachNames are the reach's columns' names on WEEKS' axis, as axisName has
// them, the last now's; but under the row's column of this week, so far,
// its mark, in text.secondary, in place of its name, where its columns are
// weeks: a month's isn't this week's, though its fullest may be.
func (cp compared) reachNames(row views.AccountsRow) []line {
	weekly := cp.page.Period != views.PeriodTwelveMonths
	names := make([]line, len(cp.page.Reach))
	for i, b := range cp.page.Reach {
		names[i] = axisName(b.Axis, i == len(cp.page.Reach)-1)
		if weekly && i < len(row.Reached) && row.Reached[i].Open {
			names[i] = line{{views.PeakSoFar, secondaryInk}}
		}
	}
	return names
}

// weeksFacts are WEEKS' facts of a column, Weeks' verdict on the whole weeks
// of its reach, its words unsaid, in its forms: how many it hit its limit
// in, in text.subtle, or of every account, how many were short, bold
// text.secondary, as views.LimitHitOutOf and ShortOnOutOf say them; then,
// where it's known, what they left at a reset, on average, in text.subtle,
// as views.LeftAtAReset says it, else as views.Left does, else not at all.
// It's a dash where no whole week is judged.
func weeksFacts(v views.AccountVerdict) partForms {
	if v.Weeks == 0 {
		return partForms{{figurePart(line{noFigure}, Number{})}}
	}
	limits := factsLimits(v)
	apart := []part{wordsIn(" · ", dimInk)}
	var forms partForms
	for _, left := range leftFacts(v) {
		forms = append(forms, slices.Concat(limits, apart, left))
	}
	return append(forms, limits)
}

// factsLimits are how many of the whole weeks Weeks' facts are on an account
// hit its limit in, in text.subtle, or of every account, how many were
// short, bold text.secondary, as views.LimitHitOutOf and ShortOnOutOf say
// them.
func factsLimits(v views.AccountVerdict) []part {
	if v.All {
		return saidParts(views.ShortOnOutOf(v.Limits, v.Weeks), strongInk)
	}
	return saidParts(views.LimitHitOutOf(v.Limits, v.Weeks), dimInk)
}

// leftFacts are what the whole weeks Weeks' facts are on left at a reset, on
// average, in text.subtle, in its forms: as views.LeftAtAReset says it, then
// as views.Left does. None where it isn't known.
func leftFacts(v views.AccountVerdict) partForms {
	if v.LeftAtReset == nil {
		return nil
	}
	return partForms{saidParts(views.LeftAtAReset(*v.LeftAtReset), dimInk), saidParts(views.Left(*v.LeftAtReset), dimInk)}
}

// worthSection draws WORTH from row y, and returns its last row: its
// heading, naming its bucket; then in each column its worth over the period
// in the finest bucket that fits the narrowest column, as chartFits has it,
// so every column's is in the same one, worthChartRows tall, as worthChart
// draws it: each account's against the highest bucket of every configured
// account's, shown or not, so they compare, and all accounts' against its
// own; and under it its peak, in the fullest of peakForms' forms its column
// holds, as its figures take theirs.
func (cp compared) worthSection(c *canvas, y int, columns []compareColumn) int {
	fits := chartFits(cp.laid.narrowest())
	cp.worthHead(c, y, y+1, fits)
	scale := cp.worthScale(fits)
	for _, col := range columns {
		by, against := scale.of(col)
		worthChart(c, cp.f.Look, col.x, y+1, col.head, worthChartRows, by, against)
		c.line(col.x, y+1+worthChartRows+1, fitParts(col.width, peakForms(by, col.all).fitting(col.width)...))
	}
	return y + compareWorthUnder
}

// worthHead draws WORTH's heading along row y, naming the bucket of the
// finest of all accounts' charts that fits reports fits, the heading of
// the rows from first.
func (cp compared) worthHead(c *canvas, y, first int, fits func(buckets int) bool) {
	c.line(0, y, groupHead("WORTH", compareWorthSays+bucketWord[cp.page.All.WorthBy.Fitting(fits).Unit], false))
	c.heading(y, first)
}

// worthScaled is the scale WORTH charts Compare's columns on: the charts
// that fit, and the highest bucket of every configured account's, shown or
// not.
type worthScaled struct {
	fits func(buckets int) bool
	top  ledger.Picodollars
}

// worthScale is WORTH's scale, its charts the finest that fits reports fit.
func (cp compared) worthScale(fits func(buckets int) bool) worthScaled {
	s := worthScaled{fits: fits}
	for _, row := range cp.page.Rows {
		s.top = max(s.top, peakWorth(row.WorthBy.Fitting(fits)))
	}
	return s
}

// of is the column's chart, the finest that fits, and what it's drawn
// against: each account's against the highest of every configured
// account's, so they compare, and all accounts' against its own.
func (s worthScaled) of(col compareColumn) (views.WorthBy, ledger.Picodollars) {
	by := col.row.WorthBy.Fitting(s.fits)
	if col.all {
		return by, peakWorth(by)
	}
	return by, s.top
}

// peakForms are WORTH's peak under a column in its forms, fullest first: as
// peakParts says it, all accounts' saying its scale is its own; then
// without that; then what it was worth alone. Nothing where nothing was
// worth anything.
func peakForms(by views.WorthBy, all bool) partForms {
	named, worth, ok := peakParts(by)
	if !ok {
		return partForms{nil}
	}
	forms := partForms{{named, worth}, {worth}}
	if all {
		forms = slices.Insert(forms, 0, []part{named, worth, wordsIn(compareOwnScale, dimInk)})
	}
	return forms
}

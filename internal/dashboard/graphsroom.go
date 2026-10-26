package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/views"
)

// A burn-down section's cards across: leastCardCells, the fewest cells a card
// is across, as a week's days read ╵Mon in and a 5-hour's hours ╵13:00, each
// whole; and cardGap, the cells between two side by side.
const (
	leastCardCells = 35
	cardGap        = 4
)

// graphsGiven is all that lays Graphs out, so the page holds still as reads
// come, changing only on a key, a resize or the config: the frame's width
// and its view's rows; the sections shown; the configured accounts, in the
// config's order; and the look, which words POOL's key.
type graphsGiven struct {
	width, rows int
	shown       GraphsSections
	accounts    config.Accounts
	look        Look
}

// graphsStep is a step of the room Graphs' sections take: whether every
// card's words have a row of their own, a blank row under them, or share its
// label's; and how tall every chart is, the cards' and POOL's alike.
type graphsStep struct {
	apart bool
	chart int
}

// graphsSteps are the steps of room in the order rows to spare go to them:
// at the least, the words on the labels' rows and the charts leastChart
// tall; then the words a row of their own, in every card at once; then every
// chart a row taller at a time, to mostChart.
func graphsSteps() []graphsStep {
	steps := []graphsStep{{chart: leastChart}}
	for h := leastChart; h <= mostChart; h++ {
		steps = append(steps, graphsStep{apart: true, chart: h})
	}
	return steps
}

// cardRows are the rows a burn-down card takes at the step: its label's;
// its words' and the blank row under them, where they're apart; its chart's;
// and its axis's.
func (s graphsStep) cardRows() int {
	rows := 1 + s.chart + 1
	if s.apart {
		rows += 2
	}
	return rows
}

// glanceWhenGap are the cells between GLANCE and WHEN YOU WORK side by side.
const glanceWhenGap = 10

// graphsShape is what of Graphs' layout no step of room changes: the
// sections shown, in the page's order, each a unit of its own but GLANCE and
// WHEN YOU WORK side by side, which share one; the columns of a row of cards,
// and how many rows of them a burn-down section has, of how many accounts;
// the width; POOL as its key words it, as poolKeyed has it; GLANCE's and WHEN
// YOU WORK's columns where they sit side by side, as glanceBesideWhen lays
// them, nil where they don't; GLANCE laid out across its columns, as
// layGlance lays it; the rows WHEN YOU WORK's body takes across its columns;
// the place of the pool's line among the units, -1 where there's none; the
// legend's lines; and the rows the sections show in, over the legend.
type graphsShape struct {
	units     [][]GraphsSection
	across    []flex.Span
	accounts  int
	cardsDown int
	width     int
	pool      poolStacked
	paired    []flex.Span
	glance    glanceLaid
	when      int
	poolPart  int
	legend    []line
	room      int
}

// shape is the shape the given lays Graphs out in: GLANCE and WHEN YOU WORK
// side by side where both show and glanceBesideWhen fits them; the pool's
// line after the last burn-down section shown, where there's a pool; and the
// legend while a burn-down section shows, as burnLegend flows it, keeping
// the sections a row over it at the least.
func (g graphsGiven) shape() graphsShape {
	s := graphsShape{accounts: len(g.accounts), width: g.width, pool: poolKeyed(g.accounts, g.look), paired: g.paired(), poolPart: -1}
	last := -1
	for _, section := range EveryGraphsSection() {
		switch {
		case !g.shown.Has(section):
		case section == GraphsWhen && s.paired != nil:
			s.units[len(s.units)-1] = append(s.units[len(s.units)-1], section)
		default:
			if burnsDown(section) {
				last = len(s.units)
			}
			s.units = append(s.units, []GraphsSection{section})
		}
	}
	s.across = cardsAcross(len(g.accounts), g.width)
	s.cardsDown = (s.accounts + len(s.across) - 1) / len(s.across)
	s.glance, s.when = layGlance(g.accounts, s.columns(GraphsGlance).Size), whenRows(s.columns(GraphsWhen).Size, g.look)
	if last >= 0 {
		s.legend = burnLegend(g.width, g.reserved())
		s.legend = s.legend[:min(len(s.legend), max(g.rows-1, 0))]
	}
	if last >= 0 && pooled(g.accounts) {
		s.poolPart = last + 1
	}
	s.room = max(g.rows-len(s.legend), 0)
	return s
}

// paired are GLANCE's and WHEN YOU WORK's columns side by side, as
// glanceBesideWhen lays them, where both show and fit so; else none.
func (g graphsGiven) paired() []flex.Span {
	if !g.shown.Has(GraphsGlance) || !g.shown.Has(GraphsWhen) {
		return nil
	}
	if spans, ok := glanceBesideWhen(g.accounts, g.width); ok {
		return spans
	}
	return nil
}

// glanceBesideWhen lays GLANCE and WHEN YOU WORK side by side width cells
// wide, of the configured accounts given, reporting whether both fit at
// their naturals with glanceWhenGap between: GLANCE from column 0 at its
// natural, as glancePart has it; WHEN YOU WORK after it, from its natural,
// as hoursPart has it, to the width's end, as every section takes the width.
func glanceBesideWhen(accounts config.Accounts, width int) ([]flex.Span, bool) {
	glance, when := atNatural(glancePart(accounts)), atNatural(hoursPart())
	when.Most, when.Gap = 0, glanceWhenGap
	return flex.Lay(width, glance, when)
}

// atNatural is the part laid no narrower than its natural.
func atNatural(p flex.Part) flex.Part {
	p.Least = p.Natural
	return p
}

// columns are the columns the section takes: GLANCE's or WHEN YOU WORK's
// beside the other, where they sit side by side; else the width.
func (s graphsShape) columns(section GraphsSection) flex.Span {
	if s.paired != nil {
		switch section {
		case GraphsGlance:
			return s.paired[0]
		case GraphsWhen:
			return s.paired[1]
		}
	}
	return flex.Span{Size: s.width}
}

// reserved reports whether any of the accounts given has a reserve.
func (g graphsGiven) reserved() bool {
	return slices.ContainsFunc(g.accounts, func(a config.Account) bool { return a.Reserve > 0 })
}

// burnsDown reports whether the section is a burn-down's, WEEK or 5-HOUR.
func burnsDown(s GraphsSection) bool {
	return s == GraphsWeek || s == GraphsFiveHour
}

// cardsAcross are the columns of a row of k cards across width cells: as
// many side by side as are leastCardCells or more, cardGap apart, each as
// wide as they share the room, the odd cells left at the end, as chartRow
// lays them; narrower than one, a card across the width.
func cardsAcross(k, width int) []flex.Span {
	if spans := chartsFitting(max(k, 1), width, cardGap, leastCardCells); len(spans) > 0 {
		return spans
	}
	return []flex.Span{{Size: max(width, 0)}}
}

// poolKeyed is POOL's stack as its key words it of the configured accounts,
// whatever a pin makes the pool's: none where they're no pool, as pooled
// has it.
func poolKeyed(accounts config.Accounts, look Look) poolStacked {
	var stack views.PoolStack
	if pooled(accounts) {
		for _, a := range accounts {
			stack.Accounts = append(stack.Accounts, views.PoolStackAccount{ID: a.ID, Label: a.Label})
		}
	}
	return poolStacked{stack: stack, look: look}
}

// whenRows are the rows WHEN YOU WORK's body takes width cells wide, as its
// grid counts them at the width it's laid at, as hoursLaid lays it.
func whenRows(width int, look Look) int {
	return hoursGrid{look: look}.rows(hoursLaid(width))
}

// rowsOf are the rows the section takes at the step, its heading's and the
// blank row under it among them: a burn-down's rows of cards, a blank row
// between two; POOL's chart, its axis and its key; and GLANCE's and WHEN YOU
// WORK's bodies, each across its columns.
func (s graphsShape) rowsOf(section GraphsSection, step graphsStep) int {
	switch section {
	case GraphsPool:
		return sectionHead + s.pool.rows(s.width, step.chart)
	case GraphsGlance:
		return sectionHead + s.glance.rows()
	case GraphsWhen:
		return sectionHead + s.when
	}
	return sectionHead + s.cardsDown*step.cardRows() + max(s.cardsDown-1, 0)
}

// graphsLaid is Graphs laid out at a step of room, as laidAt lays it: the
// step; each section shown, where it lies; the pool's line's row, -1 where
// there's none; and the rows they take, from the first heading, of the shape
// they're laid in.
type graphsLaid struct {
	graphsStep
	graphsShape
	sections []graphsPlaced
	poolLine int
	rows     int
}

// graphsPlaced is a section shown as Graphs lays it out: which it is, the row
// its heading is on and the rows it takes, its heading's among them, from the
// first heading, those of the taller where two sit side by side; and the
// columns it takes, its heading across them.
type graphsPlaced struct {
	section  GraphsSection
	at, rows int
	across   flex.Span
}

// body is the section's body width cells wide, from its left: its rows
// under its heading and the blank row under that.
func (p graphsPlaced) body(width int) bounds {
	return bounds{x: p.across.At, y: p.at + sectionHead, width: width, rows: max(p.rows-sectionHead, 0)}
}

// head is the section's heading's row, across its columns.
func (p graphsPlaced) head() bounds {
	return bounds{x: p.across.At, y: p.at, width: p.across.Size, rows: 1}
}

// GraphsPoolColumns are the columns POOL's chart is across on a terminal
// width cells wide, as layGraphs lays it out: the width, as POOL takes it
// whole.
func GraphsPoolColumns(width int) int {
	return max(width, 0)
}

// laidAt lays the shape's sections out at the step, by flex, down the rows
// over the legend: each unit the rows its tallest takes, the pool's line a
// row of its own after the last burn-down, a blank row between each two.
func (s graphsShape) laidAt(step graphsStep) graphsLaid {
	parts := make([]flex.Part, 0, len(s.units)+1)
	for _, unit := range s.units {
		rows := 0
		for _, section := range unit {
			rows = max(rows, s.rowsOf(section, step))
		}
		parts = append(parts, flex.Part{Least: rows, Natural: rows, Gap: 1})
	}
	if s.poolPart >= 0 {
		parts = slices.Insert(parts, s.poolPart, flex.Part{Least: 1, Natural: 1, Gap: 1})
	}
	spans, _ := flex.Lay(s.room, parts...)
	l := graphsLaid{graphsStep: step, graphsShape: s, poolLine: -1}
	units := s.units
	for i, span := range spans {
		if i == s.poolPart {
			l.poolLine = span.At
		} else {
			for _, section := range units[0] {
				l.sections = append(l.sections, graphsPlaced{section: section, at: span.At, rows: span.Size, across: s.columns(section)})
			}
			units = units[1:]
		}
		l.rows = span.At + span.Size
	}
	return l
}

// fits reports whether the sections laid out fit the rows over the legend.
func (l graphsLaid) fits() bool {
	return l.rows <= l.room
}

// layGraphs lays Graphs out as given: at the furthest step of room whose
// sections fit over the legend, as graphsSteps orders them; or, short of the
// least, at the least, the page scrolling whole.
func layGraphs(g graphsGiven) graphsLaid {
	s := g.shape()
	steps := graphsSteps()
	laid := s.laidAt(steps[0])
	for _, step := range steps[1:] {
		next := s.laidAt(step)
		if !next.fits() {
			break
		}
		laid = next
	}
	return laid
}

// graphsLaid is Graphs as layGraphs lays it out in the frame's view.
func (f Frame) graphsLaid() graphsLaid {
	return layGraphs(graphsGiven{width: f.Width, rows: f.viewRows(), shown: f.Graphs.Sections, accounts: f.Graphs.Accounts, look: f.Look})
}

// paged is how the sections laid out, drawn whole, page a row at a time in
// the rows over the legend: each heading shown with the first row under its
// blank row, or not at all, and so each head row of GLANCE's parts, as its
// windows stacked have one each.
func (l graphsLaid) paged() rowsPaged {
	var heads []headed
	for _, s := range l.sections {
		heads = append(heads, headed{at: s.at, first: s.at + sectionHead})
		if s.section == GraphsGlance {
			for _, at := range l.glance.heads() {
				at += s.at + sectionHead
				heads = append(heads, headed{at: at, first: at + sectionHead})
			}
		}
	}
	slices.SortStableFunc(heads, func(a, b headed) int { return a.at - b.at })
	return rowsPaged{rows: l.room, content: l.rows, heads: heads}
}

// cardLaid is where a burn-down card lies, in rows from the first heading:
// its label's row, and its words', the label's where they share it; and its
// chart's bounds, its axis's row the last, as burnCard.draw takes them.
type cardLaid struct {
	label, words int
	chart        bounds
}

// cards are where the burn-down section placed lays each account's card, in
// the config's order: as many across as cardsAcross has, in their columns,
// the rest on the rows under, a blank row between two rows of cards.
func (l graphsLaid) cards(placed graphsPlaced) []cardLaid {
	cards := make([]cardLaid, l.accounts)
	per := len(l.across)
	for i := range cards {
		col, y := l.across[i%per], placed.at+sectionHead+i/per*(l.cardRows()+1)
		c := cardLaid{label: y, words: y, chart: bounds{x: col.At, y: y + 1, width: col.Size, rows: l.chart + 1}}
		if l.apart {
			c.words, c.chart.y = y+1, y+3
		}
		cards[i] = c
	}
	return cards
}

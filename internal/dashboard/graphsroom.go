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

// graphsShape is what of Graphs' layout no step of room changes: the
// sections shown, in the page's order; the columns of a row of cards, and
// how many rows of them a burn-down section has, of how many accounts; the
// rows POOL's key takes, and GLANCE's and WHEN YOU WORK's bodies; the place
// of the pool's line among the sections, -1 where there's none; the legend's
// lines; and the rows the sections show in, over the legend.
type graphsShape struct {
	shown        []GraphsSection
	across       []flex.Span
	accounts     int
	cardsDown    int
	poolKey      int
	glance, when int
	poolPart     int
	legend       []line
	room         int
}

// shape is the shape the given lays Graphs out in: the pool's line after the
// last burn-down section shown, where there's a pool; and the legend while a
// burn-down section shows, as burnLegend flows it, keeping the sections a row
// over it at the least.
func (g graphsGiven) shape() graphsShape {
	s := graphsShape{accounts: len(g.accounts), poolPart: -1}
	last := -1
	for _, section := range EveryGraphsSection() {
		if !g.shown.Has(section) {
			continue
		}
		if burnsDown(section) {
			last = len(s.shown)
		}
		s.shown = append(s.shown, section)
	}
	s.across = cardsAcross(len(g.accounts), g.width)
	s.cardsDown = (s.accounts + len(s.across) - 1) / len(s.across)
	s.poolKey = poolKeyed(g.accounts, g.look).keyRows(g.width)
	s.glance, s.when = glanceRows(s.accounts, g.pooled()), whenRows()
	if last >= 0 {
		s.legend = burnLegend(g.width, g.reserved())
		s.legend = s.legend[:min(len(s.legend), max(g.rows-1, 0))]
	}
	if last >= 0 && g.pooled() {
		s.poolPart = last + 1
	}
	s.room = max(g.rows-len(s.legend), 0)
	return s
}

// reserved reports whether any of the accounts given has a reserve.
func (g graphsGiven) reserved() bool {
	return slices.ContainsFunc(g.accounts, func(a config.Account) bool { return a.Reserve > 0 })
}

// pooled reports whether the accounts given are a pool: two or more.
func (g graphsGiven) pooled() bool {
	return len(g.accounts) > 1
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
// whatever a pin makes the pool's: none where they're no pool.
func poolKeyed(accounts config.Accounts, look Look) poolStacked {
	var stack views.PoolStack
	if len(accounts) > 1 {
		for _, a := range accounts {
			stack.Accounts = append(stack.Accounts, views.PoolStackAccount{ID: a.ID, Label: a.Label})
		}
	}
	return poolStacked{stack: stack, look: look}
}

// glanceRows are the rows GLANCE's body takes, of the accounts given, as the
// design gives them, drawn wide: its head row and a blank row; a row an
// account, a blank row between; usage's rule and the pool's row, where
// there's a pool; and a blank row and its key.
func glanceRows(accounts int, pool bool) int {
	rows := 2 + max(2*accounts-1, 0) + 2
	if pool {
		rows += 2
	}
	return rows
}

// whenRows are the rows WHEN YOU WORK's body takes, as the design gives
// them, its key on a row: the hours over the grid; a row a weekday; and a
// blank row and its key.
func whenRows() int {
	return 1 + 7 + 2
}

// rowsOf are the rows the section takes at the step, its heading's and the
// blank row under it among them: a burn-down's rows of cards, a blank row
// between two; POOL's chart, its axis and its key; and GLANCE's and WHEN YOU
// WORK's bodies.
func (s graphsShape) rowsOf(section GraphsSection, step graphsStep) int {
	switch section {
	case GraphsPool:
		return sectionHead + step.chart + runwayAxisRows + s.poolKey
	case GraphsGlance:
		return sectionHead + s.glance
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
// first heading.
type graphsPlaced struct {
	section  GraphsSection
	at, rows int
}

// laidAt lays the shape's sections out at the step, by flex, down the rows
// over the legend: each the rows it takes, the pool's line a row of its own
// after the last burn-down, a blank row between each two.
func (s graphsShape) laidAt(step graphsStep) graphsLaid {
	parts := make([]flex.Part, 0, len(s.shown)+1)
	for _, section := range s.shown {
		rows := s.rowsOf(section, step)
		parts = append(parts, flex.Part{Least: rows, Natural: rows, Gap: 1})
	}
	if s.poolPart >= 0 {
		parts = slices.Insert(parts, s.poolPart, flex.Part{Least: 1, Natural: 1, Gap: 1})
	}
	spans, _ := flex.Lay(s.room, parts...)
	l := graphsLaid{graphsStep: step, graphsShape: s, poolLine: -1}
	for i, span := range spans {
		if i == s.poolPart {
			l.poolLine = span.At
		} else {
			l.sections = append(l.sections, graphsPlaced{section: s.shown[len(l.sections)], at: span.At, rows: span.Size})
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
// blank row, or not at all.
func (l graphsLaid) paged() rowsPaged {
	heads := make([]headed, len(l.sections))
	for i, s := range l.sections {
		heads[i] = headed{at: s.at, first: s.at + sectionHead}
	}
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

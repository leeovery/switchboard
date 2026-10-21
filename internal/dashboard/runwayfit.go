package dashboard

import "github.com/leeovery/switchboard/internal/dashboard/flex"

// runwayStep is a step of Runway's compaction, each taken as rows run short
// for the one before it.
type runwayStep int

const (
	// runwayRoomy is three rows an account, its lane, its words and a blank
	// row; blank rows under the axis, under the pool and over the legend;
	// and every line of COMING UP.
	runwayRoomy runwayStep = iota
	// runwayTwoRows is step (a): two rows an account.
	runwayTwoRows
	// runwayNoGaps is step (b): no blank rows under the pool or over the
	// legend.
	runwayNoGaps
	// runwayComingCut is step (c): COMING UP showing the lines it can, from
	// one and how many more.
	runwayComingCut
	// runwayAxisClose is step (d): no blank row between the axis and the
	// lanes.
	runwayAxisClose
	// runwayScrolled is step (e): (d)'s layout, its lanes scrolling, as many
	// shown as fit.
	runwayScrolled
	// runwayOneLane is step (f): (e)'s layout, one lane shown, the legend
	// and then COMING UP giving way in the turns below.
	runwayOneLane
)

// The turns Runway's parts give way in at (f): the legend, then COMING UP,
// heading and all.
const (
	legendGivesWay = iota + 1
	comingGivesWay
)

// The rows Runway's parts take: the axis's, now, its labels and its ticks;
// and the pool's lane's.
const (
	runwayAxisRows = 3
	runwayPoolRows = 2
)

// Runway's parts down a frame, in runwayShape.parts' order: the axis; the
// lanes shown; the pool's lane, the row under it its room's, or what's out
// of view's; COMING UP; the rows spare; and the legend, over the key line.
const (
	runwayAxisPart = iota
	runwayLanesPart
	runwayPoolPart
	runwayComingPart
	runwaySparePart
	runwayLegendPart
)

// noRow is the row of a part Runway doesn't draw.
const noRow = -1

// runwayShape is how Runway's rows go on a frame: how many rows the legend
// takes.
type runwayShape struct {
	legend int
}

// runwayHolds are what Runway lays out: how many lanes; whether the pool's
// lane shows, as it doesn't with one account, and the pool's room, as it
// doesn't on a day or a week stepped back to that has no now; and whether
// COMING UP shows, as it does live alone, and how many lines it lists.
type runwayHolds struct {
	lanes              int
	pool, room, coming bool
	lines              int
}

// holds are what Runway shown as r lays out, as runwayHolds has them: none
// before its lanes are built.
func (r RunwayShown) holds() runwayHolds {
	if r.Lanes == nil {
		return runwayHolds{}
	}
	pool := r.Lanes.Pool.Of > 0
	return runwayHolds{
		lanes: len(r.Lanes.Accounts), pool: pool, room: pool && r.Timeline.NowColumn() >= 0,
		coming: r.Back.IsZero(), lines: len(r.Coming),
	}
}

// poolRows are the rows the pool's lane takes, shown of the lanes shown: its
// own, and the row under them where it has a room, or where lanes are out of
// view, which that row says too; with no pool's lane, that row alone.
func (h runwayHolds) poolRows(shown int) int {
	rows := 0
	if h.pool {
		rows = runwayPoolRows
	}
	if h.room || shown < h.lanes {
		rows++
	}
	return rows
}

// runwayFit is how Runway fits the rows from the view's top to the one over
// the key line: at the step of its compaction taken, the row each of its
// parts starts on, or noRow where it isn't drawn. The zero runwayFit fits
// nothing, as Runway draws the page's chrome alone.
type runwayFit struct {
	step runwayStep
	// fits is set where Runway draws anything.
	fits bool
	// axis is the axis's first row, now's, its labels and its ticks on the
	// two under it.
	axis int
	// lanes is the first lane shown's row, each account taking apart rows
	// from its lane's, its words on the row under it; first and shown are
	// the accounts shown, in the moment's order, from the first, of
	// accounts.
	lanes, apart           int
	first, shown, accounts int
	// pool is the pool's lane's first row, Σ pool's; room is its room's, and
	// out the row saying what's out of view, each the row under the lane.
	pool, room, out int
	// coming is COMING UP's heading's row, lines the rows of its lines under
	// it; legend is the legend's first row.
	coming, lines, legend int
}

// runwayFitted is how Runway fits the rows from row top to the one over the
// key line, as runwayFittedAs fits them: the legend as many rows as
// runwayLegend draws.
func (f Frame) runwayFitted(top int) runwayFit {
	return f.runwayFittedAs(top, runwayShape{legend: f.runwayLegendRows()})
}

// runwayFittedAs is how Runway fits the rows from row top to the one over
// the key line in the shape given: the first of its compaction's steps whose
// parts fit there, every lane shown; else (e), as many lanes as fit shown;
// else (f), one; each from the one the frame's Scroll scrolls to; else, where
// not even (f) fits, as before the lanes are built, nothing.
func (f Frame) runwayFittedAs(top int, shape runwayShape) runwayFit {
	holds, rows := f.Runway.holds(), f.Height-underView+1-top
	if holds.lanes == 0 {
		return runwayFit{}
	}
	for step := range runwayScrolled {
		if fit, ok := shape.laidOut(step, top, rows, holds, holds.lanes); ok {
			return fit
		}
	}
	for shown := holds.lanes - 1; shown > 0; shown-- {
		if fit, ok := shape.laidOut(runwayScrolled, top, rows, holds, shown); ok {
			return f.scrolledTo(fit)
		}
	}
	if fit, ok := shape.laidOut(runwayOneLane, top, rows, holds, 1); ok {
		return f.scrolledTo(fit)
	}
	return runwayFit{}
}

// scrolledTo is the fit with its lanes shown from the one the frame's Scroll
// scrolls to, no further than shows the last.
func (f Frame) scrolledTo(fit runwayFit) runwayFit {
	fit.first = Scrolling{Most: fit.accounts - fit.shown}.clamped(f.Scroll)
	return fit
}

// laidOut is Runway laid out in rows rows from row top at the step given, in
// the shape, of what holds says it holds, shown of its lanes shown, from the
// first, each part where flex lays it, reporting false where they don't fit.
func (s runwayShape) laidOut(step runwayStep, top, rows int, holds runwayHolds, shown int) (runwayFit, bool) {
	at, fits := flex.Lay(rows, s.parts(step, holds, shown)...)
	if !fits {
		return runwayFit{}, false
	}
	pool, coming, legend := top+at[runwayPoolPart].At, at[runwayComingPart], at[runwayLegendPart]
	under := pool
	if holds.pool {
		under += runwayPoolRows
	}
	return runwayFit{
		step: step, fits: true, axis: top + at[runwayAxisPart].At,
		lanes: top + at[runwayLanesPart].At, apart: step.apart(), shown: shown, accounts: holds.lanes,
		pool:   rowWhere(holds.pool, pool),
		room:   rowWhere(holds.room, under),
		out:    rowWhere(shown < holds.lanes, under),
		coming: rowWhere(holds.coming && !coming.Left, top+coming.At), lines: max(coming.Size-1, 0),
		legend: rowWhere(!legend.Left, top+legend.At),
	}, true
}

// parts are Runway's parts down a frame at the step given, in the shape, of
// what holds says it holds, shown of its lanes shown, as flex lays them: the
// axis; the lanes, a blank row before them before (d); the pool's lane, its
// room and what's out of view, as poolRows has them; COMING UP, as
// comingPart has it; the rows spare, which take what's left; and the
// legend, over the key line, a blank row before it before (b), and with no
// COMING UP, the blank row under the pool's lane. At (f), the legend gives
// way, then COMING UP.
func (s runwayShape) parts(step runwayStep, holds runwayHolds, shown int) []flex.Part {
	lanes, pool := shown*step.apart(), holds.poolRows(shown)
	parts := []flex.Part{
		runwayAxisPart:   {Least: runwayAxisRows, Natural: runwayAxisRows},
		runwayLanesPart:  {Least: lanes, Natural: lanes, Gap: step.gap(runwayAxisClose)},
		runwayPoolPart:   {Least: pool, Natural: pool},
		runwayComingPart: step.comingPart(holds),
		runwaySparePart:  {Grows: true},
		runwayLegendPart: {Least: s.legend, Natural: s.legend, Gap: step.gap(runwayNoGaps)},
	}
	if !holds.coming {
		parts[runwayLegendPart].Gap = step.underPool(holds)
	}
	if step == runwayOneLane {
		parts[runwayLegendPart].Leaves = legendGivesWay
		parts[runwayComingPart].Leaves = comingGivesWay
	}
	return parts
}

// comingPart is COMING UP as a part at the step, of what holds says it
// holds, where it shows: its heading and every line; from (c), from its
// heading, a line and how many more, to every line; the blank row under the
// pool's lane before it.
func (s runwayStep) comingPart(holds runwayHolds) flex.Part {
	if !holds.coming {
		return flex.Part{}
	}
	natural, least := 1+holds.lines, 1+min(holds.lines, 2)
	if s < runwayComingCut {
		least = natural
	}
	return flex.Part{Least: least, Natural: natural, Gap: s.underPool(holds)}
}

// underPool is the blank rows under the pool's lane at the step, of what
// holds says it holds: one before (b), but at the roomiest with no pool's
// lane, where the last lane's own blank row is it.
func (s runwayStep) underPool(holds runwayHolds) int {
	if s == runwayRoomy && !holds.pool {
		return 0
	}
	return s.gap(runwayNoGaps)
}

// gap is a blank row before the step that closes it, else none.
func (s runwayStep) gap(closes runwayStep) int {
	if s < closes {
		return 1
	}
	return 0
}

// apart is how many rows an account takes at the step: three at the
// roomiest, else two.
func (s runwayStep) apart() int {
	if s == runwayRoomy {
		return 3
	}
	return 2
}

// rowWhere is the row given where the part is drawn, else noRow.
func rowWhere(drawn bool, row int) int {
	if !drawn {
		return noRow
	}
	return row
}

// draws reports whether the fit draws the part on the row given: whether
// anything fits, and the part is drawn.
func (r runwayFit) draws(row int) bool {
	return r.fits && row != noRow
}

// laneRow is the row the lane of the i-th account, in the moment's order, is
// drawn on, its words on the row under it, reporting false where it isn't
// drawn: scrolled out of view, or where nothing fits.
func (r runwayFit) laneRow(i int) (int, bool) {
	if i < r.first || i >= r.first+r.shown {
		return 0, false
	}
	return r.lanes + (i-r.first)*r.apart, true
}

// scrolling is how far Runway's lanes scroll, at any offset, as fitted: an
// account at a time, as far as the last shows, as many at once as are
// shown.
func (r runwayFit) scrolling(int) Scrolling {
	return Scrolling{Most: r.accounts - r.shown, Page: r.shown}
}

// lanesOut are the forms of the line saying how many of Runway's lanes are
// out of view, in text.subtle, as a scroll line is: "▲ 2 more accounts · ▼ 1
// more account", and for less room, the counts alone, "▲ 2 more · ▼ 1 more".
var lanesOut = []outForm{{noun: "account", gap: " · ", inks: subtleOut}, {gap: " · ", inks: subtleOut}}

// runwayOut draws, where Runway's lanes scroll, how many are out of view
// above and below, as in "▼ 1 more account", right-aligned along the row
// under the pool's lane, in the cells its room leaves, nameGap after it.
func (f Frame) runwayOut(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.out) {
		return
	}
	width := f.Width
	if x, room := f.roomSaid(laid); room != nil {
		width -= x + room.width() + nameGap
	}
	c.right(f.Width, fit.out, outOfView(fit.first, fit.accounts-fit.first-fit.shown, width, lanesOut...))
}

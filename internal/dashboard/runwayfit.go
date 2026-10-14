package dashboard

// runwayStep is a step of Runway's compaction, each taken as rows run short
// for the one before it.
type runwayStep int

const (
	// runwayRoomy is three rows an account, its lane, its words and a blank
	// row, and blank rows under the axis and round the legend and COMING UP.
	runwayRoomy runwayStep = iota
	// runwayTwoRows is step (a): two rows an account, and the pool's rule on
	// a row of its own.
	runwayTwoRows
	// runwayNoGaps is step (b): no blank rows round the legend and COMING
	// UP.
	runwayNoGaps
	// runwayFolded is step (c): COMING UP folded to its heading.
	runwayFolded
	// runwayAxisClose is step (d): no blank row between the axis and the
	// lanes.
	runwayAxisClose
	// runwayScrolled is step (e): (d)'s layout, its lanes scrolling, as many
	// shown as fit.
	runwayScrolled
)

// The rows Runway's parts take: the axis's, now, its labels and its ticks;
// and the pool's lane's.
const (
	runwayAxisRows = 3
	runwayPoolRows = 2
)

// noRow is the row of a part Runway doesn't draw.
const noRow = -1

// runwayShape is how Runway's rows go on a frame: whether the pool's room
// takes a row of its own under the pool's words, as on a phone, whose label
// column is too narrow for it; whether a blank row goes before the legend,
// where the step keeps blank rows round it, as none does on a phone; and how
// many rows the legend takes.
type runwayShape struct {
	roomRow, legendGap bool
	legend             int
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
	// rule, pool and words are the rows of the pool's rule, its lane's first
	// and its words; room is its room's: the words' row, or the row under
	// it, as the shape has it.
	rule, pool, words, room int
	// legend is the legend's first row; coming COMING UP's heading's, folded
	// where it's folded to its heading, else with lines lines under it.
	legend, coming, lines int
	folded                bool
	// end is the row after the last; out the row over the key line, where
	// what's out of view is said at (e).
	end, out int
}

// runwayFitted is how Runway fits the rows from row top to the one over the
// key line on a frame from phoneUnder columns, as runwayFittedAs fits them:
// its room on its words' row, blank rows round the legend, and the legend as
// many rows as runwayLegend draws.
func (f Frame) runwayFitted(top int) runwayFit {
	return f.runwayFittedAs(top, runwayShape{legendGap: true, legend: f.runwayLegendRows()})
}

// runwayFittedAs is how Runway fits the rows from row top to the one over
// the key line in the shape given: the first of its compaction's steps whose
// last row is two above the key line or higher, every lane shown; else (e),
// as many lanes as fit shown, from the one the frame's Scroll scrolls to;
// else, where not even one fits, as before the lanes are built, nothing.
func (f Frame) runwayFittedAs(top int, shape runwayShape) runwayFit {
	holds, over := f.Runway.holds(), f.Height-underView
	if holds.lanes == 0 {
		return runwayFit{}
	}
	for step := range runwayScrolled {
		if fit := shape.laidOut(step, top, holds, 0, holds.lanes); fit.end <= over {
			return fit
		}
	}
	least := shape.laidOut(runwayScrolled, top, holds, 0, 0)
	shown := (over - least.end) / least.apart
	if shown < 1 {
		return runwayFit{}
	}
	first := Scrolling{Most: holds.lanes - shown}.clamped(f.Scroll)
	fit := shape.laidOut(runwayScrolled, top, holds, first, shown)
	fit.out = over
	return fit
}

// laidOut is Runway laid out from row top at the step given, in the shape,
// of what holds says it holds, its lanes shown from the first given, shown
// of them: the axis; a blank row, before (d); the lanes; the pool's rule, on
// the last lane's blank row while each takes three, its lane, its words and
// its room; a blank row, while the step keeps them and the shape has one,
// where the row before isn't blank already; the legend; and a blank row,
// while the step keeps them, then COMING UP.
func (s runwayShape) laidOut(step runwayStep, top int, holds runwayHolds, first, shown int) runwayFit {
	fit := runwayFit{
		step: step, fits: true, axis: top, apart: 2, first: first, shown: shown, accounts: holds.lanes,
		rule: noRow, pool: noRow, words: noRow, room: noRow, coming: noRow, out: noRow, end: top + runwayAxisRows,
	}
	if step < runwayAxisClose {
		fit.take(1)
	}
	if step == runwayRoomy {
		fit.apart = 3
	}
	fit.lanes = fit.take(shown * fit.apart)
	if holds.pool {
		fit.layPool(step == runwayRoomy, s.roomRow, holds.room)
	}
	roomy := step < runwayNoGaps
	if blank := step == runwayRoomy && !holds.pool; roomy && s.legendGap && !blank {
		fit.take(1)
	}
	fit.legend = fit.take(s.legend)
	if holds.coming {
		if roomy {
			fit.take(1)
		}
		fit.coming, fit.folded = fit.take(1), step >= runwayFolded
		if !fit.folded {
			fit.lines = holds.lines
			fit.take(fit.lines)
		}
	}
	return fit
}

// layPool lays the pool's lane out at the fit's end: its rule, on the last
// lane's blank row where it's onBlank, else on a row of its own; its lane;
// its words; and where roomed, its room, on the words' row, or with roomRow,
// on a row of its own under them.
func (r *runwayFit) layPool(onBlank, roomRow, roomed bool) {
	if onBlank {
		r.rule = r.end - 1
	} else {
		r.rule = r.take(1)
	}
	r.pool = r.take(runwayPoolRows)
	r.words = r.take(1)
	switch {
	case roomed && roomRow:
		r.room = r.take(1)
	case roomed:
		r.room = r.words
	}
}

// take is the fit's end, the rows given taken from it.
func (r *runwayFit) take(rows int) int {
	y := r.end
	r.end += rows
	return y
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

// runwayOut draws, where Runway's lanes scroll at (e), how many are out of
// view above and below, right-aligned along the row over the key line, as in
// "▼ 1 more account".
func (f Frame) runwayOut(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.out) {
		return
	}
	c.right(f.Width, fit.out, outOfView(fit.first, fit.accounts-fit.first-fit.shown, f.Width, lanesOut...))
}

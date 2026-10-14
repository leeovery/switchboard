package views

import (
	"slices"
	"time"
)

// Pool is the pool's lane: Of, how many accounts the pool has, the pin's
// while it holds, none where there's one account, which has no pool's lane;
// Key, the key of the window the view is named for, whose room the pool's
// room line gives; each column's count of them with room; and each dip,
// where fewer than all of them have room, in order.
type Pool struct {
	Of      int
	Key     string
	Columns []PoolColumn
	Dips    []Dip
}

// PoolColumn is a column of the pool's lane: how many of its accounts have
// room at the column's moment, one whose state isn't known counting without;
// and whether the state of any is known, the column blank where none's is.
type PoolColumn struct {
	With  int
	Known bool
}

// Dip is a run of the pool's columns where fewer than all its accounts have
// room, those known, from First to Last: each step of its count, in order;
// Column, where its words start, the column its first step's start falls
// in; Over, set where it ended before now's column, as a live view doesn't
// say; and Ahead, set where it begins after now's.
type Dip struct {
	Steps               []Step
	Column, First, Last int
	Over, Ahead         bool
}

// Step is a run of a dip's columns, from First to Last, where the same count
// of the pool's accounts have room, With, from when its count came to it,
// till when it changed, each zero where that isn't known.
type Step struct {
	With        int
	From, Until Edge
	First, Last int
}

// Edge is when a pool's count changed: as an account's stretch without room
// began, or ended, Projected where a run-out projects it, as ~ says.
type Edge struct {
	At        time.Time
	Projected bool
}

// poolOf is the pool's lane over the accounts' lanes given, as Pool has it:
// none where there's one account.
func (from LanesFrom) poolOf(lanes []Lane) Pool {
	if len(lanes) < 2 {
		return Pool{}
	}
	in := from.Moment.Pool().Accounts
	members := slices.DeleteFunc(slices.Clone(lanes), func(l Lane) bool { return !slices.Contains(in, l.ID) })
	pool := Pool{Of: len(members), Key: from.viewKey(), Columns: make([]PoolColumn, max(from.Timeline.Columns, 0))}
	for c := range pool.Columns {
		for _, l := range members {
			pool.Columns[c].Known = pool.Columns[c].Known || l.Cells[c] != CellUnknown
			if l.Cells[c].HasRoom() {
				pool.Columns[c].With++
			}
		}
	}
	pool.Dips = pool.dipsOver(members, from.Timeline)
	return pool
}

// dipsOver are the pool's dips, as Dip has them, over its accounts' lanes
// given, on the timeline given.
func (p Pool) dipsOver(members []Lane, tl Timeline) []Dip {
	var dips []Dip
	for c, col := range p.Columns {
		if !col.Known || col.With >= p.Of {
			continue
		}
		until, from := edgesAt(members, c)
		n := len(dips)
		if n == 0 || dips[n-1].Last != c-1 {
			dips = append(dips, Dip{First: c, Last: c, Steps: []Step{{With: col.With, From: from, First: c, Last: c}}})
			continue
		}
		d := &dips[n-1]
		d.Last = c
		if s := &d.Steps[len(d.Steps)-1]; s.With == col.With {
			s.Last = c
		} else {
			s.Until = until
			d.Steps = append(d.Steps, Step{With: col.With, From: from, First: c, Last: c})
		}
	}
	now := tl.NowColumn()
	for i := range dips {
		d := &dips[i]
		d.Steps[len(d.Steps)-1].Until, _ = edgesAt(members, d.Last+1)
		d.Column = d.First
		if start := d.Steps[0].From.At; !start.IsZero() {
			d.Column = max(tl.Column(start), 0)
		}
		d.Over, d.Ahead = d.Last < now, now >= 0 && d.First > now
	}
	return dips
}

// edgesAt are the first and the last change of the pool's count between
// column c and the one before it, where its accounts' lanes given say when:
// each stretch without room beginning at c, or ending before it, where its
// account has room on the other side, the first column having none before
// it and the last none after it.
func edgesAt(members []Lane, c int) (first, last Edge) {
	for _, l := range members {
		e, ok := l.changeAt(c)
		if !ok || e.At.IsZero() {
			continue
		}
		if first.At.IsZero() || e.At.Before(first.At) {
			first = e
		}
		if last.At.IsZero() || e.At.After(last.At) {
			last = e
		}
	}
	return first, last
}

// changeAt is when the lane's room changed between column c and the one
// before it, reporting false where it didn't: as the stretch it has no room
// through from c began, where it had room before; or as the stretch before c
// ended, where it has room from c.
func (l Lane) changeAt(c int) (Edge, bool) {
	roomBefore, roomAt := c > 0 && l.Cells[c-1].HasRoom(), c < len(l.Cells) && l.Cells[c].HasRoom()
	switch {
	case c < len(l.Cells) && !roomAt && (c == 0 || roomBefore):
		if i := slices.IndexFunc(l.Stretches, func(s LaneStretch) bool { return s.First == c }); i >= 0 {
			return Edge{At: l.Stretches[i].Start, Projected: l.Stretches[i].Projected}, true
		}
	case c > 0 && !roomBefore && (c == len(l.Cells) || roomAt):
		if i := slices.IndexFunc(l.Stretches, func(s LaneStretch) bool { return s.Last == c-1 }); i >= 0 {
			return Edge{At: l.Stretches[i].Back}, true
		}
	}
	return Edge{}, false
}

// Drawn are what the lanes draw, as the legend keys it: the cells, in the
// legend's order, a blank one not among them; and whether any marks a
// reset.
type Drawn struct {
	Cells  []Cell
	Resets bool
}

// drawnOf are what the lanes given draw, as Drawn has it.
func drawnOf(lanes []Lane) Drawn {
	var d Drawn
	for _, l := range lanes {
		for _, c := range l.Cells {
			if c != CellUnknown && !slices.Contains(d.Cells, c) {
				d.Cells = append(d.Cells, c)
			}
		}
		d.Resets = d.Resets || len(l.Resets) > 0
	}
	slices.Sort(d.Cells)
	return d
}

package views

import "slices"

// Pool is the pool's lane: Of, how many accounts the pool has, the pin's
// while it holds, none where there's one account, which has no pool's lane;
// Key, the key of the window the view is named for, whose room the pool's
// room line gives; and each column's count of them with room.
type Pool struct {
	Of      int
	Key     string
	Columns []PoolColumn
}

// PoolColumn is a column of the pool's lane: how many of its accounts have
// room at the column's moment, one whose state isn't known counting without;
// and whether the state of any is known, the column blank where none's is.
type PoolColumn struct {
	With  int
	Known bool
}

// Limited reports whether none of the column's accounts has room, of those
// whose state is known: the pool at its limit then.
func (c PoolColumn) Limited() bool {
	return c.Known && c.With == 0
}

// Limited reports whether any of the pool's columns is at its limit, as
// PoolColumn.Limited has it.
func (p Pool) Limited() bool {
	return slices.ContainsFunc(p.Columns, PoolColumn.Limited)
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
	return pool
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

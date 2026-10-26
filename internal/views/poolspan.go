package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// PoolSpan is the pool's room in a window over its span, as Graphs' GLANCE
// draws it beside Σ pool: Timeline, the columns it's charted across, from the
// earliest start of its accounts' windows to the latest of their resets, as
// each window's Start and Length have them; Key, the window's; Of, how many
// accounts the pool has; and its room in each column, its accounts' summed
// in accounts' worth, each as PoolStack counts it, known where any's is, and
// the share of all of theirs that leaves used, as status.PoolUsed has it. It
// has no columns where there's no pool, as with one account, or the pool has
// no such window, or none of its accounts' spans in it is known, or no
// columns. Each look's is built once, by Look, from the last look's, which
// holds each column it judged before now's while what that's judged from
// stands.
type PoolSpan struct {
	Timeline Timeline
	Key      string
	Of       int
	Columns  []PoolRoomColumn

	// then is the room an account had left in a window at a moment gone, as
	// status.Moment.RoomThen has it where it's nil; and past is what was
	// judged before now's column, held.
	then roomJudge
	past heldRoom
}

// Look is the pool's room in the window over its span at a look, as
// PoolSpan has it, of the readings given, read through when given, charted
// across the columns given: each column's room summed of its accounts', as
// PoolStack's Look counts each, before now's at the column's moment, held
// from p where what that's judged from by then hasn't changed; where there's
// none, it holds what p judged, for a look once there is again.
func (p PoolSpan) Look(from PoolRoomFrom) PoolSpan {
	m := from.Moment
	held := PoolSpan{then: p.then, past: p.past}
	tl, ok := poolSpanOf(m, from.Key, from.Columns)
	if !ok {
		return held
	}
	accounts, past, ok := p.past.stacked(pastFrom{moment: m, readings: from.Readings, through: from.ReadThrough, key: from.Key}, tl, p.then.orRoomThen())
	if !ok {
		return held
	}
	span := PoolSpan{Timeline: tl, Key: from.Key, Of: len(m.Pool().Accounts), Columns: make([]PoolRoomColumn, tl.Columns), then: p.then, past: past}
	rooms := make([]AccountRoom, len(accounts))
	for c := range span.Columns {
		for i, a := range accounts {
			rooms[i] = a.Rooms[c]
		}
		col := poolRoomOf(rooms)
		if col.Known {
			col.Used = status.PoolUsed(col.Room, span.Of)
		}
		span.Columns[c] = col
	}
	return span
}

// poolSpanOf is the span of the pool of the moment m in the window with the
// given key, across columns columns, now where it falls: from the earliest
// start of its accounts' windows with that key to the latest of their
// resets, of those whose span is known, as their Start and Length have it.
// It reports false where none's is.
func poolSpanOf(m status.Moment, key string, columns int) (Timeline, bool) {
	var start, end time.Time
	for _, id := range m.Pool().Accounts {
		a, _ := m.Account(id)
		w, _ := a.Window(key)
		if w.Length <= 0 {
			continue
		}
		reset := w.Start.Add(w.Length)
		if start.IsZero() || w.Start.Before(start) {
			start = w.Start
		}
		if reset.After(end) {
			end = reset
		}
	}
	return Timeline{Start: start, End: end, Columns: columns, Now: m.Now}, !start.IsZero()
}

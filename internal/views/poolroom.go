package views

import (
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// PoolRoom is the pool's room in a window over its length back from now, as
// GRAPHS charts it beside Σ pool by account: Of, how many accounts the pool
// has, none where there's no pool, as with one account; and each column's.
// Each look's is built once, by Look, from the last look's, which holds each
// column it judged before now's until what that's judged from changes.
type PoolRoom struct {
	Of      int
	Columns []PoolRoomColumn

	// then is the room an account had left in a window at a moment gone, as
	// status.Moment.RoomThen has it where it's nil; and past is what was
	// judged before now's column, held.
	then roomJudge
	past heldRoom
}

// roomJudge is the room an account had left in a window at a moment gone, as
// status.Moment.RoomThen has it.
type roomJudge func(m status.Moment, a status.AccountNow, read status.Readings, told []status.Span, key string, t time.Time) (float64, bool)

// PoolRoomColumn is the pool's room at a column's moment: the room its
// accounts had left in the window then, summed in accounts' worth; the share
// of all of theirs that leaves used, as Σ pool's bar fills to, as
// status.PoolUsed has it; and whether how any of them stood then is known,
// the column blank where none's is, or it isn't judged yet.
type PoolRoomColumn struct {
	Room, Used float64
	Known      bool
}

// PoolRoomFrom is what the pool's room in a window is built from at a look:
// the moment; the readings history's readings, oldest first, as Runway's
// lanes take them, each window's run before them carried in, as
// readings.CarriedRuns has them; ReadThrough, when the read of the history
// that gave them was asked, so it gave every reading by then, zero where
// none has been read; the window's key; and how many columns it's charted
// across.
type PoolRoomFrom struct {
	Moment      status.Moment
	Readings    []readings.Reading
	ReadThrough time.Time
	Key         string
	Columns     int
}

// Look is the pool's room in the window at a look, as PoolRoom has it, over
// its length back from Now, a column every step, as poolStep has it: each
// column before now's at its moment, on a grid of those steps, the last at
// PoolRoomThrough's, so each stays at its moment as now moves on, as
// status.Moment.RoomThen judges each of the pool's accounts had room then,
// summed, held from p where what that's judged from by then hasn't changed,
// and not judged, so not known, where it's after the readings were read
// through, till a read since gives what they gained by then; and the last,
// now's, the pool's room as the moment has it, Σ pool's. It's none where the
// window's length isn't known, and where the pool has no such window, still
// holding what p judged, for a look once it has again.
func (p PoolRoom) Look(from PoolRoomFrom) PoolRoom {
	step, ok := poolStep(from.Key, from.Columns)
	if !ok {
		return PoolRoom{}
	}
	pool := from.Moment.Pool()
	i := slices.IndexFunc(pool.Windows, func(w status.PoolWindowNow) bool { return w.Key == from.Key })
	if i < 0 {
		return PoolRoom{then: p.then, past: p.past}
	}
	past := p.past.judged(from, step, p.judging())
	room := PoolRoom{Of: len(pool.Accounts), Columns: make([]PoolRoomColumn, 0, from.Columns), then: p.then, past: past}
	for _, c := range past.columns {
		if c.room.Known {
			c.room.Used = status.PoolUsed(c.room.Room, room.Of)
		}
		room.Columns = append(room.Columns, c.room)
	}
	now := pool.Windows[i]
	room.Columns = append(room.Columns, PoolRoomColumn{Room: now.Room, Used: now.Used, Known: true})
	return room
}

// PoolRoomThrough is the moment of the last column before now's of the
// pool's room in the window with the given key, charted across columns
// columns, at now, as Look lays them on the grid of its steps: each column is
// judged by the readings history's readings by its moment, so the last waits
// for a read of it asked then or since. It's zero where the window's length
// isn't known, or there are no columns.
func PoolRoomThrough(key string, columns int, now time.Time) time.Time {
	step, ok := poolStep(key, columns)
	if !ok {
		return time.Time{}
	}
	return now.Truncate(step)
}

// poolStep is how far apart the columns of the pool's room in the window with
// the given key are, charted across columns columns: its length over those
// before now's, so the first, the room at its end's moment, starts a length
// before the last of them ends, and so by a length back from now, where the
// axis says the chart starts; its length where now's is the only one. It
// reports false where the length isn't known, or there are no columns.
func poolStep(key string, columns int) (time.Duration, bool) {
	length, ok := quota.Length(key)
	if !ok || columns < 1 {
		return 0, false
	}
	return length / time.Duration(max(columns-1, 1)), true
}

// judging is how p judges the room an account had left in a window at a
// moment gone: its judge, else status.Moment.RoomThen.
func (p PoolRoom) judging() roomJudge {
	if p.then == nil {
		return status.Moment.RoomThen
	}
	return p.then
}

// heldRoom is what was judged of the pool's room in a window before now's
// column, and what that was judged from: the window's key; the step its
// columns are apart; the policy; each of the pool's accounts, as roomOf
// knows it, in the pool's order; and each column, as heldColumn has it.
type heldRoom struct {
	key      string
	step     time.Duration
	policy   score.Policy
	accounts []roomOf
	columns  []heldColumn
}

// heldColumn is a column of the pool's room before now's: its moment; the
// readings by then, as heldReadings knows them; whether it was judged; and
// its room, as judged.
type heldColumn struct {
	at     time.Time
	read   heldReadings
	judged bool
	room   PoolRoomColumn
}

// roomOf is what RoomThen reads of an account besides its readings: its id,
// whether its token is usable, its reserve, whether it holds, and where it
// runs out; and the holds the router told of.
type roomOf struct {
	id           string
	tokenSet     bool
	reserve      float64
	reserveHolds bool
	floor        float64
	told         []status.Span
}

// roomsOf are the pool's accounts in the moment given, as roomOf knows them,
// in the pool's order.
func roomsOf(m status.Moment) []roomOf {
	var of []roomOf
	for _, id := range m.Pool().Accounts {
		if a, ok := m.Account(id); ok {
			of = append(of, roomOf{id: a.ID, tokenSet: a.TokenSet, reserve: a.Reserve, reserveHolds: a.ReserveHolds, floor: a.Floor, told: m.Told(a.ID)})
		}
	}
	return of
}

// is reports whether o and r are the one account, as RoomThen reads it.
func (o roomOf) is(r roomOf) bool {
	return o.id == r.id && o.tokenSet == r.tokenSet && o.reserve == r.reserve && o.reserveHolds == r.reserveHolds && o.floor == r.floor &&
		slices.EqualFunc(o.told, r.told, sameSpan)
}

// judged is the pool's room before now's column at the look from gives, its
// columns step apart, as heldRoom has it: each column held from h where it
// was judged at its moment from what it's judged from now, the readings by
// then the same, as RoomThen reads none after; else judged afresh, as roomAt
// has it, of the readings indexed once, where they were read through its
// moment; else left not judged.
func (h heldRoom) judged(from PoolRoomFrom, step time.Duration, then roomJudge) heldRoom {
	m := from.Moment
	past := heldRoom{key: from.Key, step: step, policy: m.Policy(), accounts: roomsOf(m), columns: make([]heldColumn, from.Columns-1)}
	alike, end := h.alike(past), PoolRoomThrough(from.Key, from.Columns, m.Now)
	read := sync.OnceValue(func() status.Readings { return status.Index(from.Readings) })
	for c := range past.columns {
		col := heldColumn{at: end.Add(-time.Duration(len(past.columns)-1-c) * step)}
		col.read = readBy(from.Readings, col.at)
		switch held, ok := h.columnAt(col.at); {
		case alike && ok && held.judged && held.read == col.read:
			col = held
		case !col.at.After(from.ReadThrough):
			col.room, col.judged = past.roomAt(m, read(), then, col.at), true
		}
		past.columns[c] = col
	}
	return past
}

// alike reports whether what h was judged from is what past is, but for its
// readings and its columns' moments: the window, its step, the policy and
// the pool's accounts.
func (h heldRoom) alike(past heldRoom) bool {
	return h.key == past.key && h.step == past.step && h.policy.Equal(past.policy) && slices.EqualFunc(h.accounts, past.accounts, roomOf.is)
}

// columnAt is the column h holds at the moment t, on the grid of its steps,
// reporting false where it holds none then.
func (h heldRoom) columnAt(t time.Time) (heldColumn, bool) {
	if h.step <= 0 || len(h.columns) == 0 {
		return heldColumn{}, false
	}
	back := h.columns[len(h.columns)-1].at.Sub(t)
	c := len(h.columns) - 1 - int(back/h.step)
	if back < 0 || back%h.step != 0 || c < 0 {
		return heldColumn{}, false
	}
	return h.columns[c], true
}

// roomAt is the pool's room in the window at t, of each of its accounts as
// then has it, by the readings given, summed: known where any's is.
func (h heldRoom) roomAt(m status.Moment, read status.Readings, then roomJudge, t time.Time) PoolRoomColumn {
	var col PoolRoomColumn
	for _, of := range h.accounts {
		a, _ := m.Account(of.id)
		if room, known := then(m, a, read, of.told, h.key, t); known {
			col.Room, col.Known = col.Room+room, true
		}
	}
	return col
}

// readBy knows the readings given, oldest first, read by t, as heldReadings
// does.
func readBy(read []readings.Reading, t time.Time) heldReadings {
	return heldReadingsOf(read[:sort.Search(len(read), func(i int) bool { return read[i].At.After(t) })])
}

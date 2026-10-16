package views

import (
	"slices"
	"sort"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// PoolRoom is the pool's room in a window over its length back from now, as
// GRAPHS charts it beside Σ pool by account: Of, how many accounts the pool
// has, none where there's no pool, as with one account; and each column's.
// Each look's is built once, by Look, from the last look's, which holds the
// columns it judged before now's until what they're judged from changes.
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
type roomJudge func(m status.Moment, a status.AccountNow, read []readings.Reading, told []status.Span, key string, t time.Time) (float64, bool)

// PoolRoomColumn is the pool's room at a column's moment: the room its
// accounts had left in the window then, summed in accounts' worth; and
// whether how any of them stood then is known, the column blank where
// none's is.
type PoolRoomColumn struct {
	Room  float64
	Known bool
}

// PoolRoomFrom is what the pool's room in a window is built from at a look:
// the moment; the readings history's readings, oldest first, as Runway's
// lanes take them, each window's run before them carried in, as
// readings.CarriedRuns has them; the window's key; and how many columns it's
// charted across.
type PoolRoomFrom struct {
	Moment   status.Moment
	Readings []readings.Reading
	Key      string
	Columns  int
}

// Look is the pool's room in the window at a look, as PoolRoom has it, over
// its length back from Now, a column every step, as poolStep has it: each
// column before now's at its moment, on a grid of those steps, the last at
// PoolRoomThrough's, so each stays at its moment as now moves on, as
// status.Moment.RoomThen judges each of the pool's accounts had room then,
// summed, held from p where what that's judged from hasn't changed; and the
// last, now's, the pool's room as the moment has it, Σ pool's. It's none
// where the pool has no such window, or the window's length isn't known.
func (p PoolRoom) Look(from PoolRoomFrom) PoolRoom {
	pool := from.Moment.Pool()
	i := slices.IndexFunc(pool.Windows, func(w status.PoolWindowNow) bool { return w.Key == from.Key })
	step, ok := poolStep(from.Key, from.Columns)
	if i < 0 || !ok {
		return PoolRoom{}
	}
	past := p.past.judged(from, step, p.judging())
	now := PoolRoomColumn{Room: pool.Windows[i].Room, Known: true}
	return PoolRoom{Of: len(pool.Accounts), Columns: append(slices.Clone(past.columns), now), then: p.then, past: past}
}

// PoolRoomThrough is the moment of the last column before now's of the
// pool's room in the window with the given key, charted across columns
// columns, at now, as Look lays them on the grid of its steps: the readings
// history's readings by then are all its past is judged by, so a read of
// them since gains it nothing till the grid moves a step on. It's zero where
// the window's length isn't known, or there are no columns.
func PoolRoomThrough(key string, columns int, now time.Time) time.Time {
	step, ok := poolStep(key, columns)
	if !ok {
		return time.Time{}
	}
	return now.Truncate(step)
}

// poolStep is how far apart the columns of the pool's room in the window with
// the given key are, charted across columns columns: its length over them,
// reporting false where the length isn't known, or there are no columns.
func poolStep(key string, columns int) (time.Duration, bool) {
	length, ok := quota.Length(key)
	if !ok || columns < 1 {
		return 0, false
	}
	return length / time.Duration(columns), true
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
// columns are apart; the moment of the last of them; the readings by then,
// as heldReadings knows them; the policy; each of the pool's accounts, as
// roomOf knows it, in the pool's order; and the room judged at each column.
type heldRoom struct {
	key      string
	step     time.Duration
	end      time.Time
	read     heldReadings
	policy   score.Policy
	accounts []roomOf
	columns  []PoolRoomColumn
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
// columns step apart, as heldRoom has it: each column held from h at the
// moment it was judged at, where what it was judged from by then hasn't
// changed, as the readings history only gains; the rest judged afresh, as
// roomAt has them by then.
func (h heldRoom) judged(from PoolRoomFrom, step time.Duration, then roomJudge) heldRoom {
	m := from.Moment
	past := heldRoom{key: from.Key, step: step, end: PoolRoomThrough(from.Key, from.Columns, m.Now), policy: m.Policy(), accounts: roomsOf(m)}
	past.read = readBy(from.Readings, past.end)
	past.columns = make([]PoolRoomColumn, from.Columns-1)
	holds := h.holds(past, from.Readings)
	var read map[string][]readings.Reading
	for c := range past.columns {
		at := past.at(c)
		if held, ok := h.columnAt(at); holds && ok {
			past.columns[c] = held
			continue
		}
		if read == nil {
			read = readingsByAccount(from.Readings)
		}
		past.columns[c] = past.roomAt(m, read, then, at)
	}
	return past
}

// at is the moment of column c of those the room holds, the last at its
// end.
func (h heldRoom) at(c int) time.Time {
	return h.end.Add(-time.Duration(len(h.columns)-1-c) * h.step)
}

// holds reports whether what h was judged from is what past is, but for its
// end, the readings by h's end the same, so each column h holds stands at
// its moment.
func (h heldRoom) holds(past heldRoom, read []readings.Reading) bool {
	return h.key == past.key && h.step == past.step && h.policy.Equal(past.policy) &&
		slices.EqualFunc(h.accounts, past.accounts, roomOf.is) && readBy(read, h.end) == h.read
}

// columnAt is the column h holds at the moment t, reporting false where it
// holds none then.
func (h heldRoom) columnAt(t time.Time) (PoolRoomColumn, bool) {
	if h.step <= 0 || t.After(h.end) {
		return PoolRoomColumn{}, false
	}
	back := h.end.Sub(t)
	c := len(h.columns) - 1 - int(back/h.step)
	if back%h.step != 0 || c < 0 {
		return PoolRoomColumn{}, false
	}
	return h.columns[c], true
}

// roomAt is the pool's room in the window at t, of each of its accounts as
// then has it, by its readings given, summed: known where any's is.
func (h heldRoom) roomAt(m status.Moment, read map[string][]readings.Reading, then roomJudge, t time.Time) PoolRoomColumn {
	var col PoolRoomColumn
	for _, of := range h.accounts {
		a, _ := m.Account(of.id)
		if room, known := then(m, a, read[of.id], of.told, h.key, t); known {
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

// readingsByAccount are the readings given, oldest first, by their
// accounts' ids.
func readingsByAccount(read []readings.Reading) map[string][]readings.Reading {
	by := make(map[string][]readings.Reading)
	for _, r := range read {
		by[r.Account] = append(by[r.Account], r)
	}
	return by
}

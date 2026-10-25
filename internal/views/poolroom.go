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
// of all of theirs that leaves used, as status.PoolUsed has it; and whether how any of them stood then is known,
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
	past := p.past.judged(pastFrom{
		moment: from.Moment, readings: from.Readings, through: from.ReadThrough, key: from.Key, step: step,
		at: poolMoments(from.Key, from.Columns, from.Moment.Now, step),
	}, p.then.orRoomThen())
	room := PoolRoom{Of: len(pool.Accounts), Columns: make([]PoolRoomColumn, 0, from.Columns), then: p.then, past: past}
	for _, c := range past.columns {
		col := poolRoomOf(c.rooms)
		if col.Known {
			col.Used = status.PoolUsed(col.Room, room.Of)
		}
		room.Columns = append(room.Columns, col)
	}
	now := pool.Windows[i].Room
	room.Columns = append(room.Columns, PoolRoomColumn{Room: now, Used: status.PoolUsed(now, room.Of), Known: true})
	return room
}

// poolMoments are the moments of the columns before now's of the pool's room
// in the window with the given key, charted across columns columns a step
// apart, at now, oldest first: the last at PoolRoomThrough's.
func poolMoments(key string, columns int, now time.Time, step time.Duration) []time.Time {
	end := PoolRoomThrough(key, columns, now)
	at := make([]time.Time, columns-1)
	for c := range at {
		at[c] = end.Add(-time.Duration(len(at)-1-c) * step)
	}
	return at
}

// poolRoomOf is the pool's room of its accounts' rooms given, summed: known
// where any's is.
func poolRoomOf(rooms []AccountRoom) PoolRoomColumn {
	var col PoolRoomColumn
	for _, r := range rooms {
		if r.Known {
			col.Room, col.Known = col.Room+r.Room, true
		}
	}
	return col
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

// orRoomThen is how a look judges the room an account had left in a window
// at a moment gone: by j, else by status.Moment.RoomThen.
func (j roomJudge) orRoomThen() roomJudge {
	if j == nil {
		return status.Moment.RoomThen
	}
	return j
}

// heldRoom is what was judged of the room the pool's accounts had in a
// window before now's column, and what that was judged from: the window's
// key; the step its columns are apart; the policy; each of the pool's
// accounts, as roomOf knows it, in the pool's order; and each column, as
// heldColumn has it, oldest first.
type heldRoom struct {
	key      string
	step     time.Duration
	policy   score.Policy
	accounts []roomOf
	columns  []heldColumn
}

// heldColumn is a column before now's: its moment; the readings by then, as
// heldReadings knows them; whether it was judged; and, judged, each of the
// pool's accounts' room then, in the pool's order.
type heldColumn struct {
	at     time.Time
	read   heldReadings
	judged bool
	rooms  []AccountRoom
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

// pastFrom is what the room in a window before now's column is judged from
// at a look: the moment; the readings history's readings, oldest first, and
// when they were read through, as PoolRoomFrom has them; the window's key;
// the step its columns are apart; and each column's moment, oldest first.
type pastFrom struct {
	moment   status.Moment
	readings []readings.Reading
	through  time.Time
	key      string
	step     time.Duration
	at       []time.Time
}

// judged is the room before now's column at the look from gives, as
// heldRoom has it: each column held from h where it was judged at its moment
// from what it's judged from now, the readings by then the same, as RoomThen
// reads none after; else judged afresh, as roomsAt has it, of the readings
// indexed once, where they were read through its moment; else left not
// judged.
func (h heldRoom) judged(from pastFrom, then roomJudge) heldRoom {
	m := from.moment
	past := heldRoom{key: from.key, step: from.step, policy: m.Policy(), accounts: roomsOf(m), columns: make([]heldColumn, len(from.at))}
	alike := h.alike(past)
	read := sync.OnceValue(func() status.Readings { return status.Index(from.readings) })
	for c, at := range from.at {
		col := heldColumn{at: at, read: readBy(from.readings, at)}
		switch held, ok := h.columnAt(at); {
		case alike && ok && held.judged && held.read == col.read:
			col = held
		case !at.After(from.through):
			col.rooms, col.judged = past.roomsAt(m, read(), then, at), true
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

// columnAt is the column h holds at the moment t, reporting false where it
// holds none then.
func (h heldRoom) columnAt(t time.Time) (heldColumn, bool) {
	c, ok := slices.BinarySearchFunc(h.columns, t, func(col heldColumn, t time.Time) int { return col.at.Compare(t) })
	if !ok {
		return heldColumn{}, false
	}
	return h.columns[c], true
}

// roomsAt is the room each of the pool's accounts had in the window at t, as
// then has it, by the readings given, in the pool's order.
func (h heldRoom) roomsAt(m status.Moment, read status.Readings, then roomJudge, t time.Time) []AccountRoom {
	rooms := make([]AccountRoom, len(h.accounts))
	for i, of := range h.accounts {
		a, _ := m.Account(of.id)
		rooms[i].Room, rooms[i].Known = then(m, a, read, of.told, h.key, t)
	}
	return rooms
}

// readBy knows the readings given, oldest first, read by t, as heldReadings
// does.
func readBy(read []readings.Reading, t time.Time) heldReadings {
	return heldReadingsOf(read[:sort.Search(len(read), func(i int) bool { return read[i].At.After(t) })])
}

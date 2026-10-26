package views

import (
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// roomJudge is the room an account had left in a window at a moment gone, as
// status.Moment.RoomThen has it.
type roomJudge func(m status.Moment, a status.AccountNow, read status.Readings, told []status.Span, key string, t time.Time) (float64, bool)

// PoolRoomColumn is the pool's room at a column's moment: the room its
// accounts had left in the window then, summed in accounts' worth; the share
// of all of theirs that leaves used, as status.PoolUsed has it; and whether
// how any of them stood then is known, the column blank where none's is, or
// it isn't judged yet.
type PoolRoomColumn struct {
	Room, Used float64
	Known      bool
}

// PoolRoomFrom is what the pool's room in a window is built from at a look,
// as PoolSpan's is: the moment; the readings history's readings, oldest
// first, as Runway's lanes take them, each window's run before them carried
// in, as readings.CarriedRuns has them; ReadThrough, when the read of the
// history that gave them was asked, so it gave every reading by then, zero
// where none has been read; the window's key; and how many columns it's
// charted across.
type PoolRoomFrom struct {
	Moment      status.Moment
	Readings    []readings.Reading
	ReadThrough time.Time
	Key         string
	Columns     int
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

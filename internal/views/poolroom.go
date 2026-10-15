package views

import (
	"slices"
	"sort"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// PoolRoom is the pool's room in a window over its length back from now, as
// GRAPHS charts it beside Σ pool: Of, how many accounts the pool has, none
// where there's no pool, as with one account; and each column's.
type PoolRoom struct {
	Of      int
	Columns []PoolRoomColumn
}

// PoolRoomColumn is the pool's room at a column's moment: the room its
// accounts had left in the window then, summed in accounts' worth, as the
// pool counts it; and whether any of theirs is known then, the column blank
// where none's is.
type PoolRoomColumn struct {
	Room  float64
	Known bool
}

// PoolRoomFrom is what the pool's room in a window is worked out from: the
// moment; the window's key; and each account's readings of it over its
// length back from the moment's Now, by the account's id, in the order read,
// each its use from when it was read till the next.
type PoolRoomFrom struct {
	Moment status.Moment
	Key    string
	Read   map[string][]WindowReading
}

// Room is the pool's room in the window over its length back from Now,
// columns columns, each at its end's moment, the last at Now, as PoolRoom
// has it: each of the pool's accounts' room to where it runs out, its Floor,
// less its use then, as usedAt has it, none less, summed, an account whose
// use isn't known then counting none, as the pool counts one never read; and
// at Now, the pool's room as the moment has it. It's none where the pool has
// no such window, or the window's length isn't known.
func (from PoolRoomFrom) Room(columns int) PoolRoom {
	pool := from.Moment.Pool()
	i := slices.IndexFunc(pool.Windows, func(w status.PoolWindowNow) bool { return w.Key == from.Key })
	length, ok := quota.Length(from.Key)
	if i < 0 || !ok || columns < 1 {
		return PoolRoom{}
	}
	members := make([]poolMember, 0, len(pool.Accounts))
	for _, id := range pool.Accounts {
		if a, ok := from.Moment.Account(id); ok {
			members = append(members, from.member(a))
		}
	}
	room := PoolRoom{Of: len(pool.Accounts), Columns: make([]PoolRoomColumn, columns)}
	for c := range columns - 1 {
		at := from.Moment.Now.Add(-time.Duration(float64(length) * float64(columns-1-c) / float64(columns)))
		for _, m := range members {
			if used, ok := m.usedAt(at); ok {
				room.Columns[c].Room += max(m.floor-used, 0)
				room.Columns[c].Known = true
			}
		}
	}
	room.Columns[columns-1] = PoolRoomColumn{Room: pool.Windows[i].Room, Known: true}
	return room
}

// poolMember is an account of the pool, as its room in a window is worked
// out: the window's key; where it runs out, its Floor; and its readings of
// the window, in the order read.
type poolMember struct {
	key   string
	floor float64
	read  []WindowReading
}

// member is account a as the pool's room in from's window is worked out:
// its readings, and a reading of none at each of the window's resets the
// moment knows of, where it started, as it stands at Now, and where it was
// to reset as last read, where that has passed, each before any reading taken
// then, so a window's use runs from none from its reset till it's read
// again.
func (from PoolRoomFrom) member(a status.AccountNow) poolMember {
	w, _ := a.Window(from.Key)
	var reset []WindowReading
	for _, at := range []time.Time{w.Start, w.Read.ResetsAt} {
		if !at.IsZero() && !at.After(from.Moment.Now) {
			reset = append(reset, WindowReading{At: at})
		}
	}
	read := slices.Concat(reset, from.Read[a.ID])
	slices.SortStableFunc(read, func(a, b WindowReading) int { return a.At.Compare(b.At) })
	return poolMember{key: from.Key, floor: a.Floor, read: read}
}

// usedAt is how much of the window the account had used at t: as the last
// of its readings at or before t read it, none once its reset had passed, as
// score.Current has it; reporting false where none was read by then.
func (m poolMember) usedAt(t time.Time) (float64, bool) {
	i := sort.Search(len(m.read), func(i int) bool { return m.read[i].At.After(t) })
	if i == 0 {
		return 0, false
	}
	r := m.read[i-1]
	return score.Current(quota.Window{Key: m.key, Utilization: r.Utilization, ResetsAt: r.ResetsAt}, t).Utilization, true
}

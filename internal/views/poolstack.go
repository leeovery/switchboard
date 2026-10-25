package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// PoolStack is the pool's room over Runway's week view, as Graphs' POOL
// stacks it: Timeline, the columns it's charted across, as Runway's week's
// lanes have them live at the width given; Key, the key of the window the
// week view is named for; and each of the pool's accounts' room in each
// column, in the pool's order, the config's, as the pool counts its accounts.
// It has no accounts where there's no pool, as with one account, or the pool
// has no such window, or no columns. Each look's is built once, by Look, from
// the last look's, which holds each column it judged before now's while what
// that's judged from stands.
type PoolStack struct {
	Timeline Timeline
	Key      string
	Accounts []PoolStackAccount

	// then is the room an account had left in a window at a moment gone, as
	// status.Moment.RoomThen has it where it's nil; and past is what was
	// judged before now's column, held.
	then roomJudge
	past heldRoom
}

// PoolStackAccount is one of the pool's accounts in its stack: its id and
// label, as the config gives them, and its room in each of the timeline's
// columns. Its room is the room left to use, in accounts' worth, before its
// use reaches where it runs out, its Floor, as the pool counts it: its full
// room is its Floor, its cap where its reserve holds. Before now's column,
// it's the room it had then, as status.Moment.RoomThen judges it, not known
// where nothing was read of it by then, or the column isn't judged yet; in
// now's, its room now, as the moment has it; after now's, its room at the
// column's moment by its projection, as AccountNow.RoomAhead has it, falling
// to where it runs out, then its floor to its reset, then all of it; or,
// where RoomAhead says nothing of it, as of a window not started, stale, of
// an account without a usable token or nothing read, its room now, held.
type PoolStackAccount struct {
	ID, Label string
	Rooms     []AccountRoom
}

// AccountRoom is the room an account has in a window at a moment, in
// accounts' worth, as the pool counts it, and whether how it stood then is
// known.
type AccountRoom struct {
	Room  float64
	Known bool
}

// PoolStackFrom is what the pool's stack is built from at a look: the
// moment; the readings history's readings, oldest first, and when they were
// read through, as PoolRoomFrom has them; and how many columns it's charted
// across.
type PoolStackFrom struct {
	Moment      status.Moment
	Readings    []readings.Reading
	ReadThrough time.Time
	Columns     int
}

// Look is the pool's stack at a look, as PoolStack has it: each column before
// now's judged at its moment, as Timeline.Moment has it, held from p where
// what that's judged from by then hasn't changed, and not judged, so not
// known, where it's after the readings were read through; where the pool has
// no such window, it holds what p judged, for a look once it has again.
func (p PoolStack) Look(from PoolStackFrom) PoolStack {
	m := from.Moment
	tl, key := LiveTimeline(RunwayWeek, m.Now, from.Columns), viewKeyOf(m.Policy(), RunwayWeek)
	pool := m.Pool()
	if from.Columns < 1 || !slices.ContainsFunc(pool.Windows, func(w status.PoolWindowNow) bool { return w.Key == key }) {
		return PoolStack{then: p.then, past: p.past}
	}
	at := make([]time.Time, max(tl.NowColumn(), 0))
	for c := range at {
		at[c] = tl.Moment(c)
	}
	past := p.past.judged(pastFrom{moment: m, readings: from.Readings, through: from.ReadThrough, key: key, step: tl.Step(), at: at}, p.then.orRoomThen())
	stack := PoolStack{Timeline: tl, Key: key, Accounts: make([]PoolStackAccount, len(past.accounts)), then: p.then, past: past}
	for i, of := range past.accounts {
		a, _ := m.Account(of.id)
		stack.Accounts[i] = PoolStackAccount{ID: a.ID, Label: a.Label, Rooms: stackedRooms(a, key, tl, past.columns, i)}
	}
	return stack
}

// stackedRooms are the room of account a, the ith of the pool's, in the
// window with the given key in each of the timeline's columns, as
// PoolStackAccount has them, those before now's as past judged them.
func stackedRooms(a status.AccountNow, key string, tl Timeline, past []heldColumn, i int) []AccountRoom {
	w, _ := a.Window(key)
	now := tl.NowColumn()
	rooms := make([]AccountRoom, tl.Columns)
	for c := range rooms {
		switch {
		case c < now:
			if past[c].judged {
				rooms[c] = past[c].rooms[i]
			}
		case c == now:
			rooms[c] = AccountRoom{Room: w.Room, Known: true}
		default:
			rooms[c] = AccountRoom{Room: roomAhead(a, key, tl.Moment(c), w.Room), Known: true}
		}
	}
	return rooms
}

// roomAhead is the room account a has to use in the window with the given
// key at t, ahead of now, by its projection: RoomAhead's room, counted from
// none used, less what the account's floor keeps back, so it's counted from
// its floor, as RoomThen and the room now count it, and the two meet at now;
// or now's, where RoomAhead says nothing of it.
func roomAhead(a status.AccountNow, key string, t time.Time, now float64) float64 {
	room, ok := a.RoomAhead(key, t)
	if !ok {
		return now
	}
	return max(room-(1-a.Floor), 0)
}

package views_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// poolAt is when the pool's room is worked out in its tests: 14:20, its
// 5-hour charted in five columns, an hour apart, at 11:00, 12:00, 13:00,
// 14:00 and now.
var poolAt = poolClock(14, 20)

// poolClock is the time on poolAt's day, Wednesday 14 October 2026, at the
// hour and minute given.
func poolClock(hour, minute int) time.Time {
	return time.Date(2026, 10, 14, hour, minute, 0, 0, time.UTC)
}

// sessionRead is a reading of the session of the account with the given id,
// at the use given, resetting at resets, read at at.
func sessionRead(id string, at time.Time, used float64, resets time.Time) readings.Reading {
	return readings.Reading{At: at, Account: id, Key: claude.FiveHourWindow, Utilization: used, ResetsAt: resets, Source: readings.FromAnswer}
}

// pooledAccount is the account with the given id keeping the reserve given,
// its session as its last reading of those given has it.
func pooledAccount(id string, reserve float64, read []readings.Reading) status.Account {
	var last readings.Reading
	for _, r := range read {
		if r.Account == id {
			last = r
		}
	}
	return status.Account{
		ID: id, Label: id, TokenSet: true, FetchedAt: last.At, Reserve: reserve,
		Windows: []quota.Window{{Key: last.Key, Label: "Session", Utilization: last.Utilization, ResetsAt: last.ResetsAt}},
	}
}

// poolReadings are the readings history's readings of the pool's sessions,
// oldest first:
//   - work, keeping a reserve of a fifth, so running out at 0.8: its session
//     before reset at 10:30, and the one now started at 11:00;
//   - personal, keeping none: idle since Monday, its session then long reset,
//     and the one now started at 13:00;
//   - side, keeping a tenth: at its cap from 12:30.
var poolReadings = []readings.Reading{
	sessionRead("personal", poolClock(13, 0).AddDate(0, 0, -2), 0.6, poolClock(16, 0).AddDate(0, 0, -2)),
	sessionRead("work", poolClock(9, 30), 0.5, poolClock(10, 30)),
	sessionRead("side", poolClock(10, 0), 0.5, poolClock(15, 0)),
	sessionRead("work", poolClock(11, 30), 0.1, poolClock(16, 0)),
	sessionRead("side", poolClock(12, 30), 0.92, poolClock(15, 0)),
	sessionRead("work", poolClock(13, 30), 0.4, poolClock(16, 0)),
	sessionRead("personal", poolClock(13, 30), 0.2, poolClock(18, 0)),
}

// pooledDoc is the router's document at poolAt of the pool's accounts, each
// as its last reading of poolReadings has it.
func pooledDoc() status.Document {
	return status.Document{
		GeneratedAt: poolAt, Source: status.SourceRouter, Router: status.Health{Healthy: true},
		Accounts: []status.Account{
			pooledAccount("work", 0.2, poolReadings), pooledAccount("personal", 0, poolReadings), pooledAccount("side", 0.1, poolReadings),
		},
	}
}

// poolFrom is what the pool's 5-hour room is built from at poolAt, of doc
// and the readings given, across the columns given.
func poolFrom(doc status.Document, read []readings.Reading, columns int) views.PoolRoomFrom {
	return views.PoolRoomFrom{Moment: doc.At(poolAt, claude.Policy()), Readings: read, Key: claude.FiveHourWindow, Columns: columns}
}

// The pool's room in a window at each column's moment is each account's room
// as it stood then, as RoomThen judges it, summed: work's whole room once
// its last session reset, 0.8, and 0.7 and 0.4 as read; personal's whole
// room till its session's first reading, idle as it was, then 0.8; side's
// 0.4 till its cap, then none, as at now, so no cliff there. Now's is the
// pool's as the moment has it, Σ pool's.
func TestThePoolsRoomIsEachAccountsRoomAsItStoodThenSummed(t *testing.T) {
	from := poolFrom(pooledDoc(), poolReadings, 5)
	got := views.PoolRoom{}.Look(from)
	want := []float64{2.2, 2.1, 1.7, 1.2, poolRoomNow(from.Moment, claude.FiveHourWindow)}
	if got.Of != 3 || len(got.Columns) != len(want) {
		t.Fatalf("the pool's room is of %d accounts, %d columns, want 3 and %d", got.Of, len(got.Columns), len(want))
	}
	for c, col := range got.Columns {
		if math.Abs(col.Room-want[c]) > 1e-9 || !col.Known {
			t.Errorf("column %d holds %v, known %v, want %v, known", c, col.Room, col.Known, want[c])
		}
	}
	if now, before := got.Columns[4].Room, got.Columns[3].Room; math.Abs(now-before) > 1e-9 {
		t.Errorf("the pool's room steps from %v to %v at now, with nothing read between", before, now)
	}
}

// poolRoomNow is the pool's room in the window with the given key, as the
// moment m has it.
func poolRoomNow(m status.Moment, key string) float64 {
	for _, w := range m.Pool().Windows {
		if w.Key == key {
			return w.Room
		}
	}
	return math.NaN()
}

// Where nothing was read of any account by a column's moment, the column
// isn't known; where nothing was read of one, it counts none.
func TestAColumnNothingWasReadByIsNotKnown(t *testing.T) {
	read := poolReadings[1:]
	got := views.PoolRoom{}.Look(poolFrom(pooledDoc(), read[2:], 5))
	if got.Columns[0].Known {
		t.Errorf("at 11:00, nothing yet read of work or side the history holds, the column is known, holding %v", got.Columns[0].Room)
	}
	if col := got.Columns[2]; !col.Known || math.Abs(col.Room-0.7) > 1e-9 {
		t.Errorf("at 13:00, work's alone read, the column holds %v, known %v, want 0.7, known", col.Room, col.Known)
	}
}

// The pool has no room charted where it has no such window, or one account,
// or no columns to chart it in.
func TestThePoolHasNoRoomChartedWithoutAPoolOrItsWindow(t *testing.T) {
	alone := pooledDoc()
	alone.Accounts = alone.Accounts[:1]
	tests := []struct {
		name string
		from views.PoolRoomFrom
	}{
		{name: "a window the pool hasn't", from: views.PoolRoomFrom{Moment: pooledDoc().At(poolAt, claude.Policy()), Readings: poolReadings, Key: claude.WeekWindow, Columns: 5}},
		{name: "one account", from: poolFrom(alone, poolReadings, 5)},
		{name: "no columns", from: poolFrom(pooledDoc(), poolReadings, 0)},
	}
	for _, tt := range tests {
		if got := (views.PoolRoom{}).Look(tt.from); got.Of != 0 || len(got.Columns) != 0 {
			t.Errorf("%s: the pool's room is of %d accounts across %d columns, want none", tt.name, got.Of, len(got.Columns))
		}
	}
}

// At now, the pool's room is Σ pool's, as the moment has it, at every hard
// moment, in each of the pool's windows.
func TestThePoolsRoomAtNowIsThePoolsAsTheMomentHasIt(t *testing.T) {
	for _, hm := range statustest.HardMoments(claude.Policy()) {
		m := hm.Doc.At(hm.Now, claude.Policy())
		for _, w := range m.Pool().Windows {
			got := views.PoolRoom{}.Look(views.PoolRoomFrom{Moment: m, Readings: docReadings(hm.Doc), Key: w.Key, Columns: 24})
			if last := got.Columns[len(got.Columns)-1]; last.Room != w.Room || !last.Known {
				t.Errorf("%s: the pool's %s room at now is %v, known %v, want %v, as the moment has it", hm.Name, w.Key, last.Room, last.Known, w.Room)
			}
		}
	}
}

// The pool's room in the window a request starts, the session, agrees with
// Runway's pool lane at every moment they share before now, at every moment
// Runway has to get right that holds now: it has room where the lane counts
// any of the pool's accounts with room, none where it counts none, never
// more than an account's worth for each it counts, and is known where the
// lane is. An account that reads no session at all has no room in one, as
// Σ pool counts it, though the lane counts it with room by its week, so a
// moment with one is left out.
func TestThePoolsRoomAgreesWithRunwaysPoolLane(t *testing.T) {
	const columns = 60
	step := 5 * time.Hour / columns
	held, with := 0, 0
	for _, rm := range viewstest.RunwayMoments() {
		from := lanesFrom(rm)
		tl, now := from.Timeline, from.Timeline.NowColumn()
		lanes := views.Lanes{}.Look(from)
		if now < 0 || lanes.Pool.Of == 0 || !eachReads(from.Moment, claude.FiveHourWindow) {
			continue
		}
		pool := views.PoolRoom{}.Look(views.PoolRoomFrom{Moment: from.Moment, Readings: rm.Readings, Key: claude.FiveHourWindow, Columns: columns})
		end := rm.Now.Truncate(step)
		for c := range now {
			at := tl.Moment(c)
			back := end.Sub(at)
			k := columns - 2 - int(back/step)
			if back%step != 0 || k < 0 || k > columns-2 {
				continue
			}
			lane, col := lanes.Pool.Columns[c], pool.Columns[k]
			if col.Known != lane.Known || (col.Room > 0) != (lane.With > 0) || col.Room > float64(lane.With)+1e-9 {
				t.Errorf("%s, at %s: the pool's room is %v, known %v; its lane counts %d with room, known %v",
					rm.Name, at.Format("Mon 15:04"), col.Room, col.Known, lane.With, lane.Known)
			}
			if lane.With < lanes.Pool.Of {
				held++
			} else {
				with++
			}
		}
	}
	if held == 0 || with == 0 {
		t.Errorf("the pool's room is held to its lane at %d moments some account had no room, and %d each had, want some of each", held, with)
	}
}

// eachReads reports whether each of the moment's pool's accounts reads the
// window with the given key.
func eachReads(m status.Moment, key string) bool {
	for _, id := range m.Pool().Accounts {
		a, _ := m.Account(id)
		if _, ok := a.Window(key); !ok {
			return false
		}
	}
	return true
}

// A look judges the pool's room before now's once, an account a column, and
// holds it while what it's judged from stands: the next look a second on, or
// with a reading gained at now, judges none; a look a step on judges the
// column the step brings alone; one with a reading gained before the last
// column's moment, an account's reserve changed, or across other columns,
// judges it all again. Each builds the room a first look would.
func TestALookJudgesThePoolsPastOnceWhileWhatItsJudgedFromStands(t *testing.T) {
	doc := pooledDoc()
	at := func(d time.Duration, read []readings.Reading, columns int) views.PoolRoomFrom {
		from := poolFrom(doc, read, columns)
		from.Moment = doc.At(poolAt.Add(d), claude.Policy())
		return from
	}
	gainedNow := append(slices.Clip(poolReadings), sessionRead("work", poolAt, 0.45, poolClock(16, 0)))
	gainedBefore := append(slices.Clip(poolReadings), sessionRead("side", poolClock(13, 45), 0.93, poolClock(15, 0)))
	reserved := reserveOf(doc, "personal", 0.1)
	tests := []struct {
		name string
		then views.PoolRoomFrom
		more int
	}{
		{name: "a second on", then: at(time.Second, poolReadings, 5), more: 0},
		{name: "a reading gained at now", then: at(time.Second, gainedNow, 5), more: 0},
		{name: "a step on", then: at(time.Hour, poolReadings, 5), more: 3},
		{name: "a reading gained before the last column's moment", then: at(time.Second, gainedBefore, 5), more: 3 * 4},
		{name: "personal's reserve changed", then: views.PoolRoomFrom{Moment: reserved.At(poolAt, claude.Policy()), Readings: poolReadings, Key: claude.FiveHourWindow, Columns: 5}, more: 3 * 4},
		{name: "across other columns", then: at(time.Second, poolReadings, 10), more: 3 * 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var judged int
			first := views.CountingPoolRoom(&judged).Look(poolFrom(doc, poolReadings, 5))
			if judged != 3*4 {
				t.Errorf("the first look judged %d times, want %d", judged, 3*4)
			}
			judged = 0
			then := first.Look(tt.then)
			if judged != tt.more {
				t.Errorf("the next look judged %d times, want %d", judged, tt.more)
			}
			if fresh := (views.PoolRoom{}).Look(tt.then); fresh.Of != then.Of || !slices.Equal(fresh.Columns, then.Columns) {
				t.Errorf("the next look's room is %+v, want %+v, as a first look builds it", then.Columns, fresh.Columns)
			}
		})
	}
}

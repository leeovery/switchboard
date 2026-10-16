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
// 5-hour charted in six columns, an hour apart, at 10:00, 11:00, 12:00,
// 13:00, 14:00 and now.
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
// and the readings given, read through then, across the columns given.
func poolFrom(doc status.Document, read []readings.Reading, columns int) views.PoolRoomFrom {
	return views.PoolRoomFrom{Moment: doc.At(poolAt, claude.Policy()), Readings: read, ReadThrough: poolAt, Key: claude.FiveHourWindow, Columns: columns}
}

// The pool's room in a window at each column's moment is each account's room
// as it stood then, as RoomThen judges it, summed: work's 0.3 left before its
// last session reset, its whole room once it had, 0.8, and 0.7 and 0.4 as
// read; personal's whole room till its session's first reading, idle as it
// was, then 0.8; side's 0.4 till its cap, then none, as at now, so no cliff
// there. Now's is the pool's as the moment has it, Σ pool's. Each column's
// used share is what Σ pool's bar fills to at that room, 1 − room ÷ 3, now's
// the bar's own.
func TestThePoolsRoomIsEachAccountsRoomAsItStoodThenSummed(t *testing.T) {
	from := poolFrom(pooledDoc(), poolReadings, 6)
	got := views.PoolRoom{}.Look(from)
	now := poolWindowNow(from.Moment, claude.FiveHourWindow)
	want := []float64{1.7, 2.2, 2.1, 1.7, 1.2, now.Room}
	if got.Of != 3 || len(got.Columns) != len(want) {
		t.Fatalf("the pool's room is of %d accounts, %d columns, want 3 and %d", got.Of, len(got.Columns), len(want))
	}
	for c, col := range got.Columns {
		if math.Abs(col.Room-want[c]) > 1e-9 || math.Abs(col.Used-(1-want[c]/3)) > 1e-9 || !col.Known {
			t.Errorf("column %d holds %v, %v used, known %v, want %v, %v used, known", c, col.Room, col.Used, col.Known, want[c], 1-want[c]/3)
		}
	}
	if last := got.Columns[5]; last.Used != now.Used {
		t.Errorf("now's column is %v used, want %v, as Σ pool's bar fills to", last.Used, now.Used)
	}
	if now, before := got.Columns[5].Room, got.Columns[4].Room; math.Abs(now-before) > 1e-9 {
		t.Errorf("the pool's room steps from %v to %v at now, with nothing read between", before, now)
	}
}

// poolWindowNow is the pool's window with the given key, as the moment m
// has it.
func poolWindowNow(m status.Moment, key string) status.PoolWindowNow {
	for _, w := range m.Pool().Windows {
		if w.Key == key {
			return w
		}
	}
	return status.PoolWindowNow{Room: math.NaN(), Used: math.NaN()}
}

// Where nothing was read of any account by a column's moment, the column
// isn't known; where nothing was read of one, it counts none.
func TestAColumnNothingWasReadByIsNotKnown(t *testing.T) {
	read := poolReadings[1:]
	got := views.PoolRoom{}.Look(poolFrom(pooledDoc(), read[2:], 6))
	if got.Columns[1].Known {
		t.Errorf("at 11:00, nothing yet read of work or side the history holds, the column is known, holding %v", got.Columns[1].Room)
	}
	if col := got.Columns[3]; !col.Known || math.Abs(col.Room-0.7) > 1e-9 {
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
		{name: "one account", from: poolFrom(alone, poolReadings, 6)},
		{name: "no columns", from: poolFrom(pooledDoc(), poolReadings, 0)},
	}
	for _, tt := range tests {
		if got := (views.PoolRoom{}).Look(tt.from); got.Of != 0 || len(got.Columns) != 0 {
			t.Errorf("%s: the pool's room is of %d accounts across %d columns, want none", tt.name, got.Of, len(got.Columns))
		}
	}
}

// The pool's chart reaches a length back from now, where its axis says it
// starts, across any count of columns, wherever now falls in a step: its
// columns before now's a step apart, the last at or a step short of now, and
// the first, the room at its end's moment, starting a step before it, so a
// length back from now falls within it.
func TestThePoolsChartReachesALengthBackFromNow(t *testing.T) {
	length, _ := quota.Length(claude.FiveHourWindow)
	for _, columns := range []int{3, 6, 25, 40} {
		for _, into := range []time.Duration{0, time.Second, 7 * time.Minute, 59 * time.Minute} {
			now := poolAt.Add(into)
			from := poolFrom(pooledDoc(), poolReadings, columns)
			from.Moment, from.ReadThrough = pooledDoc().At(now, claude.Policy()), now
			var judged []time.Time
			views.NotingPoolRoom(&judged).Look(from)
			at := slices.Compact(slices.SortedFunc(slices.Values(judged), time.Time.Compare))
			if len(at) != columns-1 {
				t.Fatalf("%d columns at %s: the columns before now's are at %v, want %d", columns, now.Format("15:04:05"), at, columns-1)
			}
			step, first, last := at[1].Sub(at[0]), at[0], at[len(at)-1]
			for c := range at[1:] {
				if at[c+1].Sub(at[c]) != step {
					t.Errorf("%d columns at %s: the columns before now's are at %v, want them a step apart", columns, now.Format("15:04:05"), at)
					break
				}
			}
			if back := now.Add(-length); first.Add(-step).After(back) || !first.After(back) || last.After(now) || !last.After(now.Add(-step)) {
				t.Errorf("%d columns at %s: the first column ends at %s, the last at %s, a step %s apart; want a length back, %s, within the first, and the last within a step of now",
					columns, now.Format("15:04:05"), first.Format("15:04:05"), last.Format("15:04:05"), step, back.Format("15:04:05"))
			}
		}
	}
}

// A look whose moment has no such window in the pool, as a read can leave it
// across a plan re-read or a reconnect, charts no room, but holds what was
// judged before it, so the look once the window's back judges nothing again
// and builds the room a first look would.
func TestALookWithoutItsWindowHoldsWhatWasJudged(t *testing.T) {
	var judged int
	first := views.CountingPoolRoom(&judged).Look(poolFrom(pooledDoc(), poolReadings, 6))
	unread := pooledDoc()
	for i := range unread.Accounts {
		unread.Accounts[i].Windows = nil
	}
	without := first.Look(poolFrom(unread, poolReadings, 6))
	if without.Of != 0 || len(without.Columns) != 0 {
		t.Fatalf("without its window, the pool's room is of %d accounts across %d columns, want none", without.Of, len(without.Columns))
	}
	judged = 0
	back := without.Look(poolFrom(pooledDoc(), poolReadings, 6))
	if judged != 0 {
		t.Errorf("the look with the window back judged %d times, want none", judged)
	}
	if fresh := (views.PoolRoom{}).Look(poolFrom(pooledDoc(), poolReadings, 6)); fresh.Of != back.Of || !slices.Equal(fresh.Columns, back.Columns) {
		t.Errorf("with the window back, the pool's room is %+v, want %+v, as a first look builds it", back.Columns, fresh.Columns)
	}
}

// At now, the pool's room, and the share of it used, is Σ pool's, as the
// moment has it, at every hard moment, in each of the pool's windows.
func TestThePoolsRoomAtNowIsThePoolsAsTheMomentHasIt(t *testing.T) {
	for _, hm := range statustest.HardMoments(claude.Policy()) {
		m := hm.Doc.At(hm.Now, claude.Policy())
		for _, w := range m.Pool().Windows {
			got := views.PoolRoom{}.Look(views.PoolRoomFrom{Moment: m, Readings: docReadings(hm.Doc), ReadThrough: hm.Now, Key: w.Key, Columns: 24})
			if last := got.Columns[len(got.Columns)-1]; last.Room != w.Room || last.Used != w.Used || !last.Known {
				t.Errorf("%s: the pool's %s room at now is %v, %v used, known %v, want %v, %v used, as the moment has it", hm.Name, w.Key, last.Room, last.Used, last.Known, w.Room, w.Used)
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
	const columns = 61
	step := 5 * time.Hour / (columns - 1)
	held, with := 0, 0
	for _, rm := range viewstest.RunwayMoments() {
		from := lanesFrom(rm)
		tl, now := from.Timeline, from.Timeline.NowColumn()
		lanes := views.Lanes{}.Look(from)
		if now < 0 || lanes.Pool.Of == 0 || !eachReads(from.Moment, claude.FiveHourWindow) {
			continue
		}
		pool := views.PoolRoom{}.Look(views.PoolRoomFrom{Moment: from.Moment, Readings: rm.Readings, ReadThrough: rm.Now, Key: claude.FiveHourWindow, Columns: columns})
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

// A column after the moment the readings were read through isn't judged, so
// isn't known, nor held: once a read since gives what they gained by then,
// the next look judges it alone, as a first look would.
func TestAColumnAfterTheReadingsWereReadThroughWaitsForTheirRead(t *testing.T) {
	var judged int
	early := poolFrom(pooledDoc(), poolReadings, 6)
	early.ReadThrough = poolClock(13, 50)
	first := views.CountingPoolRoom(&judged).Look(early)
	if col := first.Columns[4]; col.Known || judged != 3*4 {
		t.Errorf("read through 13:50, the 14:00 column is %v, known %v, of %d judged; want it not known, of %d", col.Room, col.Known, judged, 3*4)
	}
	judged = 0
	read := poolFrom(pooledDoc(), poolReadings, 6)
	then := first.Look(read)
	if judged != 3 {
		t.Errorf("once read through 14:20, the next look judged %d times, want %d, the 14:00 column alone", judged, 3)
	}
	if fresh := (views.PoolRoom{}).Look(read); !slices.Equal(fresh.Columns, then.Columns) {
		t.Errorf("the next look's room is %+v, want %+v, as a first look builds it", then.Columns, fresh.Columns)
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
// holds each column while what it's judged from by its moment stands: the
// next look a second on, or with a reading gained at now, judges none; a
// look a step on judges the column the step brings alone; one with a
// reading gained before the last column's moment judges that column alone,
// and one gained before the last two's, those two; one with an account's
// reserve changed, or across other columns, as a resize lays them, judges
// them all again. Each builds the room a first look would.
func TestALookJudgesThePoolsPastOnceWhileWhatItsJudgedFromStands(t *testing.T) {
	doc := pooledDoc()
	at := func(d time.Duration, read []readings.Reading, columns int) views.PoolRoomFrom {
		from := poolFrom(doc, read, columns)
		from.Moment, from.ReadThrough = doc.At(poolAt.Add(d), claude.Policy()), poolAt.Add(d)
		return from
	}
	gainedNow := append(slices.Clip(poolReadings), sessionRead("work", poolAt, 0.45, poolClock(16, 0)))
	gainedBefore := append(slices.Clip(poolReadings), sessionRead("side", poolClock(13, 45), 0.93, poolClock(15, 0)))
	gainedEarlier := append(slices.Clip(poolReadings[:6]), sessionRead("work", poolClock(12, 45), 0.3, poolClock(16, 0)), poolReadings[6])
	slices.SortStableFunc(gainedEarlier, func(a, b readings.Reading) int { return a.At.Compare(b.At) })
	reserved := poolFrom(reserveOf(doc, "personal", 0.1), poolReadings, 6)
	tests := []struct {
		name string
		then views.PoolRoomFrom
		more int
	}{
		{name: "a second on", then: at(time.Second, poolReadings, 6), more: 0},
		{name: "a reading gained at now", then: at(time.Second, gainedNow, 6), more: 0},
		{name: "a step on", then: at(time.Hour, poolReadings, 6), more: 3},
		{name: "a reading gained before the last column's moment", then: at(time.Second, gainedBefore, 6), more: 3},
		{name: "a reading gained before the last two columns' moments", then: at(time.Second, gainedEarlier, 6), more: 3 * 2},
		{name: "personal's reserve changed", then: reserved, more: 3 * 5},
		{name: "across other columns", then: at(time.Second, poolReadings, 10), more: 3 * 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var judged int
			first := views.CountingPoolRoom(&judged).Look(poolFrom(doc, poolReadings, 6))
			if judged != 3*5 {
				t.Errorf("the first look judged %d times, want %d", judged, 3*5)
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

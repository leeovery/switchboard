package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The pool's span in its tests, at stackAt: work's week resets four days on,
// so started three days back; personal's five days on, so started two days
// back. Its span is theirs together, eight days, across 16 columns of 12
// hours, now on the edge of the seventh, column 6.
var (
	spanWorkReset     = stackAt.Add(4 * quota.Day)
	spanPersonalReset = stackAt.Add(5 * quota.Day)
	spanStart         = spanWorkReset.Add(-7 * quota.Day)
)

const (
	spanColumns = 16
	spanNow     = 6
)

// spanRead is a reading of the week of the account with the given id, at the
// use given, resetting at reset, read at at.
func spanRead(id string, at time.Time, used float64, reset time.Time) readings.Reading {
	return readings.Reading{At: at, Account: id, Key: claude.WeekWindow, Utilization: used, ResetsAt: reset, Source: readings.FromAnswer}
}

// spanReadings are the readings history's of the pool's weeks: work read at
// 20% an hour into its week, personal at 10% an hour into its; and each at
// now as it stands, work at 60%, personal at 30%.
var spanReadings = []readings.Reading{
	spanRead("work", spanStart.Add(time.Hour), 0.2, spanWorkReset),
	spanRead("personal", spanPersonalReset.Add(-7*quota.Day+time.Hour), 0.1, spanPersonalReset),
	spanRead("work", stackAt, 0.6, spanWorkReset),
	spanRead("personal", stackAt, 0.3, spanPersonalReset),
}

// spanFrom is what the pool's room over its week's span is built from at now,
// of the router's document of the accounts given then and the readings
// given, read through now, across spanColumns.
func spanFrom(now time.Time, read []readings.Reading, accounts ...status.Account) views.PoolRoomFrom {
	doc := status.Document{GeneratedAt: now, Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: accounts}
	return views.PoolRoomFrom{Moment: doc.At(now, claude.Policy()), Readings: read, ReadThrough: now, Key: claude.WeekWindow, Columns: spanColumns}
}

// spanPool is the pool of the span's tests, work and personal, as they stand
// at stackAt.
func spanPool() []status.Account {
	return []status.Account{
		weekAccount("work", stackAt, 0.6, spanWorkReset), weekAccount("personal", stackAt, 0.3, spanPersonalReset),
	}
}

// The pool's room over a window is charted from its accounts' earliest
// start in it to their latest reset, across the columns given, now where it
// falls among them, keyed by the window, of as many accounts as the pool.
func TestThePoolsSpanRunsFromItsAccountsEarliestStartToTheirLatestReset(t *testing.T) {
	got := views.PoolSpan{}.Look(spanFrom(stackAt, spanReadings, spanPool()...))
	want := views.Timeline{Start: spanStart, End: spanPersonalReset, Columns: spanColumns, Now: stackAt}
	if !got.Timeline.Start.Equal(want.Start) || !got.Timeline.End.Equal(want.End) || got.Timeline.Columns != want.Columns || !got.Timeline.Now.Equal(want.Now) {
		t.Errorf("the span's timeline is %+v, want %+v", got.Timeline, want)
	}
	if got.Key != claude.WeekWindow || got.Of != 2 || len(got.Columns) != spanColumns || got.Timeline.NowColumn() != spanNow {
		t.Errorf("the span is keyed %q, of %d accounts, across %d columns, now in %d; want %q, 2, %d, %d",
			got.Key, got.Of, len(got.Columns), got.Timeline.NowColumn(), claude.WeekWindow, spanColumns, spanNow)
	}
}

// Each column is the pool's accounts' room in it summed, each as POOL counts
// it: before now's, as RoomThen judges it at the column's middle, an account
// not known before its first reading of the window; now's, the pool's room
// now; after now's, by each one's projection, falling to where it runs out,
// then none to its reset, then all of it; the share used as the pool's is.
func TestThePoolsSpanSumsEachAccountsRoomAsPoolCountsIt(t *testing.T) {
	got := views.PoolSpan{}.Look(spanFrom(stackAt, spanReadings, spanPool()...))
	// work falls 0.2 a day, running out two days on and resetting four days
	// on; personal 0.15 a day, running out 4⅔ days on.
	tests := []struct {
		col  int
		want float64
	}{
		// Six hours into work's week: personal's not begun, nor read in it.
		{col: 0, want: 0.8},
		// 18 hours before now: work as read an hour in, personal likewise.
		{col: 4, want: 0.8 + 0.9},
		{col: spanNow, want: 0.4 + 0.7},
		// 18 hours on.
		{col: 7, want: (0.4 - 0.2*0.75) + (0.7 - 0.15*0.75)},
		// 54 hours on: work run out.
		{col: 10, want: 0 + (0.7 - 0.15*2.25)},
		// 114 hours on: work reset, full; personal run out.
		{col: 15, want: 1},
	}
	for _, tt := range tests {
		col := got.Columns[tt.col]
		if !col.Known || !near(col.Room, tt.want) || !near(col.Used, status.PoolUsed(tt.want, 2)) {
			t.Errorf("column %d is %+v, want room %v, used %v, known", tt.col, col, tt.want, status.PoolUsed(tt.want, 2))
		}
	}
}

// Every column is what POOL's stack counts of each account there, summed:
// before now's at the column's moment, by RoomThen from the readings; now's;
// and ahead by RoomAhead.
func TestThePoolsSpanAgreesWithThePoolsStackColumnForColumn(t *testing.T) {
	from := spanFrom(stackAt, spanReadings, spanPool()...)
	got := views.PoolSpan{}.Look(from)
	m := from.Moment
	read := status.Index(spanReadings)
	for c, col := range got.Columns {
		var want float64
		for _, id := range m.Pool().Accounts {
			a, _ := m.Account(id)
			w, _ := a.Window(claude.WeekWindow)
			at := got.Timeline.Moment(c)
			switch {
			case c < spanNow:
				room, _ := m.RoomThen(a, read, m.Told(id), claude.WeekWindow, at)
				want += room
			case c == spanNow:
				want += w.Room
			default:
				ahead, _ := a.RoomAhead(claude.WeekWindow, at)
				want += max(ahead-(1-a.Floor), 0)
			}
		}
		if !near(col.Room, want) {
			t.Errorf("column %d holds %v, want %v, its accounts' summed", c, col.Room, want)
		}
	}
}

// The pool's span has no columns where there's no pool, as of one account;
// where the pool has no such window; where none of its accounts' spans in it
// is known, as of accounts not read; or where there are no columns.
func TestThePoolsSpanIsNoneWithoutAPoolsSpan(t *testing.T) {
	unread := spanPool()
	for i := range unread {
		unread[i].FetchedAt, unread[i].Windows = time.Time{}, nil
	}
	noColumns := spanFrom(stackAt, spanReadings, spanPool()...)
	noColumns.Columns = 0
	otherWindow := spanFrom(stackAt, spanReadings, spanPool()...)
	otherWindow.Key = claude.FiveHourWindow
	tests := []struct {
		name string
		from views.PoolRoomFrom
	}{
		{name: "one account", from: spanFrom(stackAt, spanReadings, spanPool()[0])},
		{name: "no such window", from: otherWindow},
		{name: "nothing read", from: spanFrom(stackAt, nil, unread...)},
		{name: "no columns", from: noColumns},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (views.PoolSpan{}).Look(tt.from); len(got.Columns) != 0 || got.Of != 0 {
				t.Errorf("the span has %d columns of %d accounts, want none", len(got.Columns), got.Of)
			}
		})
	}
}

// A look judges the span's past once, an account a column, and holds each
// column while what it's judged from by its moment stands: the next look a
// second on judges none, and builds what a first look would; a look with no
// pool holds what was judged, for the look after.
func TestALookJudgesThePoolsSpanOnceWhileWhatItsJudgedFromStands(t *testing.T) {
	var judged int
	first := views.CountingPoolSpan(&judged).Look(spanFrom(stackAt, spanReadings, spanPool()...))
	if judged != 2*spanNow {
		t.Errorf("the first look judged %d times, want %d", judged, 2*spanNow)
	}
	judged = 0
	later := spanFrom(stackAt.Add(time.Second), spanReadings, spanPool()...)
	then := first.Look(spanFrom(stackAt, spanReadings, spanPool()[0])).Look(later)
	if judged != 0 {
		t.Errorf("the looks after judged %d times, want none, through a look without a pool", judged)
	}
	if fresh := (views.PoolSpan{}).Look(later); !slices.Equal(then.Columns, fresh.Columns) {
		t.Errorf("the next look's span is %+v, want %+v, as a first look builds it", then.Columns, fresh.Columns)
	}
}

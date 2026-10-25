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
	"github.com/leeovery/switchboard/internal/views"
)

// stackAt is when the pool's stack is built in its tests: 14:20 on
// Wednesday 14 October 2026, charted across 168 columns, an hour each, from
// 14:20 the day before, so now's is the 25th, column 24, and each column
// after it stands for its middle, column 24 + k for k hours and a half on.
var stackAt = time.Date(2026, 10, 14, 14, 20, 0, 0, time.UTC)

const (
	stackColumns = 168
	stackNow     = 24
)

// stackReset is when the week resets in the stack's tests: four days on, so
// it's three days in.
var stackReset = stackAt.Add(4 * quota.Day)

// stackRead is a reading of the week of the account with the given id, at the
// use given, resetting at stackReset, read at at.
func stackRead(id string, at time.Time, used float64) readings.Reading {
	return readings.Reading{At: at, Account: id, Key: claude.WeekWindow, Utilization: used, ResetsAt: stackReset, Source: readings.FromAnswer}
}

// weekAccount is the account with the given id, read at now, its week used
// as given, resetting at reset, each change made to it.
func weekAccount(id string, now time.Time, used float64, reset time.Time, changes ...func(*status.Account)) status.Account {
	a := status.Account{
		ID: id, Label: id, TokenSet: true, FetchedAt: now,
		Windows: []quota.Window{{Key: claude.WeekWindow, Label: "Week", Utilization: used, ResetsAt: reset}},
	}
	for _, change := range changes {
		change(&a)
	}
	return a
}

// keeping has the account keep back the reserve given.
func keeping(reserve float64) func(*status.Account) {
	return func(a *status.Account) { a.Reserve = reserve }
}

// stackFrom is what the pool's stack is built from at now, of the router's
// document of the accounts given then and the readings given, read through
// now, across stackColumns.
func stackFrom(now time.Time, read []readings.Reading, accounts ...status.Account) views.PoolStackFrom {
	doc := status.Document{GeneratedAt: now, Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: accounts}
	return views.PoolStackFrom{Moment: doc.At(now, claude.Policy()), Readings: read, ReadThrough: now, Columns: stackColumns}
}

// pinnedStack is from, the global pin naming the accounts with the given ids.
func pinnedStack(from views.PoolStackFrom, accounts []status.Account, ids ...string) views.PoolStackFrom {
	doc := status.Document{GeneratedAt: stackAt, Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: accounts}
	doc.Pin = status.Pin{Accounts: ids, Since: stackAt.Add(-time.Hour)}
	from.Moment = doc.At(stackAt, claude.Policy())
	return from
}

// stackedOf is the room of the account with the given id in the stack, by
// column: nil where the stack hasn't it.
func stackedOf(s views.PoolStack, id string) []views.AccountRoom {
	for _, a := range s.Accounts {
		if a.ID == id {
			return a.Rooms
		}
	}
	return nil
}

// near reports whether got is want, to within a rounding.
func near(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

// The pool's stack is charted over Runway's week view, its timeline the live
// week's at the width given, keyed by the week view's window.
func TestThePoolsStackIsChartedOverRunwaysWeek(t *testing.T) {
	from := stackFrom(stackAt, nil, weekAccount("work", stackAt, 0.3, stackReset), weekAccount("personal", stackAt, 0.3, stackReset))
	got := views.PoolStack{}.Look(from)
	if want := views.LiveTimeline(views.RunwayWeek, stackAt, stackColumns); got.Timeline != want {
		t.Errorf("the stack's timeline is %+v, want Runway's week's, %+v", got.Timeline, want)
	}
	if got.Key != claude.WeekWindow || got.Timeline.NowColumn() != stackNow {
		t.Errorf("the stack is keyed %q, now in column %d; want %q, %d", got.Key, got.Timeline.NowColumn(), claude.WeekWindow, stackNow)
	}
	for _, a := range got.Accounts {
		if len(a.Rooms) != stackColumns {
			t.Errorf("%s has %d columns, want %d", a.ID, len(a.Rooms), stackColumns)
		}
	}
}

// Before now's column, each account's room is its room then, as RoomThen
// judges it at the column's moment, its middle: none known before the first
// reading of it, then its floor less its use as last read, so an account
// keeping a reserve counts its room to its cap. Now's is its room now.
func TestThePoolsStackBeforeNowIsEachAccountsRoomThen(t *testing.T) {
	read := []readings.Reading{
		stackRead("personal", stackAt.Add(-28*time.Hour), 0.2),
		stackRead("work", stackAt.Add(-18*time.Hour-20*time.Minute), 0.5),
		stackRead("work", stackAt.Add(-6*time.Hour-20*time.Minute), 0.6),
		stackRead("personal", stackAt, 0.3),
		stackRead("work", stackAt, 0.6),
	}
	from := stackFrom(stackAt, read, weekAccount("work", stackAt, 0.6, stackReset, keeping(0.2)), weekAccount("personal", stackAt, 0.3, stackReset))
	got := views.PoolStack{}.Look(from)
	// work's first reading, at 20:00, falls in column 5, after its middle;
	// its second, at 08:00, in column 17, after its middle too.
	work := func(c int) views.AccountRoom {
		switch {
		case c <= 5:
			return views.AccountRoom{}
		case c <= 17:
			return views.AccountRoom{Room: 0.8 - 0.5, Known: true}
		}
		return views.AccountRoom{Room: 0.8 - 0.6, Known: true}
	}
	personal := func(c int) views.AccountRoom {
		if c == stackNow {
			return views.AccountRoom{Room: 0.7, Known: true}
		}
		return views.AccountRoom{Room: 0.8, Known: true}
	}
	for id, want := range map[string]func(int) views.AccountRoom{"work": work, "personal": personal} {
		rooms := stackedOf(got, id)
		for c := range stackNow + 1 {
			if w := want(c); rooms[c].Known != w.Known || !near(rooms[c].Room, w.Room) {
				t.Errorf("%s in column %d has %+v, want %+v", id, c, rooms[c], w)
			}
		}
	}
}

// From now on, each account's room is its room by its projection at each
// column's moment, as RoomAhead has it, counted as the room left to use
// before its floor: falling to where it runs out, then none to its reset,
// then all of it, its floor; held by its limit, none till it lifts; at its
// cap, none to its reset; and where nothing's projected, its room now, held.
func TestThePoolsStackAheadIsEachAccountsRoomByItsProjection(t *testing.T) {
	type at struct {
		k    int
		room float64
	}
	// The hours on from now column stackNow + k stands for.
	on := func(k int) float64 { return float64(k) + 0.5 }
	tests := []struct {
		name    string
		account status.Account
		want    []at
	}{
		// 60% used three days in, it runs out two days on, at 48 hours.
		{name: "running out", account: weekAccount("work", stackAt, 0.6, stackReset), want: []at{
			{k: 0, room: 0.4},
			{k: 1, room: 0.4 - 0.2*on(1)/24},
			{k: 47, room: 0.4 - 0.2*on(47)/24},
			{k: 48, room: 0},
			{k: 95, room: 0},
			{k: 96, room: 1},
		}},
		// Keeping a fifth, it reaches its cap, 0.8, a day on.
		{name: "running out to its reserve", account: weekAccount("work", stackAt, 0.6, stackReset, keeping(0.2)), want: []at{
			{k: 0, room: 0.2},
			{k: 1, room: 0.2 - 0.2*on(1)/24},
			{k: 23, room: 0.2 - 0.2*on(23)/24},
			{k: 24, room: 0},
			{k: 95, room: 0},
			{k: 96, room: 0.8},
		}},
		// 30% used three days in, it heads for 70% by its reset.
		{name: "lasting", account: weekAccount("work", stackAt, 0.3, stackReset), want: []at{
			{k: 0, room: 0.7},
			{k: 1, room: 0.7 - 0.1*on(1)/24},
			{k: 95, room: 0.7 - 0.1*on(95)/24},
			{k: 96, room: 1},
		}},
		{name: "held by its limit for two hours", account: weekAccount("work", stackAt, 0.3, stackReset, func(a *status.Account) {
			a.Limit = status.Limit{Windows: []string{claude.WeekWindow}, Until: stackAt.Add(2 * time.Hour)}
		}), want: []at{
			{k: 0, room: 0},
			{k: 1, room: 0},
			{k: 96, room: 1},
		}},
		// Keeping a tenth, at 92% it's past its cap, 0.9.
		{name: "at its cap", account: weekAccount("work", stackAt, 0.92, stackReset, keeping(0.1)), want: []at{
			{k: 0, room: 0},
			{k: 1, room: 0},
			{k: 95, room: 0},
			{k: 96, room: 0.9},
		}},
		{name: "stale, so not projected", account: weekAccount("work", stackAt, 0.3, stackReset, func(a *status.Account) {
			a.Error = "HTTP 529 · Overloaded"
		}), want: []at{
			{k: 0, room: 0.7},
			{k: 1, room: 0.7},
			{k: 96, room: 0.7},
			{k: stackColumns - stackNow - 1, room: 0.7},
		}},
		{name: "without a usable token", account: weekAccount("work", stackAt, 0.3, stackReset, func(a *status.Account) { a.TokenSet = false }), want: []at{
			{k: 0, room: 0},
			{k: 1, room: 0},
			{k: 96, room: 0},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := views.PoolStack{}.Look(stackFrom(stackAt, nil, tt.account, weekAccount("personal", stackAt, 0.3, stackReset)))
			rooms := stackedOf(got, "work")
			for _, w := range tt.want {
				if r := rooms[stackNow+w.k]; !r.Known || !near(r.Room, w.room) {
					t.Errorf("column now + %d has %+v, want %v, known", w.k, r, w.room)
				}
			}
		})
	}
}

// An account keeping a reserve has the room then and the room ahead counted
// alike, to its cap, so its room meets at now without a step: the last
// column before now's as read, now's, and the first after, a column's fall
// on.
func TestThePoolsStackMeetsAtNowForAnAccountKeepingAReserve(t *testing.T) {
	read := []readings.Reading{stackRead("work", stackAt.Add(-2*time.Hour), 0.6), stackRead("personal", stackAt.Add(-2*time.Hour), 0.3)}
	from := stackFrom(stackAt, read, weekAccount("work", stackAt, 0.6, stackReset, keeping(0.2)), weekAccount("personal", stackAt, 0.3, stackReset))
	rooms := stackedOf(views.PoolStack{}.Look(from), "work")
	before, now, after := rooms[stackNow-1].Room, rooms[stackNow].Room, rooms[stackNow+1].Room
	// A day's fall, 0.2, an hour and a half of it.
	fall := 0.2 * 1.5 / 24
	if !near(before, 0.2) || !near(now, 0.2) || !near(after, now-fall) {
		t.Errorf("work's room is %v, %v at now, then %v; want 0.2, 0.2, then %v", before, now, after, now-fall)
	}
}

// The stack holds the pool's accounts alone, as the pool counts them, in the
// config's order: none the global pin leaves out, one where it names one,
// and none where there's no pool, as of one account, nor where the pool has
// no week, nor where there are no columns.
func TestThePoolsStackHoldsThePoolsAccountsAlone(t *testing.T) {
	accounts := []status.Account{
		weekAccount("work", stackAt, 0.3, stackReset), weekAccount("personal", stackAt, 0.4, stackReset), weekAccount("side", stackAt, 0.5, stackReset),
	}
	all := stackFrom(stackAt, nil, accounts...)
	noWeek := stackFrom(stackAt, nil, accounts...)
	noWeek.Moment = status.Document{
		GeneratedAt: stackAt, Source: status.SourceRouter, Router: status.Health{Healthy: true},
		Accounts: []status.Account{{ID: "work", Label: "work", TokenSet: true, FetchedAt: stackAt}, {ID: "personal", Label: "personal", TokenSet: true, FetchedAt: stackAt}},
	}.At(stackAt, claude.Policy())
	noColumns := all
	noColumns.Columns = 0
	tests := []struct {
		name string
		from views.PoolStackFrom
		want []string
	}{
		{name: "every account", from: all, want: []string{"work", "personal", "side"}},
		{name: "the pin leaving one out", from: pinnedStack(all, accounts, "side", "work"), want: []string{"work", "side"}},
		{name: "the pin naming one", from: pinnedStack(all, accounts, "personal"), want: []string{"personal"}},
		{name: "one account", from: stackFrom(stackAt, nil, accounts[0]), want: nil},
		{name: "no week", from: noWeek, want: nil},
		{name: "no columns", from: noColumns, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ids []string
			for _, a := range (views.PoolStack{}).Look(tt.from).Accounts {
				ids = append(ids, a.ID)
				if a.Label != a.ID {
					t.Errorf("%s is labelled %q, want %q, as configured", a.ID, a.Label, a.ID)
				}
			}
			if !slices.Equal(ids, tt.want) {
				t.Errorf("the stack holds %v, want %v", ids, tt.want)
			}
		})
	}
}

// A look judges the stack's past once, an account a column, and holds each
// column while what it's judged from by its moment stands: the next look a
// second on, or with a reading gained at now, judges none; ten minutes on,
// when the timeline moves to its next ten minutes and every column's moment
// with it, judges them all again. Each builds the stack a first look would.
func TestALookJudgesThePoolsStackOnceWhileWhatItsJudgedFromStands(t *testing.T) {
	read := []readings.Reading{stackRead("work", stackAt.Add(-30*time.Hour), 0.5), stackRead("personal", stackAt.Add(-30*time.Hour), 0.2)}
	at := func(d time.Duration, read []readings.Reading) views.PoolStackFrom {
		return stackFrom(stackAt.Add(d), read, weekAccount("work", stackAt, 0.5, stackReset), weekAccount("personal", stackAt, 0.2, stackReset))
	}
	gainedNow := append(slices.Clip(read), stackRead("work", stackAt.Add(time.Second), 0.55))
	tests := []struct {
		name string
		then views.PoolStackFrom
		more int
	}{
		{name: "a second on", then: at(time.Second, read), more: 0},
		{name: "a reading gained at now", then: at(time.Second, gainedNow), more: 0},
		{name: "ten minutes on", then: at(10*time.Minute, read), more: 2 * stackNow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var judged int
			first := views.CountingPoolStack(&judged).Look(at(0, read))
			if judged != 2*stackNow {
				t.Errorf("the first look judged %d times, want %d", judged, 2*stackNow)
			}
			judged = 0
			then := first.Look(tt.then)
			if judged != tt.more {
				t.Errorf("the next look judged %d times, want %d", judged, tt.more)
			}
			fresh := views.PoolStack{}.Look(tt.then)
			if !slices.EqualFunc(fresh.Accounts, then.Accounts, sameStacked) {
				t.Errorf("the next look's stack is %+v, want %+v, as a first look builds it", then.Accounts, fresh.Accounts)
			}
		})
	}
}

// sameStacked reports whether a and b are the one account's room alike.
func sameStacked(a, b views.PoolStackAccount) bool {
	return a.ID == b.ID && a.Label == b.Label && slices.Equal(a.Rooms, b.Rooms)
}

// Across the clocks going back, Runway's week is still a week of columns an
// hour each by the clock that doesn't change: the past judged once a column
// at its middle, and the room ahead falling evenly across the change, at 2:00
// on Sunday 25 October 2026 in London, six hours after now.
func TestThePoolsStackAcrossTheClocksGoingBack(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skipf("no Europe/London: %v", err)
	}
	now := time.Date(2026, 10, 24, 20, 0, 0, 0, london)
	reset := now.Add(4 * quota.Day)
	from := stackFrom(now, nil, weekAccount("work", now, 0.6, reset), weekAccount("personal", now, 0.3, reset))
	var judged []time.Time
	got := views.NotingPoolStack(&judged).Look(from)
	tl := got.Timeline
	if tl.Columns != stackColumns || tl.NowColumn() != stackNow || tl.End.Sub(tl.Start) != 7*quota.Day {
		t.Fatalf("the week is %s across %d columns, now in column %d; want a week across %d, now in %d", tl.End.Sub(tl.Start), tl.Columns, tl.NowColumn(), stackColumns, stackNow)
	}
	var want []time.Time
	for c := range stackNow {
		want = append(want, tl.Moment(c), tl.Moment(c))
	}
	if !slices.EqualFunc(judged, want, time.Time.Equal) {
		t.Errorf("the past was judged at %v, want each column's middle once an account, %v", judged, want)
	}
	work := stackedOf(got, "work")
	// 60% used three days in, it falls 0.2 a day, so a column's 0.2 / 24.
	for c := stackNow + 1; c < stackNow+12; c++ {
		if fall := work[c].Room - work[c+1].Room; !near(fall, 0.2/24) {
			t.Errorf("work's room falls %v from column %d to %d, want %v, an hour's", fall, c, c+1, 0.2/24)
		}
	}
}

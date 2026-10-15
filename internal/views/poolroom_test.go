package views_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// poolAt is when the pool's room is worked out in its tests: 14:00, its
// 5-hour charted in five columns, an hour each, at 10:00, 11:00, 12:00,
// 13:00 and now.
var poolAt = time.Date(2026, 10, 14, 14, 0, 0, 0, time.UTC)

// poolClock is the time on poolAt's day at the hour and minute given.
func poolClock(hour, minute int) time.Time {
	return time.Date(2026, 10, 14, hour, minute, 0, 0, time.UTC)
}

// pooledAccount is an account read at the time given, its session at the
// use given resetting at the time given, keeping the reserve given.
func pooledAccount(id string, reserve, used float64, read, resets time.Time) status.Account {
	return status.Account{
		ID: id, Label: id, TokenSet: true, FetchedAt: read, Reserve: reserve,
		Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: used, ResetsAt: resets}},
	}
}

// The pool's three accounts, each session reset at its own time:
//   - work keeps a reserve of a fifth, so runs out at 0.8: its session before
//     reset at 10:30, and the one now started at 11:00;
//   - personal keeps none, so runs out at its limit: its session before
//     reset at 12:00, and the one now started at 13:00, read once since;
//   - side keeps none: its session before reset at 11:30, and the one now
//     started at 12:00.
var (
	pooledWork     = pooledAccount("work", 0.2, 0.4, poolClock(13, 30), poolClock(16, 0))
	pooledPersonal = pooledAccount("personal", 0, 0.2, poolClock(13, 30), poolClock(18, 0))
	pooledSide     = pooledAccount("side", 0, 0.5, poolClock(12, 30), poolClock(17, 0))
)

// poolReadings are each pooled account's readings of its session over the
// 5 hours back from poolAt, with the resets each was to reset at, as the
// readings history's files hold them; or from each session's start alone,
// as a router from before GET /history went back past it gives them.
var (
	poolReadings = map[string][]views.WindowReading{
		"work": {
			{At: poolClock(9, 30), Utilization: 0.5, ResetsAt: poolClock(10, 30)},
			{At: poolClock(11, 30), Utilization: 0.1, ResetsAt: poolClock(16, 0)},
			{At: poolClock(13, 30), Utilization: 0.4, ResetsAt: poolClock(16, 0)},
		},
		"personal": {
			{At: poolClock(9, 0), Utilization: 0.6, ResetsAt: poolClock(12, 0)},
			{At: poolClock(11, 0), Utilization: 0.9, ResetsAt: poolClock(12, 0)},
			{At: poolClock(13, 30), Utilization: 0.2, ResetsAt: poolClock(18, 0)},
		},
		"side": {
			{At: poolClock(9, 0), Utilization: 0.2, ResetsAt: poolClock(11, 30)},
			{At: poolClock(12, 30), Utilization: 0.5, ResetsAt: poolClock(17, 0)},
		},
	}
	fromStartsAlone = map[string][]views.WindowReading{
		"work":     {{At: poolClock(11, 30), Utilization: 0.1}, {At: poolClock(13, 30), Utilization: 0.4}},
		"personal": {{At: poolClock(13, 30), Utilization: 0.2}},
		"side":     {{At: poolClock(12, 30), Utilization: 0.5}},
	}
)

func TestThePoolsRoomIsEachAccountsRoomToWhereItRunsOutSummedSteppingUpAtEachReset(t *testing.T) {
	tests := []struct {
		name     string
		accounts []status.Account
		pinned   []string
		read     map[string][]views.WindowReading
		key      string
		want     views.PoolRoom
	}{
		{
			// At 10:00 work has 0.3 left to its cap, personal 0.4 and side
			// 0.8; at 11:00, work's session has started again, and side's
			// is yet to reset; at 12:00, personal's has reset, though its
			// next is yet to start, and side's has started again.
			name:     "accounts reset at different times, each to its cap or its limit",
			accounts: []status.Account{pooledWork, pooledPersonal, pooledSide},
			read:     poolReadings,
			key:      "5h",
			want:     poolRoomOf(3, 1.5, 1.7, 2.7, 2.2, 1.7),
		},
		{
			// The pin spends work's reserve, so it runs out at its limit.
			name:     "an account out of the pool, not counted",
			accounts: []status.Account{pooledWork, pooledPersonal, pooledSide},
			pinned:   []string{"work", "personal"},
			read:     poolReadings,
			key:      "5h",
			want:     poolRoomOf(2, 0.9, 1.1, 1.9, 1.9, 1.4),
		},
		{
			// None is known at 10:00; at 11:00, work's alone, as its
			// session started; personal, read nothing of before its session
			// started at 13:00, counts none till then.
			name:     "from each session's start alone, each account read nothing of before it counting none",
			accounts: []status.Account{pooledWork, pooledPersonal, pooledSide},
			read:     fromStartsAlone,
			key:      "5h",
			want: views.PoolRoom{Of: 3, Columns: []views.PoolRoomColumn{
				{}, {Room: 0.8, Known: true}, {Room: 1.7, Known: true}, {Room: 2.2, Known: true}, {Room: 1.7, Known: true},
			}},
		},
		{
			name:     "a single account, which has no pool",
			accounts: []status.Account{pooledWork},
			read:     poolReadings,
			key:      "5h",
		},
		{
			name:     "a window the pool hasn't",
			accounts: []status.Account{pooledWork, pooledPersonal, pooledSide},
			read:     poolReadings,
			key:      "7d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: poolAt, Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: tt.accounts}
			if len(tt.pinned) > 0 {
				doc.Pin = status.Pin{Accounts: tt.pinned, Since: poolAt.Add(-time.Hour)}
			}
			got := views.PoolRoomFrom{Moment: doc.At(poolAt, claude.Policy()), Key: tt.key, Read: tt.read}.Room(5)
			if got.Of != tt.want.Of || !slices.EqualFunc(got.Columns, tt.want.Columns, sameRoom) {
				t.Errorf("the pool's room is %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestThePoolsRoomAtNowIsThePoolsAsTheMomentHasIt(t *testing.T) {
	limited := pooledPersonal
	limited.Limit = status.Limit{Windows: []string{"5h"}, Until: poolClock(18, 0)}
	m := status.Document{
		GeneratedAt: poolAt, Source: status.SourceRouter, Router: status.Health{Healthy: true},
		Accounts: []status.Account{pooledWork, limited},
	}.At(poolAt, claude.Policy())

	got := views.PoolRoomFrom{Moment: m, Key: "5h", Read: poolReadings}.Room(5)
	if last := got.Columns[len(got.Columns)-1]; !sameRoom(last, views.PoolRoomColumn{Room: 0.4, Known: true}) {
		t.Errorf("the pool's room at now is %+v, want the moment's, work's 0.4 to its cap, personal, held at its limit, counting none", last)
	}
}

// poolRoomOf is the pool of the accounts given, its room in each column
// that given, each known.
func poolRoomOf(of int, rooms ...float64) views.PoolRoom {
	p := views.PoolRoom{Of: of}
	for _, r := range rooms {
		p.Columns = append(p.Columns, views.PoolRoomColumn{Room: r, Known: true})
	}
	return p
}

// sameRoom reports whether two columns of the pool's room are the same, the
// rooms within a rounding's error.
func sameRoom(a, b views.PoolRoomColumn) bool {
	return a.Known == b.Known && math.Abs(a.Room-b.Room) < 1e-9
}

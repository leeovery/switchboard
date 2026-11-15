package ledger

import (
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
)

// A pool an account did no work in, as one a move of no session off it named
// alone, is none of its pools, so its day lists the same pools made afresh,
// made again over itself and merged with a day of no work.
func TestAPoolOfNoWorkIsNoneOfAnAccountsPoolsHoweverItsDayIsMade(t *testing.T) {
	const date = "2026-10-05"
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local)
	// request is a request of the session given, in the pool given, answered
	// on the account given, moved off from where it's given.
	request := func(id, session, account, from, pool string) Line {
		return Line{At: at, Request: id, Kind: KindMessage, Session: session, Model: "claude-opus-5-5", Account: account, Reason: "sticky",
			From: from, Pool: pool, Status: 200, Attempts: 1}
	}
	tests := []struct {
		name  string
		lines []Line
		// pool is the pool side's day names, and pools the pools it lists,
		// by their ids.
		pool  string
		pools []string
	}{
		{name: "a move of no session off an account that served nothing", lines: []Line{request("1", "", "work", "side", "q7mt")}},
		{
			name:  "served in one pool, and a move of no session off it in another",
			lines: []Line{request("1", "one", "side", "", "b2n8"), request("2", "", "work", "side", "q7mt")},
			pool:  "b2n8",
		},
		{
			name:  "a move of no session off it in one pool, and of a session in another",
			lines: []Line{request("1", "", "work", "side", "b2n8"), request("2", "one", "work", "side", "q7mt")},
			pool:  "q7mt",
		},
		{
			name:  "served in two pools, and a move of no session off it in a third",
			lines: []Line{request("1", "one", "side", "", "b2n8"), request("2", "two", "side", "", "q7mt"), request("3", "", "work", "side", "x9zz")},
			pools: []string{"b2n8", "q7mt"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			none := func(time.Time, time.Time) iter.Seq[readings.Reading] { return readNone }
			made, err := Summarise(date, slices.Values(tt.lines), none, nil, Caps{}, at.AddDate(0, 0, 2))
			if err != nil {
				t.Fatal(err)
			}
			side := sidesDay(made)
			for how, day := range map[string]AccountDay{"made afresh": side, "made again over itself": sidesDay(made.knowing(made)),
				"merged with a day of no work": mergedDays(side, AccountDay{Account: "side"})} {
				if day.Pool != tt.pool || !slices.Equal(idsOf(day.Pools), tt.pools) {
					t.Errorf("%s, side's day names the pool %q and lists %q, want %q and %q", how, day.Pool, idsOf(day.Pools), tt.pool, tt.pools)
				}
			}
		})
	}
}

// sidesDay is side's day in s.
func sidesDay(s Summary) AccountDay {
	i := slices.IndexFunc(s.Accounts, func(a AccountDay) bool { return a.Account == "side" })
	if i < 0 {
		return AccountDay{}
	}
	return s.Accounts[i]
}

// idsOf are the ids of pools, in their order.
func idsOf(pools []PoolDay) []string {
	var ids []string
	for _, p := range pools {
		ids = append(ids, p.Pool)
	}
	return ids
}

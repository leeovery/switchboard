package ledger_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// A summary counts the sessions its accounts' days count but don't name by
// their ids, the sessions it might be of beyond those it names: every one of
// a day summarised before version 2, which named none; none of one that names
// them all; none of an account the readings history alone tells of, which
// served no session; and those of an account kept as an older summary gave
// it, beside one that names its own.
func TestASummaryCountsTheSessionsItDoesntName(t *testing.T) {
	tests := []struct {
		name    string
		summary ledger.Summary
		want    int
	}{
		{
			name:    "version 1, naming none of its sessions",
			summary: ledger.Summary{Version: 1, Day: date, Accounts: []ledger.AccountDay{{Account: "work", Sessions: 2}, {Account: "side", Sessions: 1}}},
			want:    3,
		},
		{
			name: "version 2, naming every one",
			summary: ledger.Summary{Version: 2, Day: date, Accounts: []ledger.AccountDay{
				{Account: "work", Sessions: 2, SessionIDs: []string{"one", "two"}}, {Account: "side", Sessions: 1, SessionIDs: []string{"one"}},
			}},
		},
		{
			name: "a day of readings alone, of no session, before version 2 and since",
			summary: ledger.Summary{Version: 4, Day: date, Accounts: []ledger.AccountDay{
				{Account: "work", Highest: map[string]float64{"5h": 0.2}},
				{Account: "side", Sessions: 0, SessionIDs: []string{}, Highest: map[string]float64{"5h": 0.4}},
			}},
		},
		{
			name: "some unnamed, an account kept as a summary of version 1 gave it",
			summary: ledger.Summary{Version: 4, Day: date, Accounts: []ledger.AccountDay{
				{Account: "work", Sessions: 2}, {Account: "side", Sessions: 1, SessionIDs: []string{"one"}},
			}},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.summary.Unnamed(); got != tt.want {
				t.Errorf("Unnamed() of %+v = %d, want %d", tt.summary.Accounts, got, tt.want)
			}
		})
	}
}

// A model's day counts the sessions it doesn't name as an account's does:
// those it counts beyond its ids, not only every one where it names none.
func TestAModelsDayCountsTheSessionsItDoesntName(t *testing.T) {
	tests := []struct {
		name string
		day  ledger.ModelDay
		want int
	}{
		{name: "before version 4, naming none of its sessions", day: ledger.ModelDay{Upstream: 4, Sessions: 3}, want: 3},
		{name: "naming every one", day: ledger.ModelDay{Upstream: 4, Sessions: 2, SessionIDs: []string{"one", "two"}}},
		{name: "of no session", day: ledger.ModelDay{Checks: 1, SessionIDs: []string{}}},
		{name: "naming some of those it counts", day: ledger.ModelDay{Upstream: 4, Sessions: 3, SessionIDs: []string{"one"}}, want: 2},
	}
	for _, tt := range tests {
		if got := tt.day.Unnamed(); got != tt.want {
			t.Errorf("%s: Unnamed() of %+v = %d, want %d", tt.name, tt.day, got, tt.want)
		}
	}
}

// A session's lines are read of a day whose summary might be of it, and of
// no other: a day before version 2 whose accounts served no session, its
// readings alone telling of them, never is, so a quota check of the session's
// there, which a summary counts toward no session, is passed over.
func TestASessionsLinesAreNotReadOfADayOfNoSessions(t *testing.T) {
	state, dir, _ := stateDirs(t)
	check := asked("1", on(-1, 9, 0))
	check.Kind = ledger.KindCheck
	holdLines(t, dir, "2026-10-04", check)
	writeFile(t, dir, "day-2026-10-04.json",
		[]byte(`{"version":1,"day":"2026-10-04","lines":1,"accounts":[{"account":"work","sessions":0,"moved_on":0,"moved_off":0}]}`+"\n"))
	holdLines(t, dir, date, asked("2", on(0, 9, 0)))
	follower := followerOf(state, &testClock{at: on(0, 12, 0)}, caps)

	var got []string
	for h := range follower.Session("one") {
		got = append(got, h.Request)
	}
	if want := []string{"2"}; !slices.Equal(got, want) {
		t.Errorf("Session() gave requests %s, want %s, today's alone", strings.Join(got, ", "), strings.Join(want, ", "))
	}
}

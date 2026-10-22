package views_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/views"
)

// busyRow is a session running on an account, as BusiestFirst orders it: the
// account's id, the session's, its requests a minute at now, and how long
// before now it was last seen.
type busyRow struct {
	account, session string
	rate             int
	seen             time.Duration
}

// busyRunning are the rows as RunningOf lists them, and the traces that
// count each one's session at its rate: that many requests sent in the
// minute to now.
func busyRunning(rows []busyRow) ([]views.Running, views.Traces) {
	running := make([]views.Running, len(rows))
	var told []views.Told
	for i, r := range rows {
		running[i] = views.Running{Account: r.account, Session: r.session, Activity: views.Activity{LastSeen: now.Add(-r.seen)}}
		for n := range r.rate {
			request := fmt.Sprintf("%s-%d", r.session, n)
			told = append(told, views.Told{Request: request, Session: r.session, Account: r.account, Sent: now.Add(-time.Duration(n+1) * time.Second)})
		}
	}
	return running, views.Traces{}.Told(told, now)
}

// rowsNamed are the running rows' sessions, in their order.
func rowsNamed(running []views.Running) []string {
	names := make([]string, len(running))
	for i, r := range running {
		names[i] = r.Session
	}
	return names
}

func TestSESSIONSListsTheBusiestAccountsFirstEachAccountsSessionsTogether(t *testing.T) {
	tests := []struct {
		name string
		rows []busyRow
		want []string
	}{
		{
			name: "the accounts by their sessions' requests a minute",
			rows: []busyRow{{account: "work", session: "w1", rate: 1}, {account: "side", session: "s1", rate: 2}, {account: "side", session: "s2", rate: 3}, {account: "personal", session: "p1", rate: 4}},
			want: []string{"s2", "s1", "p1", "w1"},
		},
		{
			name: "the grouping wins: an idle session of a busy account before a busier one of a quieter account",
			rows: []busyRow{{account: "work", session: "w1", rate: 6}, {account: "work", session: "w2"}, {account: "personal", session: "p1", rate: 5}},
			want: []string{"w1", "w2", "p1"},
		},
		{
			name: "accounts as busy, by how many run",
			rows: []busyRow{{account: "work", session: "w1", rate: 2}, {account: "side", session: "s1", rate: 1}, {account: "side", session: "s2", rate: 1}},
			want: []string{"s1", "s2", "w1"},
		},
		{
			name: "accounts as busy with as many running, in USAGE's order",
			rows: []busyRow{{account: "side", session: "s1", rate: 1}, {account: "personal", session: "p1", rate: 1}, {account: "work", session: "w1", rate: 1}},
			want: []string{"w1", "p1", "s1"},
		},
		{
			name: "an account's sessions by their requests a minute, then the one seen last",
			rows: []busyRow{
				{account: "side", session: "s1", seen: 9 * time.Minute}, {account: "side", session: "s2", rate: 2, seen: 3 * time.Minute},
				{account: "side", session: "s3", seen: time.Minute}, {account: "side", session: "s4", rate: 2},
			},
			want: []string{"s4", "s2", "s3", "s1"},
		},
	}
	m := runningDoc().At(now, claude.Policy())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			running, traces := busyRunning(tt.rows)
			listed := slices.Clone(running)
			if got := rowsNamed(views.BusiestFirst(m, running, traces)); !slices.Equal(got, tt.want) {
				t.Errorf("BusiestFirst = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(running, listed) {
				t.Error("the sessions given were reordered")
			}
		})
	}
}

// A session whose models go to two accounts is a row on each: each row
// counts its session's requests a minute toward its own account.
func TestASessionOnTwoAccountsCountsTowardEach(t *testing.T) {
	running, traces := busyRunning([]busyRow{
		{account: "work", session: "w1", rate: 2}, {account: "personal", session: "both", rate: 3},
		{account: "side", session: "s1", rate: 1}, {account: "side", session: "s2", rate: 1},
	})
	running = append(running, views.Running{Account: "side", Session: "both"})
	if got, want := rowsNamed(views.BusiestFirst(runningDoc().At(now, claude.Policy()), running, traces)), []string{"both", "s1", "s2", "both", "w1"}; !slices.Equal(got, want) {
		t.Errorf("BusiestFirst = %q, want %q: side's 5 a minute, both's 3 among them, then personal's 3, its one session to side's three", got, want)
	}
}

func TestSESSIONSKeepsItsRowsInTheOrderTakenTillTheNextLayout(t *testing.T) {
	row := func(account, session string) views.Running { return views.Running{Account: account, Session: session} }
	kept := views.RowsOf([]views.Running{row("side", "s1"), row("side", "s2"), row("work", "w1")})
	tests := []struct {
		name    string
		running []views.Running
		want    []views.Running
	}{
		{
			name:    "as rates change, nothing reorders",
			running: []views.Running{row("work", "w1"), row("side", "s2"), row("side", "s1")},
			want:    []views.Running{row("side", "s1"), row("side", "s2"), row("work", "w1")},
		},
		{
			name:    "a session new since, last",
			running: []views.Running{row("personal", "p1"), row("work", "w1"), row("side", "s1"), row("side", "s2")},
			want:    []views.Running{row("side", "s1"), row("side", "s2"), row("work", "w1"), row("personal", "p1")},
		},
		{
			name:    "a session ended, its row left blank",
			running: []views.Running{row("work", "w1"), row("side", "s2")},
			want:    []views.Running{{}, row("side", "s2"), row("work", "w1")},
		},
		{
			name:    "a session moved off an account, its row there blank, and on the account it went to, new",
			running: []views.Running{row("side", "s2"), row("work", "w1"), row("work", "s1")},
			want:    []views.Running{{}, row("side", "s2"), row("work", "w1"), row("work", "s1")},
		},
		{
			name:    "none running",
			running: nil,
			want:    []views.Running{{}, {}, {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.Steady(kept, tt.running); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Steady =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

package views

import (
	"cmp"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/status"
)

// BusiestFirst are the sessions running, as RunningOf lists them at the
// moment m, in the order the Overview's SESSIONS lists them: the accounts
// busiest first, by the requests a minute their sessions sent, as the
// traces count each session's at m's Now, then by how many run there, then
// in USAGE's order, the config's; and each account's sessions together,
// the busiest first, then the one seen last. So an idle session of a busy
// account comes before a busier one of a quieter account: the grouping
// wins. The sessions given are left as they were.
func BusiestFirst(m status.Moment, running []Running, traces Traces) []Running {
	type busy struct{ rate, count, place int }
	rates, accounts := make(map[string]int, len(running)), make(map[string]busy)
	for _, r := range running {
		if _, ok := rates[r.Session]; !ok {
			rates[r.Session] = traces.Sessions[r.Session].Rate(m.Now)
		}
		a, ok := accounts[r.Account]
		if !ok {
			now, _ := m.Account(r.Account)
			a.place = now.Place
		}
		a.rate, a.count = a.rate+rates[r.Session], a.count+1
		accounts[r.Account] = a
	}
	return slices.SortedStableFunc(slices.Values(running), func(x, y Running) int {
		if x.Account != y.Account {
			ax, ay := accounts[x.Account], accounts[y.Account]
			return cmp.Or(cmp.Compare(ay.rate, ax.rate), cmp.Compare(ay.count, ax.count),
				cmp.Compare(ax.place, ay.place), strings.Compare(x.Account, y.Account))
		}
		return cmp.Or(cmp.Compare(rates[y.Session], rates[x.Session]), y.Activity.LastSeen.Compare(x.Activity.LastSeen))
	})
}

// RunningRow is a row of the sessions running: its account's id, and its
// session's.
type RunningRow struct {
	Account, Session string
}

// Row is the row the session running r is.
func (r Running) Row() RunningRow {
	return RunningRow{Account: r.Account, Session: r.Session}
}

// Blank reports whether r is a row Steady left blank, its session no longer
// running there.
func (r Running) Blank() bool {
	return r.Session == ""
}

// RowsOf are the rows of the sessions running, in their order.
func RowsOf(running []Running) []RunningRow {
	rows := make([]RunningRow, len(running))
	for i, r := range running {
		rows[i] = r.Row()
	}
	return rows
}

// Steady are the sessions running in the order of the rows kept, as the
// Overview's SESSIONS keeps them from the layout they were taken at to the
// next: each row kept, as its session runs there now, or the zero Running
// where it no longer does, so its row is left blank; then each session
// running on a row none kept, in running's order, so a session new since
// comes last.
func Steady(kept []RunningRow, running []Running) []Running {
	now := make(map[RunningRow]Running, len(running))
	for _, r := range running {
		now[r.Row()] = r
	}
	steady := make([]Running, 0, len(kept)+len(running))
	for _, row := range kept {
		steady = append(steady, now[row])
		delete(now, row)
	}
	for _, r := range running {
		if _, unkept := now[r.Row()]; unkept {
			steady = append(steady, r)
		}
	}
	return steady
}

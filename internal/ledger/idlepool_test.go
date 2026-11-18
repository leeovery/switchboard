package ledger_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
)

// placedIn places every account in the pool with the given id, as the
// router's pools would; placedNowhere places none, as of accounts no pool
// holds.
func placedIn(pool string) func(string) (string, bool) {
	return func(string) (string, bool) { return pool, true }
}

func placedNowhere(string) (string, bool) { return "", false }

// idleDay holds the lines and readings of the idle pool tests' day in the
// state directory: work's request in acme, and personal's window read,
// personal sending nothing.
func idleDay(t *testing.T) (state, dir, history string) {
	t.Helper()
	state, dir, history = stateDirs(t)
	holdLines(t, dir, date, inPool(asked("1", on(0, 9, 0)), acme))
	holdReadings(t, history, date, personalRead(on(0, 9, 30), "5h", 0.95, on(0, 13, 0)))
	return state, dir, history
}

// summarisedBy has the router's ledger in dir summarise the days ended at
// at, with the readings history in history, its accounts placed by poolOf,
// as IdlePools has it, none placed where it's nil, and returns the summary
// it holds of the tests' day.
func summarisedBy(t *testing.T, dir, history string, at time.Time, poolOf func(string) (string, bool)) ledger.Summary {
	t.Helper()
	var opts []ledger.Option
	if poolOf != nil {
		opts = append(opts, ledger.IdlePools(poolOf))
	}
	ledger.Open(dir, 400*24*time.Hour, func() time.Time { return at }, historyIn(history), caps, logs.For("router"), opts...).SummariseEnded(at)
	return summaryHeld(t, dir)
}

func TestAnAccountsDayOfNoWorkNamesThePoolItWasInAsTheDayWasSummarised(t *testing.T) {
	tests := []struct {
		name   string
		poolOf func(string) (string, bool)
		want   string
	}{
		{name: "placed in a pool", poolOf: placedIn(first), want: first},
		{name: "placed in none, as an account no pool holds", poolOf: placedNowhere},
		{name: "placed in no pool but none, as before pools", poolOf: placedIn("")},
		{name: "by a ledger told no pools", poolOf: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, dir, history := idleDay(t)
			got := summarisedBy(t, dir, history, on(3, 10, 0), tt.poolOf)
			personal, ok := find(got, "personal")
			if !ok || personal.Pool != tt.want || personal.IdlePool() != tt.want || personal.Pools != nil {
				t.Errorf("personal's day of no work is %+v, want it naming the pool %q", personal, tt.want)
			}
			if work := accountOf(got, "work"); work.Pool != acme || work.IdlePool() != "" {
				t.Errorf("work's day is %+v, want its work in %q, the pool its request recorded, whatever pool it's placed in", work, acme)
			}
		})
	}
}

// A day of no work keeps the pool it was first summarised in, however it's
// summarised again: as a line comes to be filed under it, or as a new
// summary version summarises every kept day again; by the router, its
// account moved or removed since, or by a reader as it reads it.
func TestADayOfNoWorkKeepsThePoolItWasFirstSummarisedIn(t *testing.T) {
	tests := []struct {
		name   string
		poolOf func(string) (string, bool)
		by     string
		// older has the day summarised again as one of an older version, as
		// a new version finds it, else as a line is filed under it.
		older bool
	}{
		{name: "a line filed since, its account moved to another pool", poolOf: placedIn(first), by: "the router"},
		{name: "a line filed since, its account removed", poolOf: placedNowhere, by: "the router"},
		{name: "a line filed since, by a reader", by: "a reader"},
		{name: "of an older version, its account moved to another pool", poolOf: placedIn(first), by: "the router", older: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, history := idleDay(t)
			if before := accountOf(summarisedBy(t, dir, history, on(1, 10, 0), placedIn(acme)), "personal"); before.Pool != acme {
				t.Fatalf("personal's day of no work is %+v, summarised first, want it in %q", before, acme)
			}
			requests := 1
			if tt.older {
				written := strings.Replace(heldSummary(t, dir, date), `{"version":7,`, `{"version":6,`, 1)
				writeFile(t, dir, "day-"+date+".json", []byte(written))
			} else {
				holdLines(t, dir, date, inPool(asked("2", on(0, 22, 0)), acme))
				requests = 2
			}

			var got ledger.Summary
			if tt.by == "a reader" {
				got = readerAt(state, on(3, 10, 0)).Days(on(0, 0, 0))[0]
			} else {
				got = summarisedBy(t, dir, history, on(3, 10, 0), tt.poolOf)
			}
			if work := accountOf(got, "work"); got.Version != 7 || len(work.Models) != 1 || work.Models[0].Upstream != requests {
				t.Fatalf("the day is of version %d, work's %+v; want it summarised again, as version 7, of work's %d requests", got.Version, work, requests)
			}
			if personal := accountOf(got, "personal"); personal.Pool != acme {
				t.Errorf("personal's day of no work, summarised again, is %+v, want it in %q still, as it was first summarised", personal, acme)
			}
		})
	}
}

// A day summarised before idle days named their pools names none for an
// account that did no work, and keeps none for good: summarised again by a
// ledger that places the account in a pool, it still names none, so a read
// goes by the pool it's in now.
func TestADayOfNoWorkSummarisedBeforeNamesNoPoolAndKeepsNone(t *testing.T) {
	_, dir, history := idleDay(t)
	if before := accountOf(summarisedBy(t, dir, history, on(1, 10, 0), nil), "personal"); before.Pool != "" {
		t.Fatalf("personal's day of no work is %+v, summarised by a ledger told no pools, want it in none", before)
	}
	holdLines(t, dir, date, inPool(asked("2", on(0, 22, 0)), acme))
	if personal := accountOf(summarisedBy(t, dir, history, on(3, 10, 0), placedIn(acme)), "personal"); personal.Pool != "" {
		t.Errorf("personal's day of no work, summarised again, is %+v, want it in none still", personal)
	}
}

// A day of no work's pool is renamed as its work's would be, as an import
// gives another Mac's pool the one here holding its accounts; one naming
// none is never given the pool work in none is, as which pool it was in was
// never recorded; and two days of one account, renamed so, merge into one
// naming the pool either named, where neither worked, else its work's pool.
func TestADayOfNoWorksPoolIsRenamedAsWorksIs(t *testing.T) {
	const theirs = "x9z9"
	idle := func(account, pool string) string {
		day := map[string]any{"account": account, "sessions": 0, "moved_on": 0, "moved_off": 0, "highest": map[string]float64{"5h": 0.95}}
		if pool != "" {
			day["pool"] = pool
		}
		data, err := json.Marshal(day)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	tests := []struct {
		name    string
		days    []string
		rename  ledger.Renaming
		changes bool
		want    map[string]string
	}{
		{
			name: "another Mac's pool given the one here", days: []string{idle("personal", theirs), idle("spare", "")},
			rename:  ledger.Renaming{Pool: renaming(theirs, acme), Unpooled: first},
			changes: true, want: map[string]string{"personal": acme, "spare": ""},
		},
		{
			name: "none renamed", days: []string{idle("personal", theirs), idle("spare", "")},
			rename: ledger.Renaming{Unpooled: first}, want: map[string]string{"personal": theirs, "spare": ""},
		},
		{
			name: "two ids of one account, one naming a pool", days: []string{idle("ab3k", ""), idle("personal", theirs)},
			rename:  ledger.Renaming{Account: renaming("ab3k", "personal")},
			changes: true, want: map[string]string{"personal": theirs},
		},
		{
			name:    "two ids of one account, one idle naming a pool, the other worked in another",
			days:    []string{`{"account":"ab3k","pool":"` + acme + `",` + workJSON(on(0, 9, 0)) + `}`, idle("personal", theirs)},
			rename:  ledger.Renaming{Account: renaming("ab3k", "personal")},
			changes: true, want: map[string]string{"personal": acme},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, dir, _ := stateDirs(t)
			summary := `{"version":7,"day":"` + date + `","lines":0,"accounts":[` + tt.days[0] + `,` + tt.days[1] + `]}`
			writeFile(t, dir, "day-"+date+".json", []byte(summary+"\n"))
			if got := tt.rename.Changes([]byte(summary)); got != tt.changes {
				t.Errorf("Changes() = %v, want %v", got, tt.changes)
			}
			if err := openedIn(dir, on(3, 10, 0)).RenameSummary(date, tt.rename); err != nil {
				t.Fatalf("RenameSummary() = %v", err)
			}
			got := summaryHeld(t, dir)
			pools := make(map[string]string)
			for _, a := range got.Accounts {
				pools[a.Account] = a.Pool
			}
			if len(pools) != len(tt.want) {
				t.Fatalf("the summary renamed is %s, want the days of %v", heldSummary(t, dir, date), tt.want)
			}
			for account, want := range tt.want {
				if pools[account] != want {
					t.Errorf("%s's day of no work names %q, want %q", account, pools[account], want)
				}
			}
		})
	}
}

// renaming renames the id was to is, leaving any other as it is.
func renaming(was, is string) func(string) string {
	return func(id string) string {
		if id == was {
			return is
		}
		return id
	}
}

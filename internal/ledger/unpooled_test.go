package ledger_test

import (
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
)

// givenFirst is a line of the ledger given the first pool where it names
// none, as the move to pools gives it one.
func givenFirst(t *testing.T) func([]byte) []byte {
	return func(data []byte) []byte {
		var l ledger.Line
		if err := json.Unmarshal(data, &l); err != nil {
			t.Fatal(err)
		}
		if l.Pool != "" {
			return data
		}
		l.Pool = first
		pooled, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		return pooled
	}
}

// summaryHeld is the summary the ledger in dir holds of the tests' day, as it
// reads.
func summaryHeld(t *testing.T, dir string) ledger.Summary {
	t.Helper()
	var s ledger.Summary
	if err := json.Unmarshal([]byte(heldSummary(t, dir, date)), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestADaysWorkInNoPoolRenamedIsTheFirstPoolsAndSummarisedAgainAddsUpToTheWhole(t *testing.T) {
	for _, pruned := range []bool{false, true} {
		t.Run(map[bool]string{false: "its lines kept", true: "its lines gone"}[pruned], func(t *testing.T) {
			dir := t.TempDir()
			plainLines(t, dir)
			markOfVersion(t, dir, ofVersion(summaryOf(date, 2, windowsRead{highest: `{"5h":0.9}`}, theTwo...), 6), 6)
			if pruned {
				if err := os.Remove(dayFile(dir, on(0, 9, 0))); err != nil {
					t.Fatal(err)
				}
			}
			l := openedIn(dir, on(3, 10, 0))
			if err := l.RenameDay(date, givenFirst(t), ledger.Renaming{Unpooled: first}); err != nil {
				t.Fatalf("RenameDay() = %v", err)
			}
			if got := summaryHeld(t, dir); got.Version != 6 || got.Accounts[0].Pool != first {
				t.Fatalf("renamed, the summary is %s, want work's day in %q, of its own version, 6", heldSummary(t, dir, date), first)
			}

			l.SummariseEnded(on(3, 10, 0))
			work := summaryHeld(t, dir).Accounts[0]
			if work.Pool != first || work.Pools != nil || work.Models[0].Upstream != 2 {
				t.Errorf("summarised again, the summary is %s, want work's two requests in %q alone, its pools adding up to its whole",
					heldSummary(t, dir, date), first)
			}
		})
	}
}

// Renaming a summary's work in no pool to a pool gives the summary of its
// lines given that pool, whichever way they split.
func TestASummarysWorkInNoPoolRenamedIsItsLinesInThatPool(t *testing.T) {
	lines, history := busyDay()
	for _, tt := range splits {
		t.Run(tt.name, func(t *testing.T) {
			routed := pooled(lines, tt.pool)
			given := pooled(lines, func(i int) string {
				if pool := tt.pool(i); pool != "" {
					return pool
				}
				return first
			})
			_, dir, _ := stateDirs(t)
			writeFile(t, dir, "day-"+date+".json", append(summaryJSON(t, history, routed...), '\n'))

			if err := openedIn(dir, on(3, 10, 0)).RenameSummary(date, ledger.Renaming{Unpooled: first}); err != nil {
				t.Fatalf("RenameSummary() = %v", err)
			}
			if got, want := summaryHeld(t, dir), summarised(t, history, given...); !reflect.DeepEqual(got, want) {
				t.Errorf("renamed, the summary is\n%+v\nwant that of its lines given %q where they named no pool\n%+v", got, first, want)
			}
		})
	}
}

func TestAnAccountsDayOfNoWorkIsGivenNoPool(t *testing.T) {
	history := readingsOf(personalRead(on(0, 9, 0), "5h", 0.3, on(0, 13, 0)))
	_, dir, _ := stateDirs(t)
	writeFile(t, dir, "day-"+date+".json", append(summaryJSON(t, history, asked("1", on(0, 9, 0))), '\n'))

	if err := openedIn(dir, on(3, 10, 0)).RenameSummary(date, ledger.Renaming{Unpooled: first}); err != nil {
		t.Fatalf("RenameSummary() = %v", err)
	}
	got := summaryHeld(t, dir)
	if personal := accountOf(got, "personal"); personal.Pool != "" || personal.Pools != nil {
		t.Errorf("personal's day, of its windows alone, is %+v, want it in no pool, as it did no work", personal)
	}
	if work := accountOf(got, "work"); work.Pool != first {
		t.Errorf("work's day is %+v, want its work in %q", work, first)
	}
}

// upstreamByPool is the account's requests upstream in each pool it worked
// in, by the pool's id.
func upstreamByPool(a ledger.AccountDay) map[string]int {
	pools := make(map[string]int)
	for _, p := range a.Pools {
		pools[p.Pool] = p.Models[0].Upstream
	}
	if a.Pools == nil && len(a.Models) > 0 {
		pools[a.Pool] = a.Models[0].Upstream
	}
	return pools
}

// A day whose summary is of a version before pools, all its work in none
// whatever its lines named, as a downgrade to a release before pools leaves
// one, is split by pool as its lines are as it's summarised again, its pools
// adding up to its whole: where they name the first pool, as the move gave
// them, its work is the first pool's; where some named none, written while
// downgraded, that work's in none. Where some of its lines were lost since,
// which pool they were in can't be told, so the whole is in the summary's
// one pool. The first pool is never asked for.
func TestASummaryBeforePoolsIsSplitAsItsLinesAreAddingUpToItsWhole(t *testing.T) {
	tests := []struct {
		name  string
		lines []ledger.Line
		want  map[string]int
	}{
		{name: "its lines in the first pool", lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), first), inPool(asked("2", on(0, 10, 0)), first)}, want: map[string]int{first: 2}},
		{name: "its lines in no pool, as before pools", lines: []ledger.Line{asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0))}, want: map[string]int{"": 2}},
		{name: "one in the first pool, one written while downgraded", lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), first), asked("2", on(0, 10, 0))}, want: map[string]int{first: 1, "": 1}},
		{name: "one lost since", lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), first)}, want: map[string]int{"": 2}},
	}
	for _, tt := range tests {
		for _, by := range []string{"the router", "a follower"} {
			t.Run(tt.name+", by "+by, func(t *testing.T) {
				state, dir, _ := stateDirs(t)
				holdLines(t, dir, date, tt.lines...)
				writeFile(t, dir, "day-"+date+".json", []byte(ofVersion(summaryOf(date, 2, windowsRead{}, theTwo...), 6)+"\n"))
				asked := false
				firstPool := ledger.FirstPool(func() string { asked = true; return first })

				work := againBy(t, by, state, dir, firstPool).Accounts[0]
				if got := upstreamByPool(work); work.Models[0].Upstream != 2 || !maps.Equal(got, tt.want) {
					t.Errorf("summarised again, work's day is %+v, its requests upstream by pool %v; want 2 in all, %v", work, got, tt.want)
				}
				if asked {
					t.Error("the first pool was asked for, want it never asked of a summary before pools")
				}
			})
		}
	}
}

// A day whose lines the move to pools rewrote into the first pool, its
// summary, naming its work in none, not yet renamed with them, as where the
// move stopped between the two, is the first pool's work as it's summarised
// again, never in none beside it: so a summary renamed after counts it
// once.
func TestASummaryWhoseLinesWereMovedBeforeItIsTheFirstPoolsSummarisedAgain(t *testing.T) {
	for _, by := range []string{"the router", "a follower"} {
		t.Run(by, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			holdLines(t, dir, date, inPool(asked("1", on(0, 9, 0)), first), inPool(asked("2", on(0, 10, 0)), first), inPool(asked("3", on(0, 11, 0)), first))
			writeFile(t, dir, "day-"+date+".json", []byte(summaryOf(date, 2, windowsRead{}, theTwo...)+"\n"))

			work := againBy(t, by, state, dir, ledger.FirstPool(func() string { return first })).Accounts[0]
			if got := upstreamByPool(work); work.Models[0].Upstream != 3 || !maps.Equal(got, map[string]int{first: 3}) {
				t.Errorf("summarised again, work's day is %+v, its requests upstream by pool %v; want its three in %q alone", work, got, first)
			}
			if by != "the router" {
				return
			}
			if err := openedIn(dir, on(3, 10, 0)).RenameSummary(date, ledger.Renaming{Unpooled: first}); err != nil {
				t.Fatal(err)
			}
			if got := upstreamByPool(summaryHeld(t, dir).Accounts[0]); !maps.Equal(got, map[string]int{first: 3}) {
				t.Errorf("renamed after, work's requests upstream by pool are %v, want its three in %q, counted once", got, first)
			}
		})
	}
}

// againBy is the tests' day in the ledger in dir, of the state directory
// state, summarised again by a router's ledger or a follower, as by says,
// opened with the option given.
func againBy(t *testing.T, by, state, dir string, opt ledger.Option) ledger.Summary {
	t.Helper()
	at := func() time.Time { return on(3, 10, 0) }
	if by == "the router" {
		ledger.Open(dir, 400*24*time.Hour, at, noReadings, caps, logs.For("router"), opt).SummariseEnded(at())
		return summaryHeld(t, dir)
	}
	days := ledger.NewFollower(state, at, caps, digestBody, logs.For("cli"), opt).Days(on(0, 0, 0))
	if len(days) == 0 || days[0].Day != date {
		t.Fatalf("the follower read %+v, want the tests' day first", days)
	}
	return days[0]
}

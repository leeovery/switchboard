package ledger_test

import (
	"encoding/json"
	"fmt"
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

// knowsFirst has the ledger know the first pool, as once the move to pools
// is done; knowsNone, none, as before.
var (
	knowsFirst = ledger.FirstPool(func() string { return first })
	knowsNone  = ledger.FirstPool(func() string { return "" })
)

// A day whose summary is of a version before pools, all its work in none
// whatever its lines named, as a downgrade to a release before pools leaves
// one, is split by pool as its lines are as it's summarised again, its pools
// adding up to its whole: where they name the first pool, as the move gave
// them, its work is the first pool's; where some named none, as a release
// before pools writes them, that work's in none before the move is done,
// and the first pool's once it is. Where some of its lines were lost since,
// which pool they were in can't be told, so the whole is in the summary's
// one pool, none, which once the move is done is the first.
func TestASummaryBeforePoolsIsSplitAsItsLinesAreAddingUpToItsWhole(t *testing.T) {
	tests := []struct {
		name  string
		lines []ledger.Line
		// want is work's requests upstream by pool before the move is done,
		// and moved once it is.
		want, moved map[string]int
	}{
		{
			name:  "its lines in the first pool",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), first), inPool(asked("2", on(0, 10, 0)), first)},
			want:  map[string]int{first: 2}, moved: map[string]int{first: 2},
		},
		{
			name:  "its lines in no pool, as a release before pools writes them",
			lines: []ledger.Line{asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0))},
			want:  map[string]int{"": 2}, moved: map[string]int{first: 2},
		},
		{
			name:  "one in the first pool, one written while downgraded",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), first), asked("2", on(0, 10, 0))},
			want:  map[string]int{first: 1, "": 1}, moved: map[string]int{first: 2},
		},
		{
			name:  "one lost since",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), first)},
			want:  map[string]int{"": 2}, moved: map[string]int{first: 2},
		},
	}
	for _, tt := range tests {
		for _, by := range summarisers {
			for _, moved := range []bool{false, true} {
				t.Run(tt.name+", by "+by+map[bool]string{false: ", before the move", true: ", moved"}[moved], func(t *testing.T) {
					state, dir, _ := stateDirs(t)
					holdLines(t, dir, date, tt.lines...)
					writeFile(t, dir, "day-"+date+".json", []byte(ofVersion(summaryOf(date, 2, windowsRead{}, theTwo...), 6)+"\n"))
					knows, want := knowsNone, tt.want
					if moved {
						knows, want = knowsFirst, tt.moved
					}

					work := againBy(t, by, state, dir, knows).Accounts[0]
					if got := upstreamByPool(work); work.Models[0].Upstream != 2 || !maps.Equal(got, want) {
						t.Errorf("summarised again, work's day is %+v, its requests upstream by pool %v; want 2 in all, %v", work, got, want)
					}
				})
			}
		}
	}
}

// A line filed in no pool once the move to pools is done, as by a router
// draining as another takes over, appending to a day the move has moved, is
// the first pool's work as the day is summarised, beside the lines moved.
func TestALineInNoPoolFiledOnceTheMoveIsDoneIsTheFirstPools(t *testing.T) {
	for _, by := range summarisers {
		t.Run(by, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			holdLines(t, dir, date, inPool(asked("1", on(0, 9, 0)), first), inPool(asked("2", on(0, 10, 0)), first), asked("3", on(0, 11, 0)))

			work := againBy(t, by, state, dir, knowsFirst).Accounts[0]
			if got := upstreamByPool(work); work.Models[0].Upstream != 3 || !maps.Equal(got, map[string]int{first: 3}) {
				t.Errorf("summarised, work's day is %+v, its requests upstream by pool %v; want its three in %q alone", work, got, first)
			}
		})
	}
}

// A day whose lines the move to pools rewrote into the first pool, its
// summary, naming its work in none, not yet renamed with them, as where the
// move stopped between the two, before it's done, is the first pool's work
// as it's summarised again, never in none beside it, as its lines count all
// its summary did: so a summary renamed after counts it once.
func TestASummaryWhoseLinesWereMovedBeforeItIsTheFirstPoolsSummarisedAgain(t *testing.T) {
	for _, by := range summarisers {
		t.Run(by, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			holdLines(t, dir, date, inPool(asked("1", on(0, 9, 0)), first), inPool(asked("2", on(0, 10, 0)), first), inPool(asked("3", on(0, 11, 0)), first))
			writeFile(t, dir, "day-"+date+".json", []byte(summaryOf(date, 2, windowsRead{}, theTwo...)+"\n"))

			work := againBy(t, by, state, dir, knowsNone).Accounts[0]
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

// A day whose summary holds five requests in no pool, as made before its
// lines were given the first pool, and three in Acme, summarised again: once
// the first pool is known, its work in none is the first pool's, so each pool
// knows no less than the summary's, never the summary split as its lines
// are, which would hide Acme's lines lost since behind those filed in the
// first; before, where its lines count all it did, it's split as they are.
func TestASummarysWorkInNoPoolBesideAnothersIsSplitAsItsLinesOnlyWhileTheFirstPoolIsntKnown(t *testing.T) {
	held := append(linesIn("", 5, 9), linesIn(acme, 3, 10)...)
	tests := []struct {
		name  string
		knows ledger.Option
		lines []ledger.Line
		// want are work's requests upstream in every pool together, and in
		// each.
		want  int
		pools map[string]int
	}{
		{
			name:  "the first pool known, two of Acme's lost and three more filed in the first",
			knows: knowsFirst,
			lines: append(linesIn(first, 8, 9), linesIn(acme, 1, 10)...),
			want:  11, pools: map[string]int{first: 8, acme: 3},
		},
		{
			name:  "the first pool not known, one more filed in the first",
			knows: knowsNone,
			lines: append(linesIn(first, 6, 9), linesIn(acme, 3, 10)...),
			want:  9, pools: map[string]int{first: 6, acme: 3},
		},
	}
	for _, tt := range tests {
		for _, by := range summarisers {
			t.Run(tt.name+", by "+by, func(t *testing.T) {
				state, dir, _ := stateDirs(t)
				writeFile(t, dir, "day-"+date+".json", append(summaryJSON(t, noReadings, held...), '\n'))
				holdLines(t, dir, date, tt.lines...)

				work := againBy(t, by, state, dir, tt.knows).Accounts[0]
				if got := upstreamByPool(work); work.Models[0].Upstream != tt.want || !maps.Equal(got, tt.pools) {
					t.Errorf("summarised again, work's day is %+v, its requests upstream by pool %v; want %d in all, %v", work, got, tt.want, tt.pools)
				}
			})
		}
	}
}

// linesIn are n requests asked in the pool given, "" for none, a minute
// apart from the hour given.
func linesIn(pool string, n, hour int) []ledger.Line {
	lines := make([]ledger.Line, n)
	for i := range lines {
		lines[i] = inPool(asked(fmt.Sprintf("%s-%d-%d", pool, hour, i), on(0, hour, i)), pool)
	}
	return lines
}

// summarisers are what summarise a day: the router's ledger, a follower, and
// a reader, as againBy has each.
var summarisers = []string{"the router", "a follower", "a reader"}

// againBy is the tests' day in the ledger in dir, of the state directory
// state, summarised again by one of summarisers, as by says, opened with the
// option given.
func againBy(t *testing.T, by, state, dir string, opt ledger.Option) ledger.Summary {
	t.Helper()
	at := func() time.Time { return on(3, 10, 0) }
	var days []ledger.Summary
	switch by {
	case "the router":
		ledger.Open(dir, 400*24*time.Hour, at, noReadings, caps, logs.For("router"), opt).SummariseEnded(at())
		return summaryHeld(t, dir)
	case "a follower":
		days = ledger.NewFollower(state, at, caps, digestBody, logs.For("cli"), opt).Days(on(0, 0, 0))
	default:
		days = ledger.NewReader(state, at, caps, logs.For("cli"), opt).Days(on(0, 0, 0))
	}
	if len(days) == 0 || days[0].Day != date {
		t.Fatalf("%s read %+v, want the tests' day first", by, days)
	}
	return days[0]
}

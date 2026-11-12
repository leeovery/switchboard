package ledger_test

import (
	"encoding/json"
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

// A day whose summary is of a version before pools, all its work in none,
// while its lines name a pool, as a downgrade to a release before pools
// leaves one, is the first pool's work as it's summarised again.
func TestASummaryBeforePoolsOfADayWhoseLinesNameAPoolIsTheFirstPoolsSummarisedAgain(t *testing.T) {
	tests := []struct {
		name string
		pool string
		// want is the pool work's day is in, summarised again, and asked
		// whether the first pool was asked for.
		want  string
		asked bool
	}{
		{name: "its lines in the first pool", pool: first, want: first, asked: true},
		{name: "its lines in no pool, as before pools", pool: "", want: ""},
	}
	for _, tt := range tests {
		for _, by := range []string{"the router", "a follower"} {
			t.Run(tt.name+", by "+by, func(t *testing.T) {
				state, dir, _ := stateDirs(t)
				holdLines(t, dir, date, inPool(asked("1", on(0, 9, 0)), tt.pool), inPool(asked("2", on(0, 10, 0)), tt.pool))
				writeFile(t, dir, "day-"+date+".json", []byte(ofVersion(summaryOf(date, 2, windowsRead{}, theTwo...), 6)+"\n"))
				asked := false
				firstPool := ledger.FirstPool(func() string { asked = true; return first })

				work := againBy(t, by, state, dir, firstPool).Accounts[0]
				if work.Pool != tt.want || work.Pools != nil || work.Models[0].Upstream != 2 {
					t.Errorf("summarised again, work's day is %+v, want its two requests in %q alone", work, tt.want)
				}
				if asked != tt.asked {
					t.Errorf("the first pool was asked for: %v, want %v: only of a day whose lines name a pool", asked, tt.asked)
				}
			})
		}
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

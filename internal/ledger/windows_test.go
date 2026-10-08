package ledger_test

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// reserving are the caps of the tests' days where work keeps a tenth of
// every window in reserve, its cap at 90%.
var reserving = ledger.Caps{Reserves: map[string]float64{"work": 0.1}, Shared: []string{"5h", "7d"}}

// workDay returns work's day of the summaries' day, of a request of its, its
// windows read as read gives them, summarised by caps as of asOf.
func workDay(t *testing.T, c ledger.Caps, asOf time.Time, read ...readings.Reading) ledger.AccountDay {
	t.Helper()
	s, err := ledger.Summarise(date, slices.Values([]ledger.Line{asked("1", on(0, 9, 0))}), readingsOf(read...), nil, c, asOf)
	if err != nil {
		t.Fatalf("Summarise() error = %v", err)
	}
	for _, a := range s.Accounts {
		if a.Account == "work" {
			return a
		}
	}
	t.Fatalf("the summary is %+v, want work's day in it", s)
	return ledger.AccountDay{}
}

// allowed and rejected are work's window with the given key read at at, at
// u of its use, resetting at resets, allowed, and rejected.
func allowed(at time.Time, key string, u float64, resets time.Time) readings.Reading {
	return workRead(at, key, u, resets, quota.StatusAllowed)
}

func rejected(at time.Time, key string, u float64, resets time.Time) readings.Reading {
	return workRead(at, key, u, resets, quota.StatusRejected)
}

// theDesignsDay is a day of work's session, as the design's summary gives
// one: it began the day at 30%, and rose to 35% by its reset at 07:10; to 58%
// by its reset at 12:10; to its limit by its reset at 17:10; and to 40%
// after it lapsed, starting again at 20:00.
var theDesignsDay = []readings.Reading{
	allowed(on(-1, 23, 0), "5h", 0.3, on(0, 7, 10)),
	allowed(on(0, 6, 0), "5h", 0.35, on(0, 7, 10)),
	allowed(on(0, 9, 0), "5h", 0.2, on(0, 12, 10)),
	allowed(on(0, 11, 0), "5h", 0.58, on(0, 12, 10)),
	allowed(on(0, 13, 0), "5h", 0.1, on(0, 17, 10)),
	rejected(on(0, 14, 37), "5h", 1, on(0, 17, 10)),
	allowed(on(0, 20, 0), "5h", 0.4, on(1, 1, 0)),
}

func TestASummaryHoldsTheIdsOfTheSessionsEachAccountCounts(t *testing.T) {
	of := func(request, account, session string) ledger.Line {
		l := asked(request, on(0, 9, 0))
		l.Account, l.Session = account, session
		return l
	}
	check := of("4", "work", "resuming")
	check.Kind = ledger.KindCheck
	moved := of("7", "side", "three")
	moved.From = "gone"
	lines := []ledger.Line{
		of("1", "work", "two"), of("2", "work", "one"), of("3", "work", "two"),
		// Claude Code's quota check as a session resumes, under an id never
		// used again, and a request that carries no session.
		check, of("5", "work", ""),
		// The session's requests on another account, as after a move, and a
		// session moved off an account no request of it was answered on.
		of("6", "side", "two"), moved,
	}

	got := summarised(t, noReadings, lines...)
	want := map[string][]string{"gone": {}, "side": {"three", "two"}, "work": {"one", "two"}}
	if len(got.Accounts) != len(want) {
		t.Fatalf("the summary is %+v, want the days of %d accounts", got, len(want))
	}
	for _, a := range got.Accounts {
		if ids := want[a.Account]; a.SessionIDs == nil || !slices.Equal(a.SessionIDs, ids) || a.Sessions != len(ids) {
			t.Errorf("%s's sessions are %d, %q; want %q, in order, each once, none but those its sessions count, as a list however few",
				a.Account, a.Sessions, a.SessionIDs, ids)
		}
	}
}

func TestASummaryHoldsHowFarEachWindowRose(t *testing.T) {
	tests := []struct {
		name     string
		readings []readings.Reading
		want     map[string]float64
	}{
		{
			name:     "from the use it began the day at, to its highest before each reset, and from nothing after it, the rises added",
			readings: theDesignsDay,
			want:     map[string]float64{"5h": 2.03},
		},
		{
			name: "from the use carried into the day, read before it",
			readings: []readings.Reading{allowed(on(-2, 12, 0), "7d", 0.41, on(3, 10, 0)), allowed(on(0, 10, 0), "7d", 0.5, on(3, 10, 0)),
				allowed(on(0, 12, 0), "7d", 0.45, on(3, 10, 0))},
			want: map[string]float64{"7d": 0.09},
		},
		{
			name:     "from nothing, where the window had reset by the day's start since it was read",
			readings: []readings.Reading{allowed(on(-2, 12, 0), "7d", 0.9, on(-1, 10, 0)), allowed(on(0, 10, 0), "7d", 0.05, on(6, 9, 0))},
			want:     map[string]float64{"7d": 0.05},
		},
		{
			name:     "from nothing, of a window first read that day that began within it",
			readings: []readings.Reading{allowed(on(0, 10, 0), "5h", 0.3, on(0, 14, 0))},
			want:     map[string]float64{"5h": 0.3},
		},
		{
			name:     "from the use first read, of a window first read that day that began before it",
			readings: []readings.Reading{allowed(on(0, 10, 0), "7d", 0.4, on(3, 10, 0)), allowed(on(0, 12, 0), "7d", 0.5, on(3, 10, 0))},
			want:     map[string]float64{"7d": 0.1},
		},
		{
			name: "from nothing after a reset made by hand",
			readings: []readings.Reading{allowed(on(0, 10, 0), "7d", 0.6, on(3, 10, 0)), allowed(on(0, 12, 0), "7d", 0.05, on(3, 10, 0)),
				allowed(on(0, 15, 0), "7d", 0.2, on(3, 10, 0))},
			want: map[string]float64{"7d": 0.2},
		},
		{
			name: "to its highest, across a dip short of a reset",
			readings: []readings.Reading{allowed(on(0, 10, 0), "7d", 0.4, on(3, 10, 0)), allowed(on(0, 11, 0), "7d", 0.5, on(3, 10, 0)),
				allowed(on(0, 12, 0), "7d", 0.45, on(3, 10, 0))},
			want: map[string]float64{"7d": 0.1},
		},
		{
			name:     "none of a window it began the day at, read no more",
			readings: []readings.Reading{allowed(on(-1, 12, 0), "7d", 0.41, on(3, 10, 0))},
			want:     map[string]float64{"7d": 0},
		},
		{
			name: "of a model's own window, as of one every model shares",
			readings: []readings.Reading{allowed(on(0, 10, 0), "7d_oi", 0.2, on(3, 10, 0)), allowed(on(0, 12, 0), "7d_oi", 0.5, on(3, 10, 0)),
				allowed(on(0, 12, 0), "7d", 0.4, on(3, 10, 0))},
			want: map[string]float64{"7d": 0, "7d_oi": 0.3},
		},
		{
			name:     "none of a window read only before the day, reset by its start",
			readings: []readings.Reading{allowed(on(-1, 12, 0), "5h", 0.6, on(-1, 16, 0)), allowed(on(0, 10, 0), "7d", 0.4, on(3, 10, 0))},
			want:     map[string]float64{"7d": 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workDay(t, caps, dayDone, tt.readings...).Rise; !maps.Equal(got, tt.want) {
				t.Errorf("work's windows rose %v, want %v", got, tt.want)
			}
		})
	}
}

func TestASummaryHoldsEachResetOfAWindowTheReadingsHistorySaw(t *testing.T) {
	reset := func(key string, at time.Time, before float64) ledger.Reset {
		return ledger.Reset{Window: key, At: at.UTC(), Before: before}
	}
	tests := []struct {
		name     string
		readings []readings.Reading
		// asOf is when the day is summarised, an hour after it ended where
		// it's zero.
		asOf time.Time
		want []ledger.Reset
	}{
		{
			name:     "at each reset time read, its use as last read before it",
			readings: theDesignsDay,
			want:     []ledger.Reset{reset("5h", on(0, 7, 10), 0.35), reset("5h", on(0, 12, 10), 0.58), reset("5h", on(0, 17, 10), 1)},
		},
		{
			name:     "at its reset time, nothing read after it that day",
			readings: []readings.Reading{allowed(on(0, 10, 0), "5h", 0.5, on(0, 11, 0))},
			want:     []ledger.Reset{reset("5h", on(0, 11, 0), 0.5)},
		},
		{
			name:     "of the week the day began in, at its reset that day",
			readings: []readings.Reading{allowed(on(-3, 12, 0), "7d", 0.8, on(0, 10, 0)), allowed(on(0, 12, 0), "7d", 0.05, on(7, 10, 0))},
			want:     []ledger.Reset{reset("7d", on(0, 10, 0), 0.8)},
		},
		{
			name:     "when the reading that showed it reset by hand came",
			readings: []readings.Reading{allowed(on(0, 10, 0), "7d", 0.6, on(3, 10, 0)), allowed(on(0, 12, 0), "7d", 0.05, on(3, 10, 0))},
			want:     []ledger.Reset{reset("7d", on(0, 12, 0), 0.6)},
		},
		{
			name:     "when the reading that showed a later reset came, before the reset read before it",
			readings: []readings.Reading{allowed(on(0, 10, 0), "7d", 0.5, on(3, 10, 0)), allowed(on(0, 12, 0), "7d", 0.02, on(6, 12, 0))},
			want:     []ledger.Reset{reset("7d", on(0, 12, 0), 0.5)},
		},
		{
			name:     "of a model's own window, as of one every model shares, in the order they came",
			readings: []readings.Reading{allowed(on(0, 9, 0), "5h", 0.3, on(0, 10, 30)), allowed(on(0, 10, 0), "7d_oi", 0.7, on(0, 10, 15))},
			want:     []ledger.Reset{reset("7d_oi", on(0, 10, 15), 0.7), reset("5h", on(0, 10, 30), 0.3)},
		},
		{
			name: "none of a reading past its own reset as it was read, as an answer that came late gives",
			readings: []readings.Reading{allowed(on(0, 10, 0), "5h", 0.5, on(0, 11, 0)), allowed(on(0, 11, 30), "5h", 0.6, on(0, 11, 0)),
				allowed(on(0, 21, 0), "5h", 0.1, on(1, 2, 0))},
			want: []ledger.Reset{reset("5h", on(0, 11, 0), 0.5)},
		},
		{
			name:     "none of a dip short of a tenth",
			readings: []readings.Reading{allowed(on(0, 10, 0), "7d", 0.6, on(3, 10, 0)), allowed(on(0, 12, 0), "7d", 0.51, on(3, 10, 0))},
		},
		{
			name:     "none of a reset before the day began",
			readings: []readings.Reading{allowed(on(-1, 18, 0), "5h", 0.9, on(-1, 23, 0)), allowed(on(0, 21, 0), "5h", 0.2, on(1, 1, 0))},
		},
		{
			name:     "none after the day's end",
			readings: []readings.Reading{allowed(on(0, 21, 0), "5h", 0.2, on(1, 1, 0))},
		},
		{
			name:     "none yet, of a day summarised as it goes, before its reset",
			readings: []readings.Reading{allowed(on(0, 10, 0), "5h", 0.5, on(0, 11, 0))},
			asOf:     on(0, 10, 30),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workDay(t, caps, cmp.Or(tt.asOf, dayDone), tt.readings...).Resets
			if got == nil || !slices.EqualFunc(got, tt.want, sameReset) {
				t.Errorf("work's resets are %+v, want %+v, as a list however few", got, tt.want)
			}
		})
	}
}

// sameReset reports whether a and b are the same reset, of the same use
// before it.
func sameReset(a, b ledger.Reset) bool {
	return a.Window == b.Window && a.At.Equal(b.At) && a.Before == b.Before
}

func TestASummaryHoldsTheMinutesAnAccountSpentAtItsCapAndAtALimit(t *testing.T) {
	noReserve := ledger.Caps{Shared: reserving.Shared}
	tests := []struct {
		name     string
		readings []readings.Reading
		caps     ledger.Caps
		// asOf is when the day is summarised, an hour after it ended where
		// it's zero.
		asOf           time.Time
		atCap, atLimit int
	}{
		{
			name:     "at a limit from the reading that put it there to its reset, to the nearest minute",
			readings: []readings.Reading{allowed(on(0, 14, 0), "5h", 0.5, on(0, 17, 10)), rejected(on(0, 14, 37).Add(12*time.Second), "5h", 1, on(0, 17, 10))},
			atLimit:  153,
		},
		{
			name:     "at a limit, a window used up though not refused",
			readings: []readings.Reading{allowed(on(0, 10, 0), "5h", 1, on(0, 12, 0))},
			atLimit:  120,
		},
		{
			name:     "at a limit until the reading that took it away",
			readings: []readings.Reading{rejected(on(0, 10, 0), "5h", 1, on(0, 15, 0)), allowed(on(0, 10, 30), "5h", 0.5, on(0, 15, 0))},
			atLimit:  30,
		},
		{
			name:     "at a limit from the day's start, as the reading carried into it read",
			readings: []readings.Reading{rejected(on(-1, 22, 0), "5h", 1, on(0, 2, 0))},
			atLimit:  120,
		},
		{
			name:     "at a limit until the day's end",
			readings: []readings.Reading{rejected(on(0, 22, 0), "7d", 1, on(2, 10, 0))},
			atLimit:  120,
		},
		{
			name:     "at its cap, short of its limit, from the reading that put it there to its reset",
			readings: []readings.Reading{allowed(on(0, 9, 0), "5h", 0.5, on(0, 13, 0)), allowed(on(0, 10, 0), "5h", 0.9, on(0, 13, 0)), allowed(on(0, 11, 0), "5h", 0.95, on(0, 13, 0))},
			atCap:    180,
		},
		{
			name: "at a limit, a minute one window holds it at its cap and another at a limit",
			readings: []readings.Reading{allowed(on(0, 10, 0), "5h", 0.92, on(0, 13, 0)), rejected(on(0, 11, 0), "7d", 1, on(3, 0, 0)),
				allowed(on(0, 14, 0), "5h", 0.95, on(0, 18, 0))},
			atCap: 60, atLimit: 780,
		},
		{
			name:     "never at its cap without a reserve",
			readings: []readings.Reading{allowed(on(0, 10, 0), "5h", 0.95, on(0, 13, 0))},
			caps:     noReserve,
		},
		{
			name:     "at neither, of a window a model has of its own",
			readings: []readings.Reading{rejected(on(0, 10, 0), "7d_oi", 1, on(3, 0, 0)), allowed(on(0, 10, 0), "7d_oi", 0.95, on(3, 0, 0))},
		},
		{
			name:     "until now, of a day summarised as it goes",
			readings: []readings.Reading{rejected(on(0, 10, 0), "5h", 1, on(0, 15, 0))},
			asOf:     on(0, 12, 0),
			atLimit:  120,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := reserving
			if tt.caps.Shared != nil {
				c = tt.caps
			}
			got := workDay(t, c, cmp.Or(tt.asOf, dayDone), tt.readings...)
			if got.MinutesAtCap == nil || got.MinutesAtLimit == nil || *got.MinutesAtCap != tt.atCap || *got.MinutesAtLimit != tt.atLimit {
				t.Errorf("work's minutes at its cap and at a limit are %v and %v, want %d and %d", minutes(got.MinutesAtCap), minutes(got.MinutesAtLimit), tt.atCap, tt.atLimit)
			}
		})
	}
}

// minutes is n, or "never read" where it's nil.
func minutes(n *int) any {
	if n == nil {
		return "never read"
	}
	return *n
}

func TestTheRouterAndAReaderSummariseADayByTheCapsTheyreGiven(t *testing.T) {
	// Work at its cap from 10:00 to its session's reset at 13:00.
	read := allowed(on(0, 10, 0), "5h", 0.92, on(0, 13, 0))
	for _, by := range []string{"the router", "a reader"} {
		t.Run("by "+by, func(t *testing.T) {
			state, dir, history := stateDirs(t)
			holdLines(t, dir, date, asked("1", on(0, 9, 0)))
			var got ledger.Summary
			if by == "the router" {
				l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return dayDone }, readingsOf(read), reserving, logs.For("router"))
				l.SummariseEnded(dayDone)
				data := heldSummary(t, dir, date)
				if err := json.Unmarshal([]byte(data), &got); err != nil {
					t.Fatalf("the day's summary is %q (%v), want one", data, err)
				}
			} else {
				writeFile(t, history, "readings-"+date+".jsonl", []byte(readingJSON(t, read)+"\n"))
				got = ledger.NewReader(state, func() time.Time { return dayDone }, reserving, logs.For("cli")).Days(on(0, 0, 0))[0]
			}
			if len(got.Accounts) != 1 || got.Accounts[0].MinutesAtCap == nil || *got.Accounts[0].MinutesAtCap != 180 {
				t.Errorf("the day's summary is %+v, want work at its cap for 180 minutes, by its reserve as given", got)
			}
		})
	}
}

func TestCapsAreTheReservesOfTheAccountsConfigured(t *testing.T) {
	accounts := []config.Account{{ID: "work", Reserve: 0.1}, {ID: "personal"}}
	got := ledger.CapsOf(accounts, []string{"5h", "7d"})
	if want := map[string]float64{"work": 0.1, "personal": 0}; !maps.Equal(got.Reserves, want) || !slices.Equal(got.Shared, []string{"5h", "7d"}) {
		t.Errorf("CapsOf() = %+v, want the reserves %v, and the windows every model shares as given", got, want)
	}
}

func TestADaySummarisedAgainKnowsNoLessOfItsSessionsAndWindows(t *testing.T) {
	limitAt, resets := on(0, 11, 30), on(0, 13, 0)
	// The summary it replaces: of a session since lost with its lines, and
	// windows read higher, and longer, by readings since pruned.
	held := strings.Replace(summaryOf(date, 2, 2, windowsRead{highest: `{"5h":0.95}`, rise: `{"5h":0.9,"7d_oi":0.2}`,
		resets: `[` + resetJSON("7d", on(0, 8, 0), 0.5) + `,` + resetJSON("5h", resets, 0.95) + `]`, atCap: 30, atLimit: 60}),
		`"session_ids":["one"]`, `"session_ids":["held"]`, 1)
	// What the readings give now: the session rose to 30%, and was refused
	// from 11:30 to its reset.
	now := []readings.Reading{allowed(on(0, 10, 0), "5h", 0.3, resets), rejected(limitAt, "5h", 0.3, resets)}
	limit := `[{"window":"5h","at":"` + limitAt.UTC().Format(time.RFC3339) + `","resets_at":"` + resets.UTC().Format(time.RFC3339) + `"}]`
	want := strings.Replace(summaryOf(date, 3, 3, windowsRead{highest: `{"5h":0.95}`, rise: `{"5h":0.9,"7d_oi":0.2}`,
		resets: `[` + resetJSON("7d", on(0, 8, 0), 0.5) + `,` + resetJSON("5h", resets, 0.95) + `]`, limits: limit, atCap: 30, atLimit: 90}),
		`"sessions":1,"session_ids":["one"]`, `"sessions":2,"session_ids":["held","one"]`, 1)
	for _, by := range []string{"the router", "a reader"} {
		t.Run("by "+by, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			writeFile(t, dir, "day-"+date+".json", []byte(held+"\n"))
			holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)), asked("3", on(0, 23, 59)))

			if got := summaryAgain(t, by, state, dir, on(1, 11, 0), now); got != want {
				t.Errorf("the day's summary is\n%s\nwant its session ids and the one before's together; each rise, and each minute, the one "+
					"before's where that's more; and the resets the one before held among its own, before the higher use\n%s", got, want)
			}
		})
	}
}

func TestAnAccountKeptFromASummaryOfVersionOneIsKeptAsItWasNeverRead(t *testing.T) {
	// The side account's day, as a summary of version 1 gave it: its line
	// lost since, as to a damaged file.
	side := `{"account":"side","models":[{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,` +
		`"usage":{"input_tokens":10,"output_tokens":20}}],"sessions":1,"moved_on":0,"moved_off":0,"highest":{"5h":0.4}}`
	work := strings.TrimSuffix(strings.TrimPrefix(versionOne(date, 2, 2, ""), `{"version":1,"day":"2026-10-05","lines":2,"accounts":[`), "]}")
	held := `{"version":1,"day":"2026-10-05","lines":3,"accounts":[` + side + `,` + work + `]}`
	want := `{"version":2,"day":"2026-10-05","lines":4,"accounts":[` + side + `,` + workDayJSON(3, windowsRead{}) + `]}`
	for _, by := range []string{"the router", "a reader"} {
		t.Run("by "+by, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			writeFile(t, dir, "day-"+date+".json", []byte(held+"\n"))
			holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)), asked("3", on(0, 23, 59)))

			if got := summaryAgain(t, by, state, dir, on(1, 11, 0), nil); got != want {
				t.Errorf("the day's summary is\n%s\nwant one of version 2, side's day kept as version 1 gave it, what it never read absent\n%s", got, want)
			}
		})
	}
}

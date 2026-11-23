package router

import (
	"iter"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// workReserving are work, keeping a tenth of every window, and side, keeping
// none, with their tokens.
func workReserving() accounts {
	return resolve(config.Accounts{{ID: "work", Label: "Work", Reserve: 0.1}, {ID: "side", Label: "Side"}}, testTokens.Read, unrecorded)
}

// pinTo is the pin of the one pool there is, before pools, naming the
// accounts given; none where none are.
func pinTo(ids ...string) pins {
	if len(ids) == 0 {
		return nil
	}
	return pins{"": status.Pin{Accounts: ids, Since: start}}
}

// readingsHeard has s's readings history queue every line it's handed, as
// queueing does, unless dropping reports true, and returns what gives the
// lines it has queued since it was last asked.
func readingsHeard(s *state, dropping func() bool) (heard func() []readings.Reading) {
	var read []readings.Reading
	queue := queueing(&read)
	s.history = func(r []readings.Reading) bool { return !dropping() && queue(r) }
	return func() []readings.Reading {
		got := read
		read = nil
		return got
	}
}

// queuesAll drops no line.
func queuesAll() bool { return false }

func TestAReadingRecordsTheReserveTheRouterHoldsItsAccountTo(t *testing.T) {
	tests := []struct {
		name string
		pin  pins
		want float64
	}{
		{name: "its own", want: 0.1},
		{name: "none, where the pool's pin names it and spends it", pin: pinTo("work")},
		{name: "its own, where the pool's pin names another", pin: pinTo("side"), want: 0.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(workReserving(), unpooled(), testPolicy, claude.Provider{}.Family, at(start), unkept, unkept)
			s.repin(tt.pin)
			heard := readingsHeard(s, queuesAll)

			s.record("work", []quota.Window{session, week}, s.mark())
			s.recordProbe("work", probed(nil, fableWeek), nil, s.mark(), readings.FromProbe)

			for _, r := range heard() {
				if r.Reserve == nil || *r.Reserve != tt.want {
					t.Errorf("work's %s read off %s is recorded under %v, want %v", r.Key, r.Source, reserveRecorded(r.Reserve), tt.want)
				}
			}
		})
	}
}

func TestAPinThatSpendsAnAccountsReserveRestatesItsFreshWindows(t *testing.T) {
	ending := session
	ending.ResetsAt = start.Add(5 * time.Minute)
	tests := []struct {
		name          string
		before, after pins
		// held are work's windows, read at start; session and week where
		// it's nil.
		held []quota.Window
		at   time.Time
		// want are work's windows restated, under the reserve given.
		want    []quota.Window
		reserve float64
	}{
		{name: "pinned, as the pin spends it", after: pinTo("work"), at: start, want: []quota.Window{session, week}},
		{name: "pinned no longer", before: pinTo("work"), at: start, want: []quota.Window{session, week}, reserve: 0.1},
		{name: "pinned among others still", before: pinTo("work"), after: pinTo("work", "side"), at: start},
		{name: "the pin naming another, which keeps none", after: pinTo("side"), at: start},
		{name: "pinned, its session reset by then left out", held: []quota.Window{ending, week}, after: pinTo("work"), at: start.Add(10 * time.Minute),
			want: []quota.Window{week}},
		{name: "pinned, read too long before to stand as read now", after: pinTo("work"), at: start.Add(staleAfter + time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			var changes changeCount
			s := newState(workReserving(), unpooled(), testPolicy, claude.Provider{}.Family, clock.read, changes.hear, unkept)
			heard := readingsHeard(s, queuesAll)
			held := tt.held
			if held == nil {
				held = []quota.Window{session, week}
			}
			s.record("work", held, s.mark())
			s.record("side", []quota.Window{session, week}, s.mark())
			s.repin(tt.before)
			heard()
			changes = 0
			clock.now = tt.at

			s.repin(tt.after)

			var want []readings.Reading
			if tt.want != nil {
				want = readings.Under(readings.Of("work", tt.want, tt.at, readings.FromReserve), tt.reserve)
			}
			if got := heard(); !reflect.DeepEqual(got, want) {
				t.Errorf("the history heard of %+v, want %+v: work's windows still running, as it holds them, read lately, where the reserve it's held "+
					"to changed", got, want)
			}
			if wantChanges := changeCount(min(len(tt.want), 1)); changes != wantChanges {
				t.Errorf("told of %d changes, want %d: the reserve work's windows are recorded under, for the state file, where it changed", changes, wantChanges)
			}
		})
	}
}

// A window the pin's restatement missed, its line dropped or its reading too
// old to stand as read then, is recorded under the reserve in force at its
// next reading, though that reads as before.
func TestAWindowNotRestatedIsRecordedAtItsNextReading(t *testing.T) {
	tests := []struct {
		name     string
		dropping bool
		pinnedAt time.Time
	}{
		{name: "the restatement dropped, the history's queue full", dropping: true, pinnedAt: start},
		{name: "read too long before the pin to stand as read then", pinnedAt: start.Add(staleAfter + time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newState(workReserving(), unpooled(), testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
			dropping := false
			heard := readingsHeard(s, func() bool { return dropping })
			s.record("work", []quota.Window{session, week}, s.mark())
			heard()
			clock.now, dropping = tt.pinnedAt, tt.dropping

			s.repin(pinTo("work"))
			dropping = false
			if got := heard(); got != nil {
				t.Fatalf("the history queued %+v at the pin, want nothing", got)
			}
			s.record("work", []quota.Window{session, week}, s.mark())

			want := readings.Under(readings.Of("work", []quota.Window{session, week}, tt.pinnedAt, readings.FromAnswer), 0)
			if got := heard(); !reflect.DeepEqual(got, want) {
				t.Errorf("the history heard of %+v, want %+v: work's windows as read, under the reserve the pin left it", got, want)
			}
		})
	}
}

func TestTheRouterRestatesAsItStartsTheWindowsRecordedUnderAnotherReserve(t *testing.T) {
	lately := start.Add(-5 * time.Minute)
	now := map[string]float64{"5h": 0.1, "7d": 0.1}
	tests := []struct {
		name string
		// kept is what the state file kept work's windows as recorded
		// under, and readAt when it kept them as read.
		kept   map[string]float64
		readAt time.Time
		// want are work's windows restated, and recorded what the state
		// file keeps them as recorded under then.
		want     []quota.Window
		recorded map[string]float64
	}{
		{name: "changed", kept: map[string]float64{"5h": 0.2, "7d": 0.2}, readAt: lately, want: []quota.Window{session, week}, recorded: now},
		{name: "kept by a router from before, which recorded none", readAt: lately, want: []quota.Window{session, week}, recorded: now},
		{name: "as it was", kept: now, readAt: lately, recorded: now},
		{name: "one as it was", kept: map[string]float64{"5h": 0.1}, readAt: lately, want: []quota.Window{week}, recorded: now},
		{name: "read too long before the start to stand as read then", kept: map[string]float64{"7d": 0.2}, readAt: start.Add(-staleAfter - time.Minute),
			recorded: map[string]float64{"7d": 0.2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changes changeCount
			s := newState(workReserving(), unpooled(), testPolicy, claude.Provider{}.Family, at(start), changes.hear, unkept)
			readAt := map[string]time.Time{"5h": tt.readAt, "7d": tt.readAt}
			s.recall(savedUsage{Readings: map[string]savedReading{
				"work": {ReadAt: tt.readAt, Windows: []quota.Window{session, week}, WindowsReadAt: readAt, RecordedUnder: tt.kept},
				"side": {ReadAt: tt.readAt, Windows: []quota.Window{week}, WindowsReadAt: readAt, RecordedUnder: map[string]float64{"7d": 0}},
			}}, stillHeld)
			heard := readingsHeard(s, queuesAll)
			changes = 0

			s.restateNow()

			var want []readings.Reading
			if tt.want != nil {
				want = readings.Under(readings.Of("work", tt.want, start, readings.FromReserve), 0.1)
			}
			if got := heard(); !reflect.DeepEqual(got, want) {
				t.Errorf("the history heard of %+v, want %+v: work's windows read lately, under its reserve now, where it isn't the one they were "+
					"recorded under", got, want)
			}
			if kept, _ := s.usage["work"].saved(); !maps.Equal(kept.RecordedUnder, tt.recorded) {
				t.Errorf("the state file keeps work's windows as recorded under %v, want %v", kept.RecordedUnder, tt.recorded)
			}
			if wantChanges := changeCount(min(len(tt.want), 1)); changes != wantChanges {
				t.Errorf("told of %d changes, want %d: where a window was recorded afresh alone", changes, wantChanges)
			}
		})
	}
}

// A reserve changed while the router was down, its account's use as it was,
// ends the stretch the reserve before held as the router starts again, or,
// where the router's reading had grown stale by then, at the next reading:
// the day's summary counts work's time at its cap by the reserve in force
// through each part of it.
func TestAReserveChangedAcrossARestartSplitsTheDay(t *testing.T) {
	tests := []struct {
		name          string
		before, after float64
		// lately is set where work was read again ten minutes before the
		// restart, nothing changed; else its last reading is the hour's.
		lately bool
		// readAfter, where it's set, is how long after the restart work is
		// read again, nothing changed.
		readAfter time.Duration
		restated  bool
		atCap     int
	}{
		{name: "taken off, as the router starts", before: 0.1, lately: true, restated: true, atCap: 60},
		{name: "given, as the router starts", after: 0.1, lately: true, restated: true, atCap: 60},
		{name: "as it was", before: 0.1, after: 0.1, lately: true, atCap: 120},
		{name: "taken off, read too long before the start, at the next reading", before: 0.1, readAfter: 30 * time.Minute, atCap: 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			ten := time.Date(2026, 9, 28, 10, 0, 0, 0, time.Local)
			// Work's week at 93%, at its cap with a tenth in reserve.
			capped := quota.Window{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: ten.AddDate(0, 0, 3).UTC(), Status: quota.StatusAllowed}
			stateDir, clock := t.TempDir(), &setClock{now: ten}
			started := func(reserve float64) *Router {
				cfg := testConfig(clock.read, &stubProber{}, testTokens.Read)
				cfg.StateDir, cfg.Accounts = stateDir, config.Accounts{{ID: "work", Label: "Work", Reserve: reserve}}
				r := newTestRouterOf(t, cfg)
				r.file.load(filepath.Join(stateDir, stateFileName))
				r.openHistory()
				return r
			}
			first := started(tt.before)
			first.state.record("work", []quota.Window{capped}, first.state.mark())
			if tt.lately {
				clock.set(ten.Add(50 * time.Minute))
				first.state.record("work", []quota.Window{capped}, first.state.mark())
			}
			keeping(t, first.history)()
			first.file.save()
			clock.set(ten.Add(time.Hour))

			second := started(tt.after)
			if tt.readAfter > 0 {
				clock.set(ten.Add(time.Hour + tt.readAfter))
				second.state.record("work", []quota.Window{capped}, second.state.mark())
			}
			keeping(t, second.history)()

			history := func(from, to time.Time) iter.Seq[readings.Reading] {
				return readings.Between(second.history.files, from, to, nil)
			}
			read := slices.Collect(history(ten, ten.Add(2*time.Hour)))
			if got := slices.ContainsFunc(read, func(r readings.Reading) bool { return r.Source == readings.FromReserve }); got != tt.restated {
				t.Errorf("the history holds %+v; restated as the router started: %v, want %v", read, got, tt.restated)
			}
			caps := ledger.CapsOf(config.Accounts{{ID: "work", Reserve: tt.after}}, testPolicy.Shared)
			s, err := ledger.Summarise(dayfile.DateOf(ten), slices.Values([]ledger.Line(nil)), history, nil, caps, ten.Add(2*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Accounts) != 1 {
				t.Fatalf("the day's summary holds %+v, want work's day alone", s.Accounts)
			}
			if at := s.Accounts[0].MinutesAtCap; at == nil || *at != tt.atCap {
				t.Errorf("work spent %v minutes at its cap, want %d, by the reserve in force before the change and after it", minutesSaid(at), tt.atCap)
			}
		})
	}
}

// minutesSaid is n, or "never read" where it's nil.
func minutesSaid(n *int) any {
	if n == nil {
		return "never read"
	}
	return *n
}

// reserveRecorded is reserve, or "none recorded" where it's nil.
func reserveRecorded(reserve *float64) any {
	if reserve == nil {
		return "none recorded"
	}
	return *reserve
}

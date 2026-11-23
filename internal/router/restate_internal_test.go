package router

import (
	"iter"
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

// readingsHeard has s's readings history heard by what it returns, which
// gives what it has heard since it was last asked.
func readingsHeard(s *state) (heard func() []readings.Reading) {
	var read []readings.Reading
	s.history = func(r []readings.Reading) { read = append(read, r...) }
	return func() []readings.Reading {
		got := read
		read = nil
		return got
	}
}

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
			heard := readingsHeard(s)

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

func TestAPinThatSpendsAnAccountsReserveRestatesItsWindows(t *testing.T) {
	afterSession := session.ResetsAt.Add(time.Minute)
	tests := []struct {
		name          string
		before, after pins
		at            time.Time
		// want are work's windows restated, under the reserve given.
		want    []quota.Window
		reserve float64
	}{
		{name: "pinned, as the pin spends it", after: pinTo("work"), at: start, want: []quota.Window{session, week}},
		{name: "pinned no longer", before: pinTo("work"), at: start, want: []quota.Window{session, week}, reserve: 0.1},
		{name: "pinned among others still", before: pinTo("work"), after: pinTo("work", "side"), at: start},
		{name: "the pin naming another, which keeps none", after: pinTo("side"), at: start},
		{name: "pinned, its session reset by then left out", after: pinTo("work"), at: afterSession, want: []quota.Window{week}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			var changes changeCount
			s := newState(workReserving(), unpooled(), testPolicy, claude.Provider{}.Family, clock.read, changes.hear, unkept)
			s.record("work", []quota.Window{session, week}, s.mark())
			s.record("side", []quota.Window{session, week}, s.mark())
			s.repin(tt.before)
			heard := readingsHeard(s)
			changes = 0
			clock.now = tt.at

			s.repin(tt.after)

			var want []readings.Reading
			if tt.want != nil {
				want = readings.Under(readings.Of("work", tt.want, tt.at, readings.FromReserve), tt.reserve)
			}
			if got := heard(); !reflect.DeepEqual(got, want) {
				t.Errorf("the history heard of %+v, want %+v: work's windows still running, as it holds them, where the reserve it's held to changed", got, want)
			}
			if wantChanges := changeCount(min(len(tt.want), 1)); changes != wantChanges {
				t.Errorf("told of %d changes, want %d: the reserve work's windows are recorded under, for the state file, where it changed", changes, wantChanges)
			}
		})
	}
}

func TestTheRouterRestatesAsItStartsWhereTheReserveChangedSinceTheStateFileWasKept(t *testing.T) {
	tests := []struct {
		name     string
		kept     *float64
		restated bool
	}{
		{name: "changed", kept: new(0.2), restated: true},
		{name: "kept by a router from before, which recorded none", restated: true},
		{name: "as it was", kept: new(0.1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(workReserving(), unpooled(), testPolicy, claude.Provider{}.Family, at(start), unkept, unkept)
			s.recall(savedUsage{Readings: map[string]savedReading{
				"work": {ReadAt: start.Add(-time.Hour), Windows: []quota.Window{session, week}, RecordedUnder: tt.kept},
				"side": {ReadAt: start.Add(-time.Hour), Windows: []quota.Window{week}, RecordedUnder: new(0.0)},
			}}, stillHeld)
			heard := readingsHeard(s)

			s.restateNow()

			var want []readings.Reading
			if tt.restated {
				want = readings.Under(readings.Of("work", []quota.Window{session, week}, start, readings.FromReserve), 0.1)
			}
			if got := heard(); !reflect.DeepEqual(got, want) {
				t.Errorf("the history heard of %+v, want %+v: work's windows, under its reserve now, where it isn't the one they were recorded under", got, want)
			}
			if kept, _ := s.usage["work"].saved(); kept.RecordedUnder == nil || *kept.RecordedUnder != 0.1 {
				t.Errorf("the state file keeps work's windows as recorded under %v, want 0.1", reserveRecorded(kept.RecordedUnder))
			}
		})
	}
}

// A reserve changed while the router was down, its account's use as it was,
// ends the stretch the reserve before held as the router starts again: the
// day's summary counts work's time at its cap by the reserve in force
// through each part of it.
func TestAReserveChangedAcrossARestartSplitsTheDayWhereTheRouterStarted(t *testing.T) {
	tests := []struct {
		name          string
		before, after float64
		restated      bool
		atCap         int
	}{
		{name: "taken off", before: 0.1, restated: true, atCap: 60},
		{name: "given", after: 0.1, restated: true, atCap: 60},
		{name: "as it was", before: 0.1, after: 0.1, atCap: 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			ten := time.Date(2026, 9, 28, 10, 0, 0, 0, time.Local)
			// Work's week at 93%, at its cap with a tenth in reserve, read
			// at ten, and read as it was since.
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
			keeping(t, first.history)()
			first.file.save()
			clock.set(ten.Add(time.Hour))

			second := started(tt.after)
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
				t.Errorf("work spent %v minutes at its cap, want %d, by the reserve in force before the restart and after it", minutesSaid(at), tt.atCap)
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

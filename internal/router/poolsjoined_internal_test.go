package router

import (
	"cmp"
	"encoding/json"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// joinedInNoPool is the record of the pools testConfigured have been in, as
// the state file of a router of them in no pools keeps it: each in the one
// pool there is since an hour before start. A file that holds it, and was
// otherwise as the router keeps it, loads as it was.
const joinedInNoPool = `"pools_joined": {` +
	`"work": [{"pool": "", "since": "2026-09-28T12:12:00Z"}], ` +
	`"personal": [{"pool": "", "since": "2026-09-28T12:12:00Z"}], ` +
	`"side": [{"pool": "", "since": "2026-09-28T12:12:00Z"}]}`

// keptInNoPool is joinedInNoPool as savedUsage holds it.
func keptInNoPool(t *testing.T) map[string]poolsJoined {
	t.Helper()
	var saved savedUsage
	if err := json.Unmarshal([]byte("{"+joinedInNoPool+"}"), &saved); err != nil {
		t.Fatal(err)
	}
	return saved.PoolsJoined
}

// joins is the record of an account joining the pools given, by id, at
// the times given, in turn.
func joins(pools []string, since ...time.Time) poolsJoined {
	js := make(poolsJoined, len(pools))
	for i, pool := range pools {
		js[i] = poolJoined{Pool: pool, Since: since[i]}
	}
	return js
}

// joinedOf returns the record of the pools the account with the given id has
// been in, as r's state holds it.
func joinedOf(r *Router, id string) poolsJoined {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return r.state.joined[id]
}

// keepJoined writes a state file at path holding the record of the pools
// each account has been in given, and nothing else.
func keepJoined(t *testing.T, path string, joined map[string]poolsJoined) {
	t.Helper()
	putState(t, path, savedState{Version: stateVersion, PoolsJoined: joined})
}

// pairedIn is pairedAccounts with the account with the given id in the pool
// given.
func pairedIn(id, pool string) config.Accounts {
	accounts := pairedAccounts()
	for i := range accounts {
		if accounts[i].ID == id {
			accounts[i].Pool = pool
		}
	}
	return accounts
}

// weekReads is the account's week as the readings history holds it read at
// at, at u of its use, resetting as week does.
func weekReads(id string, at time.Time, u float64) readings.Reading {
	r := workReads(at, u, week.ResetsAt)
	r.Account = id
	return r
}

// seededOn builds a router of accounts in pools on now's time, as one
// restarted then, which loads the state file at path, holds each window as
// read last of the readings given, and takes up the history they make.
func seededOn(t *testing.T, now time.Time, accounts config.Accounts, pools config.Pools, path string, read ...readings.Reading) *Router {
	t.Helper()
	h := newHistory(config.History{}, at(now))
	h.open(t.TempDir())
	writeLines(t, h, read...)
	cfg := testConfig(at(now), &stubProber{}, pairedTokens.Read)
	cfg.Accounts, cfg.Pools = accounts, pools
	r := newTestRouterOf(t, cfg)
	r.file.load(path)
	for _, rd := range read {
		r.state.usage[rd.Account].windows[rd.Key] = rd.Window()
	}
	r.state.seed(h.readBack(now))
	return r
}

// risenIn returns the rises the day rate of the pool with the given id takes
// in at at, in Pro weeks, reporting false where it can't give a rate then.
func risenIn(r *Router, pool string, at time.Time) (float64, bool) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	rate, over, ok := r.state.days[pool].Rate(at)
	return rate * over.Hours(), ok
}

func TestTheRecordOfAnAccountsPoolsGivesThePoolItWasInAtAMoment(t *testing.T) {
	js := joins([]string{defaultPool, acmePool, defaultPool}, start, start.Add(time.Hour), start.Add(3*time.Hour))
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "before it joined any, the first", at: start.Add(-time.Hour), want: defaultPool},
		{name: "as it joined the first", at: start, want: defaultPool},
		{name: "as it joined the second", at: start.Add(time.Hour), want: acmePool},
		{name: "between the second and third", at: start.Add(2 * time.Hour), want: acmePool},
		{name: "after the last", at: start.Add(5 * time.Hour), want: defaultPool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := js.at(tt.at); !ok || got != tt.want {
				t.Errorf("at(%v) = %q, %v, want %q, true", tt.at, got, ok, tt.want)
			}
		})
	}
	if got, ok := poolsJoined(nil).at(start); ok {
		t.Errorf("no record gives %q at start, want none", got)
	}
}

func TestTheRecordOfAnAccountsPoolsKeepsItsLatestFour(t *testing.T) {
	day := 24 * time.Hour
	tests := []struct {
		name string
		// joined is how many pools the account joined, a day apart, the
		// first a year ago.
		joined int
		// want are the indexes of the entries kept.
		want []int
	}{
		{name: "none", joined: 0, want: nil},
		{name: "one, however old", joined: 1, want: []int{0}},
		{name: "four, however old", joined: 4, want: []int{0, 1, 2, 3}},
		{name: "five, the oldest goes", joined: 5, want: []int{1, 2, 3, 4}},
		{name: "seven, the oldest three go", joined: 7, want: []int{3, 4, 5, 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var js, want poolsJoined
			for i := range tt.joined {
				js = append(js, poolJoined{Pool: string(rune('a' + i)), Since: start.Add(-365*day + time.Duration(i)*day)})
			}
			for _, i := range tt.want {
				want = append(want, js[i])
			}
			if got := js.pruned(); !reflect.DeepEqual(got, want) {
				t.Errorf("pruned = %+v, want %+v", got, want)
			}
		})
	}
}

func TestTheRouterRecordsEachAccountsPoolAsItStartsAndAsItMoves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	clock := &testClock{now: start}
	r, _ := newPairedRouterOf(t, clock.read, pairedAccounts())
	r.file.load(path)
	if !r.file.changes.unsaved.Load() {
		t.Error("the first start left nothing to save, want each account's pool recorded")
	}
	if got, want := joinedOf(r, "side"), joins([]string{defaultPool}, start); !reflect.DeepEqual(got, want) {
		t.Errorf("as the router starts, side's pools are %+v, want %+v", got, want)
	}
	r.file.save()

	clock.now = start.Add(time.Hour)
	moveAccount(r, "side", acmePool)
	if got, want := joinedOf(r, "side"), joins([]string{defaultPool, acmePool}, start, start.Add(time.Hour)); !reflect.DeepEqual(got, want) {
		t.Errorf("once side moved, its pools are %+v, want %+v", got, want)
	}
	if got, want := joinedOf(r, "work"), joins([]string{defaultPool}, start); !reflect.DeepEqual(got, want) {
		t.Errorf("once side moved, work's pools are %+v, want %+v: work stayed", got, want)
	}
	if !r.file.changes.unsaved.Load() {
		t.Error("side's move left nothing to save, want it recorded")
	}

	r.file.save()
	want := map[string]poolsJoined{}
	for _, id := range []string{"work", "personal", "side", "extra"} {
		want[id] = joinedOf(r, id)
	}
	if got := readState(t, path).PoolsJoined; !reflect.DeepEqual(got, want) {
		t.Errorf("the state file keeps the accounts' pools as %+v, want %+v", got, want)
	}
}

func TestAClosedTableRecordsNoPool(t *testing.T) {
	r, _ := newPairedRouterOf(t, at(start), pairedAccounts())
	r.state.mu.Lock()
	r.state.grouped = closedTable("the pools can't be told")
	changed := r.state.joinPools(start)
	r.state.mu.Unlock()
	if changed || len(joinedOf(r, "work")) > 0 {
		t.Errorf("a closed table recorded %+v, changed %v, want nothing", joinedOf(r, "work"), changed)
	}
}

func TestAConfigWithoutPoolsRecordsTheOnePoolThereIs(t *testing.T) {
	f := newTestFile(at(start), testAccounts())
	f.load(filepath.Join(t.TempDir(), "state.json"))
	if got, want := f.state.joined["work"], joins([]string{""}, start); !reflect.DeepEqual(got, want) {
		t.Errorf("work's pools are %+v, want %+v", got, want)
	}
}

func TestLoadingTheRecordOfTheAccountsPools(t *testing.T) {
	now := start.Add(10 * 24 * time.Hour)
	tests := []struct {
		name      string
		kept      map[string]poolsJoined
		want      poolsJoined
		wantSaved bool
	}{
		{
			name: "pools as they stand load as they were",
			kept: map[string]poolsJoined{"work": joins([]string{defaultPool}, start)},
			want: joins([]string{defaultPool}, start),
		},
		{
			name: "a pool kept spelt in another case is the same pool",
			kept: map[string]poolsJoined{"work": joins([]string{"B2N8"}, start)},
			want: joins([]string{"B2N8"}, start),
		},
		{
			name:      "an account no longer configured goes",
			kept:      map[string]poolsJoined{"work": joins([]string{defaultPool}, start), "retired": joins([]string{defaultPool}, start)},
			want:      joins([]string{defaultPool}, start),
			wantSaved: true,
		},
		{
			name:      "of five pools kept, the oldest goes",
			kept:      map[string]poolsJoined{"work": joins([]string{acmePool, defaultPool, acmePool, "x4pz", defaultPool}, start, start.Add(time.Hour), start.Add(2*time.Hour), start.Add(3*time.Hour), start.Add(4*time.Hour))},
			want:      joins([]string{defaultPool, acmePool, "x4pz", defaultPool}, start.Add(time.Hour), start.Add(2*time.Hour), start.Add(3*time.Hour), start.Add(4*time.Hour)),
			wantSaved: true,
		},
		{
			name: "four pools kept, however old, stay",
			kept: map[string]poolsJoined{"work": joins([]string{acmePool, defaultPool, acmePool, defaultPool}, start, start.Add(time.Hour), start.Add(2*time.Hour), start.Add(3*time.Hour))},
			want: joins([]string{acmePool, defaultPool, acmePool, defaultPool}, start, start.Add(time.Hour), start.Add(2*time.Hour), start.Add(3*time.Hour)),
		},
		{
			name:      "pools kept out of order are taken in the order joined",
			kept:      map[string]poolsJoined{"work": {{Pool: defaultPool, Since: now.Add(-time.Hour)}, {Pool: acmePool, Since: now.Add(-2 * time.Hour)}}},
			want:      joins([]string{acmePool, defaultPool}, now.Add(-2*time.Hour), now.Add(-time.Hour)),
			wantSaved: true,
		},
		{
			name:      "an account kept in another pool than it's in now joins its pool as the router starts",
			kept:      map[string]poolsJoined{"work": joins([]string{acmePool}, start)},
			want:      joins([]string{acmePool, defaultPool}, start, now),
			wantSaved: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept := map[string]poolsJoined{"side": joins([]string{defaultPool}, start), "personal": joins([]string{acmePool}, start), "extra": joins([]string{acmePool}, start)}
			maps.Copy(kept, tt.kept)
			r, _ := newPairedRouterOf(t, at(now), pairedAccounts())
			r.state.mu.Lock()
			changed := r.state.recallJoined(kept, now)
			r.state.mu.Unlock()

			if got := joinedOf(r, "work"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("work's pools load as %+v, want %+v", got, tt.want)
			}
			if _, kept := r.state.joined["retired"]; kept {
				t.Error("an account no longer configured kept its pools, want them dropped")
			}
			if changed != tt.wantSaved {
				t.Errorf("the record taken in differs from the one kept = %v, want %v", changed, tt.wantSaved)
			}
		})
	}
}

func TestARestartAfterAMoveCountsEachReadingInThePoolItsAccountWasInThen(t *testing.T) {
	// Work and side, Default's, read their weeks; side rises four points in
	// the two hours before it moves to Acme, and one in the half hour after.
	// Five hours in, the router restarts.
	path := filepath.Join(t.TempDir(), "state.json")
	clock := &testClock{now: start}
	live, _ := newPairedRouterOf(t, clock.read, pairedAccounts())
	live.file.load(path)
	var heard []readings.Reading
	live.state.history = queueing(&heard)
	steps := []struct {
		after time.Duration
		do    func()
	}{
		{0, func() { readWeekAt(live, "work", 0.5); readWeekAt(live, "side", 0.4) }},
		{2 * time.Hour, func() { readWeekAt(live, "side", 0.44) }},
		{2*time.Hour + 30*time.Minute, func() { moveAccount(live, "side", acmePool) }},
		{3 * time.Hour, func() { readWeekAt(live, "side", 0.45) }},
	}
	for _, step := range steps {
		clock.now = start.Add(step.after)
		step.do()
	}
	clock.now = start.Add(5 * time.Hour)
	live.file.save()
	want := map[string]*status.Demand{
		defaultPool: {Day: 0.04 / 5, Hours: 5},
		acmePool:    {Day: 0.01 / 2.5, Hours: 2.5},
	}
	for pool, w := range want {
		if got := demandIn(live, pool); !sameDemand(got, w) {
			t.Fatalf("before the restart, pool %s's demand is %+v, want %+v", pool, got, w)
		}
	}

	restarted := seededOn(t, clock.now, pairedIn("side", acmePool), testPools, path, heard...)
	for pool, w := range want {
		if got := demandIn(restarted, pool); !sameDemand(got, w) {
			t.Errorf("after the restart, pool %s's demand is %+v, want %+v, as before it: side's rises in the pool it was in as each was read", pool, got, w)
		}
	}
}

// liveStep is what a live router does some time after it starts.
type liveStep struct {
	after time.Duration
	do    func(r *Router)
}

// reads has a router read the week of the account with the given id at u.
func reads(id string, u float64) func(r *Router) {
	return func(r *Router) { readWeekAt(r, id, u) }
}

// movesTo has a router's pools follow a config that puts the account with the
// given id in the pool given.
func movesTo(id, pool string) func(r *Router) {
	return func(r *Router) { moveAccount(r, id, pool) }
}

// followed returns the accounts r's pools last followed, as configured.
func followed(r *Router) config.Accounts {
	m := r.state.pools
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.accounts)
}

// fastRates returns the fast rate at now of each window of each account r
// has, by the account's id and the window's key, but those that are none.
func fastRates(r *Router, now time.Time) map[string]map[string]float64 {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	rates := make(map[string]map[string]float64)
	for id, u := range r.state.usage {
		for key := range u.fast {
			if rate := u.fastRate(key, now); rate > 0 {
				if rates[id] == nil {
					rates[id] = make(map[string]float64)
				}
				rates[id][key] = rate
			}
		}
	}
	return rates
}

// sameFastRates reports whether two accounts' fast rates, as fastRates gives
// them, match, to within rounding.
func sameFastRates(a, b map[string]map[string]float64) bool {
	return maps.EqualFunc(a, b, func(x, y map[string]float64) bool { return maps.EqualFunc(x, y, near) })
}

func TestARestartTakesUpTheRatesARouterRunningOnWouldHave(t *testing.T) {
	// As the router starts, work, Default's, and personal, Acme's, read their
	// weeks at 50%, giving their pools room, and side, Default's, at 40%;
	// side reads 44% forty minutes in, and moves, or reads again, as each
	// case says, till the router stops and starts again.
	tests := []struct {
		name  string
		steps []liveStep
		// down moves an account while the router is down, as the config it
		// starts again on has it, where it's set.
		down func(r *Router)
		end  time.Duration
	}{
		{
			name:  "a move, then a restart with no reading since",
			steps: []liveStep{{50 * time.Minute, movesTo("side", acmePool)}},
			end:   55 * time.Minute,
		},
		{
			name: "a move while the router was down",
			down: movesTo("side", acmePool),
			end:  55 * time.Minute,
		},
		{
			name: "a move away and back between two readings",
			steps: []liveStep{
				{45 * time.Minute, movesTo("side", acmePool)}, {50 * time.Minute, movesTo("side", defaultPool)}, {time.Hour, reads("side", 0.45)},
			},
			end: 70 * time.Minute,
		},
		{
			name:  "a move with readings on both sides",
			steps: []liveStep{{50 * time.Minute, movesTo("side", acmePool)}, {time.Hour, reads("side", 0.45)}},
			end:   70 * time.Minute,
		},
		{
			name:  "a reading at the moment of a move",
			steps: []liveStep{{50 * time.Minute, movesTo("side", acmePool)}, {50 * time.Minute, reads("side", 0.45)}},
			end:   70 * time.Minute,
		},
		{
			name:  "two accounts' moves, the earlier one's account read between them",
			steps: []liveStep{{30 * time.Minute, movesTo("side", acmePool)}, {50 * time.Minute, movesTo("extra", defaultPool)}},
			end:   55 * time.Minute,
		},
	}
	// Each case starts again this many times: the record's accounts are
	// walked in map order, so a start that took up their moves out of order
	// could pass by chance.
	const restarts = 20
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			clock := &testClock{now: start}
			live, _ := newPairedRouterOf(t, clock.read, pairedAccounts())
			live.file.load(path)
			var heard []readings.Reading
			live.state.history = queueing(&heard)
			steps := append([]liveStep{
				{0, reads("work", 0.5)}, {0, reads("personal", 0.5)}, {0, reads("side", 0.4)}, {40 * time.Minute, reads("side", 0.44)},
			}, tt.steps...)
			slices.SortStableFunc(steps, func(a, b liveStep) int { return cmp.Compare(a.after, b.after) })
			for _, step := range steps {
				clock.now = start.Add(step.after)
				step.do(live)
			}
			clock.now = start.Add(tt.end)
			live.file.save()
			if tt.down != nil {
				tt.down(live)
			}

			for range restarts {
				restarted := seededOn(t, clock.now, followed(live), testPools, path, heard...)
				if got, want := fastRates(restarted, clock.now), fastRates(live, clock.now); !sameFastRates(got, want) {
					t.Errorf("after the restart, the fast rates are %v, want %v, as the router running on has them", got, want)
				}
				for _, pool := range []string{defaultPool, acmePool} {
					if got, want := demandIn(restarted, pool), demandIn(live, pool); !sameDemand(got, want) {
						t.Errorf("after the restart, pool %s's demand is %+v, want %+v, as the router running on has it", pool, got, want)
					}
				}
				if t.Failed() {
					return
				}
			}
		})
	}
}

func TestARestartForgetsAnAccountsFastRatesWhereItMovedBetweenPools(t *testing.T) {
	// Side rises four points forty minutes in, and a point an hour in, in
	// Default as the config has it now; the record has it join Default as
	// each case says. Seventy minutes in, the router restarts.
	now := start.Add(70 * time.Minute)
	both := forecast.Fast{}.Rose(0.04, start.Add(40*time.Minute)).Rose(0.01, start.Add(time.Hour)).Rate(now)
	since := forecast.Fast{}.Rose(0.01, start.Add(time.Hour)).Rate(now)
	before, joined := start.Add(-time.Hour), start.Add(50*time.Minute)
	tests := []struct {
		name   string
		joined poolsJoined
		want   float64
	}{
		{name: "from another pool, its rise since alone", joined: joins([]string{acmePool, defaultPool}, before, joined), want: since},
		{name: "from a pool since removed, its rise since alone", joined: joins([]string{"x4pz", defaultPool}, before, joined), want: since},
		{name: "from the pool before pools, as the pools came in, both rises", joined: joins([]string{"", defaultPool}, before, joined), want: both},
		{
			name:   "from another pool by way of the pool before pools, both rises",
			joined: joins([]string{acmePool, "", defaultPool}, before, start.Add(45*time.Minute), joined), want: both,
		},
		{name: "from itself, spelt in another case, both rises", joined: joins([]string{"B2N8", defaultPool}, before, joined), want: both},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			keepJoined(t, path, map[string]poolsJoined{"side": tt.joined})
			r := seededOn(t, now, pairedAccounts(), testPools, path,
				weekReads("side", start, 0.4), weekReads("side", start.Add(40*time.Minute), 0.44), weekReads("side", start.Add(time.Hour), 0.45))
			if got := fastRates(r, now)["side"]["7d"]; !near(got, tt.want) {
				t.Errorf("side's week's fast rate is %v, want %v", got, tt.want)
			}
		})
	}
}

func TestARestartForgetsTheFastRateOfEachWindowOfAnAccountThatMoved(t *testing.T) {
	// Side's session and week each rise forty minutes in. The router
	// restarts fifty-five minutes in, side in Default.
	now := start.Add(55 * time.Minute)
	sessionReads := func(at time.Time, u float64) readings.Reading {
		return readings.Reading{At: at, Account: "side", Key: "5h", Utilization: u, ResetsAt: session.ResetsAt, Source: readings.FromProbe}
	}
	tests := []struct {
		name   string
		joined poolsJoined
		// want are the windows with a fast rate.
		want []string
	}{
		{name: "moved from Acme fifty minutes in, none", joined: joins([]string{acmePool, defaultPool}, start.Add(-time.Hour), start.Add(50*time.Minute))},
		{name: "in Default all along, both", joined: joins([]string{defaultPool}, start.Add(-time.Hour)), want: []string{"5h", "7d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			keepJoined(t, path, map[string]poolsJoined{"side": tt.joined})
			r := seededOn(t, now, pairedAccounts(), testPools, path,
				weekReads("side", start, 0.4), sessionReads(start, 0.1),
				weekReads("side", start.Add(40*time.Minute), 0.44), sessionReads(start.Add(40*time.Minute), 0.2))
			if got := slices.Sorted(maps.Keys(fastRates(r, now)["side"])); !slices.Equal(got, tt.want) {
				t.Errorf("side's windows with a fast rate are %v, want %v", got, tt.want)
			}
		})
	}
}

func TestARestartOnAConfigChangedWhileTheRouterWasDownCountsTheHistoryInThePoolKept(t *testing.T) {
	// The state file keeps side in Default; the config now puts it in Acme.
	// Side rose four points in the three hours before the restart.
	path := filepath.Join(t.TempDir(), "state.json")
	now := start.Add(3 * time.Hour)
	keepJoined(t, path, map[string]poolsJoined{
		"work": joins([]string{defaultPool}, start.Add(-time.Hour)), "side": joins([]string{defaultPool}, start.Add(-time.Hour)),
	})

	r := seededOn(t, now, pairedIn("side", acmePool), testPools, path,
		weekReads("work", start, 0.5), weekReads("side", start, 0.4), weekReads("side", start.Add(2*time.Hour), 0.44))
	if got, want := demandIn(r, defaultPool), (&status.Demand{Day: 0.04 / 3, Hours: 3}); !sameDemand(got, want) {
		t.Errorf("Default's demand is %+v, want %+v: side was Default's as each reading was read", got, want)
	}
	if risen, ok := risenIn(r, acmePool, now.Add(2*time.Hour)); !ok || risen != 0 {
		t.Errorf("Acme's day rate, two hours on, takes in %v (%v), want none: side joined it only as the router started", risen, ok)
	}
	if got, want := joinedOf(r, "side"), joins([]string{defaultPool, acmePool}, start.Add(-time.Hour), now); !reflect.DeepEqual(got, want) {
		t.Errorf("side's pools are %+v, want %+v: it joined Acme as the router started", got, want)
	}
}

func TestAFirstStartCountsTheHistoryInEachAccountsPoolNow(t *testing.T) {
	// No state file: side, in Acme now, rose four points in three hours.
	now := start.Add(3 * time.Hour)
	r := seededOn(t, now, pairedIn("side", acmePool), testPools, filepath.Join(t.TempDir(), "state.json"),
		weekReads("work", start, 0.5), weekReads("side", start, 0.4), weekReads("side", start.Add(2*time.Hour), 0.44))
	if got, want := demandIn(r, acmePool), (&status.Demand{Day: 0.04 / 3, Hours: 3}); !sameDemand(got, want) {
		t.Errorf("Acme's demand is %+v, want %+v: side's history in the pool it's in now", got, want)
	}
	if got, want := demandIn(r, defaultPool), (&status.Demand{Day: 0, Hours: 3}); !sameDemand(got, want) {
		t.Errorf("Default's demand is %+v, want %+v: none of side's history", got, want)
	}
}

func TestARestartCountsEachReadingInThePoolTheRecordPlacesItsAccountIn(t *testing.T) {
	// Work, Default's, and personal, Acme's, give their pools room from the
	// start; side rose four points two hours in. The router restarts three
	// hours in, side in the pool the record has it join then, the config's
	// pools as each case says.
	now := start.Add(3 * time.Hour)
	withSpare := append(slices.Clone(testPools), config.Pool{ID: "x4pz", Label: "Spare"})
	tests := []struct {
		name   string
		pools  config.Pools
		joined poolsJoined
		// want is the pool whose day rate takes in side's rise, "" for none.
		want string
	}{
		{
			name:  "one before its first entry, in its first entry's pool",
			pools: testPools, joined: joins([]string{acmePool, defaultPool}, start.Add(150*time.Minute), now), want: acmePool,
		},
		{
			name:  "one in a pool still configured, in it",
			pools: withSpare, joined: joins([]string{"x4pz", defaultPool}, start.Add(-time.Hour), now), want: "x4pz",
		},
		{
			name:  "one in a pool since removed, in none",
			pools: testPools, joined: joins([]string{"x4pz", defaultPool}, start.Add(-time.Hour), now),
		},
		{
			name:  "one in the pool before pools, in the first",
			pools: testPools, joined: joins([]string{"", acmePool}, start.Add(-time.Hour), now), want: defaultPool,
		},
		{
			name:  "one in a pool spelt in another case, in the pool as the config spells it",
			pools: testPools, joined: joins([]string{"Q7MT", defaultPool}, start.Add(-time.Hour), now), want: acmePool,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			keepJoined(t, path, map[string]poolsJoined{"side": tt.joined})
			r := seededOn(t, now, pairedIn("side", tt.joined[len(tt.joined)-1].Pool), tt.pools, path,
				weekReads("work", start, 0.5), weekReads("personal", start, 0.5),
				weekReads("side", start, 0.4), weekReads("side", start.Add(2*time.Hour), 0.44))
			for _, pool := range r.state.grouped.each() {
				want := 0.0
				if pool == tt.want {
					want = 0.04
				}
				if risen, ok := risenIn(r, pool, now); !ok || !near(risen, want) {
					t.Errorf("pool %s's day rate takes in %v (%v), want %v", pool, risen, ok, want)
				}
			}
			for pool := range r.state.days {
				if !slices.Contains(r.state.grouped.each(), pool) {
					t.Errorf("seed made a day rate of pool %q, which isn't configured as spelt", pool)
				}
			}
		})
	}
}

func TestARestartJudgesEachPoolsRoomByTheAccountsInItThen(t *testing.T) {
	// Work, Default's, is at its reserve. Side, which has room, was
	// Default's till it moved to Acme three hours in; extra, which has room
	// too, joined Spare, a pool made five hours in, from a pool since
	// removed. Personal is never read. Work, side and extra read their weeks
	// at the start and as the router restarts, seven hours in.
	accounts := pairedIn("extra", "x4pz")
	accounts[0].Reserve = 0.1
	accounts[2].Pool = acmePool
	pools := append(slices.Clone(testPools), config.Pool{ID: "x4pz", Label: "Spare"})
	now := start.Add(7 * time.Hour)
	path := filepath.Join(t.TempDir(), "state.json")
	keepJoined(t, path, map[string]poolsJoined{
		"work":  joins([]string{defaultPool}, start.Add(-time.Hour)),
		"side":  joins([]string{defaultPool, acmePool}, start.Add(-time.Hour), start.Add(3*time.Hour)),
		"extra": joins([]string{"gone", "x4pz"}, start.Add(-time.Hour), start.Add(5*time.Hour)),
	})
	var read []readings.Reading
	for id, u := range map[string]float64{"work": 0.95, "side": 0.4, "extra": 0.2} {
		read = append(read, weekReads(id, start, u), weekReads(id, now, u))
	}
	slices.SortFunc(read, func(a, b readings.Reading) int { return a.At.Compare(b.At) })
	r := seededOn(t, now, accounts, pools, path, read...)

	// Default had room the three hours side was in it; Acme the four since
	// side joined it; Spare the two since extra did.
	for pool, want := range map[string]float64{defaultPool: 3, acmePool: 4, "x4pz": 2} {
		if got := demandIn(r, pool); got == nil || !near(got.Hours, want) {
			t.Errorf("pool %s's demand is %+v, want it over %v hours of room", pool, got, want)
		}
	}
}

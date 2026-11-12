package router

import (
	"maps"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// withPlans is pairedAccounts on the plans given, by id: none for the rest.
func withPlans(plans map[string]string) config.Accounts {
	accounts := pairedAccounts()
	for i, a := range accounts {
		accounts[i].Plan = plans[a.ID]
	}
	return accounts
}

// readWeekAt has r read the week of the account with the given id at
// utilization, resetting as week does, as the clock reads now.
func readWeekAt(r *Router, id string, utilization float64) {
	r.state.record(id, []quota.Window{weekUsed(utilization)}, r.state.mark())
}

// demandIn returns the demand of r's document of the pool with the given id.
func demandIn(r *Router, pool string) *status.Demand {
	return r.statusOf(r.readOf(pool)).Demand
}

func TestEachPoolsDemandIsItsOwn(t *testing.T) {
	// Work, on Max, reads its week at 40% and 42% ten minutes on; personal,
	// on Pro, at 10% and 13%.
	clock := &testClock{now: start}
	r, _ := newPairedRouterOf(t, clock.read, withPlans(map[string]string{"work": config.PlanMax, "personal": config.PlanPro}))
	readWeekAt(r, "work", 0.4)
	readWeekAt(r, "personal", 0.1)
	clock.now = start.Add(10 * time.Minute)
	readWeekAt(r, "work", 0.42)
	readWeekAt(r, "personal", 0.13)

	// Too few hours with room for a day rate: each pool's fast rate.
	if got, want := demandIn(r, defaultPool), (&status.Demand{Day: 0.02 * 5}); !sameDemand(got, want) {
		t.Errorf("Default's demand is %+v, want %+v: work's rise alone", got, want)
	}
	if got, want := demandIn(r, acmePool), (&status.Demand{Day: 0.03}); !sameDemand(got, want) {
		t.Errorf("Acme's demand is %+v, want %+v: personal's rise alone", got, want)
	}
	clock.now = start.Add(2 * time.Hour)
	if got, want := demandIn(r, defaultPool), (&status.Demand{Day: 0.02 * 5 / 2, Hours: 2}); !sameDemand(got, want) {
		t.Errorf("two hours on, Default's demand is %+v, want %+v", got, want)
	}
	if got, want := demandIn(r, acmePool), (&status.Demand{Day: 0.03 / 2, Hours: 2}); !sameDemand(got, want) {
		t.Errorf("two hours on, Acme's demand is %+v, want %+v", got, want)
	}
}

func TestAnAccountWhosePlanIsntKnownIsSizedAmongItsPoolsPlans(t *testing.T) {
	// Work's plan isn't known: of every account's, Pro is most, but in
	// Default, side's Ultra is alone. Work's week rises two points in five
	// minutes.
	clock := &testClock{now: start}
	plans := map[string]string{"side": config.PlanUltra, "personal": config.PlanPro, "extra": config.PlanPro}
	r, _ := newPairedRouterOf(t, clock.read, withPlans(plans))
	readWeekAt(r, "work", 0.4)
	clock.now = start.Add(5 * time.Minute)
	readWeekAt(r, "work", 0.42)

	if got, want := demandIn(r, defaultPool), (&status.Demand{Day: 0.02 * 10}); !sameDemand(got, want) {
		t.Errorf("Default's demand is %+v, want %+v: work sized as Ultra, as its pool's accounts are", got, want)
	}
}

func TestAnAccountMovingPoolsBringsNoneOfItsOldPoolsDemand(t *testing.T) {
	// Work and side, in Default, read their weeks; side's rises two points
	// in ten minutes. Side moves to Acme fifteen minutes in.
	clock := &testClock{now: start}
	r, _ := newPairedRouterOf(t, clock.read, pairedAccounts())
	readWeekAt(r, "work", 0.5)
	readWeekAt(r, "side", 0.4)
	clock.now = start.Add(10 * time.Minute)
	readWeekAt(r, "side", 0.42)
	clock.now = start.Add(15 * time.Minute)
	moveAccount(r, "side", acmePool)

	doc := r.statusOf(r.readOf(acmePool))
	if side, _ := doc.Account("side"); fastOf(side, "7d") != 0 {
		t.Errorf("in Acme, side's week's fast rate is %v, want none: it was Default's use", fastOf(side, "7d"))
	}
	if got, want := doc.Demand, (&status.Demand{}); !sameDemand(got, want) {
		t.Errorf("Acme's demand is %+v, want %+v: none of side's use in Default", got, want)
	}

	// Side's next rise is Acme's alone.
	clock.now = start.Add(20 * time.Minute)
	readWeekAt(r, "side", 0.45)
	if side, _ := r.statusOf(r.readOf(acmePool)).Account("side"); !near(fastOf(side, "7d"), 0.03) {
		t.Errorf("in Acme, side's week's fast rate is %v, want 0.03: its rise since it moved alone", fastOf(side, "7d"))
	}

	// Default's day rate keeps side's rise from before it moved, as Default's
	// use, till it's older than its day.
	clock.now = start.Add(2 * time.Hour)
	if got, want := demandIn(r, defaultPool), (&status.Demand{Day: 0.02 / 2, Hours: 2}); !sameDemand(got, want) {
		t.Errorf("two hours on, Default's demand is %+v, want %+v: side's rise while it was Default's", got, want)
	}
}

func TestEachPoolsRoomIsJudgedByItsOwnAccountsAndPin(t *testing.T) {
	// Work and side, Default's, keep a tenth back, and their weeks are at
	// their reserves; Acme's personal has room.
	accounts := pairedAccounts()
	accounts[0].Reserve, accounts[2].Reserve = 0.1, 0.1
	tests := []struct {
		name string
		// pin pins the pool given to the account given, where it's set.
		pinPool, pinned string
		want            map[string]bool
	}{
		{name: "unpinned", want: map[string]bool{defaultPool: false, acmePool: true}},
		{name: "Default pinned to work, spending its reserve", pinPool: defaultPool, pinned: "work", want: map[string]bool{defaultPool: true, acmePool: true}},
		{name: "Acme pinned", pinPool: acmePool, pinned: "personal", want: map[string]bool{defaultPool: false, acmePool: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r, _ := newPairedRouterOf(t, at(start), accounts)
			readWeekAt(r, "work", 0.92)
			readWeekAt(r, "side", 0.95)
			readWeekAt(r, "personal", 0.5)
			if tt.pinned != "" {
				pinnedIn(t, r, tt.pinPool, status.Pin{Accounts: []string{tt.pinned}, Since: start})
			}
			for pool, want := range tt.want {
				if got := roomIn(r.state, pool, start); got != want {
					t.Errorf("pool %s has room = %v, want %v", pool, got, want)
				}
			}
		})
	}
}

func TestTheDayRateOfThePoolBeforePoolsIsTheFirstsAsThePoolsComeIn(t *testing.T) {
	logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	clock := &testClock{now: start}
	cfg := testConfig(clock.read, &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.ConfigFile = config.Accounts{{ID: "work", Label: "work"}, {ID: "personal", Label: "personal"}, {ID: "side", Label: "side"}}, Watch(path)
	r := newTestRouterOf(t, cfg)
	readWeekAt(r, "work", 0.4)
	clock.now = start.Add(10 * time.Minute)
	readWeekAt(r, "work", 0.42)
	clock.now = start.Add(2 * time.Hour)
	want := &status.Demand{Day: 0.02 / 2, Hours: 2}
	if got := demandIn(r, ""); !sameDemand(got, want) {
		t.Fatalf("before the pools, the demand is %+v, want %+v", got, want)
	}

	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()
	if got := demandIn(r, defaultPool); !sameDemand(got, want) {
		t.Errorf("Default's demand is %+v, want %+v: the pool before pools', as all work before pools was the first's", got, want)
	}
	if got := demandIn(r, acmePool); got != nil {
		t.Errorf("Acme's demand is %+v, want none: personal, its own, is unread", got)
	}
}

func TestARestartTakesUpEachPoolsDayRateFromItsAccountsHistory(t *testing.T) {
	// Work, Default's, rose two points over the day; personal, Acme's, four.
	now := start.Add(3 * time.Hour)
	h := newHistory(config.History{}, at(now))
	h.open(t.TempDir())
	reads := func(id string, at time.Time, u float64) readings.Reading {
		r := workReads(at, u, week.ResetsAt)
		r.Account = id
		return r
	}
	writeLines(t, h,
		reads("work", start, 0.4), reads("personal", start, 0.1),
		reads("work", start.Add(time.Hour), 0.42), reads("personal", start.Add(time.Hour), 0.14),
	)
	r, _ := newPairedRouterOf(t, at(now), pairedAccounts())
	r.state.usage["work"].windows["7d"] = weekUsed(0.42)
	r.state.usage["personal"].windows["7d"] = weekUsed(0.14)

	r.state.seed(h.readBack(now))
	if got, want := demandIn(r, defaultPool), (&status.Demand{Day: 0.02 / 3, Hours: 3}); !sameDemand(got, want) {
		t.Errorf("Default's demand is %+v, want %+v: work's history alone", got, want)
	}
	if got, want := demandIn(r, acmePool), (&status.Demand{Day: 0.04 / 3, Hours: 3}); !sameDemand(got, want) {
		t.Errorf("Acme's demand is %+v, want %+v: personal's history alone", got, want)
	}
}

// newPairedPrimingRouter builds a router on now's time of pairedAccounts in
// testPools, each with a token, priming through the day with prober.
func newPairedPrimingRouter(t *testing.T, now func() time.Time, prober Prober) *Router {
	t.Helper()
	cfg := testConfig(now, prober, pairedTokens.Read)
	cfg.Accounts, cfg.Pools, cfg.Prime = pairedAccounts(), testPools, daytime
	return newTestRouterOf(t, cfg)
}

// slotsIn returns when each account of the pool with the given id is primed
// as r's document of the pool gives it, by the account's id.
func slotsIn(t *testing.T, r *Router, pool string) map[string]string {
	t.Helper()
	slots := make(map[string]string)
	for _, s := range statusIn(t, r, pool).Prime.Slots {
		slots[s.Account] = s.At
	}
	return slots
}

func TestEachPoolStaggersItsOwnResetsThroughTheDay(t *testing.T) {
	log := logstest.Capture(t)
	r := newPairedPrimingRouter(t, at(onDay(1, 2, 0)), &stubProber{})

	// Two accounts a pool: each pool's resets fall as two accounts' do.
	want := map[string]map[string]string{
		defaultPool: {"work": "04:10", "side": "06:40"},
		acmePool:    {"personal": "04:10", "extra": "06:40"},
	}
	for pool, slots := range want {
		if got := slotsIn(t, r, pool); !maps.Equal(got, slots) {
			t.Errorf("pool %s primes at %v, want %v", pool, got, slots)
		}
	}

	// Side moves to Acme: Default's one account and Acme's three are each
	// staggered afresh, as the pools are taken up.
	moveAccount(r, "side", acmePool)
	want = map[string]map[string]string{
		defaultPool: {"work": "05:30"},
		acmePool:    {"personal": "03:50", "side": "05:30", "extra": "07:10"},
	}
	for pool, slots := range want {
		if got := slotsIn(t, r, pool); !maps.Equal(got, slots) {
			t.Errorf("side moved to Acme, pool %s primes at %v, want %v", pool, got, slots)
		}
	}
	if !log.Has("level=INFO", `msg="priming schedule worked out again"`) {
		t.Errorf("log reads\n%s\nwant the schedule worked out again as side moved", log)
	}
}

func TestEachPoolsAccountsArePrimedAtTheirPoolsSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 0, 0))
		upstream := newWindowsUpstream(clock)
		r := newPairedPrimingRouter(t, clock.read, upstream)
		stop := startPriming(r)
		defer stop()

		time.Sleep(8 * time.Hour)
		synctest.Wait()
		want := map[string][]time.Time{
			workToken:     {afterResets(onDay(1, 4, 10), 1)},
			personalToken: {afterResets(onDay(1, 4, 10), 1)},
			sideToken:     {afterResets(onDay(1, 6, 40), 1)},
			extraToken:    {afterResets(onDay(1, 6, 40), 1)},
		}
		if got := upstream.probes(); !reflect.DeepEqual(got, want) {
			t.Errorf("primed at\n%v\nwant each pool's accounts at its own pool's slots\n%v", got, want)
		}
	})
}

func TestEachPoolShowsItsOwnSlotsAsThePoolsComeIn(t *testing.T) {
	logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	cfg := testConfig(at(onDay(1, 2, 0)), &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.ConfigFile, cfg.Prime = config.Accounts{{ID: "work", Label: "work"}, {ID: "personal", Label: "personal"}, {ID: "side", Label: "side"}}, Watch(path), daytime
	r := newTestRouterOf(t, cfg)

	// The pools come in, before the primer's next look: each pool's document
	// shows its own accounts' slots alone, staggered on its own.
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()
	want := map[string]map[string]string{
		defaultPool: {"work": "04:10", "side": "06:40"},
		acmePool:    {"personal": "05:30"},
	}
	for pool, slots := range want {
		if got := slotsIn(t, r, pool); !maps.Equal(got, slots) {
			t.Errorf("once the pools came in, pool %s primes at %v, want %v", pool, got, slots)
		}
	}
}

func TestARestartJudgesEachPoolsRoomByItsOwnPinFromWhenItWasSet(t *testing.T) {
	// Work, Default's, and personal, Acme's, each keep a tenth back, and
	// nothing else is read. Each reads its week at 50%, and at its reserve an
	// hour on. Default is pinned to work two hours on, and Acme to personal
	// four; work reads again three hours on, between the pins, and personal
	// five, after both. Six hours on, the router restarts.
	path := filepath.Join(t.TempDir(), "state.json")
	reserving := pairedAccounts()
	reserving[0].Reserve, reserving[1].Reserve = 0.1, 0.1
	clock := &testClock{now: start}
	live, _ := newPairedRouterOf(t, clock.read, reserving)
	live.file.load(path)
	var heard []readings.Reading
	live.state.history = func(read []readings.Reading) { heard = append(heard, read...) }
	steps := []struct {
		after time.Duration
		do    func()
	}{
		{0, func() { readWeekAt(live, "work", 0.5); readWeekAt(live, "personal", 0.5) }},
		{time.Hour, func() { readWeekAt(live, "work", 0.92); readWeekAt(live, "personal", 0.92) }},
		{2 * time.Hour, func() { askedOK(t, live, http.MethodPost, "/pin", `{"accounts": ["work"]}`) }},
		{3 * time.Hour, func() { readWeekAt(live, "work", 0.94) }},
		{4 * time.Hour, func() { askedOK(t, live, http.MethodPost, "/pin", `{"accounts": ["personal"]}`) }},
		{5 * time.Hour, func() { readWeekAt(live, "personal", 0.94) }},
	}
	for _, step := range steps {
		clock.now = start.Add(step.after)
		step.do()
	}
	clock.now = start.Add(6 * time.Hour)
	live.file.save()
	// Default had room the hour before work reached its reserve, and the four
	// since its pin; Acme the hour before personal did, and the two since its
	// own.
	hours := map[string]float64{defaultPool: 5, acmePool: 3}
	want := make(map[string]*status.Demand)
	for pool, h := range hours {
		if want[pool] = demandIn(live, pool); want[pool] == nil || want[pool].Hours != h {
			t.Fatalf("before the restart, pool %s's demand is %+v, want it measured over %v hours", pool, want[pool], h)
		}
	}

	h := newHistory(config.History{}, at(clock.now))
	h.open(t.TempDir())
	writeLines(t, h, heard...)
	restarted, _ := newPairedRouterOf(t, at(clock.now), reserving)
	restarted.file.load(path)
	restarted.state.seed(h.readBack(clock.now))
	for pool := range hours {
		if got := demandIn(restarted, pool); !sameDemand(got, want[pool]) {
			t.Errorf("after the restart, pool %s's demand is %+v, want %+v, as before it: its own pin held from when it was set alone", pool, got, want[pool])
		}
	}
}

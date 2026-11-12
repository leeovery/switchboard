package router

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
)

// movedToPools marks the move to pools done in stateDir, its accounts record
// noting Default as the first pool, as the move does.
func movedToPools(t *testing.T, stateDir string) {
	t.Helper()
	noteFirstPool(t, stateDir, defaultPool)
	writeFile(t, filepath.Join(stateDir, "pools.moved"), "")
}

// membersOf are the pools the config file at path has each account in, by
// its id.
func membersOf(t *testing.T, path string) map[string]string {
	t.Helper()
	cfg, err := config.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	members := make(map[string]string)
	for _, a := range cfg.Accounts {
		members[a.ID] = a.Pool
	}
	return members
}

func TestAConfigThatLosesItsPoolsIsGivenThemBackAtTheLookAsTheyLastStood(t *testing.T) {
	logstest.Capture(t)
	stateDir := t.TempDir()
	r, path := newWatchingPoolsRouter(t, stateDir)
	r.upkeep.keepRecord()
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()
	movedToPools(t, stateDir)

	// Acme fenced to a second directory: its directories alone change, which
	// the record keeps, as it keeps all it heals the config with.
	widened := strings.Replace(acmeTOML, `["/work/acme"]`, `["/work/acme", "/work/acme-two"]`, 1)
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, widened))
	r.upkeep.look()

	// Put back from before pools: the look gives it them back, as they stood.
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	r.upkeep.look()
	acmeDirs := []string{"/work/acme", "/work/acme-two"}
	want := config.Pools{{ID: defaultPool, Label: "Default"}, {ID: acmePool, Label: "Acme", Dirs: acmeDirs, WrittenDirs: acmeDirs}}
	cfg, err := config.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !identicalPools(cfg.Pools, want) {
		t.Errorf("healed, the config's pools are %+v, want %+v, Acme's directories as they last stood", cfg.Pools, want)
	}
	wantMembers := map[string]string{"work": defaultPool, "personal": acmePool, "side": defaultPool}
	if got := membersOf(t, path); !maps.Equal(got, wantMembers) {
		t.Errorf("healed, the config has the accounts in the pools %v, want %v", got, wantMembers)
	}

	// The look after routes within them.
	r.upkeep.look()
	if got := r.proxy.pools.current().pools; !identicalPools(got, want) {
		t.Errorf("the look after the heal routes within the pools %+v, want %+v", got, want)
	}
}

func TestAConfigThatCantBeGivenItsPoolsBackIsTriedOnceAMinuteToldOfOnceForEachConfig(t *testing.T) {
	log := logstest.Capture(t)
	stateDir := t.TempDir()
	r, path := newWatchingPoolsRouter(t, stateDir)
	clock := &testClock{now: start}
	r.upkeep.heal.now = clock.read
	// Done, but with no first pool noted, so there's none to heal with.
	writeFile(t, filepath.Join(stateDir, "pools.moved"), "")
	unpooled := unpooledTOML("work", "personal", "side")

	writeConfigFile(t, path, unpooled)
	r.upkeep.look()
	r.upkeep.look()
	if n := timesLogged(log, "level=WARN", "couldn't write the pools back in the config"); n != 1 {
		t.Errorf("the log says the config couldn't be healed %d times, want once for the config\n%s", n, log)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != unpooled {
		t.Errorf("the config reads\n%s\n(%v), want it as it was\n%s", got, err, unpooled)
	}

	writeConfigFile(t, path, unpooledTOML("work", "personal"))
	r.upkeep.look()
	if n := timesLogged(log, "level=WARN", "couldn't write the pools back in the config"); n != 2 {
		t.Errorf("the log says the config couldn't be healed %d times, want again for a config changed since\n%s", n, log)
	}

	// What held the heal up passes, the config as it was: it's tried again
	// once a minute has passed, not before.
	noteFirstPool(t, stateDir, defaultPool)
	clock.now = clock.now.Add(healRetry - time.Second)
	r.upkeep.look()
	if got := membersOf(t, path); !maps.Equal(got, map[string]string{"work": "", "personal": ""}) {
		t.Errorf("within a minute of the heal that failed, the config has the accounts in the pools %v, want it as it was, not tried again yet", got)
	}
	clock.now = clock.now.Add(time.Second)
	r.upkeep.look()
	if got := membersOf(t, path); !maps.Equal(got, map[string]string{"work": defaultPool, "personal": defaultPool}) {
		t.Errorf("a minute on, the config has the accounts in the pools %v, want each in the first, the unfenced one", got)
	}
	if n := timesLogged(log, "level=WARN", "couldn't write the pools back in the config"); n != 2 {
		t.Errorf("the log says the config couldn't be healed %d times, want twice, once for each config\n%s", n, log)
	}
}

func TestAConfigPutBackFromBeforeBothMovesIsHealedUnderTheNewIDsAndRoutedInItsPools(t *testing.T) {
	logstest.Capture(t)
	stateDir := t.TempDir()
	err := identity.NewFile(stateDir).Update(func(r *identity.Record) error {
		r.Accounts = []identity.Entry{
			{ID: "work", Added: start, Was: "old-work", Pool: defaultPool},
			{ID: "personal", Added: start, Was: "old-personal", Pool: acmePool},
			{ID: "side", Added: start, Was: "old-side", Pool: defaultPool},
		}
		r.Pools = []identity.PoolEntry{
			{ID: defaultPool, Added: start, First: true, Here: true, Written: identity.PoolWritten{Label: "Default"}},
			{ID: acmePool, Added: start, Here: true, Written: identity.PoolWritten{Label: "Acme", Dirs: []string{"/work/acme"}}},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stateDir, "accounts.moved"), "")
	writeFile(t, filepath.Join(stateDir, "pools.moved"), "")
	r, path := newWatchingPoolsRouter(t, stateDir, func(cfg *Config) { cfg.Formerly = formerlyOld })
	// Personal's quota needs using first, then side's, then work's.
	readWeeks(r, map[string]quota.Window{"personal": soonWeek, "side": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	writeConfigFile(t, path, unpooledTOML("old-work", "old-personal", "old-side"))
	r.upkeep.look()
	r.upkeep.look()
	defaults := routeIn(t, r, workToken, "one", defaultPool, nil)
	acmes := routeIn(t, r, workToken, "two", acmePool, nil)

	if got, want := membersOf(t, path), map[string]string{"work": defaultPool, "personal": acmePool, "side": defaultPool}; !maps.Equal(got, want) {
		t.Errorf("healed, the config has the accounts in the pools %v, want %v, each by its new id", got, want)
	}
	if defaults.Code != http.StatusOK || acmes.Code != http.StatusOK {
		t.Errorf("Default's request was answered %d and Acme's %d, want 200 each", defaults.Code, acmes.Code)
	}
	if sent, want := upstream.sent(), []string{"side", "personal"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: each in its pool", sent, want)
	}
}

func TestARouterThatCantTellWhichPoolAnAccountIsInRoutesNothingSayingWhyTillItCan(t *testing.T) {
	log := logstest.Capture(t)
	stateDir := t.TempDir()
	writeFile(t, filepath.Join(stateDir, "pools.moved"), "")
	writeFile(t, identity.NewFile(stateDir).Path(), "not a record\n")
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.ConfigFile, cfg.StateDir = Watch(path), stateDir
	r := newTestRouterOf(t, cfg)
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "personal": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	const why = "the config names no pools and the accounts record can't be read"

	r.upkeep.look()
	firsts := routeIn(t, r, workToken, "one", "", nil)
	acmes := routeIn(t, r, workToken, "two", acmePool, nil)
	r.upkeep.look()

	if sent := upstream.sent(); len(sent) != 0 {
		t.Errorf("the requests went out on %q, want none: which pool each account is in can't be told", sent)
	}
	for _, w := range []*httptest.ResponseRecorder{firsts, acmes} {
		if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), why) {
			t.Errorf("a request was answered %d, %s; want switchboard's 429 saying %s", w.Code, w.Body, why)
		}
	}
	if n := timesLogged(log, "level=WARN", "nothing is routed"); n != 1 {
		t.Errorf("the log says nothing is routed %d times, want once\n%s", n, log)
	}
	if health := r.Status().Router; health.Healthy || !strings.Contains(health.Reason, why) {
		t.Errorf("the status document says the router is %+v, want it unhealthy, saying %s", health, why)
	}

	// The record is put right: the next look routes within the pools it
	// keeps, the config as it was.
	if err := os.Remove(identity.NewFile(stateDir).Path()); err != nil {
		t.Fatal(err)
	}
	if err := identity.NewFile(stateDir).Update(func(rec *identity.Record) error {
		*rec = recordOfThePools()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r.upkeep.look()
	routeIn(t, r, workToken, "three", "", nil)
	routeIn(t, r, workToken, "four", acmePool, nil)
	if sent, want := upstream.sent(), []string{"side", "personal"}; !slices.Equal(sent, want) {
		t.Errorf("once the record reads, the requests went out on %q, want %q: each in the pool it keeps", sent, want)
	}
	if health := r.Status().Router; !health.Healthy {
		t.Errorf("once the record reads, the status document says the router is %+v, want it healthy", health)
	}
}

func TestARouterStartedOnAConfigItCouldntHealKeepsAClientsAccountInItsPool(t *testing.T) {
	log := logstest.Capture(t)
	stateDir := t.TempDir()
	// Every pool fenced, and side, in none of them, has none to go to: the
	// heal as the router starts couldn't be written.
	err := identity.NewFile(stateDir).Update(func(r *identity.Record) error {
		r.Accounts = []identity.Entry{{ID: "work", Added: start, Pool: defaultPool}, {ID: "personal", Added: start, Pool: acmePool}}
		r.Pools = []identity.PoolEntry{
			{ID: defaultPool, Added: start, First: true, Here: true, Written: identity.PoolWritten{Label: "Default", Dirs: []string{"/work/own"}}},
			{ID: acmePool, Added: start, Here: true, Written: identity.PoolWritten{Label: "Acme", Dirs: []string{"/work/acme"}}},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stateDir, "pools.moved"), "")
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.ConfigFile, cfg.StateDir = Watch(path), stateDir
	cfg.Unhealed = errors.New("account side is in no pool the accounts record keeps")
	r := newTestRouterOf(t, cfg)
	// Side's quota needs using first, then personal's, then work's.
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "personal": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	r.upkeep.look()
	routeIn(t, r, workToken, "one", "", nil)
	routeIn(t, r, workToken, "two", acmePool, nil)

	if sent, want := upstream.sent(), []string{"work", "personal"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: the first's on work alone, Acme's on personal, side in no pool", sent, want)
	}
	if n := timesLogged(log, "level=WARN", "couldn't write the pools back in the config"); n != 1 {
		t.Errorf("the log says the config couldn't be healed %d times, want once, the heal as it started and the look's together\n%s", n, log)
	}
}

func TestTheAccountsRecordFollowsThePoolsLastFoundWhereTheConfigDoesntRead(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, t.TempDir())
	const broken = "[[account]\n"

	writeConfigFile(t, path, broken)
	accounts, pools := r.upkeep.record.followed()
	if side, _ := accounts.ByID("side"); side.Pool != defaultPool || !identicalPools(pools, testPools) {
		t.Errorf("followed() = side in %q, %+v; want side in Default, and the pools the router started with", side.Pool, pools)
	}

	writeConfigFile(t, path, poolsTOML(acmePool, defaultTOML, acmeTOML))
	r.upkeep.look()
	writeConfigFile(t, path, broken)
	accounts, pools = r.upkeep.record.followed()
	if side, _ := accounts.ByID("side"); side.Pool != acmePool || !identicalPools(pools, testPools) {
		t.Errorf("followed() = side in %q, %+v; want side in Acme, and the pools, as upkeep last found the config making them", side.Pool, pools)
	}
}

func TestTheRouterTakesUpThePoolsTheMoveWritesAtOnce(t *testing.T) {
	logstest.Capture(t)
	stateDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.ConfigFile, cfg.StateDir = Watch(path), stateDir
	r := newTestRouterOf(t, cfg)

	if r.takeUpPools() {
		t.Error("takeUpPools() = true, want false: the config names no pools")
	}
	noteFirstPool(t, stateDir, defaultPool)
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	if !r.takeUpPools() {
		t.Fatal("takeUpPools() = false, want true: the config holds the pools")
	}
	if got := r.proxy.pools.current(); !identicalPools(got.pools, testPools) || got.first != defaultPool || !got.holds(acmePool, "personal") {
		t.Errorf("the router routes within %+v, first %q; want the config's pools, personal in Acme, Default first", got.pools, got.first)
	}
}

func TestALineOfARequestResolvedBeforeThePoolsWereTakenUpNamesThePoolItsIn(t *testing.T) {
	tests := []struct {
		name string
		// pool is what the request's header names, "" for none.
		pool string
		want string
	}{
		{name: "naming none: the first", want: defaultPool},
		{name: "naming Acme", pool: acmePool, want: acmePool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r := newPoolsRouter(t, at(start), false)
			takingUp := func(req *http.Request) *http.Response {
				r.proxy.pools.follow(pooledAccounts(), testPools)
				return served(req)
			}
			r.proxy.transport = servingAll(takingUp, takingUp, takingUp)
			lines := keepingLedger(t, r)

			if w := routeIn(t, r, workToken, "one", tt.pool, nil); w.Code != http.StatusOK {
				t.Fatalf("answered %d, want 200", w.Code)
			}
			if got := poolsOf(lines()); !slices.Equal(got, []string{tt.want}) {
				t.Errorf("the ledger's lines name the pools %q, want one naming %q, the pools taken up as it was answered", got, tt.want)
			}
		})
	}
}

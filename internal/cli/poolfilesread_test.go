package cli_test

import (
	"encoding/json"
	"log/slog"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// splitSession is a session whose work is split between pools, as on the
// owner's first day with pools: refused as having left Default, then
// resumed in Acme.
const splitSession = "7c1d2e3f-0005-4a00-8000-000000000005"

// splitDeps are pooledDeps whose day holds splitSession's requests: two in
// Default, on side, then one in Acme, on work.
func splitDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps := pooledDeps(t, false)
	appendRequests(t, deps,
		ledger.Line{At: october(7, 11, 30, 0), Request: "s1", Session: splitSession, Account: "side", Pool: "b2n8"},
		ledger.Line{At: october(7, 11, 40, 0), Request: "s2", Session: splitSession, Account: "side", Pool: "b2n8"},
		ledger.Line{At: october(7, 11, 50, 0), Request: "s3", Session: splitSession, Account: "work", Pool: "q7mt"},
	)
	return deps
}

// loadedConfig is the config deps load, failing the test where it can't be.
func loadedConfig(t *testing.T, deps cli.Deps) *config.Config {
	t.Helper()
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// poolReadOf is how the dashboard reads the work of the pool with the given
// id from the files deps' state directory holds, Default the first pool, as
// its PoolRead.
func poolReadOf(t *testing.T, deps cli.Deps, id string) views.PoolRead {
	t.Helper()
	record := func() identity.Record { return identity.Record{} }
	return views.PoolRead{ID: id, First: "b2n8", PoolOf: status.AccountPools(loadedConfig(t, deps).Accounts, record)}
}

// dashboardsFollower follows the request ledger in deps' state directory,
// knowing Default the first pool, as the dashboard reads it.
func dashboardsFollower(t *testing.T, deps cli.Deps) *ledger.Follower {
	t.Helper()
	cfg := loadedConfig(t, deps)
	first := ledger.FirstPool(func() string { return "b2n8" })
	return ledger.NewFollower(stateDir(t, deps), deps.Now, ledger.CapsOf(cfg.Accounts, claude.SharedWindows()), views.DigestVersion, slog.New(slog.DiscardHandler), first)
}

// dashboardsPage is the page of the session with the given id as the
// dashboard reads it in the pool with the given id, with no router, its
// accounts named, as the JSON sessions <id> prints holds it.
func dashboardsPage(t *testing.T, deps cli.Deps, pool, id string) map[string]any {
	t.Helper()
	now := deps.Now()
	read := views.ReadSession(poolReadOf(t, deps, pool).Ledger(dashboardsFollower(t, deps)), now, id)
	page, ok := views.SessionPageOf(id, views.PageSources{Unlisted: true, Read: read, Prices: ledger.Pricing, Keep: config.DefaultLedgerKeep, Now: now})
	if !ok {
		t.Fatalf("the dashboard has no page of %s in pool %s", status.ShortID(id), pool)
	}
	data, err := json.Marshal(page.Named(status.NamesOf(loadedConfig(t, deps).Accounts, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return jsonObject(t, string(data))
}

// jsonObject is the JSON object text holds, its schema left out.
func jsonObject(t *testing.T, text string) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal([]byte(text), &object); err != nil {
		t.Fatalf("%q isn't a JSON object: %v", text, err)
	}
	delete(object, "schema")
	return object
}

// A session split between pools has, in each pool named, a page of its work
// there alone, as the dashboard's page of it in that pool: its requests,
// turns, worth, tokens and points. Without --pool, it's the page of its work
// in the pool it's in now, the one its newest request was routed in.
func TestASplitSessionsPageIsOfItsWorkInThePoolAlone(t *testing.T) {
	deps := splitDeps(t)
	for _, tt := range []struct {
		args     []string
		pool     string
		requests float64
	}{
		{args: []string{"--pool", "b2n8"}, pool: "b2n8", requests: 2},
		{args: []string{"--pool", "q7mt"}, pool: "q7mt", requests: 1},
		{pool: "q7mt", requests: 1},
	} {
		got := run(t, deps, append([]string{"sessions", splitSession, "--json"}, tt.args...)...)
		if got.code != 0 {
			t.Fatalf("switchboard sessions %s --json %s = %+v, want its page", status.ShortID(splitSession), strings.Join(tt.args, " "), got)
		}
		page, want := jsonObject(t, got.stdout), dashboardsPage(t, deps, tt.pool, splitSession)
		if !reflect.DeepEqual(page, want) {
			t.Errorf("switchboard sessions %s --json %s printed\n%v\nwant the dashboard's page in pool %s\n%v", status.ShortID(splitSession), strings.Join(tt.args, " "), page, tt.pool, want)
		}
		if totals, _ := page["totals"].(map[string]any); totals["requests"] != tt.requests {
			t.Errorf("switchboard sessions %s --json %s totals %v, want %v requests, those in pool %s alone", status.ShortID(splitSession), strings.Join(tt.args, " "), totals, tt.requests, tt.pool)
		}
	}
}

// A session with no work in the pool named isn't found there, as a session
// the ledger doesn't hold isn't, the pool named: one ended wholly in
// Default, one from before pools among them, asked for in Acme.
func TestASessionWithNoWorkInThePoolNamedIsNotFoundThere(t *testing.T) {
	deps := pooledDeps(t, false)
	for _, id := range []string{sessionA, sessionC} {
		got := run(t, deps, "sessions", id, "--pool", "q7mt", "--json")
		want := "no session " + id + " in the ledger, or among the router's sessions in pool Acme"
		if got.code == 0 || got.stdout != "" || !strings.Contains(got.stderr, want) {
			t.Errorf("switchboard sessions %s --pool q7mt --json = %+v, want it refused, saying %q", status.ShortID(id), got, want)
		}
		if got := run(t, deps, "sessions", id, "--pool", "b2n8", "--json"); got.code != 0 || !strings.Contains(got.stdout, id) {
			t.Errorf("switchboard sessions %s --pool b2n8 --json = %+v, want its page in Default", status.ShortID(id), got)
		}
	}
}

// requests --session without --pool reads the session's requests in the
// pool it's in now, the one its newest request was routed in, so a session
// split between pools never has every pool's requests listed together;
// with --pool, those in that pool alone.
func TestASplitSessionsRequestsAreThoseInItsPoolNowElseInThePoolNamed(t *testing.T) {
	deps := splitDeps(t)
	for _, tt := range []struct {
		args []string
		want []string
	}{
		{args: nil, want: []string{"s3"}},
		{args: []string{"--pool", "q7mt"}, want: []string{"s3"}},
		{args: []string{"--pool", "b2n8"}, want: []string{"s1", "s2"}},
	} {
		got := run(t, deps, append([]string{"requests", "--session", splitSession, "--json"}, tt.args...)...)
		if requests := fieldsOf(t, got, "request"); got.code != 0 || !slices.Equal(requests, tt.want) {
			t.Errorf("switchboard requests --session %s --json %s printed %q (%+v), want %q", status.ShortID(splitSession), strings.Join(tt.args, " "), requests, got, tt.want)
		}
	}
	got := run(t, deps, "requests", "--session", splitSession, "--pretty")
	if !strings.HasPrefix(got.stdout, "pool Acme\n\n") {
		t.Errorf("switchboard requests --session %s --pretty printed\n%s\nwant it to name the pool it read, Acme", status.ShortID(splitSession), got.stdout)
	}
}

// acmeLaterConfig is pooledConfig with work on a plan, so its worth is
// priced against it.
const acmeLaterConfig = `[[account]]
id    = "work"
label = "Work"
pool  = "q7mt"
plan  = "max"

[[account]]
id    = "side"
label = "Side"
pool  = "b2n8"

[[pool]]
id    = "b2n8"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["/work/acme"]
`

// history --pool of a pool whose first work falls after --since counts its
// plans, months and tokens from that first day, as the dashboard counts the
// pool's days from where its read's Start tells it begins: never the days
// before the pool worked as days of none.
func TestHistoryOfAPoolCountsItsDaysFromItsFirstWork(t *testing.T) {
	deps := pooledDeps(t, false)
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(acmeLaterConfig))
	for day := 1; day <= 6; day++ {
		appendRequests(t, deps, ledger.Line{At: october(day, 9, 0, 0), Request: "d" + strconv.Itoa(day), Session: sessionC, Account: "side", Pool: "b2n8"})
	}
	got := run(t, deps, "history", "--pool", "q7mt", "--since", "2026-10-01", "--json")
	if got.code != 0 {
		t.Fatalf("switchboard history --pool q7mt --since 2026-10-01 --json = %+v", got)
	}
	printed := jsonObject(t, got.stdout)

	cfg, now := loadedConfig(t, deps), deps.Now()
	read, follower := poolReadOf(t, deps, "q7mt"), dashboardsFollower(t, deps)
	first, err := read.Start(follower).FirstDay(now)
	began, _, err := ledger.Begins(first, err, now)
	if err != nil || !began.Equal(october(7, 0, 0, 0)) {
		t.Fatalf("Acme's work begins %v, %v, as the dashboard reads it, want the start of 7 October", began, err)
	}
	in := views.HistoryInput{
		Ledger: read.Days(follower), From: october(1, 0, 0, 0), Now: now, Prices: ledger.Pricing, Accounts: cfg.Accounts.InPool("q7mt"),
		WeekStarts: cfg.WeekStarts, WeekWindow: claude.WeekWindow, Family: claude.Provider{}.Family,
	}
	unbegun := views.NewHistory(in)
	in.Began = began
	dashboards := views.NewHistory(in)
	for _, block := range []string{"plans", "months", "tokens"} {
		want, before := jsonField(t, dashboards, block), jsonField(t, unbegun, block)
		if reflect.DeepEqual(want, before) {
			t.Fatalf("history's %s are alike counted from 1 and 7 October, so they can't tell where it counted from", block)
		}
		if !reflect.DeepEqual(printed[block], want) {
			t.Errorf("switchboard history --pool q7mt --since 2026-10-01 --json gave the %s\n%v\nwant those counted from Acme's first work, 7 October, as the dashboard counts them\n%v", block, printed[block], want)
		}
	}
}

// jsonField is the field named of the JSON object v marshals as, as JSON
// decodes it.
func jsonField(t *testing.T, v any, field string) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	object := jsonObject(t, string(data))
	if _, ok := object[field]; !ok {
		t.Fatalf("%T's JSON has no %s among %v", v, field, slices.Collect(maps.Keys(object)))
	}
	return object[field]
}

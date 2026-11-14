package cli_test

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
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
		delete(page, "pool")
		delete(page, "pool_label")
		if !reflect.DeepEqual(page, want) {
			t.Errorf("switchboard sessions %s --json %s printed\n%v\nwant the dashboard's page in pool %s\n%v", status.ShortID(splitSession), strings.Join(tt.args, " "), page, tt.pool, want)
		}
		if totals, _ := page["totals"].(map[string]any); totals["requests"] != tt.requests {
			t.Errorf("switchboard sessions %s --json %s totals %v, want %v requests, those in pool %s alone", status.ShortID(splitSession), strings.Join(tt.args, " "), totals, tt.requests, tt.pool)
		}
	}
}

// digestedAsTheRouter has the days before today of the request ledger in
// deps' state directory summarised and digested as the router digests them,
// at deps' now, Default the first pool, failing where yesterday's digest
// isn't written.
func digestedAsTheRouter(t *testing.T, deps cli.Deps) {
	t.Helper()
	quiet, state, now := slog.New(slog.DiscardHandler), stateDir(t, deps), deps.Now()
	caps := ledger.CapsOf(loadedConfig(t, deps).Accounts, claude.SharedWindows())
	l := ledger.Open(ledger.Dir(state), 400*24*time.Hour, deps.Now, ledger.HistoryIn(readings.Files(readings.Dir(state), quiet)), caps, quiet,
		ledger.Digesting(views.Digests{}), ledger.FirstPool(func() string { return "b2n8" }))
	l.SummariseEnded(now)
	l.DigestEnded(t.Context(), now)
	if found, _ := filepath.Glob(filepath.Join(ledger.Dir(state), "digest-2026-10-06*")); len(found) == 0 {
		t.Fatal("no digest of 6 October was written")
	}
}

// A session split between pools on a day since digested, as the owner's
// first day with pools is read any later day, rests on the day's digest,
// with no lines after it: its page and its requests are read in the pool
// its newest request was routed in, else in the pool named, its page as the
// dashboard's in that pool.
func TestASplitSessionOnADigestedDayIsReadInItsPoolNowElseInThePoolNamed(t *testing.T) {
	deps := pooledDeps(t, false)
	appendRequests(t, deps,
		ledger.Line{At: october(6, 11, 30, 0), Request: "s1", Session: splitSession, Account: "side", Pool: "b2n8"},
		ledger.Line{At: october(6, 11, 40, 0), Request: "s2", Session: splitSession, Account: "side", Pool: "b2n8"},
		ledger.Line{At: october(6, 11, 50, 0), Request: "s3", Session: splitSession, Account: "work", Pool: "q7mt"},
	)
	digestedAsTheRouter(t, deps)
	for _, tt := range []struct {
		args     []string
		pool     string
		requests []string
	}{
		{args: []string{"--pool", "b2n8"}, pool: "b2n8", requests: []string{"s1", "s2"}},
		{args: []string{"--pool", "q7mt"}, pool: "q7mt", requests: []string{"s3"}},
		{pool: "q7mt", requests: []string{"s3"}},
	} {
		asked := strings.Join(tt.args, " ")
		got := run(t, deps, append([]string{"sessions", splitSession, "--json"}, tt.args...)...)
		if got.code != 0 {
			t.Fatalf("switchboard sessions %s --json %s = %+v, want its page", status.ShortID(splitSession), asked, got)
		}
		page, want := jsonObject(t, got.stdout), dashboardsPage(t, deps, tt.pool, splitSession)
		if page["pool"] != tt.pool {
			t.Errorf("switchboard sessions %s --json %s read pool %v, want %s", status.ShortID(splitSession), asked, page["pool"], tt.pool)
		}
		delete(page, "pool")
		delete(page, "pool_label")
		if !reflect.DeepEqual(page, want) {
			t.Errorf("switchboard sessions %s --json %s printed\n%v\nwant the dashboard's page in pool %s\n%v", status.ShortID(splitSession), asked, page, tt.pool, want)
		}
		got = run(t, deps, append([]string{"requests", "--session", splitSession, "--since", "2026-10-01", "--json"}, tt.args...)...)
		if requests := fieldsOf(t, got, "request"); got.code != 0 || !slices.Equal(requests, tt.requests) {
			t.Errorf("switchboard requests --session %s --since 2026-10-01 --json %s printed %q (%+v), want %q", status.ShortID(splitSession), asked, requests, got, tt.requests)
		}
	}
}

// A session's page names the pool it's of, once there's a second: its text
// headed by it, as requests' is, and its JSON giving its id and its label,
// as every verb's JSON gives a pool. That's the pool named, else the one the
// session is in now: a split session's, the pool its newest request was
// routed in; a session of one pool's work, that pool.
func TestASessionsPageNamesThePoolItsOf(t *testing.T) {
	deps := splitDeps(t)
	for _, tt := range []struct {
		session string
		args    []string
		id      string
		label   string
	}{
		{session: splitSession, id: "q7mt", label: "Acme"},
		{session: splitSession, args: []string{"--pool", "b2n8"}, id: "b2n8", label: "Default"},
		{session: sessionB, id: "q7mt", label: "Acme"},
		{session: sessionA, id: "b2n8", label: "Default"},
	} {
		asked := strings.Join(append([]string{status.ShortID(tt.session)}, tt.args...), " ")
		got := run(t, deps, append([]string{"sessions", tt.session, "--json"}, tt.args...)...)
		if page := jsonObject(t, got.stdout); got.code != 0 || page["pool"] != tt.id || page["pool_label"] != tt.label {
			t.Errorf("switchboard sessions %s --json gave the pool %v labelled %v (%+v), want %s labelled %s", asked, page["pool"], page["pool_label"], got, tt.id, tt.label)
		}
		got = run(t, deps, append([]string{"sessions", tt.session, "--pretty"}, tt.args...)...)
		if want := "pool " + tt.label + "\n\nsession "; got.code != 0 || !strings.HasPrefix(got.stdout, want) {
			t.Errorf("switchboard sessions %s --pretty printed\n%s\nwant it headed %q", asked, got.stdout, want)
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

// A split session whose newest request names a pool neither the config nor
// the accounts record keeps can't be read in it without --pool: refused,
// saying so, never asking for a --pool that wasn't given.
func TestASplitSessionInAPoolNothingKeepsIsRefusedSayingSo(t *testing.T) {
	deps := pooledDeps(t, false)
	appendRequests(t, deps,
		ledger.Line{At: october(7, 11, 30, 0), Request: "u1", Session: splitSession, Account: "side", Pool: "b2n8"},
		ledger.Line{At: october(7, 11, 40, 0), Request: "u2", Session: splitSession, Account: "side", Pool: "x9z9"},
	)
	const want = `can't tell which pool to read: the newest request names pool "x9z9", which neither the config nor the accounts record keeps`
	for _, args := range [][]string{{"sessions", splitSession}, {"requests", "--session", splitSession}} {
		got := run(t, deps, append(args, "--json")...)
		if got.code == 0 || got.stdout != "" || !strings.Contains(got.stderr, want) || strings.Contains(got.stderr, "--pool") {
			t.Errorf("switchboard %s --json = %+v, want it refused, saying %q, and nothing of --pool", strings.Join(args, " "), got, want)
		}
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

// history --pool of a pool whose first work falls after the days asked for
// begin counts its plans, months and tokens from that first day, as the
// dashboard's History tab counts them, its Past begun where the pool's read's
// Start tells it begins: the verb's plans as the tab's Accounts over its 30
// days, the days the verb reads by default, its months as the Year's BY
// MONTH, and its weeks' tokens as Tokens' weeks. Never the days before the
// pool worked as days of none.
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
	got := run(t, deps, "history", "--pool", "q7mt", "--json")
	if got.code != 0 {
		t.Fatalf("switchboard history --pool q7mt --json = %+v", got)
	}
	printed := jsonObject(t, got.stdout)
	printed["tokens"] = weeksRecorded(printed["tokens"])
	tab, unbegun := dashboardsPast(t, deps, "q7mt", october(7, 0, 0, 0))
	for _, figures := range []struct {
		name string
		of   func(views.Past) any
	}{
		{name: "plans", of: pastPlans},
		{name: "months", of: func(p views.Past) any { return pastMonths(t, p, printed["months"]) }},
		{name: "tokens", of: pastTokens},
	} {
		want, before := asJSON(t, figures.of(tab)), asJSON(t, figures.of(unbegun))
		if reflect.DeepEqual(want, before) {
			t.Fatalf("the History tab's %s are alike counted from Acme's first work and from the first day it shows, so they can't tell where it counted from", figures.name)
		}
		if !reflect.DeepEqual(printed[figures.name], want) {
			t.Errorf("switchboard history --pool q7mt --json gave the %s\n%v\nwant those counted from Acme's first work, 7 October, as the History tab counts them\n%v", figures.name, printed[figures.name], want)
		}
	}
}

// dashboardsPast is the Past the dashboard's History tab reads of the pool
// with the given id, of the files in deps' state directory, as the watch
// reads the last 12 months, begun where the pool's read's Start tells it
// begins, which must be at want; and that Past not begun, as it stands
// before Start is learned.
func dashboardsPast(t *testing.T, deps cli.Deps, pool string, want time.Time) (begun, unbegun views.Past) {
	t.Helper()
	cfg, now := loadedConfig(t, deps), deps.Now()
	read, follower := poolReadOf(t, deps, pool), dashboardsFollower(t, deps)
	first, err := read.Start(follower).FirstDay(now)
	began, _, err := ledger.Begins(first, err, now)
	if err != nil || !began.Equal(want) {
		t.Fatalf("pool %s's work begins %v, %v, as the dashboard reads it, want %v", pool, began, err, want)
	}
	from := views.YearFrom(views.YearAsk{}, now, cfg.WeekStarts)
	in := views.HistoryInput{
		From: from, Now: now, Prices: ledger.Pricing, Accounts: cfg.Accounts.InPool(pool),
		WeekStarts: cfg.WeekStarts, WeekWindow: claude.WeekWindow, Family: claude.Provider{}.Family,
	}
	unbegun = views.NewPast(in, read.Days(follower).Days(from))
	return unbegun.Begun(began), unbegun
}

// pastPlans are the plans the History tab's Accounts prices over its 30
// days, each account's and every account's, as history's JSON gives them.
func pastPlans(p views.Past) any {
	page := p.Accounts(views.AccountsThirtyDays)
	var plans []views.PlanCost
	for _, row := range page.Rows {
		plans = append(plans, row.Plan)
	}
	return append(plans, page.All.Plan)
}

// pastMonths are the months of the Year's BY MONTH among those history's
// JSON gave, printed, as it gives them, failing where any it gave isn't
// among them.
func pastMonths(t *testing.T, p views.Past, printed any) any {
	t.Helper()
	rows := p.Year(views.YearAsk{}).ByMonth
	gave, _ := printed.([]any)
	months := make([]views.Month, 0, len(gave))
	for _, m := range gave {
		month, _ := m.(map[string]any)["month"].(string)
		i := slices.IndexFunc(rows, func(r views.MonthRow) bool { return r.Month.Month == month })
		if i < 0 {
			t.Fatalf("history gave the month %q, which the Year's BY MONTH doesn't hold", month)
		}
		months = append(months, rows[i].Month)
	}
	return months
}

// pastTokens are the weeks of Tokens with anything recorded, oldest first,
// as history's JSON gives them.
func pastTokens(p views.Past) any {
	periods := p.Tokens(views.TokensAsk{Period: views.TokensByWeek}).Periods
	weeks := make([]views.TokenWeek, 0, len(periods))
	for _, t := range slices.Backward(periods) {
		weeks = append(weeks, views.TokenWeek{Week: t.First, Tokens: t.Tokens, Worth: t.Worth, Against: t.Against, ByAccount: t.ByAccount, ByModel: t.ByModel})
	}
	return weeks
}

// weeksRecorded are the weeks of history's JSON tokens, printed, with
// anything recorded of any account, as Tokens lists them: history lists
// every week its days span.
func weeksRecorded(printed any) any {
	weeks, _ := printed.([]any)
	recorded := []any{}
	for _, w := range weeks {
		if _, ok := w.(map[string]any)["by_account"]; ok {
			recorded = append(recorded, w)
		}
	}
	return recorded
}

// asJSON is v as JSON decodes it once marshalled.
func asJSON(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

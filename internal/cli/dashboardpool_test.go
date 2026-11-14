package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// eventIDs are the ids of the router's events the dashboard reads through
// events, in their order.
func eventIDs(events views.Events) []int {
	var ids []int
	for _, e := range views.ReadEventsFrom(events, views.RunwayLatelyFrom(ledgerNow), ledgerNow).Events() {
		ids = append(ids, e.Event.ID)
	}
	return ids
}

// writeDepsConfig writes text as deps' config file.
func writeDepsConfig(t *testing.T, deps cli.Deps, text string) {
	t.Helper()
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(text))
}

// The dashboard shows the pool --pool names, else the one the verbs act on:
// the pool's label, directories and accounts, the pools counted; its share
// of the router's events alone, by the one rule the verbs read them by.
func TestTheDashboardShowsThePoolNamedElseTheOneTheVerbsActOn(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		want   status.ShownPool
		events []int
	}{
		{name: "none named: the unfenced pool", want: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 1, Pools: 2}, events: []int{1, 4, 5}},
		{name: "one named", args: []string{"--pool", "q7mt"}, want: status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"/work/acme"}, Accounts: 1, Pools: 2}, events: []int{2, 3, 5}},
		{name: "one named empty: none named", args: []string{"--pool", ""}, want: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 1, Pools: 2}, events: []int{1, 4, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := pooledDeps(t, false)
			cfg := recordWatch(t, &deps)
			if got := run(t, deps, append([]string{"dashboard"}, tt.args...)...); got != (result{}) {
				t.Fatalf("switchboard dashboard %s = %+v, want exit status 0 and nothing printed", strings.Join(tt.args, " "), got)
			}
			if got := cfg.Pool; got.ID != tt.want.ID || got.Label != tt.want.Label || !slices.Equal(got.Dirs, tt.want.Dirs) || got.Accounts != tt.want.Accounts || got.Pools != tt.want.Pools {
				t.Errorf("the dashboard shows %+v, want %+v", got, tt.want)
			}
			if ids := eventIDs(cfg.Events); !slices.Equal(ids, tt.events) {
				t.Errorf("the dashboard's events are those of the ids %v, want %v", ids, tt.events)
			}
		})
	}
}

// The dashboard lists the accounts of the pool it shows alone, as named and
// as P turns to the next, and none where it shows none: no pool's accounts
// are listed in another's, nor History's kept view of one carried into it.
func TestTheDashboardListsTheAccountsOfThePoolItShowsAlone(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "q7mt"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool q7mt = %+v, want exit status 0 and nothing printed", got)
	}
	if got := cfg.Accounts.IDs(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("in Acme, the dashboard lists the accounts %v, want Acme's alone, work", got)
	}
	turned, err := cfg.Turn(t.Context(), "q7mt", watch.NextPool)
	if err != nil {
		t.Fatal(err)
	}
	if got := turned.Accounts.IDs(); !slices.Equal(got, []string{"side"}) {
		t.Errorf("turned to Default, the dashboard lists the accounts %v, want Default's alone, side", got)
	}

	fenced := pooledDeps(t, true)
	none := recordWatch(t, &fenced)
	if got := run(t, fenced, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	if got := none.Accounts.IDs(); len(got) != 0 {
		t.Errorf("with every pool fenced and none named, the dashboard lists the accounts %v, want none", got)
	}
}

// The dashboard's source reads the router's document, sessions, history and
// request stream of the pool it shows, as named and as P turns to the next:
// never another pool's under the pool's name.
func TestTheDashboardsSourceReadsThePoolItShows(t *testing.T) {
	deps := pooledDeps(t, false)
	var mu sync.Mutex
	asked := map[string][]string{}
	serveRouter(t, router.SocketPath(stateDir(t, deps)), new(atomic.Int64), map[string]http.HandlerFunc{
		"GET /health": reportsPools,
		"GET /": func(w http.ResponseWriter, req *http.Request) {
			pool := req.URL.Query().Get(router.PoolQuery)
			mu.Lock()
			asked[req.URL.Path] = append(asked[req.URL.Path], pool)
			mu.Unlock()
			switch req.URL.Path {
			case "/status":
				_ = json.NewEncoder(w).Encode(status.Document{PoolID: pool, Accounts: []status.Account{{ID: map[string]string{"q7mt": "work", "b2n8": "side"}[pool]}}})
			case "/sessions":
				_, _ = io.WriteString(w, "[]")
			case "/history":
				_, _ = io.WriteString(w, "{}")
			}
		},
	})
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "q7mt"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool q7mt = %+v, want exit status 0 and nothing printed", got)
	}
	turned, err := cfg.Turn(t.Context(), "q7mt", watch.NextPool)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, pool, account string
		source              watch.Source
	}{
		{name: "opened on Acme", pool: "q7mt", account: "work", source: cfg.Source},
		{name: "turned to Default", pool: "b2n8", account: "side", source: turned.Source},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mu.Lock()
			clear(asked)
			mu.Unlock()
			doc, _, err := tt.source.Read(t.Context(), status.Read{})
			if err != nil {
				t.Fatal(err)
			}
			if doc.PoolID != tt.pool || len(doc.Accounts) != 1 || doc.Accounts[0].ID != tt.account {
				t.Errorf("the source read the document of %q, holding %+v; want %s's, holding %s", doc.PoolID, doc.Accounts, tt.pool, tt.account)
			}
			if _, err := tt.source.Sessions(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := tt.source.History(t.Context(), "7d", time.Hour); err != nil {
				t.Fatal(err)
			}
			stream, err := tt.source.Stream(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for range stream {
			}
			mu.Lock()
			defer mu.Unlock()
			for _, path := range []string{"/status", "/sessions", "/history", "/stream"} {
				if got := asked[path]; len(got) == 0 || slices.ContainsFunc(got, func(p string) bool { return p != tt.pool }) {
					t.Errorf("the source asked %s of the pools %q, want %s's alone", path, got, tt.pool)
				}
			}
		})
	}
}

// A pool the config doesn't configure is refused, naming those it does,
// before the dashboard opens: one never configured, and one the accounts
// record keeps since its removal, which has no accounts to show.
func TestTheDashboardRefusesAPoolTheConfigDoesntConfigure(t *testing.T) {
	tests := []struct {
		name, pool, want string
		removed          bool
	}{
		{name: "never configured", pool: "x9z9", want: `there's no pool "x9z9": name b2n8 (Default) or q7mt (Acme), with --pool`},
		{name: "removed", pool: "q7mt", want: `there's no pool "q7mt": name b2n8 (Default), with --pool`, removed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := pooledDeps(t, false)
			if tt.removed {
				onePoolLeft(t, deps, acmeRemovedEntry)
			}
			deps.Watch = func(context.Context, watch.Config, io.Writer, []string) error {
				t.Error("the dashboard opened, want it refused")
				return nil
			}
			if got := run(t, deps, "dashboard", "--pool", tt.pool); got.code == 0 || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("switchboard dashboard --pool %s = %+v, want it refused, saying %q", tt.pool, got, tt.want)
			}
		})
	}
}

// Where every pool is fenced and none is named, the dashboard opens on
// none, its source refused by the router, as the status line says, the
// pools counted for P to turn to the first, and none of the router's
// events read. Named, it opens on the pool named.
func TestWhereEveryPoolIsFencedTheDashboardOpensOnNoneTillOneIsNamed(t *testing.T) {
	deps := pooledDeps(t, true)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	if got := cfg.Pool; got.ID != "" || got.Pools != 2 {
		t.Errorf("the dashboard shows %+v, want no pool, of two", got)
	}
	if ids := eventIDs(cfg.Events); len(ids) != 0 {
		t.Errorf("the dashboard read the events %v, want none", ids)
	}
	if got := run(t, deps, "dashboard", "--pool", "b2n8"); got != (result{}) || cfg.Pool.ID != "b2n8" {
		t.Errorf("switchboard dashboard --pool b2n8 = %+v, showing %q; want Default shown", got, cfg.Pool.ID)
	}
}

// P turns to the next pool in the config's order, round, and from none
// shown to the first, reading the config afresh at each: a pool added
// while the dashboard is open shows at the next P, with its share of the
// router's events and their pools named afresh.
func TestTheDashboardsTurnReadsThePoolsAfresh(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	turned := func(from string) watch.Pooled {
		t.Helper()
		pooled, err := cfg.Turn(t.Context(), from, watch.NextPool)
		if err != nil {
			t.Fatalf("Turn(%q) failed: %v", from, err)
		}
		return pooled
	}
	for _, tt := range []struct{ from, want string }{{"b2n8", "q7mt"}, {"q7mt", "b2n8"}, {"", "b2n8"}} {
		if got := turned(tt.from); got.Pool.ID != tt.want || got.Source == nil {
			t.Errorf("Turn(%q) shows %q, want %q, with a source of its own", tt.from, got.Pool.ID, tt.want)
		}
	}
	if ids := eventIDs(turned("b2n8").Events); !slices.Equal(ids, []int{2, 3, 5}) {
		t.Errorf("turned to Acme, the dashboard's events are those of the ids %v, want Acme's, 2, 3 and 5", ids)
	}

	writeDepsConfig(t, deps, pooledConfig(false)+"\n[[pool]]\nid    = \"r2d4\"\nlabel = \"Client\"\ndirs  = [\"/work/client\"]\n")
	client := turned("q7mt")
	if client.Pool.ID != "r2d4" || client.Pool.Pools != 3 || !client.Pool.Empty() {
		t.Errorf("turned from Acme once Client is added, the dashboard shows %+v, want Client, of three pools, with no accounts", client.Pool)
	}
	if client.PoolNamed == nil || client.PoolNamed("r2d4") != "Client" {
		t.Error("turned to Client, its events don't name it, want the pools named afresh")
	}
}

// A turn that can't read the config fails, saying why, for the dashboard
// to keep the pool it shows.
func TestTheDashboardsTurnFailsWhereTheConfigCantBeRead(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	writeDepsConfig(t, deps, "[[account]\n")
	if _, err := cfg.Turn(t.Context(), "b2n8", watch.NextPool); err == nil {
		t.Error("Turn() of a config that can't be read turned, want it to fail")
	}
}

// In a config without pools, before the move to pools, the dashboard shows
// no pool, and --pool is passed over, as on every verb.
func TestADashboardOfAConfigWithoutPoolsPassesOverPool(t *testing.T) {
	deps := testDeps(map[string]string{"XDG_STATE_HOME": shortTempDir(t)}, t.TempDir())
	writeDepsConfig(t, deps, "[[account]]\nid = \"work\"\nlabel = \"Work\"\n")
	markDone(t, stateDir(t, deps), idsMark)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "q7mt"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool q7mt = %+v, want exit status 0 and nothing printed", got)
	}
	if cfg.Pool.ID != "" || cfg.Pool.Turns() {
		t.Errorf("the dashboard shows %+v, want no pool, and none to turn to", cfg.Pool)
	}
}

// Where nothing names the pools once the move to pools is done, a pool
// named is refused, saying why, as the verbs refuse it; none named, the
// dashboard opens on no pool, reading none of the router's events, though
// their files hold some.
func TestADashboardWhereNothingNamesThePoolsRefusesAPoolNamed(t *testing.T) {
	deps := unpooledDeps(t, unpooledConfig, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	if ids := eventIDs(cfg.Events); cfg.Pool.ID != "" || len(ids) != 0 {
		t.Errorf("the dashboard shows %+v, reading the events %v; want no pool, and none", cfg.Pool, ids)
	}

	deps.Watch = func(context.Context, watch.Config, io.Writer, []string) error {
		t.Error("the dashboard opened, want it refused")
		return nil
	}
	if got := run(t, deps, "dashboard", "--pool", "q7mt"); got.code == 0 || !strings.Contains(got.stderr, "can't tell which pool to read") {
		t.Errorf("switchboard dashboard --pool q7mt = %+v, want it refused, saying it can't tell the pools", got)
	}
}

// A dashboard opened where nothing names the pools lists no accounts, and
// its source reads none, as where every pool is fenced: no document, probed
// or the router's, no sessions, history or request stream, no order given,
// each saying why. Once the router answers with the pools it routes for, a
// read fails with watch.ErrPoolsTold, for the dashboard to read the pool
// shown again, telling them.
func TestADashboardWhereNothingNamesThePoolsReadsNoneTillTheRouterTellsThem(t *testing.T) {
	const why = "can't tell which pool to read"
	deps := unpooledDeps(t, unpooledConfig, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	if len(cfg.Accounts) != 0 {
		t.Errorf("the dashboard lists the accounts %+v, want none", cfg.Accounts)
	}
	ctx := t.Context()
	doc, _, err := cfg.Source.Read(ctx, status.Read{Probe: true})
	if err == nil || !strings.Contains(err.Error(), why) || len(doc.Accounts) != 0 {
		t.Errorf("the dashboard's source read %d accounts, failing with %v; want none, saying %q", len(doc.Accounts), err, why)
	}
	_, sessionsErr := cfg.Source.Sessions(ctx)
	_, historyErr := cfg.Source.History(ctx, "5h", time.Minute)
	_, streamErr := cfg.Source.Stream(ctx)
	for name, err := range map[string]error{
		"sessions": sessionsErr, "history": historyErr, "stream": streamErr,
		"pin": cfg.Source.Pin(ctx, []string{"work"}, false), "unpin": cfg.Source.Unpin(ctx),
		"session's pin": cfg.Source.PinSession(ctx, sessionA, "work"), "session's unpin": cfg.Source.UnpinSession(ctx, sessionA),
	} {
		if err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("the dashboard's source's %s failed with %v, want it refused, saying %q", name, err, why)
		}
	}

	serveHealth(t, deps, router.Health{OK: true, Listen: "127.0.0.1:4747", StripsOwnHeaders: true, ReportsPools: true, Pools: []router.RoutedPool{
		{ID: "b2n8", Label: "Default", Accounts: []string{"side"}, Primary: "side"},
		{ID: "q7mt", Label: "Acme", Dirs: []string{"/work/acme"}, WrittenDirs: []string{"/work/acme"}, Accounts: []string{"work"}, Primary: "work"},
	}})
	if doc, _, err := cfg.Source.Read(ctx, status.Read{Probe: true}); !errors.Is(err, watch.ErrPoolsTold) || len(doc.Accounts) != 0 {
		t.Errorf("with the router telling the pools, the dashboard's source read %d accounts, failing with %v; want none, failing with watch.ErrPoolsTold", len(doc.Accounts), err)
	}
	turned, err := cfg.Turn(ctx, "", watch.SamePool)
	if err != nil || turned.Pool.ID != "b2n8" || len(turned.Accounts) != 1 {
		t.Errorf("the pool shown read again, the dashboard shows %+v of the accounts %+v, failing with %v; want Default, of side", turned.Pool, turned.Accounts, err)
	}
}

// A dashboard opened on no pool, in a config without pools before the move
// to pools, with no history yet to move, or once the move is done where
// nothing names the pools, reads the pool the verbs act on once the config
// names pools, as the dashboard reads the pool shown again as the config
// changes: Default, its requests alone of the ledger filed since, where it
// read every pool's, or none.
func TestADashboardOpenedOnNoPoolReadsThePoolTheConfigNamesOnceItDoes(t *testing.T) {
	tests := []struct {
		name   string
		deps   func(t *testing.T) cli.Deps
		opened []string
	}{
		{name: "a config without pools, before the move", deps: func(t *testing.T) cli.Deps {
			deps := testDeps(map[string]string{"XDG_STATE_HOME": shortTempDir(t)}, t.TempDir())
			deps.Now = func() time.Time { return ledgerNow }
			writeDepsConfig(t, deps, unpooledConfig)
			markDone(t, stateDir(t, deps), idsMark)
			return deps
		}, opened: []string{"p1", "p2", "p3"}},
		{name: "the pools not to be told", deps: func(t *testing.T) cli.Deps { return unpooledDeps(t, unpooledConfig, false) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := tt.deps(t)
			cfg := recordWatch(t, &deps)
			if got := run(t, deps, "dashboard"); got != (result{}) {
				t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
			}
			fileDay(t, stateDir(t, deps), pooledLines...)
			if got := requestsRead(cfg, cfg.PoolRead); cfg.Pool.ID != "" || !slices.Equal(got, tt.opened) {
				t.Fatalf("the dashboard opened on %q, reading the requests %q; want no pool, reading %q", cfg.Pool.ID, got, tt.opened)
			}

			writeDepsConfig(t, deps, pooledConfig(false))
			again, err := cfg.Turn(t.Context(), "", watch.SamePool)
			if err != nil {
				t.Fatalf("Turn() of the same pool failed: %v", err)
			}
			if got, want := requestsRead(cfg, again.Read), []string{"p1", "p3"}; again.Pool.ID != "b2n8" || !slices.Equal(got, want) {
				t.Errorf("the config naming pools, the dashboard reads %q again, reading the requests %q; want Default, reading %q", again.Pool.ID, got, want)
			}
		})
	}
}

// A dashboard open across the move to pools takes the first pool afresh at
// each turn, as its ledger and the router do: opened before the move, its
// grounds naming none, a reserve given before the move has it make a ledger
// that summarises the day's work, every pool's; it follows the move to the
// first pool the accounts record notes, Default; and a pool put ahead of
// Default while it's open leaves its grounds, its pool's read and the
// ledger it makes on them on Default, work naming no pool still Default's.
// The ledger's first pool is in its summaries alone: a day's lines name the
// pool they were routed in, or none.
func TestADashboardOpenAcrossTheMoveKeepsToTheFirstPoolTheRecordNotes(t *testing.T) {
	deps := testDeps(map[string]string{"XDG_STATE_HOME": shortTempDir(t)}, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	writeDepsConfig(t, deps, unpooledConfig)
	state := stateDir(t, deps)
	markDone(t, state, idsMark)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	if cfg.LedgerGrounds.First != "" {
		t.Fatalf("opened before the move, the dashboard's ledger is grounded on the first pool %q, want none", cfg.LedgerGrounds.First)
	}

	fileDay(t, state, pooledLines...)
	writeDepsConfig(t, deps, unpooledConfig+"reserve = 0.05\n")
	reserved, err := cfg.Turn(t.Context(), "", watch.SamePool)
	if err != nil {
		t.Fatalf("a reserve given before the move, Turn() of the same pool failed: %v", err)
	}
	if got, want := sessionsCounted(reserved.NewLedger(), reserved.Read), sortedSessions(sessionA, sessionB, sessionC); reserved.Grounds.Same(cfg.LedgerGrounds) || !slices.Equal(got, want) {
		t.Fatalf("a reserve given before the move, the ledger's grounds the same %v, the one made on them counting the sessions %q; want them changed, counting every pool's %q",
			reserved.Grounds.Same(cfg.LedgerGrounds), got, want)
	}

	markMoved(t, state)
	client := "\n[[pool]]\nid    = \"c5d6\"\nlabel = \"Client\"\ndirs  = [\"/work/client\"]\n"
	for _, tt := range []struct{ name, config string }{
		{name: "the move followed", config: pooledConfig(false)},
		{name: "Client put ahead of Default", config: strings.Replace(pooledConfig(false), "\n[[pool]]\nid    = \"b2n8\"", client+"\n[[pool]]\nid    = \"b2n8\"", 1)},
	} {
		writeDepsConfig(t, deps, tt.config)
		again, err := cfg.Turn(t.Context(), "b2n8", watch.SamePool)
		if err != nil {
			t.Fatalf("%s, Turn() of the same pool failed: %v", tt.name, err)
		}
		if got, want := requestsRead(cfg, again.Read), []string{"p1", "p3"}; again.Pool.ID != "b2n8" || again.Read.First != "b2n8" || again.Grounds.First != "b2n8" || !slices.Equal(got, want) {
			t.Errorf("%s, the dashboard shows %q, its read of the first pool %q, its ledger grounded on %q, reading %q; want Default, all of Default, reading %q",
				tt.name, again.Pool.ID, again.Read.First, again.Grounds.First, got, want)
		}
		if got, want := sessionsCounted(again.NewLedger(), again.Read), sortedSessions(sessionA, sessionC); !slices.Equal(got, want) {
			t.Errorf("%s, the ledger made on the turn's grounds counts the sessions %q in Default, want %q, work in no pool among them", tt.name, got, want)
		}
	}
}

// sessionsCounted are the sessions the summary of the day of pooledLines
// that the ledger l makes counts in the pool read reads, sorted, each once.
func sessionsCounted(l views.Ledger, read views.PoolRead) []string {
	var ids []string
	for _, day := range read.Dates(l).DaysOn([]string{"2026-10-07"}) {
		for _, a := range day.Accounts {
			ids = append(ids, a.SessionIDs...)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// sortedSessions are the session ids given, sorted.
func sortedSessions(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

// requestsRead are the requests of the day of pooledLines that read keeps,
// of the ledger the dashboard given cfg reads.
func requestsRead(cfg *watch.Config, read views.PoolRead) []string {
	var ids []string
	for _, h := range read.Held(cfg.Ledger.Day("2026-10-07")) {
		ids = append(ids, h.Request)
	}
	return ids
}

// The dashboard reads the request ledger of the pool it shows alone, by its
// id, as named and as P turns to the next, work naming no pool the first
// pool's: none where it shows none, every pool being fenced and none named,
// or the pools not to be told; and the pool's alone where the config has
// one pool, whatever the accounts record keeps beside it.
func TestTheDashboardReadsTheLedgerOfThePoolItShowsAlone(t *testing.T) {
	tests := []struct {
		name string
		deps func(t *testing.T) cli.Deps
		args []string
		turn string
		want []string
	}{
		{name: "none named: the unfenced pool", deps: func(t *testing.T) cli.Deps { return pooledDeps(t, false) }, want: []string{"p1", "p3"}},
		{name: "one named", deps: func(t *testing.T) cli.Deps { return pooledDeps(t, false) }, args: []string{"--pool", "q7mt"}, want: []string{"p2"}},
		{name: "turned to the next", deps: func(t *testing.T) cli.Deps { return pooledDeps(t, false) }, args: []string{"--pool", "q7mt"}, turn: "q7mt", want: []string{"p1", "p3"}},
		{name: "every pool fenced and none named", deps: func(t *testing.T) cli.Deps { return pooledDeps(t, true) }},
		{name: "the pools not to be told", deps: func(t *testing.T) cli.Deps { return unpooledDeps(t, unpooledConfig, false) }},
		{name: "one pool, none other kept", deps: func(t *testing.T) cli.Deps {
			deps := pooledDeps(t, false)
			onePoolLeft(t, deps)
			return deps
		}, want: []string{"p1", "p3"}},
		{name: "one pool, a removed one kept", deps: func(t *testing.T) cli.Deps {
			deps := pooledDeps(t, false)
			onePoolLeft(t, deps, acmeRemovedEntry)
			return deps
		}, want: []string{"p1", "p3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := tt.deps(t)
			cfg := recordWatch(t, &deps)
			if got := run(t, deps, append([]string{"dashboard"}, tt.args...)...); got != (result{}) {
				t.Fatalf("switchboard dashboard %s = %+v, want exit status 0 and nothing printed", strings.Join(tt.args, " "), got)
			}
			read := cfg.PoolRead
			if tt.turn != "" {
				turned, err := cfg.Turn(t.Context(), tt.turn, watch.NextPool)
				if err != nil {
					t.Fatal(err)
				}
				read = turned.Read
			}
			if got := requestsRead(cfg, read); !slices.Equal(got, tt.want) {
				t.Errorf("the dashboard reads the requests %q of the ledger, want %q", got, tt.want)
			}
		})
	}
}

// A dashboard opened on one pool reads its pool's work of the request
// ledger by its id, so a pool added to the config while it's open, an
// account moved into it, and that pool's lines filed since, never show in
// the pool it shows: neither through the read it opened with, nor through
// the one it reads the pool shown again with as the config changes, placing
// the accounts otherwise, so the view starts afresh on it.
func TestADashboardOpenedOnOnePoolNeverReadsAPoolAddedSince(t *testing.T) {
	deps := pooledDeps(t, false)
	onePoolLeft(t, deps)
	state := stateDir(t, deps)
	fileDay(t, state, pooledLines[0], pooledLines[2])
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}

	writeDepsConfig(t, deps, pooledConfig(false))
	err := identity.NewFile(state).Update(func(r *identity.Record) error {
		r.Pools = append(r.Pools, identity.PoolEntry{ID: "q7mt", Added: testNow, Here: true})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fileDay(t, state, pooledLines...)
	if got, want := requestsRead(cfg, cfg.PoolRead), []string{"p1", "p3"}; !slices.Equal(got, want) {
		t.Errorf("the dashboard reads the requests %q of the ledger, Acme added since, want %q, Default's alone", got, want)
	}
	again, err := cfg.Turn(t.Context(), "b2n8", watch.SamePool)
	if err != nil {
		t.Fatalf("Turn() of the same pool failed: %v", err)
	}
	if got, want := requestsRead(cfg, again.Read), []string{"p1", "p3"}; again.Placed.Same(cfg.Placed) || !slices.Equal(got, want) {
		t.Errorf("read again once Acme is added, placed alike %v, the dashboard reads the requests %q of the ledger; want placed otherwise, %q, Default's alone",
			again.Placed.Same(cfg.Placed), got, want)
	}
}

// The dashboard reads the pool it shows again as the config changes: that
// pool, as the config now has it, its label and pools counted, and every
// account's placing in the pools, which a change to any of them changes;
// where it's gone, the first, as P would turn to; and of none shown, as
// where every pool is fenced, none again. Each is stamped as the config file
// stood, which a change to it changes.
func TestTheDashboardReadsThePoolItShowsAgainAsTheConfigChanges(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "q7mt"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool q7mt = %+v, want exit status 0 and nothing printed", got)
	}
	if cfg.ConfigStamp == nil || !cfg.Stamp.There() || !cfg.ConfigStamp().Same(cfg.Stamp) {
		t.Fatal("the dashboard is stamped otherwise than the config stands, or not at all; want them the same, as nothing changed")
	}
	again := func(from string) watch.Pooled {
		t.Helper()
		pooled, err := cfg.Turn(t.Context(), from, watch.SamePool)
		if err != nil {
			t.Fatalf("Turn(%q) of the same pool failed: %v", from, err)
		}
		return pooled
	}
	if got := again("q7mt"); !got.Placed.Same(cfg.Placed) {
		t.Error("read again with nothing changed, the accounts are placed otherwise; want them placed alike")
	}

	writeDepsConfig(t, deps, strings.Replace(pooledConfig(false), `label = "Acme"`, `label = "Acme Corp"`, 1))
	modified(t, configAt(t, deps), time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC))
	got := again("q7mt")
	if got.Pool.ID != "q7mt" || got.Pool.Label != "Acme Corp" || got.Placed.Same(cfg.Placed) {
		t.Errorf("read again once Acme is relabelled, the dashboard shows %+v, placed alike %v; want Acme Corp, placed otherwise", got.Pool, got.Placed.Same(cfg.Placed))
	}
	if !got.Stamp.Same(cfg.ConfigStamp()) || got.Stamp.Same(cfg.Stamp) {
		t.Error("read again once Acme is relabelled, the dashboard is stamped as before, or otherwise than the config stands; want it stamped afresh")
	}
	if got := again("x9z9"); got.Pool.ID != "b2n8" {
		t.Errorf("read again once the pool shown is gone, the dashboard shows %q, want Default, the first", got.Pool.ID)
	}

	fenced := pooledDeps(t, true)
	none := recordWatch(t, &fenced)
	if got := run(t, fenced, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	if got, err := none.Turn(t.Context(), "", watch.SamePool); err != nil || got.Pool.ID != "" || got.Pool.Pools != 2 {
		t.Errorf("with every pool fenced, none read again shows %+v, %v; want none again, of two", got.Pool, err)
	}
}

// What the dashboard works out of the config follows it at a turn: the
// accounts removed, so one taken out while it's open is named by the label
// the accounts record keeps.
func TestATurnWorksOutTheAccountsRemovedOfTheConfigRead(t *testing.T) {
	deps := pooledDeps(t, false)
	err := identity.NewFile(stateDir(t, deps)).Update(func(r *identity.Record) error {
		r.Accounts = []identity.Entry{{ID: "work", Label: "Work", Added: testNow, Pool: "q7mt"}, {ID: "side", Label: "Side", Added: testNow, Pool: "b2n8"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "q7mt"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool q7mt = %+v, want exit status 0 and nothing printed", got)
	}
	if len(cfg.Removed) != 0 {
		t.Fatalf("the dashboard opened with the accounts removed %+v, want none", cfg.Removed)
	}

	writeDepsConfig(t, deps, strings.Replace(pooledConfig(false), "[[account]]\nid    = \"side\"\nlabel = \"Side\"\npool  = \"b2n8\"\n", "", 1))
	turned, err := cfg.Turn(t.Context(), "q7mt", watch.SamePool)
	if err != nil {
		t.Fatalf("Turn() of the same pool failed: %v", err)
	}
	if len(turned.Removed) != 1 || turned.Removed[0].ID != "side" || turned.Removed[0].Label != "Side" {
		t.Errorf("side taken out of the config, the turn has the accounts removed %+v, want side, labelled Side", turned.Removed)
	}
}

// sideAtItsCap are today's readings of side's 5-hour window at 95%, from
// 10:00 to the clock's 13:12: 192 minutes at a cap of 90%.
var sideAtItsCap = []readings.Reading{
	{At: october(7, 10, 0, 0), Account: "side", Key: "5h", Utilization: 0.95, ResetsAt: october(7, 15, 0, 0), Status: quota.StatusAllowed},
}

// sideMinutesAtCap are the minutes side spent at its cap today, as ledger
// summarises today.
func sideMinutesAtCap(t *testing.T, ledger views.Ledger) int {
	t.Helper()
	days := ledger.Days(october(7, 0, 0, 0))
	for _, a := range days[len(days)-1].Accounts {
		if a.Account == "side" && a.MinutesAtCap != nil {
			return *a.MinutesAtCap
		}
	}
	t.Fatalf("today's summary has no minutes at its cap of side's: %+v", days[len(days)-1].Accounts)
	return 0
}

// A turn's ledger is made on the grounds of the config read: on the same
// caps and first pool as the one in use, as at P with nothing of them
// changed, the turn's grounds are the same, for the watch to keep the one in
// use; a reserve changed, the ledger it makes summarises today with the new
// cap.
func TestATurnsLedgerIsMadeOnTheCapsOfTheConfigRead(t *testing.T) {
	deps := pooledDeps(t, false)
	writeReadings(t, readings.Dir(stateDir(t, deps)), sideAtItsCap...)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "b2n8"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool b2n8 = %+v, want exit status 0 and nothing printed", got)
	}
	if got := sideMinutesAtCap(t, cfg.Ledger); got != 0 {
		t.Errorf("with no reserve, side spent %d minutes at its cap today, want none: it has no cap", got)
	}
	turned, err := cfg.Turn(t.Context(), "b2n8", watch.NextPool)
	if err != nil {
		t.Fatalf("Turn() failed: %v", err)
	}
	if !turned.Grounds.Same(cfg.LedgerGrounds) {
		t.Errorf("P with no cap changed has the ledger's grounds %+v, want those the dashboard opened with, %+v", turned.Grounds, cfg.LedgerGrounds)
	}

	writeDepsConfig(t, deps, strings.Replace(pooledConfig(false), "label = \"Side\"\n", "label = \"Side\"\nreserve = 0.1\n", 1))
	turned, err = cfg.Turn(t.Context(), "b2n8", watch.SamePool)
	if err != nil {
		t.Fatalf("Turn() of the same pool failed: %v", err)
	}
	if turned.Grounds.Same(cfg.LedgerGrounds) {
		t.Fatal("side given a reserve, the ledger's grounds are those the dashboard opened with, want others")
	}
	if got := sideMinutesAtCap(t, turned.NewLedger()); got != 192 {
		t.Errorf("side given a reserve of 10%%, the turn's ledger has it %d minutes at its cap today, want 192", got)
	}
}

// The dashboard holds no ledger beside the one its model reads: once the
// model lets go of the one it opened with, as a turn on other grounds has it
// do, nothing the dashboard was given, its Turn among them, still holds it.
func TestTheDashboardHoldsNoLedgerBesideTheModels(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "b2n8"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool b2n8 = %+v, want exit status 0 and nothing printed", got)
	}
	follower, ok := cfg.Ledger.(*ledger.Follower)
	if !ok {
		t.Fatalf("the dashboard's ledger is %T, want a follower of the state directory's", cfg.Ledger)
	}
	opened := weak.Make(follower)
	cfg.Ledger = nil
	runtime.KeepAlive(follower)
	runtime.GC()
	if opened.Value() != nil {
		t.Error("the ledger the dashboard opened with is still held once the model lets it go, want it let go")
	}
	if _, err := cfg.Turn(t.Context(), "b2n8", watch.SamePool); err != nil {
		t.Errorf("Turn() failed once the ledger was let go: %v", err)
	}
}

// The dashboard stamps the config file it reads: the one --config names,
// whose change it follows, and not the one config.Path finds.
func TestTheDashboardStampsTheConfigFileItReads(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	named := filepath.Join(t.TempDir(), "named.toml")
	writeStateFile(t, filepath.Dir(named), filepath.Base(named), []byte(pooledConfig(false)))
	if got := run(t, deps, "dashboard", "--config", named); got != (result{}) {
		t.Fatalf("switchboard dashboard --config = %+v, want exit status 0 and nothing printed", got)
	}
	if !cfg.Stamp.Same(filestate.Of(named)) {
		t.Fatal("the dashboard is stamped otherwise than the config --config names stands, want it stamped as that")
	}

	modified(t, configAt(t, deps), time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC))
	if !cfg.ConfigStamp().Same(cfg.Stamp) {
		t.Error("the config file config.Path finds changed, and the dashboard's stamp with it; want it unchanged, as it isn't the one read")
	}
	modified(t, named, time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC))
	if cfg.ConfigStamp().Same(cfg.Stamp) {
		t.Error("the config file --config names changed, and the dashboard's stamp didn't; want it to follow")
	}
}

// configAt is the path of deps' config file, as config.Path finds it.
func configAt(t *testing.T, deps cli.Deps) string {
	t.Helper()
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// modified sets the file at path as last modified at, as a change to it
// made then.
func modified(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

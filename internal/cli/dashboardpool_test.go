package cli_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
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

// The dashboard reads the pool it shows again as the config changes: that
// pool, as the config now has it, its label and pools counted; where it's
// gone, the one P would turn to; and of none shown, as where every pool is
// fenced, none again. Each is stamped as the config file stood, which a
// change to it changes.
func TestTheDashboardReadsThePoolItShowsAgainAsTheConfigChanges(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard", "--pool", "q7mt"); got != (result{}) {
		t.Fatalf("switchboard dashboard --pool q7mt = %+v, want exit status 0 and nothing printed", got)
	}
	if cfg.ConfigStamp == nil || cfg.Stamp == "" || cfg.ConfigStamp() != cfg.Stamp {
		t.Fatalf("the dashboard is stamped %q, the config standing %q; want them the same, as nothing changed", cfg.Stamp, cfg.ConfigStamp())
	}
	again := func(from string) watch.Pooled {
		t.Helper()
		pooled, err := cfg.Turn(t.Context(), from, watch.SamePool)
		if err != nil {
			t.Fatalf("Turn(%q) of the same pool failed: %v", from, err)
		}
		return pooled
	}

	writeDepsConfig(t, deps, strings.Replace(pooledConfig(false), `label = "Acme"`, `label = "Acme Corp"`, 1))
	if got := again("q7mt"); got.Pool.ID != "q7mt" || got.Pool.Label != "Acme Corp" || got.Stamp != cfg.ConfigStamp() || got.Stamp == cfg.Stamp {
		t.Errorf("read again once Acme is relabelled, the dashboard shows %+v, stamped %q; want Acme Corp, stamped afresh", got.Pool, got.Stamp)
	}
	if got := again("x9z9"); got.Pool.ID != "b2n8" {
		t.Errorf("read again once the pool shown is gone, the dashboard shows %q, want Default, as P would turn to", got.Pool.ID)
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

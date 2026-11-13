package cli_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// sessionD is a session of Client's, another Mac's pool.
const sessionD = "7c1d2e3f-0005-4a00-8000-000000000005"

// acmeRemoved is pooledConfig once Acme is removed, work moved to Default
// first, the one pool left.
const acmeRemoved = `[[account]]
id    = "work"
label = "Work"
pool  = "b2n8"

[[account]]
id    = "side"
label = "Side"
pool  = "b2n8"

[[pool]]
id    = "b2n8"
label = "Default"
`

// keptDeps are pooledDeps once Acme is removed, as acmeRemoved has it, the
// accounts record keeping it, labelled, removed on 6 October, as this Mac's
// config last wrote it; and keeping Client, f4xk, another Mac's, as an import
// joins its record in, with its account c1nt, in no pool of this Mac's, as
// the import leaves it, and a request and a session's start of its own in
// Client filed today.
func keptDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps := pooledDeps(t, false)
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(acmeRemoved))
	state := stateDir(t, deps)
	err = identity.NewFile(state).Update(func(r *identity.Record) error {
		r.Pools = []identity.PoolEntry{
			{ID: "b2n8", Added: testNow, First: true, Here: true},
			{ID: "q7mt", Label: "Acme", Added: testNow, Removed: october(6, 18, 0, 0), Written: identity.PoolWritten{Label: "Acme", PrimaryKnown: true}},
			{ID: "f4xk", Label: "Client", Added: testNow},
		}
		r.Accounts = append(r.Accounts, identity.Entry{ID: "c1nt", Label: "Client 1", Added: testNow})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	client := ledger.Line{At: october(7, 12, 0, 0).UTC(), Request: "p4", Kind: ledger.KindMessage, Session: sessionD, Model: "claude-opus-5-5",
		Account: "c1nt", Pool: "f4xk", Reason: "new", Status: 200, Attempts: 1, TotalMS: 100, Usage: json.RawMessage(unknownUsage)}
	data, err := json.Marshal(client)
	if err != nil {
		t.Fatal(err)
	}
	day := readFile(t, filepath.Join(ledger.Dir(state), "requests-2026-10-07.jsonl"))
	writeStateFile(t, ledger.Dir(state), "requests-2026-10-07.jsonl", append([]byte(day), append(data, '\n')...))
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 6, status.Event{At: october(7, 12, 0, 0), Kind: status.EventStarted, Session: sessionD, Account: "c1nt", Pool: "f4xk", Reason: status.ReasonNew}))
	return deps
}

// The verbs that read the recorded files read a pool the accounts record
// keeps but the config no longer has, removed, or another Mac's from an
// import, by its id: its work as it was recorded, under its id alone, never
// another pool's. Without --pool, nothing changes: the pool the verbs act on
// is read, and neither kept pool's work is folded into it.
func TestAPoolTheRecordKeepsIsReadByItsID(t *testing.T) {
	deps := keptDeps(t)
	tests := []struct {
		name  string
		args  []string
		field string
		want  []string
	}{
		{name: "requests in Acme, removed", args: []string{"requests", "--pool", "q7mt"}, field: "request", want: []string{"p2"}},
		{name: "requests in Client, another Mac's", args: []string{"requests", "--pool", "f4xk"}, field: "request", want: []string{"p4"}},
		{name: "requests, none named: Default's alone", args: []string{"requests"}, field: "request", want: []string{"p1", "p3"}},
		{name: "events in Acme: its start, and the router's own", args: []string{"events", "--pool", "q7mt"}, field: "kind", want: []string{"started", "restart"}},
		{name: "events in Client: its start, and the router's own", args: []string{"events", "--pool", "f4xk"}, field: "kind", want: []string{"restart", "started"}},
		{name: "events, none named: Default's alone", args: []string{"events"}, field: "kind", want: []string{"started", "limit", "pin", "restart"}},
		{name: "requests in Acme label it as the record keeps it", args: []string{"requests", "--pool", "q7mt"}, field: "pool_label", want: []string{"Acme"}},
		{name: "requests in Client label it as the record keeps it", args: []string{"requests", "--pool", "f4xk"}, field: "pool_label", want: []string{"Client"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, deps, append(tt.args, "--json")...)
			if values := fieldsOf(t, got, tt.field); got.code != 0 || !slices.Equal(values, tt.want) {
				t.Errorf("switchboard %s --json printed %s %q (%+v), want %q", strings.Join(tt.args, " "), tt.field, values, got, tt.want)
			}
		})
	}

	// c1nt, in no pool of this Mac's, shows in every pool read, without
	// work in Acme.
	history := []struct {
		args []string
		want []string
	}{
		{args: []string{"--pool", "q7mt"}, want: []string{"c1nt q7mt Acme ", "work q7mt Acme s"}},
		{args: []string{"--pool", "f4xk"}, want: []string{"c1nt f4xk Client s"}},
	}
	for _, tt := range history {
		got := run(t, deps, append([]string{"history", "--json"}, tt.args...)...)
		if accounts := historyAccounts(t, got); !slices.Equal(accounts, tt.want) {
			t.Errorf("switchboard history --json %s gave the accounts %q, want %q", strings.Join(tt.args, " "), accounts, tt.want)
		}
	}

	sessions := []struct {
		args []string
		want []string
	}{
		{args: []string{"--pool", "q7mt"}, want: []string{status.ShortID(sessionB)}},
		{args: []string{"--pool", "f4xk"}, want: []string{status.ShortID(sessionD)}},
	}
	for _, tt := range sessions {
		got := run(t, deps, append([]string{"sessions", "--json"}, tt.args...)...)
		if ids := sessionsListed(t, got); !slices.Equal(ids, tt.want) || got.stderr != "" {
			t.Errorf("switchboard sessions --json %s listed %q, said %q; want %q, saying nothing of the router, which runs nothing in it", strings.Join(tt.args, " "), ids, got.stderr, tt.want)
		}
	}
}

// The text of a verb reading a kept pool names it by the record's label,
// marked: when it was removed, else that it isn't configured here.
func TestAPoolTheRecordKeepsIsNamedMarkedAsNotConfigured(t *testing.T) {
	deps := keptDeps(t)
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"requests", "--pool", "q7mt"}, want: "pool Acme (removed 6 Oct)\n\n"},
		{args: []string{"requests", "--pool", "f4xk"}, want: "pool Client (not configured here)\n\n"},
		{args: []string{"events", "--pool", "q7mt"}, want: "pool Acme (removed 6 Oct)\n\n"},
		{args: []string{"sessions", "--pool", "f4xk"}, want: "pool Client (not configured here)\n\n"},
		{args: []string{"history", "--pool", "q7mt", "--since", "2026-10-07"}, want: "pool Acme (removed 6 Oct)\n\n"},
	}
	for _, tt := range tests {
		got := run(t, deps, append(tt.args, "--pretty")...)
		if got.code != 0 || !strings.HasPrefix(got.stdout, tt.want) {
			t.Errorf("switchboard %s --pretty = %+v, want it to start %q", strings.Join(tt.args, " "), got, tt.want)
		}
	}
}

// The verbs that read the router refuse a pool it doesn't route, kept or
// not, as the router does, whether it runs or not; and a pool the record
// doesn't keep either is refused by every verb, naming the pools configured.
func TestAPoolNotConfiguredIsRefusedByTheVerbsReadingTheRouter(t *testing.T) {
	deps := keptDeps(t)
	tests := [][]string{
		{"status", "--json", "--pool", "q7mt"},
		{"usage", "--json", "--pool", "f4xk"},
		{"requests", "--json", "--pool", "x9z9"},
		{"events", "--json", "--pool", "x9z9"},
	}
	for _, args := range tests {
		checkNoPool(t, run(t, deps, args...), args)
	}

	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	err := identity.NewFile(srv.state).Update(func(r *identity.Record) error {
		r.Pools = append(r.Pools, identity.PoolEntry{ID: "q7mt", Label: "Acme", Added: testNow, Removed: october(6, 18, 0, 0)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.start(t)
	for _, args := range [][]string{{"status", "--json", "--pool", "q7mt"}, {"usage", "--json", "--pool", "q7mt"}, {"pin", "auto", "--pool", "q7mt"}} {
		checkNoPool(t, run(t, srv.deps, args...), args)
	}
}

// checkNoPool checks the verb run with args was refused, naming the pool
// --pool named, the last of them, as no pool, and Default, b2n8, the one to
// name.
func checkNoPool(t *testing.T, got result, args []string) {
	t.Helper()
	if got.code == 0 || got.stdout != "" || !strings.Contains(got.stderr, `there's no pool "`+args[len(args)-1]+`": name b2n8 (Default)`) {
		t.Errorf("switchboard %s = %+v, want it refused, naming Default", strings.Join(args, " "), got)
	}
}

// Once the first pool is emptied and removed, its history, the work from
// before pools the move gave its id among it, is read with --pool by its id;
// work filed naming no pool since is the config's first's, as the router
// routes it, never the removed pool's.
func TestTheFirstPoolRemovedIsReadByItsID(t *testing.T) {
	deps := pooledDeps(t, false)
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte("[[account]]\nid = \"work\"\nlabel = \"Work\"\npool = \"q7mt\"\n\n"+
		"[[account]]\nid = \"side\"\nlabel = \"Side\"\npool = \"q7mt\"\n\n[[pool]]\nid = \"q7mt\"\nlabel = \"Acme\"\n"))
	err = identity.NewFile(stateDir(t, deps)).Update(func(r *identity.Record) error {
		r.Pools[0].Label, r.Pools[0].Removed = "Default", october(6, 18, 0, 0)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args []string
		want []string
	}{
		{args: []string{"requests", "--pool", "b2n8"}, want: []string{"p3"}},
		{args: []string{"requests"}, want: []string{"p1", "p2"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append(tt.args, "--json")...)
		if values := fieldsOf(t, got, "request"); got.code != 0 || !slices.Equal(values, tt.want) {
			t.Errorf("switchboard %s --json printed requests %q (%+v), want %q", strings.Join(tt.args, " "), values, got, tt.want)
		}
	}
}

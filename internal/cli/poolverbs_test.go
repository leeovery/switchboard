package cli_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/status"
)

// unfencedBesideFenced are fencedAccounts with Default fenced to no
// directory: the pool the verbs act on.
const unfencedBesideFenced = `
[[account]]
id    = "work"
label = "Work"
pool  = "q7mt"

[[account]]
id    = "personal"
label = "Personal"
pool  = "b2n8"

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

// everyPoolFenced is what a verb says, asking for --pool, where every pool
// of fencedAccounts is fenced and none is named.
const everyPoolFenced = "every pool is fenced: name a pool, b2n8 (Default) or q7mt (Acme), with --pool"

// documentRead is what a status document read names: its pool, and its
// accounts' ids.
type documentRead struct {
	pool     string
	accounts []string
}

// readDocument reads the pool and the accounts of the status document a
// verb printed as JSON.
func readDocument(t *testing.T, got result) documentRead {
	t.Helper()
	var doc status.Document
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("%+v doesn't print a status document: %v", got, err)
	}
	read := documentRead{pool: doc.PoolID}
	for _, a := range doc.Accounts {
		read.accounts = append(read.accounts, a.ID)
	}
	return read
}

func TestTheReadingVerbsReadOnePoolsAccountsByProbingWhileTheRouterIsDown(t *testing.T) {
	tests := []struct {
		name     string
		accounts string
		args     []string
		want     documentRead
		needs    bool
	}{
		{name: "one pool, none named: the only one", accounts: threeInFirstPool, want: documentRead{pool: "b2n8", accounts: []string{"work", "personal", "side"}}},
		{name: "one named", accounts: threeInFirstPool, args: []string{"--pool", "b2n8"}, want: documentRead{pool: "b2n8", accounts: []string{"work", "personal", "side"}}},
		{name: "an unfenced pool beside a fenced one, none named: the unfenced", accounts: unfencedBesideFenced, want: documentRead{pool: "b2n8", accounts: []string{"personal", "side"}}},
		{name: "an unfenced pool beside a fenced one, the fenced named", accounts: unfencedBesideFenced, args: []string{"--pool", "q7mt"}, want: documentRead{pool: "q7mt", accounts: []string{"work"}}},
		{name: "every pool fenced, one named", accounts: fencedAccounts, args: []string{"--pool", "q7mt"}, want: documentRead{pool: "q7mt", accounts: []string{"work"}}},
		{name: "every pool fenced, none named", accounts: fencedAccounts, needs: true},
	}
	for _, tt := range tests {
		for _, verb := range []string{"status", "usage"} {
			t.Run(verb+", "+tt.name, func(t *testing.T) {
				srv := newServeSetup(t, fakeClaudeAPI(t), nil)
				srv.accounts = tt.accounts
				srv.writeConfig(t)

				got := run(t, srv.deps, slices.Concat([]string{verb, "--json"}, tt.args)...)
				if tt.needs {
					if got.code == 0 || got.stdout != "" || !strings.Contains(got.stderr, everyPoolFenced) {
						t.Errorf("switchboard %s --json = %+v, want it to ask for --pool, naming the pools, and print nothing", verb, got)
					}
					return
				}
				if read := readDocument(t, got); read.pool != tt.want.pool || !slices.Equal(read.accounts, tt.want.accounts) {
					t.Errorf("switchboard %s --json %s read %+v, want %+v", verb, strings.Join(tt.args, " "), read, tt.want)
				}
			})
		}
	}
}

func TestAVerbNeverChoosesItsPoolByTheDirectoryItsRunIn(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	here, err := srv.deps.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	srv.accounts = strings.Replace(unfencedBesideFenced, `"/work/acme"`, fmt.Sprintf("%q", here), 1)
	srv.writeConfig(t)

	if read := readDocument(t, run(t, srv.deps, "status", "--json")); read.pool != "b2n8" {
		t.Errorf("switchboard status --json, run inside Acme's directory, read pool %q, want the unfenced pool, b2n8", read.pool)
	}
}

func TestTheReadingVerbsAskTheRouterForThePoolNamed(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = unfencedBesideFenced
	srv.writeConfig(t)
	srv.start(t)

	tests := []struct {
		args []string
		want documentRead
	}{
		{args: []string{"status", "--json", "--pool", "q7mt"}, want: documentRead{pool: "q7mt", accounts: []string{"work"}}},
		{args: []string{"usage", "--json", "--pool", "q7mt"}, want: documentRead{pool: "q7mt", accounts: []string{"work"}}},
		{args: []string{"status", "--json"}, want: documentRead{pool: "b2n8", accounts: []string{"personal", "side"}}},
		{args: []string{"usage", "--json"}, want: documentRead{pool: "b2n8", accounts: []string{"personal", "side"}}},
	}
	for _, tt := range tests {
		if read := readDocument(t, run(t, srv.deps, tt.args...)); read.pool != tt.want.pool || !slices.Equal(read.accounts, tt.want.accounts) {
			t.Errorf("switchboard %s read %+v, want %+v", strings.Join(tt.args, " "), read, tt.want)
		}
	}
}

func TestStatusNamesThePoolItShowsOnceThereIsASecond(t *testing.T) {
	tests := []struct {
		name, accounts, args, want string
	}{
		{name: "two pools", accounts: unfencedBesideFenced, args: "--pool q7mt", want: "pool Acme\n\nwork · Work (primary)\n"},
		{name: "one pool", accounts: threeInFirstPool, want: "work · Work (primary)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), nil)
			srv.accounts = tt.accounts
			srv.writeConfig(t)

			got := onATerminal.run(t, srv.deps, append([]string{"status"}, strings.Fields(tt.args)...)...)
			if !strings.HasPrefix(got.stdout, tt.want) {
				t.Errorf("switchboard status %s printed\n%s\nwant it to start\n%s", tt.args, got.stdout, tt.want)
			}
		})
	}
}

func TestPinAutoClearsThePinOfThePoolNamedElseOfTheOneTheVerbsActOn(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = unfencedBesideFenced
	srv.writeConfig(t)
	srv.start(t)

	// Each pool keeps its own pin: clearing one leaves the other's standing.
	steps := []struct {
		args       []string
		said       string
		acme, dflt []string
	}{
		{args: []string{"pin", "work"}, said: "new sessions in pool Acme go to work · Work\n", acme: []string{"work"}},
		{args: []string{"pin", "side"}, said: "new sessions in pool Default go to side · Side\n", acme: []string{"work"}, dflt: []string{"side"}},
		{args: []string{"pin", "auto"}, said: "routing automatically in pool Default\n", acme: []string{"work"}},
		{args: []string{"pin", "side"}, said: "new sessions in pool Default go to side · Side\n", acme: []string{"work"}, dflt: []string{"side"}},
		{args: []string{"pin", "auto", "--pool", "q7mt"}, said: "routing automatically in pool Acme\n", dflt: []string{"side"}},
	}
	for _, step := range steps {
		if got := run(t, srv.deps, step.args...); got != (result{stdout: step.said}) {
			t.Fatalf("switchboard %s = %+v, want it to say %q", strings.Join(step.args, " "), got, step.said)
		}
		if acme, dflt := poolPin(t, srv.deps, "q7mt"), poolPin(t, srv.deps, "b2n8"); !slices.Equal(acme, step.acme) || !slices.Equal(dflt, step.dflt) {
			t.Errorf("after switchboard %s, Acme pins %q and Default %q, want %q and %q", strings.Join(step.args, " "), acme, dflt, step.acme, step.dflt)
		}
	}
}

func TestPinAutoWithEveryPoolFencedAsksForThePool(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = fencedAccounts
	srv.writeConfig(t)
	srv.start(t)

	if got := run(t, srv.deps, "pin", "auto"); got.code == 0 || !strings.Contains(got.stderr, everyPoolFenced) {
		t.Errorf("switchboard pin auto = %+v, want it to ask for --pool, naming the pools", got)
	}
}

func TestPinningAccountsOfTwoPoolsIsRefused(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = unfencedBesideFenced
	srv.writeConfig(t)
	srv.start(t)

	got := run(t, srv.deps, "pin", "work", "side")
	if got.code == 0 || !strings.Contains(got.stderr, "pin accounts of one pool: work is in pool Acme, side is in pool Default") {
		t.Errorf("switchboard pin work side = %+v, want it refused, naming each account's pool", got)
	}
	if acme, dflt := poolPin(t, srv.deps, "q7mt"), poolPin(t, srv.deps, "b2n8"); acme != nil || dflt != nil {
		t.Errorf("Acme pins %q and Default %q, want neither pinned", acme, dflt)
	}
}

// poolPin returns the accounts the pin of the pool with the given id names,
// as status --json --pool reads it.
func poolPin(t *testing.T, deps cli.Deps, pool string) []string {
	t.Helper()
	var doc status.Document
	got := run(t, deps, "status", "--json", "--pool", pool)
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("switchboard status --json --pool %s = %+v: %v", pool, got, err)
	}
	return doc.Pin.Accounts
}

func TestPinTakesAPoolWithAutoAlone(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	for _, args := range [][]string{{"pin", "work", "--pool", "q7mt"}, {"pin", "auto", "--session", "18bb", "--pool", "q7mt"}} {
		if got := run(t, deps, args...); got.code == 0 || !strings.Contains(got.stderr, "--pool goes with auto alone") {
			t.Errorf("switchboard %s = %+v, want it refused", strings.Join(args, " "), got)
		}
	}
}

func TestStatusOfASessionTakesNoPool(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "status", "--session", "18bb", "--pool", "q7mt")
	if got.code == 0 || !strings.Contains(got.stderr, "takes no --pool") {
		t.Errorf("switchboard status --session 18bb --pool q7mt = %+v, want it refused", got)
	}
}

package cli_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// personalAndSidePooled are personalAndSide, personal in Default, unfenced,
// and side in Acme, fenced.
const personalAndSidePooled = `
[[account]]
id    = "personal"
label = "Personal"
pool  = "b2n8"

[[account]]
id    = "side"
label = "Side"
pool  = "q7mt"

[[pool]]
id    = "b2n8"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["/work/acme"]
`

// loadPooled loads the config at path, for its pools and accounts.
func loadPooled(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestPoolListsThePoolsTheirDirectoriesAccountsAndPrimaries(t *testing.T) {
	deps, _ := accountsDeps(t, "http://127.0.0.1:1", unfencedBesideFenced)

	if got, want := onATerminal.run(t, deps, "pool"), (result{stdout: "ID    LABEL    DIRS        ACCOUNTS\n" +
		"b2n8  Default  -           personal (primary), side\n" +
		"q7mt  Acme     /work/acme  work (primary)\n"}); got != want {
		t.Errorf("switchboard pool =\n%+v\nwant\n%+v", got, want)
	}
	want := `{
  "schema": 1,
  "pools": [
    {
      "pool": "b2n8",
      "pool_label": "Default",
      "dirs": [],
      "accounts": [
        {
          "account": "personal",
          "label": "Personal",
          "primary": true
        },
        {
          "account": "side",
          "label": "Side"
        }
      ]
    },
    {
      "pool": "q7mt",
      "pool_label": "Acme",
      "dirs": [
        "/work/acme"
      ],
      "accounts": [
        {
          "account": "work",
          "label": "Work",
          "primary": true
        }
      ]
    }
  ]
}
`
	if got := run(t, deps, "pool", "--json"); got != (result{stdout: want}) {
		t.Errorf("switchboard pool --json =\n%+v\nwant\n%s", got, want)
	}
}

// addedPool matches what pool add says, its id the first group.
var addedPool = regexp.MustCompile(`^added pool "Client" as ([a-z][a-z0-9]{3}), (.+)\n$`)

func TestPoolAddMakesAPoolUnderAnIDSwitchboardMakes(t *testing.T) {
	tests := []struct {
		name, accounts string
		args           []string
		said           string
		dirs           []string
	}{
		{name: "fenced", accounts: unfencedBesideFenced, args: []string{"/work/client", "/work/other"}, said: "fenced to /work/client and /work/other: sessions running in its directories keep the pool they started in, and its accounts, till they're resumed", dirs: []string{"/work/client", "/work/other"}},
		{name: "unfenced, where every pool is fenced", accounts: fencedAccounts, said: "unfenced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, "http://127.0.0.1:1", tt.accounts)

			got := run(t, deps, append([]string{"pool", "add", "Client"}, tt.args...)...)
			m := addedPool.FindStringSubmatch(got.stdout)
			if got.code != 0 || m == nil || m[2] != tt.said {
				t.Fatalf("switchboard pool add Client %s = %+v, want it added, %s", strings.Join(tt.args, " "), got, tt.said)
			}
			p, ok := loadPooled(t, path).Pools.ByID(m[1])
			if !ok || p.Label != "Client" || !slices.Equal(p.WrittenDirs, tt.dirs) {
				t.Errorf("the config has pool %s as %+v (%v), want it labelled Client, fenced to %q", m[1], p, ok, tt.dirs)
			}
			if m[1] == "b2n8" || m[1] == "q7mt" {
				t.Errorf("pool add made the id %s, which a pool has", m[1])
			}
		})
	}
}

func TestPoolAddAndEditRefuse(t *testing.T) {
	tests := []struct {
		name, accounts string
		args           []string
		want           string
	}{
		{name: "a second pool unfenced", accounts: unfencedBesideFenced, args: []string{"pool", "add", "Client"},
			want: "Default (b2n8) is unfenced already, and only one pool can be"},
		{name: "a label another pool shows, ignoring case", accounts: unfencedBesideFenced, args: []string{"pool", "add", "acme", "/work/client"},
			want: `"acme" is Acme (q7mt)'s label already, ignoring case`},
		{name: "directories inside another pool's", accounts: unfencedBesideFenced, args: []string{"pool", "add", "Client", "/work/acme/api"},
			want: "no directory may be the same as, inside or around another pool's"},
		{name: "a pool added to a config from before pools", accounts: personalAndSide, args: []string{"pool", "add", "Client", "/work/client"},
			want: "the config names no pools: the router writes them into it"},
		{name: "unfencing a second pool", accounts: unfencedBesideFenced, args: []string{"pool", "edit", "q7mt", "--unfence"},
			want: "Default (b2n8) is unfenced already, and only one pool can be"},
		{name: "relabelling as another pool shows", accounts: unfencedBesideFenced, args: []string{"pool", "edit", "q7mt", "--label", "DEFAULT"},
			want: `"DEFAULT" is Default (b2n8)'s label already`},
		{name: "fencing a pool whose directories are written across lines", args: []string{"pool", "edit", "q7mt", "--dirs", "/work/client"},
			accounts: strings.Replace(unfencedBesideFenced, `dirs  = ["/work/acme"]`, "dirs = [\n  \"/work/acme\",\n]", 1),
			want:     `pool "q7mt" writes its dirs across lines, which editing`},
		{name: "a pool that isn't configured", accounts: unfencedBesideFenced, args: []string{"pool", "edit", "z9z9", "--label", "Client"},
			want: `pool "z9z9" is not configured`},
		{name: "no flag", accounts: unfencedBesideFenced, args: []string{"pool", "edit", "q7mt"}, want: "give one or more of --label, --dirs or --unfence"},
		{name: "--dirs and --unfence", accounts: unfencedBesideFenced, args: []string{"pool", "edit", "q7mt", "--dirs", "/a", "--unfence"}, want: "takes no --dirs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, "http://127.0.0.1:1", tt.accounts)
			before := readFile(t, path)

			got := run(t, deps, tt.args...)
			if got.code == 0 || got.stdout != "" || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("switchboard %s = %+v, want it refused, saying %q", strings.Join(tt.args, " "), got, tt.want)
			}
			if readFile(t, path) != before {
				t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
			}
		})
	}
}

func TestPoolEditChangesALabelAndTheDirectories(t *testing.T) {
	tests := []struct {
		name, accounts string
		args           []string
		said           string
		label          string
		dirs           []string
	}{
		{name: "the label", accounts: unfencedBesideFenced, args: []string{"--label", "Client"}, said: `edited pool q7mt: label "Client" (was "Acme")`,
			label: "Client", dirs: []string{"/work/acme"}},
		{name: "the directories, replaced", accounts: unfencedBesideFenced, args: []string{"--dirs", "/work/client", "--dirs", "/work/other"},
			said: "edited pool q7mt: fenced to /work/client and /work/other (was fenced to /work/acme)", label: "Acme", dirs: []string{"/work/client", "/work/other"}},
		{name: "unfenced, where every other pool is fenced", accounts: fencedAccounts, args: []string{"--unfence"},
			said: "edited pool q7mt: unfenced (was fenced to /work/acme)", label: "Acme"},
		{name: "nothing new", accounts: unfencedBesideFenced, args: []string{"--label", "Acme"}, said: "pool q7mt has every value given already, so nothing changed",
			label: "Acme", dirs: []string{"/work/acme"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, "http://127.0.0.1:1", tt.accounts)

			if got := run(t, deps, append([]string{"pool", "edit", "q7mt"}, tt.args...)...); got != (result{stdout: tt.said + "\n"}) {
				t.Errorf("switchboard pool edit q7mt %s = %+v, want it to say %q", strings.Join(tt.args, " "), got, tt.said)
			}
			if p, _ := loadPooled(t, path).Pools.ByID("q7mt"); p.Label != tt.label || !slices.Equal(p.WrittenDirs, tt.dirs) {
				t.Errorf("the config has q7mt as %+v, want it labelled %s, fenced to %q", p, tt.label, tt.dirs)
			}
		})
	}
}

func TestPoolRemoveRemovesAPoolWithNoAccountsAlone(t *testing.T) {
	const empty = "\n[[pool]]\nid    = \"h4w2\"\nlabel = \"Handed back\"\ndirs  = [\"/work/old\"]\n"
	deps, path := accountsDeps(t, "http://127.0.0.1:1", unfencedBesideFenced+empty)

	if got := run(t, deps, "pool", "remove", "q7mt"); got.code == 0 || !strings.Contains(got.stderr, "has accounts: move each to another pool, or remove it, first") {
		t.Errorf("switchboard pool remove q7mt = %+v, want it refused while work is in it", got)
	}
	if got, want := run(t, deps, "pool", "remove", "h4w2"), (result{stdout: "removed pool h4w2 (Handed back): its history stays, read with --pool h4w2 by requests, history, sessions and events\n"}); got != want {
		t.Errorf("switchboard pool remove h4w2 = %+v, want %+v", got, want)
	}
	if pools := loadPooled(t, path).Pools.IDs(); !slices.Equal(pools, []string{"b2n8", "q7mt"}) {
		t.Errorf("the config has the pools %q, want b2n8 and q7mt", pools)
	}
}

// addedAccount matches what accounts add Work says, the account's id the
// first group, and what follows it the second.
var addedAccount = regexp.MustCompile(`^added "Work" as ([a-z][a-z0-9]{3,})(.*)\n$`)

// accountPool returns the pool of the account with the given id, as the
// config at path has it, and whether it's its pool's primary.
func accountPool(t *testing.T, path, id string) (pool string, primary bool) {
	t.Helper()
	a, ok := loadPooled(t, path).Accounts.ByID(id)
	if !ok {
		t.Fatalf("the config has no account %s", id)
	}
	return a.Pool, a.Primary
}

func TestAccountsAddPutsTheAccountInThePoolNamedElseTheOneTheVerbsActOn(t *testing.T) {
	tests := []struct {
		name     string
		accounts string
		args     []string
		said     string
		pool     string
		primary  bool
	}{
		{name: "none named: the unfenced pool", accounts: personalAndSidePooled, said: " in pool Default", pool: "b2n8"},
		{name: "one named", accounts: personalAndSidePooled, args: []string{"--pool", "q7mt"}, said: " in pool Acme", pool: "q7mt"},
		{name: "one named, its primary", accounts: personalAndSidePooled, args: []string{"--pool", "q7mt", "--primary"}, said: " in pool Acme, the primary", pool: "q7mt", primary: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newOrganizationsAPI(t)
			deps, path := accountsDeps(t, api.URL, tt.accounts)
			writeToken(t, deps, "personal", personalsToken)

			got := runWithInput(t, deps, madeWork+"\n", append([]string{"accounts", "add", "Work"}, tt.args...)...)
			m := addedAccount.FindStringSubmatch(got.stdout)
			if got.code != 0 || m == nil || m[2] != tt.said {
				t.Fatalf("switchboard accounts add Work %s = %+v, want it added%s", strings.Join(tt.args, " "), got, tt.said)
			}
			id := m[1]
			if pool, primary := accountPool(t, path, id); pool != tt.pool || primary != tt.primary {
				t.Errorf("the config has %s in %s, primary %v; want it in %s, primary %v", id, pool, primary, tt.pool, tt.primary)
			}
			if tt.primary {
				if _, primary := accountPool(t, path, "side"); primary {
					t.Errorf("side is still Acme's primary, want %s in its place", id)
				}
			}
		})
	}
}

func TestAccountsAddWithEveryPoolFencedAsksForThePool(t *testing.T) {
	api := newOrganizationsAPI(t)
	deps, path := accountsDeps(t, api.URL, strings.Replace(personalAndSidePooled, "label = \"Default\"\n", "label = \"Default\"\ndirs  = [\"/work/default\"]\n", 1))
	before := readFile(t, path)

	got := runWithInput(t, deps, madeWork+"\n", "accounts", "add", "Work")
	if got.code == 0 || !strings.Contains(got.stderr, "every pool is fenced: name a pool, b2n8 (Default) or q7mt (Acme), with --pool") {
		t.Errorf("switchboard accounts add Work = %+v, want it to ask for --pool, naming the pools", got)
	}
	if readFile(t, path) != before {
		t.Errorf("the config reads\n%s\nwant it as it was", readFile(t, path))
	}
}

func TestAccountsEditMovesAnAccountToAPoolAndMakesItItsPrimary(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		said    string
		primary bool
	}{
		{name: "moved, after the pool's primary", args: []string{"--pool", "b2n8"}, said: "edited side: pool Default (was Acme)"},
		{name: "moved, its primary", args: []string{"--pool", "b2n8", "--primary"}, said: "edited side: pool Default (was Acme), the primary in place of personal", primary: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, "http://127.0.0.1:1", personalAndSidePooled)

			got := run(t, deps, append([]string{"accounts", "edit", "side"}, tt.args...)...)
			if first, _, _ := strings.Cut(got.stdout, "\n"); got.code != 0 || first != tt.said {
				t.Errorf("switchboard accounts edit side %s = %+v, want it to say %q", strings.Join(tt.args, " "), got, tt.said)
			}
			if pool, primary := accountPool(t, path, "side"); pool != "b2n8" || primary != tt.primary {
				t.Errorf("the config has side in %s, primary %v; want it in b2n8, primary %v", pool, primary, tt.primary)
			}
			if _, primary := accountPool(t, path, "personal"); primary == tt.primary {
				t.Errorf("personal is Default's primary: %v, want %v", primary, !tt.primary)
			}
		})
	}
}

// Moving a pool's primary into another pool says which account is the
// primary of the pool it left now; and, where its token is usable, that the
// sessions running on it are refused, and which account's token they start
// on resumed, as a launch would choose it. Moving its last says the pool has
// none now, as removing it does.
func TestAccountsEditSaysWhatMovingAnAccountLeftOfItsPool(t *testing.T) {
	inAcme := strings.Replace(personalAndSidePooled, "[[pool]]", "[[account]]\nid    = \"work\"\nlabel = \"Work\"\npool  = \"q7mt\"\n\n"+
		"[[account]]\nid    = \"spare\"\nlabel = \"Spare\"\npool  = \"q7mt\"\n\n[[pool]]", 1)
	const now = "edited side: pool Default (was Acme)\nwork is pool Acme's primary now"
	const refused = "; the sessions running on side's token as Claude Code's own are refused now it has left the pool"
	tests := []struct {
		name, accounts string
		tokens         []string
		want           string
	}{
		{name: "its primary, each with a token", accounts: inAcme, tokens: []string{"side", "work", "spare"},
			want: now + refused + ": resume them, and they start on work's\n"},
		{name: "its primary, the next without one", accounts: inAcme, tokens: []string{"side", "spare"},
			want: now + refused + ": resume them, and they start on spare's, the first account there with a usable token, as work has none\n"},
		{name: "its primary, none left with one", accounts: inAcme, tokens: []string{"side"},
			want: now + refused + ", and no account left there has a usable token to resume them on (switchboard accounts token work)\n"},
		{name: "its primary, without a token, so none started on it", accounts: inAcme, tokens: []string{"work"}, want: now + "\n"},
		{name: "its last", accounts: personalAndSidePooled, tokens: []string{"side"}, want: "edited side: pool Default (was Acme)\n" +
			"pool Acme has no accounts now: its sessions aren't routed until one is added to it (switchboard accounts add <label> --pool q7mt)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := accountsDeps(t, "http://127.0.0.1:1", tt.accounts)
			for _, id := range tt.tokens {
				writeToken(t, deps, id, "test-token-"+id)
			}
			if got := run(t, deps, "accounts", "edit", "side", "--pool", "b2n8"); got != (result{stdout: tt.want}) {
				t.Errorf("switchboard accounts edit side --pool b2n8 = %+v, want it to print\n%s", got, tt.want)
			}
		})
	}
	deps, _ := accountsDeps(t, "http://127.0.0.1:1", inAcme)
	if got, want := run(t, deps, "accounts", "edit", "work", "--pool", "b2n8"), (result{stdout: "edited work: pool Default (was Acme)\n"}); got != want {
		t.Errorf("switchboard accounts edit work --pool b2n8 = %+v, want %+v: work wasn't Acme's primary, nor its last", got, want)
	}
}

// accounts edit given no flag names --pool among those it takes only where
// there are pools to move an account between, two or more.
func TestAccountsEditNamesPoolAmongItsFlagsOnceThereIsASecondPool(t *testing.T) {
	tests := []struct {
		name, accounts, want string
	}{
		{name: "two pools", accounts: personalAndSidePooled, want: "give one or more of --label, --plan, --primary, --reserve or --pool"},
		{name: "one pool", accounts: threeInFirstPool, want: "give one or more of --label, --plan, --primary or --reserve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := accountsDeps(t, "http://127.0.0.1:1", tt.accounts)
			if got := run(t, deps, "accounts", "edit", "side"); got.code == 0 || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("switchboard accounts edit side = %+v, want it to say %q", got, tt.want)
			}
		})
	}
}

// accounts add and edit take --pool "" as --pool not given, and edit checks
// a pool named before anything, naming the pools, as add does.
func TestAccountsTakeAPoolGivenEmptyAsNoneAndRefuseOneNotConfigured(t *testing.T) {
	api := newOrganizationsAPI(t)
	deps, path := accountsDeps(t, api.URL, personalAndSidePooled)
	writeToken(t, deps, "personal", personalsToken)

	got := runWithInput(t, deps, madeWork+"\n", "accounts", "add", "Work", "--pool", "")
	if m := addedAccount.FindStringSubmatch(got.stdout); got.code != 0 || m == nil || m[2] != " in pool Default" {
		t.Errorf("switchboard accounts add Work --pool \"\" = %+v, want it added in the unfenced pool, Default", got)
	}
	if got := run(t, deps, "accounts", "edit", "side", "--pool", "", "--label", "Spare"); got != (result{stdout: "edited side: label \"Spare\" (was \"Side\")\n"}) {
		t.Errorf("switchboard accounts edit side --pool \"\" --label Spare = %+v, want the label alone changed", got)
	}
	before := readFile(t, path)
	got = run(t, deps, "accounts", "edit", "side", "--pool", "x9z9")
	if got.code == 0 || !strings.Contains(got.stderr, `there's no pool "x9z9": name b2n8 (Default) or q7mt (Acme), with --pool`) {
		t.Errorf("switchboard accounts edit side --pool x9z9 = %+v, want it refused, naming the pools", got)
	}
	if readFile(t, path) != before {
		t.Errorf("the config reads\n%s\nwant it as it was", readFile(t, path))
	}
}

// An account added to a config that has lost its pools once the move is
// done, its pool written none, is said added in no pool, though the record
// keeps two.
func TestAccountsAddToAConfigThatLostItsPoolsNamesNoPool(t *testing.T) {
	api := newOrganizationsAPI(t)
	deps, _ := accountsDeps(t, api.URL, personalAndSide)
	writeToken(t, deps, "personal", personalsToken)
	state := stateDir(t, deps)
	markMoved(t, state)
	err := identity.NewFile(state).Update(func(r *identity.Record) error {
		r.Pools = []identity.PoolEntry{
			{ID: "b2n8", Added: testNow, First: true, Here: true, Written: identity.PoolWritten{Label: "Default"}},
			{ID: "q7mt", Added: testNow, Here: true, Written: identity.PoolWritten{Label: "Acme", Dirs: []string{"/work/acme"}}},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := runWithInput(t, deps, madeWork+"\n", "accounts", "add", "Work")
	if m := addedAccount.FindStringSubmatch(got.stdout); got.code != 0 || m == nil || m[2] != "" {
		t.Errorf("switchboard accounts add Work = %+v, want it added, naming no pool", got)
	}
}

// An account moved without --primary into a pool whose primary is its first,
// unmarked, after it in the file, leaves that one the primary, marked so.
func TestAccountsEditMovingAnAccountAheadOfThePoolsPrimaryMarksIt(t *testing.T) {
	deps, path := accountsDeps(t, "http://127.0.0.1:1", personalAndSidePooled)

	got := run(t, deps, "accounts", "edit", "personal", "--pool", "q7mt")
	if want := (result{stdout: "edited personal: pool Acme (was Default), marked side the primary there, which it stays\n" +
		"pool Default has no accounts now: its sessions aren't routed until one is added to it (switchboard accounts add <label> --pool b2n8)\n"}); got != want {
		t.Errorf("switchboard accounts edit personal --pool q7mt =\n%+v\nwant\n%+v", got, want)
	}
	if primary, _ := loadPooled(t, path).Accounts.PrimaryOf("q7mt"); primary.ID != "side" {
		t.Errorf("Acme's primary is %s, want side", primary.ID)
	}
	draft, err := config.Edit(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !draft.MarksPrimaryOf("side") || draft.MarksPrimaryOf("personal") {
		t.Errorf("the config reads\n%s\nwant side marked the primary, and personal not", readFile(t, path))
	}
}

// personalBesideAcme has personal in Default, unfenced, and side, its
// primary, and extra in Acme, fenced.
const personalBesideAcme = `
[[account]]
id    = "personal"
label = "Personal"
pool  = "b2n8"

[[account]]
id      = "side"
label   = "Side"
pool    = "q7mt"
primary = true

[[account]]
id    = "extra"
label = "Extra"
pool  = "q7mt"

[[pool]]
id    = "b2n8"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["/work/acme"]
`

// Making an account its pool's primary tells of the sessions running in its
// pool alone, as they started on its pool's accounts' tokens: never of
// another pool's account.
func TestAccountsEditPrimaryTellsOfTheSessionsOfItsPoolAlone(t *testing.T) {
	const edited = "edited extra: the primary in place of side\n"
	tests := []struct {
		name    string
		tokens  []string
		stdout  string
		warning bool
	}{
		{name: "no account of the pool has a token", tokens: []string{"personal"}, stdout: edited, warning: true},
		{name: "the former primary has one", tokens: []string{"personal", "side"}, stdout: edited + keepingTheFormer("side"), warning: true},
		{name: "the new primary alone has one in the pool", tokens: []string{"personal", "extra"}, stdout: edited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := accountsDeps(t, "http://127.0.0.1:1", personalBesideAcme)
			for _, id := range tt.tokens {
				writeToken(t, deps, id, "test-token-"+id)
			}
			got := run(t, deps, "accounts", "edit", "extra", "--primary")
			if got.code != 0 || got.stdout != tt.stdout {
				t.Errorf("switchboard accounts edit extra --primary printed\n%s\nwant\n%s", got.stdout, tt.stdout)
			}
			if warned := strings.Contains(got.stderr, "extra, the primary now, has no usable token"); warned != tt.warning || strings.Contains(got.stderr, "personal") {
				t.Errorf("switchboard accounts edit extra --primary warned %q, want a warning that extra has no token: %v, and nothing of personal", got.stderr, tt.warning)
			}
		})
	}
}

// Removing a pool's primary says which of its accounts is its primary now,
// and removing its last, that it has none: never another pool's.
func TestAccountsRemoveTellsOfThePrimaryOfTheRemovedAccountsPool(t *testing.T) {
	stayRouted := func(primary, removed string) string {
		return primary + " is the primary now; the sessions running on " + removed + "'s token stay routed, as " + primary + "'s, for a week from when the router takes this up\n"
	}
	tests := []struct {
		name, accounts, id, want string
	}{
		{name: "Default's primary, first of its pool, after Acme's in the file", accounts: unfencedBesideFenced, id: "personal",
			want: "removed personal, and its token file\n" + stayRouted("side", "personal")},
		{name: "Acme's last account, first in the file", accounts: unfencedBesideFenced, id: "work",
			want: "removed work, and its token file\npool Acme has no accounts now: its sessions aren't routed until one is added to it (switchboard accounts add <label> --pool q7mt)\n"},
		{name: "an account no pool's primary", accounts: unfencedBesideFenced, id: "side", want: "removed side, and its token file\n"},
		{name: "Acme's primary, marked", accounts: personalBesideAcme, id: "side",
			want: "removed side, and its token file\n" + stayRouted("extra", "side")},
		{name: "Default's last account", accounts: personalBesideAcme, id: "personal",
			want: "removed personal, and its token file\npool Default has no accounts now: its sessions aren't routed until one is added to it (switchboard accounts add <label> --pool b2n8)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, "http://127.0.0.1:1", tt.accounts)
			for _, a := range loadPooled(t, path).Accounts {
				writeToken(t, deps, a.ID, "test-token-"+a.ID)
			}
			if got, want := run(t, deps, "accounts", "remove", tt.id), (result{stdout: tt.want}); got != want {
				t.Errorf("switchboard accounts remove %s =\n%+v\nwant\n%+v", tt.id, got, want)
			}
		})
	}
}

func TestAccountsShowsEachAccountsPoolOnceThereIsASecond(t *testing.T) {
	onePool := strings.Replace(threeInFirstPool, "reserve = 0.05\n", "", 1)
	tests := []struct {
		name, accounts, want string
	}{
		{name: "two pools", accounts: personalAndSidePooled, want: "ID        LABEL     POOL     PRIMARY  TOKEN\n" +
			"personal  Personal  Default  yes      token missing: write it to %s\n" +
			"side      Side      Acme     yes      token missing: write it to %s\n"},
		{name: "one pool", accounts: onePool, want: "ID        LABEL     PRIMARY  TOKEN\n" +
			"work      Work      yes      token missing: write it to %s\n" +
			"personal  Personal           token missing: write it to %s\n" +
			"side      Side               token missing: write it to %s\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, "http://127.0.0.1:1", tt.accounts)
			var paths []any
			for _, a := range loadPooled(t, path).Accounts {
				paths = append(paths, tokenPath(t, deps, a.ID))
			}
			if got, want := run(t, deps, "accounts"), (result{stdout: fmt.Sprintf(tt.want, paths...)}); got != want {
				t.Errorf("switchboard accounts =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

func TestThePoolsJSONNamesEachPoolByIDAndLabel(t *testing.T) {
	deps, _ := accountsDeps(t, "http://127.0.0.1:1", unfencedBesideFenced)
	var doc struct {
		Pools []map[string]any `json:"pools"`
	}
	got := run(t, deps, "pool", "--json")
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("switchboard pool --json = %+v: %v", got, err)
	}
	for _, p := range doc.Pools {
		if _, ok := p["pool"].(string); !ok {
			t.Errorf("a pool %v gives no pool id", p)
		}
		if _, ok := p["pool_label"].(string); !ok {
			t.Errorf("a pool %v gives no pool_label", p)
		}
	}
}

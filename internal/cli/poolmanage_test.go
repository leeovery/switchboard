package cli_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
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
		{name: "fenced", accounts: unfencedBesideFenced, args: []string{"/work/client", "/work/other"}, said: "fenced to /work/client and /work/other", dirs: []string{"/work/client", "/work/other"}},
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
			want: "the config has no pools yet"},
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
	if got, want := run(t, deps, "pool", "remove", "h4w2"), (result{stdout: "removed pool h4w2 (Handed back)\n"}); got != want {
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

package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestLoadPools(t *testing.T) {
	path := writeConfig(t, `
[[account]]
id    = "k3f9"
label = "Acme 1"
pool  = "Q7MT"

[[account]]
id      = "m2x7"
label   = "Acme 2"
pool    = "q7mt"
primary = true

[[account]]
id    = "b6s7"
label = "Work"
pool  = "b2n8"

[[pool]]
id    = "b2n8"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["~/Code/acme", "/Volumes/Work/acme/./"]

[[pool]]
id   = "h4w2"
dirs = ["/Volumes/Client/"]
`)
	cfg, err := load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	wantPools := config.Pools{
		{ID: "b2n8", Label: "Default"},
		{ID: "q7mt", Label: "Acme", Dirs: []string{"/Users/test/Code/acme", "/Volumes/Work/acme"}, WrittenDirs: []string{"~/Code/acme", "/Volumes/Work/acme/./"}},
		{ID: "h4w2", Label: "h4w2", Dirs: []string{"/Volumes/Client"}, WrittenDirs: []string{"/Volumes/Client/"}},
	}
	if !reflect.DeepEqual(cfg.Pools, wantPools) {
		t.Errorf("Load() pools = %+v, want %+v", cfg.Pools, wantPools)
	}
	wantAccounts := config.Accounts{
		{ID: "k3f9", Label: "Acme 1", Pool: "q7mt"},
		{ID: "m2x7", Label: "Acme 2", Pool: "q7mt", Primary: true},
		{ID: "b6s7", Label: "Work", Pool: "b2n8", Primary: true},
	}
	if !slices.Equal(cfg.Accounts, wantAccounts) {
		t.Errorf("Load() accounts = %+v, want %+v", cfg.Accounts, wantAccounts)
	}
}

func TestLoadEachPoolsPrimary(t *testing.T) {
	pools := poolTOML("b2n8") + poolTOML("q7mt", "~/Code/acme")
	tests := []struct {
		name     string
		accounts string
		want     []string
	}{
		{
			name:     "each pool's first, without one marked",
			accounts: accountIn("k3f9", "q7mt") + accountIn("b6s7", "b2n8") + accountIn("m2x7", "q7mt"),
			want:     []string{"k3f9", "b6s7"},
		},
		{
			name:     "the one marked in each pool",
			accounts: accountIn("k3f9", "q7mt") + accountIn("m2x7", "q7mt") + "primary = true\n" + accountIn("b6s7", "b2n8") + accountIn("n4p8", "b2n8") + "primary = true\n",
			want:     []string{"m2x7", "n4p8"},
		},
		{
			name:     "one pool's marked, the other's its first",
			accounts: accountIn("k3f9", "q7mt") + accountIn("m2x7", "q7mt") + "primary = true\n" + accountIn("b6s7", "b2n8"),
			want:     []string{"m2x7", "b6s7"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := load(writeConfig(t, tt.accounts+pools))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			var primaries []string
			for _, a := range cfg.Accounts {
				if a.Primary {
					primaries = append(primaries, a.ID)
				}
			}
			if !slices.Equal(primaries, tt.want) {
				t.Errorf("the primaries load as %v, want %v", primaries, tt.want)
			}
		})
	}
}

func TestLoadThePoolsRules(t *testing.T) {
	tests := []struct {
		name   string
		config string
		// want are the problems, none where it loads.
		want []string
	}{
		{
			name:   "a pool's directory beside another whose name starts the same",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme") + poolTOML("b2n8", "~/Code/acme-api"),
		},
		{
			name:   "one pool unfenced, beside one fenced",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme") + poolTOML("b2n8"),
		},
		{
			name:   "a pool unfenced by an empty list of directories",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + "dirs = []\n",
		},
		{
			name:   "one primary in each of two pools",
			config: accountIn("k3f9", "q7mt") + "primary = true\n" + accountIn("b6s7", "b2n8") + "primary = true\n" + poolTOML("q7mt", "~/Code/acme") + poolTOML("b2n8"),
		},
		{
			name:   "a pool with no accounts",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + poolTOML("b2n8", "/Volumes/Client"),
		},
		{
			name:   "two pools' directories the same",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme") + poolTOML("b2n8", "~/Code/acme/"),
			want:   []string{`pool "q7mt"'s dirs "~/Code/acme" is the same as pool "b2n8"'s "~/Code/acme/": no directory may be the same as, inside or around another pool's`},
		},
		{
			name:   "a pool's directory around another's",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code") + poolTOML("b2n8", "~/Code/acme"),
			want:   []string{`pool "q7mt"'s dirs "~/Code" holds pool "b2n8"'s "~/Code/acme": no directory may be the same as, inside or around another pool's`},
		},
		{
			name:   "a pool's directory inside another's",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme/api") + poolTOML("b2n8", "~/Code/acme"),
			want:   []string{`pool "q7mt"'s dirs "~/Code/acme/api" is inside pool "b2n8"'s "~/Code/acme": no directory may be the same as, inside or around another pool's`},
		},
		{
			name:   "two pools' directories alike but for their case",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/Acme") + poolTOML("b2n8", "~/code/acme/api"),
			want:   []string{`pool "q7mt"'s dirs "~/Code/Acme" holds pool "b2n8"'s "~/code/acme/api": no directory may be the same as, inside or around another pool's`},
		},
		{
			name:   "a directory from ~ and the same one written whole",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme") + poolTOML("b2n8", "/Users/test/Code/acme"),
			want:   []string{`pool "q7mt"'s dirs "~/Code/acme" is the same as pool "b2n8"'s "/Users/test/Code/acme": no directory may be the same as, inside or around another pool's`},
		},
		{
			name:   "a pool's own directories overlapping",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme", "~/Code/acme/api") + poolTOML("b2n8"),
			want:   []string{`pool "q7mt": dirs "~/Code/acme" holds "~/Code/acme/api": a pool's directories mustn't overlap`},
		},
		{
			name:   "the root around every other directory",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "/") + poolTOML("b2n8", "/Volumes/Client"),
			want:   []string{`pool "q7mt"'s dirs "/" holds pool "b2n8"'s "/Volumes/Client": no directory may be the same as, inside or around another pool's`},
		},
		{
			name:   "two pools unfenced",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + poolTOML("b2n8") + "dirs = []\n",
			want:   []string{`pool "q7mt" and pool "b2n8" have no dirs: only one pool can be unfenced, as a session started outside every fence would have two pools to be in`},
		},
		{
			name:   "an account naming a pool that isn't configured",
			config: accountIn("k3f9", "q7mt") + accountIn("b6s7", "z9z9") + poolTOML("q7mt"),
			want:   []string{`account "b6s7": pool "z9z9": isn't configured: give the id of a [[pool]]`},
		},
		{
			name:   "an account naming no pool where the config has pools",
			config: accountIn("k3f9", "q7mt") + accountTOML("b6s7") + poolTOML("q7mt"),
			want:   []string{`account "b6s7": pool is required, as the config has pools: give the id of the one it's in`},
		},
		{
			name:   "an account naming a pool where the config has none",
			config: accountTOML("k3f9") + accountIn("b6s7", "q7mt"),
			want:   []string{`account "b6s7": pool "q7mt": isn't configured, as the config has no [[pool]] table`},
		},
		{
			name:   "two primaries in one pool",
			config: accountIn("k3f9", "q7mt") + "primary = true\n" + accountIn("m2x7", "q7mt") + "primary = true\n" + accountIn("b6s7", "b2n8") + poolTOML("q7mt", "~/Code/acme") + poolTOML("b2n8"),
			want:   []string{`primary is set on account "k3f9" and account "m2x7" in pool "q7mt": only one account in a pool can be its primary, the one the browser and the Claude apps use for it`},
		},
		{
			name:   "two pools with one id",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + poolTOML("q7mt", "~/Code/acme"),
			want:   []string{`duplicate pool id "q7mt"`},
		},
		{
			name:   "a pool's id given three times, told once",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + poolTOML("q7mt", "/Volumes/A") + poolTOML("q7mt", "/Volumes/B"),
			want:   []string{`duplicate pool id "q7mt"`},
		},
		{
			name:   "a pool's id an account's, ignoring case",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + poolTOML("K3F9", "/Volumes/Client"),
			want:   []string{`pool "K3F9": id is account "k3f9"'s, ignoring case: a pool's id is never an account's`},
		},
		{
			name:   "two pools' ids alike but for their case",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + poolTOML("Q7MT", "~/Code/acme"),
			want:   []string{`pool ids "q7mt" and "Q7MT" differ only in case, which commands take for the same pool`},
		},
		{
			name:   "a pool's id that isn't one",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + poolTOML("../acme", "~/Code/acme") + poolTOML("auto", "~/Code/other") + "\n[[pool]]\ndirs = [\"/Volumes/Client\"]\n",
			want: []string{
				`pool "../acme": id must start with a letter or digit and contain only letters, digits, '-' and '_'`,
				`pool "auto": id is reserved for switchboard pin auto`,
				`pool #4: id is required`,
			},
		},
		{
			name:   "a pool's id or label shaped like a token",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + "label = \"" + tokenShaped + "\"\n" + poolTOML(tokenShaped, "~/Code/acme"),
			want: []string{
				`pool "q7mt": label looks like a token, which a label mustn't, as it shows wherever the pool does`,
				`pool #2: id looks like a token, which an id mustn't, as it shows wherever the pool does`,
			},
		},
		{
			name:   "a pool's label longer than a label may be",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt") + "label = \"" + strings.Repeat("a", 101) + "\"\n",
			want:   []string{`pool "q7mt": label is 101 characters: must be at most 100`},
		},
		{
			name:   "a directory neither absolute nor from ~, nor shaped like a token",
			config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "Code/acme", "~", "/Volumes/"+tokenShaped),
			want: []string{
				`pool "q7mt": dirs "Code/acme": must be an absolute path, or one starting ~/, the home directory's, such as ~/Code/acme`,
				`pool "q7mt": dirs "~": must be an absolute path, or one starting ~/, the home directory's, such as ~/Code/acme`,
				`pool "q7mt": dirs "/Volumes/[redacted]": holds something shaped like a token, which switchboard never writes: give another directory`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := load(path)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Load() error = %v, want it to load", err)
				}
				return
			}
			if got := problems(t, path, err); !slices.Equal(got, tt.want) {
				t.Errorf("Load() problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestLoadAsksForTheHomeDirectoryOnlyForAPoolsDirectoryInIt(t *testing.T) {
	noHome := func() (string, error) { return "", errors.New("$HOME is not defined") }
	tests := []struct {
		name    string
		config  string
		homeDir func() (string, error)
		wantErr string
	}{
		{name: "a config from before pools, given no home", config: accountTOML("k3f9")},
		{name: "pools whose directories are absolute, the home unknown", config: accountIn("k3f9", "q7mt") + poolTOML("q7mt", "/Volumes/Client"), homeDir: noHome},
		{
			name:    "a pool's directory in the home, the home unknown",
			config:  accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme") + poolTOML("b2n8", "~/Code/other"),
			homeDir: noHome,
			wantErr: "find the home directory the pools' dirs are in: $HOME is not defined",
		},
		{
			name:    "a pool's directory in the home, given no home",
			config:  accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme"),
			wantErr: "find the home directory the pools' dirs are in: none was given",
		},
		{
			name:    "a pool's directory in the home, the home not absolute",
			config:  accountIn("k3f9", "q7mt") + poolTOML("q7mt", "~/Code/acme"),
			homeDir: homeAt("home"),
			wantErr: `find the home directory the pools' dirs are in: "home" isn't an absolute path`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := config.Load(path, tt.homeDir)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Load() error = %v, want it to load", err)
				}
				return
			}
			if got := problems(t, path, err); !slices.Equal(got, []string{tt.wantErr}) {
				t.Errorf("Load() problems = %q, want %q", got, tt.wantErr)
			}
		})
	}
}

func TestPoolsOfReadsThePoolsWhateverElseIsWrong(t *testing.T) {
	const pools = "[[pool]]\nid = \"b2n8\"\nlabel = \"Default\"\n\n[[pool]]\nid = \"q7mt\"\nlabel = \"Acme\"\ndirs = [\"~/Code/acme\", \"/Volumes/Acme/\"]\n"
	read := config.Pools{
		{ID: "b2n8", Label: "Default"},
		{ID: "q7mt", Label: "Acme", Dirs: []string{"/Users/test/Code/acme", "/Volumes/Acme"}, WrittenDirs: []string{"~/Code/acme", "/Volumes/Acme/"}},
	}
	tests := []struct {
		name string
		// text is the config file's, or none where there's no file.
		text    *string
		want    config.Pools
		wantErr string
	}{
		{name: "no config"},
		{name: "no pools", text: new("[[account]]\nid = \"work\"\nlabel = \"Work\"\n")},
		{name: "pools", text: new(pools + "\n[[account]]\nid = \"work\"\nlabel = \"Work\"\npool = \"b2n8\"\n"), want: read},
		{
			name: "a config otherwise invalid, its accounts naming no pool and a key gone: read all the same",
			text: new("week_starts = 3\n[[account]]\nid = \"work\"\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n\n" + pools),
			want: read,
		},
		{name: "not TOML", text: new("[[pool]\n"), wantErr: "parse config "},
		{name: "a pool's table of the wrong type", text: new("[[pool]]\nid = \"b2n8\"\ndirs = \"~/Code/acme\"\n"), wantErr: "parse config "},
		{name: "pools Load refuses", text: new(pools + "\n[[pool]]\nid = \"h4w2\"\nlabel = \"Client\"\n"), wantErr: "only one pool can be unfenced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if tt.text != nil {
				if err := os.WriteFile(path, []byte(*tt.text), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := config.PoolsOf(path, homeAt(testHome))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("PoolsOf() = %+v, %v; want an error saying %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PoolsOf() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestFencesOfReadsEveryPoolsDirectoriesWhateverIsWrongWithThem(t *testing.T) {
	tests := []struct {
		name string
		// text is the config file's, or none where there's no file.
		text    *string
		want    config.Pools
		wantErr string
	}{
		{name: "no config"},
		{name: "no pools", text: new("[[account]]\nid = \"work\"\n")},
		{
			name: "two unfenced, overlapping fences, a label too long and a directory that can't be placed: every fence found, and that one unplaced",
			text: new("[[pool]]\nid = \"b2n8\"\n\n[[pool]]\nid = \"h4w2\"\n\n" +
				"[[pool]]\nid = \"q7mt\"\nlabel = \"" + strings.Repeat("A", 101) + "\"\ndirs = [\"~/Code/acme/\", \"Code/relative\"]\n\n" +
				"[[pool]]\nid = \"m5r1\"\nlabel = \"Client\"\ndirs = [\"/Users/test/Code/acme/client\"]\n"),
			want: config.Pools{
				{ID: "b2n8", Label: "b2n8"},
				{ID: "h4w2", Label: "h4w2"},
				{ID: "q7mt", Label: strings.Repeat("A", 101), Dirs: []string{"/Users/test/Code/acme"}, WrittenDirs: []string{"~/Code/acme/", "Code/relative"}, Unplaced: []string{"Code/relative"}},
				{ID: "m5r1", Label: "Client", Dirs: []string{"/Users/test/Code/acme/client"}, WrittenDirs: []string{"/Users/test/Code/acme/client"}},
			},
		},
		{name: "not TOML", text: new("[[pool]\n"), wantErr: "parse config "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if tt.text != nil {
				if err := os.WriteFile(path, []byte(*tt.text), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := config.FencesOf(path, homeAt(testHome))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("FencesOf() = %+v, %v; want an error saying %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FencesOf() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestFencesOfWithoutAHomeDirectoryLeavesThoseInItUnplaced(t *testing.T) {
	path := writeConfig(t, "[[pool]]\nid = \"q7mt\"\ndirs = [\"~/Code/acme\", \"/Volumes/Acme\"]\n")

	got, err := config.FencesOf(path, func() (string, error) { return "", errors.New("no home directory") })
	wantDirs, wantUnplaced := []string{"/Volumes/Acme"}, []string{"~/Code/acme"}
	if err != nil || len(got) != 1 || !slices.Equal(got[0].Dirs, wantDirs) || !slices.Equal(got[0].Unplaced, wantUnplaced) {
		t.Errorf("FencesOf() = %+v, %v; want one pool fenced to %q, %q unplaced", got, err, wantDirs, wantUnplaced)
	}
}

func TestPoolPlacedIn(t *testing.T) {
	tests := []struct {
		name         string
		home         string
		unplaced     []string
		wantDirs     []string
		wantUnplaced []string
	}{
		{name: "none"},
		{name: "absolute, cleaned", unplaced: []string{"/Volumes/Acme/"}, wantDirs: []string{"/Volumes/Acme"}},
		{name: "in the home directory", home: testHome, unplaced: []string{"~/Code/acme"}, wantDirs: []string{testHome + "/Code/acme"}},
		{name: "in the home directory, none found", unplaced: []string{"~/Code/acme"}, wantUnplaced: []string{"~/Code/acme"}},
		{name: "in the home directory, a relative one", home: "home/test", unplaced: []string{"~/Code/acme"}, wantUnplaced: []string{"~/Code/acme"}},
		{name: "relative", home: testHome, unplaced: []string{"Code/acme"}, wantUnplaced: []string{"Code/acme"}},
		{name: "a home directory written plainly", home: testHome, unplaced: []string{"~"}, wantUnplaced: []string{"~"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			placed := config.Pool{ID: "q7mt", Dirs: []string{"/Volumes/Old"}, Unplaced: tt.unplaced}.PlacedIn(tt.home)
			wantDirs := append([]string{"/Volumes/Old"}, tt.wantDirs...)
			if !slices.Equal(placed.Dirs, wantDirs) || !slices.Equal(placed.Unplaced, tt.wantUnplaced) {
				t.Errorf("PlacedIn(%q) = dirs %q, unplaced %q; want %q, %q", tt.home, placed.Dirs, placed.Unplaced, wantDirs, tt.wantUnplaced)
			}
		})
	}
}

func TestReadPoolsReadsThemAsLoadReadsAConfigsOwn(t *testing.T) {
	tests := []struct {
		name  string
		pools []config.NewPool
		// file is the same pools as a config file writes them.
		file    string
		wantErr bool
	}{
		{
			name:  "pools in the home and outside it, one shown by its id",
			pools: []config.NewPool{{ID: "b2n8", Label: "Default"}, {ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme", "/Volumes/Work/acme/./"}}, {ID: "h4w2", Dirs: []string{"/Volumes/Client/"}}},
			file:  poolTOML("b2n8") + "label = \"Default\"\n" + poolTOML("q7mt", "~/Code/acme", "/Volumes/Work/acme/./") + "label = \"Acme\"\n" + poolTOML("h4w2", "/Volumes/Client/"),
		},
		{name: "two unfenced", pools: []config.NewPool{{ID: "b2n8"}, {ID: "q7mt"}}, file: poolTOML("b2n8") + poolTOML("q7mt"), wantErr: true},
		{
			name:    "one's directory inside another's",
			pools:   []config.NewPool{{ID: "b2n8", Dirs: []string{"~/Code"}}, {ID: "q7mt", Dirs: []string{"/Users/test/Code/acme"}}},
			file:    poolTOML("b2n8", "~/Code") + poolTOML("q7mt", "/Users/test/Code/acme"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, accountIn("k3f9", "b2n8")+tt.file)
			cfg, loadErr := load(path)
			if (loadErr != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, want an error: %v", loadErr, tt.wantErr)
			}

			got, err := config.ReadPools(tt.pools, homeAt(testHome))
			if !tt.wantErr {
				if err != nil || !reflect.DeepEqual(got, cfg.Pools) {
					t.Errorf("ReadPools() = %+v, %v, want %+v, as Load reads them", got, err, cfg.Pools)
				}
				return
			}
			if want := problems(t, path, loadErr); err == nil || !slices.Equal(strings.Split(err.Error(), "\n"), want) {
				t.Errorf("ReadPools() error = %v, want Load's problems %q", err, want)
			}
		})
	}
}

func TestPoolsCovering(t *testing.T) {
	acme := config.Pool{ID: "q7mt", Dirs: []string{"/Users/test/Code/acme", "/Volumes/Acme"}}
	client := config.Pool{ID: "h4w2", Dirs: []string{"/Users/test/Code/client"}}
	unfenced := config.Pool{ID: "b2n8"}
	tests := []struct {
		name  string
		pools config.Pools
		dir   string
		want  string
	}{
		{name: "a fenced pool's directory itself", pools: config.Pools{unfenced, acme}, dir: "/Users/test/Code/acme", want: "q7mt"},
		{name: "inside a fenced pool's directory", pools: config.Pools{unfenced, acme}, dir: "/Users/test/Code/acme/api/.claude/worktrees/x", want: "q7mt"},
		{name: "inside a fenced pool's other directory", pools: config.Pools{acme, unfenced}, dir: "/Volumes/Acme/site", want: "q7mt"},
		{name: "inside one, ignoring case", pools: config.Pools{unfenced, acme}, dir: "/users/TEST/code/ACME/api", want: "q7mt"},
		{name: "beside one whose name starts the same", pools: config.Pools{unfenced, acme}, dir: "/Users/test/Code/acme-api", want: "b2n8"},
		{name: "around one", pools: config.Pools{unfenced, acme}, dir: "/Users/test/Code", want: "b2n8"},
		{name: "outside every fence", pools: config.Pools{acme, client, unfenced}, dir: "/tmp", want: "b2n8"},
		{name: "in the second of two fenced pools", pools: config.Pools{acme, client, unfenced}, dir: "/Users/test/Code/client/app", want: "h4w2"},
		{name: "outside every fence, every pool fenced", pools: config.Pools{acme, client}, dir: "/Users/test/Code/other"},
		{name: "no pools", dir: "/Users/test/Code/acme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.pools.Covering(tt.dir)
			if got.ID != tt.want || ok != (tt.want != "") {
				t.Errorf("Covering(%q) = %q, %v; want %q", tt.dir, got.ID, ok, tt.want)
			}
		})
	}
}

func TestPoolsImplicit(t *testing.T) {
	acme := config.Pool{ID: "q7mt", Dirs: []string{"/Users/test/Code/acme"}}
	client := config.Pool{ID: "h4w2", Dirs: []string{"/Users/test/Code/client"}}
	unfenced := config.Pool{ID: "b2n8"}
	tests := []struct {
		name  string
		pools config.Pools
		want  string
	}{
		{name: "the only pool, unfenced", pools: config.Pools{unfenced}, want: "b2n8"},
		{name: "the only pool, fenced", pools: config.Pools{acme}, want: "q7mt"},
		{name: "the unfenced of several", pools: config.Pools{acme, unfenced, client}, want: "b2n8"},
		{name: "several, every one fenced", pools: config.Pools{acme, client}},
		{name: "none, as in a config from before pools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.pools.Implicit()
			if got.ID != tt.want || ok != (tt.want != "") {
				t.Errorf("Implicit() = %q, %v; want %q", got.ID, ok, tt.want)
			}
		})
	}
}

func TestPoolsAsked(t *testing.T) {
	acme := config.Pool{ID: "q7mt", Dirs: []string{"/Users/test/Code/acme"}}
	client := config.Pool{ID: "h4w2", Dirs: []string{"/Users/test/Code/client"}}
	unfenced := config.Pool{ID: "b2n8"}
	tests := []struct {
		name  string
		pools config.Pools
		named string
		want  string
	}{
		{name: "one named", pools: config.Pools{acme, unfenced}, named: "q7mt", want: "q7mt"},
		{name: "one named in another case", pools: config.Pools{acme, unfenced}, named: "Q7MT", want: "q7mt"},
		{name: "one named that isn't configured", pools: config.Pools{acme, unfenced}, named: "x9z9"},
		{name: "none named: the unfenced", pools: config.Pools{acme, unfenced}, want: "b2n8"},
		{name: "none named, every pool fenced", pools: config.Pools{acme, client}},
		{name: "one named, none configured", named: "q7mt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.pools.Asked(tt.named)
			if got.ID != tt.want || ok != (tt.want != "") {
				t.Errorf("Asked(%q) = %q, %v; want %q", tt.named, got.ID, ok, tt.want)
			}
		})
	}
}

func TestAPoolsAccountsAndPrimary(t *testing.T) {
	cfg, err := load(writeConfig(t, accountIn("k3f9", "q7mt")+accountIn("b6s7", "b2n8")+accountIn("m2x7", "Q7MT")+"primary = true\n"+
		poolTOML("b2n8")+poolTOML("q7mt", "~/Code/acme")+poolTOML("h4w2", "~/Code/client")))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tests := []struct {
		pool         string
		wantAccounts []string
		wantPrimary  string
	}{
		{pool: "q7mt", wantAccounts: []string{"k3f9", "m2x7"}, wantPrimary: "m2x7"},
		{pool: "Q7MT", wantAccounts: []string{"k3f9", "m2x7"}, wantPrimary: "m2x7"},
		{pool: "b2n8", wantAccounts: []string{"b6s7"}, wantPrimary: "b6s7"},
		{pool: "h4w2"},
		{pool: "z9z9"},
	}
	for _, tt := range tests {
		t.Run(tt.pool, func(t *testing.T) {
			if got := cfg.Accounts.InPool(tt.pool).IDs(); !slices.Equal(got, tt.wantAccounts) {
				t.Errorf("InPool(%q) = %v, want %v", tt.pool, got, tt.wantAccounts)
			}
			primary, ok := cfg.Accounts.PrimaryOf(tt.pool)
			if primary.ID != tt.wantPrimary || ok != (tt.wantPrimary != "") {
				t.Errorf("PrimaryOf(%q) = %q, %v; want %q", tt.pool, primary.ID, ok, tt.wantPrimary)
			}
		})
	}
}

func TestAnAccountsPool(t *testing.T) {
	cfg, err := load(writeConfig(t, accountIn("k3f9", "Q7MT")+accountIn("b6s7", "b2n8")+poolTOML("b2n8")+poolTOML("q7mt", "~/Code/acme")))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tests := []struct {
		account, want string
	}{
		{account: "k3f9", want: "q7mt"},
		{account: "B6S7", want: "b2n8"},
		{account: "z9z9"},
	}
	for _, tt := range tests {
		t.Run(tt.account, func(t *testing.T) {
			got, ok := cfg.PoolOf(tt.account)
			if got.ID != tt.want || ok != (tt.want != "") {
				t.Errorf("PoolOf(%q) = %q, %v; want %q", tt.account, got.ID, ok, tt.want)
			}
		})
	}
}

func TestAPoolByItsID(t *testing.T) {
	pools := config.Pools{{ID: "b2n8"}, {ID: "q7mt"}}
	tests := []struct {
		id, want string
	}{
		{id: "q7mt", want: "q7mt"},
		{id: "Q7MT", want: "q7mt"},
		{id: "z9z9"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, ok := pools.ByID(tt.id)
			if got.ID != tt.want || ok != (tt.want != "") {
				t.Errorf("ByID(%q) = %q, %v; want %q", tt.id, got.ID, ok, tt.want)
			}
		})
	}
	if got := pools.IDs(); !slices.Equal(got, []string{"b2n8", "q7mt"}) {
		t.Errorf("IDs() = %v, want the pools' ids in their order", got)
	}
}

func TestPoolsLabelTaken(t *testing.T) {
	pools := config.Pools{{ID: "b2n8", Label: "Default"}, {ID: "q7mt", Label: "Acme"}}
	tests := []struct {
		name, id, label string
		want            bool
	}{
		{name: "another pool's", id: "", label: "Acme", want: true},
		{name: "another pool's, in another case", id: "b2n8", label: "aCME", want: true},
		{name: "its own", id: "q7mt", label: "Acme"},
		{name: "none's", id: "", label: "Client"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pools.LabelTaken(tt.id, tt.label); got != tt.want {
				t.Errorf("LabelTaken(%q, %q) = %v, want %v", tt.id, tt.label, got, tt.want)
			}
		})
	}
}

// poolTOML returns a [[pool]] table with the given id, fenced to dirs where
// any are given, which the test's own keys can follow.
func poolTOML(id string, dirs ...string) string {
	table := fmt.Sprintf("\n[[pool]]\nid = %q\n", id)
	if len(dirs) == 0 {
		return table
	}
	quoted := make([]string, len(dirs))
	for i, dir := range dirs {
		quoted[i] = fmt.Sprintf("%q", dir)
	}
	return table + "dirs = [" + strings.Join(quoted, ", ") + "]\n"
}

// accountIn returns an [[account]] table with the given id, in the pool
// with the given id, which the test's own keys can follow.
func accountIn(id, pool string) string {
	return accountTOML(id) + fmt.Sprintf("pool = %q\n", pool)
}

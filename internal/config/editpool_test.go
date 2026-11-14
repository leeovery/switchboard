package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// pooled is a config with pools, laid out as commented is: a comment heading
// it, comments on the keys and above the tables, and a table left commented
// out at its end.
const pooled = `# One [[account]] per Claude subscription.

[[account]]
id      = "work"  # for good
label   = "Work"
pool    = "b2n8"
primary = true

[[account]]
id    = "acme"
label = "Acme 1"
pool  = "q7mt"  # the client's

[[account]]
id      = "acme2"
label   = "Acme 2"
pool    = "q7mt"
primary = true    # the client's browser

[[pool]]
id    = "b2n8"
label = "Default"

# The client's.
[[pool]]
id    = "q7mt"
label = "Acme"  # as the client calls it
dirs  = ["~/Code/acme"]

# Handed back.
[[pool]]
id   = "h4w2"
dirs = ["/Volumes/Client"]

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`

// pooledWith is pooled with each pair of swaps, a piece of its text, found
// in it once, and what goes in its place, swapped in turn.
func pooledWith(t *testing.T, swaps ...string) string {
	t.Helper()
	text := pooled
	for i := 0; i+1 < len(swaps); i += 2 {
		if n := strings.Count(text, swaps[i]); n != 1 {
			t.Fatalf("%q is in the config %d times, want once", swaps[i], n)
		}
		text = strings.Replace(text, swaps[i], swaps[i+1], 1)
	}
	return text
}

func TestPoolEdits(t *testing.T) {
	tests := []struct {
		name   string
		config string
		edit   func(*config.Draft) error
		want   string
	}{
		{
			name:   "adding a pool after the last pool's, before the comments that follow it",
			config: pooled,
			edit: func(d *config.Draft) error {
				return d.AddPool(config.NewPool{ID: "m5r1", Label: "Side", Dirs: []string{"~/Code/side", "/Volumes/Side"}})
			},
			want: pooledWith(t, "dirs = [\"/Volumes/Client\"]\n", "dirs = [\"/Volumes/Client\"]\n\n[[pool]]\nid    = \"m5r1\"\nlabel = \"Side\"\ndirs  = [\"~/Code/side\", \"/Volumes/Side\"]\n"),
		},
		{
			name:   "adding a pool labelled by its id, writing no label",
			config: pooled,
			edit: func(d *config.Draft) error {
				return d.AddPool(config.NewPool{ID: "m5r1", Label: "m5r1", Dirs: []string{"/Volumes/Side"}})
			},
			want: pooledWith(t, "dirs = [\"/Volumes/Client\"]\n", "dirs = [\"/Volumes/Client\"]\n\n[[pool]]\nid   = \"m5r1\"\ndirs = [\"/Volumes/Side\"]\n"),
		},
		{
			name:   "adding the first pool to a config from before pools, putting every account in it",
			config: commented,
			edit:   func(d *config.Draft) error { return d.AddPool(config.NewPool{ID: "b2n8", Label: "Default"}) },
			want: `# One [[account]] per Claude subscription.

[[account]]
id      = "work"  # for good
label   = "Work"  # optional; defaults to the id
primary = true    # optional: the account the browser and the Claude apps use
reserve = 0.1     # optional
pool = "b2n8"

[[account]]
id    = "personal"
label = "Personal"
pool = "b2n8"

[[pool]]
id    = "b2n8"
label = "Default"

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`,
		},
		{
			name:   "relabelling a pool, keeping the comment beside its label",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPoolLabel("Q7MT", "Acme Ltd") },
			want:   pooledWith(t, `label = "Acme"  # as the client calls it`, `label = "Acme Ltd"  # as the client calls it`),
		},
		{
			name:   "labelling a pool that had none, below its last key",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPoolLabel("h4w2", "Old client") },
			want:   pooledWith(t, "dirs = [\"/Volumes/Client\"]\n", "dirs = [\"/Volumes/Client\"]\nlabel = \"Old client\"\n"),
		},
		{
			name:   "taking a pool's label out, showing it by its id",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPoolLabel("b2n8", "") },
			want:   pooledWith(t, "label = \"Default\"\n", ""),
		},
		{
			name:   "labelling a pool by its id, taking its label out",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPoolLabel("b2n8", "b2n8") },
			want:   pooledWith(t, "label = \"Default\"\n", ""),
		},
		{
			name:   "labelling a pool as it's labelled, leaving it as it is",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPoolLabel("h4w2", "h4w2") },
			want:   pooled,
		},
		{
			name:   "fencing a pool in place of its directories, on one line",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPoolDirs("q7mt", []string{"~/Code/acme", "/Volumes/Acme"}) },
			want:   pooledWith(t, `dirs  = ["~/Code/acme"]`, `dirs  = ["~/Code/acme", "/Volumes/Acme"]`),
		},
		{
			name:   "fencing a pool in place of its directories, keeping the comment beside them",
			config: pooledWith(t, `dirs  = ["~/Code/acme"]`, `dirs  = ["~/Code/acme"]  # the client's repos, "]" and all`),
			edit:   func(d *config.Draft) error { return d.SetPoolDirs("q7mt", []string{"/Volumes/Acme"}) },
			want:   pooledWith(t, `dirs  = ["~/Code/acme"]`, `dirs  = ["/Volumes/Acme"]  # the client's repos, "]" and all`),
		},
		{
			name:   "fencing the unfenced pool, below its last key",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPoolDirs("b2n8", []string{"~/Work"}) },
			want:   pooledWith(t, "label = \"Default\"\n", "label = \"Default\"\ndirs = [\"~/Work\"]\n"),
		},
		{
			name:   "unfencing a pool where none is unfenced, taking its directories out",
			config: pooledWith(t, "label = \"Default\"\n", "label = \"Default\"\ndirs  = [\"~/Work\"]\n"),
			edit:   func(d *config.Draft) error { return d.SetPoolDirs("h4w2", nil) },
			want: pooledWith(t,
				"label = \"Default\"\n", "label = \"Default\"\ndirs  = [\"~/Work\"]\n",
				"dirs = [\"/Volumes/Client\"]\n", "",
			),
		},
		{
			name:   "removing a pool with no accounts, with the comments above it",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.RemovePool("H4W2") },
			want:   pooledWith(t, "# Handed back.\n[[pool]]\nid   = \"h4w2\"\ndirs = [\"/Volumes/Client\"]\n\n", ""),
		},
		{
			name:   "moving an account to another pool, keeping the comment beside its pool",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPool("acme", "B2N8") },
			want:   pooledWith(t, `pool  = "q7mt"  # the client's`, `pool  = "b2n8"  # the client's`),
		},
		{
			name:   "moving a pool's primary to another pool, taking its mark off",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPool("acme2", "h4w2") },
			want:   pooledWith(t, "pool    = \"q7mt\"\nprimary = true    # the client's browser\n", "pool    = \"h4w2\"\n"),
		},
		{
			name:   "moving an account to the pool it's in, leaving it as it is",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPool("acme", "q7mt") },
			want:   pooled,
		},
		{
			name:   "adding an account to a pool as its primary, taking primary off that pool's alone",
			config: pooled,
			edit: func(d *config.Draft) error {
				return d.AddAccount(config.NewAccount{ID: "acme3", Label: "Acme 3", Pool: "Q7MT", Plan: "max", Primary: true})
			},
			want: pooledWith(t,
				"pool    = \"q7mt\"\nprimary = true    # the client's browser\n",
				"pool    = \"q7mt\"\n\n[[account]]\nid      = \"acme3\"\nlabel   = \"Acme 3\"\npool    = \"q7mt\"\nplan    = \"max\"\nprimary = true\n",
			),
		},
		{
			name:   "making an account its pool's primary, taking primary off that pool's alone",
			config: pooled,
			edit:   func(d *config.Draft) error { return d.SetPrimary("acme") },
			want: pooledWith(t,
				"pool  = \"q7mt\"  # the client's\n", "pool  = \"q7mt\"  # the client's\nprimary = true\n",
				"primary = true    # the client's browser\n", "",
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := tt.edit(draft); err != nil {
				t.Fatalf("edit error = %v", err)
			}
			save(t, draft)
			if got := readFile(t, path); got != tt.want {
				t.Errorf("the config reads\n%s\nwant\n%s", got, tt.want)
			}
			cfg, err := load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(draft.Config(), cfg) {
				t.Errorf("Config() = %+v, want what the file saved loads as, %+v", draft.Config(), cfg)
			}
		})
	}
}

// A pool not configured is told apart from an account not configured, which
// it matches too.
func TestAnAccountNotConfiguredIsNoPoolNotConfigured(t *testing.T) {
	if err := config.NotConfigured("side"); errors.Is(err, config.ErrPoolNotConfigured) {
		t.Errorf("NotConfigured() = %v, want no ErrPoolNotConfigured", err)
	}
	if err := config.PoolNotConfigured("z9z9"); !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("PoolNotConfigured() = %v, want an ErrNotConfigured too", err)
	}
}

func TestPoolEditsRefuse(t *testing.T) {
	const invalid = "invalid config %s:\n"
	tests := []struct {
		name   string
		config string
		edit   func(*config.Draft) error
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
		wantIs  error
	}{
		{
			name:   "adding a pool with an id a pool has already",
			config: pooled,
			edit: func(d *config.Draft) error {
				return d.AddPool(config.NewPool{ID: "q7mt", Dirs: []string{"/Volumes/Side"}})
			},
			wantErr: `pool "q7mt" is already configured`,
			wantIs:  config.ErrConfigured,
		},
		{
			name:   "adding a pool with an id a pool has already, in another case",
			config: pooled,
			edit: func(d *config.Draft) error {
				return d.AddPool(config.NewPool{ID: "Q7MT", Dirs: []string{"/Volumes/Side"}})
			},
			wantErr: `pool "Q7MT" is already configured`,
			wantIs:  config.ErrConfigured,
		},
		{
			name:   "adding a pool with an account's id",
			config: pooled,
			edit: func(d *config.Draft) error {
				return d.AddPool(config.NewPool{ID: "Acme", Dirs: []string{"/Volumes/Side"}})
			},
			wantErr: invalid + `pool "Acme": id is account "acme"'s, ignoring case: a pool's id is never an account's`,
		},
		{
			name:    "adding an account with a pool's id",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "h4w2", Pool: "b2n8"}) },
			wantErr: invalid + `pool "h4w2": id is account "h4w2"'s, ignoring case: a pool's id is never an account's`,
		},
		{
			name:    "adding a pool with an id no pool can have",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.AddPool(config.NewPool{ID: "../side"}) },
			wantErr: `pool "../side": id must start with a letter or digit and contain only letters, digits, '-' and '_'`,
		},
		{
			name:    "adding a pool with a label no pool can have",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.AddPool(config.NewPool{ID: "m5r1", Label: tokenShaped}) },
			wantErr: `pool "m5r1": label looks like a token, which a label mustn't, as it shows wherever the pool does`,
		},
		{
			name:    "adding a second pool unfenced",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.AddPool(config.NewPool{ID: "m5r1", Label: "Side"}) },
			wantErr: invalid + `pool "b2n8" and pool "m5r1" have no dirs: only one pool can be unfenced, as a session started outside every fence would have two pools to be in`,
		},
		{
			name:   "adding a pool fenced inside another's directory",
			config: pooled,
			edit: func(d *config.Draft) error {
				return d.AddPool(config.NewPool{ID: "m5r1", Dirs: []string{"~/Code/acme/api"}})
			},
			wantErr: invalid + `pool "q7mt"'s dirs "~/Code/acme" holds pool "m5r1"'s "~/Code/acme/api": no directory may be the same as, inside or around another pool's`,
		},
		{
			name:    "adding a pool fenced to a directory neither absolute nor from ~",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.AddPool(config.NewPool{ID: "m5r1", Dirs: []string{"Code/side"}}) },
			wantErr: invalid + `pool "m5r1": dirs "Code/side": must be an absolute path, or one starting ~/, the home directory's, such as ~/Code/acme`,
		},
		{
			name:    "relabelling a pool that isn't configured",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.SetPoolLabel("z9z9", "Side") },
			wantErr: `pool "z9z9" is not configured`,
			wantIs:  config.ErrPoolNotConfigured,
		},
		{
			name:    "relabelling a pool with a label no pool can have",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.SetPoolLabel("q7mt", "Acme\nLtd") },
			wantErr: `pool "q7mt": label holds a control character, such as a newline or a tab, which can't show`,
		},
		{
			name:    "fencing a pool that isn't configured",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.SetPoolDirs("z9z9", []string{"/Volumes/Side"}) },
			wantErr: `pool "z9z9" is not configured`,
			wantIs:  config.ErrPoolNotConfigured,
		},
		{
			name:    "fencing a pool around another's directory",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.SetPoolDirs("h4w2", []string{"~/Code"}) },
			wantErr: invalid + `pool "q7mt"'s dirs "~/Code/acme" is inside pool "h4w2"'s "~/Code": no directory may be the same as, inside or around another pool's`,
		},
		{
			name:    "unfencing a second pool",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.SetPoolDirs("q7mt", nil) },
			wantErr: invalid + `pool "b2n8" and pool "q7mt" have no dirs: only one pool can be unfenced, as a session started outside every fence would have two pools to be in`,
		},
		{
			name:    "fencing a pool whose directories go on over lines",
			config:  pooledWith(t, "dirs = [\"/Volumes/Client\"]\n", "dirs = [\n  \"/Volumes/Client\",\n]\n"),
			edit:    func(d *config.Draft) error { return d.SetPoolDirs("h4w2", []string{"/Volumes/Other"}) },
			wantErr: `pool "h4w2" writes its dirs across lines, which editing %s as text can't change: set them by hand`,
		},
		{
			name:    "removing a pool that isn't configured",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.RemovePool("z9z9") },
			wantErr: `pool "z9z9" is not configured`,
			wantIs:  config.ErrPoolNotConfigured,
		},
		{
			name:    "removing a pool that has accounts",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.RemovePool("q7mt") },
			wantErr: `pool "q7mt", holding account "acme" and account "acme2", has accounts: move each to another pool, or remove it, first`,
			wantIs:  config.ErrPoolHasAccounts,
		},
		{
			name:    "removing the only pool",
			config:  "[[pool]]\nid = \"b2n8\"\n",
			edit:    func(d *config.Draft) error { return d.RemovePool("b2n8") },
			wantErr: `pool "b2n8" is the only one, and a config with pools needs one at least, for an account added to go into`,
			wantIs:  config.ErrOnlyPool,
		},
		{
			name:    "moving an account that isn't configured",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.SetPool("side", "b2n8") },
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "moving an account to a pool that isn't configured",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.SetPool("acme", "z9z9") },
			wantErr: `pool "z9z9" is not configured`,
			wantIs:  config.ErrPoolNotConfigured,
		},
		{
			name:    "adding an account to a pool that isn't configured",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "side", Pool: "z9z9"}) },
			wantErr: `pool "z9z9" is not configured`,
			wantIs:  config.ErrPoolNotConfigured,
		},
		{
			name:    "adding an account to no pool where the config has pools",
			config:  pooled,
			edit:    func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "side"}) },
			wantErr: invalid + `account "side": pool is required, as the config has pools: give the id of the one it's in`,
		},
		{
			name:    "adding an account to a pool where the config has none",
			config:  commented,
			edit:    func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "side", Pool: "b2n8"}) },
			wantErr: `pool "b2n8" is not configured`,
			wantIs:  config.ErrPoolNotConfigured,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := tt.edit(draft)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, tt.wantIs)
		})
	}
}

package config_test

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// commented is a config laid out as the example is: a comment heading it,
// comments on the keys, and a table left commented out at its end.
const commented = `# One [[account]] per Claude subscription.

[[account]]
id      = "work"  # for good
label   = "Work"  # optional; defaults to the id
primary = true    # optional: the account the browser and the Claude apps use
reserve = 0.1     # optional

[[account]]
id    = "personal"
label = "Personal"

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`

func TestAddAccount(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		account config.NewAccount
		want    string
	}{
		{
			name:    "after the last account, before the comments that follow it",
			config:  commented,
			account: config.NewAccount{ID: "side", Label: "Side"},
			want: `# One [[account]] per Claude subscription.

[[account]]
id      = "work"  # for good
label   = "Work"  # optional; defaults to the id
primary = true    # optional: the account the browser and the Claude apps use
reserve = 0.1     # optional

[[account]]
id    = "personal"
label = "Personal"

[[account]]
id    = "side"
label = "Side"

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`,
		},
		{
			name:    "as the primary, taking primary off the account that had it",
			config:  commented,
			account: config.NewAccount{ID: "side", Label: "Side", Primary: true},
			want: `# One [[account]] per Claude subscription.

[[account]]
id      = "work"  # for good
label   = "Work"  # optional; defaults to the id
reserve = 0.1     # optional

[[account]]
id    = "personal"
label = "Personal"

[[account]]
id      = "side"
label   = "Side"
primary = true

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`,
		},
		{
			name:    "as the primary, where none is marked",
			config:  "[[account]]\nid = \"work\"\nprimary = false\n",
			account: config.NewAccount{ID: "side", Primary: true},
			want:    "[[account]]\nid = \"work\"\nprimary = false\n\n[[account]]\nid      = \"side\"\nprimary = true\n",
		},
		{
			name:    "without a label",
			config:  "[[account]]\nid = \"work\"\n",
			account: config.NewAccount{ID: "side"},
			want:    "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n",
		},
		{
			name:    "the first, before the first table and the comments above it",
			config:  "listen = \"127.0.0.1:4747\"\n\n# The day's windows.\n[prime]\nday = \"08:00-23:00\"\n",
			account: config.NewAccount{ID: "work", Label: "Work"},
			want:    "listen = \"127.0.0.1:4747\"\n\n[[account]]\nid    = \"work\"\nlabel = \"Work\"\n\n# The day's windows.\n[prime]\nday = \"08:00-23:00\"\n",
		},
		{
			name:    "the first, into an empty file",
			config:  "",
			account: config.NewAccount{ID: "work", Label: "Work", Primary: true},
			want:    "[[account]]\nid      = \"work\"\nlabel   = \"Work\"\nprimary = true\n",
		},
		{
			name:    "the first, after a file's comments",
			config:  "# Switchboard's config.\n",
			account: config.NewAccount{ID: "work"},
			want:    "# Switchboard's config.\n\n[[account]]\nid = \"work\"\n",
		},
		{
			name:    "after the last account, with tables between the accounts and after them",
			config:  "[[account]]\nid = \"work\"\n\n[prime]\nday = \"08:00-23:00\"\n\n[[account]]\nid = \"personal\"\n\n# Quieter.\n[notifications]\nmoves = false\n",
			account: config.NewAccount{ID: "side"},
			want:    "[[account]]\nid = \"work\"\n\n[prime]\nday = \"08:00-23:00\"\n\n[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"side\"\n\n# Quieter.\n[notifications]\nmoves = false\n",
		},
		{
			name:    "set apart from tables with no blank line between them",
			config:  "[[account]]\nid = \"work\"\n[prime]\nday = \"08:00-23:00\"\n",
			account: config.NewAccount{ID: "side"},
			want:    "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n\n[prime]\nday = \"08:00-23:00\"\n",
		},
		{
			name:    "after the comments directly below the last account's keys",
			config:  "[[account]]\nid = \"work\"\n# reserve = 0.2\n\n# The end.\n",
			account: config.NewAccount{ID: "side"},
			want:    "[[account]]\nid = \"work\"\n# reserve = 0.2\n\n[[account]]\nid = \"side\"\n\n# The end.\n",
		},
		{
			name:    "a label TOML asks to be escaped",
			config:  "[[account]]\nid = \"work\"\n",
			account: config.NewAccount{ID: "side", Label: "Side \"B\" \\ two\nlines\ttabbed\x7f"},
			want:    "[[account]]\nid = \"work\"\n\n[[account]]\nid    = \"side\"\nlabel = \"Side \\\"B\\\" \\\\ two\\u000Alines\\u0009tabbed\\u007F\"\n",
		},
		{
			name: "past a label that goes on over lines, whatever its lines look like",
			config: "[[account]]\nid = \"work\"\nlabel = \"\"\"\n[[account]]\nprimary = true\n\"\"\"\nprimary = true\n\n" +
				"[[account]]\nid = \"personal\"\nlabel = '''\n# Not a comment.\n\n[prime]\n'''\n",
			account: config.NewAccount{ID: "side", Primary: true},
			want: "[[account]]\nid = \"work\"\nlabel = \"\"\"\n[[account]]\nprimary = true\n\"\"\"\n\n" +
				"[[account]]\nid = \"personal\"\nlabel = '''\n# Not a comment.\n\n[prime]\n'''\n\n[[account]]\nid      = \"side\"\nprimary = true\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := draft.AddAccount(tt.account); err != nil {
				t.Fatalf("AddAccount() error = %v", err)
			}
			save(t, draft)
			if got := readFile(t, path); got != tt.want {
				t.Errorf("the config reads\n%s\nwant\n%s", got, tt.want)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if added := cfg.Accounts[len(cfg.Accounts)-1]; added.ID != tt.account.ID || added.Label != cmp.Or(tt.account.Label, tt.account.ID) {
				t.Errorf("the account added loads as %+v, want %+v", added, tt.account)
			}
			if !reflect.DeepEqual(draft.Config(), cfg) {
				t.Errorf("Config() = %+v, want what the file saved loads as, %+v", draft.Config(), cfg)
			}
		})
	}
}

func TestRemoveAccount(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		want   string
	}{
		{
			name:   "the last, leaving the comments after it",
			config: commented,
			id:     "personal",
			want: `# One [[account]] per Claude subscription.

[[account]]
id      = "work"  # for good
label   = "Work"  # optional; defaults to the id
primary = true    # optional: the account the browser and the Claude apps use
reserve = 0.1     # optional

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`,
		},
		{
			name:   "the first, and the primary",
			config: commented,
			id:     "work",
			want: `# One [[account]] per Claude subscription.

[[account]]
id    = "personal"
label = "Personal"

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`,
		},
		{
			name:   "one between others",
			config: "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n\n[[account]]\nid = \"spare\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"spare\"\n",
		},
		{
			name:   "with the comments directly above it, but not those set apart from it",
			config: "[[account]]\nid = \"work\"\n\n# Accounts for the weekend.\n\n# Side projects.\n# Mostly Sonnet.\n[[account]]\nid = \"side\"\n\n[[account]]\nid = \"spare\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n\n# Accounts for the weekend.\n\n[[account]]\nid = \"spare\"\n",
		},
		{
			name:   "with the comments directly below its keys",
			config: "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n# reserve = 0.2\n\n[prime]\nday = \"08:00-23:00\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n\n[prime]\nday = \"08:00-23:00\"\n",
		},
		{
			name:   "at the start of the file",
			config: "[[account]]\nid = \"work\"\n\n\n[[account]]\nid = \"side\"\n",
			id:     "work",
			want:   "[[account]]\nid = \"side\"\n",
		},
		{
			name:   "at the end of the file",
			config: "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n",
		},
		{
			name:   "between tables with no blank lines between them",
			config: "[[account]]\nid = \"work\"\n[[account]]\nid = \"side\"\n[prime]\nday = \"08:00-23:00\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n[prime]\nday = \"08:00-23:00\"\n",
		},
		{
			name:   "with a label that goes on over lines",
			config: "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\nlabel = \"\"\"\nSide\n\n[[account]]\n\"\"\"\n\n[[account]]\nid = \"spare\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"spare\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := draft.RemoveAccount(tt.id); err != nil {
				t.Fatalf("RemoveAccount() error = %v", err)
			}
			save(t, draft)
			if got := readFile(t, path); got != tt.want {
				t.Errorf("the config reads\n%s\nwant\n%s", got, tt.want)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(draft.Config(), cfg) {
				t.Errorf("Config() = %+v, want what the file saved loads as, %+v", draft.Config(), cfg)
			}
		})
	}
}

func TestEditsLoadAsMeant(t *testing.T) {
	path := writeConfig(t, commented)
	steps := []struct {
		name string
		edit func(*config.Draft) error
		want []config.Account
	}{
		{
			name: "add side",
			edit: func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "side", Label: "Side"}) },
			want: []config.Account{
				{ID: "work", Label: "Work", Primary: true, Reserve: 0.1},
				{ID: "personal", Label: "Personal"},
				{ID: "side", Label: "Side"},
			},
		},
		{
			name: "add spare, the primary",
			edit: func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "spare", Primary: true}) },
			want: []config.Account{
				{ID: "work", Label: "Work", Reserve: 0.1},
				{ID: "personal", Label: "Personal"},
				{ID: "side", Label: "Side"},
				{ID: "spare", Label: "spare", Primary: true},
			},
		},
		{
			name: "remove spare",
			edit: func(d *config.Draft) error { return d.RemoveAccount("spare") },
			want: []config.Account{
				{ID: "work", Label: "Work", Primary: true, Reserve: 0.1},
				{ID: "personal", Label: "Personal"},
				{ID: "side", Label: "Side"},
			},
		},
		{
			name: "remove work",
			edit: func(d *config.Draft) error { return d.RemoveAccount("work") },
			want: []config.Account{
				{ID: "personal", Label: "Personal", Primary: true},
				{ID: "side", Label: "Side"},
			},
		},
	}
	for _, step := range steps {
		draft := edit(t, path)
		if err := step.edit(draft); err != nil {
			t.Fatalf("%s: error = %v", step.name, err)
		}
		save(t, draft)
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatalf("%s: Load() error = %v", step.name, err)
		}
		if !reflect.DeepEqual(cfg.Accounts, config.Accounts(step.want)) {
			t.Errorf("%s: the accounts load as\n%+v\nwant\n%+v", step.name, cfg.Accounts, step.want)
		}
	}
}

func TestAddingAndRemovingAnAccountLeavesTheConfigAsItWas(t *testing.T) {
	for _, before := range []string{
		commented,
		config.Example,
		"listen = \"127.0.0.1:4747\"\n\n[[account]]\nid = \"work\"\n\n[notifications]\nmoves = true\n",
		"[[account]]\nid = \"work\"\n\n[history]\nkeep = \"30d\"\n",
		"[[account]]\nid = \"work\"\n\n[ledger]\nkeep = \"120d\"\n",
	} {
		path := writeConfig(t, before)
		for _, change := range []func(*config.Draft) error{
			func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "side", Label: "Side"}) },
			func(d *config.Draft) error { return d.RemoveAccount("side") },
		} {
			draft := edit(t, path)
			if err := change(draft); err != nil {
				t.Fatal(err)
			}
			save(t, draft)
		}
		if got := readFile(t, path); got != before {
			t.Errorf("once side is added and removed, the config reads\n%s\nwant it as it was\n%s", got, before)
		}
	}
}

func TestAddAccountRefuses(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		account config.NewAccount
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
		wantIs  error
	}{
		{
			name:    "an id an account has already",
			config:  commented,
			account: config.NewAccount{ID: "personal", Label: "Personal again"},
			wantErr: `account "personal" is already configured`,
			wantIs:  config.ErrConfigured,
		},
		{
			name:    "an id an account has already, in another case",
			config:  commented,
			account: config.NewAccount{ID: "Personal"},
			wantErr: `account ids "personal" and "Personal" differ only in case, so they'd share a token file, as macOS ignores case in file names`,
		},
		{
			name:    "an id that isn't a plain file name",
			config:  commented,
			account: config.NewAccount{ID: "../side"},
			wantErr: `account "../side": id must start with a letter or digit and contain only letters, digits, '-' and '_'`,
		},
		{
			name:    "the id pin takes to mean routing",
			config:  commented,
			account: config.NewAccount{ID: "Auto"},
			wantErr: `account "Auto": id is reserved for switchboard pin auto`,
		},
		{
			name:    "no id",
			config:  commented,
			account: config.NewAccount{},
			wantErr: `account "": id is required`,
		},
		{
			name:    "an id that looks like a token, never quoted",
			config:  commented,
			account: config.NewAccount{ID: tokenShaped},
			wantErr: `account "[redacted]": id looks like a token, which an id mustn't, as it shows wherever the account does`,
		},
		{
			name:    "a label that looks like a token, never quoted",
			config:  commented,
			account: config.NewAccount{ID: "side", Label: "Side " + tokenShaped},
			wantErr: `account "side": label looks like a token, which a label mustn't, as it shows wherever the account does`,
		},
		{
			name:    "accounts listed inline, which a table can't follow",
			config:  "account = [{ id = \"work\" }]\n",
			account: config.NewAccount{ID: "side"},
			wantErr: `editing %s as text wouldn't make exactly the change meant, so it's left as it was: add account "side" by hand`,
		},
		{
			name:    "the primary marked in a way the edit doesn't find",
			config:  "[[account]]\nid = \"work\"\n\"pri\\u006Dary\" = true\n",
			account: config.NewAccount{ID: "side", Primary: true},
			wantErr: `editing %s as text wouldn't make exactly the change meant, so it's left as it was: add account "side" by hand`,
		},
		{
			name:    "a config without accounts, but with problems",
			config:  "listen = \"0.0.0.0:4747\"\n",
			account: config.NewAccount{ID: "work"},
			wantErr: "invalid config %s:\nlisten \"0.0.0.0:4747\": host must be a loopback IP address, 127.0.0.1 or ::1, so no other machine can use the proxy's tokens; a name, even localhost, can lead a client to another address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := draft.AddAccount(tt.account)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, tt.wantIs)
		})
	}
}

func TestRemoveAccountRefuses(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
		wantIs  error
	}{
		{
			name:    "an id no account has",
			config:  commented,
			id:      "side",
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "a token given as the id, never quoted",
			config:  commented,
			id:      tokenShaped,
			wantErr: `account "[redacted]" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "an id in another case",
			config:  commented,
			id:      "Work",
			wantErr: `account "Work" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "the only account",
			config:  "[[account]]\nid = \"work\"\n",
			id:      "work",
			wantErr: `account "work" is the only one, and a config needs one at least`,
		},
		{
			name:    "accounts listed inline, which a table can't follow",
			config:  "account = [{ id = \"work\" }, { id = \"side\" }]\n",
			id:      "side",
			wantErr: `editing %s as text wouldn't make exactly the change meant, so it's left as it was: remove account "side" by hand`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := draft.RemoveAccount(tt.id)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, tt.wantIs)
		})
	}
}

func TestSetPrimary(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		want   string
	}{
		{
			name:   "none marked: below the account's last key, above the comments directly below it",
			config: "[[account]]\nid = \"work\"\n\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\n# reserve = 0.2\n\n[prime]\nday = \"08:00-23:00\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\nprimary = true\n# reserve = 0.2\n\n[prime]\nday = \"08:00-23:00\"\n",
		},
		{
			name:   "the first, the primary only for want of a mark",
			config: "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n",
			id:     "work",
			want:   "[[account]]\nid = \"work\"\nprimary = true\n\n[[account]]\nid = \"side\"\n",
		},
		{
			name:   "taking primary off the account marked",
			config: commented,
			id:     "personal",
			want: `# One [[account]] per Claude subscription.

[[account]]
id      = "work"  # for good
label   = "Work"  # optional; defaults to the id
reserve = 0.1     # optional

[[account]]
id    = "personal"
label = "Personal"
primary = true

# Optional: start the 5-hour windows on a staggered schedule.
# [prime]
# day = "08:00-23:00"
`,
		},
		{
			name:   "in place of its own primary = false, leaving another's",
			config: "[[account]]\nid = \"work\"\nprimary = false\n\n[[account]]\nid = \"side\"\nprimary = false\nlabel = \"Side\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\nprimary = false\n\n[[account]]\nid = \"side\"\nlabel = \"Side\"\nprimary = true\n",
		},
		{
			name:   "below a label that goes on over lines",
			config: "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\nlabel = \"\"\"\nSide\n[[account]]\n\"\"\"\n",
			id:     "side",
			want:   "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\nlabel = \"\"\"\nSide\n[[account]]\n\"\"\"\nprimary = true\n",
		},
		{
			name:   "the primary already, left as it is",
			config: commented,
			id:     "work",
			want:   commented,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := draft.SetPrimary(tt.id); err != nil {
				t.Fatalf("SetPrimary() error = %v", err)
			}
			save(t, draft)
			if got := readFile(t, path); got != tt.want {
				t.Errorf("the config reads\n%s\nwant\n%s", got, tt.want)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if primary := cfg.Accounts.Primary(); primary.ID != tt.id || !draft.MarksPrimary() {
				t.Errorf("the primary loads as %s, marked: %v; want %s, marked", primary.ID, draft.MarksPrimary(), tt.id)
			}
			if !reflect.DeepEqual(draft.Config(), cfg) {
				t.Errorf("Config() = %+v, want what the file saved loads as, %+v", draft.Config(), cfg)
			}
		})
	}
}

func TestSetPrimaryRefuses(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
		wantIs  error
	}{
		{
			name:    "an id no account has",
			config:  commented,
			id:      "side",
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "a token given as the id, never quoted",
			config:  commented,
			id:      tokenShaped,
			wantErr: `account "[redacted]" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "accounts listed inline, which a key can't be put in",
			config:  "account = [{ id = \"work\" }, { id = \"side\" }]\n",
			id:      "side",
			wantErr: `editing %s as text wouldn't make exactly the change meant, so it's left as it was: make account "side" the primary by hand`,
		},
		{
			name:    "the primary marked in a way the edit doesn't find",
			config:  "[[account]]\nid = \"work\"\n\"pri\\u006Dary\" = true\n\n[[account]]\nid = \"side\"\n",
			id:      "side",
			wantErr: `editing %s as text wouldn't make exactly the change meant, so it's left as it was: make account "side" the primary by hand`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := draft.SetPrimary(tt.id)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, tt.wantIs)
		})
	}
}

func TestMarksPrimary(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   bool
	}{
		{name: "an account marked", config: commented, want: true},
		{name: "none marked", config: "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n"},
		{name: "one marked false", config: "[[account]]\nid = \"work\"\nprimary = false\n"},
		{name: "no config", config: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := edit(t, writeConfig(t, tt.config)).MarksPrimary(); got != tt.want {
				t.Errorf("MarksPrimary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetPrimeDay(t *testing.T) {
	const work = "[[account]]\nid = \"work\"\n"
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{
			name:   "in a [prime] table made at the end",
			config: work,
			want:   work + "\n[prime]\nday = \"08:00-23:00\"\n",
		},
		{
			name:   "past a [prime] table left commented out",
			config: config.Example,
			want:   config.Example + "\n[prime]\nday = \"08:00-23:00\"\n",
		},
		{
			name:   "in place of the day given, keeping the comment beside it",
			config: "[prime]\nday = \"\"  # off for now\n\n" + work,
			want:   "[prime]\nday = \"08:00-23:00\"  # off for now\n\n" + work,
		},
		{
			name:   "in place of a day given as a literal string",
			config: "[prime]\n'day'='06:00-21:00'\n\n" + work,
			want:   "[prime]\n'day'=\"08:00-23:00\"\n\n" + work,
		},
		{
			name:   "as the first key of a [prime] table without one",
			config: work + "\n# The day's windows.\n[prime]\n# Local time.\n\n[notifications]\nmoves = true\n",
			want:   work + "\n# The day's windows.\n[prime]\nday = \"08:00-23:00\"\n# Local time.\n\n[notifications]\nmoves = true\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := draft.SetPrimeDay("08:00-23:00"); err != nil {
				t.Fatalf("SetPrimeDay() error = %v", err)
			}
			save(t, draft)
			if got := readFile(t, path); got != tt.want {
				t.Errorf("the config reads\n%s\nwant\n%s", got, tt.want)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !cfg.Prime.On() || cfg.Prime.Day.String() != "08:00-23:00" {
				t.Errorf("the day loads as %v, on: %v; want 08:00-23:00, on", cfg.Prime.Day, cfg.Prime.On())
			}
			if !reflect.DeepEqual(draft.Config(), cfg) {
				t.Errorf("Config() = %+v, want what the file saved loads as, %+v", draft.Config(), cfg)
			}
		})
	}
}

func TestSetPrimeDayRefuses(t *testing.T) {
	const work = "[[account]]\nid = \"work\"\n"
	tests := []struct {
		name   string
		config string
		day    string
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
	}{
		{
			name:    "a day that isn't one",
			config:  work,
			day:     "8-23",
			wantErr: `prime.day "8-23": must be two times of day, HH:MM, joined by -, such as 08:00-23:00`,
		},
		{
			name:    "a day that ends as it starts",
			config:  work,
			day:     "08:00-08:00",
			wantErr: `prime.day "08:00-08:00": must end at another time than it starts; an end before the start is past midnight`,
		},
		{
			name:    "priming given as an inline table, which a table can't follow",
			config:  "prime = { day = \"\" }\n\n" + work,
			day:     "08:00-23:00",
			wantErr: `editing %s as text wouldn't make exactly the change meant, so it's left as it was: set prime.day to "08:00-23:00" by hand`,
		},
		{
			name:    "the day given as a dotted key",
			config:  "prime.day = \"\"\n\n" + work,
			day:     "08:00-23:00",
			wantErr: `editing %s as text wouldn't make exactly the change meant, so it's left as it was: set prime.day to "08:00-23:00" by hand`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := draft.SetPrimeDay(tt.day)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, nil)
		})
	}
}

// checkRefused checks an edit of the config at path, which held before,
// failed with wantErr, %s standing for the path, and wantIs when it's given,
// and that the draft, saved, leaves the config as it was.
func checkRefused(t *testing.T, path, before string, draft *config.Draft, err error, wantErr string, wantIs error) {
	t.Helper()
	if want := strings.ReplaceAll(wantErr, "%s", path); err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	if wantIs != nil && !errors.Is(err, wantIs) {
		t.Errorf("error = %v, want one matching %v", err, wantIs)
	}
	save(t, draft)
	if got := readFile(t, path); got != before {
		t.Errorf("the config reads\n%s\nwant it as it was\n%s", got, before)
	}
}

func TestEditRefusesAConfigItCantRead(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{name: "not TOML", config: "listen = \"127.0.0.1:4747\n", wantErr: "parse config %s: toml: line 1"},
		{
			name:    "not TOML, a token given twice as a key",
			config:  tokenShaped + " = 1\n" + tokenShaped + " = 2\n",
			wantErr: `parse config %s: toml: line 2 (last key "[redacted]"): Key '[redacted]' has already been defined.`,
		},
		{name: "invalid", config: "[[account]]\nid = \"work\"\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n", wantErr: "invalid config %s:\n" + `unknown key "account.token_env"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := config.Edit(path)
			if want := strings.ReplaceAll(tt.wantErr, "%s", path); err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Errorf("Edit() error = %v, want one starting %q", err, want)
			}
		})
	}
}

func TestSaveMakesTheConfigWithItsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "switchboard", "config.toml")
	draft := edit(t, path)
	if draft.Config() != nil {
		t.Errorf("Config() = %+v before any account, want nil", draft.Config())
	}
	if err := draft.AddAccount(config.NewAccount{ID: "work", Label: "Work"}); err != nil {
		t.Fatal(err)
	}

	save(t, draft)
	if want := "[[account]]\nid    = \"work\"\nlabel = \"Work\"\n"; readFile(t, path) != want {
		t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
	}
	checkMode(t, path, 0o644)
	if cfg, err := config.Load(path); err != nil || cfg.Upstream != "https://api.anthropic.com" {
		t.Errorf("Load() = %+v, %v, want the config with its defaults", cfg, err)
	}
}

func TestSaveKeepsTheConfigsPermissions(t *testing.T) {
	path := writeConfig(t, commented)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	draft := edit(t, path)
	if err := draft.AddAccount(config.NewAccount{ID: "side"}); err != nil {
		t.Fatal(err)
	}

	save(t, draft)
	checkMode(t, path, 0o640)
}

func TestSaveWritesThroughLinks(t *testing.T) {
	tests := []struct {
		name string
		// link puts links at path, in the config directory, leading to target,
		// in the dotfiles directory, which is where the dotfiles are.
		link func(t *testing.T, path, dotfiles string)
		// target is where the links lead, in the dotfiles directory.
		target string
		// missing leaves no file where the links lead.
		missing bool
	}{
		{
			name:   "a link",
			target: "config.toml",
			link: func(t *testing.T, path, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "config.toml"), path)
			},
		},
		{
			name:   "a relative link, through a link to its directory",
			target: filepath.Join("switchboard", "config.toml"),
			link: func(t *testing.T, path, dotfiles string) {
				// The config directory's parent is itself a link, into the
				// dotfiles, where "..", taken from the link, leads.
				parent := filepath.Dir(filepath.Dir(path))
				mkdir(t, filepath.Join(dotfiles, "linked", "config"))
				if err := os.RemoveAll(parent); err != nil {
					t.Fatal(err)
				}
				symlink(t, filepath.Join(dotfiles, "linked"), parent)
				symlink(t, filepath.Join("..", "..", "switchboard", "config.toml"), path)
			},
		},
		{
			name:   "a link to a link",
			target: "config.toml",
			link: func(t *testing.T, path, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "config.toml"), filepath.Join(dotfiles, "current.toml"))
				symlink(t, filepath.Join(dotfiles, "current.toml"), path)
			},
		},
		{
			name:    "a link to a file that isn't there yet",
			target:  "config.toml",
			missing: true,
			link: func(t *testing.T, path, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "config.toml"), path)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dotfiles := filepath.Join(root, "dotfiles")
			path := filepath.Join(root, "home", "config", "config.toml")
			mkdir(t, filepath.Dir(path))
			target := filepath.Join(dotfiles, tt.target)
			mkdir(t, filepath.Dir(target))
			before := ""
			if !tt.missing {
				before = "[[account]]\nid = \"work\"\n"
				writeFile(t, target, before)
			}
			tt.link(t, path, dotfiles)
			draft := edit(t, path)
			if err := draft.AddAccount(config.NewAccount{ID: "side"}); err != nil {
				t.Fatal(err)
			}

			save(t, draft)
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("the config is %v, want it a link still", info.Mode())
			}
			want := strings.TrimPrefix(before+"\n", "\n") + "[[account]]\nid = \"side\"\n"
			if got := readFile(t, target); got != want {
				t.Errorf("where the link leads reads\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestEditRefusesALoopOfLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	loopOfLinks(t, path)

	_, err := config.Edit(path)
	if err == nil || !strings.HasPrefix(err.Error(), "read config: ") {
		t.Errorf("Edit() error = %v, want it to fail reading the config", err)
	}
}

func TestSaveRefusesALoopOfLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	draft := edit(t, path)
	if err := draft.AddAccount(config.NewAccount{ID: "work"}); err != nil {
		t.Fatal(err)
	}
	loopOfLinks(t, path)

	err := draft.Save()
	if want := "write config: " + path + ": too many links"; err == nil || err.Error() != want {
		t.Errorf("Save() error = %v, want %q", err, want)
	}
}

// loopOfLinks puts a link at path to a link that leads back to it.
func loopOfLinks(t *testing.T, path string) {
	t.Helper()
	other := filepath.Join(filepath.Dir(path), "other.toml")
	symlink(t, other, path)
	symlink(t, path, other)
}

func TestSaveRefusesAConfigChangedMeanwhile(t *testing.T) {
	tests := []struct {
		name string
		// before is what the config holds as it's read: nothing when nil.
		before *string
		// meanwhile is what it holds when it's saved: nothing when nil.
		meanwhile *string
	}{
		{name: "changed", before: new(commented), meanwhile: new(commented + "\n[prime]\nday = \"08:00-23:00\"\n")},
		{name: "made", meanwhile: new("[[account]]\nid = \"personal\"\n")},
		{name: "removed", before: new(commented)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if tt.before != nil {
				writeFile(t, path, *tt.before)
			}
			draft := edit(t, path)
			if err := draft.AddAccount(config.NewAccount{ID: "side"}); err != nil {
				t.Fatal(err)
			}
			if tt.meanwhile != nil {
				writeFile(t, path, *tt.meanwhile)
			} else if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}

			err := draft.Save()
			if want := path + " changed while it was being edited, so it's left as it is: try again"; err == nil || err.Error() != want {
				t.Errorf("Save() error = %v, want %q", err, want)
			}
			got, err := os.ReadFile(path)
			if (tt.meanwhile == nil && !errors.Is(err, fs.ErrNotExist)) || (tt.meanwhile != nil && string(got) != *tt.meanwhile) {
				t.Errorf("the config reads %q (%v), want what it held meanwhile", got, err)
			}
		})
	}
}

// edit returns a draft of the config file at path.
func edit(t *testing.T, path string) *config.Draft {
	t.Helper()
	draft, err := config.Edit(path)
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	return draft
}

// save saves the draft.
func save(t *testing.T, draft *config.Draft) {
	t.Helper()
	if err := draft.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// checkMode checks the file at path has the permissions perm.
func checkMode(t *testing.T, path string, perm fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != perm {
		t.Errorf("%s has mode %v, want %v", path, info.Mode(), perm)
	}
}

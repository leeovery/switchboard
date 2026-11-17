package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// copiedConfig is a config a copy is made of, its pool fenced to a folder in
// the home directory.
const copiedConfig = `[[account]]
id    = "b6s7"
label = "Work"
pool  = "b2n8"

[[pool]]
id    = "b2n8"
label = "Default"
dirs  = ["~/Code"]
`

// inHome is a home directory a pool's ~/ is found in.
func inHome() (string, error) {
	return "/Users/someone", nil
}

// fullConfig is a config giving a value of every kind a copy writes anew:
// keys at the top, tables, arrays of tables, an array across lines, a
// float, an integer, nested tables, an escape, and comments.
const fullConfig = `# switchboard's config
listen   = "127.0.0.1:4747"  # where the router listens
upstream = "https://gw.example.com/anthropic"
week_starts = "sunday"

[[account]]
id      = "b6s7"  # do not edit: links the account to its history
label   = "Work \"main\""
primary = true
reserve = 0.1
plan    = "max"
pool    = "b2n8"

[[account]]
id    = "q8rt"
label = "Acme"
pool  = "q7mt"

[[pool]]
id    = "b2n8"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = [
  "~/Code/acme",  # the client's work
  "/Volumes/Work/acme",
]

[prime]
day = "08:00-23:00"

[notifications]
warning = 0.85
moves   = true

[ledger]
keep = "forever"

[prices.models.claude-opus-5-5]
input  = 15
output = 75.5
`

// fullCopy is the copy of fullConfig.
const fullCopy = `listen = "127.0.0.1:4747"
upstream = "https://gw.example.com/anthropic"
week_starts = "sunday"

[[account]]
id = "b6s7"
label = "Work \"main\""
plan = "max"
pool = "b2n8"
primary = true
reserve = 0.1

[[account]]
id = "q8rt"
label = "Acme"
pool = "q7mt"

[ledger]
keep = "forever"

[notifications]
moves = true
warning = 0.85

[[pool]]
id = "b2n8"
label = "Default"

[[pool]]
dirs = ["~/Code/acme", "/Volumes/Work/acme"]
id = "q7mt"
label = "Acme"

[prices]
[prices.models]
[prices.models.claude-opus-5-5]
input = 15
output = 75.5

[prime]
day = "08:00-23:00"
`

// A copy of a config holds its values alone, written anew: each table's keys
// in order, then its tables, none of its comments nor its layout; and it
// loads as the config does.
func TestACopyOfAConfigHoldsItsValuesAlone(t *testing.T) {
	got, err := config.Copy("config.toml", []byte(fullConfig), inHome)
	if err != nil {
		t.Fatalf("Copy() = %v, want a copy", err)
	}
	if string(got) != fullCopy {
		t.Errorf("Copy() =\n%s\nwant\n%s", got, fullCopy)
	}
	original, err := config.Parse("config.toml", []byte(fullConfig), inHome)
	if err != nil {
		t.Fatal(err)
	}
	if copied, err := config.Parse("config.toml", got, inHome); err != nil || !reflect.DeepEqual(copied, original) {
		t.Errorf("the copy loads as %+v, %v; want the config, %+v", copied, err, original)
	}
}

// No comment reaches a copy, whatever it holds: a token, an address with
// credentials and a key, a comment after a value, or a token wrapped across
// two comments' lines.
func TestNoCommentReachesACopy(t *testing.T) {
	tests := []struct {
		name, data string
	}{
		{name: "a token", data: "# pasted by mistake: " + tokenShaped + "\n" + copiedConfig},
		{name: "an address with credentials and a key", data: "# upstream = \"https://user:test-secret@gw.example.com/v1?key=test-secret\"\n" + copiedConfig},
		{name: "after the upstream", data: "upstream = \"https://gw.example.com\"  # user:test-secret@gw.example.com\n" + copiedConfig},
		{name: "a token wrapped across two lines", data: "# sk-ant-oat01-fake_token-\n# test-secret-shaped-tail\n" + copiedConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Copy("config.toml", []byte(tt.data), inHome)
			if err != nil {
				t.Fatalf("Copy() = %v, want a copy", err)
			}
			if text := string(got); strings.Contains(text, "#") || strings.Contains(text, "test-secret") || strings.Contains(text, "sk-ant-") {
				t.Errorf("Copy() = %q, want no comment in it", got)
			}
		})
	}
}

// A copy laid down on another Mac is edited there as any config is: setup,
// and every verb that edits a config, makes its change in it, which loads.
func TestACopyIsEditedAsAnyConfigIs(t *testing.T) {
	tests := []struct {
		name string
		edit func(d *config.Draft) error
	}{
		{name: "an account added", edit: func(d *config.Draft) error {
			return d.AddAccount(config.NewAccount{ID: "m2x7", Label: "Mine", Plan: "pro", Primary: true, Pool: "q7mt"})
		}},
		{name: "an account removed", edit: func(d *config.Draft) error { return d.RemoveAccount("q8rt") }},
		{name: "a label set", edit: func(d *config.Draft) error { return d.SetLabel("b6s7", "Office") }},
		{name: "an id set", edit: func(d *config.Draft) error { return d.SetID("q8rt", "q9rt") }},
		{name: "a plan set", edit: func(d *config.Draft) error { return d.SetPlan("q8rt", "ultra") }},
		{name: "a reserve set", edit: func(d *config.Draft) error { return d.SetReserve("b6s7", 0.25) }},
		{name: "the primary unmarked", edit: func(d *config.Draft) error { return d.UnmarkPrimary("b6s7") }},
		{name: "an account moved to another pool", edit: func(d *config.Draft) error { return d.SetPool("q8rt", "b2n8") }},
		{name: "the primary set", edit: func(d *config.Draft) error { return d.SetPrimary("q8rt") }},
		{name: "a pool added", edit: func(d *config.Draft) error {
			return d.AddPool(config.NewPool{ID: "h0me", Label: "Home", Dirs: []string{"~/Home"}})
		}},
		{name: "a pool relabelled", edit: func(d *config.Draft) error { return d.SetPoolLabel("q7mt", "Client") }},
		{name: "a pool fenced anew", edit: func(d *config.Draft) error { return d.SetPoolDirs("q7mt", []string{"~/Client"}) }},
		{name: "a pool removed", edit: func(d *config.Draft) error {
			if err := d.RemoveAccount("q8rt"); err != nil {
				return err
			}
			return d.RemovePool("q7mt")
		}},
		{name: "priming's day set", edit: func(d *config.Draft) error { return d.SetPrimeDay("09:00-21:00") }},
	}
	copied, err := config.Copy("config.toml", []byte(fullConfig), inHome)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := config.LayDown(path, copied); err != nil {
				t.Fatal(err)
			}
			draft, err := config.Edit(path, inHome)
			if err != nil {
				t.Fatalf("Edit() = %v", err)
			}
			if err := tt.edit(draft); err != nil {
				t.Fatalf("the edit = %v, want it made", err)
			}
			if err := draft.Save(); err != nil {
				t.Fatalf("Save() = %v", err)
			}
			if _, err := config.Load(path, inHome); err != nil {
				t.Errorf("edited, the copy loads as %v, want a config", err)
			}
		})
	}
}

// A config giving a value that may hold a secret, or that doesn't load,
// makes no copy: the value is judged as the config is read, decoded, never
// by its text, so a token written with escapes, or split across lines, is
// found; hiding a secret would make another config; and a copy that doesn't
// load is none to lay down.
func TestNoCopyIsMadeOfAConfigGivingASecretOrThatDoesntLoad(t *testing.T) {
	tests := []struct {
		name string
		data string
		want error
	}{
		{name: "a token in the upstream's path", data: `upstream = "https://gateway.example.com/` + tokenShaped + `"`, want: config.ErrSecretValue},
		{name: "a token in the upstream's credentials", data: `upstream = "https://user:` + tokenShaped + `@gateway.example.com"`, want: config.ErrSecretValue},
		{name: "credentials in the upstream", data: `upstream = "https://user:test-secret@gateway.example.com"`, want: config.ErrSecretValue},
		{name: "a key in the upstream's query", data: `upstream = "https://gateway.example.com/v1?key=test-secret"`, want: config.ErrSecretValue},
		{name: "a key in the upstream's fragment", data: `upstream = "https://gateway.example.com/v1#key=test-secret"`, want: config.ErrSecretValue},
		{name: "a token as a key", data: "[prices.models]\n" + tokenShaped + " = {input = 1.0}", want: config.ErrSecretValue},
		{name: "a token written with an escape", data: `upstream = "https://gateway.example.com/\u0073k-ant-oat01-fake_token-shaped"`, want: config.ErrSecretValue},
		{name: "a token split across lines", data: "upstream = \"\"\"https://gateway.example.com/sk-\\\n  ant-oat01-fake_token-shaped\"\"\"", want: config.ErrSecretValue},
		{name: "a token written with an escape in a label", data: "[[account]]\nid = \"m2x7\"\nlabel = \"\\u0073k-ant-oat01-fake_token-shaped\"\npool = \"b2n8\"\n"},
		{name: "a config that doesn't load", data: "[[account]]\nid = 7\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Copy("config.toml", []byte(tt.data+"\n"+copiedConfig), inHome)
			if err == nil || tt.want != nil && !errors.Is(err, tt.want) || strings.Contains(err.Error(), "sk-ant-") || strings.Contains(err.Error(), "test-secret") {
				t.Errorf("Copy() = %q, %v; want no copy, failing with %v, saying no secret", got, err, tt.want)
			}
		})
	}
}

// Members reads each account's id and pool alone, whatever else the config
// gives, as a newer switchboard's may give keys this one doesn't know.
func TestMembersReadsEachAccountsIDAndPoolAlone(t *testing.T) {
	data := []byte(copiedConfig + `
[[account]]
id       = "qgr2"
pool     = "q7mt"
sometime = "a key a newer switchboard reads"
`)
	got, err := config.Members(data)
	want := config.Accounts{{ID: "b6s7", Pool: "b2n8"}, {ID: "qgr2", Pool: "q7mt"}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("Members() = %+v, %v; want %+v", got, err, want)
	}
	if _, err := config.Members([]byte("[[account]\n")); err == nil {
		t.Error("Members() of what isn't TOML = nil, want it to fail")
	}
}

// Laying a config down writes it, its folder made, where nothing is at its
// path, leaving nothing beside it, each id marked, as switchboard marks
// those it writes, so none is edited.
func TestLayingAConfigDownWritesItWhereThereIsNone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "switchboard")
	path := filepath.Join(dir, "config.toml")
	const marked = `[[account]]
id    = "b6s7"  # do not edit: links the account to its history
label = "Work"
pool  = "b2n8"

[[pool]]
id    = "b2n8"  # do not edit: links the pool to its history
label = "Default"
dirs  = ["~/Code"]
`

	if err := config.LayDown(path, []byte(copiedConfig)); err != nil {
		t.Fatalf("LayDown() = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != marked {
		t.Errorf("the config laid down holds %q (%v), want the copy, its ids marked, %q", got, err, marked)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the config laid down: %v, %v; want it 0644, as a config switchboard writes is", info, err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Errorf("the config's folder holds %v (%v), want the config alone", entries, err)
	}
}

// Laying a config down never writes in place of anything at its path: a
// config, or a link, even one that leads nowhere, as a dotfiles repository's
// not yet cloned.
func TestLayingAConfigDownNeverReplacesWhatsThere(t *testing.T) {
	tests := []struct {
		name  string
		there func(t *testing.T, path string)
	}{
		{name: "a config", there: func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("# mine\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a link leading nowhere", there: func(t *testing.T, path string) {
			if err := os.Symlink(filepath.Join(filepath.Dir(path), "dotfiles", "config.toml"), path); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			tt.there(t, path)
			was, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			mine, _ := os.ReadFile(path)

			if err := config.LayDown(path, []byte(copiedConfig)); !errors.Is(err, fs.ErrExist) {
				t.Errorf("LayDown() over %s = %v, want it refused, as something is there", tt.name, err)
			}
			now, err := os.Lstat(path)
			if got, _ := os.ReadFile(path); err != nil || now.Mode() != was.Mode() || string(got) != string(mine) {
				t.Errorf("once refused, %s is %v (%v), holding %q; want it as it was", tt.name, now, err, got)
			}
			if entries, err := os.ReadDir(filepath.Dir(path)); err != nil || len(entries) != 1 {
				t.Errorf("the config's folder holds %v (%v), want nothing left beside it", entries, err)
			}
		})
	}
}

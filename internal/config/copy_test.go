package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// A copy of a config holding nothing shaped like a token is the config as it
// is, byte for byte.
func TestACopyOfAConfigHoldingNoTokenIsTheConfig(t *testing.T) {
	got, err := config.Copy("config.toml", []byte(copiedConfig), inHome)
	if err != nil || string(got) != copiedConfig {
		t.Errorf("Copy() = %q, %v; want the config as it is", got, err)
	}
}

// A token in a comment is hidden in the copy, which loads as the config does.
func TestACopyOfAConfigHidesATokenInAComment(t *testing.T) {
	data := []byte("# pasted by mistake: " + tokenShaped + "\n" + copiedConfig)

	got, err := config.Copy("config.toml", data, inHome)
	if err != nil {
		t.Fatalf("Copy() = %v, want a copy", err)
	}
	if strings.Contains(string(got), "sk-ant-") || !strings.Contains(string(got), "# pasted by mistake: [redacted]\n") {
		t.Errorf("Copy() = %q, want the token hidden", got)
	}
	if cfg, err := config.Parse("config.toml", got, inHome); err != nil || len(cfg.Accounts) != 1 {
		t.Errorf("the copy loads as %+v, %v; want the config's one account", cfg, err)
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
// path, leaving nothing beside it.
func TestLayingAConfigDownWritesItWhereThereIsNone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "switchboard")
	path := filepath.Join(dir, "config.toml")

	if err := config.LayDown(path, []byte(copiedConfig)); err != nil {
		t.Fatalf("LayDown() = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != copiedConfig {
		t.Errorf("the config laid down holds %q (%v), want the copy", got, err)
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

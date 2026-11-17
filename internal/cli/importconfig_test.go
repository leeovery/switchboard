package cli_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/router"
)

// laidConfig is the config the folders of the import tests hold, as another
// Mac's export copies it: two accounts in two pools, their ids the history's.
const laidConfig = `[[account]]
id    = "b6s7"  # do not edit: links the account to its history
label = "Work"
pool  = "b2n8"

[[account]]
id    = "q8rt"  # do not edit: links the account to its history
label = "Acme"
pool  = "q7mt"

[[pool]]
id    = "b2n8"  # do not edit: links the pool to its history
label = "Default"

[[pool]]
id    = "q7mt"  # do not edit: links the pool to its history
label = "Acme"
dirs  = ["~/Code/acme"]
`

// ownConfig is the config a Mac of the import tests has of its own.
const ownConfig = "[[account]]\nid = \"m2x7\"\nlabel = \"Mine\"\n"

// hasConfig gives the Mac deps give a config of its own, and returns where.
func hasConfig(t *testing.T, deps cli.Deps) string {
	t.Helper()
	path := configAt(t, deps)
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(ownConfig))
	return path
}

// withConfig gives the Mac's folder at folder a copy of its Mac's config
// holding text, its export.json saying it holds one, and returns it.
func withConfig(t *testing.T, folder, text string) string {
	t.Helper()
	writeStateFile(t, folder, "config.toml", []byte(text))
	writeStateFile(t, folder, "export.json", []byte(`{"version":3,"install":"`+strings.TrimPrefix(filepath.Base(folder), "mac-")+`","exported_at":"2026-10-09T14:42:07Z","ids":true,"config":true}`+"\n"))
	return folder
}

// macFolder is the Mac's folder of the install with the given id in dir, as
// importFolder writes one.
func macFolder(t *testing.T, dir, install string) string {
	t.Helper()
	return importFolder(t, filepath.Join(dir, export.FolderName(install)))
}

// wantNoConfig fails t where the Mac deps give has a config.
func wantNoConfig(t *testing.T, deps cli.Deps) {
	t.Helper()
	if _, err := os.Lstat(configAt(t, deps)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the Mac's config: %v, want none laid down", err)
	}
}

// On a Mac with no config and no router, as a new Mac taking over from
// another, or restoring its backup, import lays down the config of the
// Mac's folder, its pools under the ids its history names, and says what
// it laid down and what to run next, staging nothing: the history waits for
// setup to give each account its token and start the router.
func TestImportOnAMacWithNoConfigLaysDownTheFoldersConfig(t *testing.T) {
	deps, state := importDeps(t)
	dir := t.TempDir()
	withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
	path := configAt(t, deps)

	want := result{stdout: "laid down the config of mac-0123456789abcdef at " + path + ": 2 accounts in 2 pools\n" +
		"next, run switchboard setup to give each account its token and start the router, then switchboard import " + dir + " again to bring in the history\n"}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != laidConfig {
		t.Errorf("the config laid down holds %q (%v), want the folder's", got, err)
	}
	if _, err := os.Stat(router.ImportDir(state)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: %v, want nothing staged", router.ImportDir(state), err)
	}
}

// The config laid down never holds a token, as a folder's copy edited by
// hand might: one in a comment is hidden, and a config giving one as a value
// isn't laid down.
func TestImportLaysDownNoToken(t *testing.T) {
	const token = "sk-ant-oat01-fake_token-shaped"
	t.Run("in a comment", func(t *testing.T) {
		deps, _ := importDeps(t)
		folder := withConfig(t, macFolder(t, t.TempDir(), "0123456789abcdef"), "# "+token+"\n"+laidConfig)

		if got := run(t, deps, "import", folder); got.code != 0 || strings.Contains(got.stdout+got.stderr, "sk-ant-") {
			t.Fatalf("switchboard import = %+v, want the config laid down, saying no token", got)
		}
		if got, err := os.ReadFile(configAt(t, deps)); err != nil || string(got) != "# [redacted]\n"+laidConfig {
			t.Errorf("the config laid down holds %q (%v), want the token hidden", got, err)
		}
	})
	t.Run("as a value", func(t *testing.T) {
		deps, _ := importDeps(t)
		folder := withConfig(t, macFolder(t, t.TempDir(), "0123456789abcdef"), `upstream = "https://gateway.example.com/`+token+`"`+"\n"+laidConfig)

		if got := run(t, deps, "import", folder); got.code != 1 || strings.Contains(got.stdout+got.stderr, "sk-ant-") || !strings.Contains(got.stderr, "can't be laid down") {
			t.Errorf("switchboard import = %+v, want it refused, saying why without the token", got)
		}
		wantNoConfig(t, deps)
	})
}

// Laying a config down is refused, laying none, where no folder holds one,
// where several Macs' folders hold configs that differ, and where a folder
// says it holds one a sync hasn't carried yet, each saying what to do;
// folders holding the one config, as Macs sharing it, lay it down.
func TestImportLaysDownOneConfigOrSaysWhy(t *testing.T) {
	other := strings.Replace(laidConfig, `"Work"`, `"Office"`, 1)
	tests := []struct {
		name string
		put  func(t *testing.T, dir string)
		want string
	}{
		{name: "none held, as an export from before the config was copied", put: func(t *testing.T, dir string) {
			macFolder(t, dir, "0123456789abcdef")
		}, want: "Error: this Mac has no config, and {dir} holds none to lay down, as an export from before exports copied the config holds none: run switchboard setup to make one, then import again\n"},
		{name: "several that differ", put: func(t *testing.T, dir string) {
			withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			withConfig(t, macFolder(t, dir, "fedcba9876543210"), other)
		}, want: "Error: this Mac has no config, and {dir} holds several Macs' configs: import from the folder of the Mac whose config to lay down, such as switchboard import {dir}/mac-fedcba9876543210\n"},
		{name: "one not synced", put: func(t *testing.T, dir string) {
			folder := withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			if err := os.Remove(filepath.Join(folder, "config.toml")); err != nil {
				t.Fatal(err)
			}
		}, want: "Error: this Mac has no config, and the one in {dir}/mac-0123456789abcdef hasn't synced yet: import again once it has\n"},
		{name: "one, held by two Macs sharing it", put: func(t *testing.T, dir string) {
			withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			withConfig(t, macFolder(t, dir, "fedcba9876543210"), laidConfig)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := importDeps(t)
			dir := t.TempDir()
			tt.put(t, dir)

			got := run(t, deps, "import", dir)
			if tt.want == "" {
				if got.code != 0 || !strings.HasPrefix(got.stdout, "laid down the config of mac-0123456789abcdef") {
					t.Errorf("switchboard import = %+v, want the one config laid down", got)
				}
				return
			}
			if want := (result{stderr: strings.ReplaceAll(tt.want, "{dir}", dir), code: 1}); got != want {
				t.Errorf("switchboard import = %+v, want %+v", got, want)
			}
			wantNoConfig(t, deps)
		})
	}
}

// Import says each pool of another Mac's it kept apart, why, and how to read
// it, once it has said what it took in.
func TestImportSaysEachPoolItKeptApart(t *testing.T) {
	deps, state := importDeps(t)
	imported := router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}, Apart: []router.ApartPool{
		{Install: "fedcba9876543210", Pool: "c1nt", Label: "Client", Why: identity.ApartSplit},
		{Install: "fedcba9876543210", Pool: "h0me", Label: "Home", Why: identity.ApartNoneHere},
		{Install: "0123456789abcdef", Pool: "0ld1", Label: "Old", Why: identity.ApartNoConfig},
	}}
	(&imports{post: answering(imported)}).serve(t, state)

	want := result{
		stdout: "imported 1 folder: 1 day added\n",
		stderr: `switchboard: kept the pool "Client" (c1nt) of mac-fedcba9876543210 apart, as its accounts are in more than one pool here: read it with --pool c1nt` + "\n" +
			`switchboard: kept the pool "Home" (h0me) of mac-fedcba9876543210 apart, as none of its accounts is configured here: read it with --pool h0me` + "\n" +
			`switchboard: kept the pool "Old" (0ld1) of mac-0123456789abcdef apart, as its folder holds no copy of that Mac's config to match its accounts by: read it with --pool 0ld1` + "\n",
	}
	if got := run(t, deps, "import", importFolder(t, t.TempDir())); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
}

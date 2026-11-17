package cli_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/tokens"
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

// laidAt is when the accounts of the import tests' folders were added.
var laidAt = time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

// laidRecord is the accounts record the folders of the import tests hold,
// as another Mac's export copies it, of laidConfig's accounts and pools.
var laidRecord = identity.Record{
	Accounts: []identity.Entry{
		{ID: "b6s7", Organization: "test-org-work", Label: "Work", Added: laidAt, Configured: true},
		{ID: "q8rt", Organization: "test-org-acme", Label: "Acme", Added: laidAt, Configured: true},
	},
	Pools: []identity.PoolEntry{
		{ID: "b2n8", Label: "Default", Added: laidAt, First: true, Configured: true},
		{ID: "q7mt", Label: "Acme", Added: laidAt, Configured: true},
	},
}

// unfencedAcme is the warning a config laid down from laidConfig gives, as
// the home of the tests holds no ~/Code/acme.
const unfencedAcme = `switchboard: ~/Code/acme isn't a directory, so no session starts in it yet: if it's on this Mac by another name, fence the pool "Acme" anew with switchboard pool edit q7mt --dirs` + "\n"

// ownConfig is the config a Mac of the import tests has of its own.
const ownConfig = "[[account]]\nid = \"m2x7\"\nlabel = \"Mine\"\n"

// newMacDeps are deps of a Mac with nothing of switchboard's yet, as a new
// one, and its state directory.
func newMacDeps(t *testing.T) (cli.Deps, string) {
	t.Helper()
	deps := testDeps(map[string]string{"XDG_STATE_HOME": shortTempDir(t)}, t.TempDir())
	return deps, stateDir(t, deps)
}

// hasConfig gives the Mac deps give a config of its own, and returns where.
func hasConfig(t *testing.T, deps cli.Deps) string {
	t.Helper()
	path := configAt(t, deps)
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(ownConfig))
	return path
}

// withConfig gives the Mac's folder at folder its Mac's accounts record, as
// laidRecord, and a copy of its Mac's config holding text, its export.json
// saying it holds both, and returns it.
func withConfig(t *testing.T, folder, text string) string {
	t.Helper()
	data, err := laidRecord.Encode()
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, folder, "accounts.json", data)
	writeStateFile(t, folder, "config.toml", []byte(text))
	writeStateFile(t, folder, "export.json", []byte(`{"version":3,"install":"`+strings.TrimPrefix(filepath.Base(folder), "mac-")+`","exported_at":"2026-10-09T14:42:07Z","ids":true,"accounts":true,"config":true}`+"\n"))
	return folder
}

// macFolder is the Mac's folder of the install with the given id in dir, as
// importFolder writes one.
func macFolder(t *testing.T, dir, install string) string {
	t.Helper()
	return importFolder(t, filepath.Join(dir, export.FolderName(install)))
}

// laidDown is what import prints laying down laidConfig at path from the
// folder of the install given, in dir, then importing again by the command
// line given: "switchboard import <dir>".
func laidDown(path, install, again string) string {
	return "laid down the config of mac-" + install + " at " + path + ": 2 accounts in 2 pools\n" +
		"next, run switchboard setup to give each account its token and start the router, then " + again + " again to bring in the history\n"
}

// laidCopy is the config laid down from a folder holding laidConfig: its
// values alone, as config.Copy writes them.
func laidCopy(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	copied, err := config.Copy("config.toml", []byte(laidConfig), func() (string, error) { return home, nil })
	if err != nil {
		t.Fatal(err)
	}
	return string(copied)
}

// wantLaid fails t where the config at path isn't laidConfig's copy.
func wantLaid(t *testing.T, path string) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || string(got) != laidCopy(t) {
		t.Errorf("the config laid down holds %q (%v), want the folder's values", got, err)
	}
}

// wantNoConfig fails t where the Mac deps give has a config.
func wantNoConfig(t *testing.T, deps cli.Deps) {
	t.Helper()
	if _, err := os.Lstat(configAt(t, deps)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the Mac's config: %v, want none laid down", err)
	}
}

// wantNoRecord fails t where the state directory state holds an accounts
// record.
func wantNoRecord(t *testing.T, state string) {
	t.Helper()
	if _, err := os.Lstat(identity.NewFile(state).Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the Mac's accounts record: %v, want none laid down", err)
	}
}

// On a Mac with no config and no router, as a new Mac taking over from
// another, or restoring its backup, import lays down the config of the
// Mac's folder, its pools under the ids its history names, and says what
// it laid down, each folder a pool fences that isn't one here, and what to
// run next, staging nothing: the history waits for setup to give each
// account its token and start the router.
func TestImportOnAMacWithNoConfigLaysDownTheFoldersConfig(t *testing.T) {
	deps, state := newMacDeps(t)
	dir := t.TempDir()
	withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
	path := configAt(t, deps)

	want := result{stdout: laidDown(path, "0123456789abcdef", "switchboard import "+dir), stderr: unfencedAcme}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantLaid(t, path)
	if _, err := os.Stat(router.ImportDir(state)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: %v, want nothing staged", router.ImportDir(state), err)
	}
}

// The config laid down comes with the accounts record seeded from the
// folder's: the accounts and pools that config configures alone, each
// account with its organization, never the marks the folder's copy carries,
// nor an account the other Mac removed or a pool it kept apart, which the
// import of its history takes in, so it's said there as kept apart, never
// as taken in before. A token of another subscription given an account, as
// on the wrong row of setup, is refused, naming the account it's for.
func TestImportLaysDownTheAccountsRecordWithTheConfig(t *testing.T) {
	deps, state := newMacDeps(t)
	dir := t.TempDir()
	folder := withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
	theirs := identity.Record{
		Accounts: append(slices.Clone(laidRecord.Accounts), identity.Entry{ID: "r7kq", Organization: "test-org-old", Label: "Old", Added: laidAt, Removed: laidAt}),
		Pools:    append(slices.Clone(laidRecord.Pools), identity.PoolEntry{ID: "c1nt", Label: "Client", Added: laidAt}),
	}
	data, err := theirs.Encode()
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, folder, "accounts.json", data)

	if got := run(t, deps, "import", dir); got.code != 0 {
		t.Fatalf("switchboard import = %+v, want the config laid down", got)
	}
	record, err := identity.NewFile(state).Read()
	if err != nil {
		t.Fatal(err)
	}
	want := []identity.Entry{{ID: "b6s7", Organization: "test-org-work", Added: laidAt}, {ID: "q8rt", Organization: "test-org-acme", Added: laidAt}}
	if !slices.Equal(record.Accounts, want) {
		t.Errorf("the accounts record holds %+v, want %+v: each configured account's organization, no mark of the folder's", record.Accounts, want)
	}
	wantPools := []identity.PoolEntry{{ID: "b2n8", Added: laidAt}, {ID: "q7mt", Added: laidAt}}
	if !slices.EqualFunc(record.Pools, wantPools, identity.PoolEntry.Equal) {
		t.Errorf("the accounts record's pools are %+v, want %+v: the configured ones, none marked, as the router marks them", record.Pools, wantPools)
	}

	token, err := tokens.Parse("test-token-acme")
	if err != nil {
		t.Fatal(err)
	}
	registry := accounts.Registry{ConfigPath: configAt(t, deps), HomeDir: deps.HomeDir, Tokens: tokens.NewStore(state, os.Getuid()), Record: identity.NewFile(state)}
	err = registry.SetTokenChecked(t.Context(), "b6s7", token, accounts.Checked{Organization: "test-org-acme"})
	if other, ok := errors.AsType[*accounts.OtherSubscriptionError](err); !ok || other.OtherID != "q8rt" || !other.OtherConfigured {
		t.Errorf("giving Work Acme's token = %v, want it refused, naming Acme, configured here", err)
	}
}

// The config laid down never holds a token, as a folder's copy edited by
// hand might: one in a comment isn't laid down, as no comment is, and a
// config giving one as a value isn't laid down at all.
func TestImportLaysDownNoToken(t *testing.T) {
	const token = "sk-ant-oat01-fake_token-shaped"
	t.Run("in a comment", func(t *testing.T) {
		deps, _ := newMacDeps(t)
		folder := withConfig(t, macFolder(t, t.TempDir(), "0123456789abcdef"), "# "+token+"\n"+laidConfig)

		if got := run(t, deps, "import", folder); got.code != 0 || strings.Contains(got.stdout+got.stderr, "sk-ant-") {
			t.Fatalf("switchboard import = %+v, want the config laid down, saying no token", got)
		}
		wantLaid(t, configAt(t, deps))
	})
	t.Run("as a value", func(t *testing.T) {
		deps, state := newMacDeps(t)
		folder := withConfig(t, macFolder(t, t.TempDir(), "0123456789abcdef"), `upstream = "https://gateway.example.com/`+token+`"`+"\n"+laidConfig)

		if got := run(t, deps, "import", folder); got.code != 1 || strings.Contains(got.stdout+got.stderr, "sk-ant-") || !strings.Contains(got.stderr, "can't be laid down") {
			t.Errorf("switchboard import = %+v, want it refused, saying why without the token", got)
		}
		wantNoConfig(t, deps)
		wantNoRecord(t, state)
	})
}

// Laying a config down is refused, laying nothing, where the one Mac's
// folder holds none, saying first to export again from the Mac that has it;
// where dir holds several Macs' folders, none this Mac's own, naming each,
// whether or not they hold a copy, or one alike, as the right one can't be
// told; where dir holds no export, or isn't there; and where any folder is
// passed over, as one a sync hasn't carried whole, saying which.
func TestImportLaysDownOneConfigOrSaysWhy(t *testing.T) {
	const several = "Error: this Mac has no config, and {dir} holds the folders of several Macs, mac-0123456789abcdef and mac-fedcba9876543210, so none is laid down: import from the folder of the Mac whose config to lay down, such as switchboard import {dir}/mac-0123456789abcdef\n"
	other := strings.Replace(laidConfig, `"Work"`, `"Office"`, 1)
	tests := []struct {
		name string
		put  func(t *testing.T, dir string)
		// at is where import is pointed in dir: dir itself where it's "".
		at   string
		want result
	}{
		{name: "none held, as an export from before the config was copied", put: func(t *testing.T, dir string) {
			macFolder(t, dir, "0123456789abcdef")
		}, want: result{stderr: "Error: this Mac has no config, and {dir}/mac-0123456789abcdef holds none to lay down, as a switchboard from before exports copied the config exported it: if that Mac still runs, upgrade switchboard there, export again, and import that; else run switchboard setup to make a config, then import again, its history then kept in pools of its own\n", code: 1}},
		{name: "several that differ", put: func(t *testing.T, dir string) {
			withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			withConfig(t, macFolder(t, dir, "fedcba9876543210"), other)
		}, want: result{stderr: several, code: 1}},
		{name: "several holding one config, as Macs sharing it", put: func(t *testing.T, dir string) {
			withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			withConfig(t, macFolder(t, dir, "fedcba9876543210"), laidConfig)
		}, want: result{stderr: several, code: 1}},
		{name: "an old Mac's from before the config was copied, beside another Mac's", put: func(t *testing.T, dir string) {
			macFolder(t, dir, "0123456789abcdef")
			withConfig(t, macFolder(t, dir, "fedcba9876543210"), laidConfig)
		}, want: result{stderr: several, code: 1}},
		{name: "no export", put: func(*testing.T, string) {}, want: result{
			stderr: "Error: this Mac has no config, and none is laid down: there's nothing to import in {dir}: it holds no Mac's folder, one with the export.json export writes\n", code: 1,
		}},
		{name: "a folder that isn't there", put: func(*testing.T, string) {}, at: "mistyped", want: result{
			stderr: "Error: this Mac has no config, and none is laid down: can't import from {dir}/mistyped: open {dir}/mistyped: no such file or directory\n", code: 1,
		}},
		{name: "one not synced", put: func(t *testing.T, dir string) {
			folder := withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			if err := os.Remove(filepath.Join(folder, "config.toml")); err != nil {
				t.Fatal(err)
			}
		}, want: result{stderr: "switchboard: passed over {dir}/mac-0123456789abcdef: its config hasn't synced yet: import it again once it has\n" +
			"Error: this Mac has no config, and none is laid down while a folder in {dir} is passed over: import again once each reads, or import from the folder of the Mac whose config to lay down alone\n", code: 1}},
		{name: "one beside a folder passed over", put: func(t *testing.T, dir string) {
			withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			writeStateFile(t, filepath.Join(dir, "mac-fedcba9876543210"), "export.json", []byte(`{"version":3,"install":"fedc`))
		}, want: result{stderr: "switchboard: passed over {dir}/mac-fedcba9876543210: its export.json doesn't read\n" +
			"Error: this Mac has no config, and none is laid down while a folder in {dir} is passed over: import again once each reads, or import from the folder of the Mac whose config to lay down alone\n", code: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, state := newMacDeps(t)
			dir := t.TempDir()
			tt.put(t, dir)

			got := run(t, deps, "import", filepath.Join(dir, tt.at))
			want := tt.want
			want.stderr = strings.ReplaceAll(want.stderr, "{dir}", dir)
			if got != want {
				t.Errorf("switchboard import = %+v, want %+v", got, want)
			}
			wantNoConfig(t, deps)
			wantNoRecord(t, state)
		})
	}
}

// A Mac with a history of its own, its accounts record, but no config never
// takes another Mac's config, which would leave its pools marked removed.
func TestImportLaysNoConfigDownBesideAHistoryOfTheMacsOwn(t *testing.T) {
	deps, state := importDeps(t)
	was, err := os.ReadFile(identity.NewFile(state).Path())
	if err != nil {
		t.Fatal(err)
	}
	folder := withConfig(t, macFolder(t, t.TempDir(), "0123456789abcdef"), laidConfig)

	want := result{stderr: "Error: this Mac has no config, but its state directory holds an accounts record of its own, which another Mac's config would leave in pools marked removed, so none is laid down: run switchboard setup to make a config, then import again\n", code: 1}
	if got := run(t, deps, "import", folder); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantNoConfig(t, deps)
	if got, err := os.ReadFile(identity.NewFile(state).Path()); err != nil || string(got) != string(was) {
		t.Errorf("the accounts record holds %s (%v), want it as it was", got, err)
	}
}

// A Mac that lost its config alone, its history kept, restores its own
// backup: the config of its own folder, among other Macs' in dir, is laid
// down, and its accounts record, which is this Mac's already, left as it is.
func TestImportRestoresTheConfigOfThisMacsOwnFolder(t *testing.T) {
	deps, state := importDeps(t)
	writeStateFile(t, state, "install-id", []byte("0123456789abcdef\n"))
	writeStateFile(t, ledger.Dir(state), "requests-2026-10-02.jsonl", []byte(`{"n":1}`+"\n"))
	was, err := os.ReadFile(identity.NewFile(state).Path())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
	withConfig(t, macFolder(t, dir, "fedcba9876543210"), strings.Replace(laidConfig, `"Work"`, `"Office"`, 1))

	want := result{stdout: laidDown(configAt(t, deps), "0123456789abcdef", "switchboard import "+dir), stderr: unfencedAcme}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantLaid(t, configAt(t, deps))
	if got, err := os.ReadFile(identity.NewFile(state).Path()); err != nil || string(got) != string(was) {
		t.Errorf("the accounts record holds %s (%v), want it as it was, this Mac's own", got, err)
	}
}

// A restore cut short once the accounts record is seeded, before the config
// is laid down, is done again: a record holding nothing but what a restore
// seeds, beside no history, is no history of the Mac's own. Once the state
// directory holds a day, it is.
func TestImportLaysTheConfigDownAgainOnceARestoreWasCutShort(t *testing.T) {
	for _, history := range []bool{false, true} {
		t.Run(map[bool]string{false: "no history", true: "a day of history"}[history], func(t *testing.T) {
			deps, state := newMacDeps(t)
			dir := t.TempDir()
			withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
			if got := run(t, deps, "import", dir); got.code != 0 {
				t.Fatalf("switchboard import = %+v, want the config laid down", got)
			}
			seeded, err := os.ReadFile(identity.NewFile(state).Path())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(configAt(t, deps)); err != nil {
				t.Fatal(err)
			}
			if history {
				markDone(t, state, idsMark, poolsMark)
				writeStateFile(t, ledger.Dir(state), "requests-2026-10-02.jsonl", []byte(`{"n":1}`+"\n"))
			}

			got := run(t, deps, "import", dir)
			if history {
				if got.code != 1 || !strings.Contains(got.stderr, "holds an accounts record of its own") {
					t.Errorf("switchboard import = %+v, want it refused, the Mac's history its own", got)
				}
				wantNoConfig(t, deps)
			} else {
				if got.code != 0 {
					t.Errorf("switchboard import = %+v, want the config laid down again", got)
				}
				wantLaid(t, configAt(t, deps))
			}
			if again, err := os.ReadFile(identity.NewFile(state).Path()); err != nil || string(again) != string(seeded) {
				t.Errorf("the accounts record holds %s (%v), want it as seeded, %s", again, err, seeded)
			}
		})
	}
}

// Anything at the config's path that isn't a config that loads, as a link
// to one that's gone, is never replaced, and import says what's there, not
// that the router's down.
func TestImportSaysWhatIsAtTheConfigsPathWhereItIsntAConfig(t *testing.T) {
	deps, state := newMacDeps(t)
	path := configAt(t, deps)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "dotfiles", "config.toml")
	if err := os.Symlink(gone, path); err != nil {
		t.Fatal(err)
	}
	folder := withConfig(t, macFolder(t, t.TempDir(), "0123456789abcdef"), laidConfig)

	want := result{stderr: "Error: import lays a config down only where there's none, and " + path + " is a link to " + gone + ", which isn't there: put it right, or move it aside, then import again\n", code: 1}
	if got := run(t, deps, "import", folder); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	if target, err := os.Readlink(path); err != nil || target != gone {
		t.Errorf("the config's path holds a link to %q (%v), want it as it was", target, err)
	}
	wantNoRecord(t, state)
}

// Given --config, import lays the config down there, and the commands it
// says to run next carry it, each word quoted for the shell where it needs
// it, as a folder in iCloud Drive's Mobile Documents does.
func TestImportLaysTheConfigDownWhereConfigSaysAndSaysToRunWithIt(t *testing.T) {
	deps, _ := newMacDeps(t)
	dir := filepath.Join(t.TempDir(), "Mobile Documents")
	withConfig(t, macFolder(t, dir, "0123456789abcdef"), laidConfig)
	path := filepath.Join(t.TempDir(), "elsewhere.toml")

	got := run(t, deps, "import", "--config", path, dir)
	next := "next, run switchboard setup --config " + path + " to give each account its token and start the router, then switchboard import --config " + path + " '" + dir + "' again to bring in the history\n"
	if got.code != 0 || !strings.HasSuffix(got.stdout, next) || !strings.Contains(got.stderr, "switchboard pool edit --config "+path+" q7mt --dirs") {
		t.Errorf("switchboard import --config = %+v, want it to say to run\n%s", got, next)
	}
	wantLaid(t, path)
	wantNoConfig(t, deps)
}

// Import says each pool of another Mac's it kept apart, why, and how to read
// it, once it has said what it took in; and of a pool it takes in as one
// here whose days an import took in before, that those stay apart.
func TestImportSaysEachPoolItKeptApart(t *testing.T) {
	deps, state := importDeps(t)
	imported := router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}, Apart: []router.ApartPool{
		{Install: "fedcba9876543210", Pool: "c1nt", Label: "Client", Why: identity.ApartSplit},
		{Install: "fedcba9876543210", Pool: "h0me", Label: "Home", Why: identity.ApartNoneHere},
		{Install: "0123456789abcdef", Pool: "0ld1", Label: "Old", Why: identity.ApartNoConfig},
		{Install: "0123456789abcdef", Pool: "e4e4", Label: "Spare", Why: identity.ApartEmpty},
		{Install: "0123456789abcdef", Pool: "n0p1", Label: "Lone", Why: identity.ApartNoPoolsHere},
		{Install: "fedcba9876543210", Pool: "w0rk", Label: "Office", Why: identity.ApartBefore},
	}}
	(&imports{post: answering(imported)}).serve(t, state)

	want := result{
		stdout: "imported 1 folder: 1 day added\n",
		stderr: `switchboard: kept the pool "Client" (c1nt) of mac-fedcba9876543210 apart, as its accounts are in more than one pool here: read it with --pool c1nt` + "\n" +
			`switchboard: kept the pool "Home" (h0me) of mac-fedcba9876543210 apart, as none of its accounts is configured here: read it with --pool h0me` + "\n" +
			`switchboard: kept the pool "Old" (0ld1) of mac-0123456789abcdef apart, as its folder holds no copy of that Mac's config to match its accounts by: read it with --pool 0ld1` + "\n" +
			`switchboard: kept the pool "Spare" (e4e4) of mac-0123456789abcdef apart, as that Mac's config puts no account in it: read it with --pool e4e4` + "\n" +
			`switchboard: kept the pool "Lone" (n0p1) of mac-0123456789abcdef apart, as this Mac's config has no pools: read it with --pool n0p1` + "\n" +
			`switchboard: the pool "Office" (w0rk) of mac-fedcba9876543210 goes to the pool here holding its accounts now, but the days an import took in before stay under w0rk: read them with --pool w0rk` + "\n",
	}
	if got := run(t, deps, "import", importFolder(t, t.TempDir())); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
}

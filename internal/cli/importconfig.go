package cli

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/prose"
)

// errOwnHistory is what laying another Mac's config down fails with where
// this Mac has a history of its own, an accounts record or a day of it:
// that config would leave it in pools it doesn't have.
var errOwnHistory = errors.New("this Mac has no config, but its state directory holds a history of its own, an accounts record or a day of it, which another Mac's config would leave in pools it doesn't have, so none is laid down: run switchboard setup to make a config, then import again")

// restore lays down the config of the Mac's folder in dir, as restoreFrom
// picks it, at the path the command reads the config at, where nothing is
// there and no router runs: as on a new Mac taking over from another, or
// one that lost its config alone, restoring its own backup, so the pools
// keep the ids the history names, and setup then asks for nothing but each
// account's token. With another Mac's config, it seeds the accounts record,
// in the state directory state, from that Mac's folder, as seeding says, so
// each account's organization is known, and setup refuses a token of
// another subscription given it, naming the account it's for. Then it says
// what it laid down, warns of each folder a pool fences that isn't one here,
// as of another home's, and says what to run next, as the history waits for
// a router to take it in.
//
// It lays nothing down, failing, where something is at the config's path,
// as configAt says; where restoreFrom finds no config to lay down; where
// seeding refuses; and where the config can't be laid down, as config.Copy
// says. It never writes in place of anything at the config's path, and lays
// the record down with the config or not at all.
func (a *app) restore(out, errOut io.Writer, state, dir string) error {
	path, err := a.configFile()
	if err != nil {
		return err
	}
	if err := a.configAt(path); err != nil {
		return err
	}
	from, err := a.restoreFrom(errOut, state, dir)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(path, from.config, a.HomeDir)
	if err != nil {
		return err
	}
	record := identity.NewFile(state)
	err = record.Change(func(r *identity.Record, commit func() error) error {
		if err := seeding(record.Path(), r, from, cfg, state); err != nil {
			return err
		}
		if err := commit(); err != nil {
			return err
		}
		return a.layDown(path, from.config)
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "laid down the config of %s at %s: %s\n", filepath.Base(from.folder), path, configured(cfg)); err != nil {
		return err
	}
	if err := a.sayUnfenced(errOut, cfg.Pools); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "next, run %s to give each account its token and start the router, then %s again to bring in the history\n",
		a.command("setup"), a.command("import", dir))
	return err
}

// restoring is a config to lay down: the Mac's folder it's the copy of,
// whether that's this install's own, and the copy, as config.Copy makes it.
type restoring struct {
	folder string
	own    bool
	config []byte
}

// seeding seeds r, the accounts record at path, in the state directory
// state, for the config from, as cfg reads, to be laid down, as idmove's
// SeedFolder seeds one from from's folder. Of this Mac's own folder,
// restoring its own backup, it seeds only where there's no record, and
// leaves one there as it is. Of another Mac's, it seeds only where this Mac
// has no history of its own, as ownHistory tells, failing with
// errOwnHistory where it has.
func seeding(path string, r *identity.Record, from restoring, cfg *config.Config, state string) error {
	_, err := os.Lstat(path)
	recorded := err == nil
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("look for an accounts record at %s: %w", path, err)
	case from.own && recorded:
		return nil
	case !from.own:
		if err := ownHistory(r, recorded, state); err != nil {
			return err
		}
	}
	here := idmove.Here{Install: sync.OnceValues(func() (string, error) { return export.InstallID(state) }), Accounts: cfg.Accounts, Pools: cfg.Pools}
	if err := idmove.SeedFolder(from.folder, r, here); err != nil {
		return fmt.Errorf("this Mac has no config, and the accounts of %s can't be recorded, so none is laid down: %w", from.folder, err)
	}
	return nil
}

// ownHistory fails with errOwnHistory where the state directory state holds
// a history of this Mac's own, which another Mac's config would leave in
// pools it doesn't have: r, its accounts record, where it's recorded,
// holding more than a restore seeds, as identity's Seeded tells, or a day of
// the ledger, the events or the readings, as idmove's HoldsHistory tells. A
// record holding no more than a restore seeds, and no day, is one a restore
// cut short left, between seeding it and laying the config down.
func ownHistory(r *identity.Record, recorded bool, state string) error {
	if recorded && !r.Seeded() {
		return errOwnHistory
	}
	held, err := idmove.HoldsHistory(state)
	switch {
	case err != nil:
		return fmt.Errorf("look for a history in %s: %w", state, err)
	case held:
		return errOwnHistory
	}
	return nil
}

// layDown lays data down as the config at path, as config.LayDown does,
// failing, as configAt says, where something came to be there meanwhile.
func (a *app) layDown(path string, data []byte) error {
	err := config.LayDown(path, data)
	if errors.Is(err, fs.ErrExist) {
		return cmp.Or(a.configAt(path), err)
	}
	return err
}

// restoreFrom returns the config to lay down from the Macs' folders in dir,
// as foldersFound finds them: that of this install's own folder, where it's
// among them, as on a Mac restoring its own backup; else that of the one
// folder there is, as on a new Mac taking over from another. It fails,
// saying each folder passed over, where any is, as a config laid down then
// might be the wrong Mac's, or one a sync has yet to carry; where dir holds
// several Macs' folders, none this install's, as severalMacs says; where
// the folder holds no copy of its Mac's config, as noCopy says; and where
// the copy can't be laid down, as config.Copy says.
func (a *app) restoreFrom(errOut io.Writer, state, dir string) (restoring, error) {
	folders, passed, err := foldersFound(state, dir)
	if sayErr := sayPassed(errOut, passed); sayErr != nil {
		return restoring{}, sayErr
	}
	switch {
	case err != nil:
		return restoring{}, fmt.Errorf("this Mac has no config, and none is laid down: %w", err)
	case len(passed) > 0:
		return restoring{}, fmt.Errorf("this Mac has no config, and none is laid down while a folder in %s is passed over: import again once each reads, or import from the folder of the Mac whose config to lay down alone", dir)
	}
	from := restoring{folder: folders[0]}
	switch own, isOwn, err := ownFolder(state, folders); {
	case err != nil:
		return restoring{}, err
	case isOwn:
		from.folder, from.own = own, true
	case len(folders) > 1:
		return restoring{}, a.severalMacs(dir, folders)
	}
	data, err := idmove.ConfigCopy(from.folder)
	switch {
	case err != nil:
		return restoring{}, fmt.Errorf("this Mac has no config, and the one in %s can't be read: %w", from.folder, err)
	case data == nil:
		return restoring{}, a.noCopy(from)
	}
	if from.config, err = config.Copy(export.ConfigFile(from.folder), data, a.HomeDir); err != nil {
		return restoring{}, fmt.Errorf("this Mac has no config, and the one in %s can't be laid down: %w", from.folder, err)
	}
	return from, nil
}

// ownFolder returns the folder of folders, Macs' folders, this install
// exported, as its export.json names this install's id, reporting whether
// there's one: none where no id is made yet, as on a Mac that has never
// exported.
func ownFolder(state string, folders []string) (string, bool, error) {
	id, err := export.ReadInstallID(state)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	for _, folder := range folders {
		if m, err := export.ReadManifest(folder); err == nil && m.Install == id {
			return folder, true, nil
		}
	}
	return "", false, nil
}

// severalMacs is the error of dir holding the folders of several Macs,
// folders, none this install's, whose config to lay down can't be told:
// naming each, and saying to import from the one whose config to lay down.
func (a *app) severalMacs(dir string, folders []string) error {
	names := make([]string, len(folders))
	for i, folder := range folders {
		names[i] = filepath.Base(folder)
	}
	return fmt.Errorf("this Mac has no config, and %s holds the folders of several Macs, %s, so none is laid down: import from the folder of the Mac whose config to lay down, such as %s",
		dir, prose.List(names), a.command("import", folders[0]))
}

// noCopy is the error of from's folder holding no copy of its Mac's config,
// as one exported before an export copied the config: saying first, of
// another Mac's, to export again from that Mac, if it still runs, as setup's
// config here would give the pools new ids, keeping the history apart.
func (a *app) noCopy(from restoring) error {
	again := "if that Mac still runs, upgrade switchboard there, export again, and import that; else "
	if from.own {
		again = ""
	}
	return fmt.Errorf("this Mac has no config, and %s holds none to lay down, as a switchboard from before exports copied the config exported it: %srun %s to make a config, then import again, its history then kept in pools of its own",
		from.folder, again, a.command("setup"))
}

// configAt fails where something is at path, the config's: with
// errRouterDown where it's a config that loads, as the router would run on,
// so an import needs it running; else saying what's there, as a config is
// laid down only where there's none.
func (a *app) configAt(path string) error {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("look for a config at %s: %w", path, err)
	}
	_, loadErr := config.Load(path, a.HomeDir)
	if loadErr == nil {
		return errRouterDown
	}
	return fmt.Errorf("import lays a config down only where there's none, and %s is %s: put it right, or move it aside, then import again", path, whatsAt(path, info, loadErr))
}

// whatsAt says what's at path, the config's, by info, as Lstat gives it,
// and loadErr, why the config there doesn't load.
func whatsAt(path string, info fs.FileInfo, loadErr error) string {
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, _ := os.Readlink(path)
		if _, err := os.Stat(path); err != nil {
			return "a link to " + target + ", which isn't there"
		}
		return "a link to " + target + ", a config that doesn't load: " + loadErr.Error()
	case info.IsDir():
		return "a folder"
	}
	return "a config that doesn't load: " + loadErr.Error()
}

// configured counts what cfg configures: "3 accounts in 2 pools", its pools
// left out where it names none.
func configured(cfg *config.Config) string {
	said := prose.Counted(len(cfg.Accounts), "account")
	if len(cfg.Pools) > 0 {
		said += " in " + prose.Counted(len(cfg.Pools), "pool")
	}
	return said
}

// sayUnfenced warns on errOut of each folder a pool of pools fences that
// isn't a folder on this Mac, as one in another home is, no session
// starting in it until the pool's fenced anew.
func (a *app) sayUnfenced(errOut io.Writer, pools config.Pools) error {
	for _, p := range pools {
		for i, dir := range p.Dirs {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				continue
			}
			if _, err := fmt.Fprintf(errOut, "switchboard: %s isn't a directory, so no session starts in it yet: if it's on this Mac by another name, fence the pool %s anew with %s\n",
				p.WrittenDirs[i], prose.Quoted(p.Label), a.command("pool edit", p.ID, "--dirs")); err != nil {
				return err
			}
		}
	}
	return nil
}

// command is the switchboard command line of the subcommand sub, as "pool
// edit", with args, each a word to a shell, as shellWord makes it, carrying
// the --config this command was given, where it was given one.
func (a *app) command(sub string, args ...string) string {
	words := []string{"switchboard", sub}
	if a.configPath != "" {
		words = append(words, "--config", shellWord(a.configPath))
	}
	for _, arg := range args {
		words = append(words, shellWord(arg))
	}
	return strings.Join(words, " ")
}

// shellWord is s as one word to a shell: as it is where it holds nothing a
// shell reads specially, else between single quotes, each single quote in
// it ended, escaped and begun again.
func shellWord(s string) string {
	if s != "" && strings.Trim(s, plainInShell) == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// plainInShell are the characters a shell reads as themselves anywhere in a
// word.
const plainInShell = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:@%+=,"

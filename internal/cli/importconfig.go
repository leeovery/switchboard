package cli

import (
	"bytes"
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

// errOwnHistory is what laying a config down fails with where this Mac has
// a history of its own: another Mac's config would leave its pools marked
// removed, as the router keeps the record in step with the config.
var errOwnHistory = errors.New("this Mac has no config, but its state directory holds an accounts record of its own, which another Mac's config would leave in pools marked removed, so none is laid down: run switchboard setup to make a config, then import again")

// restore lays down the config of the Mac's folder in dir, as configIn finds
// it, at the path the command reads the config at, where nothing is there
// and no router runs, as on a new Mac taking over from another, or one
// restoring its backup: so its pools keep the ids the history names, and
// setup then asks for nothing but each account's token. With it, it seeds
// the accounts record, in the state directory state, from the folder's
// record, as idmove's JoinFolder joins it, so each account's organization is
// known, and setup refuses a token of another subscription given it, naming
// the account it's for. Then it says what it laid down, warns of each folder
// a pool fences that isn't one here, as of another home's, and says what to
// run next, as the history waits for a router to take it in.
//
// It lays nothing down, failing, where something is at the config's path,
// as configAt says; where the state directory holds an accounts record,
// with errOwnHistory; where a folder in dir is passed over, as foldersFound
// passes one over, saying which; and where the config can't be laid down,
// as configIn and config.Copy say. It never writes in place of anything at
// the config's path, and lays the record down with the config or not at
// all.
func (a *app) restore(out, errOut io.Writer, state, dir string) error {
	path, err := a.configFile()
	if err != nil {
		return err
	}
	if err := a.configAt(path); err != nil {
		return err
	}
	var folder string
	var cfg *config.Config
	record := identity.NewFile(state)
	err = record.Change(func(r *identity.Record, commit func() error) error {
		if _, err := os.Lstat(record.Path()); !errors.Is(err, fs.ErrNotExist) {
			return errOwnHistory
		}
		var copied []byte
		if folder, copied, err = a.restored(errOut, state, dir); err != nil {
			return err
		}
		if cfg, err = config.Parse(path, copied, a.HomeDir); err != nil {
			return err
		}
		here := idmove.Here{Install: sync.OnceValues(func() (string, error) { return export.InstallID(state) }), Accounts: cfg.Accounts, Pools: cfg.Pools}
		if _, err := idmove.JoinFolder(folder, r, here); err != nil {
			return fmt.Errorf("this Mac has no config, and the accounts of %s can't be recorded, so none is laid down: %w", folder, err)
		}
		if err := commit(); err != nil {
			return err
		}
		return a.layDown(path, copied)
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "laid down the config of %s at %s: %s\n", filepath.Base(folder), path, configured(cfg)); err != nil {
		return err
	}
	if err := a.sayUnfenced(errOut, cfg.Pools); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "next, run %s to give each account its token and start the router, then %s again to bring in the history\n",
		a.command("setup"), a.command("import", dir))
	return err
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

// restored returns the Mac's folder in dir whose config to lay down, and
// that config, as config.Copy copies it: of the folders foldersFound finds,
// as configIn picks one. It fails, saying each folder passed over, where
// any is, as a config laid down then might be the wrong Mac's, or one a
// sync has yet to carry.
func (a *app) restored(errOut io.Writer, state, dir string) (string, []byte, error) {
	folders, passed, err := foldersFound(state, dir)
	if sayErr := sayPassed(errOut, passed); sayErr != nil {
		return "", nil, sayErr
	}
	switch {
	case err != nil:
		return "", nil, err
	case len(passed) > 0:
		return "", nil, fmt.Errorf("this Mac has no config, and none is laid down while a folder in %s is passed over: import again once each reads, or import from the folder of the Mac whose config to lay down alone", dir)
	}
	folder, data, err := a.configIn(dir, folders)
	if err != nil {
		return "", nil, err
	}
	copied, err := config.Copy(export.ConfigFile(folder), data, a.HomeDir)
	if err != nil {
		return "", nil, fmt.Errorf("this Mac has no config, and the one in %s can't be laid down: %w", folder, err)
	}
	return folder, copied, nil
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

// configIn returns the Mac's folder of folders, those in dir, whose copy of
// its Mac's config to lay down, and the copy, as idmove's ConfigCopy reads
// it: the one copy there is, or the one every copy is, as of Macs sharing a
// config. It fails where none holds one, as folders exported before an
// export copied the config don't; and where several hold copies that
// differ, each saying what to do.
func (a *app) configIn(dir string, folders []string) (string, []byte, error) {
	var first string
	var data []byte
	for _, folder := range folders {
		copied, err := idmove.ConfigCopy(folder)
		switch {
		case err != nil:
			return "", nil, fmt.Errorf("this Mac has no config, and the one in %s can't be read: %w", folder, err)
		case copied == nil:
			continue
		case first == "":
			first, data = folder, copied
		case !bytes.Equal(copied, data):
			return "", nil, fmt.Errorf("this Mac has no config, and %s holds several Macs' configs: import from the folder of the Mac whose config to lay down, such as %s", dir, a.command("import", folder))
		}
	}
	if first == "" {
		return "", nil, fmt.Errorf("this Mac has no config, and %s holds none to lay down, as a switchboard from before exports copied the config exported it: if that Mac still runs, upgrade switchboard there, export again, and import that; else run switchboard setup to make a config, then import again, its history then kept in pools of its own", dir)
	}
	return first, data, nil
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

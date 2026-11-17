package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/prose"
)

// restore lays down the config of the Mac's folder in dir, as configIn finds
// it, as this Mac's, where it has none and no router runs, as on a new Mac
// taking over from another, or one restoring its backup: so its pools keep
// the ids the history names, and setup then asks for nothing but each
// account's token. Then it says what it laid down, and what to run next, as
// the history waits for a router to take it in. Where this Mac has a config,
// it fails as an import with no router does; and where the config can't be
// laid down, saying why. It never writes in place of anything at the
// config's path.
func (a *app) restore(out io.Writer, dir string) error {
	path, err := a.configFile()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		return errRouterDown
	}
	folder, data, err := configIn(dir)
	if err != nil {
		return err
	}
	copied, err := config.Copy(export.ConfigFile(folder), data, a.HomeDir)
	if err != nil {
		return fmt.Errorf("this Mac has no config, and the one in %s can't be laid down: %w", folder, err)
	}
	cfg, err := config.Parse(path, copied, a.HomeDir)
	if err != nil {
		return err
	}
	switch err := config.LayDown(path, copied); {
	case errors.Is(err, fs.ErrExist):
		return errRouterDown
	case err != nil:
		return err
	}
	_, err = fmt.Fprintf(out, "laid down the config of %s at %s: %s\n"+
		"next, run switchboard setup to give each account its token and start the router, then switchboard import %s again to bring in the history\n",
		filepath.Base(folder), path, configured(cfg), dir)
	return err
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

// configIn returns the Mac's folder in dir, as export.Folders finds them,
// whose copy of its Mac's config to lay down, and the copy: the one copy
// there is, or the one every copy is, as of Macs sharing a config. It fails
// where none holds one, as folders exported before an export copied the
// config don't; where several hold copies that differ; and where a folder's
// export.json says it holds one it doesn't, as a sync not done leaves it,
// each saying what to do.
func configIn(dir string) (string, []byte, error) {
	folders, err := export.Folders(dir, func(string, error) {})
	if err != nil {
		return "", nil, err
	}
	var first string
	var data []byte
	for _, folder := range folders {
		copied, err := os.ReadFile(export.ConfigFile(folder))
		switch {
		case errors.Is(err, fs.ErrNotExist) && claimsConfig(folder):
			return "", nil, fmt.Errorf("this Mac has no config, and the one in %s hasn't synced yet: import again once it has", folder)
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return "", nil, fmt.Errorf("this Mac has no config, and the one in %s can't be read: %w", folder, err)
		}
		switch {
		case first == "":
			first, data = folder, copied
		case !bytes.Equal(copied, data):
			return "", nil, fmt.Errorf("this Mac has no config, and %s holds several Macs' configs: import from the folder of the Mac whose config to lay down, such as switchboard import %s", dir, folder)
		}
	}
	if first == "" {
		return "", nil, fmt.Errorf("this Mac has no config, and %s holds none to lay down, as an export from before exports copied the config holds none: run switchboard setup to make one, then import again", dir)
	}
	return first, data, nil
}

// claimsConfig reports whether the export.json of the Mac's folder at folder
// says it holds a copy of its Mac's config.
func claimsConfig(folder string) bool {
	m, err := export.ReadManifest(folder)
	return err == nil && m.Config
}

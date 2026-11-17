package export

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

// configName is the name of the copy of its Mac's config in a Mac's folder.
const configName = "config.toml"

// ConfigFile returns where the copy of its Mac's config is in folder, a Mac's
// folder, as an export writes it.
func ConfigFile(folder string) string {
	return filepath.Join(folder, configName)
}

// config writes a copy of the config file at path, read in the home
// directory homeDir returns, into the Mac's folder at folder, as
// config.Copy makes one, so it loads and holds no secret, with the config
// file's modification time, making the folder as x.mkdir does: passed over
// where the folder's copy is as long and was last modified then, as a day's
// compressed file is, so nothing in the folder is read. It notes whether the
// folder holds a copy, as x.configured: this export's, or, where it makes
// none, the last an export made, which it keeps, as a restore wants the last
// config that loaded. Where path is "", it makes none. Where the config
// isn't there, can't be read, or doesn't load, as when a router running on
// the config it read before goes on exporting once the file's gone, it
// keeps that copy, and notes why, as Result's Uncopied says: but of a config
// that isn't there where the folder holds no copy either, which is none to
// copy, and none kept. Where the config loads, but config.Copy refuses it,
// as holding a secret, it removes the folder's copy, as one of an older
// config would read as current, and notes why. It fails where the copy
// can't be written, or the folder's can't be looked at or removed.
func (x *exporting) config(path string, homeDir func() (string, error), folder string) error {
	dst := ConfigFile(folder)
	if path == "" {
		return x.keptConfig(dst)
	}
	data, modified, err := copyOfConfig(path, homeDir)
	switch {
	case err == nil:
		if err := x.writeConfig(dst, data, modified); err != nil {
			return fmt.Errorf("export the config: %w", err)
		}
		x.configured = true
	case errors.Is(err, config.ErrSecretValue) || errors.Is(err, config.ErrCopyDiffers):
		x.uncopied = err
		if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove the copy of the config an export before made: %w", err)
		}
	default:
		if err := x.keptConfig(dst); err != nil {
			return err
		}
		if x.configured || !errors.Is(err, fs.ErrNotExist) {
			x.uncopied = err
		}
	}
	return nil
}

// keptConfig notes, as x.configured, whether the folder holds the copy of
// the config at dst an export before made, which this one keeps.
func (x *exporting) keptConfig(dst string) error {
	_, err := os.Lstat(dst)
	switch {
	case err == nil:
		x.configured = true
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("look for the copy of the config an export before made: %w", err)
	}
	return nil
}

// copyOfConfig reads the config file at path, as config.Copy copies it, read
// in the home directory homeDir returns, and returns the copy and the file's
// modification time.
func copyOfConfig(path string, homeDir func() (string, error)) ([]byte, time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read config: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read config: %w", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read config: %w", err)
	}
	copied, err := config.Copy(path, data, homeDir)
	return copied, info.ModTime(), err
}

// writeConfig writes data, a copy of the config, at dst, last modified at
// modified, but where the copy there is as long and was last modified then,
// as keptAs tells.
func (x *exporting) writeConfig(dst string, data []byte, modified time.Time) error {
	if was, err := os.Stat(dst); err == nil && was.Size() == int64(len(data)) && keptAs(modified, was.ModTime()) {
		return nil
	}
	return writeFile(dst, modified, x.mkdir, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

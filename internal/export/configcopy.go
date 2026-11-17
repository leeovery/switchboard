package export

import (
	"bytes"
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
// config.Copy makes one, so it loads and holds nothing shaped like a token:
// where it differs from the one there, with the config file's modification
// time, making the folder as x.mkdir does. A config that can't be read, or
// copied so, isn't, the folder keeping the copy it has, as a backup keeps
// the last config that loaded, and why is noted, as Result's Uncopied says,
// but of one that isn't there, which is none to copy; "" copies none. It
// reports whether the folder holds a copy, and fails where one can't be
// written.
func (x *exporting) config(path string, homeDir func() (string, error), folder string) (bool, error) {
	dst := ConfigFile(folder)
	if path != "" {
		data, modified, err := copyOfConfig(path, homeDir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			x.uncopied = err
		default:
			if err := x.writeConfig(dst, data, modified); err != nil {
				return false, fmt.Errorf("export the config: %w", err)
			}
		}
	}
	_, err := os.Lstat(dst)
	return err == nil, nil
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
// modified, where the copy there differs.
func (x *exporting) writeConfig(dst string, data []byte, modified time.Time) error {
	if was, err := os.ReadFile(dst); err == nil && bytes.Equal(was, data) {
		return nil
	}
	return writeFile(dst, modified, x.mkdir, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

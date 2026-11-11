package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

const (
	// Version is the version of export.json this switchboard writes: 2 since
	// account ids were made, so a switchboard from before them, which reads
	// version 1, refuses a folder this one exports, whose days name accounts
	// by ids it has no token files for. A folder of version 1, from before
	// them or after, is read as its export.json's IDs says.
	Version = 2
	// folderPrefix begins the name of a Mac's folder, before its install's
	// id.
	folderPrefix = "mac-"
	// manifestFile is the name of a Mac's folder's export.json.
	manifestFile = "export.json"
)

// Manifest is what a Mac's folder's export.json says of it: its version, the
// install it's of, by its id, when it was last exported to, in UTC; IDs,
// whether a switchboard that makes account ids exported it, so its days name
// accounts by those ids; and Accounts, whether that export wrote the
// accounts record, as one does where the Mac has one: a folder that says so
// but holds none hasn't synced whole.
type Manifest struct {
	Version    int       `json:"version"`
	Install    string    `json:"install"`
	ExportedAt time.Time `json:"exported_at"`
	IDs        bool      `json:"ids,omitzero"`
	Accounts   bool      `json:"accounts,omitzero"`
}

// Folder returns the folder of the install with the given id in dir:
// <dir>/mac-<id>.
func Folder(dir, id string) string {
	return filepath.Join(dir, FolderName(id))
}

// FolderName is the name of the folder of the install with the given id:
// mac-<id>.
func FolderName(id string) string {
	return folderPrefix + id
}

// ReadManifest reads the export.json in folder, a Mac's folder. It fails
// with fs.ErrNotExist where there's none, and where it doesn't read as JSON
// naming an install by its id.
func ReadManifest(folder string) (Manifest, error) {
	path := filepath.Join(folder, manifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	if !isID(m.Install) {
		return Manifest{}, fmt.Errorf("read %s: %q isn't an install's id", path, m.Install)
	}
	return m, nil
}

// removeOlder removes the export.json of the Mac's folder at folder where
// it's of an older version than this switchboard writes, before an export
// writes anything else in it, as the days it writes name accounts by ids a
// switchboard of that version has no token files for: so an export cut
// short, or a sync that carries the days first, never leaves them beside an
// export.json that version would import. The export writes its own once
// every file has been. One that doesn't read is left, as every switchboard
// passes it over.
func removeOlder(folder string) error {
	if m, err := ReadManifest(folder); err != nil || m.Version >= Version {
		return nil
	}
	if err := os.Remove(filepath.Join(folder, manifestFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the export.json of an older version: %w", err)
	}
	return nil
}

// writeManifest writes the export.json of the Mac's folder at folder, of the
// install with the given id, exported to at the time given, saying it's of
// a switchboard that makes account ids, and whether it wrote the accounts
// record, whole.
func writeManifest(folder, id string, at time.Time, accounts bool) error {
	data, err := json.Marshal(Manifest{Version: Version, Install: id, ExportedAt: at.UTC().Truncate(time.Second), IDs: true, Accounts: accounts})
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(folder, manifestFile), append(data, '\n'), 0o600)
}

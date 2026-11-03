package export

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// idFile is the install's id's file in the state directory.
	idFile = "install-id"
	// idBytes is how many random bytes an id is made of, each two lowercase
	// hex characters of it.
	idBytes = 8
)

// InstallID returns the install's id, as <state dir>/install-id holds it,
// making it first where it isn't there: 16 random lowercase hex characters
// and a line ending, the user's alone. It's made once and never replaced, so
// two exports at once, the command's and the router's, end up with one id:
// the second to make it reads the first's. A file there that doesn't hold an
// id fails, as replacing it would give the Mac another's folder.
func InstallID(stateDir string) (string, error) {
	path := filepath.Join(stateDir, idFile)
	id, err := readID(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return id, err
	}
	if err := makeID(stateDir, path); err != nil {
		return "", fmt.Errorf("make the install's id: %w", err)
	}
	return readID(path)
}

// readID reads the id the file at path holds.
func readID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	id, ended := strings.CutSuffix(string(data), "\n")
	if !ended || !isID(id) {
		return "", fmt.Errorf("%s doesn't hold an install id, 16 lowercase hex characters: removing it gives this Mac a new id, and a folder of its own", path)
	}
	return id, nil
}

// isID reports whether s is an install's id: 16 lowercase hex characters.
func isID(s string) bool {
	return len(s) == 2*idBytes && strings.Trim(s, "0123456789abcdef") == ""
}

// makeID writes a new id to a file beside path, in dir, which it makes where
// it isn't there, then links it to path, which fails where path is there,
// as another maker put its id there first, leaving that one in place.
func makeID(dir, path string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+idFile+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	var b [idBytes]byte
	// crypto/rand's Read never fails: it crashes the program instead.
	_, _ = rand.Read(b[:])
	_, err = tmp.WriteString(hex.EncodeToString(b[:]) + "\n")
	if err == nil {
		err = tmp.Sync()
	}
	if err = errors.Join(err, tmp.Close()); err != nil {
		return err
	}
	if err := os.Link(tmp.Name(), path); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

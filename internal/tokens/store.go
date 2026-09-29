// Package tokens keeps the accounts' tokens: a file each, holding the token
// alone, in a directory of the state directory's that only the user can use.
// A token file counts only while it's the user's, and no one else can read or
// write it.
package tokens

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

const (
	// dirName is the tokens directory's name in the state directory.
	dirName = "tokens"
	// maxFile is the most a token file can hold: setup tokens are about a
	// hundred bytes, so a file any larger holds more than a token.
	maxFile = 4 << 10
	// shared are the permission bits that let others read or write a file.
	shared fs.FileMode = 0o066
	// private are the tokens directory's permissions: the user's alone.
	private fs.FileMode = 0o700
)

// Store is the token files of a state directory: <state dir>/tokens/<id>, one
// for each account. Each is named by its account's id, which config keeps a
// plain file name.
type Store struct {
	dir string
	// uid is the user's id: a token file must be theirs.
	uid int
}

// NewStore returns the token files in the state directory given, which count
// as the user's whose id is uid.
func NewStore(stateDir string, uid int) Store {
	return Store{dir: filepath.Join(stateDir, dirName), uid: uid}
}

// Path is where the token file of the account with the given id is.
func (s Store) Path(id string) string {
	return filepath.Join(s.dir, id)
}

// Read returns the token in the account's token file, ignoring the whitespace
// around it. It fails, saying why and what would put it right, when the file
// is missing, isn't the user's alone, or holds no token or more than one: an
// error that wraps ErrMissing when the file is missing or holds no token.
func (s Store) Read(id string) (Token, error) {
	path := s.Path(id)
	data, err := s.readPrivate(path)
	if err != nil {
		return Token{}, err
	}
	token, err := parse(data)
	switch {
	case errors.Is(err, ErrMissing):
		return Token{}, fmt.Errorf("%w: write it to %s, which is empty", ErrMissing, path)
	case err != nil:
		return Token{}, fmt.Errorf("the token file %s holds more than a token: write the token alone to it", path)
	}
	return token, nil
}

// readPrivate returns what the file at path holds, up to a byte more than
// maxFile, once it's shown to be a file of the user's that no one else can
// read or write. What it checks is the file it has open, whatever takes its
// place at path meanwhile.
func (s Store) readPrivate(path string) ([]byte, error) {
	f, err := os.OpenFile(path, openFlags, 0)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("%w: write it to %s", ErrMissing, path)
	case err != nil:
		return nil, fmt.Errorf("read the token file: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("read the token file: %w", err)
	}
	if err := s.checkPrivate(path, info); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, fmt.Errorf("read the token file: %w", err)
	}
	return data, nil
}

// checkPrivate fails, saying why and what would put it right, for the token
// file at path, which info describes, unless it's a file, it's the user's,
// and no one else can read or write it.
func (s Store) checkPrivate(path string, info fs.FileInfo) error {
	mode := info.Mode()
	if !mode.IsRegular() {
		return fmt.Errorf("the token file %s isn't a file: replace it with one holding the token", path)
	}
	if uid, ok := owner(info); !ok || uid != s.uid {
		return fmt.Errorf("the token file %s is another user's: replace it with one of your own", path)
	}
	if exposed := mode.Perm() & shared; exposed != 0 {
		return fmt.Errorf("other users can %s the token file (mode %04o): chmod 600 %s", access(exposed), mode.Perm(), path)
	}
	return nil
}

// access says what the permission bits exposed let others do with a file.
func access(exposed fs.FileMode) string {
	read, write := exposed&0o044 != 0, exposed&0o022 != 0
	switch {
	case read && write:
		return "read and write"
	case read:
		return "read"
	default:
		return "write"
	}
}

// parse returns the token a token file's contents hold, as Parse does, but
// for contents larger than a token file can be.
func parse(data []byte) (Token, error) {
	if len(data) > maxFile {
		return Token{}, ErrNotAToken
	}
	return Parse(string(data))
}

// Write replaces the account's token file with one holding token, which only
// the user can read or write, having made the tokens directory, or made it
// theirs alone. The file is written whole or not at all, and synced, as a
// token lost to a crash takes a new setup token to replace.
func (s Store) Write(id string, token Token) error {
	if err := s.keepPrivate(); err != nil {
		return err
	}
	if err := atomicfile.Write(s.Path(id), []byte(token.Reveal()+"\n"), 0o600); err != nil {
		return fmt.Errorf("write the token file: %w", err)
	}
	return nil
}

// keepPrivate makes the tokens directory, with any of the state directory
// missing, or makes it the user's alone when it's there already.
func (s Store) keepPrivate() error {
	if err := os.MkdirAll(s.dir, private); err != nil {
		return fmt.Errorf("create the tokens directory: %w", err)
	}
	if err := os.Chmod(s.dir, private); err != nil {
		return fmt.Errorf("make the tokens directory private: %w", err)
	}
	return nil
}

// Dir is where the token files are.
func (s Store) Dir() string {
	return s.dir
}

// Tighten makes the tokens directory the user's alone, as Write keeps it,
// when it's there: it never makes one. It returns the permissions the
// directory had, and reports whether it changed them.
func (s Store) Tighten() (was fs.FileMode, tightened bool, err error) {
	info, err := os.Stat(s.dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("make the tokens directory private: %w", err)
	case !info.IsDir():
		return 0, false, fmt.Errorf("the tokens directory %s isn't a directory: remove it, and write the tokens again", s.dir)
	}
	was = info.Mode().Perm()
	if was == private {
		return was, false, nil
	}
	if err := os.Chmod(s.dir, private); err != nil {
		return was, false, fmt.Errorf("make the tokens directory private: %w", err)
	}
	return was, true, nil
}

// Has reports whether the account has a token file, usable or not.
func (s Store) Has(id string) bool {
	_, err := os.Lstat(s.Path(id))
	return err == nil
}

// Remove deletes the account's token file, if it has one.
func (s Store) Remove(id string) error {
	if err := os.Remove(s.Path(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the token file: %w", err)
	}
	return nil
}

// Package tokens keeps the accounts' tokens: a file each, holding the token
// alone, in a directory of the state directory's that only the user can use.
// A token file counts only while it's the user's, and no one else can read or
// write it. Beside each it writes goes the record of when it took it.
package tokens

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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
// for each account, and beside each the record of when switchboard took its
// token, <id>.taken. Each is named by its account's id, which config keeps a
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
// error that wraps ErrMissing when the file is missing or holds no token, and
// ErrEmpty too when it holds none.
func (s Store) Read(id string) (Token, error) {
	path := s.Path(id)
	data, err := s.readPrivate(path)
	if err != nil {
		return Token{}, err
	}
	token, err := parse(data)
	switch {
	case errors.Is(err, ErrMissing):
		return Token{}, fmt.Errorf("%w: write it to %s, which is empty", ErrEmpty, path)
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
	data, err := readChecked(path, "the token file", maxFile, func(info fs.FileInfo) error { return s.checkPrivate(path, info) })
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: write it to %s", ErrMissing, path)
	}
	return data, err
}

// readChecked returns what the file at path holds, up to a byte more than
// most, once check passes it, as the file it has open: whatever takes its
// place at path meanwhile. It fails with check's error, else saying it
// couldn't read what, wrapping fs.ErrNotExist where there's nothing at path.
func readChecked(path, what string, most int64, check func(fs.FileInfo) error) ([]byte, error) {
	f, err := os.OpenFile(path, openFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	if err := check(info); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, most+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
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
	if !s.owns(info) {
		return fmt.Errorf("the token file %s is another user's: replace it with one of your own", path)
	}
	if exposed := mode.Perm() & shared; exposed != 0 {
		return fmt.Errorf("other users can %s the token file (mode %04o): chmod 600 %s", access(exposed), mode.Perm(), path)
	}
	return nil
}

// owns reports whether the file info describes is the user's.
func (s Store) owns(info fs.FileInfo) bool {
	uid, ok := owner(info)
	return ok && uid == s.uid
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
// theirs alone: through a link, where it leads, rather than in its place. The
// file is written whole or not at all, and synced, as a token lost to a crash
// takes a new setup token to replace. Beside it, the record of when each of
// the account's latest tokens was taken, which Taken reads, gains token,
// taken at taken, as writeTaken writes it: a token it has a date of already
// keeps the earlier of the two. Failing to write that leaves the token's date
// unknown, and is logged, never failing the write. The token and its record
// are written under the tokens directory's lock, which Remove takes too, so a
// write and a removal at once can't leave a record of a removed account.
func (s Store) Write(id string, token Token, taken time.Time) error {
	if err := s.keepPrivate(); err != nil {
		return err
	}
	defer lockDir(s.dir)()
	target, err := atomicfile.Target(s.Path(id))
	if err != nil {
		return fmt.Errorf("write the token file: %w", err)
	}
	if err := atomicfile.Write(target, []byte(token.Reveal()+"\n"), 0o600); err != nil {
		return fmt.Errorf("write the token file: %w", err)
	}
	if err := s.writeTaken(id, token, taken); err != nil {
		logger.Warn("couldn't record when the token was taken", "account", id, "error", err)
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
// directory had, and reports whether it changed them. It makes the records
// in it of when each token was taken private too, as tightenRecord does.
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
	tightened = was != private
	if tightened {
		if err := os.Chmod(s.dir, private); err != nil {
			return was, false, fmt.Errorf("make the tokens directory private: %w", err)
		}
	}
	s.tightenRecords()
	return was, tightened, nil
}

// Removed is what removing an account's token file removed.
type Removed struct {
	// File is set when there was a token file, usable or not.
	File bool
	// LinkedTo is where the token file led, when it was a link: "" when it
	// wasn't.
	LinkedTo string
	// LinkedToGone is set where the name the token file led to, the
	// record's own path, a second name of the token's file, went with it:
	// the file's kept under its other name.
	LinkedToGone bool
}

// Remove deletes the account's token file, if it has one, then the record of
// when its token was taken, and says what it removed of the token file. A
// token file it can't remove keeps its record. A link in either's place goes,
// but not the file it leads to, which is kept elsewhere, as by a dotfiles
// step, and isn't switchboard's to delete. As tokenAtRecord judges it before
// the token file goes: a record that's the token's file, the one it leads to,
// by that one name, is the token's only copy, and is left; one that's a
// second name of it is the token being removed, and its name goes with it.
// Anything else at the record's path goes as forgetTaken says. It holds the
// tokens directory's lock, as Write does.
func (s Store) Remove(id string) (Removed, error) {
	defer lockDir(s.dir)()
	same, only := s.tokenAtRecord(id)
	removed, err := s.removeToken(id)
	if err != nil {
		return Removed{}, err
	}
	switch {
	case only:
	case same:
		gone := s.removeRecord(id)
		removed.LinkedToGone = gone && removed.LinkedTo != "" && sameName(removed.LinkedTo, s.takenPath(id))
	default:
		s.forgetTaken(id)
	}
	return removed, nil
}

// tokenAtRecord reports whether the file at the account's record's own path,
// by that name, not followed where it's a link, is the token file's, or the
// one the token file leads to, by the file, however a path to it is spelled;
// and whether that's its only name. Where it can't tell how many names the
// file has, it reports neither, so the record goes as forgetTaken says.
func (s Store) tokenAtRecord(id string) (same, only bool) {
	record, err := os.Lstat(s.takenPath(id))
	if err != nil || !record.Mode().IsRegular() {
		return false, false
	}
	token, err := os.Stat(s.Path(id))
	if err != nil || !os.SameFile(record, token) {
		return false, false
	}
	n, ok := links(record)
	if !ok {
		return false, false
	}
	return true, n == 1
}

// removeToken deletes the account's token file, if it has one, as Remove
// does, and says what it removed.
func (s Store) removeToken(id string) (Removed, error) {
	path := s.Path(id)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Removed{}, nil
	case err != nil:
		return Removed{}, fmt.Errorf("remove the token file: %w", err)
	}
	removed := Removed{File: true}
	if info.Mode()&fs.ModeSymlink != 0 {
		removed.LinkedTo = linkedTo(path)
	}
	if err := os.Remove(path); err != nil {
		return Removed{}, fmt.Errorf("remove the token file: %w", err)
	}
	return removed, nil
}

// linkedTo is where the link at path leads, as atomicfile.Target finds it,
// else as the link itself says, as of a loop of links.
func linkedTo(path string) string {
	if target, err := atomicfile.Target(path); err == nil {
		return target
	}
	target, _ := os.Readlink(path)
	return target
}

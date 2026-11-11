package tokens

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Give gives the account with the id to the token file of the account with
// the id from: to's token file a link leading to from's, by its name, so the
// token reads the same by either id, and a token written by either is
// written to the one file, as Write writes through a link. So a router going
// by either id finds the account's token, till Drop moves from's file onto
// to's name, and the two make a rename that never leaves the token unnamed.
// The record of when the token was taken stays from's alone till then, so a
// token written by to meanwhile is recorded as to's own, as a record that's
// a second name of another account's is never written. A token file to has
// already is made afresh where it isn't that link, as from's is the one
// there to go by; where from has none, nothing's given. It holds the tokens
// directory's lock, as Write does.
func (s Store) Give(from, to string) error {
	defer lockDir(s.dir)()
	if err := give(s.Path(from), s.Path(to)); err != nil {
		return fmt.Errorf("give %s's token file to %s: %w", from, to, err)
	}
	return nil
}

// Drop takes away the name of the token file of the account with the id
// from, moving what it names, a file or a link of the user's own, onto the
// name Give gave the account with the id to, in place of the link it made
// there, so to's holds whatever was written by either id since; and takes
// away from's record of when its token was taken, moving it onto to's where
// to has none, as where to has one, it's of a token written by to since.
// Where to's token file isn't that link any more, as where what's at to was
// written afresh, from's token file and record are left, with a warning. It
// holds the tokens directory's lock, as Write does.
func (s Store) Drop(from, to string) error {
	defer lockDir(s.dir)()
	moved, err := drop(s.Path(from), s.Path(to))
	if err == nil && moved {
		err = dropRecord(s.takenPath(from), s.takenPath(to))
	}
	if err != nil {
		return fmt.Errorf("move %s's token file onto %s: %w", from, to, err)
	}
	if !moved {
		logger.Warn("left the token file of an account's former id, as its id has one of its own", "id", to, "former", from)
	}
	return nil
}

// give gives the name to to what's at from, as Give says.
func give(from, to string) error {
	if there, err := present(from); err != nil || !there {
		return err
	}
	if linked(from, to) {
		return nil
	}
	tmp := filepath.Join(filepath.Dir(to), "."+filepath.Base(to)+".given")
	_ = os.Remove(tmp)
	if err := os.Symlink(filepath.Base(from), tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, to); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// drop moves what's at from onto to, where to is the link Give made,
// reporting whether it did, or there was nothing at from to move.
func drop(from, to string) (bool, error) {
	if there, err := present(from); err != nil || !there {
		return err == nil, err
	}
	if !linked(from, to) {
		return false, nil
	}
	if err := os.Rename(from, to); err != nil {
		return false, err
	}
	return true, nil
}

// dropRecord takes away the record at from, moving it onto to where nothing's
// there, as Drop says.
func dropRecord(from, to string) error {
	there, err := present(to)
	switch {
	case err != nil:
		return err
	case there:
		err = os.Remove(from)
	default:
		err = os.Rename(from, to)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// present reports whether anything's at path, not followed where it's a
// link.
func present(path string) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// linked reports whether to is the link Give makes, leading to from by its
// name in the directory both are in.
func linked(from, to string) bool {
	dest, err := os.Readlink(to)
	return err == nil && dest == filepath.Base(from)
}

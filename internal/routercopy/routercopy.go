// Package routercopy keeps the router's own copy of switchboard, the binary
// the LaunchAgent runs. macOS keeps what the user allows the router against
// the path of the file it runs, and Homebrew puts each version at a path of
// its own, so the copy sits at a path that never changes.
//
// The copy is only ever replaced by a rename, never written into: macOS
// remembers a signed binary's signature by its file, and kills one changed in
// place.
package routercopy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
)

// FromFlag names serve's flag, --from, naming the binary the router upgrades
// from: the one service install was run by, as it was given, such as
// Homebrew's link.
const FromFlag = "from"

const (
	// name is the copy's file name, in its directory.
	name = "switchboard"
	// nextName is the one name StageNext stages a binary at, beside the copy.
	// It's named as a file written beside the copy is, so one a router left
	// there is removed with what copies cut short left.
	nextName = "." + name + ".next"
	// lockName is the file in the copy's directory whose lock Lock takes.
	// It isn't named as a file written beside the copy is, so it's never
	// taken for one a copy cut short left, and it goes with the directory.
	lockName = ".lock"
	// fileMode is the copy's permissions, and dirMode its directory's, which
	// holds nothing of anyone else's.
	fileMode fs.FileMode = 0o755
	dirMode  fs.FileMode = 0o700
)

// Copy is the router's own copy of switchboard.
type Copy struct {
	path string
}

// Find returns the router's copy, where the environment getenv reads keeps
// switchboard's data, as config.RouterDir finds it.
func Find(getenv func(string) string, homeDir func() (string, error)) (Copy, error) {
	dir, err := config.RouterDir(getenv, homeDir)
	if err != nil {
		return Copy{}, err
	}
	return Copy{path: filepath.Join(dir, name)}, nil
}

// At returns the router's copy at path, as a router running it knows it: by
// the path it was started as.
func At(path string) Copy {
	return Copy{path: path}
}

// Path is where the copy is.
func (c Copy) Path() string {
	return c.path
}

// dir is the copy's directory.
func (c Copy) dir() string {
	return filepath.Dir(c.path)
}

// Make makes the copy of binary, its links followed, in place of any copy
// there, as Stage then Commit do.
func (c Copy) Make(binary string) error {
	staged, err := c.Stage(binary)
	if err != nil {
		return err
	}
	return staged.Commit()
}

// Staged is a copy of a binary made beside the router's copy, hidden, to be
// renamed over it or removed.
type Staged struct {
	path, over string
	from       fs.FileInfo
}

// Path is where the staged copy is, for it to be run before it's committed.
func (s Staged) Path() string {
	return s.path
}

// From is the file the copy was made of, as it stood once it was opened to
// be copied, its links followed: for a caller to tell whether that's the
// file it meant, as a link can move on in the moment before it's opened.
func (s Staged) From() fs.FileInfo {
	return s.from
}

// Commit renames the staged copy over the router's copy, removing it where
// it can't.
func (s Staged) Commit() error {
	if err := os.Rename(s.path, s.over); err != nil {
		_ = os.Remove(s.path)
		return fmt.Errorf("put the copy of switchboard in place of the router's, %s: %w", s.over, err)
	}
	return nil
}

// Discard removes the staged copy: one not there is gone already.
func (s Staged) Discard() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the copy of switchboard staged beside the router's: %w", err)
	}
	return nil
}

// Stage copies binary, its links followed, to a hidden file beside the
// router's copy, synced, and last modified when binary was, its directory
// made where it isn't there.
func (c Copy) Stage(binary string) (Staged, error) {
	staged, err := c.stage(binary)
	if err != nil {
		return Staged{}, fmt.Errorf("copy %s beside the router's copy, %s: %w", binary, c.path, err)
	}
	return staged, nil
}

// StageNext stages binary as Stage does, but at one name beside the router's
// copy that never changes, .switchboard.next, in place of any staged there:
// macOS keeps what it allows a binary run there against its path, as it does
// the copy's. It's staged as Stage stages it, then renamed to that name, so
// what's found there is always whole. Two staged at once write over each
// other: whoever stages here does so one at a time.
func (c Copy) StageNext(binary string) (Staged, error) {
	staged, err := c.Stage(binary)
	if err != nil {
		return Staged{}, err
	}
	next := filepath.Join(c.dir(), nextName)
	if err := os.Rename(staged.path, next); err != nil {
		_ = os.Remove(staged.path)
		return Staged{}, fmt.Errorf("copy %s beside the router's copy, %s: %w", binary, c.path, err)
	}
	staged.path = next
	return staged, nil
}

// stage is Stage, unwrapped.
func (c Copy) stage(binary string) (Staged, error) {
	source, err := os.Open(binary)
	if err != nil {
		return Staged{}, err
	}
	defer func() { _ = source.Close() }()
	info, err := source.Stat()
	if err != nil {
		return Staged{}, err
	}
	if !info.Mode().IsRegular() {
		return Staged{}, fmt.Errorf("%s isn't a file", binary)
	}
	if err := c.makeDir(); err != nil {
		return Staged{}, err
	}
	staged, err := atomicfile.StageModified(c.path, fileMode, info.ModTime(), func(w io.Writer) error {
		_, err := io.Copy(w, source)
		return err
	})
	if err != nil {
		return Staged{}, err
	}
	return Staged{path: staged, over: c.path, from: info}, nil
}

// makeDir makes the copy's directory, private, where it isn't there, and the
// directories it's in as any of switchboard's data directories are made.
func (c Copy) makeDir() error {
	if err := os.MkdirAll(filepath.Dir(c.dir()), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(c.dir(), dirMode); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

// Matches reports whether the copy is one of binary, its links followed, as
// the copy keeps its size and its modification time; and, where the sizes
// agree but the times don't, as on a disk that keeps coarser times than
// binary's, by their bytes: no copy matches none.
func (c Copy) Matches(binary string) (bool, error) {
	want, err := os.Stat(binary)
	if err != nil {
		return false, fmt.Errorf("compare the router's copy with %s: %w", binary, err)
	}
	have, err := os.Stat(c.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("compare the router's copy with %s: %w", binary, err)
	case have.Size() != want.Size():
		return false, nil
	case have.ModTime().Equal(want.ModTime()):
		return true, nil
	}
	same, err := sameBytes(c.path, binary)
	if err != nil {
		return false, fmt.Errorf("compare the router's copy with %s: %w", binary, err)
	}
	return same, nil
}

// sameBytes reports whether the files at a and b hold the same bytes, read
// only as far as the first that differs.
func sameBytes(a, b string) (bool, error) {
	fa, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer func() { _ = fa.Close() }()
	fb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer func() { _ = fb.Close() }()
	bufA, bufB := make([]byte, 64<<10), make([]byte, 64<<10)
	for {
		na, errA := io.ReadFull(fa, bufA)
		nb, errB := io.ReadFull(fb, bufB)
		if err := errors.Join(unended(errA), unended(errB)); err != nil {
			return false, err
		}
		if !bytes.Equal(bufA[:na], bufB[:nb]) {
			return false, nil
		}
		if na < len(bufA) {
			return true, nil
		}
	}
}

// unended is err, a full read's failure, where it's more than the file
// ending first: nil otherwise.
func unended(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil
	}
	return err
}

// Missing reports whether the copy is known not to be there, nor a file its
// link leads to: one that can't be looked at isn't counted missing.
func (c Copy) Missing() bool {
	_, err := os.Stat(c.path)
	return errors.Is(err, fs.ErrNotExist)
}

// RemoveLeftovers removes the hidden copies beside the router's copy that a
// copy cut short left, or that were staged and neither committed nor
// discarded: none where its directory isn't there.
func (c Copy) RemoveLeftovers() error {
	leftovers, err := atomicfile.Leftovers(c.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("find what copies cut short left beside the router's copy: %w", err)
	}
	var errs []error
	for _, leftover := range leftovers {
		if err := os.Remove(leftover); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("remove what copies cut short left beside the router's copy: %w", err)
	}
	return nil
}

// Lock takes the copy's lock, waiting while another holds it, and returns
// what lets it go, which may be called more than once. Whoever changes the
// copy and the plist that runs it holds it, service install and a router
// moving over to the copy, so neither writes over what the other is
// writing. It makes the copy's directory where it isn't there. The lock goes
// with the process, its exec among its ends.
func (c Copy) Lock() (unlock func(), err error) {
	f, err := c.openLock()
	if err != nil {
		return nil, err
	}
	if err := lock(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock the router's copy, %s: %w", f.Name(), err)
	}
	return func() { _ = f.Close() }, nil
}

// TryLock takes the copy's lock as Lock does, but where another holds it,
// reports false at once, holding nothing.
func (c Copy) TryLock() (unlock func(), ok bool, err error) {
	f, err := c.openLock()
	if err != nil {
		return nil, false, err
	}
	switch taken, err := tryLock(f); {
	case err != nil:
		_ = f.Close()
		return nil, false, fmt.Errorf("lock the router's copy, %s: %w", f.Name(), err)
	case !taken:
		_ = f.Close()
		return nil, false, nil
	}
	return func() { _ = f.Close() }, true, nil
}

// openLock opens the file whose lock Lock takes, making it and the copy's
// directory where they aren't there.
func (c Copy) openLock() (*os.File, error) {
	if err := c.makeDir(); err != nil {
		return nil, fmt.Errorf("make the router's copy's directory, %s: %w", c.dir(), err)
	}
	f, err := os.OpenFile(filepath.Join(c.dir(), lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the router's copy's lock: %w", err)
	}
	return f, nil
}

// Remove removes the copy's directory whole, the copy and anything staged
// beside it: none where it isn't there.
func (c Copy) Remove() error {
	if err := os.RemoveAll(c.dir()); err != nil {
		return fmt.Errorf("remove the router's copy of switchboard: %w", err)
	}
	return nil
}

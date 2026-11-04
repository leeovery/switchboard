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
}

// Path is where the staged copy is, for it to be run before it's committed.
func (s Staged) Path() string {
	return s.path
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
	return Staged{path: staged, over: c.path}, nil
}

// stage is Stage, unwrapped.
func (c Copy) stage(binary string) (string, error) {
	source, err := os.Open(binary)
	if err != nil {
		return "", err
	}
	defer func() { _ = source.Close() }()
	info, err := source.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s isn't a file", binary)
	}
	if err := c.makeDir(); err != nil {
		return "", err
	}
	return atomicfile.StageModified(c.path, fileMode, info.ModTime(), func(w io.Writer) error {
		_, err := io.Copy(w, source)
		return err
	})
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
// the copy keeps its size and its modification time: no copy matches none.
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
	}
	return have.Size() == want.Size() && have.ModTime().Equal(want.ModTime()), nil
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

// Remove removes the copy's directory whole, the copy and anything staged
// beside it: none where it isn't there.
func (c Copy) Remove() error {
	if err := os.RemoveAll(c.dir()); err != nil {
		return fmt.Errorf("remove the router's copy of switchboard: %w", err)
	}
	return nil
}

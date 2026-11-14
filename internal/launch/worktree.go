package launch

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// worktreeFileMost is the most bytes read of a .git file or a commondir, a
// path on a line: one longer is none git wrote.
// configMost is the most read of a common directory's config, for its
// [core] section, which git writes first.
const (
	worktreeFileMost = 4096
	configMost       = 64 << 10
)

// Worktree is what FindWorktree finds of the linked worktrees around a
// directory.
type Worktree struct {
	// Around are the linked worktrees around the directory, nearest first.
	Around []Linked
	// Lost is the root of the nearest linked worktree around the directory
	// that can't be placed, Around then being none: "" where there's none.
	// NoCheckout says why: its repository has no checkout to place it by, as
	// a submodule's or a separate git directory's; else its repository
	// can't be found.
	Lost       string
	NoCheckout bool
}

// Linked is a linked worktree around a directory: its root, and the
// directory's place in its repository, as far below the main checkout as
// the directory is below the root; or, Bare, the bare repository's own
// directory, whatever the directory's path below the root.
type Linked struct {
	Root, Place string
	Bare        bool
}

// worktreeKind is what a directory's .git says of a linked worktree there.
type worktreeKind int

const (
	// worktreeNone is no .git, or one that isn't a linked worktree's: a
	// directory, or a file whose git directory has no commondir, as a main
	// checkout's or a submodule's.
	worktreeNone worktreeKind = iota
	// worktreeLinked is a linked worktree's, whose repository is found.
	worktreeLinked
	// worktreeLost is a .git that can't be looked at, or a file leading to
	// no repository that can be found.
	worktreeLost
	// worktreeNoCheckout is a linked worktree's whose repository has no
	// checkout to place it by: its common directory isn't named .git, and
	// the repository isn't bare.
	worktreeNoCheckout
)

// FindWorktree returns what dir is in among linked worktrees, walking up
// from dir, as it's given, through every directory to the filesystem's
// root, as worktreeAt reads each one's .git: every linked worktree around
// it, so a submodule or a clone nested in a worktree is placed by the
// worktree around it; or, where it meets one that can't be placed, that one
// alone, as Lost. It runs no git. None for "".
func FindWorktree(dir string) Worktree {
	if dir == "" {
		return Worktree{}
	}
	var w Worktree
	for at, rest := filepath.Clean(dir), ""; ; at, rest = filepath.Dir(at), filepath.Join(filepath.Base(at), rest) {
		switch linked, kind := worktreeAt(at, rest); kind {
		case worktreeLinked:
			w.Around = append(w.Around, linked)
		case worktreeLost, worktreeNoCheckout:
			return Worktree{Lost: at, NoCheckout: kind == worktreeNoCheckout}
		}
		if filepath.Dir(at) == at {
			return w
		}
	}
}

// worktreeAt says what the .git in the directory at is, and, where it's a
// linked worktree's, where a path rest below at is in its repository: below
// the main checkout, the directory holding the common directory its git
// directory's commondir names, where that's named .git; else, where the
// common directory's config says the repository is bare, the common
// directory itself. It's lost where the .git can't be looked at, or names
// no git directory that's there, or the git directory's commondir can't be
// read, is empty or names no directory; and it has no checkout where the
// common directory, not named .git, is of a repository that isn't bare, as
// a submodule's or a separate git directory's, whose checkout nothing names.
// Each path read is read as git reads it, from the real path of where it's
// written (see gitPath).
func worktreeAt(at, rest string) (Linked, worktreeKind) {
	info, err := os.Stat(filepath.Join(at, ".git"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Linked{}, worktreeNone
	case err != nil:
		return Linked{}, worktreeLost
	case info.IsDir():
		return Linked{}, worktreeNone
	}
	gitDir, ok := gitPath(at, ".git", "gitdir: ")
	if !ok || !isDirectory(gitDir) {
		return Linked{}, worktreeLost
	}
	if _, err := os.Stat(filepath.Join(gitDir, "commondir")); errors.Is(err, fs.ErrNotExist) {
		return Linked{}, worktreeNone
	}
	common, ok := gitPath(gitDir, "commondir", "")
	switch {
	case !ok || !isDirectory(common):
		return Linked{}, worktreeLost
	case filepath.Base(common) == ".git":
		return Linked{Root: at, Place: filepath.Join(filepath.Dir(common), rest)}, worktreeLinked
	case isBare(common):
		return Linked{Root: at, Place: common, Bare: true}, worktreeLinked
	}
	return Linked{}, worktreeNoCheckout
}

// gitPath returns the path the file name in the directory dir names, as git
// writes one: on a line, after prefix, and where it isn't absolute, relative
// to dir's real path, every link resolved, as git reads it, so a submodule's
// .git, which git writes relative, reads alike through a link to it. It
// reports false where the file can't be read, or names none.
func gitPath(dir, name, prefix string) (string, bool) {
	data, read := readHead(filepath.Join(dir, name), worktreeFileMost)
	named, ok := strings.CutPrefix(strings.TrimRight(string(data), "\r\n"), prefix)
	if !read || len(data) > worktreeFileMost || !ok || named == "" {
		return "", false
	}
	if filepath.IsAbs(named) {
		return filepath.Clean(named), true
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return filepath.Join(dir, named), true
}

// isBare reports whether the repository whose common directory is common is
// bare, as its config's [core] section says: bare = true, yes, on or 1, or
// bare alone.
func isBare(common string) bool {
	data, ok := readHead(filepath.Join(common, "config"), configMost)
	if !ok {
		return false
	}
	section := ""
	for line := range strings.Lines(string(data)) {
		line, _, _ = strings.Cut(strings.TrimSpace(line), "#")
		line, _, _ = strings.Cut(line, ";")
		key, value, valued := strings.Cut(line, "=")
		switch {
		case strings.HasPrefix(line, "["):
			section, _, _ = strings.Cut(line[1:], "]")
		case strings.EqualFold(strings.TrimSpace(section), "core") && strings.EqualFold(strings.TrimSpace(key), "bare"):
			value = strings.ToLower(strings.TrimSpace(value))
			return !valued || value == "true" || value == "yes" || value == "on" || value == "1"
		}
	}
	return false
}

// readHead returns the start of the regular file at path: most bytes and
// one more at the most, so a reader can tell one longer than most. It
// reports false where it can't be read, or is no regular file, as opening a
// pipe would wait.
func readHead(path string, most int64) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, most+1))
	return data, err == nil
}

// isDirectory reports whether path is a directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

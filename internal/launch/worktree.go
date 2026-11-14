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
// configMost is the most read of a common directory's config or
// config.worktree, for the settings in it, last ones winning.
const (
	worktreeFileMost = 4096
	configMost       = 1 << 20
)

// Worktree is what FindWorktree finds of the linked worktrees around a
// directory.
type Worktree struct {
	// Around are the linked worktrees around the directory, nearest first.
	Around []Linked
	// Lost is the root of the nearest linked worktree around the directory
	// that can't be placed, Around then being none: "" where there's none.
	// NoCheckout says why: it's a submodule's or a separate git directory's
	// worktree, which can't be placed; else its repository can't be found.
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
	// worktreeNoCheckout is a linked worktree's that can't be placed: its
	// common directory isn't named .git, and the repository isn't bare, as a
	// submodule's or a separate git directory's.
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
// repository is bare, as isBare reads it, the common directory itself.
// It's lost where the .git can't be looked at, or names no git directory
// that's there, or the git directory's commondir can't be read, is empty or
// names no directory; and it can't be placed where the common directory,
// not named .git, is of a repository that isn't bare, as a submodule's or a
// separate git directory's. Each path read is read as git reads it, from
// the real path of where it's written (see gitPath).
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
// bare, as git reads core.bare there: from its config, then, where that
// turns on extensions.worktreeConfig, from its config.worktree, where git
// moves core.bare, the later setting winning.
func isBare(common string) bool {
	read := func(name string) []byte {
		data, _ := readHead(filepath.Join(common, name), configMost)
		return data
	}
	config := read("config")
	bare, _ := configBool(config, "core", "bare")
	if perWorktree, _ := configBool(config, "extensions", "worktreeConfig"); perWorktree {
		if moved, set := configBool(read("config.worktree"), "core", "bare"); set {
			bare = moved
		}
	}
	return bare
}

// configBool returns the last setting of key in section of a git config,
// data, as git reads a boolean: true, yes, on or 1, or key alone; and
// whether it's set at all.
func configBool(data []byte, section, key string) (value, set bool) {
	at := ""
	for line := range strings.Lines(string(data)) {
		line, _, _ = strings.Cut(strings.TrimSpace(line), "#")
		line, _, _ = strings.Cut(line, ";")
		name, said, valued := strings.Cut(line, "=")
		switch {
		case strings.HasPrefix(line, "["):
			at, _, _ = strings.Cut(line[1:], "]")
		case strings.EqualFold(strings.TrimSpace(at), section) && strings.EqualFold(strings.TrimSpace(name), key):
			said = strings.ToLower(strings.TrimSpace(said))
			value, set = !valued || said == "true" || said == "yes" || said == "on" || said == "1", true
		}
	}
	return value, set
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

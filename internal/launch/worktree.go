package launch

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// worktreeFileMost is the most bytes FindWorktree reads of a .git file, a
// path on a line: one longer is none git wrote, as FindProject reads it.
const worktreeFileMost = 4096

// Worktree is what FindWorktree finds of the linked worktree a directory is
// in.
type Worktree struct {
	// Mapped is the directory's place in the main checkout of the repository
	// the worktree is of: as far below the main checkout as the directory is
	// below the worktree's root. "" where the directory is in no linked
	// worktree.
	Mapped string
	// Lost reports a .git file up from the directory that leads to no
	// repository that can be found: it may be a worktree of any.
	Lost bool
}

// FindWorktree returns what dir is in, where it's in a linked worktree: its
// place in its repository's main checkout, as FindProject finds the main
// checkout, the worktree's root being the first directory up from dir, as
// it's given, holding a .git, there a file. Where FindProject finds no
// repository but the worktree itself, as a submodule's or a bare
// repository's, it's in none; but Lost where the .git file names no git
// directory that's there, or can't be read. FindProject gives a worktree
// started at its root as itself both where it counts as itself and where it
// can't be read, so the .git file is read here to tell the two apart. It's
// in none, too, in a main checkout, or where no directory up from dir holds
// a .git. It runs no git. None for "".
func FindWorktree(dir string) Worktree {
	if dir == "" {
		return Worktree{}
	}
	dir = filepath.Clean(dir)
	root, rest, linked := worktreeRoot(dir)
	if !linked {
		return Worktree{}
	}
	switch repo := FindProject(dir); {
	case repo != root && repo != dir:
		return Worktree{Mapped: filepath.Join(repo, rest)}
	case namesGitDir(root):
		return Worktree{}
	}
	return Worktree{Lost: true}
}

// worktreeRoot returns the first directory up from dir, itself included,
// holding a .git, and the rest of dir's path below it, where that .git isn't
// a directory, or can't be looked at: reporting false where it's a
// directory, as a main checkout's, or no directory holds one.
func worktreeRoot(dir string) (string, string, bool) {
	for at, rest := dir, ""; ; at, rest = filepath.Dir(at), filepath.Join(filepath.Base(at), rest) {
		info, err := os.Stat(filepath.Join(at, ".git"))
		switch {
		case err == nil:
			return at, rest, !info.IsDir()
		case !errors.Is(err, fs.ErrNotExist):
			return at, rest, true
		case filepath.Dir(at) == at:
			return "", "", false
		}
	}
}

// namesGitDir reports whether the .git file in the directory at names a git
// directory that's there, as git writes one: "gitdir: " and its path, on a
// line, relative to at where it isn't absolute.
func namesGitDir(at string) bool {
	line, read := worktreeLine(filepath.Join(at, ".git"))
	named, ok := strings.CutPrefix(line, "gitdir: ")
	if !read || !ok || named == "" {
		return false
	}
	if !filepath.IsAbs(named) {
		named = filepath.Join(at, named)
	}
	info, err := os.Stat(named)
	return err == nil && info.IsDir()
}

// worktreeLine returns what the file at path holds, its line ending let go,
// reporting false where it can't be read, is no regular file, as opening a
// pipe would wait, or runs past worktreeFileMost.
func worktreeLine(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, worktreeFileMost+1))
	if err != nil || len(data) > worktreeFileMost {
		return "", false
	}
	return strings.TrimRight(string(data), "\r\n"), true
}

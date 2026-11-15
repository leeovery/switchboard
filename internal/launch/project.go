package launch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// FindProject returns the project a session started in dir runs in: the
// repository dir is in, found by walking up from dir to the first directory
// holding a .git, stopping at the filesystem's root, each read as worktreeAt
// reads it; walked with dir's links resolved first, so a link into a
// repository's subdirectory counts as that repository's, then, where that
// finds no .git, as dir is given, so a link within a repository leading
// outside every one counts as the repository it's in, as a pool's reading
// looks at dir both ways. A linked worktree counts as its repository: its
// main checkout, or, where the repository is bare, the bare repository's
// directory, so its worktrees are one project, but in a .bare layout the
// folder holding it, as projectOf has it. A worktree whose git directory has
// no commondir, as a submodule's, counts as itself, as does one worktreeAt
// can't place, as a submodule's linked worktree, or a separate git
// directory's. Each is given by the path dir is given by, as walkedTo has
// it, where that goes through the repository or a directory it's in; else
// by its path with every link resolved. Where neither walk finds a .git, or
// the first found can't be read as a repository, as worktreeAt finds it
// lost, the project is dir itself, as given. It runs no git. "" for "".
func FindProject(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	for _, from := range resolvedFirst(dir) {
		if repo, held := repositoryAround(from); held {
			if repo == "" {
				return dir
			}
			return walkedTo(dir, repo)
		}
	}
	return dir
}

// RepositoryOf returns the project of the repository the directory dir is
// in, as FindProject named a worktree kept there while it was, and whether
// dir is in one: of the nearest directory, dir itself or one up from it,
// that's a bare repository's directory, as isBare reads it, that
// directory; that holds a bare repository named .bare, that folder, as
// projectOf names a .bare layout's; or that holds a .git, as repositoryAt
// reads it, the project FindProject finds of dir. So the folder a worktree
// since removed was kept in, in any of those, gives the project the
// worktree had, though FindProject finds the folder its own. It runs no git,
// and follows dir as it's given. False for "".
func RepositoryOf(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	for at := filepath.Clean(dir); ; at = filepath.Dir(at) {
		switch _, held := repositoryAt(at); {
		case isBare(at):
			return at, true
		case isBare(filepath.Join(at, bareLayout)):
			return at, true
		case held:
			return FindProject(dir), true
		}
		if filepath.Dir(at) == at {
			return "", false
		}
	}
}

// resolvedFirst are the paths dir is walked up from: dir with its links
// resolved, then dir as it's given, where that differs.
func resolvedFirst(dir string) []string {
	if real := resolvedPath(dir); real != dir {
		return []string{real, dir}
	}
	return []string{dir}
}

// resolvedPath is path with its links resolved, as a pool's reading
// resolves the directory a session starts in, or path as it is where they
// can't be, as where nothing's there.
func resolvedPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// repositoryAround returns the repository the directory from is in, walking
// up from it to the first directory holding a .git, to the filesystem's root
// at most, as repositoryAt reads it, and whether any holds one: "" where
// the first can't be read as a repository.
func repositoryAround(from string) (string, bool) {
	for at := from; ; at = filepath.Dir(at) {
		if repo, held := repositoryAt(at); held {
			return repo, true
		}
		if filepath.Dir(at) == at {
			return "", false
		}
	}
}

// repositoryAt returns the repository whose .git the directory at holds, and
// whether it holds one, as worktreeAt reads it: a linked worktree's is its
// repository, as projectOf has it, and every other's at itself; "" where
// it's lost.
func repositoryAt(at string) (string, bool) {
	if _, err := os.Stat(filepath.Join(at, ".git")); errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	switch linked, kind := worktreeAt(at, ""); kind {
	case worktreeLinked:
		return projectOf(linked), true
	case worktreeLost:
		return "", true
	}
	return at, true
}

// bareLayout is the name a .bare layout gives the bare repository's
// directory, inside the folder its worktrees are kept in.
const bareLayout = ".bare"

// projectOf is the project of the linked worktree w: its repository's
// place, but in a .bare layout, a bare repository's directory named .bare
// whose worktree lies inside the folder holding it, with its links
// resolved, that folder, as a project is named by its folder's name, and
// .bare would name every such project alike.
func projectOf(w Linked) string {
	if holder := filepath.Dir(w.Place); w.Bare && filepath.Base(w.Place) == bareLayout && inside(resolvedPath(w.Root), holder) {
		return holder
	}
	return w.Place
}

// inside reports whether path is dir or below it, both clean, by their
// paths alone.
func inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

// walkedTo returns the repository repo, as a commondir leads to it, every
// link resolved, by the path the walk up from dir took: the deepest
// directory up from dir, itself included, that is repo, or a directory repo
// is in, through whatever links, with the rest of repo's path below it; repo
// where none is. Each directory up from either is looked at once.
func walkedTo(dir, repo string) string {
	ups := upsOf(repo)
	for at := dir; ; at = filepath.Dir(at) {
		if rest, ok := below(at, ups); ok {
			return filepath.Join(at, rest)
		}
		if filepath.Dir(at) == at {
			return repo
		}
	}
}

// up is a directory up from a path, or the path itself: what it is, and the
// rest of the path below it.
type up struct {
	info fs.FileInfo
	rest string
}

// upsOf returns the directories up from path, path itself first, but those
// that can't be looked at.
func upsOf(path string) []up {
	var ups []up
	for at, rest := path, ""; ; at, rest = filepath.Dir(at), filepath.Join(filepath.Base(at), rest) {
		if info, err := os.Stat(at); err == nil {
			ups = append(ups, up{info: info, rest: rest})
		}
		if filepath.Dir(at) == at {
			return ups
		}
	}
}

// below returns the rest of a path below the directory at, where at is one
// of ups, the directories up from the path, and whether it is.
func below(at string, ups []up) (string, bool) {
	info, err := os.Stat(at)
	if err != nil {
		return "", false
	}
	for _, u := range ups {
		if os.SameFile(info, u.info) {
			return u.rest, true
		}
	}
	return "", false
}

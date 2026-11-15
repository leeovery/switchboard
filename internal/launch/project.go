package launch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FindProject returns the project a session started in dir runs in: the
// repository dir is in, found by walking up from dir with its links
// resolved, as a pool's reading resolves them, so a project and a pool agree
// on where a directory lies, to the first directory holding a .git, stopping
// at the filesystem's root, each read as worktreeAt reads it. A linked
// worktree counts as its repository: its main checkout, or, where the
// repository is bare, the bare repository's directory, so its worktrees are
// one project, or, where that directory's name starts with a dot, as a .bare
// layout's, the folder holding it, as projectOf has it. A worktree whose git
// directory has no commondir, as a submodule's, counts as itself, as does
// one worktreeAt can't place, as a submodule's linked worktree, or a
// separate git directory's. Each is given by the path dir is given by, as
// walkedTo has it, where that goes through the repository or a directory
// it's in; else by its path with every link resolved. Where no directory
// holds a .git, or the first that does can't be read as a repository, as
// worktreeAt finds it lost, the project is dir itself, as given. It runs no
// git. "" for "".
func FindProject(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	repo, found := repositoryAround(resolvedPath(dir))
	if !found {
		return dir
	}
	return walkedTo(dir, repo)
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
// at most, as repositoryAt reads it, and whether there's one that can be
// read as a repository.
func repositoryAround(from string) (string, bool) {
	for at := from; ; at = filepath.Dir(at) {
		if repo, held := repositoryAt(at); held {
			return repo, repo != ""
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

// projectOf is the project of the linked worktree w: its repository's
// place, but the folder holding a bare repository's directory whose name
// starts with a dot, as a .bare layout's, as a project is named by its
// folder's name, and .bare would name every such project alike.
func projectOf(w Linked) string {
	if w.Bare && strings.HasPrefix(filepath.Base(w.Place), ".") {
		return filepath.Dir(w.Place)
	}
	return w.Place
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

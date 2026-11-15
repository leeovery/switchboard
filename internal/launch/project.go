package launch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// FindProject returns the project a session started in dir runs in: the
// repository dir is in, by its path, found by walking up from dir as it's
// given, never resolving a link, to the first directory holding a .git,
// stopping at the filesystem's root, each read as worktreeAt reads it, so a
// project and a pool agree on what a worktree belongs to. A linked
// worktree counts as its repository: its main checkout, or, where the
// repository is bare, the bare repository's directory, so its worktrees are
// one project; each by the path the walk took, as walkedTo has it, where it
// went through the repository, or a directory it's in, as from a worktree
// within it or beside it; else by the path git wrote, every link resolved.
// A worktree whose git directory has no commondir, as a submodule's, counts
// as itself, as does one worktreeAt can't place, as a submodule's linked
// worktree, or a separate git directory's. Where no directory holds a .git,
// or the first that does can't be read as a repository, as worktreeAt finds
// it lost, the project is dir itself. It runs no git. "" for "".
func FindProject(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	for at := dir; ; at = filepath.Dir(at) {
		if repo, held := repositoryAt(at); held {
			switch repo {
			case "":
				return dir
			case at:
				return at
			}
			return walkedTo(dir, repo)
		}
		if filepath.Dir(at) == at {
			return dir
		}
	}
}

// repositoryAt returns the repository whose .git the directory at holds, and
// whether it holds one, as worktreeAt reads it: a linked worktree's is its
// repository's place, and every other's at itself; "" where it's lost.
func repositoryAt(at string) (string, bool) {
	if _, err := os.Stat(filepath.Join(at, ".git")); errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	switch linked, kind := worktreeAt(at, ""); kind {
	case worktreeLinked:
		return linked.Place, true
	case worktreeLost:
		return "", true
	}
	return at, true
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

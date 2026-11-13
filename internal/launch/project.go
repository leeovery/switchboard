package launch

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// gitFileMost is the most bytes FindProject reads of a .git file or a
// commondir, each a path on a line of its own: one longer is none git wrote.
const gitFileMost = 4096

// errNotAPath is a .git file or a commondir that doesn't hold a path, as git
// writes one.
var errNotAPath = errors.New("not a path git wrote")

// FindProject returns the project a session started in dir runs in: the
// repository dir is in, by its path, found by walking up from dir as it's
// given, never resolving a link, to the first directory holding a .git,
// stopping at the filesystem's root. A linked worktree's .git is a file
// naming its git directory, whose commondir leads to its main repository's,
// which it counts as, by the path the walk took, as walkedTo has it: where
// it went through the main repository, or a directory it's in, as from a
// worktree within it or beside it; else by the path git wrote, every link
// resolved. A worktree whose git directory has no commondir, as a
// submodule's, or whose common directory isn't named .git, as a bare
// repository's, counts as itself. Where no directory holds a .git, or the
// first that does can't be read as a repository, as a .git file unreadable
// or naming no git directory, or a git directory or common directory that
// isn't there, the project is dir itself. It runs no git, and reads no more
// than a worktree's .git file and its commondir. "" for "".
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
// whether it holds one: a .git directory's repository is at itself, and a
// .git file's is the one it leads to, as linkedRepository has it; "" where
// what it holds can't be read as a repository.
func repositoryAt(at string) (string, bool) {
	git := filepath.Join(at, ".git")
	info, err := os.Stat(git)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", false
	case err != nil:
		return "", true
	case info.IsDir():
		return at, true
	}
	return linkedRepository(at, git), true
}

// linkedRepository returns the repository the .git file at git, of the
// worktree at, leads to: the main repository, the directory holding the
// common directory its git directory's commondir names, a relative path
// each from where it's written; or at itself, where the git directory has no
// commondir, or its common directory isn't named .git. "" where it can't be
// read as one.
func linkedRepository(at, git string) string {
	line, err := readPath(git)
	named, ok := strings.CutPrefix(line, "gitdir: ")
	if err != nil || !ok || named == "" {
		return ""
	}
	gitDir := from(at, named)
	common, err := readPath(filepath.Join(gitDir, "commondir"))
	switch {
	case errors.Is(err, fs.ErrNotExist) && isDir(gitDir):
		return at
	case err != nil || common == "":
		return ""
	}
	switch common = from(gitDir, common); {
	case !isDir(common):
		return ""
	case filepath.Base(common) != ".git":
		return at
	}
	return filepath.Dir(common)
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

// readPath returns the path the file at path holds, as git writes one: a
// line, its line ending let go. It fails where the file is no regular file,
// as reading a pipe would wait, or runs past gitFileMost.
func readPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errNotAPath
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, gitFileMost+1))
	switch {
	case err != nil:
		return "", err
	case len(data) > gitFileMost:
		return "", errNotAPath
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

// from returns path as it's read from the directory dir: as it is where it's
// absolute, else within dir.
func from(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

// isDir reports whether path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

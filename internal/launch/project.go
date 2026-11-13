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
// which it counts as; a git directory without a commondir, as a submodule's,
// is the worktree's own. Where no directory holds a .git, or the first that
// does can't be read as a repository, as a .git file unreadable or naming no
// git directory, a git directory or common directory that isn't there, or a
// worktree of a bare repository, the project is dir itself. It runs no git,
// and reads no more than a worktree's .git file and its commondir. "" for "".
func FindProject(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	for at := dir; ; at = filepath.Dir(at) {
		if repo, held := repositoryAt(at); held {
			if repo == "" {
				return dir
			}
			return repo
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
// commondir. "" where it can't be read as one, as a common directory not
// named .git is a bare repository's.
func linkedRepository(at, git string) string {
	line, err := readPath(git)
	named, ok := strings.CutPrefix(line, "gitdir: ")
	if err != nil || !ok || named == "" {
		return ""
	}
	gitDir := from(at, named)
	common, err := readPath(filepath.Join(gitDir, "commondir"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if isDir(gitDir) {
			return at
		}
		return ""
	case err != nil || common == "":
		return ""
	}
	common = from(gitDir, common)
	if filepath.Base(common) != ".git" || !isDir(common) {
		return ""
	}
	return filepath.Dir(common)
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

package status

import (
	"cmp"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/leeovery/switchboard/internal/claude"
)

// TemporaryFolders is what Projects reads in place of a project for the
// sessions that ran in a temporary folder, together, as Projects' list rows
// them: no path, which no project's can be.
const TemporaryFolders = "temporary folders"

// temporaryRoots are the folders whose every folder is a temporary one, as
// macOS and other Unixes keep them.
var temporaryRoots = []string{"/tmp", "/private/tmp", "/var/folders", "/private/var/folders"}

// ProjectsConfig is what Projects reads a project's path against.
type ProjectsConfig struct {
	// Root is the filesystem from its root, as os.DirFS("/") gives it: it
	// must be given.
	Root fs.FS
	// Home is the home directory, which a project's path gives as ~.
	Home string
	// TempDir is the reader's temporary directory, as os.TempDir gives it.
	TempDir string
	// UID is the user's id, which Claude Code names its scratch root by.
	UID int
	// Find is the project of a directory, by its whole path, as
	// launch.FindProject finds it: nil reads it as ProjectOf does.
	Find func(dir string) string
	// Repository is the project of the repository a directory, by its
	// whole path, is in, and whether it's in one, as launch.RepositoryOf
	// has them: nil, that none is.
	Repository func(dir string) (string, bool)
}

// Projects reads the project a line, a session or the router names, as
// ProjectOf does, and then against the filesystem:
//
//   - a path in a Claude Code scratchpad, as claude.ScratchpadSlug finds
//     one, counts as the project of the directory its slug names, as
//     claude.SlugDir finds it, by its path as Find has it; but as
//     TemporaryFolders where the slug names nothing below the home's
//     parents; and where it names only the start of itself, as of a
//     worktree, a repository or a volume gone, as the project of the
//     repository what it names is in, as Repository has it, else as
//     TemporaryFolders;
//   - a path in a temporary folder, as isTemporary has it, counts as
//     TemporaryFolders;
//   - any other path is given in the case its folders have, so a project
//     named in two cases on a volume that ignores case is one.
//
// Each is given home as ~. Each path and each slug is decoded once, against
// the filesystem as it is the first time it's asked of, and kept for every
// read after, so a view of a thousand scratchpads looks at the filesystem
// only as first built, and a directory gone since leaves the project it
// named. Each folder is listed once for its case, and looked in once for the
// scratch root. The zero Projects reads as ProjectOf alone.
type Projects struct {
	read *projectsRead
}

// projectsRead is what Projects reads against, and what it has read once:
// the project of each path, by the path; of each scratchpad's slug, by the
// slug; each whole path in the case its folders have, by the path; what's
// in each folder, by the folder, for its case alone; and whether each folder
// holds Claude Code's scratch root, by the folder.
type projectsRead struct {
	ProjectsConfig
	temporary           []string
	paths, slugs, cases readOnce[string]
	listed              readOnce[[]fs.DirEntry]
	holds               readOnce[bool]
}

// readOnce is what each key read reads, by the key.
type readOnce[V any] struct {
	mu   sync.Mutex
	read map[string]V
}

// NewProjects returns the Projects reading against c.
func NewProjects(c ProjectsConfig) Projects {
	c.Home = strings.TrimSuffix(cleaned(c.Home), "/")
	r := &projectsRead{
		ProjectsConfig: c, temporary: temporaryRoots,
		paths: readOnce[string]{read: make(map[string]string)}, slugs: readOnce[string]{read: make(map[string]string)},
		cases: readOnce[string]{read: make(map[string]string)}, listed: readOnce[[]fs.DirEntry]{read: make(map[string][]fs.DirEntry)},
		holds: readOnce[bool]{read: make(map[string]bool)},
	}
	if dir := strings.TrimSuffix(cleaned(c.TempDir), "/"); dir != "" && !r.homeOrAbove(dir) {
		r.temporary = append([]string{dir}, temporaryRoots...)
	}
	return Projects{read: r}
}

// Of is the project of the project and directory given, as Projects reads
// them: "" where they name neither.
func (p Projects) Of(project, dir string) string {
	at := ProjectOf(project, dir)
	if p.read == nil || at == "" {
		return at
	}
	return p.read.paths.of(at, p.read.project)
}

// of is what read reads of key, read once.
func (o *readOnce[V]) of(key string, read func(string) V) V {
	o.mu.Lock()
	got, ok := o.read[key]
	o.mu.Unlock()
	if ok {
		return got
	}
	got = read(key)
	o.mu.Lock()
	o.read[key] = got
	o.mu.Unlock()
	return got
}

// project is the project of the path at, as ProjectOf gives it, as Projects
// reads it.
func (r *projectsRead) project(at string) string {
	if slug, ok := claude.ScratchpadSlug(at); ok {
		return r.slugs.of(slug, r.slugged)
	}
	return r.settled(at)
}

// slugged is the project of the directory the scratchpad's slug names, as
// Projects reads it.
func (r *projectsRead) slugged(slug string) string {
	named := claude.SlugDir(r.Root, slug)
	if named.Dir == "" || r.aboveHome(named.Dir) {
		return TemporaryFolders
	}
	if named.Whole {
		return r.settled(r.shown(r.find(named.Dir)))
	}
	project, ok := r.repository(named.Dir)
	if !ok {
		return TemporaryFolders
	}
	return r.settled(r.shown(cleaned(project)))
}

// settled is the project at, in the case its folders have: TemporaryFolders
// where it's a scratchpad or a temporary folder.
func (r *projectsRead) settled(at string) string {
	whole := r.whole(at)
	if _, ok := claude.ScratchpadSlug(at); ok || r.isTemporary(whole) {
		return TemporaryFolders
	}
	if !path.IsAbs(whole) {
		return at
	}
	return r.shown(r.cased(whole))
}

// find is the project of the directory dir, by its whole path, as Find has
// it, else as ProjectOf reads it.
func (r *projectsRead) find(dir string) string {
	if r.Find == nil {
		return ProjectOf("", dir)
	}
	return cleaned(r.Find(dir))
}

// repository is the project of the repository the directory dir, a whole
// path, is in, and whether it's in one, as Repository has them: none where
// there's none to ask.
func (r *projectsRead) repository(dir string) (string, bool) {
	if r.Repository == nil {
		return "", false
	}
	return r.Repository(dir)
}

// aboveHome reports whether dir, a whole path, holds the home directory.
func (r *projectsRead) aboveHome(dir string) bool {
	return r.Home != "" && dir != r.Home && within(r.Home, dir)
}

// homeOrAbove reports whether dir, a whole path, is the root, the home
// directory or a folder holding it, their case folded.
func (r *projectsRead) homeOrAbove(dir string) bool {
	return dir == "/" || r.Home != "" && withinFolded(r.Home, dir)
}

// isTemporary reports whether the whole path at is a temporary folder or in
// one, its case folded, and none else: a temporary root, or TempDir, where
// it isn't the home or above it; in a folder named for Claude Code's
// scratch root, whatever holds it; or in a folder holding the scratch root,
// where that isn't the home or above it, as Claude Code keeps its own
// temporary files there, looked for in the case its folders have. The home
// and the folders above it are never one.
func (r *projectsRead) isTemporary(at string) bool {
	for _, root := range r.temporary {
		if withinFolded(at, root) {
			return true
		}
	}
	if !path.IsAbs(at) {
		return false
	}
	scratchRoot := claude.ScratchRoot(r.UID)
	for dir := at; dir != "/"; dir = path.Dir(dir) {
		if strings.EqualFold(path.Base(dir), scratchRoot) {
			return true
		}
	}
	for dir := r.cased(at); !r.homeOrAbove(dir); dir = path.Dir(dir) {
		if r.holds.of(dir, r.holdsScratchRoot) {
			return true
		}
	}
	return false
}

// holdsScratchRoot reports whether the folder dir, a whole path, holds
// Claude Code's scratch root.
func (r *projectsRead) holdsScratchRoot(dir string) bool {
	info, err := fs.Stat(r.Root, rooted(path.Join(dir, claude.ScratchRoot(r.UID))))
	return err == nil && info.IsDir()
}

// cased is the whole path at, but the root, in the case its folders have:
// each folder named as its parent names it, as it's cased, else with its
// case folded, the first so named; as it's given from the first that's not
// there.
func (r *projectsRead) cased(at string) string {
	return r.cases.of(at, func(at string) string {
		parent, name := path.Dir(at), path.Base(at)
		if parent != "/" {
			parent = r.cased(parent)
		}
		folded := ""
		for _, e := range r.listing(parent) {
			switch there := e.Name(); {
			case there == name:
				return path.Join(parent, name)
			case folded == "" && strings.EqualFold(there, name):
				folded = there
			}
		}
		return path.Join(parent, cmp.Or(folded, name))
	})
}

// listing is what's in the folder dir, a whole path, in order, read once:
// nothing where it can't be read.
func (r *projectsRead) listing(dir string) []fs.DirEntry {
	return r.listed.of(dir, func(dir string) []fs.DirEntry {
		entries, _ := fs.ReadDir(r.Root, rooted(dir))
		return entries
	})
}

// rooted is the whole path p as a path of the filesystem from its root.
func rooted(p string) string {
	if p == "/" {
		return "."
	}
	return strings.TrimPrefix(p, "/")
}

// whole is the path p, ~ given as the home directory.
func (r *projectsRead) whole(p string) string {
	if rest, ok := strings.CutPrefix(p, "~"); ok && r.Home != "" && (rest == "" || rest[0] == '/') {
		return r.Home + rest
	}
	return p
}

// shown is the whole path p, the home directory given as ~, as a line
// gives a project.
func (r *projectsRead) shown(p string) string {
	if rest, ok := strings.CutPrefix(p, r.Home); r.Home != "" && ok && (rest == "" || rest[0] == '/') {
		return "~" + rest
	}
	return p
}

// within reports whether the cleaned path p is dir or in it.
func within(p, dir string) bool {
	rest, ok := strings.CutPrefix(p, dir)
	return ok && (rest == "" || rest[0] == '/')
}

// withinFolded reports whether the cleaned path p is dir or in it, their
// case folded.
func withinFolded(p, dir string) bool {
	return len(p) >= len(dir) && strings.EqualFold(p[:len(dir)], dir) && (len(p) == len(dir) || p[len(dir)] == '/')
}

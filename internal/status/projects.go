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
}

// Projects reads the project a line, a session or the router names, as
// ProjectOf does, and then against the filesystem:
//
//   - a path in a Claude Code scratchpad, as claude.ScratchpadSlug finds
//     one, counts as the project of the directory its slug names, as
//     claude.SlugDir finds it, by its path as Find has it; but as
//     TemporaryFolders where the slug names nothing below the home's
//     parents, or names only the start of itself and no repository holds
//     what it names, as of a repository or a volume gone;
//   - a path in a temporary folder counts as TemporaryFolders: under /tmp,
//     /private/tmp, /var/folders, /private/var/folders or TempDir, or under a
//     folder holding Claude Code's scratch root, but the home or above it;
//   - any other path is given in the case its folders have, so a project
//     named in two cases on a volume that ignores case is one.
//
// Each is given home as ~. Each path, each slug and each folder is read
// once, the first time it's asked of, and kept for every read after, so a
// view of a thousand scratchpads looks at the filesystem only as first
// built, and a directory gone since leaves the project it named. The zero
// Projects reads as ProjectOf alone.
type Projects struct {
	read *projectsRead
}

// projectsRead is what Projects reads against; the project of each path it
// has read, by the path, and of each scratchpad's slug, by the slug; and
// each whole path it has cased, in the case its folders have, by the path.
type projectsRead struct {
	ProjectsConfig
	temporary           []string
	paths, slugs, cases readOnce
}

// readOnce is what each key read reads, by the key.
type readOnce struct {
	mu   sync.Mutex
	read map[string]string
}

// NewProjects returns the Projects reading against c.
func NewProjects(c ProjectsConfig) Projects {
	c.Home = strings.TrimSuffix(cleaned(c.Home), "/")
	temporary := temporaryRoots
	if dir := strings.TrimSuffix(cleaned(c.TempDir), "/"); dir != "" {
		temporary = append([]string{dir}, temporaryRoots...)
	}
	return Projects{read: &projectsRead{
		ProjectsConfig: c, temporary: temporary,
		paths: readOnce{read: make(map[string]string)}, slugs: readOnce{read: make(map[string]string)}, cases: readOnce{read: make(map[string]string)},
	}}
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
func (o *readOnce) of(key string, read func(string) string) string {
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
	if named.Dir == "" || r.aboveHome(named.Dir) || !named.Whole && !r.inRepository(named.Dir) {
		return TemporaryFolders
	}
	return r.settled(r.shown(r.find(named.Dir)))
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

// aboveHome reports whether dir, a whole path, holds the home directory.
func (r *projectsRead) aboveHome(dir string) bool {
	return r.Home != "" && dir != r.Home && within(r.Home, dir)
}

// homeOrAbove reports whether dir, a whole path, is the root, the home
// directory or a folder holding it.
func (r *projectsRead) homeOrAbove(dir string) bool {
	return dir == "/" || r.Home != "" && within(r.Home, dir)
}

// inRepository reports whether a .git is in the directory dir, a whole
// path, or a folder above it, but the root.
func (r *projectsRead) inRepository(dir string) bool {
	for at := dir; at != "/"; at = path.Dir(at) {
		if r.exists(path.Join(at, ".git")) {
			return true
		}
	}
	return false
}

// isTemporary reports whether the whole path at is a temporary folder or in
// one: a temporary root, or a folder holding Claude Code's scratch root,
// as its own temporary files are kept there, but the home or above it.
func (r *projectsRead) isTemporary(at string) bool {
	for _, root := range r.temporary {
		if within(at, root) {
			return true
		}
	}
	if !path.IsAbs(at) {
		return false
	}
	for dir := at; !r.homeOrAbove(dir); dir = path.Dir(dir) {
		if r.isDir(path.Join(dir, claude.ScratchRoot(r.UID))) {
			return true
		}
	}
	return false
}

// cased is the whole path at, but the root, in the case its folders have:
// each folder named as its parent names it, as it's cased, else with its
// case folded; as it's given from the first that's not there.
func (r *projectsRead) cased(at string) string {
	return r.cases.of(at, func(at string) string {
		parent, name := path.Dir(at), path.Base(at)
		if parent != "/" {
			parent = r.cased(parent)
		}
		entries, err := fs.ReadDir(r.Root, rooted(parent))
		if err != nil {
			return path.Join(parent, name)
		}
		folded := ""
		for _, e := range entries {
			switch {
			case e.Name() == name:
				return path.Join(parent, name)
			case folded == "" && strings.EqualFold(e.Name(), name):
				folded = e.Name()
			}
		}
		return path.Join(parent, cmp.Or(folded, name))
	})
}

// exists reports whether anything is at the whole path p.
func (r *projectsRead) exists(p string) bool {
	_, err := fs.Stat(r.Root, rooted(p))
	return err == nil
}

// isDir reports whether a directory is at the whole path p.
func (r *projectsRead) isDir(p string) bool {
	info, err := fs.Stat(r.Root, rooted(p))
	return err == nil && info.IsDir()
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

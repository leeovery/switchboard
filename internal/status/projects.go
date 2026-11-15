package status

import (
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/leeovery/switchboard/internal/claude"
)

// TemporaryFolders is the project, as Projects reads it, of the sessions that
// ran in a temporary folder, together, which no project of a path can be.
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
// ProjectOf does, and then against the filesystem: a path in a Claude Code
// scratchpad, as claude.ScratchpadSlug finds one, counts as the project of
// the directory its slug names, as claude.SlugDir finds it, by its path as
// Find has it, home as ~; and one in a temporary folder, or a scratchpad
// whose slug names none but the home's parents, as TemporaryFolders, a
// scratchpad's project among them. Each path, and each slug, is read once,
// the first time it's asked of, and kept for every read after, so a view
// of a thousand scratchpads looks at the filesystem only as first built,
// and a directory gone since leaves the project it named. The zero Projects
// reads as ProjectOf alone.
type Projects struct {
	read *projectsRead
}

// projectsRead is what Projects reads against, and the project of each path
// it has read, by the path, and of each scratchpad's slug, by the slug.
type projectsRead struct {
	ProjectsConfig
	temporary    []string
	paths, slugs readOnce
}

// readOnce is the project of each key read, by the key.
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
		ProjectsConfig: c, temporary: temporary, paths: readOnce{read: make(map[string]string)}, slugs: readOnce{read: make(map[string]string)},
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

// of is the project of key, as read reads it, read once.
func (o *readOnce) of(key string, read func(string) string) string {
	o.mu.Lock()
	project, ok := o.read[key]
	o.mu.Unlock()
	if ok {
		return project
	}
	project = read(key)
	o.mu.Lock()
	o.read[key] = project
	o.mu.Unlock()
	return project
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
	dir, ok := claude.SlugDir(r.Root, slug)
	if !ok || r.aboveHome(dir) {
		return TemporaryFolders
	}
	return r.settled(r.shown(r.find(dir)))
}

// settled is the project at, TemporaryFolders where it's a scratchpad or a
// temporary folder.
func (r *projectsRead) settled(at string) string {
	if _, ok := claude.ScratchpadSlug(at); ok || r.isTemporary(r.whole(at)) {
		return TemporaryFolders
	}
	return at
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

// isTemporary reports whether the whole path at is a temporary folder or in
// one, or a folder beside Claude Code's scratch root, as its own temporary
// files are kept.
func (r *projectsRead) isTemporary(at string) bool {
	for _, root := range r.temporary {
		if within(at, root) {
			return true
		}
	}
	if !path.IsAbs(at) || at == "/" {
		return false
	}
	held := path.Join(path.Dir(at), claude.ScratchRoot(r.UID))
	info, err := fs.Stat(r.Root, strings.TrimPrefix(held, "/"))
	return err == nil && info.IsDir()
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

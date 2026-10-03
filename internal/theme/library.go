package theme

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/leeovery/switchboard/internal/logs"
)

// logger notes the themes that don't load, and why.
var logger = logs.For("theme")

// Library is where the dashboard's themes are: built in, and the .theme files
// in a themes directory, its top level alone, links followed, read afresh
// each time, so a theme edited shows without a restart.
type Library struct {
	// dir is the themes directory: "" for none.
	dir string
	// told notes each problem logged, so a picker opened again and again
	// tells of a broken theme once.
	mu   sync.Mutex
	told map[string]bool
}

// NewLibrary is the library of the built-ins and the themes directory dir,
// "" for none.
func NewLibrary(dir string) *Library {
	return &Library{dir: dir, told: make(map[string]bool)}
}

// Load loads the theme slug names: the built-in, else its file in the themes
// directory, found among them as List finds it, by its name exactly, however
// the filesystem matches names.
func (l *Library) Load(slug string) (Theme, error) {
	if !ValidSlug(slug) {
		return Theme{}, &Problem{Reason: badName, Detail: fmt.Sprintf("%q can't name a theme", slug)}
	}
	if t, ok := Builtin(slug); ok {
		return t, nil
	}
	if l.dir == "" {
		return Theme{}, &Problem{Reason: notFound}
	}
	files, err := l.files()
	if err != nil {
		return Theme{}, &Problem{Reason: unreadable, Detail: err.Error(), Err: err}
	}
	name := slug + fileExt
	i := slices.IndexFunc(files, func(path string) bool { return filepath.Base(path) == name })
	if i < 0 {
		return Theme{}, &Problem{Reason: notFound, Detail: "no " + name + " in " + l.dir}
	}
	t, p := readFile(files[i], slug)
	if p != nil {
		return Theme{}, p
	}
	return t, nil
}

// Pair loads the pair of themes choice draws in, a theme that doesn't load,
// which the log tells of, giving way to its half's default.
func (l *Library) Pair(c Choice) Pair {
	return c.pair(func(slug string) (Theme, bool) {
		t, err := l.Load(slug)
		if err != nil {
			l.tell("a chosen theme doesn't load, so its default stands in", "theme", slug, "error", err)
			return Theme{}, false
		}
		return t, true
	})
}

// Listing is what the theme picker lists: every theme there is, whether it
// loads or not, and, where the themes directory couldn't be read, why.
type Listing struct {
	Entries []Entry
	// Problem is why the themes directory couldn't be read, if it couldn't.
	Problem error
}

// Entry is a theme the picker lists: built in, or a file in the themes
// directory, which may not load.
type Entry struct {
	// Name is what the picker calls it: its slug, or for a file whose name
	// gives none, or a built-in's, the file's name.
	Name string
	// Slug is the slug it's chosen by: "" for a file whose name gives none,
	// or a built-in's.
	Slug  string
	Theme Theme
	// Problem says why it doesn't load: nil for one that does.
	Problem *Problem
}

// List lists every theme there is to pick from, in order of their names: the
// built-ins, and every file in the themes directory named .theme, in any
// case, whether it loads or not, as a broken file is named rather than left
// out, the log telling why, once. A file taking a built-in's slug follows
// the built-in.
func (l *Library) List() Listing {
	var listing Listing
	for _, t := range Builtins() {
		listing.Entries = append(listing.Entries, Entry{Name: t.Slug, Slug: t.Slug, Theme: t})
	}
	files, err := l.files()
	if err != nil {
		listing.Problem = err
		l.tell("can't read the themes directory", "dir", l.dir, "error", err)
	}
	for _, path := range files {
		e := l.entry(path)
		if e.Problem != nil {
			l.tell("a theme doesn't load", "path", path, "error", e.Problem)
		}
		listing.Entries = append(listing.Entries, e)
	}
	sortEntries(listing.Entries)
	return listing
}

// files are the paths of the candidates in the themes directory: every
// entry at its top level whose name ends .theme, in any case, but one whose
// name starts with a dot, as an editor's lock file's and macOS's own files'
// do, such as .#lake.theme and ._lake.theme; and that's a regular file,
// links followed, as reading anything else, such as a FIFO or a device, may
// never end. A link that leads nowhere is one, as it's named rather than left
// out. A directory that isn't there holds none.
func (l *Library) files() ([]string, error) {
	if l.dir == "" {
		return nil, nil
	}
	dirEntries, err := os.ReadDir(l.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var paths []string
	for _, d := range dirEntries {
		name := d.Name()
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), fileExt) {
			continue
		}
		path := filepath.Join(l.dir, name)
		if info, err := os.Stat(path); err == nil && !info.Mode().IsRegular() {
			continue
		}
		paths = append(paths, path)
	}
	return paths, err
}

// entry is the file at path as the picker lists it: named by the slug its
// name gives, and loaded, unless its name gives none, or a built-in's.
func (l *Library) entry(path string) Entry {
	name := filepath.Base(path)
	slug, ok := strings.CutSuffix(name, fileExt)
	switch {
	case !ok || !ValidSlug(slug):
		return Entry{Name: name, Problem: &Problem{Reason: badName, Detail: "a theme file is named <slug>.theme, its slug lower-case letters, digits and hyphens"}}
	case isBuiltin(slug):
		return reserved(slug)
	}
	t, p := readFile(path, slug)
	return Entry{Name: slug, Slug: slug, Theme: t, Problem: p}
}

// FileEntry is a theme read from a file, as the picker lists it, as List
// lists one in the themes directory: under its slug, unless that's a
// built-in's, which no file may take.
func FileEntry(t Theme) Entry {
	if isBuiltin(t.Slug) {
		return reserved(t.Slug)
	}
	return Entry{Name: t.Slug, Slug: t.Slug, Theme: t}
}

// reserved is the file of a theme taking the built-in slug names, as the
// picker lists it: by its file's name, never to be picked.
func reserved(slug string) Entry {
	return Entry{Name: slug + fileExt, Problem: &Problem{Reason: reservedName, Detail: slug + " is a built-in theme's"}}
}

// Pair is the pair of themes choice draws in, as the listing has them: a
// theme it doesn't list, or lists as not loading, gives way to its half's
// default.
func (l Listing) Pair(c Choice) Pair {
	return c.pair(func(slug string) (Theme, bool) {
		i := slices.IndexFunc(l.Entries, func(e Entry) bool { return e.Slug == slug && e.Problem == nil })
		if i < 0 {
			return Theme{}, false
		}
		return l.Entries[i].Theme, true
	})
}

// Rows are what the picker lists for a choice: the listing's themes, and
// each theme the choice names that the listing doesn't, which can't be
// picked, but bears the badge that says where the choice stands, in order of
// their names.
func (l Listing) Rows(c Choice) []Entry {
	rows := slices.Clone(l.Entries)
	for _, slug := range c.Named() {
		if slices.ContainsFunc(rows, func(e Entry) bool { return e.Slug == slug }) {
			continue
		}
		missing := Entry{Name: slug, Slug: slug, Problem: &Problem{Reason: notFound}}
		switch {
		case !ValidSlug(slug):
			missing.Problem = &Problem{Reason: badName}
		case l.Problem != nil:
			missing.Problem = &Problem{Reason: unreadable}
		}
		rows = append(rows, missing)
	}
	sortEntries(rows)
	return rows
}

// sortEntries puts entries in order of their names, in any case first, then
// as they're written, a built-in before a file taking its name.
func sortEntries(entries []Entry) {
	slices.SortStableFunc(entries, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.Name, b.Name))
	})
}

// tell logs a problem, once.
func (l *Library) tell(msg string, args ...any) {
	key := fmt.Sprint(append([]any{msg}, args...)...)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.told[key] {
		return
	}
	l.told[key] = true
	logger.Warn(msg, args...)
}

// isBuiltin reports whether slug names a built-in, which no file may take.
func isBuiltin(slug string) bool {
	_, ok := builtins[slug]
	return ok
}

// ReadFile reads the theme in the file at path, whatever it's named: a theme
// given as a file, rather than found in the themes directory. Its slug is its
// file's name, less .theme.
func ReadFile(path string) (Theme, error) {
	t, p := readFile(path, strings.TrimSuffix(filepath.Base(path), fileExt))
	if p != nil {
		return Theme{}, p
	}
	return t, nil
}

// readFile reads the theme in the file at path, under the slug given.
func readFile(path, slug string) (Theme, *Problem) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, &Problem{Reason: unreadable, Detail: err.Error(), Err: err}
	}
	return parse(slug, data)
}

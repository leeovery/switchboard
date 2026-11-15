package claude

import (
	"cmp"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/leeovery/switchboard/internal/prose"
)

// scratchRootPrefix begins the name of the folder Claude Code keeps a user's
// scratchpads in, within its temporary directory: claude-<uid>.
const scratchRootPrefix = "claude-"

// scratchpadName is the name of a session's scratchpad, within the folder
// Claude Code keeps of the session: <root>/<slug>/<session id>/scratchpad.
const scratchpadName = "scratchpad"

// ScratchRoot is the name of the folder Claude Code keeps the scratchpads of
// the user with the given id in, within its temporary directory, as
// "claude-501".
func ScratchRoot(uid int) string {
	return scratchRootPrefix + strconv.Itoa(uid)
}

// Slug is the name Claude Code gives a folder of its own of the directory
// dir a session runs in, as its folders in ~/.claude/projects and its
// scratchpads' are named: dir with every character but an ASCII letter or
// digit as "-", as "-Users-you-Code-api--claude-worktrees-fix" of
// "/Users/you/Code/api/.claude/worktrees/fix". Claude Code replaces each
// UTF-16 unit, so a character beyond the Basic Multilingual Plane is "--".
func Slug(dir string) string {
	var slug strings.Builder
	slug.Grow(len(dir))
	for _, r := range dir {
		switch {
		case isAlphanumeric(r):
			slug.WriteRune(r)
		case utf16.RuneLen(r) == 2:
			slug.WriteString("--")
		default:
			slug.WriteByte('-')
		}
	}
	return slug.String()
}

// isAlphanumeric reports whether r is an ASCII letter or digit.
func isAlphanumeric(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || isDigit(r)
}

// isDigit reports whether r is an ASCII digit.
func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

// ScratchpadSlug returns the slug, as Slug gives it, of the directory of the
// session whose scratchpad the path p lies in, and whether it lies in one:
// <root>/claude-<uid>/<slug>/<session id>/scratchpad, or a folder in it,
// whatever the root; or such a path cut from its front, as a line cuts a
// long one, its claude-<uid> cut short, or away, after a leading ellipsis.
// Where that session was started in another's scratchpad, as its slug
// says, the slug is the other's directory's, as many times over as they
// were, so no scratchpad, gone since, need be there to read.
func ScratchpadSlug(p string) (string, bool) {
	parts := strings.Split(p, "/")
	for i := 3; i < len(parts); i++ {
		if parts[i] == scratchpadName && sessionID.MatchString(parts[i-1]) && parts[i-2] != "" && isScratchRootAt(parts, i-3) {
			return outermost(parts[i-2]), true
		}
	}
	return "", false
}

// sessionIDShape is the shape of a session's id, a UUID, as
// "0b5c6f2e-6a1d-4c3e-9f7a-2b8d4e6f1a3c".
const sessionIDShape = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

var (
	// sessionID is a session's id, whole.
	sessionID = regexp.MustCompile(`^` + sessionIDShape + `$`)
	// scratchpadSlug is the slug of a directory in a scratchpad, the slug of
	// the directory of the session it's for between its root's and its own.
	scratchpadSlug = regexp.MustCompile(`^.*?` + scratchRootPrefix + `[0-9]+-(-.*)-` + sessionIDShape + `-` + scratchpadName + `(?:-.*)?$`)
)

// outermost is slug, or where it's of a directory in a scratchpad, the slug
// of the directory of the session that's for, and so on.
func outermost(slug string) string {
	for {
		in := scratchpadSlug.FindStringSubmatch(slug)
		if in == nil {
			return slug
		}
		slug = in[1]
	}
}

// isScratchRootAt reports whether the part of a path at i is the folder
// Claude Code keeps scratchpads in: claude-<uid> after another part; or, the
// first part, an ellipsis then what a cut left of claude-<uid>, if anything.
func isScratchRootAt(parts []string, i int) bool {
	if i > 0 {
		uid, ok := strings.CutPrefix(parts[i], scratchRootPrefix)
		return ok && uid != "" && strings.TrimFunc(uid, isDigit) == ""
	}
	left, ok := strings.CutPrefix(parts[0], prose.Ellipsis)
	if !ok || left == "" {
		return ok
	}
	named := strings.TrimRightFunc(left, isDigit)
	return named != left && strings.HasSuffix(scratchRootPrefix, named)
}

// SlugDir returns the directory the slug, as Slug gives it, names among
// those of fsys, a filesystem from the root, by its path from the root, as
// "/Users/you/Code/api": the deepest whose slug is slug, or begins it up to
// a "-", so a directory since gone names the one that held it; and whether
// any but the root does. Links aren't followed.
func SlugDir(fsys fs.FS, slug string) (string, bool) {
	var found named
	for walk := []string{"."}; len(walk) > 0; {
		dir := walk[len(walk)-1]
		walk = walk[:len(walk)-1]
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			at := named{dir: "/" + path.Join(dir, e.Name())}
			if own := Slug(at.dir); own == slug || strings.HasPrefix(slug, own+"-") {
				at.slugged = len(own)
				found = deeper(found, at)
				walk = append(walk, at.dir[1:])
			}
		}
	}
	return found.dir, found.dir != ""
}

// named is a directory a slug names, by its path from the root, and how
// much of the slug its own slug is: none where dir is "".
type named struct {
	dir     string
	slugged int
}

// deeper is whichever of found and at names more of the slug: else the one
// with more parts to its path, else the one whose path sorts first.
func deeper(found, at named) named {
	if found.dir == "" {
		return at
	}
	more := cmp.Or(cmp.Compare(at.slugged, found.slugged), cmp.Compare(strings.Count(at.dir, "/"), strings.Count(found.dir, "/")),
		strings.Compare(found.dir, at.dir))
	if more > 0 {
		return at
	}
	return found
}

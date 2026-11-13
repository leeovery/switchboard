package status

import (
	"path"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/prose"
)

// ProjectOf is the project a session or a request ran in, by its path,
// cleaned, so a path written two ways is one project, given the project and
// directory its line, or the router, names: project, the repository its
// directory is in, as run found it; else, from before run told one, its
// directory's, dir: a worktree Claude Code made,
// <repo>/.claude/worktrees/<name>, the repository's, <repo>, and any other
// directory its own. "" where it names neither.
func ProjectOf(project, dir string) string {
	if project != "" {
		return cleaned(project)
	}
	dir = cleaned(dir)
	if repo, ok := worktreeRepository(dir); ok {
		return repo
	}
	return dir
}

// cleaned is the path p, cleaned as path.Clean cleans it: "" for "".
func cleaned(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(p)
}

// worktreeRepository returns the repository of dir, a cleaned path, where
// it's a worktree Claude Code made, as claude.WorktreesDir has it, and
// whether it is.
func worktreeRepository(dir string) (string, bool) {
	parent, name := path.Split(dir)
	if name == "" {
		return "", false
	}
	repo, ok := strings.CutSuffix(strings.TrimSuffix(parent, "/"), "/"+claude.WorktreesDir)
	return repo, ok && repo != ""
}

// ProjectNames names projects shown together, each by its path as ProjectOf
// gives it, cleaned, the names keyed so: by its last part, as
// "switchboard", and where two share that, by as many of its parents' too
// as tell them apart, as "client/api" and "own/api", or "a/x/api" and
// "b/x/api"; each cut with an ellipsis to the cells given, keeping its
// start, as DirName cuts, and cleaned, as what comes from elsewhere is. None
// names "".
func ProjectNames(projects []string, cells int) map[string]string {
	parts := make(map[string]int, len(projects))
	for _, p := range projects {
		if p = cleaned(p); p != "" {
			parts[p] = 1
		}
	}
	setApart(parts)
	names := make(map[string]string, len(parts))
	for p, n := range parts {
		names[p] = prose.Cut(shownName(p, n), cells)
	}
	return names
}

// setApart gives each project, by its path, as many of its path's last
// parts, as parts holds them, from one, as tell its name from every other's:
// round by round, one more to each sharing its name with another, that has
// one more to give.
func setApart(parts map[string]int) {
	for grown := true; grown; {
		grown = false
		for _, shared := range sharing(parts) {
			for _, p := range shared {
				if _, whole := lastParts(p, parts[p]); len(shared) > 1 && !whole {
					parts[p]++
					grown = true
				}
			}
		}
	}
}

// sharing are the projects, by their paths, by the name each shows as, as
// shownName gives it of as many of its path's last parts as parts holds.
func sharing(parts map[string]int) map[string][]string {
	byName := make(map[string][]string, len(parts))
	for p, n := range parts {
		name := shownName(p, n)
		byName[name] = append(byName[name], p)
	}
	return byName
}

// shownName is the name the last n parts of the cleaned path p show as,
// cleaned, as what comes from elsewhere is, so two that show alike share it.
func shownName(p string, n int) string {
	name, _ := lastParts(p, n)
	return prose.Clean(name)
}

// lastParts returns the last n parts of the cleaned path p, as "x/api" are
// the last two of "~/a/x/api", and whether they're the whole of it: p, where
// it has no more, as "/api" are the last two of "/api".
func lastParts(p string, n int) (string, bool) {
	at := len(p)
	for range n {
		if at = strings.LastIndexByte(p[:at], '/'); at < 0 {
			return p, true
		}
	}
	if name := p[at+1:]; name != "" {
		return name, false
	}
	return p, true
}

package status

import (
	"path"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/prose"
)

// ProjectOf is the project a session or a request ran in, by its path, given
// the project and directory its line, or the router, names: project, the
// repository its directory is in, as run found it; else, from before run
// told one, its directory's, dir: a worktree Claude Code made,
// <repo>/.claude/worktrees/<name>, the repository's, <repo>, and any other
// directory its own. "" where it names neither.
func ProjectOf(project, dir string) string {
	if project != "" {
		return project
	}
	if repo, ok := worktreeRepository(dir); ok {
		return repo
	}
	return dir
}

// worktreeRepository returns the repository of dir, where it's a worktree
// Claude Code made, as claude.WorktreesDir has it, and whether it is.
func worktreeRepository(dir string) (string, bool) {
	parent, name := path.Split(dir)
	if name == "" {
		return "", false
	}
	repo, ok := strings.CutSuffix(strings.TrimSuffix(parent, "/"), "/"+claude.WorktreesDir)
	return repo, ok && repo != ""
}

// ProjectNames names projects shown together, each by its path as ProjectOf
// gives it, cleaned: by its last part, as "switchboard", and where two share
// that, each by its parent's last part too, as "client/api" and "own/api";
// each cut with an ellipsis to the cells given, keeping its start, as DirName
// cuts. None names "".
func ProjectNames(projects []string, cells int) map[string]string {
	shared := make(map[string]int, len(projects))
	names := make(map[string]string, len(projects))
	for _, p := range projects {
		if _, named := names[p]; p == "" || named {
			continue
		}
		names[p] = path.Base(prose.Clean(p))
		shared[names[p]]++
	}
	for p, name := range names {
		if shared[name] > 1 {
			name = path.Join(path.Base(path.Dir(prose.Clean(p))), name)
		}
		names[p] = prose.Cut(name, cells)
	}
	return names
}

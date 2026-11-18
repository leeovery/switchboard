package views

import (
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// ShownName is what names a session on Sessions and Projects, as ShownName
// gives it: Name, cleaned, as what came from elsewhere is; and Worktree, set
// where it's the name of the worktree the session runs in, which the
// dashboard marks.
type ShownName struct {
	Name     string
	Worktree bool
}

// ShownName names the session, as Sessions' List and a project's SESSIONS
// name one: by the name of the terminal session it was started in; else,
// where it runs in a worktree Claude Code made, as status.WorktreeName finds
// it, by the worktree's name; else by its id, as status.ShortID cuts it. It
// never names a session by its directory: that's the project's, which the
// List shows beside it, and a project's SESSIONS above it.
func (s ListedSession) ShownName() ShownName {
	if name := prose.Clean(s.Name); name != "" {
		return ShownName{Name: name}
	}
	if worktree, ok := status.WorktreeName(s.Dir); ok && prose.Clean(worktree) != "" {
		return ShownName{Name: prose.Clean(worktree), Worktree: true}
	}
	return ShownName{Name: status.ShortID(prose.Clean(s.Session))}
}

// place is where the session runs, as the List has it.
func (s ListedSession) place() sessionPlace {
	return sessionPlace{dir: s.Dir, project: s.Project, name: s.Name}
}

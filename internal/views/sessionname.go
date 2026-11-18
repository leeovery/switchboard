package views

import (
	"math"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// ShownName is what names a session on Sessions and Projects, as ShownName
// gives it: Name, cleaned, as what came from elsewhere is; Worktree, set
// where it's the name of the worktree the session runs in, which the
// dashboard marks; and ID, the session's id as status.ShownID gives it,
// where another row of its list shows the same project and name, which the
// dashboard sets after the name to tell the two apart: "" where none does.
type ShownName struct {
	Name     string
	Worktree bool
	ID       string
}

// ShownName names the session, as Sessions' List and a project's SESSIONS
// name one, as shownName has it.
func (s ListedSession) ShownName() ShownName {
	shown, _ := s.shownName()
	return shown
}

// shownName names the session: by the name of the terminal session it was
// started in; else, where it runs in a worktree Claude Code made, as
// status.WorktreeName finds it, by the worktree's name; else by its id, as
// status.ShownID gives it, as the rest of the dashboard names a session,
// byID saying so. It never names a session by its directory: that's the
// project's, which the List shows beside it, and a project's SESSIONS above
// it.
func (s ListedSession) shownName() (shown ShownName, byID bool) {
	if name := prose.Clean(s.Name); name != "" {
		return ShownName{Name: name}, false
	}
	if worktree, ok := status.WorktreeName(s.Dir); ok && prose.Clean(worktree) != "" {
		return ShownName{Name: prose.Clean(worktree), Worktree: true}, false
	}
	return ShownName{Name: status.ShownID(s.Session)}, true
}

// ShownName names the session as its ListedSession's ShownName does, with
// its id where Twinned says another row of its list shows alike.
func (s PeriodSession) ShownName() ShownName {
	shown := s.ListedSession.ShownName()
	if s.Twinned {
		shown.ID = status.ShownID(s.Session)
	}
	return shown
}

// twinsApart marks as Twinned each of rows that shows the same project and
// name as another of them, as its ProjectName and ShownName show them, so
// each shows its id too: never one named by its id, which an id beside it
// wouldn't tell apart.
func twinsApart(rows []PeriodSession) {
	type shown struct{ project, name string }
	keys := make([]shown, len(rows))
	named := make([]bool, len(rows))
	count := make(map[shown]int, len(rows))
	for i, r := range rows {
		name, byID := r.shownName()
		keys[i], named[i] = shown{r.ProjectName, name.Name}, !byID
		if named[i] {
			count[keys[i]]++
		}
	}
	for i := range rows {
		rows[i].Twinned = named[i] && count[keys[i]] > 1
	}
}

// projectNamedAlone names the project with the given path as Projects names
// one shown alone, uncut, as status.ProjectNames has it: none of temporary
// folders, nor of "".
func projectNamedAlone(project string) string {
	if project == status.TemporaryFolders {
		return ""
	}
	for _, name := range status.ProjectNames([]string{project}, math.MaxInt) {
		return name
	}
	return ""
}

// place is where the session runs, as the List has it.
func (s ListedSession) place() sessionPlace {
	return sessionPlace{dir: s.Dir, project: s.Project, name: s.Name}
}

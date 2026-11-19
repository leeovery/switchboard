package views

import (
	"math"
	"slices"

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

// ShownName names the session as its ListedSession's ShownName does, its id
// as long as IDCells says: as its name where it's named by its id, and after
// its name where Twinned says another row of its list shows alike.
func (s PeriodSession) ShownName() ShownName {
	shown, byID := s.shownName()
	id := prose.Truncate(prose.Clean(s.Session), max(s.IDCells, status.IDShown))
	switch {
	case byID:
		shown.Name = id
	case s.Twinned:
		shown.ID = id
	}
	return shown
}

// toldApart tells apart the rows given that would show alike: those that
// show the same project and name as another, as their ProjectName and
// ShownName show them, each marked Twinned, to show its id after its name;
// and those of one project each named by its id. Each of either shows as
// much of its id as tells it from the rest of them, as idCellsApart has it.
func toldApart(rows []PeriodSession) {
	type alike struct {
		project, name string
		byID          bool
	}
	groups := make(map[alike][]int)
	for i, r := range rows {
		name, byID := r.shownName()
		key := alike{project: r.ProjectName, byID: byID}
		if !byID {
			key.name = name.Name
		}
		groups[key] = append(groups[key], i)
	}
	for key, at := range groups {
		if len(at) < 2 {
			continue
		}
		ids := make([]string, len(at))
		for j, i := range at {
			ids[j] = rows[i].Session
		}
		for j, cells := range idCellsApart(ids) {
			rows[at[j]].Twinned, rows[at[j]].IDCells = !key.byID, cells
		}
	}
}

// idCellsApart are how many characters of each of the ids given, cleaned,
// tell it from every other: one past the longest start it shares with
// another, status.IDShown at the least and the whole id at the most, as two
// ids alike throughout can't be told apart.
func idCellsApart(ids []string) []int {
	cleaned := make([][]rune, len(ids))
	for i, id := range ids {
		cleaned[i] = []rune(prose.Clean(id))
	}
	order := make([]int, len(ids))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return slices.Compare(cleaned[a], cleaned[b]) })
	cells := make([]int, len(ids))
	for k, i := range order {
		shared := 0
		if k > 0 {
			shared = sharedStart(cleaned[i], cleaned[order[k-1]])
		}
		if k+1 < len(order) {
			shared = max(shared, sharedStart(cleaned[i], cleaned[order[k+1]]))
		}
		cells[i] = min(len(cleaned[i]), max(status.IDShown, shared+1))
	}
	return cells
}

// sharedStart is how many characters a and b start alike.
func sharedStart(a, b []rune) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
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

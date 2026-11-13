package dashboard

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/views"
)

// ProjectsShown is Projects as the watch gives it: the project the list has
// chosen, by its path, as the page's rows hold it, and how many of the list's
// rows are scrolled out of view above, as the list scrolls with the choice;
// whether ↑ and ↓ act in the chosen project's SESSIONS, → having taken them
// there from the list; the session selected there, by id, "" for the first;
// and the period's page and the chosen project's, as the watch built them of
// the days read: nil till they're read; and its SESSIONS' ended sessions
// measured, as MeasureSessions measures them as the project's page is built.
type ProjectsShown struct {
	Chosen     string
	ListScroll int
	InSessions bool
	Selected   string
	Page       *views.ProjectsPage
	Project    *views.ProjectPage
	Measured   SessionsMeasured
}

// heading is Projects' heading row: its name, and at its right, the period
// given between < and >, as periodDial has it.
func (s ProjectsShown) heading(period views.Period) heading {
	h := heading{name: strings.ToUpper(Projects.Title())}
	h.period, h.widest = periodDial(period)
	return h
}

// rows are the list's rows: none till the page is read.
func (s ProjectsShown) rows() []views.ProjectRow {
	if s.Page == nil {
		return nil
	}
	return s.Page.Rows
}

// chosenAt is the place among the list's rows of the one chosen: -1 where
// none is.
func (s ProjectsShown) chosenAt() int {
	return slices.IndexFunc(s.rows(), func(r views.ProjectRow) bool { return r.Path == s.Chosen })
}

// sessions are the chosen project's sessions, as SESSIONS rows them: none
// till its page is built.
func (s ProjectsShown) sessions() []views.ProjectSession {
	if s.Project == nil {
		return nil
	}
	return s.Project.Sessions
}

// Selection is the session SESSIONS has selected, by id: Selected, where the
// chosen project has it, else its first; "" where it has none.
func (s ProjectsShown) Selection() string {
	sessions := s.sessions()
	if i := slices.IndexFunc(sessions, func(p views.ProjectSession) bool { return p.Session == s.Selected }); i >= 0 {
		return s.Selected
	}
	if len(sessions) == 0 {
		return ""
	}
	return sessions[0].Session
}

// Opens reports whether enter opens a session's page: in SESSIONS, with one
// selected.
func (s ProjectsShown) Opens() bool {
	return s.InSessions && s.Selection() != ""
}

// Choosing is s with ↑ or ↓ pressed, by -1 or 1: in the list, the project
// before or after the one chosen, SESSIONS' selection let go, for its first;
// in SESSIONS, the session before or after the one selected. It reports
// false where there's none that way.
func (s ProjectsShown) Choosing(by int) (ProjectsShown, bool) {
	if s.InSessions {
		sessions := s.sessions()
		at := slices.IndexFunc(sessions, func(p views.ProjectSession) bool { return p.Session == s.Selection() })
		if next := at + by; at >= 0 && next >= 0 && next < len(sessions) {
			s.Selected = sessions[next].Session
			return s, true
		}
		return s, false
	}
	rows := s.rows()
	if next := s.chosenAt() + by; next >= 0 && next < len(rows) {
		s.Chosen, s.Selected = rows[next].Path, ""
		return s, true
	}
	return s, false
}

// Moved is s with → or ← pressed, by 1 or -1: → takes ↑ and ↓ into the
// chosen project's SESSIONS, where it has any, and ← back to the list. It
// reports false where neither moves them.
func (s ProjectsShown) Moved(by int) (ProjectsShown, bool) {
	into := by > 0
	if into == s.InSessions || into && len(s.sessions()) == 0 {
		return s, false
	}
	s.InSessions = into
	return s, true
}

// ChosenOn is s with the project with the given path chosen, as a click on
// its row chooses it, ↑ and ↓ back in the list.
func (s ProjectsShown) ChosenOn(path string) ProjectsShown {
	if path != s.Chosen {
		s.Chosen, s.Selected = path, ""
	}
	s.InSessions = false
	return s
}

// SelectedOn is s with the session with the given id selected, as a click on
// its row selects it, ↑ and ↓ in SESSIONS.
func (s ProjectsShown) SelectedOn(id string) ProjectsShown {
	s.Selected, s.InSessions = id, true
	return s
}

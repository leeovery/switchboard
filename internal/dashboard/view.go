package dashboard

import (
	"cmp"
	"slices"
	"strings"
)

// View is one of the dashboard's tabs, each a view of the document, as the
// preferences file keeps the one shown.
type View string

const (
	// Overview is the default tab: every account's windows and the router,
	// understood at a glance.
	Overview View = "overview"
	// Accounts is each account in depth, and all of them together.
	Accounts View = "accounts"
	// Projects is each project sessions ran in over the period, ranked by
	// worth, and the one chosen in depth.
	Projects View = "projects"
	// Sessions is the view of where each session's requests go, as a
	// switchboard draws it: the sessions as calls, the accounts as lines,
	// and a cord from each call to its line, with requests travelling it.
	Sessions View = "sessions"
	// Runway is the view of when each account has room, as a timeline: a lane
	// per account, over the window its view is named for, the 5-hour or the
	// week.
	Runway View = "runway"
	// Graphs is Trends, the tab once called Graphs, its name kept where the
	// preferences keep the tab shown: the shapes of use over time, across
	// every account, every account's room drawn at once among them, in
	// sections its user shows and hides, as the Overview's are.
	Graphs View = "graphs"
	// Log is every request and every event the router tells of, as they
	// happen.
	Log View = "log"
	// SessionPage is no tab but a session's page, open over the tab it was
	// opened from, as a frame's Shown has it: never one the title row lists,
	// tab moves to, or the preferences keep.
	SessionPage View = "session"
)

// titles are the tabs' names in the title row, and a session's page's, as
// help names it.
var titles = map[View]string{
	Overview: "Overview", Accounts: "Accounts", Projects: "Projects", Sessions: "Sessions", Runway: "Runway", Graphs: "Trends", Log: "Log", SessionPage: "Session",
}

// Views are the tabs, in the order the title row shows them and tab moves
// through them.
func Views() []View {
	return []View{Overview, Accounts, Projects, Sessions, Runway, Graphs, Log}
}

// historyView is the History tab, by the name the preferences kept it under
// in builds that had it.
const historyView View = "history"

// retiredViews are tabs earlier builds had, by the names the preferences
// kept them under, each with the tab that took its place: History's, whose
// pictures over time went to Trends.
var retiredViews = map[View]View{historyView: Graphs}

// ViewKept is the tab the preferences kept by the name given: the one that
// took its place where it's retired, else the one it names, whether a tab
// there is or not, as one a later build kept may not be.
func ViewKept(name string) View {
	kept := View(name)
	return cmp.Or(retiredViews[kept], kept)
}

// headsItself reports whether the tab's sections head themselves, as the
// Overview's and Trends' do, with no heading row over them.
func (v View) headsItself() bool {
	return v == Overview || v == Graphs
}

// SubViews are the tab's own views, named as its heading row names them, in
// lower case, the one it opens on first: its sub-tabs;
// none where it has none.
func (v View) SubViews() []string {
	switch v {
	case Accounts:
		return lowerNames(accountsViews)
	case Sessions:
		return lowerNames(sessionsViews)
	case Log:
		return lowerNames(logViews)
	}
	return nil
}

// Title is the tab's name in the title row, such as Overview.
func (v View) Title() string {
	return cmp.Or(titles[v], string(v))
}

// lowerNames are a tab's sub-tabs' names as the preferences keep them, in
// the heading row's order: lowercase, as "compare".
func lowerNames(names []string) []string {
	lowered := make([]string, len(names))
	for i, name := range names {
		lowered[i] = strings.ToLower(name)
	}
	return lowered
}

// placeAmong is the place of name among names: the first's, 0, where it's
// none of them.
func placeAmong(name string, names []string) int {
	return max(slices.Index(names, name), 0)
}

// round is i among n places, round: as far on from the first as i is past
// the last, or back from the last as it's before the first.
func round(i, n int) int {
	return (i%n + n) % n
}

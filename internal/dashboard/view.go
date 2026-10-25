package dashboard

import "cmp"

// View is one of the dashboard's tabs, each a view of the document, as the
// preferences file keeps the one shown.
type View string

const (
	// Overview is the default tab: every account's windows and the router,
	// understood at a glance.
	Overview View = "overview"
	// Accounts is each account in depth, and all of them together.
	Accounts View = "accounts"
	// History is when: the weeks and days gone, and what used them.
	History View = "history"
	// Sessions is the view of where each session's requests go, as a
	// switchboard draws it: the sessions as calls, the accounts as lines,
	// and a cord from each call to its line, with requests travelling it.
	Sessions View = "sessions"
	// Runway is the view of when each account has room, as a timeline: a lane
	// per account, over the window its view is named for, the 5-hour or the
	// week.
	Runway View = "runway"
	// Graphs is every account's room drawn at once, in sections its user
	// shows and hides, as the Overview's are.
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
	Overview: "Overview", Accounts: "Accounts", History: "History", Sessions: "Sessions", Runway: "Runway", Graphs: "Graphs", Log: "Log", SessionPage: "Session",
}

// Views are the tabs, in the order the title row shows them and tab moves
// through them.
func Views() []View {
	return []View{Overview, Accounts, History, Sessions, Runway, Graphs, Log}
}

// headsItself reports whether the tab's sections head themselves, as the
// Overview's and Graphs' do, with no heading row over them.
func (v View) headsItself() bool {
	return v == Overview || v == Graphs
}

// Title is the tab's name in the title row, such as Overview.
func (v View) Title() string {
	return cmp.Or(titles[v], string(v))
}

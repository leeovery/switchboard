package dashboard

import "cmp"

// View is one of the dashboard's tabs, each a view of the document, as the
// preferences file keeps the one shown.
type View string

const (
	// Overview is the default tab: every account's windows and the router,
	// understood at a glance. Until stage 7 builds it, it shows milestone 5's
	// view of a card per account.
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
	// per account, over the day, or the week.
	Runway View = "runway"
	// Log is every request and every event the router tells of, as they
	// happen.
	Log View = "log"
)

// titles are the tabs' names in the title row.
var titles = map[View]string{
	Overview: "Overview", Accounts: "Accounts", History: "History", Sessions: "Sessions", Runway: "Runway", Log: "Log",
}

// Views are the tabs, in the order the title row shows them and tab moves
// through them.
func Views() []View {
	return []View{Overview, Accounts, History, Sessions, Runway, Log}
}

// Title is the tab's name in the title row, such as Overview.
func (v View) Title() string {
	return cmp.Or(titles[v], string(v))
}

// Span is how far ahead Runway looks, as w switches it: the day, as it
// opens, or the week.
type Span string

const (
	// Day is from the hour before now to about a day ahead.
	Day Span = ""
	// Week is from yesterday to six days ahead.
	Week Span = "week"
)

// Next is the span w switches to from s: the week from the day, and back.
func (s Span) Next() Span {
	if s == Week {
		return Day
	}
	return Week
}

// Name is what the key line and the help call the span: "day" or "week".
func (s Span) Name() string {
	return cmp.Or(string(s), "day")
}

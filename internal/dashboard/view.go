package dashboard

import "cmp"

// View is one of the dashboard's views of the document, as the preferences
// file keeps the one shown.
type View string

const (
	// Accounts is the view of a card per account: how much of each is left,
	// and where that's heading.
	Accounts View = "accounts"
	// Runway is the view of when each account has room, as a timeline: a lane
	// per account, over the day, or the week.
	Runway View = "runway"
)

// titles are the views' names on their tabs.
var titles = map[View]string{Accounts: "Accounts", Runway: "Runway"}

// Views are the views there are, in the order tab moves through them.
func Views() []View {
	return []View{Accounts, Runway}
}

// Title is the view's name on its tab, such as Accounts.
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

// Name is what the footer and the help call the span: "day" or "week".
func (s Span) Name() string {
	return cmp.Or(string(s), "day")
}

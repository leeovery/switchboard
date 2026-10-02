package dashboard

import "cmp"

// View is one of the dashboard's views of the document, as the preferences
// file keeps the one shown.
type View string

// Accounts is the view of a card per account: how much of each is left, and
// where that's heading.
const Accounts View = "accounts"

// titles are the views' names on their tabs.
var titles = map[View]string{Accounts: "Accounts"}

// Views are the views there are, in the order tab moves through them.
func Views() []View {
	return []View{Accounts}
}

// Title is the view's name on its tab, such as Accounts.
func (v View) Title() string {
	return cmp.Or(titles[v], string(v))
}

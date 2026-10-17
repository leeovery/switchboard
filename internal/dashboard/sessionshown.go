package dashboard

import "github.com/leeovery/switchboard/internal/views"

// SessionShown is a session's page as the frame draws it, in place of the
// view it was opened from: the page and its story, as the views build them;
// the turn chosen, and the turn open, with its requests' rows and the
// request chosen among them; how far its turns, and the open turn's
// requests, are scrolled; p, as the key line lists it; the home its
// directory is shown under; and whether it's loading.
type SessionShown struct {
	Page  views.SessionPage
	Story views.Story
	// Turn is the turn chosen, by its number, from 1 at the first: 0 where
	// it has none.
	Turn int
	// Open is the turn open, by its number: 0 while none is. Requests are the
	// Log's lines of its requests, oldest first, as
	// views.HeldPage.TurnRequests gives them, and Request the one chosen
	// among them, by its index; RequestsLoading is set while they're read,
	// as views.BuiltPage's has it, its foot saying so.
	Open            int
	Requests        []views.LogLine
	Request         int
	RequestsLoading bool
	// Scroll is how many rows its turns are scrolled down by, and
	// RequestsScroll how many the open turn's requests are.
	Scroll, RequestsScroll int
	// Pin is p as the key line lists it, and Pinned whether it lists it.
	Pin    Key
	Pinned bool
	// Home is the home directory, which the heading shows the session's
	// directory under as ~: "" shows it whole.
	Home string
	// Loading is set while its lines are being read, asked to open, its page
	// knowing no more than views.AskedPage gives: its heading alone is drawn,
	// saying so.
	Loading bool
}

package dashboard

import "github.com/leeovery/switchboard/internal/views"

// SessionShown is a session's page as the frame draws it, in place of the
// view it was opened from: the page and its story, as the views build them;
// the turn chosen, and the turn open, with its requests' rows and the
// request chosen among them; how far its turns, and the open turn's
// requests, are scrolled; and p, as the key line lists it.
type SessionShown struct {
	Page  views.SessionPage
	Story views.Story
	// Turn is the turn chosen, by its number, from 1 at the first: 0 where
	// it has none.
	Turn int
	// Open is the turn open, by its number: 0 while none is. Requests are its
	// requests' rows, oldest first, as views.HeldPage.TurnRequests gives
	// them, and Request the one chosen among them, by its index.
	Open     int
	Requests []views.RequestRow
	Request  int
	// Scroll is how many rows its turns are scrolled down by, and
	// RequestsScroll how many the open turn's requests are.
	Scroll, RequestsScroll int
	// Pin is p as the key line lists it, and Pinned whether it lists it.
	Pin    Key
	Pinned bool
}

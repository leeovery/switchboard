package dashboard

import "strings"

// Help is the page's help, open over it, as the watch gives it: every key
// that works on the page, those the key line leaves out among them, worded
// as the key line words them; and how many rows it's scrolled down by. The
// page's name, for HELP · <PAGE>, and its parts, each a title and its lines,
// come of the frame: of the views milestone 5 built, the glyphs their key
// explains.
type Help struct {
	Keys   []Key
	Scroll int
}

// card is the help card of the frame's page. Until it's built, it's its
// title alone.
func (h Help) card(f Frame) Card {
	return Card{title: "HELP · " + strings.ToUpper(f.View.Title()), width: f.cardWidth(helpWidth)}
}

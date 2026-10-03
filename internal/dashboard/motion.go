package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Motion is what moves on screen in a frame, which a watch draws frames of:
// Smooth while anything moves frame by frame, as a pulse along a cord, or a
// cord a move let go fading; Shimmer while the shimmer down a cord steps on
// as its answer streams back, and Falling while the sand falls in a card's
// hourglass, each a step at a time.
type Motion struct {
	Smooth, Shimmer, Falling bool
}

// Motion is what moves on screen of the frame of doc at now: what travels
// the cords Sessions' switchboard shows, as travelling has it; or the sand
// falling in the hourglasses of the Accounts view's cards, as falling has
// it. A frame printed once has nothing that moves.
func (f Frame) Motion(doc status.Document, now time.Time) Motion {
	switch {
	case f.printed():
		return Motion{}
	case f.View == Sessions:
		return f.travelling(doc, now)
	case f.View == Accounts:
		return Motion{Falling: f.falling(doc, now)}
	default:
		return Motion{}
	}
}

// box is a block of cells: from x along row y, width cells wide and rows
// tall.
type box struct {
	x, y, width, rows int
}

// covers reports whether the box covers every one of the cells from x along
// row y, width cells wide and rows tall.
func (b box) covers(x, y, width, rows int) bool {
	return x >= b.x && x+width <= b.x+b.width && y >= b.y && y+rows <= b.y+b.rows
}

// sight is how a view's content shows on screen: rows of its rows, from
// the row offset, from the screen's row top; but where the help is open
// over it, the cells it covers.
type sight struct {
	top, offset, rows int
	help              box
}

// shows reports whether any of the cells from x along the content's row y,
// width cells wide and rows tall, shows on screen.
func (s sight) shows(x, y, width, rows int) bool {
	for row := max(y, s.offset); row < min(y+rows, s.offset+s.rows); row++ {
		if width > 0 && !s.help.covers(x, s.top+row-s.offset, width, 1) {
			return true
		}
	}
	return false
}

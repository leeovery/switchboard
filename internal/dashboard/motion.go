package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Motion is what moves on screen in a frame, which a watch draws frames of:
// Smooth while anything moves frame by frame, as a pulse along a cord, or a
// cord a move let go fading; and Shimmer while the shimmer down a cord steps
// on as its answer streams back, a step at a time.
type Motion struct {
	Smooth, Shimmer bool
}

// Motion is what moves on screen of the frame of doc at now: what travels
// the cords Sessions' switchboard shows, as travelling has it. Nothing moves
// of a view too short to be drawn, as cramped says.
func (f Frame) Motion(doc status.Document, now time.Time) Motion {
	if f.cramped() || f.View != Sessions {
		return Motion{}
	}
	return f.travelling(doc, now)
}

// sight is how a view's content shows on screen: rows of its rows, from the
// row offset; and none of it where a card is open over the page, which
// stands still behind it.
type sight struct {
	offset, rows int
	covered      bool
}

// inSight is how the frame's page shows its view's content, rows of its rows
// from the row offset.
func (f Frame) inSight(offset, rows int) sight {
	return sight{offset: offset, rows: rows, covered: f.Card != nil}
}

// shows reports whether any of the content's cells, width cells wide from
// its row y, rows tall, shows on screen.
func (s sight) shows(y, width, rows int) bool {
	return !s.covered && width > 0 && max(y, s.offset) < min(y+rows, s.offset+s.rows)
}

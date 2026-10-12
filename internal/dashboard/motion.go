package dashboard

import (
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/status"
)

// Motion is what moves on screen in a frame, which a watch draws frames of:
// Smooth while anything moves frame by frame, as a pulse along a cord, a
// cord a move let go fading, a number brightened or a cue fading back, or a
// bar easing to a new reading; and Shimmer while the shimmer down a cord
// steps on as its answer streams back, a step at a time.
type Motion struct {
	Smooth, Shimmer bool
}

// Motion is what moves on screen of the frame of doc at now: what travels
// the cords Sessions' switchboard shows, as travelling has it, but nothing
// of a view too short to be drawn, as cramped says; and anything the page
// draws settling, as settling says.
func (f Frame) Motion(doc status.Document, now time.Time) Motion {
	var moving Motion
	if !f.cramped() && f.View == Sessions {
		moving = f.travelling(doc, now)
	}
	moving.Smooth = moving.Smooth || f.settling(doc, now)
	return moving
}

// settling reports whether anything the frame's page draws of doc at now
// settles still: a number brightening, as its Brightened says; a bar
// easing, as its Eased says; or a cue fading back, on an event fresh, as its
// Fresh says, an account's state changed, as its Changed says, or a window's
// words turned to out ~, as its TurnedOut says. What the page doesn't draw
// never does, as one scrolled out of view, in a section folded, or left off
// for want of room.
func (f Frame) settling(doc status.Document, now time.Time) bool {
	if len(f.Brightened)+len(f.Eased)+len(f.Fresh)+len(f.Changed)+len(f.TurnedOut) == 0 {
		return false
	}
	for _, row := range f.page(doc, now).cells {
		for _, here := range row {
			if here.mark != (unique.Handle[mark]{}) && f.settles(here.mark.Value()) {
				return true
			}
		}
	}
	return false
}

// settles reports whether what m marks settles still in the frame: a cue,
// drawn only while it fades back; a bar easing; or a number brightening.
func (f Frame) settles(m mark) bool {
	if m.cued {
		return true
	}
	if m.bar != (Ref{}) {
		_, easing := f.Eased[m.bar]
		return easing
	}
	return f.Brightened[m.number] > 0
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

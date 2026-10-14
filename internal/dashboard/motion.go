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

// Motion is what moves on screen of the frame of doc at now, as moving has
// it: its page drawn to know only where anything on it may settle, as
// MaySettle says, as what travels the cords needs none of it drawn.
func (f Frame) Motion(doc status.Document, now time.Time) Motion {
	if !f.MaySettle() {
		return f.travel(doc, now)
	}
	return f.moving(f.page(doc, now), doc, now)
}

// moving is what moves on screen of the frame of doc at now, its page drawn
// on c: what travels the cords, as travel has it; and anything the page
// draws settling, as settling says.
func (f Frame) moving(c *canvas, doc status.Document, now time.Time) Motion {
	moving := f.travel(doc, now)
	moving.Smooth = moving.Smooth || f.settling(c)
	return moving
}

// travel is what travels the cords Sessions' switchboard shows, as
// travelling has it, but nothing of a view too short to be drawn, as
// cramped says.
func (f Frame) travel(doc status.Document, now time.Time) Motion {
	if f.cramped() || f.View != Sessions {
		return Motion{}
	}
	return f.travelling(doc, now)
}

// MaySettle reports whether anything the frame's page draws may be
// settling: a number brightening, as its Brightened says; a bar easing, as
// its Eased says; or a cue fading back, on an event fresh, as its Fresh
// says, an account's state changed, as its Changed says, or a window's words
// turned to out ~, as its TurnedOut says. Where nothing may, what moves on
// screen needs none of the page drawn.
func (f Frame) MaySettle() bool {
	return len(f.Brightened)+len(f.Eased)+len(f.Fresh)+len(f.Changed)+len(f.TurnedOut) > 0
}

// Filled are how much of each bar the frame's view draws of doc at now it
// fills, as drawn without easing, by its window's account and key, the
// pool's by its key alone, for a watch to ease each bar from and to: each
// account's window as it stands at now, which reads empty once it has reset
// since it was read; on the Overview, as USAGE draws them, and the pool's as
// the frame judges it at now, but none of a bar USAGE draws empty, which has
// nothing to ease; on Sessions, as its panels draw them, but none of a window
// a panel draws without a bar; and none on a view that draws no bar.
func (f Frame) Filled(doc status.Document, now time.Time) map[Ref]float64 {
	switch f.View {
	case Overview:
		return f.overviewFilled(doc, now)
	case Sessions:
		return f.sessionsFilled(doc, now)
	}
	return nil
}

// overviewFilled are how much of each bar the Overview's USAGE draws of doc
// at now it fills, as Filled has them.
func (f Frame) overviewFilled(doc status.Document, now time.Time) map[Ref]float64 {
	filled := make(map[Ref]float64)
	for _, a := range doc.Accounts {
		states := doc.WindowStatesOf(a, now, f.Policy)
		for _, w := range a.Windows {
			if st := states.Of(w.Key); !drawsEmpty(st) {
				filled[Ref{Account: a.ID, Window: w.Key}] = st.Used
			}
		}
	}
	for _, p := range f.pool(doc, now).Windows {
		filled[Ref{Window: p.Key}] = p.Used
	}
	return filled
}

// sessionsFilled are how much of each bar Sessions' panels draw of doc at
// now it fills, as Filled has them, by its account and key: each window as
// its state reads it, where its panel draws it a bar, as it does where it
// doesn't stand not started.
func (f Frame) sessionsFilled(doc status.Document, now time.Time) map[Ref]float64 {
	used := make(map[Ref]float64)
	for _, a := range doc.Accounts {
		states := doc.WindowStatesOf(a, now, f.Policy)
		for _, w := range a.Windows {
			if st := states.Of(w.Key); st.Kind != status.WindowNotStarted {
				used[Ref{Account: a.ID, Window: w.Key}] = st.Used
			}
		}
	}
	return used
}

// settling reports whether anything the frame's page, drawn on c, settles
// still, as MaySettle has it may. What the page doesn't draw never does, as
// one scrolled out of view, in a section folded, or left off for want of
// room.
func (f Frame) settling(c *canvas) bool {
	if !f.MaySettle() {
		return false
	}
	for _, row := range c.cells {
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

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
// it: its page drawn, of the one moment, as At holds it, only where anything
// on it may settle, as MaySettle says.
func (f Frame) Motion(doc status.Document, now time.Time) Motion {
	f = f.At(doc, now)
	if !f.MaySettle() {
		return Motion{}
	}
	return f.moving(f.page(doc, now))
}

// moving is what moves on screen of the frame's page, drawn on c, as
// MaySettle has it may: anything it draws settling, as settles says, and
// what travels a cord it draws, as travelling says. What the page doesn't
// draw never moves, as one scrolled out of view, in a section folded, or
// left off for want of room.
func (f Frame) moving(c *canvas) Motion {
	var moving Motion
	if !f.MaySettle() {
		return moving
	}
	for _, row := range c.cells {
		for _, here := range row {
			if here.mark != (unique.Handle[mark]{}) {
				moving = moving.or(f.motionOf(here.mark.Value()))
			}
		}
	}
	return moving
}

// or is what moves of m and n together.
func (m Motion) or(n Motion) Motion {
	return Motion{Smooth: m.Smooth || n.Smooth, Shimmer: m.Shimmer || n.Shimmer}
}

// motionOf is what moves of what m marks: of a cord, as travelling has it,
// else Smooth while it settles, as settles has it.
func (f Frame) motionOf(m mark) Motion {
	if m.cord != (Plug{}) {
		return f.travelling(m)
	}
	return Motion{Smooth: f.settles(m)}
}

// travelling is what moves of the cord m marks, as the Traffic tells: a
// loose one Smooth while it hangs, fading; else Smooth while a pulse runs
// along it, and Shimmer while the answer of the call plugged in there
// streams back.
func (f Frame) travelling(m mark) Motion {
	if m.loose {
		return Motion{Smooth: f.Traffic.hangs(m.cord)}
	}
	call := f.Traffic.Calls[m.cord]
	return Motion{Smooth: call.Pulsing, Shimmer: call.Doing == Streaming}
}

// MaySettle reports whether anything the frame's page draws may be
// settling: a number brightening, as its Brightened says; a bar easing, as
// its Eased says; a cue fading back, on an event fresh, as its Fresh says,
// an account's state changed, as its Changed says, or in a word, as its
// StateChanged says, or a window's words turned to out ~, as its TurnedOut
// says; or anything travelling the cords Sessions draws, as travels says.
// Where nothing may, what moves on screen needs none of the page drawn.
func (f Frame) MaySettle() bool {
	return len(f.Brightened)+len(f.Eased)+len(f.Fresh)+len(f.Changed)+len(f.StateChanged)+len(f.TurnedOut) > 0 || f.travels()
}

// travels reports whether anything travels the cords Sessions draws, with
// no session's page over it, as its Traffic tells: a pulse along one, a
// shimmer down one, or a cord a move let go, fading.
func (f Frame) travels() bool {
	return f.View == Sessions && f.Session == nil && f.Traffic.moves()
}

// Filled are how much of each bar the frame's view draws of doc at now it
// fills, as drawn without easing, by its window's account and key, the
// pool's by its key alone, for a watch to ease each bar from and to, of the
// moment of doc at now, as At holds it: each account's window as it stands
// at now, which reads empty once it has reset since it was read; on the
// Overview, as USAGE draws them, and the pool's at now, but none of a bar
// USAGE draws empty, which has nothing to ease; on Accounts and Sessions,
// as accountsFilled and sessionsFilled have them; and none on a view that
// draws no bar, nor under a session's page, which draws none.
func (f Frame) Filled(doc status.Document, now time.Time) map[Ref]float64 {
	switch m := f.Moment(doc, now); {
	case f.Session != nil:
	case f.View == Overview:
		return overviewFilled(m)
	case f.View == Accounts:
		return f.accountsFilled(doc, now)
	case f.View == Sessions:
		return f.sessionsFilled(doc, now)
	}
	return nil
}

// overviewFilled are how much of each bar the Overview's USAGE draws of m it
// fills, as Filled has them.
func overviewFilled(m status.Moment) map[Ref]float64 {
	filled := windowsFilled(m, func(w status.WindowNow) bool { return !drawsEmpty(w.Kind) })
	for _, p := range m.Pool().Windows {
		filled[Ref{Window: p.Key}] = p.Used
	}
	return filled
}

// windowsFilled are how much of each of m's accounts' windows a bar drawn of
// it fills, as it's used as it stands, by its account and key, of those
// drawn says draws a bar.
func windowsFilled(m status.Moment, drawn func(status.WindowNow) bool) map[Ref]float64 {
	filled := make(map[Ref]float64)
	for _, a := range m.Accounts {
		for _, w := range a.Windows {
			if drawn(w) {
				filled[Ref{Account: a.ID, Window: w.Key}] = w.Used
			}
		}
	}
	return filled
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

package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// RoomAhead is the room the account's window with the given key has at t,
// from Now on, a share of the window, 1 − its use, by the projection the
// moment goes by, its Out, for every chart heading from now to draw, never
// worked out again. A window falling ahead falls from the room Used leaves,
// at Now, or once a limit holding it lifts, where score anchors the
// projection at ReadAt, at the use as read: the two start from the same room,
// as Used is the use as read until the window resets, and end at the same
// place, Out.At and the room Out's heading leaves it at its reset, the
// moment's own; between, the line from Now runs below score's from ReadAt.
//
// Each kind has one course, from Now for a window as it stands, and from
// Back for one a limit holds, by the kind it's Unheld once the limit lifts:
//
//   - A window that runs out falls straight to its account's Floor at
//     Out.At, the room there being its reserve where that holds the account
//     back, else none, and keeps that till its reset; it's at the floor from
//     the start where Out.At is no later.
//   - An open one falls straight to the room its heading leaves it at its
//     reset, where that heads on pace there: what Projected fills to, which
//     the moment gives no window a limit holds.
//   - One at its cap keeps the room its cap leaves till its reset.
//   - One a limit holds has none till it lifts, at Back; then its unheld
//     kind's course, or, where that kind has none, as stale, not started or
//     heading nowhere said, the room Used leaves till its reset.
//   - Past its reset it has all of it, as nothing is known of its next
//     window. Where its reset, or when a limit lifts, isn't known, it keeps
//     the room it has.
//
// It reports false, the room 0, where nothing ahead is known to draw: before
// Now, which the projection says nothing of; of a window of an account
// without a usable token, or nothing read; of one not started, or open and
// heading nowhere said; and of one stale, unless a limit holds it, by choice
// rather than for want of a projection, as the moment projects a stale
// window all the same, but what's ahead of a reading gone stale isn't drawn.
func (a AccountNow) RoomAhead(key string, t time.Time) (float64, bool) {
	w, _ := a.Window(key)
	c, ok := a.courseOf(w)
	if !ok || t.Before(a.now) {
		return 0, false
	}
	return c.at(t), true
}

// courseOf is where the account's window w goes from Now on, as RoomAhead
// has it, reporting false where nothing ahead is known to draw.
func (a AccountNow) courseOf(w WindowNow) (course, bool) {
	if w.Kind == WindowLimited {
		return a.heldFrom(w), true
	}
	return a.courseFrom(w.Kind, point{a.now, 1 - w.Used}, w)
}

// courseFrom is window w's course from from on, standing as kind says, as
// RoomAhead has each kind's, alike for a window as it stands and one a limit
// holds once that lifts, reporting false of a kind with none.
func (a AccountNow) courseFrom(kind WindowKind, from point, w WindowNow) (course, bool) {
	switch {
	case kind == WindowRunsOut:
		return course{from, {w.Out.At, 1 - a.Floor}, {w.Next, 1 - a.Floor}}, true
	case kind == WindowOpen && w.Out.Kind == score.OnPace:
		return course{from, {w.Next, 1 - w.Out.AtReset}}, true
	case kind == WindowCapped:
		return course{{from.at, 1 - a.Cap}, {w.Next, 1 - a.Cap}}, true
	}
	return nil, false
}

// heldFrom is the course of window w, which a limit holds, from Now on: none
// till it lifts, at its Back, where that's known; then from the room its use
// leaves, as the course of the kind it's Unheld goes, else keeping that room
// till its reset.
func (a AccountNow) heldFrom(w WindowNow) course {
	held := course{{a.now, 0}, {w.Back, 0}}
	if w.Back.IsZero() {
		return held
	}
	lifted := point{w.Back, 1 - w.Used}
	if unheld, ok := a.courseFrom(w.Unheld, lifted, w); ok {
		return append(held, unheld...)
	}
	return append(held, lifted, point{w.Next, lifted.room})
}

// course is where a window's room goes from a moment on: straight from each
// point to the next, stepping to one no later than the one before; and past
// the last, its reset, all of it. A last point at no known time, zero, keeps
// its room for ever.
type course []point

// point is the room a window has at a time.
type point struct {
	at   time.Time
	room float64
}

// at is the room on the course at t, at or after its first point.
func (c course) at(t time.Time) float64 {
	for i := 1; i < len(c); i++ {
		if from, to := c[i-1], c[i]; to.at.IsZero() || !t.After(to.at) {
			return from.toward(to, t)
		}
	}
	return 1
}

// toward is the room at t on the straight line from p to q: q's where q is
// at p's time, and p's where q's time isn't known.
func (p point) toward(q point, t time.Time) float64 {
	switch span := q.at.Sub(p.at); {
	case q.at.IsZero():
		return p.room
	case span <= 0:
		return q.room
	default:
		return p.room + (q.room-p.room)*float64(t.Sub(p.at))/float64(span)
	}
}

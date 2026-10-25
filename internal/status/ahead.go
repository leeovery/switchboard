package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// RoomAhead is the room the account's window with the given key has at t,
// from Now on, a share of the window, 1 − its use, by the projection the
// moment goes by, its Out and Projected, for every chart heading from now to
// draw, never worked out again. A window falling ahead falls from the room
// Used leaves, at Now, or once a limit holding it lifts, where score anchors
// the projection at ReadAt, at the use as read: the two meet at both ends, as
// Used is the use as read until the window resets, and the ends, Out.At,
// Projected and Out's heading, are the moment's own.
//
//   - A window that runs out falls straight from the room Used leaves to its
//     account's Floor at Out.At, the room there being its reserve where that
//     holds the account back, else none, and keeps that till its reset; one
//     its projection keeps short of running out falls straight to the room
//     Projected leaves it at its reset.
//   - One at its cap keeps the room its cap leaves, from Now till its reset.
//   - One a limit holds has none till it lifts, at Back. Then, where it runs
//     out ahead, as RunsOutAhead says, it falls as one that runs out, from
//     Back, and is at its Floor from Back where Out.At is sooner; else it
//     falls straight from the room Used leaves to the room Out's heading
//     leaves it at its reset, the moment giving a window a limit holds no
//     Projected, or keeps the room Used leaves where that heading says
//     nothing of its reset.
//   - Past its reset it has all of it, as nothing is known of its next
//     window. Where its reset, or when a limit lifts, isn't known, it keeps
//     the room it has.
//
// It reports false, the room 0, where nothing ahead is known to draw: before
// Now, which the projection says nothing of; of a window of an account
// without a usable token, or nothing read; of one not started, or open with
// nothing said of where it's heading, as a nil Projected says; and of one
// stale, unless a limit holds it, by choice rather than for want of a
// projection, as the moment projects a stale window all the same, but what's
// ahead of a reading gone stale isn't drawn.
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
	switch {
	case w.Kind == WindowRunsOut:
		return a.runningOut(point{a.now, 1 - w.Used}, w), true
	case w.Kind == WindowOpen && w.Projected != nil:
		return course{{a.now, 1 - w.Used}, {w.Next, 1 - *w.Projected}}, true
	case w.Kind == WindowCapped:
		return course{{a.now, 1 - a.Cap}, {w.Next, 1 - a.Cap}}, true
	case w.Kind == WindowLimited:
		return a.heldFrom(w), true
	}
	return nil, false
}

// runningOut is window w's course from from on, as it runs out: straight to
// its account's Floor at its Out, then that floor till its reset; at the
// floor from from where its Out is no later, as the course steps there.
func (a AccountNow) runningOut(from point, w WindowNow) course {
	return course{from, {w.Out.At, 1 - a.Floor}, {w.Next, 1 - a.Floor}}
}

// heldFrom is the course of window w, which a limit holds, from Now on: none
// till it lifts, at its Back, where that's known; then from the room its use
// leaves, running out where it runs out ahead, else to the room its heading
// leaves it at its reset, as heldToReset has it.
func (a AccountNow) heldFrom(w WindowNow) course {
	held := course{{a.now, 0}, {w.Back, 0}}
	lifted := point{w.Back, 1 - w.Used}
	switch {
	case w.Back.IsZero():
		return held
	case w.RunsOutAhead:
		return append(held, a.runningOut(lifted, w)...)
	}
	return append(held, lifted, point{w.Next, heldToReset(w)})
}

// heldToReset is the room window w, which a limit holds and which doesn't run
// out ahead, has at its reset once the limit lifts: what its Out's heading
// leaves it, where that heads on pace to its reset; else the room its use
// leaves, as nothing says where it heads.
func heldToReset(w WindowNow) float64 {
	if w.Out.Kind != score.OnPace {
		return 1 - w.Used
	}
	return 1 - w.Out.AtReset
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

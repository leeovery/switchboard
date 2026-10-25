package status

import "time"

// RoomAhead is the room the account's window with the given key has at t,
// from Now on, a share of the window, 1 − its use, by the projection the
// moment goes by, its Out and Projected, for every chart heading from now to
// draw, never worked out again. It goes from Now, at the room Used leaves,
// where score anchors the projection at ReadAt, at the use as read: the two
// meet at both ends, as Used at Now is the use as read until the window
// resets, and the ends, Out.At and Projected, are the moment's own.
//
//   - A window that runs out falls straight to its account's Floor at Out.At,
//     the room there being its reserve where that holds the account back,
//     else none, and keeps that till its reset; one its projection keeps
//     short of running out falls straight to the room Projected leaves it at
//     its reset.
//   - One at its cap keeps the room its cap leaves till its reset. One a
//     limit holds has none till it lifts, at Back; and where that's before
//     its reset, the room its use leaves from then till its reset, as nothing
//     projects a window while a limit holds it.
//   - Past its reset it has all of it, as nothing is known of its next
//     window. Where its reset, or when a limit lifts, isn't known, it keeps
//     the room it has.
//
// It reports false, the room 0, where nothing ahead is known to draw: before
// Now, which the projection says nothing of; and of a window of an account
// without a usable token, nothing read, stale, not started, or open with
// nothing said of where it's heading, as a nil Projected says, as none of
// those has a projection the moment goes by.
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
	now, room := a.now, 1-w.Used
	switch {
	case w.Kind == WindowRunsOut:
		return course{{now, room}, {w.Out.At, 1 - a.Floor}, {w.Next, 1 - a.Floor}}, true
	case w.Kind == WindowOpen && w.Projected != nil:
		return course{{now, room}, {w.Next, 1 - *w.Projected}}, true
	case w.Kind == WindowCapped:
		return course{{now, 1 - a.Cap}, {w.Next, 1 - a.Cap}}, true
	case w.Kind == WindowLimited:
		return heldFrom(now, w), true
	}
	return nil, false
}

// heldFrom is where window w, which a limit holds, goes from now on: none
// till it lifts, at its Back; then, where that's before its reset, or its
// reset isn't known, the room its use leaves.
func heldFrom(now time.Time, w WindowNow) course {
	held := course{{now, 0}, {w.Back, 0}}
	if w.Back.IsZero() || !w.Next.IsZero() && !w.Back.Before(w.Next) {
		return held
	}
	return append(held, point{w.Back, 1 - w.Used}, point{w.Next, 1 - w.Used})
}

// course is where a window's room goes from a moment on: straight from each
// point to the next, stepping where two are at one time; and past the last,
// its reset, all of it. A last point at no known time, zero, keeps its room
// for ever.
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

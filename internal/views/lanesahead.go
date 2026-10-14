package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// stood is how an account stood, or stands, at a column's moment: its cell;
// and of a hold, when it began and when it lifts, each zero where that isn't
// known, and whether a run-out projects its start.
type stood struct {
	cell         Cell
	since, until time.Time
	projected    bool
}

// wasCells are the cells each kind of how an account stood at a moment gone
// draws as.
var wasCells = map[status.WasKind]Cell{
	status.WasTokenless: CellTokenless, status.WasLimited: CellLimited, status.WasRefused: CellRefused,
	status.WasUnread: CellUnknown, status.WasCapped: CellCapped, status.WasRoom: CellRoom,
}

// stoodThen is how an account stood at a moment gone, as Then judged it.
func stoodThen(was status.Was) stood {
	return stood{cell: wasCells[was.Kind], since: was.Since, until: was.Until}
}

// holdsAt reports whether the hold holds at t: until it lifts, all along
// where that isn't known.
func (s stood) holdsAt(t time.Time) bool {
	return s.until.IsZero() || t.Before(s.until)
}

// ahead is how an account stands from now on, as the moment has it: without
// a usable token, or nothing known of it, all along; what holds it back at
// now, from when the router's event of it tells, till it lifts, as its
// Routing has it; and each window every model shares that runs out ahead,
// as its RunsOutAhead has it.
type ahead struct {
	tokenless, unknown bool
	hold               stood
	outs               []runOut
}

// runOut is a window running out ahead: from when, at its pace, till it
// resets, where it holds its account back, at its limit or its cap, as the
// cell given; and whether its room till then is drawn as running out, as the
// 5-hour view doesn't draw a week's.
type runOut struct {
	at, resets time.Time
	cell       Cell
	drawn      bool
}

// aheadOf is how the account a stands from now on, as ahead has it, in the
// view of the window w.
func aheadOf(a status.AccountNow, w RunwayWindow) ahead {
	var h ahead
	switch a.Routing.Kind {
	case status.RoutingTokenless:
		h.tokenless = true
	case status.RoutingUnreadable, status.RoutingUnread:
		h.unknown = true
	case status.RoutingLimited:
		h.hold = stood{cell: CellLimited, since: limitSince(a), until: a.Routing.Back}
	case status.RoutingRefused:
		h.hold = stood{cell: CellRefused, since: a.RefusedSince, until: a.Refusal.Until}
	case status.RoutingCapped:
		h.hold = stood{cell: CellCapped, since: a.CapSince, until: a.Routing.Back}
	}
	for _, win := range a.Windows {
		if !win.Shared || !win.RunsOutAhead || win.Out.At.IsZero() {
			continue
		}
		out := runOut{at: win.Out.At, resets: win.Next, cell: CellLimited, drawn: w == RunwayWeek || !quota.MultiDay(win.Key)}
		if win.Out.Reserve {
			out.cell = CellCapped
		}
		h.outs = append(h.outs, out)
	}
	return h
}

// limitSince is when the account a reached the limit holding it back from
// every request, as the router's events tell: the earliest of the windows
// every model shares that a limit holds, each as the event of the limit
// naming it, or naming none, tells; zero where none tells. The router's own
// limit's event is among those, as it names those windows, or none.
func limitSince(a status.AccountNow) time.Time {
	var since time.Time
	for _, w := range a.Windows {
		if w.Shared && w.Held && !w.Since.IsZero() && (since.IsZero() || w.Since.Before(since)) {
			since = w.Since
		}
	}
	return since
}

// at is how the account stands at t, from now on: without a usable token, or
// nothing known of it, all along, as neither has a hold or a run-out; else
// by the precedence Then judges by, held at a limit, by the hold at now or a
// run-out to it, its token refused, or held at its cap, by the hold at now or
// a run-out to it; with room, running out ahead, where the view draws that;
// else with room.
func (h ahead) at(t time.Time) stood {
	switch {
	case h.tokenless:
		return stood{cell: CellTokenless}
	case h.unknown:
		return stood{}
	}
	for _, cell := range []Cell{CellLimited, CellRefused, CellCapped} {
		if s, ok := h.heldAt(cell, t); ok {
			return s
		}
	}
	if slices.ContainsFunc(h.outs, func(o runOut) bool { return o.drawn && t.Before(o.at) }) {
		return stood{cell: CellRunsOut}
	}
	return stood{cell: CellRoom}
}

// heldAt is how the account is held back as cell says at t, reporting false
// where it isn't: by the hold at now, where it's of that cell and holds then,
// and by each run-out to it under way then, as one, since the first of them
// began, and until the last lifts, zero where one lifts at no known time.
func (h ahead) heldAt(cell Cell, t time.Time) (stood, bool) {
	var parts []stood
	if h.hold.cell == cell && h.hold.holdsAt(t) {
		parts = append(parts, h.hold)
	}
	for _, o := range h.outs {
		if o.cell == cell && !t.Before(o.at) && (o.resets.IsZero() || t.Before(o.resets)) {
			parts = append(parts, stood{cell: cell, since: o.at, until: o.resets, projected: true})
		}
	}
	return asOne(parts)
}

// asOne is the parts of a hold as one, reporting false where there are none:
// since the first began, as score.Earlier has it, so a part from before
// anything known begins first, projected where that part is; and until the
// last lifts, zero where one lifts at no known time.
func asOne(parts []stood) (stood, bool) {
	if len(parts) == 0 {
		return stood{}, false
	}
	one := parts[0]
	for _, p := range parts[1:] {
		if earlier := score.Earlier(one.since, p.since); !earlier.Equal(one.since) {
			one.since, one.projected = p.since, p.projected
		}
		if one.until.IsZero() || p.until.IsZero() {
			one.until = time.Time{}
		} else {
			one.until = score.Later(one.until, p.until)
		}
	}
	return one, true
}

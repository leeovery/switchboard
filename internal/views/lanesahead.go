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
// a usable token, or nothing known of it, all along; each hold at now, a
// part of its own, in the precedence Then judges by, from when the router's
// event of it tells till it lifts: a limit holding back every request, till
// its hold's until; its token refused, till the refusal's; and its cap, till
// it lets go; and each window every model shares that runs out ahead, as its
// RunsOutAhead has it.
type ahead struct {
	tokenless, unknown bool
	holds              []stood
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
	}
	if a.Held && a.Hold.Every {
		h.holds = append(h.holds, stood{cell: CellLimited, since: a.LimitSince, until: a.Hold.Until})
	}
	if a.TokenRefused {
		h.holds = append(h.holds, stood{cell: CellRefused, since: a.RefusedSince, until: a.Refusal.Until})
	}
	if a.CapHolds {
		h.holds = append(h.holds, stood{cell: CellCapped, since: a.CapSince, until: a.CapBack})
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

// at is how the account stands at t, from now on: without a usable token, or
// nothing known of it, all along, as neither has a hold or a run-out; else
// by the precedence Then judges by, held at a limit, by its hold at now or a
// run-out to it, its token refused, or held at its cap, by its hold at now
// or a run-out to it; with room, running out ahead, where the view draws
// that; else with room.
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
// where it isn't: by the parts of that cell holding it then, as partsAt has
// them, as one, since the first of them began, and until the last lifts,
// zero where one lifts at no known time.
func (h ahead) heldAt(cell Cell, t time.Time) (stood, bool) {
	return asOne(h.partsAt(t, func(c Cell) bool { return c == cell }))
}

// partsAt are the parts of what holds the account back at t, of the cells
// kept: its holds at now that hold then, and each run-out to its limit or
// its cap under way then, projected.
func (h ahead) partsAt(t time.Time, kept func(Cell) bool) []stood {
	var parts []stood
	for _, p := range h.holds {
		if kept(p.cell) && p.holdsAt(t) {
			parts = append(parts, p)
		}
	}
	for _, o := range h.outs {
		if kept(o.cell) && o.holdsAt(t) {
			parts = append(parts, stood{cell: o.cell, since: o.at, until: o.resets, projected: true})
		}
	}
	return parts
}

// holdsAt reports whether the run-out holds its account back at t: from when
// it runs out till its window resets.
func (o runOut) holdsAt(t time.Time) bool {
	return !t.Before(o.at) && (o.resets.IsZero() || t.Before(o.resets))
}

// turns are the moments after from and before to at which how the account
// stands may turn, as at has it, in order, each once: as a hold at now
// lifts, and as a run-out begins and as its window resets.
func (h ahead) turns(from, to time.Time) []time.Time {
	var at []time.Time
	for _, p := range h.holds {
		at = append(at, p.until)
	}
	for _, o := range h.outs {
		at = append(at, o.at, o.resets)
	}
	at = slices.DeleteFunc(at, func(t time.Time) bool { return !t.After(from) || !t.Before(to) })
	slices.SortFunc(at, time.Time.Compare)
	return slices.CompactFunc(at, time.Time.Equal)
}

// roomFrom is when the account next has room from t, as a hold lifts: once
// no part of what holds it back, as partsAt has them, holds it then, each
// holding on till it lifts; zero where one never does.
func (h ahead) roomFrom(t time.Time) time.Time {
	for !t.IsZero() {
		lift, held := asOne(h.partsAt(t, Cell.held))
		if !held {
			break
		}
		t = lift.until
	}
	return t
}

// drawnFrom is when the i'th of the holds at now is first drawn, reporting
// false where it never is: at now, or, under the holds before it, which the
// precedence draws first, as the last of them lifts, where it holds on past
// that.
func (h ahead) drawnFrom(i int, now time.Time) (time.Time, bool) {
	at := now
	for _, p := range h.holds[:i] {
		if p.until.IsZero() {
			return time.Time{}, false
		}
		at = score.Later(at, p.until)
	}
	return at, h.holds[i].holdsAt(at)
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

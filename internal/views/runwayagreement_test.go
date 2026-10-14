package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// Runway's lanes agree with the moment they're built from, at every hard
// moment, in either view, and at every moment Runway has to get right that
// holds now: now's column stands as the moment has the account stand; the
// use under a name, live, is its view's window's, as usage has it; the
// pool's lane counts the moment's pool; COMING UP lists what the moment
// does, in its order; a stretch holding now ends where the moment has room
// return, or, where a window runs out before then, as it resets, the run-out
// carrying the hold on; and a run-out's starts where its window runs out, as
// usage's out ~ has it.
func TestRunwaysLanesAgreeWithTheMoment(t *testing.T) {
	for _, c := range agreementCases() {
		t.Run(c.name, func(t *testing.T) {
			m, now := c.from.Moment, c.from.Timeline.NowColumn()
			l := views.Lanes{}.Look(c.from)
			for i, lane := range l.Accounts {
				a := m.Accounts[i]
				if got, want := lane.Cells[now], asRouted(a); got != want && (want != views.CellRoom || !got.HasRoom()) {
					t.Errorf("%s's now's column is %d, want %d, as it's routed (%d)", a.ID, got, want, a.Routing.Kind)
				}
				if w, ok := a.Window(viewKeys[c.from.Window]); ok && !lane.Use.Highest && (lane.Use.Used != w.Used || lane.Use.Read != read(w)) {
					t.Errorf("%s's use is %v, read %v; want %v, read %v, as usage has it", a.ID, lane.Use.Used, lane.Use.Read, w.Used, read(w))
				}
				for _, s := range lane.Stretches {
					if back := heldTill(a, s.Cell); s.First <= now && now <= s.Last && !s.Back.Equal(back) && !carriedOn(a, back, s.Back) {
						t.Errorf("%s's stretch holding now is back %v, want %v, as the moment has it", a.ID, s.Back, back)
					}
					if s.Projected && !slices.ContainsFunc(a.Windows, func(w status.WindowNow) bool { return w.RunsOutAhead && w.Out.At.Equal(s.Start) }) {
						t.Errorf("%s's stretch starts ~%v, where none of its windows runs out", a.ID, s.Start)
					}
				}
			}
			if want := poolOf(m); l.Pool.Of != want {
				t.Errorf("the pool is of %d, want %d", l.Pool.Of, want)
			}
			if coming := views.RunwayComing(m, c.from.Window); !inOrderAmong(coming, m.ComingUp) {
				t.Errorf("COMING UP lists %v, not in order among the moment's %v", coming, m.ComingUp)
			}
		})
	}
}

// laneCase is what a moment's lanes are built from, named.
type laneCase struct {
	name string
	from views.LanesFrom
}

// agreementCases are the lanes the agreement test builds: of each hard
// moment, in either view, its readings those its document read; and of each
// moment Runway has to get right that holds now.
func agreementCases() []laneCase {
	var cases []laneCase
	for _, hm := range statustest.HardMoments(claude.Policy) {
		m := hm.Doc.At(hm.Now, claude.Policy)
		var read []readings.Reading
		for _, a := range hm.Doc.Accounts {
			if !a.FetchedAt.IsZero() {
				read = append(read, readings.Of(a.ID, a.Windows, a.FetchedAt, readings.FromAnswer)...)
			}
		}
		slices.SortStableFunc(read, func(a, b readings.Reading) int { return a.At.Compare(b.At) })
		for _, w := range []views.RunwayWindow{views.RunwayFiveHour, views.RunwayWeek} {
			from := views.LanesFrom{Moment: m, Readings: read, Timeline: views.LiveTimeline(w, hm.Now, 136), Window: w}
			cases = append(cases, laneCase{name: hm.Name + ", the " + viewKeys[w] + " view", from: from})
		}
	}
	for _, rm := range viewstest.RunwayMoments() {
		if from := lanesFrom(rm); from.Timeline.NowColumn() >= 0 {
			cases = append(cases, laneCase{name: rm.Name, from: from})
		}
	}
	return cases
}

// viewKeys are the key of the window each view is named for.
var viewKeys = map[views.RunwayWindow]string{views.RunwayFiveHour: claude.FiveHourWindow, views.RunwayWeek: claude.WeekWindow}

// asRouted is the cell an account routed as a is drawn in now: held back
// from every request by a limit, a refusal of its token or its cap, as its
// kind; without a usable token; blank where nothing's read of it; else with
// room.
func asRouted(a status.AccountNow) views.Cell {
	switch a.Routing.Kind {
	case status.RoutingTokenless:
		return views.CellTokenless
	case status.RoutingLimited:
		return views.CellLimited
	case status.RoutingRefused:
		return views.CellRefused
	case status.RoutingUnreadable, status.RoutingUnread:
		return views.CellUnknown
	case status.RoutingCapped:
		return views.CellCapped
	}
	return views.CellRoom
}

// read reports whether usage gives the window w a use: it's read, of an
// account with a usable token.
func read(w status.WindowNow) bool {
	return w.Kind != status.WindowTokenless && w.Kind != status.WindowUnread
}

// heldTill is when the moment has room return to the account a, held back
// as the cell given: its refusal's until, else its routing's back.
func heldTill(a status.AccountNow, held views.Cell) time.Time {
	if held == views.CellRefused {
		return a.Refusal.Until
	}
	return a.Routing.Back
}

// carriedOn reports whether a window of the account a that runs out ahead,
// by back, when the moment has room return, resets at the back given,
// carrying the hold on to it.
func carriedOn(a status.AccountNow, back, carried time.Time) bool {
	return slices.ContainsFunc(a.Windows, func(w status.WindowNow) bool {
		return w.RunsOutAhead && !w.Out.At.After(back) && w.Next.Equal(carried)
	})
}

// poolOf is how many accounts the moment's pool has, none where it has one
// account, which has no pool's lane.
func poolOf(m status.Moment) int {
	if len(m.Accounts) < 2 {
		return 0
	}
	return len(m.Pool().Accounts)
}

// inOrderAmong reports whether each of some is among all, in all's order.
func inOrderAmong(some, all []status.Upcoming) bool {
	i := 0
	for _, u := range some {
		for i < len(all) && all[i] != u {
			i++
		}
		if i == len(all) {
			return false
		}
		i++
	}
	return true
}

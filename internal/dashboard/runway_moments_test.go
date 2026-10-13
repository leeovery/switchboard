package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// At each hard moment, Runway's lanes, its badges and its weeks' resets, and
// GRAPHS' spans, draw each account as its moment has it, and agree with the
// one verdict of how it stands.
func TestRunwayAndGRAPHSAgreeWithTheMomentAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claudeLike) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, claudeLike)
			for _, a := range m.Accounts {
				checkLaneAgrees(t, a, hm.Now)
				checkWeeksAgree(t, a, hm.Now)
				checkChartsAgree(t, a, hm.Now)
			}
		})
	}
}

// routingHolds reports whether ROUTING's verdict k holds an account back from
// every request.
func routingHolds(k status.RoutingKind) bool {
	switch k {
	case status.RoutingTokenless, status.RoutingLimited, status.RoutingRefused, status.RoutingUnreadable, status.RoutingCapped:
		return true
	default:
		return false
	}
}

// checkLaneAgrees checks account a's lane over the day, at now: blank where
// ROUTING says nothing's read of it; without room where ROUTING holds it back
// from every request, or, as D7 keeps it, its last read failed; draining
// where a window every model shares runs out, as its RunsOut says; else with
// room. Each window counted holds it back as its facts say: its limit until
// it's back, its reserve as read until it resets as read, and from where it
// runs out, as its Out has it, while one stale runs out nowhere (D2).
func checkLaneAgrees(t *testing.T, a status.AccountNow, now time.Time) {
	t.Helper()
	l := Frame{Policy: claudeLike, Span: Day}.laneOf(a, now)
	want := roomOpen
	switch {
	case a.Routing.Kind == status.RoutingUnread:
		want = roomUnknown
	case routingHolds(a.Routing.Kind) || a.Error != "":
		want = roomNone
	case slices.ContainsFunc(a.Windows, func(w status.WindowNow) bool { return w.Shared && w.RunsOut }):
		want = roomDraining
	}
	if got := l.at(now, now); got != want {
		t.Errorf("%s's lane stands %v at now, want %v, as ROUTING has it %v", a.ID, got, want, a.Routing.Kind)
	}
	if !l.known || a.Error != "" {
		return
	}
	ends := func(until time.Time) bool {
		return slices.ContainsFunc(l.stretches, func(s stretch) bool {
			return slices.ContainsFunc(s.causes, func(c cause) bool { return c.until.Equal(until) })
		})
	}
	for _, w := range l.counted {
		switch {
		case w.Held && (!ends(w.Back) || l.at(now, now) != roomNone):
			t.Errorf("%s's %s is held till %v, but its lane has %+v", a.ID, w.Key, w.Back, l.stretches)
		case w.CappedAsRead && !ends(w.Read.ResetsAt):
			t.Errorf("%s's %s was at its reserve as read, but its lane has %+v", a.ID, w.Key, l.stretches)
		case w.RunsOut && (l.at(w.Out.At, now) != roomNone || l.drains.Before(w.Out.At)):
			t.Errorf("%s's %s runs out at %v, but its lane has %+v, draining till %v", a.ID, w.Key, w.Out.At, l.stretches, l.drains)
		case w.Kind == status.WindowStale && !w.Out.At.IsZero() && l.at(w.Out.At, now) != roomOpen:
			t.Errorf("%s's %s, stale, runs out nowhere, but its lane has %+v", a.ID, w.Key, l.stretches)
		}
	}
	if a.TokenRefused && !ends(a.Refusal.Until) {
		t.Errorf("%s's token is refused till %v, but its lane has %+v", a.ID, a.Refusal.Until, l.stretches)
	}
}

// checkWeeksAgree checks account a's badges at now: over the day, ▲ where
// it's next, as its moment has it, and anything is known of it; over the
// week, its week's use, as it stands, in attention where USAGE says it runs
// out, its Kind WindowRunsOut, in destructive at its limit; and its weeks'
// resets, as their Next has them, with what they're heading for where their
// Projected says.
func checkWeeksAgree(t *testing.T, a status.AccountNow, now time.Time) {
	t.Helper()
	day, week := Frame{Policy: claudeLike, Span: Day}, Frame{Policy: claudeLike, Span: Week}
	if got, want := len(day.badge(day.laneOf(a, now))) > 0, a.Next && a.Routing.Kind != status.RoutingUnread; got != want {
		t.Errorf("%s has ▲ over the day: %v, want %v", a.ID, got, want)
	}
	l := week.laneOf(a, now)
	weeks := week.weeks(l)
	if len(weeks) == 0 {
		return
	}
	w := weeks[0]
	wantInk := strongInk
	switch w.Kind {
	case status.WindowLimited:
		wantInk = exhaustedInk
	case status.WindowRunsOut:
		wantInk = alertInk
	}
	if got, want := week.badge(l), (line{{status.Percent(w.Used), wantInk}}); !slices.Equal(got, want) {
		t.Errorf("%s's week's badge is %+v, want %+v, as its week stands as kind %v", a.ID, got, want, w.Kind)
	}
	for _, w := range weeks {
		said := resetSays(w, now).plain()
		if !strings.HasPrefix(said, "resets "+status.Dated(now, w.Next)) || strings.HasSuffix(said, "used by then") != (w.Projected != nil) {
			t.Errorf("%s's %s's reset says %q, its next reset at %v, heading for %v", a.ID, w.Key, said, w.Next, w.Projected)
		}
	}
}

// checkChartsAgree checks account a's charts at now: each over its window's
// span as it stands, ending where its next reset is, as Runway marks it; none
// of a window not running; each with its account's cap.
func checkChartsAgree(t *testing.T, a status.AccountNow, now time.Time) {
	t.Helper()
	for _, w := range a.Windows {
		ch := chartOf(a, shownWindow{key: w.Key, name: w.Name}, Trail{}, now)
		if ch.spanned != (w.Kind != status.WindowNotStarted) || ch.spanned && !ch.start.Add(ch.length).Equal(w.Next) {
			t.Errorf("%s's %s is charted from %v for %v, spanned %v, want it to its next reset, %v, where it's running, as its kind %v says", a.ID, w.Key, ch.start, ch.length, ch.spanned, w.Next, w.Kind)
		}
		if ch.capAt != a.Cap {
			t.Errorf("%s's %s is charted capped at %v, want %v", a.ID, w.Key, ch.capAt, a.Cap)
		}
	}
}

package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// standing is how an account's window stands at a moment, as its card shows
// it: where it's heading, where it runs out, and whether it's at its limit or
// has lapsed.
type standing struct {
	window quota.Window
	// heading is where it's heading, as Project says; and out where it runs
	// out before it resets, as RunsOut says, runsOut set where it does.
	heading status.Heading
	out     status.RunOut
	runsOut bool
	// held is set while it's at its limit, its account held back, until
	// back, zero where that's unknown.
	held bool
	back time.Time
	// lapsed is set once it has lapsed: it isn't running, and reads empty,
	// until a request starts it.
	lapsed bool
	// floor is the share of it used where its account runs out: where its
	// reserve starts, where that holds the account back, else its limit.
	floor float64
}

// standingOf is how doc's account a's window w stands at now, as policy says
// which windows every model shares.
func standingOf(doc status.Document, a status.Account, w quota.Window, now time.Time, policy score.Policy) standing {
	s := standing{window: w, heading: a.Project(w, now), lapsed: a.HasLapsed(w), floor: 1}
	s.out, s.runsOut = doc.RunsOut(a, w, now)
	if doc.ReserveHolds(a) {
		s.floor = 1 - a.Reserve
	}
	if held, ok := a.Held(now, policy); ok && slices.Contains(held.Windows, w.Key) {
		s.held, s.back = true, held.Until
	} else if s.heading.Kind == score.Exhausted {
		s.held, s.back = true, w.ResetsAt
	}
	return s
}

// projected is the share of the window it's heading for by its reset, as a
// bar shows it beyond its use: all of it where it runs out; reporting false
// where nothing says, and while it's held at its limit or has lapsed.
func (s standing) projected() (float64, bool) {
	switch {
	case s.held || s.lapsed:
		return 0, false
	case s.heading.Kind == score.RunsOut:
		return 1, true
	case s.heading.Kind == score.OnPace:
		return s.heading.AtReset, true
	default:
		return 0, false
	}
}

// nearing reports whether the share the window is heading for by its reset,
// short of where its account runs out, comes within 10 points of it.
func (s standing) nearing() bool {
	share, ok := s.projected()
	return ok && !s.runsOut && share >= s.floor-status.Near-score.Tolerance
}

// use is how much of the window is used, bold: in the ink given, but at its
// limit, destructive, and once it has lapsed, dim.
func (s standing) use(k ink) span {
	switch {
	case s.held:
		k = ink{token: theme.StateDestructive}
	case s.lapsed:
		k = dimInk
	}
	k.bold = true
	return span{status.Percent(s.window.Utilization), k}
}

// whither says in brief where the window is heading at now, as a bar line or
// a card's header does: that it's back at a time, at its limit; that it runs
// out at a time, as in "→ out ~16:05", with tilde as given; that it's heading
// for a share by its reset, as in "→ 87%", calling for attention where that
// nears where its account runs out; or that it hasn't started. It's empty
// where nothing says.
func (s standing) whither(now time.Time, tilde string) span {
	switch {
	case s.lapsed:
		return span{"not started", dimInk}
	case s.held && !s.back.IsZero():
		return span{"back " + brief(now, s.back), exhaustedInk}
	case s.held:
		return span{}
	case s.runsOut:
		return span{"→ out " + tilde + brief(now, s.out.At), alertInk}
	}
	share, ok := s.projected()
	switch {
	case !ok:
		return span{}
	case s.nearing():
		return span{"→ " + status.Percent(share), warningInk}
	default:
		return span{"→ " + status.Percent(share), mutedInk}
	}
}

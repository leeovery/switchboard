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
	// account is the id of the account whose window it is.
	account string
	window  quota.Window
	// heading is where it's heading, as doc's Project says; and out where it
	// runs out before it resets, as RunsOut says, runsOut set where it does.
	heading status.Heading
	out     status.RunOut
	runsOut bool
	// held is set while it's at its limit, its account held back, until
	// back, zero where that's unknown; and since when, where the router told
	// of its reaching the limit that holds it, zero otherwise.
	held        bool
	back, since time.Time
	// lapsed is set once it has lapsed, while it isn't held: it isn't
	// running, and reads empty, until a request starts it.
	lapsed bool
	// floor is the share of it used where its account runs out: where its
	// reserve starts, where that holds the account back, else its limit.
	floor float64
}

// standingOf is how doc's account a's window w stands at now, as policy says
// which windows every model shares. Held at its limit, it's held until the
// hold lifts off it, as Lifts says, and not lapsed, as no request can start
// it then.
func standingOf(doc status.Document, a status.Account, w quota.Window, now time.Time, policy score.Policy) standing {
	s := standing{account: a.ID, window: w, heading: doc.Project(a, w, now), floor: doc.Floor(a)}
	s.out, s.runsOut = doc.RunsOut(a, w, now)
	if held, ok := a.Held(now, policy); ok && held.Holds(w.Key) {
		s.held, s.back, s.since = true, held.Lifts(w.Key), limitedSince(doc, a, w, now)
	}
	s.lapsed = !s.held && a.HasLapsed(w)
	return s
}

// limitedSince is when doc's account a reached the limit that holds its
// window w at now, as reachedIn tells of it: zero where it tells of none.
func limitedSince(doc status.Document, a status.Account, w quota.Window, now time.Time) time.Time {
	e, _ := reachedIn(doc, a, w, now)
	return e.At
}

// reachedIn is the router's event of doc's account a reaching the limit that
// holds its window w at now, reporting false where none tells of it. Where
// the account's limit holds w, it's that limit's event, as limitOf finds it.
// Where the account has no such limit, as a probed document has none, or one
// without an identity, as a router from before limits had them gives, it's
// the newest event of the account reaching a limit naming w, or naming no
// window, as a limit naming none holds every window, that came while w runs
// as it does now, since it last started: an older limit's event says nothing
// of when w reached it.
func reachedIn(doc status.Document, a status.Account, w quota.Window, now time.Time) (status.Event, bool) {
	if l := a.Limit; l.ID != 0 && l.Holds(now) && (len(l.Windows) == 0 || slices.Contains(l.Windows, w.Key)) {
		return limitOf(doc, a, now)
	}
	start, _, spanned := w.Span()
	if !spanned {
		return status.Event{}, false
	}
	i := slices.IndexFunc(doc.Events, func(e status.Event) bool {
		named := len(e.Windows) == 0 || slices.Contains(e.Windows, w.Key)
		return e.Kind == status.EventLimit && e.Account == a.ID && named && !e.At.Before(start) && !e.At.After(now)
	})
	if i < 0 {
		return status.Event{}, false
	}
	return doc.Events[i], true
}

// limitOf is the router's event of the limit holding doc's account a at now:
// the one whose identity is its limit's. It reports false where no router
// limit holds the account, as none holds one probed, where its limit has no
// identity, as a router from before limits had them gives none, or where no
// event tells of it.
func limitOf(doc status.Document, a status.Account, now time.Time) (status.Event, bool) {
	if a.Limit.ID == 0 || !a.Limit.Holds(now) {
		return status.Event{}, false
	}
	i := slices.IndexFunc(doc.Events, func(e status.Event) bool {
		return e.Kind == status.EventLimit && e.Account == a.ID && e.Limit == a.Limit.ID
	})
	if i < 0 {
		return status.Event{}, false
	}
	return doc.Events[i], true
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

// paceCell is the cell of the window's bar, cells long, where even use
// across the window would put it at now: noMarker, leaving the marker off,
// where that can't be said, and while it's held at its limit or has lapsed.
func (s standing) paceCell(now time.Time, cells int) int {
	elapsed, ok := score.Elapsed(s.window, now)
	if !ok || s.held || s.lapsed {
		return noMarker
	}
	return cellAt(elapsed, cells)
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
// a card's header does, its times as brief shows them, as headed says it.
func (s standing) whither(now time.Time, tilde string) span {
	return s.headed(now, tilde, brief)
}

// headed says where the window is heading at now, its times as when shows
// them: that it's back at a time, at its limit; that it runs out at a time,
// as in "→ out ~16:05", with tilde as given; that it's heading for a share
// by its reset, as in "→ 87%", calling for attention where that nears where
// its account runs out; or that it hasn't started. It's empty where nothing
// says.
func (s standing) headed(now time.Time, tilde string, when func(now, t time.Time) string) span {
	switch {
	case s.lapsed:
		return span{"not started", dimInk}
	case s.held && !s.back.IsZero():
		return span{"back " + when(now, s.back), exhaustedInk}
	case s.held:
		return span{}
	case s.runsOut:
		return span{"→ out " + tilde + when(now, s.out.At), alertInk}
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

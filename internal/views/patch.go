package views

import (
	"maps"
	"slices"

	"github.com/leeovery/switchboard/internal/status"
)

// Patched is who sits where on Sessions' Switchboard, kept from look to look
// as Patch keeps it: on each account's line, its calls in their places, each
// plugged into its jack, and the jacks the cords a limit let go hang from.
// The zero Patched is the board before its first look. What changes it
// returns afresh, so a copy holding it never sees it change.
type Patched struct {
	lines map[string]PatchedLine
}

// PatchedLine is an account's line as the patch keeps it: its calls, top to
// bottom, each on its jack; the jacks its stubs hang from, the first hung
// first, as each is a cell shorter than the one before; and the sessions
// that moved elsewhere, each on the jack it left, the last to leave last,
// that nothing has taken since, the jack a cord a move let go hangs loose
// from. A stub the line's limit counts later hangs from the jack its session
// left, as the request stream may tell of the move before the router's
// document counts it. Jacks are counted down the line from its first, 0; one
// past its rows grows it.
type PatchedLine struct {
	Calls []Plugged
	Stubs []int
	Left  []Plugged
}

// Plugged is a call as the patch keeps it: its session, by its id, and the
// jack its cord ends at.
type Plugged struct {
	Session string
	Jack    int
}

// Line is the line of the account with the given id, as p keeps it: none
// where no call is on it, nor stub hangs from it. What it holds is the
// caller's.
func (p Patched) Line(account string) PatchedLine {
	l := p.lines[account]
	return PatchedLine{Calls: slices.Clone(l.Calls), Stubs: slices.Clone(l.Stubs), Left: slices.Clone(l.Left)}
}

// StubsOf are the stubs each account's line hangs at the moment m, by its
// id, as Patch takes them: a stub for each session its limit moved off it,
// while the limit holds, as AccountNow.LimitMoved counts them; none for an
// account whose limit moved none.
func StubsOf(m status.Moment) map[string]int {
	stubs := make(map[string]int)
	for _, a := range m.Accounts {
		if a.LimitMoved > 0 {
			stubs[a.ID] = a.LimitMoved
		}
	}
	return stubs
}

// Patch is the board kept, as a look finds the calls, as CallsOf has them,
// in RunningOf's order, and each line's stubs, by its account's id, as
// StubsOf has them. Each line's calls keep their places and their jacks,
// those whose sessions no longer run on it gone, those below closing up; and
// a session new to it, or one a move brings, joins the foot, in calls'
// order, plugged into the line's first free jack. A stub hangs from a jack a
// session that moved elsewhere left, the jacks the last to leave left taken
// first, else from the line's first free jack, once the sessions new to it
// have taken theirs; a jack a stub hangs from is never free, and the stubs
// go as the limit lifts. At the first look, kept is the zero Patched, so
// every call joins in calls' order, the one seen last first.
func Patch(kept Patched, calls []Call, stubs map[string]int) Patched {
	on := make(map[string][]string)
	goes := make(map[string][]string)
	for _, c := range calls {
		on[c.Account] = append(on[c.Account], c.Session)
		goes[c.Session] = append(goes[c.Session], c.Account)
	}
	patched := Patched{lines: make(map[string]PatchedLine)}
	accounts := slices.Concat(slices.Collect(maps.Keys(kept.lines)), slices.Collect(maps.Keys(on)), slices.Collect(maps.Keys(stubs)))
	for _, account := range slices.Compact(slices.Sorted(slices.Values(accounts))) {
		elsewhere := func(session string) bool {
			return slices.ContainsFunc(goes[session], func(a string) bool { return a != account })
		}
		l := kept.lines[account].patched(on[account], stubs[account], elsewhere)
		if len(l.Calls) > 0 || len(l.Stubs) > 0 || len(l.Left) > 0 {
			patched.lines[account] = l
		}
	}
	return patched
}

// patched is the line l as a look finds the sessions on its account, in
// calls' order, its limit having moved stubs sessions off it, elsewhere
// telling of a session gone from it whether it runs on another account now.
func (l PatchedLine) patched(on []string, stubs int, elsewhere func(session string) bool) PatchedLine {
	next := PatchedLine{Left: slices.Clone(l.Left)}
	for _, c := range l.Calls {
		switch {
		case slices.Contains(on, c.Session):
			next.Calls = append(next.Calls, c)
		case elsewhere(c.Session):
			next.Left = append(next.Left, c)
		}
	}
	next.Stubs = slices.Clone(l.Stubs[:min(len(l.Stubs), stubs)])
	if hung := min(stubs-len(next.Stubs), len(next.Left)); hung > 0 {
		cut := len(next.Left) - hung
		for _, c := range next.Left[cut:] {
			next.Stubs = append(next.Stubs, c.Jack)
		}
		next.Left = next.Left[:cut]
	}
	for _, session := range on {
		if !slices.ContainsFunc(next.Calls, func(c Plugged) bool { return c.Session == session }) {
			next.Calls = append(next.Calls, Plugged{Session: session, Jack: next.free()})
		}
	}
	for len(next.Stubs) < stubs {
		next.Stubs = append(next.Stubs, next.free())
	}
	next.Left = slices.DeleteFunc(next.Left, func(c Plugged) bool {
		return next.taken(c.Jack) || slices.ContainsFunc(next.Calls, func(on Plugged) bool { return on.Session == c.Session })
	})
	return next
}

// free is the line's first jack no call is plugged into, nor stub hangs
// from.
func (l PatchedLine) free() int {
	jack := 0
	for l.taken(jack) {
		jack++
	}
	return jack
}

// taken reports whether a call is plugged into the jack, or a stub hangs
// from it.
func (l PatchedLine) taken(jack int) bool {
	return slices.Contains(l.Stubs, jack) || slices.ContainsFunc(l.Calls, func(c Plugged) bool { return c.Jack == jack })
}

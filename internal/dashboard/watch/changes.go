package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// changes is what the watch knows of the accounts' states and their
// windows' words: where the last look read them from, and the cues on each
// account's state and each window's words that looks saw change.
type changes struct {
	from   origin
	states cues[string, state]
	outs   cues[dashboard.Ref, bool]
}

// cues are what was last found of each of a kind of thing, by its key, and
// when each a look saw turn as its kind is cued was seen to, while it's
// picked out.
type cues[K comparable, V comparable] struct {
	found map[K]V
	since map[K]time.Time
}

// seen takes in what's found at now: each found before that turned since, as
// turned says of what it was and is, is picked out from now, and each
// already picked out stays so while it's picked out still.
func (c cues[K, V]) seen(found map[K]V, now time.Time, turned func(was, is V) bool) cues[K, V] {
	since := lasting(c.since, now, highlightFor)
	for key, is := range found {
		if was, ok := c.found[key]; ok && turned(was, is) {
			since[key] = now
		}
	}
	return cues[K, V]{found: found, since: since}
}

// looked takes in doc, read at now from the router whose health check said
// from, or by probing, as found says. A look that doesn't follow on from the
// last, as follows says, picks out nothing: what it finds starts afresh.
func (c changes) looked(doc status.Document, from router.Health, now time.Time, policy score.Policy) changes {
	read := originOf(doc, from)
	if !read.follows(c.from) {
		c = changes{}
	}
	c.from = read
	return c.found(doc, now, policy)
}

// ticked takes in doc at now, as found says, as the clock moves on with no
// read, as a limit lifting at its reset moves an account's state on, or a
// window's use heading past its floor before it resets has its words turn to
// out ~. Before anything is read, there's nothing to tell apart.
func (c changes) ticked(doc status.Document, now time.Time, policy score.Policy) changes {
	if !c.from.read {
		return c
	}
	return c.found(doc, now, policy)
}

// found takes in doc's accounts' states at now, as policy judges them, and
// whether its windows' words say they run out: each account whose state
// differs from what was found last is picked out from now, and each window
// whose words turn to out ~.
func (c changes) found(doc status.Document, now time.Time, policy score.Policy) changes {
	c.states = c.states.seen(statesOf(doc, now, policy), now, func(was, is state) bool { return was != is })
	c.outs = c.outs.seen(outsOf(doc, now, policy), now, func(was, is bool) bool { return is && !was })
	return c
}

// state is an account's state as its cue tells states apart: its kind,
// which its glyph and colour follow, and the first words ROUTING says it in,
// without the clauses after them, whose times move on with nothing changed;
// nor whether new sessions come to it, or the pin spends its cap, which the
// page marks besides.
type state struct {
	kind status.RoutingKind
	says string
}

// stateOf is the state routing says, as its cue tells states apart: an
// account new sessions come to is open, and one the pin spends the cap of
// is at its cap.
func stateOf(routing status.Routing) state {
	kind := routing.Kind
	switch kind {
	case status.RoutingNext:
		kind = status.RoutingOpen
	case status.RoutingCappedPinned:
		kind = status.RoutingCapped
	}
	return state{kind: kind, says: routing.Says}
}

// statesOf are the states of doc's accounts at now, as policy judges them,
// by id, as stateOf tells them apart.
func statesOf(doc status.Document, now time.Time, policy score.Policy) map[string]state {
	states := make(map[string]state, len(doc.Accounts))
	for _, a := range doc.Accounts {
		states[a.ID] = stateOf(doc.RoutingOf(a, now, policy))
	}
	return states
}

// outsOf are whether the words of each window of doc's accounts say at now,
// as policy judges it, that it runs out, out ~…, by its account and key.
func outsOf(doc status.Document, now time.Time, policy score.Policy) map[dashboard.Ref]bool {
	outs := make(map[dashboard.Ref]bool)
	for _, a := range doc.Accounts {
		states := doc.WindowStatesOf(a, now, policy)
		for _, w := range a.Windows {
			outs[dashboard.Ref{Account: a.ID, Window: w.Key}] = states.Of(w.Key).Kind == status.WindowRunsOut
		}
	}
	return outs
}

// faded is how far the cue on each account's state picked out at now has
// faded, by its account's id, as fading says over highlightFor.
func (c changes) faded(now time.Time) map[string]float64 {
	return fading(c.states.since, now, highlightFor)
}

// turnedOut is how far the cue on each window's words picked out at now has
// faded, by its account and key, as fading says over highlightFor.
func (c changes) turnedOut(now time.Time) map[dashboard.Ref]float64 {
	return fading(c.outs.since, now, highlightFor)
}

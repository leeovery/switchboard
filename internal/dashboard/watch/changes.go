package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// changes is what the watch knows of the accounts' states: where the last
// look read them from, each account's as that look found it, and when each
// that a look saw change was seen to, while it's picked out.
type changes struct {
	// read is set once a document has been read; routed whether it was the
	// router's, and router which router's, as its health check said.
	read   bool
	routed bool
	router router.Health
	states map[string]string
	since  map[string]time.Time
}

// looked takes in doc, read at now from the router whose health check said
// from, or by probing, its accounts' states as policy judges them: each
// account's that differs from what the last look found is picked out from
// now. The first look picks out none, there being no look before it, and
// nor does one read from elsewhere than the last, the router after probing
// or probing after it, or another router, as one restarted: their states
// start afresh.
func (c changes) looked(doc status.Document, from router.Health, now time.Time, policy score.Policy) changes {
	states := statesOf(doc, now, policy)
	since := make(map[string]time.Time)
	if c.read && c.routed == routed(doc) && (!c.routed || from.Same(c.router)) {
		since = c.changed(states, now)
	}
	return changes{read: true, routed: routed(doc), router: from, states: states, since: since}
}

// ticked takes in the states of doc's accounts as policy judges them at now,
// as the clock moves on with no read, as a limit lifting at its reset moves
// one on: each that differs from what was last found is picked out from now.
// Before anything is read, there's nothing to tell apart.
func (c changes) ticked(doc status.Document, now time.Time, policy score.Policy) changes {
	if !c.read {
		return c
	}
	states := statesOf(doc, now, policy)
	c.since, c.states = c.changed(states, now), states
	return c
}

// changed are when each account picked out was seen to change at now, of
// states as found against those found last: those already picked out while
// they're picked out still, and those whose states differ, from now.
func (c changes) changed(states map[string]string, now time.Time) map[string]time.Time {
	since := make(map[string]time.Time)
	for id, at := range c.since {
		if now.Sub(at) < highlightFor {
			since[id] = at
		}
	}
	for id, s := range states {
		if was, ok := c.states[id]; ok && was != s {
			since[id] = now
		}
	}
	return since
}

// statesOf are the states of doc's accounts at now, as policy judges them,
// by id: the first words ROUTING says each in, without the clauses after
// them, whose times move on with nothing changed; nor whether new sessions
// come to it, or the pin spends its cap, which the page marks besides.
func statesOf(doc status.Document, now time.Time, policy score.Policy) map[string]string {
	states := make(map[string]string, len(doc.Accounts))
	for _, a := range doc.Accounts {
		states[a.ID] = doc.RoutingOf(a, now, policy).Says
	}
	return states
}

// faded is how far the cue on each account's state picked out at now has
// faded, by its account's id, as fading says.
func (c changes) faded(now time.Time) map[string]float64 {
	return fading(c.since, now)
}

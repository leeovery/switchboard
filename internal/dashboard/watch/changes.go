package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// changes is what the watch knows of the cards' states: where the last look
// read them from, each account's as that look found it, and when each that a
// look saw change was seen to, while it's picked out.
type changes struct {
	// read is set once a document has been read; routed whether it was the
	// router's, and router which router's, as its health check said.
	read   bool
	routed bool
	router router.Health
	states map[string]cardState
	since  map[string]time.Time
}

// cardState is an account's state as a card says it, for telling whether it
// changed: its condition, and what holds it back or what it's doing, without
// what that means, whose countdowns move on with nothing changed.
type cardState struct {
	condition status.Condition
	says      string
}

// looked takes in doc, read at now from the router whose health check said
// from, or by probing, its accounts' states as policy judges them: each
// account's that differs from what the last look found is picked out from
// now. The first look picks out none, there being no look before it, and
// nor does one read from elsewhere than the last, the router after probing
// or probing after it, or another router, as one restarted: their states
// start afresh.
func (c changes) looked(doc status.Document, from router.Health, now time.Time, policy score.Policy) changes {
	states := make(map[string]cardState, len(doc.Accounts))
	for _, a := range doc.Accounts {
		s := doc.StateOf(a, now, policy)
		states[a.ID] = cardState{condition: s.Condition, says: s.Says}
	}
	since := make(map[string]time.Time)
	if c.read && c.routed == routed(doc) && (!c.routed || from.Same(c.router)) {
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
	}
	return changes{read: true, routed: routed(doc), router: from, states: states, since: since}
}

// faded is how far the highlight on each card picked out at now has faded,
// by its account's id, as fading says.
func (c changes) faded(now time.Time) map[string]float64 {
	return fading(c.since, now)
}

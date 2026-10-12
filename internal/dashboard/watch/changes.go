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
	// read is set once a document has been read; routed whether it was the
	// router's, and router which router's, as its health check said.
	read   bool
	routed bool
	router router.Health
	states cues[string, string]
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
// from, or by probing, as found says. The first look picks out nothing, there
// being no look before it, and nor does one read from elsewhere than the
// last, the router after probing or probing after it, or another router, as
// one restarted: what they find starts afresh.
func (c changes) looked(doc status.Document, from router.Health, now time.Time, policy score.Policy) changes {
	if !c.read || c.routed != routed(doc) || c.routed && !from.Same(c.router) {
		c = changes{}
	}
	c.read, c.routed, c.router = true, routed(doc), from
	return c.found(doc, now, policy)
}

// ticked takes in doc at now, as found says, as the clock moves on with no
// read, as a limit lifting at its reset moves an account's state on, or a
// window's use heading past its floor before it resets has its words turn to
// out ~. Before anything is read, there's nothing to tell apart.
func (c changes) ticked(doc status.Document, now time.Time, policy score.Policy) changes {
	if !c.read {
		return c
	}
	return c.found(doc, now, policy)
}

// found takes in doc's accounts' states at now, as policy judges them, and
// whether its windows' words say they run out: each account whose state
// differs from what was found last is picked out from now, and each window
// whose words turn to out ~.
func (c changes) found(doc status.Document, now time.Time, policy score.Policy) changes {
	c.states = c.states.seen(statesOf(doc, now, policy), now, func(was, is string) bool { return was != is })
	c.outs = c.outs.seen(outsOf(doc, now), now, func(was, is bool) bool { return is && !was })
	return c
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

// outsOf are whether the words of each window of doc's accounts say at now
// that it runs out, out ~…, by its account and key.
func outsOf(doc status.Document, now time.Time) map[dashboard.Ref]bool {
	outs := make(map[dashboard.Ref]bool)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			outs[dashboard.Ref{Account: a.ID, Window: w.Key}] = doc.WindowStateOf(a, w.Key, now).Kind == status.WindowRunsOut
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

// cueing reports whether any cue, on a state or on a window's words, is
// fading back at now.
func (c changes) cueing(now time.Time) bool {
	return c.faded(now) != nil || c.turnedOut(now) != nil
}

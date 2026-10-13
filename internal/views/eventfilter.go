package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/status"
)

// EventFilter is which events the Log's Events show, as its panel's ONLY
// ticks them: of each choice, a choice ticked keeps those ticked, and none
// ticked is every one.
type EventFilter struct {
	// Kinds are the kinds of event shown, as the router names them, as
	// status.EventLimit.
	Kinds []string
	// Accounts are the accounts whose events are shown, by id: an event
	// naming any of them, as LogEvent's Accounts names them.
	Accounts []string
	// Sessions are the sessions whose events are shown, by their whole ids.
	Sessions []string
}

// Keeps reports whether the filter shows the event e: of each choice, where
// any is ticked, e is of one ticked.
func (f EventFilter) Keeps(e LogEvent) bool {
	return ticked(f.Kinds, e.Kind) && tickedAny(f.Accounts, e.Accounts) && ticked(f.Sessions, e.Session)
}

// EventCount is what today's events count: every one, and those the filter
// shows; and every one by its kind, as the router names it, by each account
// it names, and by its session, by the session's whole id, an account's
// event counted under none.
type EventCount struct {
	All, Shown int
	Kinds      map[string]int
	Accounts   map[string]int
	Sessions   map[string]int
}

// Hidden is how many of today's events the filter hides.
func (c EventCount) Hidden() int {
	return c.All - c.Shown
}

// add counts the event e, shown where the filter keeps it.
func (c *EventCount) add(e LogEvent, shown bool) {
	if c.Kinds == nil {
		c.Kinds, c.Accounts, c.Sessions = make(map[string]int), make(map[string]int), make(map[string]int)
	}
	c.All++
	if shown {
		c.Shown++
	}
	c.Kinds[e.Kind]++
	for _, a := range e.Accounts {
		c.Accounts[a]++
	}
	if e.Session != "" {
		c.Sessions[e.Session]++
	}
}

// namedAccounts are the accounts the event e names, by id, each once, in the
// order it names them: a move's, from and to; the global pin's, set or
// cleared, those it names; any other's, the one it befell; none for a
// restart or the router's health. A limit's, a cap's or a refusal's are its
// own, never where its sessions went.
func namedAccounts(e status.Event) []string {
	var ids []string
	switch {
	case e.Kind == status.EventMoved:
		ids = []string{e.From, e.To}
	case (e.Kind == status.EventPin || e.Kind == status.EventAuto) && e.Session == "" && len(e.Accounts) > 0:
		ids = e.Accounts
	default:
		ids = []string{e.Account}
	}
	var named []string
	for _, id := range ids {
		if id != "" && !slices.Contains(named, id) {
			named = append(named, id)
		}
	}
	return named
}

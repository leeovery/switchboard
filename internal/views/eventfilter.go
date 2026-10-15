package views

import (
	"slices"
	"time"

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

// Equal reports whether f and o tick the same, each choice in the same
// order.
func (f EventFilter) Equal(o EventFilter) bool {
	return slices.Equal(f.Kinds, o.Kinds) && slices.Equal(f.Accounts, o.Accounts) && slices.Equal(f.Sessions, o.Sessions)
}

// Keeps reports whether the filter shows the event e: of each choice, where
// any is ticked, e is of one ticked.
func (f EventFilter) Keeps(e LogEvent) bool {
	return ticked(f.Kinds, e.Kind) && tickedAny(f.Accounts, e.Accounts) && ticked(f.Sessions, e.Session)
}

// Reach is the filter of the ledger's lines whose reach, as KeptFrom reads
// it, bounds how far back the Events read through f to fill their view, and
// whether one does: the lines of every kind counted toward the sessions f
// keeps the events of, as the router tells of a session only once a request
// of it has come; none where f keeps every session's. It bounds only their
// filling, as the router files a session's start and moves as it routes,
// before the line is written, which a request lost as the router goes away
// never has. No summary tells the days of an event's kind or account as the
// router tells of them: it tells of a limit no reading counts, as a refusal
// naming no window has it; of a move no line names, as one whose request is
// answered from an account it left; and of room again as a limit lapses, no
// window reset.
func (f EventFilter) Reach() (LogFilter, bool) {
	return LogFilter{Sessions: f.Sessions}, len(f.Sessions) > 0
}

// Bounds reports whether f keeps the events of some kinds, or naming some
// accounts, alone: whose days, as no summary tells them, the events' files
// are read for, as FiledFrom reads them, to bound how far back the Events
// read through f to fill their view.
func (f EventFilter) Bounds() bool {
	return len(f.Kinds) > 0 || len(f.Accounts) > 0
}

// FiledFrom is the date of the local day of the earliest event the router
// filed from from up to to that f keeps by its kind and the accounts it
// names, as Keeps has it, of every session, as Reach bounds those: "" where
// it filed none. It reads the files the oldest first, and no further than
// the first such event.
func (f EventFilter) FiledFrom(filed Events, from, to time.Time) string {
	if filed == nil {
		return ""
	}
	f.Sessions = nil
	for line := range filed.Between(from, to) {
		if f.Keeps(LogEvent{Kind: line.Kind, Session: line.Session, Accounts: namedAccounts(line.Event)}) {
			return Today(line.At).Date
		}
	}
	return ""
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

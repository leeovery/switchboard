package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// happening is something coming up: when, the account it befalls, by its
// name, what befalls it, in words that follow the name, and their ink.
type happening struct {
	at   time.Time
	name string
	what string
	ink  ink
}

// upcoming are the things coming up after now, soonest first, those at one
// time in the order of doc's accounts: for each account, its limit lifting,
// while one holds; but for one that holds back its every request, its
// session running out, where it does before it resets, as RunsOut has it,
// and its session resetting; and, from the router, each account's next
// prime.
func upcoming(doc status.Document, now time.Time, policy score.Policy) []happening {
	var all []happening
	for _, a := range doc.Accounts {
		all = append(all, coming(doc, a, now, policy)...)
	}
	for _, s := range doc.Prime.Slots {
		all = append(all, happening{at: s.Next, name: named(doc, s.Account), what: " is primed", ink: primedInk})
	}
	all = slices.DeleteFunc(all, func(h happening) bool { return !h.at.After(now) })
	slices.SortStableFunc(all, func(a, b happening) int { return a.at.Compare(b.at) })
	return all
}

// coming are the things coming up of doc's account a at now, as upcoming
// says, but for its prime. A limit of some models alone, reached in their
// own window, is named by that window, as in "back from its Fable week
// limit"; and a session running out where its reserve holds the account
// back reaches its reserve, rather than running out at its pace.
func coming(doc status.Document, a status.Account, now time.Time, policy score.Policy) []happening {
	var things []happening
	if a.Limit.Holds(now) {
		what := " back from its limit"
		if !limited(a, now, policy) {
			what = " back from its " + limits(status.Event{Account: a.ID, Windows: a.Limit.Windows}, doc)
		}
		things = append(things, happening{at: a.Limit.Until, name: name(a), what: what, ink: mutedInk})
	}
	w, ok := a.Window(policy.Started)
	if !ok || a.HasLapsed(w) || limited(a, now, policy) {
		return things
	}
	if out, ok := doc.RunsOut(a, w, now); ok {
		what := " runs out at its pace"
		if out.Reserve {
			what = " reaches its reserve"
		}
		things = append(things, happening{at: out.At, name: name(a), what: what, ink: warningInk})
	}
	return append(things, happening{at: w.ResetsAt, name: name(a), what: "'s " + prosed(w.Label) + " resets", ink: mutedInk})
}

// limited reports whether a limit holds the account back from every request
// at now: one reached in a window every model shares, as policy says, or in
// no window named.
func limited(a status.Account, now time.Time, policy score.Policy) bool {
	return a.Limit.Holds(now) && (len(a.Limit.Windows) == 0 || slices.ContainsFunc(a.Limit.Windows, policy.IsShared))
}

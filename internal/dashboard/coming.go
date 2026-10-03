package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
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
// time in the order of doc's accounts, as coming has them for each account,
// and, from the router, each account's next prime.
func upcoming(doc status.Document, now time.Time, policy score.Policy) []happening {
	shown := shownWindows(doc, now, policy)
	var all []happening
	for _, a := range doc.Accounts {
		all = append(all, coming(doc, a, now, policy, shown)...)
	}
	for _, s := range doc.Prime.Slots {
		all = append(all, happening{at: s.Next, name: named(doc, s.Account), what: " is primed", ink: primedInk})
	}
	all = slices.DeleteFunc(all, func(h happening) bool { return !h.at.After(now) })
	slices.SortStableFunc(all, func(a, b happening) int { return a.at.Compare(b.at) })
	return all
}

// coming are the things coming up of doc's account a at now, of the windows
// shown, as upcoming says, but for its prime: its limit lifting, while one
// holds it back, as Held says, a limit of some models alone named by its
// window, as in "back from its Fable week limit"; then, unless that holds it
// back from every request, each of its windows running out, where it does
// before it resets, as RunsOut has it, but one the limit holds, and its
// session resetting.
func coming(doc status.Document, a status.Account, now time.Time, policy score.Policy, shown []string) []happening {
	var things []happening
	held, isHeld := a.Held(now, policy)
	if isHeld {
		what := " back from its limit"
		if !held.Every {
			what = " back from its " + limits(status.Event{Account: a.ID, Windows: held.Windows}, doc)
		}
		things = append(things, happening{at: held.Until, name: name(a), what: what, ink: mutedInk})
		if held.Every {
			return things
		}
	}
	for _, key := range shown {
		w, ok := a.Window(key)
		if !ok || a.HasLapsed(w) || (isHeld && held.Holds(key)) {
			continue
		}
		if out, ok := doc.RunsOut(a, w, now); ok {
			things = append(things, happening{at: out.At, name: name(a), what: runningOut(w, out, policy), ink: warningInk})
		}
	}
	if w, ok := a.Window(policy.Started); ok && !a.HasLapsed(w) {
		things = append(things, happening{at: w.ResetsAt, name: name(a), what: "'s " + status.InProse(w.Label) + " resets", ink: mutedInk})
	}
	return things
}

// runningOut says what befalls an account as its window w runs out, as out
// says: it runs out at its pace, or reaches its reserve where that holds it
// back; as the window a request starts does, unnamed, as in "work runs out
// at its pace", or as any other, named, as in "client's week reaches its
// reserve".
func runningOut(w quota.Window, out status.RunOut, policy score.Policy) string {
	what := " runs out at its pace"
	if out.Reserve {
		what = " reaches its reserve"
	}
	if w.Key == policy.Started {
		return what
	}
	return "'s " + status.InProse(w.Label) + what
}

// limited reports whether a limit holds the account back from every request
// at now, as Held says.
func limited(a status.Account, now time.Time, policy score.Policy) bool {
	held, ok := a.Held(now, policy)
	return ok && held.Every
}

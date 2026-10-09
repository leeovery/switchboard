package dashboard

import (
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

// upcoming are the things coming up after now, as doc's COMING UP lists
// them, in its order, soonest first, each as happeningOf says it.
func upcoming(doc status.Document, now time.Time, policy score.Policy) []happening {
	var all []happening
	for _, u := range doc.ComingUp {
		if h, ok := happeningOf(doc, u, policy); ok && h.at.After(now) {
			all = append(all, h)
		}
	}
	return all
}

// happeningOf is what doc's COMING UP says comes of u: its account back from
// its limit, as in "back from its limit", or from one of some models alone,
// named by its window, as in "back from its Fable week limit", or from its
// reserve; a window running out, as runningOut says; a window resetting, as
// in "'s session resets"; or its being primed. It reports false for a kind
// it doesn't tell of, as one from a later router.
func happeningOf(doc status.Document, u status.Upcoming, policy score.Policy) (happening, bool) {
	h := happening{at: u.At, name: named(doc, u.Account), ink: mutedInk}
	switch u.Kind {
	case status.UpcomingBack:
		h.what = " back from its " + holder(doc, u)
	case status.UpcomingRunsOut:
		h.what, h.ink = runningOut(doc, u, policy), warningInk
	case status.UpcomingReset:
		h.what = "'s " + windowName(doc, u.Account, u.Window) + " resets"
	case status.UpcomingPrime:
		h.what, h.ink = " is primed", primedInk
	default:
		return happening{}, false
	}
	return h, true
}

// holder names what held back the account u is back from: its reserve,
// where u has it back from its cap, else its limit, each named by the window
// that held back some models' requests alone, as in "Fable week limit".
func holder(doc status.Document, u status.Upcoming) string {
	what := "limit"
	if u.Cap {
		what = "reserve"
	}
	if u.Window == "" {
		return what
	}
	return windowName(doc, u.Account, u.Window) + " " + what
}

// runningOut says what befalls an account as its window runs out, as u
// says: it runs out at its pace, or reaches its reserve where that holds it
// back; as the window a request starts does, unnamed, as in "work runs out
// at its pace", or as any other, named, as in "client's week reaches its
// reserve".
func runningOut(doc status.Document, u status.Upcoming, policy score.Policy) string {
	what := " runs out at its pace"
	if u.Cap {
		what = " reaches its reserve"
	}
	if u.Window == policy.Started {
		return what
	}
	return "'s " + windowName(doc, u.Account, u.Window) + what
}

// limited reports whether a limit holds the account back from every request
// at now, as Held says.
func limited(a status.Account, now time.Time, policy score.Policy) bool {
	held, ok := a.Held(now, policy)
	return ok && held.Every
}

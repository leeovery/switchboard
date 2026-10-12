package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// RunningWithin is how lately the router has routed a session's model on an
// account for the session to be running there: within the hour its cache
// lasts.
const RunningWithin = time.Hour

// Running is a session running on an account, as SESSIONS lists it: a row
// on each account its models go to, grouped by account in the config's
// order.
type Running struct {
	// Account is the account's id, and Session the session's.
	Account, Session string
	// Family is the family of its model seen last there, as "opus".
	Family string
	// LastSeen is when the router last saw it there.
	LastSeen time.Time
	// Why is why its model seen last there is there, in the views' words for
	// the router's reasons, with when, as "started here 14:41, the best".
	Why status.Words
	// Models are its models whose requests go there, the one seen last
	// first.
	Models []string
	// InFlight is what a request of its in flight there is doing, as the
	// router listed it: status.Answering while an answer streams to any,
	// else status.Asking while any waits, and "" while none is in flight.
	InFlight string
}

// RunningOf are the sessions running on doc's accounts at now, as the router
// lists them in sessions: a row on each account their models go to, those
// it routed there within RunningWithin, the accounts in the config's order,
// and on each, the session seen last first.
func RunningOf(doc status.Document, sessions []status.Session, now time.Time) []Running {
	var running []Running
	for _, a := range doc.Accounts {
		var on []Running
		for _, s := range sessions {
			if r, ok := runningOn(doc, s, a.ID, now); ok {
				on = append(on, r)
			}
		}
		slices.SortStableFunc(on, func(x, y Running) int { return y.LastSeen.Compare(x.LastSeen) })
		running = append(running, on...)
	}
	return running
}

// runningOn is the session s as it runs on the account with the given id at
// now, of doc, reporting false where none of its models runs there.
func runningOn(doc status.Document, s status.Session, id string, now time.Time) (Running, bool) {
	var there []status.Assignment
	for _, a := range s.Assignments {
		if a.Account == id && now.Sub(score.Later(a.LastSeen, a.AssignedAt)) <= RunningWithin {
			there = append(there, a)
		}
	}
	if len(there) == 0 {
		return Running{}, false
	}
	slices.SortStableFunc(there, func(x, y status.Assignment) int { return y.LastSeen.Compare(x.LastSeen) })
	last := there[0]
	r := Running{
		Account: id, Session: s.ID, Family: last.Name(), LastSeen: last.LastSeen,
		Why: whyRunning(doc, s, last, now), InFlight: inFlight(there),
	}
	for _, a := range there {
		r.Models = append(r.Models, a.Model)
	}
	return r, true
}

// whyRunning is why the session s's model is on the account its assignment
// a gives, at now, as status.RunningWords says it: since it was pinned
// there, where it was pinned as it ran, else since it went there; from the
// account doc's newest event moving it there tells, where one does.
func whyRunning(doc status.Document, s status.Session, a status.Assignment, now time.Time) status.Words {
	facts := status.Facts{To: a.Account, At: a.AssignedAt, Best: a.Account == doc.Best, Now: now}
	if a.Pinned && !a.PinnedAt.IsZero() {
		facts.At = a.PinnedAt
	}
	i := slices.IndexFunc(doc.Events, func(e status.Event) bool {
		return e.Kind == status.EventMoved && e.Session == s.ID && e.Model == a.Model && e.To == a.Account
	})
	if i >= 0 {
		facts.From = doc.Events[i].From
	}
	return status.RunningWords(a.Reason, facts)
}

// inFlight is what the assignments' requests in flight are doing, as the
// router listed them: status.Answering over status.Asking, and "" where none
// is in flight.
func inFlight(there []status.Assignment) string {
	doing := ""
	for _, a := range there {
		switch a.InFlight {
		case status.Answering:
			return status.Answering
		case status.Asking:
			doing = status.Asking
		}
	}
	return doing
}

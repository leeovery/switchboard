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
	// Why is why its model seen last there is there, in the views' words for
	// the router's reasons, with when, as "started here 14:41, the best".
	Why status.Words
	// Models are its models whose requests go there, the one seen last
	// first.
	Models []string
	// Activity is what its models there are doing, the busiest of them, and
	// when it was last seen at work there.
	Activity Activity
}

// RunningOf are the sessions running on the accounts at the moment m, of
// the sessions the router listed, as the request stream s has seen them
// since, as placed has them: a row on each account their models go to,
// those running there, as runsOn says, the accounts in the config's order,
// and on each, the session seen last first.
func RunningOf(m status.Moment, sessions []status.Session, s Stream) []Running {
	h := s.hearing()
	placed := s.placed(sessions, h)
	var running []Running
	for _, a := range m.Accounts {
		var on []Running
		for _, session := range placed {
			if r, ok := runningOn(m, session, a.ID, h); ok {
				on = append(on, r)
			}
		}
		slices.SortStableFunc(on, func(x, y Running) int { return y.Activity.LastSeen.Compare(x.Activity.LastSeen) })
		running = append(running, on...)
	}
	return running
}

// runsOn reports whether a session's model, as it's placed in a, runs on
// the account with the given id at now: it goes there, and was seen there,
// or put there, within RunningWithin.
func runsOn(a status.Assignment, id string, now time.Time) bool {
	return a.Account == id && now.Sub(score.Later(a.LastSeen, a.AssignedAt)) <= RunningWithin
}

// runningOn is the session s as it runs on the account with the given id at
// the moment m, as h hears its models there, reporting false where none of
// them runs there.
func runningOn(m status.Moment, s status.Session, id string, h hearing) (Running, bool) {
	there := slices.DeleteFunc(slices.Clone(s.Assignments), func(a status.Assignment) bool { return !runsOn(a, id, m.Now) })
	if len(there) == 0 {
		return Running{}, false
	}
	slices.SortStableFunc(there, func(x, y status.Assignment) int { return y.LastSeen.Compare(x.LastSeen) })
	last := there[0]
	r := Running{Account: id, Session: s.ID, Family: last.Name(), Why: whyRunning(m, s, last)}
	activities := make([]Activity, len(there))
	for i, a := range there {
		r.Models = append(r.Models, a.Model)
		activities[i] = h.seat(s.ID, a)
	}
	r.Activity = busiest(activities)
	return r, true
}

// placed are the sessions the router listed, as the stream s has seen them
// since, h hearing what it told of them: each model seen as late as the
// router saw it there, or the stream last told of a request of it there.
// The sessions given are left as they were.
func (s Stream) placed(sessions []status.Session, h hearing) []status.Session {
	placed := make([]status.Session, len(sessions))
	for i, session := range sessions {
		session.Assignments = slices.Clone(session.Assignments)
		for j, a := range session.Assignments {
			session.Assignments[j].LastSeen = score.Later(a.LastSeen, h.seats[seatOf{session: session.ID, model: a.Model, account: a.Account}].last)
		}
		placed[i] = session
	}
	return placed
}

// whyRunning is why the session s's model is on the account its assignment
// a gives, at the moment m, as status.RunningWords says it: since it was
// pinned there, where it was pinned as it ran, else since it went there; from
// the account m's newest event moving it there tells, where one does; the
// best while that account is where new sessions go, which ▲ marks.
func whyRunning(m status.Moment, s status.Session, a status.Assignment) status.Words {
	facts := status.Facts{To: a.Account, At: a.AssignedAt, Best: a.Account == m.Next, Now: m.Now}
	if a.Pinned && !a.PinnedAt.IsZero() {
		facts.At = a.PinnedAt
	}
	i := slices.IndexFunc(m.Events, func(e status.Event) bool {
		return e.Kind == status.EventMoved && e.Session == s.ID && e.Model == a.Model && e.To == a.Account
	})
	if i >= 0 {
		facts.From = m.Events[i].From
	}
	return status.RunningWords(a.Reason, facts)
}

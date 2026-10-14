package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// IdleHour is the hour a session goes without a request before the router
// lets it go, its cache gone cold: the router lists a session while it has
// routed it within the hour, one it no longer lists ended an hour after its
// last request, and one that asks again after an idle hour has its account
// chosen afresh, as resumed. It's the session's, as ledger.LongCache is the
// cache's.
const IdleHour = time.Hour

// Running is a session running on an account, as SESSIONS lists it: a row
// on each account its models go to, grouped by account in the config's
// order.
type Running struct {
	// Account is the account's id, and Session the session's.
	Account, Session string
	// Family is the family of its model named there, as "opus": the model
	// naming it, as the List names it from what the request stream told,
	// where that runs there, else its model seen last there.
	Family string
	// Why is why its model named there is there, in the views' words for the
	// router's reasons, with when, as "started here 14:41, the best".
	Why status.Words
	// Models are its models whose requests go there, the one seen last
	// first.
	Models []string
	// Activity is what its models there are doing, the busiest of them, and
	// when it was last seen at work there.
	Activity Activity
}

// RunningOf are the sessions running on the accounts at the moment m, of
// the sessions the router listed, as the request stream s has moved and seen
// them since, as placed has them: a row on each account their models go to,
// those running there, as runs says, the accounts in the config's order,
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

// RunningOn are the sessions of running, as RunningOf lists them, running on
// the account with the given id, in their order, the one seen last first:
// Detail's SESSIONS, the rows RunningCounts counts of it.
func RunningOn(running []Running, id string) []Running {
	var on []Running
	for _, r := range running {
		if r.Account == id {
			on = append(on, r)
		}
	}
	return on
}

// RunningCounts counts the sessions running on each account, by the
// account's id, as RunningOf lists them in running: a row each; none for an
// account none runs on. They're the counts ROUTING and the routing card give,
// so they never disagree with the rows SESSIONS lists.
func RunningCounts(running []Running) map[string]int {
	counts := make(map[string]int)
	for _, r := range running {
		counts[r.Account]++
	}
	return counts
}

// runs reports whether a session's model, as it's placed in a, runs on the
// account it goes to at now: it was seen there, or put there, within the
// IdleHour.
func runs(a status.Assignment, now time.Time) bool {
	return now.Sub(score.Later(a.LastSeen, a.AssignedAt)) <= IdleHour
}

// runningOn is the session s as it runs on the account with the given id at
// the moment m, as h hears its models there, reporting false where none of
// them runs there: named there by the model naming it, as names has it of
// what the stream told, where that runs there, else by its model seen last
// there.
func runningOn(m status.Moment, s status.Session, id string, h hearing) (Running, bool) {
	there := slices.DeleteFunc(slices.Clone(s.Assignments), func(a status.Assignment) bool { return a.Account != id || !runs(a, m.Now) })
	if len(there) == 0 {
		return Running{}, false
	}
	slices.SortStableFunc(there, func(x, y status.Assignment) int { return y.LastSeen.Compare(x.LastSeen) })
	named := there[0]
	_, model := naming{}.heard(h.told[s.ID]).names(s.Assignments)
	if i := slices.IndexFunc(there, func(a status.Assignment) bool { return a.Model == model }); i >= 0 {
		named = there[i]
	}
	r := Running{Account: id, Session: s.ID, Family: named.Name(), Why: whyRunning(m, s, named)}
	activities := make([]Activity, len(there))
	for i, a := range there {
		r.Models = append(r.Models, a.Model)
		activities[i] = h.seat(s.ID, a)
	}
	r.Activity = busiest(activities)
	return r, true
}

// placed are the sessions the router listed, as the stream s has moved and
// seen them since, h hearing what it told of them: each model the newest
// move of it took to another account put there, as brought has it, a
// session the router didn't list that a move brought on put first, as the
// one seen last; and each model seen as late as the router saw it there, or
// the stream last told of a request of it there. The sessions given are left
// as they were.
func (s Stream) placed(sessions []status.Session, h hearing) []status.Session {
	placed := make([]status.Session, len(sessions))
	for i, session := range sessions {
		session.Assignments = slices.Clone(session.Assignments)
		placed[i] = session
	}
	for _, m := range newestMoves(s.Moves) {
		brought := brought(sessions, m)
		i := slices.IndexFunc(placed, func(session status.Session) bool { return session.ID == m.Session })
		if i < 0 {
			placed = slices.Insert(placed, 0, status.Session{ID: m.Session, Assignments: []status.Assignment{brought}})
			continue
		}
		if j := slices.IndexFunc(placed[i].Assignments, func(a status.Assignment) bool { return a.Model == m.Model }); j >= 0 {
			placed[i].Assignments[j] = brought
		} else {
			placed[i].Assignments = append(placed[i].Assignments, brought)
		}
	}
	for _, session := range placed {
		for i, a := range session.Assignments {
			session.Assignments[i].LastSeen = score.Later(a.LastSeen, h.seats[seatOf{session: session.ID, model: a.Model, account: a.Account}].last)
		}
	}
	return placed
}

// newestMoves are the newest of the moves of each session's model, in the
// order they were told of.
func newestMoves(moves []Moved) []Moved {
	type model struct{ session, model string }
	newest := make(map[model]int, len(moves))
	for i, m := range moves {
		newest[model{session: m.Session, model: m.Model}] = i
	}
	var kept []Moved
	for i, m := range moves {
		if newest[model{session: m.Session, model: m.Model}] == i {
			kept = append(kept, m)
		}
	}
	return kept
}

// brought is the model the move m took to the account it went to, put there
// as it moved: as the router listed it before it moved, of those listed in
// sessions, its reason the router's still; or, where it didn't, as the move
// tells of it, its family as the router names the model for any session it
// lists.
func brought(sessions []status.Session, m Moved) status.Assignment {
	for _, s := range sessions {
		if s.ID != m.Session {
			continue
		}
		for _, a := range s.Assignments {
			if a.Model == m.Model {
				a.Account, a.AssignedAt = m.To, m.At
				return a
			}
		}
	}
	return status.Assignment{Model: m.Model, Family: familyOf(sessions, m.Model), Account: m.To, AssignedAt: m.At}
}

// familyOf is the family the router names the model with the given id by,
// as it lists any session of it in sessions: "" where it lists none.
func familyOf(sessions []status.Session, model string) string {
	for _, s := range sessions {
		for _, a := range s.Assignments {
			if a.Model == model && a.Family != "" {
				return a.Family
			}
		}
	}
	return ""
}

// cameAt is when a session's model came to the account its assignment a
// gives, as the router lists it: when it was pinned there, where it was
// pinned as it ran, else when it went there.
func cameAt(a status.Assignment) time.Time {
	if a.Pinned && !a.PinnedAt.IsZero() {
		return a.PinnedAt
	}
	return a.AssignedAt
}

// whyRunning is why the session s's model is on the account its assignment
// a gives, at the moment m, as status.RunningWords says it: since it was
// pinned there, where it was pinned as it ran, else since it went there; from
// the account m's newest event moving it there tells, where one does; the
// best while that account is where new sessions go, which ▲ marks.
func whyRunning(m status.Moment, s status.Session, a status.Assignment) status.Words {
	facts := status.Facts{To: a.Account, At: cameAt(a), Best: a.Account == m.Next, Now: m.Now}
	i := slices.IndexFunc(m.Events, func(e status.Event) bool {
		return e.Kind == status.EventMoved && e.Session == s.ID && e.Model == a.Model && e.To == a.Account
	})
	if i >= 0 {
		facts.From = m.Events[i].From
	}
	return status.RunningWords(a.Reason, facts)
}

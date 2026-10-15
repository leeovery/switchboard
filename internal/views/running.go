package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
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
	// Model is the id of its model named there, and Family that model's
	// family, as "opus": the model naming the session, as the List names it,
	// where that goes there, running or not, else its model seen last there.
	Model, Family string
	// Why is why its model named there is there, in the views' words for the
	// router's reasons, with when, as "started here 14:41, the best".
	Why status.Words
	// Activity is what its models there are doing, the busiest of them, and
	// when it was last seen at work there.
	Activity Activity
}

// RunningOf are the sessions running on the accounts at the moment m, of
// the sessions live gives the router listing, at m's Now, as the request
// stream it gives has moved and seen them since, as placed has them, those
// that run, as runningNow has them, each named as the List names it, its
// lines as the List held, list, holds them: a row on each account their
// models go to that run there, as runningOn has it, the accounts in the
// config's order, and on each, the session seen last first. What each is
// doing is as last known where live says no router lists them, as known
// has it.
func RunningOf(m status.Moment, live ListLive, list HeldList) []Running {
	return runningOf(m, newLister(live.Running, live.Unlisted, live.Stream, m.Now), list)
}

// RunningCountsOf counts the sessions running on each account at the moment
// m, of the sessions the router listed, as the request stream s has moved and
// seen them since, as RunningCounts counts RunningOf's rows: which rows there
// are doesn't hang on what names a session, so it reads no lines.
func RunningCountsOf(m status.Moment, sessions []status.Session, s Stream) map[string]int {
	return RunningCounts(RunningOf(m, ListLive{Running: sessions, Stream: s}, HeldList{}))
}

// runningOf are the sessions running at the moment m, as RunningOf has
// them, as by lists them, the List held, list, holding their lines.
func runningOf(m status.Moment, by lister, list HeldList) []Running {
	var running []Running
	for _, a := range m.Accounts {
		var on []Running
		for _, session := range by.running {
			if r, ok := by.runningOn(m, session, a.ID, list.namingOf(session.ID)); ok {
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
// IdleHour, so before it stops.
func runs(a status.Assignment, now time.Time) bool {
	return now.Before(stops(a))
}

// stops is the first moment a session's model, as it's placed in a, no
// longer runs on the account it goes to, as lapsed has it of when it was seen
// there, or put there.
func stops(a status.Assignment) time.Time {
	return lapsed(score.Later(a.LastSeen, a.AssignedAt))
}

// lapsed is the first moment more than an IdleHour has passed since at.
func lapsed(at time.Time) time.Time {
	return at.Add(IdleHour + time.Nanosecond)
}

// runningNow are the sessions given, as a listing of the router's places
// them, that run at now, as the router keeps a session: any of whose models
// runs, as runs says, each with all of them, cold ones too. It's the one rule
// of a session running by a listing, which every view reads every listing
// through, as RunningOf and newLister do: the router's own listing keeps no
// other, so it changes nothing of a fresh one, but one the watch holds while
// the router doesn't answer, or its sessions can't be listed, grows stale.
func runningNow(sessions []status.Session, now time.Time) []status.Session {
	return slices.DeleteFunc(slices.Clone(sessions), func(s status.Session) bool {
		return !slices.ContainsFunc(s.Assignments, func(a status.Assignment) bool { return runs(a, now) })
	})
}

// runningOn is the session s, as by places it, as it runs on the account
// with the given id at the moment m, reporting false where none of its
// models runs there, as runs says: named there by the model naming it, its
// lines naming it as lines has it, as names has it, where that goes there,
// running or not, as the List names it by its main thread however long it
// has waited, else by its model seen last there; and doing what its models
// there are, as at has them.
func (by lister) runningOn(m status.Moment, s status.Session, id string, lines naming) (Running, bool) {
	there := slices.DeleteFunc(slices.Clone(s.Assignments), func(a status.Assignment) bool { return a.Account != id || !runs(a, m.Now) })
	if len(there) == 0 {
		return Running{}, false
	}
	named := slices.MaxFunc(there, func(x, y status.Assignment) int { return x.LastSeen.Compare(y.LastSeen) })
	_, model := by.names(s.ID, lines)
	if i := slices.IndexFunc(s.Assignments, func(a status.Assignment) bool { return a.Account == id && a.Model == model }); i >= 0 {
		named = s.Assignments[i]
	}
	return Running{
		Account: id, Session: s.ID, Model: named.Model, Family: named.Name(), Why: whyRunning(m, s.ID, named),
		Activity: by.known(busiest(by.h.at(s.ID, id, s.Assignments))),
	}, true
}

// placed are the sessions the router listed, as the stream s has moved them
// since, by the moves no listing has shown done, as unlisted has them, and
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
	for _, m := range newestMoves(s.unlisted()) {
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
			session.Assignments[i].LastSeen = score.Later(a.LastSeen, h.seats[Seat{Session: session.ID, Model: a.Model, Account: a.Account}].last)
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
// tells of it, its family as ledger.FamilyOf, which the router names it by,
// reads it.
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
	return status.Assignment{Model: m.Model, Family: ledger.FamilyOf(m.Model), Account: m.To, AssignedAt: m.At}
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

// whyRunning is why the model of the session with the given id is on the
// account its assignment a gives, at the moment m, as status.RunningWords
// says it: since it was pinned there, where it was pinned as it ran, else
// since it went there; from the account m's newest event moving it there
// tells, where one does; the best while that account is where new sessions
// go, which ▲ marks.
func whyRunning(m status.Moment, session string, a status.Assignment) status.Words {
	facts := status.Facts{To: a.Account, At: cameAt(a), Best: a.Account == m.Next, Now: m.Now}
	i := slices.IndexFunc(m.Events, func(e status.Event) bool {
		return e.Kind == status.EventMoved && e.Session == session && e.Model == a.Model && e.To == a.Account
	})
	if i >= 0 {
		facts.From = m.Events[i].From
	}
	return status.RunningWords(a.Reason, facts)
}

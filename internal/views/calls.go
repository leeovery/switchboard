package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/status"
)

// Call is a session's row on an account, as Sessions' Switchboard draws it:
// the session running there, as RunningOf has it; the directory it runs in;
// and the account its named model left, while the move that brought it there
// is that model's latest.
type Call struct {
	Running
	// Dir is the directory its session runs in, as the List has it: as the
	// router gives it, else as its lines last name it; "" where neither names
	// one.
	Dir string
	// From is the account the move that brought its named model to the
	// account left, while that move is the model's latest, as from has it:
	// "" where no move did.
	From string
}

// CallsOf are the calls of the sessions running at the moment m, as RunningOf
// lists them, of the sessions live gives the router listing and what the
// request stream it gives told, each named as the List held names it, with
// its directory and the account it came from, as it tells of its lines.
func CallsOf(m status.Moment, live ListLive, list HeldList) []Call {
	held := list.at(m.Now)
	by := newLister(live.Running, live.Unlisted, live.Stream, m.Now)
	running := runningOf(m, by, held)
	calls := make([]Call, len(running))
	for i, r := range running {
		story := held.storyOf(r.Session)
		calls[i] = Call{Running: r, Dir: by.dir(r.Session, story), From: from(r, story, live.Stream.Moves)}
	}
	return calls
}

// from is the account the move that brought the call r's named model, as
// its Running names it, to its account left, its session's story s telling
// of its lines, where that move is the model's latest: the newest of the
// model's moves the request stream told of, moves, oldest first, or of its
// lines, whichever came later, a line's only where it came on the day; ""
// where the latest went elsewhere, or none is known.
func from(r Running, s *story, moves []Moved) string {
	model := r.Model
	lined := s.latestMove(model)
	for _, m := range slices.Backward(moves) {
		if m.Session != r.Session || m.Model != model {
			continue
		}
		if lined == nil || !m.At.Before(lined.at) {
			return movedFrom(m.From, m.To, r.Account)
		}
		break
	}
	if lined == nil || !s.day.Holds(lined.at) {
		return ""
	}
	return movedFrom(lined.from, lined.to, r.Account)
}

// movedFrom is the account a move from one account to another left, where it
// went to the account given: "" where it went elsewhere.
func movedFrom(from, to, account string) string {
	if to != account {
		return ""
	}
	return from
}

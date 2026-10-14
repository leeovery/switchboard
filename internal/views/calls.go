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
// lists them, of the sessions the router lists and what the request stream s
// told, each with its directory and the account it came from, as the List
// held tells of its lines.
func CallsOf(m status.Moment, sessions []status.Session, s Stream, list HeldList) []Call {
	held := list.at(m.Now)
	by := newLister(sessions, false, s, m.Now)
	running := RunningOf(m, sessions, s)
	calls := make([]Call, len(running))
	for i, r := range running {
		story := held.storyOf(r.Session)
		calls[i] = Call{Running: r, Dir: by.dir(r.Session, story), From: by.from(r, story, s.Moves)}
	}
	return calls
}

// from is the account the move that brought the call r's named model to its
// account left, its session's story s telling of its lines, where that move
// is the model's latest: the newest of the model's moves the request stream
// told of, moves, oldest first, or of its lines, whichever came later, a
// line's only where it came on the day; "" where the latest went elsewhere,
// or none is known. The model naming a call is the one naming its session,
// as the List names it, where it runs on the call's account, else its model
// seen last there.
func (by lister) from(r Running, s *story, moves []Moved) string {
	model := r.Models[0]
	if _, named := by.names(r.Session, s); slices.Contains(r.Models, named) {
		model = named
	}
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

package views

import (
	"slices"
	"unique"

	"github.com/leeovery/switchboard/internal/ledger"
)

// takenAnswers are every account's answers a read of the ledger's days took,
// by the date of the day whose files hold them, as dayAnswers holds each
// day's: each answered once as its line is taken, never its line held, so a
// read of a busy day holds its answers small, whichever session's page they
// count toward.
type takenAnswers map[string]*dayAnswers

// dayAnswers are a day's answers taken, in the order taken: each as answerOf
// has it, of no session; and the session of each, and the pool its line
// recorded, interned.
type dayAnswers struct {
	answers  []answered
	sessions []unique.Handle[string]
	pools    []unique.Handle[string]
}

// take takes in the line l, of the day with the given date, as a read of
// the ledger's days hands it, where it's an answer the points share rises
// among, as isAnswer has it.
func (t takenAnswers) take(date string, l ledger.Line) {
	if !isAnswer(&l) {
		return
	}
	d := t[date]
	if d == nil {
		d = &dayAnswers{}
		t[date] = d
	}
	d.answers = append(d.answers, answerOf(&l, false))
	d.sessions = append(d.sessions, unique.Make(l.Session))
	d.pools = append(d.pools, unique.Make(l.Pool))
}

// in are the answers taken, each of its session where its line was routed
// in the pool in reads, as ledger.DigestDay's In has it, else of none, as a
// pool's read hands a session's lines in another pool, as PoolRead's owned
// has it: so a page's points take as a session's own its requests in the
// pool alone. They're t itself where in is nil, of every pool's; else the
// answers t holds, shared.
func (t takenAnswers) in(in ledger.RoutedIn) takenAnswers {
	if in == nil {
		return t
	}
	none, owned := unique.Make(""), make(takenAnswers, len(t))
	for date, d := range t {
		sessions := slices.Clone(d.sessions)
		for i, pool := range d.pools {
			if !in(pool.Value()) {
				sessions[i] = none
			}
		}
		owned[date] = &dayAnswers{answers: d.answers, sessions: sessions, pools: d.pools}
	}
	return owned
}

// of are the answers taken of the days after the one with the date after,
// by date, each as answeredOf has it of the page of the session with the
// given id, as of a read of its every line: of every day taken, of an after
// of "", as of a session resting on no base. Those of its base's day and
// before, its points took in as they were sealed, are never made.
func (t takenAnswers) of(id, after string) map[string][]answered {
	session := unique.Make(id)
	answers := make(map[string][]answered, len(t))
	for date, d := range t {
		if date > after {
			answers[date] = d.of(session)
		}
	}
	return answers
}

// of are the day's answers, the given session's own ours, each summed of
// what it holds then, as answeredOf has it: of a day none of whose answers
// are its own, the day's, shared, as nothing changes an answer it's given in
// place.
func (d *dayAnswers) of(session unique.Handle[string]) []answered {
	first := slices.Index(d.sessions, session)
	if first < 0 {
		return d.answers
	}
	own := slices.Clone(d.answers)
	for i := first; i < len(own); i++ {
		if d.sessions[i] == session {
			own[i].ours = true
			own[i].sum = own[i].summed()
		}
	}
	return own
}

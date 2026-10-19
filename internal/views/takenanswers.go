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
// has it, of no session, and the session of each, interned.
type dayAnswers struct {
	answers  []answered
	sessions []unique.Handle[string]
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
}

// of are the answers taken, by date, each as answeredOf has it of the page of
// the session with the given id, as of a read of its every line.
func (t takenAnswers) of(id string) map[string][]answered {
	session := unique.Make(id)
	answers := make(map[string][]answered, len(t))
	for date, d := range t {
		answers[date] = d.of(session)
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

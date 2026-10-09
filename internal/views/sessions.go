package views

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// SessionList is Sessions' List, as sessions prints it: today's sessions,
// those the router lists, the oldest started first, then those that ended
// today, the latest ended first, and what they come to together.
type SessionList struct {
	GeneratedAt time.Time `json:"generated_at"`
	// PricesAsOf is the date of the day the price table was read, as
	// 2026-10-07.
	PricesAsOf string          `json:"prices_as_of"`
	Sessions   []ListedSession `json:"sessions"`
	Today      SessionsToday   `json:"today"`
}

// ListedSession is a session as the List has it. Fields may be added to it,
// never renamed.
type ListedSession struct {
	// Session is its whole id.
	Session string `json:"session"`
	// Dir is the directory it runs in, as the router gives it, else as its
	// lines last name it, the home as ~: "" where neither names one.
	Dir string `json:"dir,omitempty"`
	// Account and Model are the account its latest request went to, and that
	// request's model: of one running, as the router last routed it; of one
	// ended, its last request's.
	Account string `json:"account,omitempty"`
	Model   string `json:"model,omitempty"`
	// Running is set while the router lists it.
	Running bool `json:"running"`
	// LastSeen, Models, State and MoveCost are of one running alone:
	// when the router last saw it; each of its models, the account it goes
	// to now and why; what its requests in flight are doing, status.Answering
	// where any is answered, else status.Asking where any waits, "" where
	// none is in flight; and what moving it now would cost, nil where its
	// cache would have run out anyway, or that isn't known.
	LastSeen time.Time           `json:"last_seen,omitzero"`
	Models   []SessionModel      `json:"models,omitempty"`
	State    string              `json:"state,omitempty"`
	MoveCost *ledger.Picodollars `json:"move_cost,omitempty"`
	// Ended is, of one the router doesn't list, when its last request came.
	Ended time.Time `json:"ended,omitzero"`
	// Started is when its first request came, today or on an earlier day:
	// zero where the ledger holds none.
	Started time.Time `json:"started,omitzero"`
	// Requests counts its requests today, and Worth is what they'd have cost
	// through the API at today's prices, Unpriced naming the counts it leaves
	// out, as it can't price them.
	Requests int                `json:"requests"`
	Worth    ledger.Picodollars `json:"worth"`
	Unpriced []string           `json:"unpriced,omitempty"`
	// Moved is the move that brought it to Account, where a request of it
	// today made it: nil where none did.
	Moved *Move `json:"moved,omitempty"`
}

// SessionModel is a model of a running session: the account its requests go
// to now, and why, as the router gives it.
type SessionModel struct {
	Model   string `json:"model"`
	Account string `json:"account"`
	Reason  string `json:"reason"`
}

// Move is a request's move of its session onto the account it's on: when it
// came, the account it left, why, as the router gave it, and what writing its
// context again there cost, nil where its cache would have run out anyway,
// or that isn't known.
type Move struct {
	At     time.Time           `json:"at"`
	From   string              `json:"from"`
	Reason string              `json:"reason"`
	Cost   *ledger.Picodollars `json:"cost,omitempty"`
}

// SessionsToday sums the List's sessions: how many there are, their requests
// today, and what those would have cost through the API.
type SessionsToday struct {
	Sessions int                `json:"sessions"`
	Requests int                `json:"requests"`
	Worth    ledger.Picodollars `json:"worth"`
}

// SessionSources are what the List is built from.
type SessionSources struct {
	// Running are the sessions the router lists, as GET /sessions gives
	// them: none without the router.
	Running []status.Session
	// Ledger reads the request ledger's lines, and its days' summaries, which
	// name the sessions of each.
	Ledger Ledger
	// Prices are the price table worth is priced by.
	Prices ledger.Table
	Now    time.Time
}

// ListSessions builds the List from src: the sessions the router lists, and
// those with requests today, a session being the lines of its id, each with
// what the router says of it and what its lines tell, those of the days
// before today among them for one the summaries of those days name.
func ListSessions(src SessionSources) SessionList {
	today := dayOf(src.Now)
	todays, _, _ := src.Ledger.Today(ledger.Mark{})
	stories, ids := storiesOf(todays, src.Prices, today)
	routed := make(map[string]status.Session, len(src.Running))
	for _, s := range src.Running {
		routed[s.ID] = s
		if _, ok := stories[s.ID]; !ok {
			stories[s.ID] = newStory(src.Prices, today)
			ids = append(ids, s.ID)
		}
	}
	for id := range namedBefore(src.Ledger, ids, today) {
		stories[id] = sessionStory(src.Ledger, id, src.Prices, today)
	}
	list := SessionList{GeneratedAt: src.Now.UTC(), PricesAsOf: src.Prices.AsOf, Sessions: []ListedSession{}}
	for _, id := range ids {
		s := stories[id]
		listed := listedFrom(id, s)
		if session, running := routed[id]; running {
			listed.run(session, s, src.Now)
		}
		listed.Moved = s.movedOnto(listed.Account)
		list.Sessions = append(list.Sessions, listed)
		list.Today.Requests += listed.Requests
		list.Today.Worth += listed.Worth
	}
	list.Today.Sessions = len(list.Sessions)
	slices.SortFunc(list.Sessions, inListOrder)
	return list
}

// storiesOf returns the story of each session lines, oldest first, hold a
// request of, by its id, and their ids, in the order their first requests
// came.
func storiesOf(lines []ledger.Held, prices ledger.Table, d day) (map[string]*story, []string) {
	stories := make(map[string]*story)
	var ids []string
	for i := range lines {
		l := &lines[i].Line
		if !isRequest(l) {
			continue
		}
		s, ok := stories[l.Session]
		if !ok {
			s = newStory(prices, d)
			stories[l.Session], ids = s, append(ids, l.Session)
		}
		s.add(l)
	}
	return stories, ids
}

// namedBefore returns those of ids the summaries of the days before d name
// among their sessions: those that started on an earlier day.
func namedBefore(l Ledger, ids []string, d day) map[string]bool {
	named := make(map[string]bool)
	for _, summary := range l.Days(time.Time{}) {
		if summary.Day >= d.date {
			continue
		}
		for _, a := range summary.Accounts {
			for _, id := range a.SessionIDs {
				if slices.Contains(ids, id) {
					named[id] = true
				}
			}
		}
	}
	return named
}

// sessionStory returns the story of the session with the given id told by
// every line of it the ledger holds, whatever day each is of.
func sessionStory(l Ledger, id string, prices ledger.Table, d day) *story {
	var newestFirst []ledger.Held
	for h := range l.Session(id) {
		newestFirst = append(newestFirst, h)
	}
	s := newStory(prices, d)
	for i := range slices.Backward(newestFirst) {
		s.add(&newestFirst[i].Line)
	}
	return s
}

// listedFrom returns the session with the given id as its story tells it,
// ended, its last request's account and model its own.
func listedFrom(id string, s *story) ListedSession {
	listed := ListedSession{Session: id, Dir: s.dir, Requests: s.requests, Worth: s.worth.cost, Unpriced: s.worth.unpriced}
	if s.first != nil {
		listed.Started = s.first.At.UTC()
	}
	if s.last != nil {
		listed.Account, listed.Model, listed.Ended = s.last.Account, s.last.Model, s.last.At.UTC()
	}
	return listed
}

// run has the session running, as the router says of it at now: its models,
// the account and model it last routed, its directory, where it names one,
// what it's doing, and what moving it would cost, its story telling the
// rest.
func (l *ListedSession) run(session status.Session, s *story, now time.Time) {
	l.Running, l.Ended = true, time.Time{}
	if len(session.Assignments) > 0 {
		l.Account, l.Model = session.Assignments[0].Account, session.Assignments[0].Model
	}
	if i := slices.IndexFunc(session.Assignments, func(a status.Assignment) bool { return a.Dir != "" }); i >= 0 {
		l.Dir = session.Assignments[i].Dir
	}
	for _, a := range session.Assignments {
		l.Models = append(l.Models, SessionModel{Model: a.Model, Account: a.Account, Reason: a.Reason})
		l.LastSeen = latest(l.LastSeen, a.LastSeen.UTC())
		l.State = busier(l.State, a.InFlight)
	}
	idle := now.Sub(l.LastSeen)
	if l.State != "" {
		idle = 0
	}
	if cost, ok := s.moveCost(idle); ok {
		l.MoveCost = &cost
	}
}

// latest returns the later of a and b.
func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// busier returns the busier of two models' requests in flight: answering
// before asking, and either before none.
func busier(a, b string) string {
	switch {
	case a == status.Answering || b == status.Answering:
		return status.Answering
	case a == status.Asking || b == status.Asking:
		return status.Asking
	}
	return ""
}

// inListOrder orders the List: those running first, the oldest started
// first, one whose start isn't known last; then those ended, the latest
// ended first; each by its id where nothing else tells them apart.
func inListOrder(a, b ListedSession) int {
	switch {
	case a.Running != b.Running:
		if a.Running {
			return -1
		}
		return 1
	case a.Running:
		return cmp.Or(startedFirst(a.Started, b.Started), strings.Compare(a.Session, b.Session))
	}
	return cmp.Or(b.Ended.Compare(a.Ended), strings.Compare(a.Session, b.Session))
}

// startedFirst orders starts, the earliest first, and an unknown one, zero,
// after any known.
func startedFirst(a, b time.Time) int {
	switch {
	case a.IsZero() != b.IsZero():
		if a.IsZero() {
			return 1
		}
		return -1
	}
	return a.Compare(b)
}

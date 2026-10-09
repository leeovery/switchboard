package views

import (
	"cmp"
	"maps"
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
	// ended, its last request of its own conversation's, where its requests
	// say their class, else its last request's.
	Account string `json:"account,omitempty"`
	Model   string `json:"model,omitempty"`
	// Running is set while the router lists it, and left out otherwise.
	Running bool `json:"running,omitempty"`
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
// today, and what those would have cost through the API, Unpriced naming the
// counts any session's worth leaves out.
type SessionsToday struct {
	Sessions int                `json:"sessions"`
	Requests int                `json:"requests"`
	Worth    ledger.Picodollars `json:"worth"`
	Unpriced []string           `json:"unpriced,omitempty"`
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
// those with requests today, each with what the router says of it and what
// its lines tell, of whatever day, as the ledger's Sessions reads them.
func ListSessions(src SessionSources) SessionList {
	today := dayOf(src.Now)
	read := src.Ledger.Sessions(runningIDs(src.Running), nil)
	routed := make(map[string]status.Session, len(src.Running))
	for _, s := range src.Running {
		routed[s.ID] = s
	}
	list := SessionList{GeneratedAt: src.Now.UTC(), PricesAsOf: src.Prices.AsOf, Sessions: []ListedSession{}}
	var worth worthSum
	for _, id := range listedIDs(read, src.Running) {
		s := storyOf(requestsOf(read[id], src.Prices, today.date), src.Prices, today)
		session, running := routed[id]
		if !running && s.requests == 0 {
			continue
		}
		listed := listedFrom(id, s)
		if running {
			listed.run(session, s, src.Now)
		}
		listed.Moved = s.movedOnto(listed.Model, listed.Account)
		list.Sessions = append(list.Sessions, listed)
		list.Today.Requests += listed.Requests
		worth.add(ledger.Worth{Cost: listed.Worth, Unpriced: listed.Unpriced})
	}
	list.Today.Sessions, list.Today.Worth, list.Today.Unpriced = len(list.Sessions), worth.cost, worth.unpriced
	slices.SortFunc(list.Sessions, inListOrder)
	return list
}

// runningIDs returns the ids of the sessions running gives, as the router
// lists them.
func runningIDs(running []status.Session) []string {
	ids := make([]string, len(running))
	for i, s := range running {
		ids[i] = s.ID
	}
	return ids
}

// listedIDs returns the ids of the sessions read and those running gives,
// each once.
func listedIDs(read map[string][]ledger.Line, running []status.Session) []string {
	ids := slices.Collect(maps.Keys(read))
	for _, s := range running {
		if _, ok := read[s.ID]; !ok {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// listedFrom returns the session with the given id as its story tells it,
// ended at its last request, the account and model its own its
// conversation's, as story.conversation says.
func listedFrom(id string, s *story) ListedSession {
	listed := ListedSession{Session: id, Dir: s.dir, Requests: s.requests, Worth: s.worth.cost, Unpriced: s.worth.unpriced}
	if s.first != nil {
		listed.Started = s.first.At.UTC()
	}
	if s.last != nil {
		listed.Ended = s.last.At.UTC()
	}
	if c := s.conversation(); c != nil {
		listed.Account, listed.Model = c.Account, c.Model
	}
	return listed
}

// run has the session running, as the router says of it at now: its models;
// the account its conversation's model goes to, and that model, as its story
// tells its conversation's, else as the router last routed it; its
// directory, where it names one; what it's doing; and what moving it would
// cost, its story telling the rest.
func (l *ListedSession) run(session status.Session, s *story, now time.Time) {
	l.Running, l.Ended = true, time.Time{}
	model := s.conversationModel()
	if i := slices.IndexFunc(session.Assignments, func(a status.Assignment) bool { return a.Model == model }); i >= 0 {
		l.Account, l.Model = session.Assignments[i].Account, model
	} else if len(session.Assignments) > 0 {
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
	flying := func(model string) bool {
		return slices.ContainsFunc(session.Assignments, func(a status.Assignment) bool { return a.Model == model && a.InFlight != "" })
	}
	if cost, ok := s.moveCost(now, flying); ok {
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

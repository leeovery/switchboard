package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
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
	// Account and Model name it, following its main thread: the model of its
	// latest main request, and the account that model goes to now, as the
	// router lists it and the request stream has moved it since, else the
	// account that request went to; where none of its requests is its main
	// thread's, the model the router saw last, and its account, else its
	// latest request's.
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
	// through the API at today's prices, as Worth sums it.
	Requests int `json:"requests"`
	Worth
	// Moved is the move that brought it to Account, where a request of it
	// today made it: nil where none did.
	Moved *Move `json:"moved,omitempty"`
	// Activity is what it's doing, as the router lists it, the request
	// stream tells and its lines give, one rule whether the router lists it
	// or not: ended where none has seen it within the IdleHour. It's the
	// views' alone.
	Activity Activity `json:"-"`
	// Mark and Routing are its mark, of those the List shows, and how it
	// came to Account, as the Log's row of its request says them, as cameBy
	// has them. They're the views' alone.
	Mark    RowMark     `json:"-"`
	Routing RoutingSaid `json:"-"`
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
// today, and what those would have cost through the API, as Worth sums it.
type SessionsToday struct {
	Sessions int `json:"sessions"`
	Requests int `json:"requests"`
	Worth
}

// SessionSources are what the List is built from.
type SessionSources struct {
	// Running are the sessions the router lists, as GET /sessions gives
	// them: none without the router.
	Running []status.Session
	// Unlisted is set where no router lists the sessions, as while it's
	// gone, or the dashboard probes: one whose last request came within the
	// IdleHour is idle, rather than ended, as the router's own rule keeps a
	// session.
	Unlisted bool
	// Stream is what the request stream has told of the sessions' requests:
	// zero where it's unheard, as by the verbs.
	Stream Stream
	// Ledger reads the request ledger's lines, and its days' summaries, which
	// name the sessions of each.
	Ledger Ledger
	// Prices are the price table worth is priced by.
	Prices ledger.Table
	Now    time.Time
}

// ListSessions builds the List from src: the sessions the router lists, and
// those with requests today, each with what the router says of it, what the
// request stream tells of its requests, and what its lines tell, of
// whatever day, as the ledger's Sessions reads them.
func ListSessions(src SessionSources) SessionList {
	today := Today(src.Now)
	by := newLister(src.Running, src.Unlisted, src.Stream, src.Now)
	read := src.Ledger.Sessions(runningIDs(src.Running), nil)
	list := SessionList{GeneratedAt: src.Now.UTC(), PricesAsOf: src.Prices.AsOf, Sessions: []ListedSession{}}
	for _, id := range listedIDs(read, src.Running) {
		s := storyOf(requestsOf(read[id], src.Prices, today.Date), src.Prices, today)
		if _, running := by.routed[id]; !running && s.requests == 0 {
			continue
		}
		listed := by.listed(id, s)
		list.Sessions = append(list.Sessions, listed)
		list.Today.Requests += listed.Requests
		list.Today.add(listed.Worth)
	}
	list.Today.Sessions = len(list.Sessions)
	slices.SortFunc(list.Sessions, inListOrder)
	return list
}

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

// lister is what the List lists each session by, beside what its requests
// tell, as a session's page has it too: the sessions the router lists, by
// their ids, and where their models go now, as the router lists them and the
// request stream has moved them since; whether no router lists them; what
// the stream told, as h hears it; and now.
type lister struct {
	routed   map[string]status.Session
	goes     map[string][]status.Assignment
	unlisted bool
	h        hearing
	now      time.Time
}

// newLister returns the lister of the sessions running, as the router lists
// them, unlisted saying no router does, the request stream s telling of
// them, at now.
func newLister(running []status.Session, unlisted bool, s Stream, now time.Time) lister {
	by := lister{
		routed: make(map[string]status.Session, len(running)), goes: make(map[string][]status.Assignment),
		unlisted: unlisted, h: s.hearing(), now: now,
	}
	for _, session := range running {
		by.routed[session.ID] = session
	}
	for _, session := range s.placed(running, by.h) {
		by.goes[session.ID] = session.Assignments
	}
	return by
}

// listed returns the session with the given id as the List has it, its story
// s telling of its requests: ended at its last request; named as names has
// it, the stream's requests of it taken in, its models going where they go
// now; running, where the router lists it, as run has it; doing as activity
// has it; and come to its account as cameBy has it.
func (by lister) listed(id string, s *story) ListedSession {
	listed := ListedSession{Session: id, Dir: s.dir, Requests: s.requests, Worth: s.worth}
	if s.first != nil {
		listed.Started = s.first.At.UTC()
	}
	if s.last != nil {
		listed.Ended = s.last.At.UTC()
	}
	listed.Account, listed.Model = s.naming.heard(by.h.told[id]).names(by.goes[id])
	if session, running := by.routed[id]; running {
		listed.run(session, s, by.now)
	}
	listed.Activity = by.activity(id, s)
	listed.cameBy(s.moveOnto(listed.Model, listed.Account))
	return listed
}

// activity is what the session with the given id is doing at now, its story
// s telling of its lines, one rule whether the router lists it or not: what
// the router listed its models doing, where it lists it, what the request
// stream told, and what its last line gives, together, as Activity.with has
// them, so idle since the latest of them. It's ended, since its last request
// came, where none has seen it within the IdleHour, as the router's own rule
// lets a session go: not the router, which lists it while it's seen; not the
// live stream; nor, where no router lists the sessions, as while it's gone
// or the dashboard probes, its last line.
func (by lister) activity(id string, s *story) Activity {
	session, listed := by.routed[id]
	session.ID = id
	a := by.h.session(session).inUTC()
	if s.last == nil {
		return a
	}
	last := s.last.At.UTC()
	a = a.with(Activity{Since: ended(s.last).UTC(), LastSeen: last})
	if listed || by.h.saw(id, by.now) || by.unlisted && by.now.Sub(last) <= IdleHour {
		return a
	}
	return Activity{Doing: Ended, Since: last, LastSeen: last}
}

// run has the session running, as the router says of it at now: its models;
// its directory, of the model seen most lately that names one, where any
// does; what its requests in flight are doing, as the router listed them;
// and what moving it would cost, its story telling the rest.
func (l *ListedSession) run(session status.Session, s *story, now time.Time) {
	l.Running, l.Ended = true, time.Time{}
	if named := slices.DeleteFunc(slices.Clone(session.Assignments), func(a status.Assignment) bool { return a.Dir == "" }); len(named) > 0 {
		l.Dir = slices.MaxFunc(named, func(a, b status.Assignment) int { return a.LastSeen.Compare(b.LastSeen) }).Dir
	}
	for _, a := range session.Assignments {
		l.Models = append(l.Models, SessionModel{Model: a.Model, Account: a.Account, Reason: a.Reason})
		l.LastSeen = score.Later(l.LastSeen, a.LastSeen.UTC())
		l.State = busier(l.State, a.InFlight)
	}
	flying := func(model string) bool {
		return slices.ContainsFunc(session.Assignments, func(a status.Assignment) bool { return a.Model == model && a.InFlight != "" })
	}
	if cost, ok := s.moveCost(now, flying); ok {
		l.MoveCost = &cost
	}
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

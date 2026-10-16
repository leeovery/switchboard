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
	// Since is when it went to the account, or was pinned there, as the
	// router lists it, as cameAt has it: zero where that isn't known. It's
	// the views' alone.
	Since time.Time `json:"-"`
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
	read := src.Ledger.Sessions(runningIDs(src.Running), nil)
	stories := make(map[string]*story, len(read))
	for id, lines := range read {
		stories[id] = storyOf(requestsOf(lines, src.Prices, today.Date), src.Prices, today)
	}
	return listOf(stories, newLister(src.Running, src.Unlisted, src.Stream, src.Now), src.Prices)
}

// listOf is the List of the sessions by lists, as the router lists them,
// and those stories tells of with requests today, each as listed has it, its
// story the one stories holds by its id, else one of no requests: each day's
// List, whether it's built afresh or held.
func listOf(stories map[string]*story, by lister, prices ledger.Table) SessionList {
	list := SessionList{GeneratedAt: by.now.UTC(), PricesAsOf: prices.AsOf, Sessions: []ListedSession{}}
	for _, id := range listedIDs(stories, by.routed) {
		s := stories[id]
		if s == nil {
			s = newStory(prices, Today(by.now))
		}
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

// listedIDs returns the ids of the sessions told of and those routed, each
// once.
func listedIDs(told map[string]*story, routed map[string]status.Session) []string {
	ids := slices.Collect(maps.Keys(told))
	for id := range routed {
		if _, ok := told[id]; !ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// lister is what the List lists each session by, beside what its requests
// tell, as a session's page and SESSIONS have it too: the sessions the
// router lists, by their ids; those that run, where their models go now, as
// the router lists them and the request stream has moved them since, in the
// order placed has them, and by their ids; whether no router lists them;
// what the stream told, as h hears it; and now.
type lister struct {
	routed   map[string]status.Session
	running  []status.Session
	goes     map[string][]status.Assignment
	unlisted bool
	h        hearing
	now      time.Time
}

// newLister returns the lister of the sessions running, as the router lists
// them, unlisted saying no router does, the request stream s telling of
// them, at now: those that run, as runningNow has them, every model of each,
// as the Switchboard reads them, whether the listing is fresh or the last the
// watch holds.
func newLister(running []status.Session, unlisted bool, s Stream, now time.Time) lister {
	by := lister{
		routed: make(map[string]status.Session, len(running)), goes: make(map[string][]status.Assignment),
		unlisted: unlisted, h: s.hearing(), now: now,
	}
	by.running = runningNow(s.placed(running, by.h), now)
	for _, session := range by.running {
		by.goes[session.ID] = session.Assignments
	}
	for _, session := range running {
		if _, runs := by.goes[session.ID]; runs {
			by.routed[session.ID] = session
		}
	}
	return by
}

// listed returns the session with the given id as the List has it, its story
// s telling of its requests: in the directory dir gives; ended at its last
// request; named as names has it; running, where the router lists it, as run
// has it; doing as activity has it; and come to its account as cameBy has
// it.
func (by lister) listed(id string, s *story) ListedSession {
	listed := ListedSession{Session: id, Dir: by.dir(id, s), Requests: s.requests, Worth: s.worth}
	if s.first != nil {
		listed.Started = s.first.At.UTC()
	}
	if s.last != nil {
		listed.Ended = s.last.At.UTC()
	}
	listed.Account, listed.Model = by.names(id, s.naming)
	if session, running := by.routed[id]; running {
		listed.run(session, s, by.now)
	}
	listed.Activity = by.activity(id, s)
	listed.cameBy(s.moveOnto(listed.Model, listed.Account))
	return listed
}

// dir is the directory the session with the given id runs in, its story s
// telling of its lines: as the router gives it, where it lists it, of the
// model seen most lately that names one; else as its lines last name it; ""
// where neither names one.
func (by lister) dir(id string, s *story) string {
	if dir := routedDir(by.routed[id]); dir != "" {
		return dir
	}
	return s.dir
}

// routedDir is the directory the session s runs in as the router lists it:
// of its model seen most lately that names one; "" where none does.
func routedDir(s status.Session) string {
	named := slices.DeleteFunc(slices.Clone(s.Assignments), func(a status.Assignment) bool { return a.Dir == "" })
	if len(named) == 0 {
		return ""
	}
	return slices.MaxFunc(named, func(a, b status.Assignment) int { return a.LastSeen.Compare(b.LastSeen) }).Dir
}

// names are the account and model naming the session with the given id, its
// lines naming it as lines has it, as naming.names has them: the stream's
// requests of it taken in after them, its models going where they go now.
// It's the one rule of which model is a session's, and where it's named on,
// that the List, a session's page, the Log's panel, SESSIONS and the
// Switchboard all read.
func (by lister) names(id string, lines naming) (account, model string) {
	return lines.with(by.h.named[id]).names(by.goes[id])
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
		return by.known(a)
	}
	last := s.last.At.UTC()
	a = a.with(Activity{Since: ended(s.last).UTC(), LastSeen: last})
	if listed || by.h.saw(id, by.now) || by.unlisted && by.now.Before(lapsed(last)) {
		return by.known(a)
	}
	return Activity{Doing: Ended, Since: last, LastSeen: last}
}

// known is a, as last known where no router lists the sessions, as while it
// doesn't answer: a request of it in flight then Stale. It's the one rule of
// what's in flight being as last known, which the List, a session's page,
// SESSIONS, Detail and the Switchboard all read.
func (by lister) known(a Activity) Activity {
	a.Stale = by.unlisted && a.InFlight()
	return a
}

// run has the session running, as the router says of it at now: its models;
// what its requests in flight are doing, as the router listed them; and what
// moving it would cost, its story telling the rest.
func (l *ListedSession) run(session status.Session, s *story, now time.Time) {
	l.Running, l.Ended = true, time.Time{}
	for _, a := range session.Assignments {
		l.Models = append(l.Models, SessionModel{Model: a.Model, Account: a.Account, Reason: a.Reason, Since: cameAt(a)})
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

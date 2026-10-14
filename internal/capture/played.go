package capture

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// world is the router as a scenario has it at a moment: its base, and what
// the scenario has had befall it since, laid over it: the readings its
// answers and limits read, how far its answers have used each window, and
// each as last read since; the limits reached; what the router has told of;
// the sessions as it lists them, how many it has sent each account in the
// last hour, and how many have started; its global pin, and each session's
// own pin, by the session's id.
type world struct {
	base     *base
	readings readingsOf
	used     map[windowOf]float64
	read     map[windowOf]readings.Reading
	limits   map[string]status.Limit
	events   []status.Event
	sessions []status.Session
	sent     map[string]int
	started  int
	pin      status.Pin
	pins     map[string]ownPin
}

// startingOut is the world as the base has it, nothing laid over it yet: its
// sessions and their pins the world's own, to change.
func (b *base) startingOut() world {
	sessions := slices.Clone(b.listed)
	for i := range sessions {
		sessions[i].Assignments = slices.Clone(sessions[i].Assignments)
	}
	return world{
		base: b, sessions: sessions, pins: maps.Clone(b.pins), pin: b.world.pin,
		used: make(map[windowOf]float64), read: make(map[windowOf]readings.Reading),
		limits: make(map[string]status.Limit), sent: make(map[string]int),
	}
}

// accountsAt is the world at now as its accounts' world has it, as
// accountWorld.document reads it, read fresh: its history the windows as
// they ran as it started, and what's been read since, the base's read in
// place; its limits, the base's and those reached since; its events,
// oldest first; and where new sessions go, as destination says.
func (w world) accountsAt(now time.Time) accountWorld {
	a := w.base.world
	a.history = slices.Concat(w.base.current, w.readings.read)
	a.limits = maps.Clone(a.limits)
	if a.limits == nil {
		a.limits = make(map[string]status.Limit)
	}
	maps.Copy(a.limits, w.limits)
	a.sessions = maps.Clone(a.sessions)
	if a.sessions == nil {
		a.sessions = make(map[string]int)
	}
	for id, n := range w.sent {
		a.sessions[id] += n
	}
	a.events = slices.Concat(a.events, w.events)
	a.pin, a.best, a.fresh = w.pin, w.destination(now), true
	a.routed, a.stream = nil, nil
	return a
}

// document is the router's status document of the world at now, as
// accountWorld.document builds it, but for the sessions it counts: each
// once, however many accounts it went to, as the router counts them.
func (w world) document(now time.Time) status.Document {
	doc := w.accountsAt(now).document(now)
	doc.Sessions = w.base.sessions + w.started
	return doc
}

// listing are the sessions the router lists at now, as GET /sessions gives
// them, with what the stream told tells of what's in flight: each with its
// own pin, its models pinned where they're on the account it names, since
// it was given; what each model's request in flight is doing; where auto
// would send each, where the base's listing said, as the router's own rule
// chooses it then; the one used last first, and the session seen last
// first.
func (w world) listing(now time.Time, told []router.StreamEvent) []status.Session {
	doing := make(map[[3]string]string)
	for _, f := range flying(told, now) {
		what := status.Asking
		if !f.FirstAt.IsZero() {
			what = status.Answering
		}
		doing[[3]string{f.Session, f.Model, f.Account}] = what
	}
	listed := make([]status.Session, len(w.sessions))
	for i, s := range w.sessions {
		pin := w.pins[s.ID]
		s.Pin, s.Assignments = pin.account, slices.Clone(s.Assignments)
		for j := range s.Assignments {
			a := &s.Assignments[j]
			a.Pinned, a.PinnedAt = pin.account != "" && a.Account == pin.account, time.Time{}
			if a.Pinned {
				a.PinnedAt = pin.at.UTC()
			}
			a.InFlight = doing[[3]string{s.ID, a.Model, a.Account}]
			if a.Auto != "" {
				a.Auto, _ = w.routedAs(*a, ownPin{}, false, now)
			}
		}
		slices.SortStableFunc(s.Assignments, seenLater)
		listed[i] = s
	}
	slices.SortStableFunc(listed, func(a, b status.Session) int { return seenLater(a.Assignments[0], b.Assignments[0]) })
	return listed
}

// seenLater orders assignments the one seen last first.
func seenLater(a, b status.Assignment) int {
	return b.LastSeen.Compare(a.LastSeen)
}

// seat is the assignment of the session's model, as the world lists it:
// nil where it lists none.
func (w *world) seat(session, model string) *status.Assignment {
	for i := range w.sessions {
		if w.sessions[i].ID != session {
			continue
		}
		for j := range w.sessions[i].Assignments {
			if a := &w.sessions[i].Assignments[j]; a.Model == model {
				return a
			}
		}
	}
	return nil
}

// limitOf is the limit holding the account with the given id back at at:
// one reached since the base, else the base's; reporting false where none
// does.
func (w world) limitOf(id string, at time.Time) (status.Limit, bool) {
	l, ok := w.limits[id]
	if !ok {
		l, ok = w.base.world.limits[id]
	}
	return l, ok && l.Until.After(at)
}

// room reports whether the account with the given id has room at at: no
// limit holds it back, nor a refusal of its token.
func (w world) room(id string, at time.Time) bool {
	_, held := w.limitOf(id, at)
	refused, ok := w.base.world.refused[id]
	return !held && (!ok || !refused.Until.After(at))
}

// pinned is the account of those the global pin names that new sessions go
// to at at, reporting false where none has room: the best, where the pin
// names it and it has room, as the world names the best rather than scoring
// the accounts; else the first the pin names that has room.
func (w world) pinned(at time.Time) (string, bool) {
	if w.pin.Has(w.base.best) && w.room(w.base.best, at) {
		return w.base.best, true
	}
	i := slices.IndexFunc(w.pin.Accounts, func(id string) bool { return w.room(id, at) })
	if i < 0 {
		return "", false
	}
	return w.pin.Accounts[i], true
}

// destination is the account a new session goes to at at: the one of the
// global pin's that pinned has, else the best.
func (w world) destination(at time.Time) string {
	if id, ok := w.pinned(at); ok {
		return id
	}
	return w.base.best
}

// routed is where the router sends a request of the session's model, now on
// the account a names, at at, and why, where that's another, as routedAs
// says of the session's own pin.
func (w world) routed(session string, a status.Assignment, at time.Time) (string, string) {
	pin, own := w.pins[session]
	return w.routedAs(a, pin, own, at)
}

// routedAs is where the router sends a request of a model on the account a
// names at at, and why, where that's another, of a session with its own pin
// where own says: to the account its own pin names, where that has room;
// where a pin that moves running sessions doesn't name a's, and the session
// has no pin of its own, to the global pin's, once, as the router moves a
// session only where it was put on its account before that pin was given;
// else, where a's account has no room, to where new sessions go.
func (w world) routedAs(a status.Assignment, pin ownPin, own bool, at time.Time) (string, string) {
	from := a.Account
	moving, pinning := w.pinned(at)
	switch {
	case own && pin.account != from && w.room(pin.account, at):
		return pin.account, status.ReasonPinned
	case !own && pinning && w.pin.Move && !w.pin.Has(from) && a.AssignedAt.Before(w.pin.Since):
		return moving, status.ReasonMovedByPin
	case !w.room(from, at):
		return w.destination(at), w.movedOff(from, at)
	}
	return from, ""
}

// movedOff is the router's reason for a move off the account with the given
// id at at, as it has no room: a limit holding it back, a refusal of its
// token, or else no room.
func (w world) movedOff(id string, at time.Time) string {
	why := status.WhyNoRoom
	if _, held := w.limitOf(id, at); held {
		why = status.WhyLimit
	} else if _, refused := w.base.world.refused[id]; refused {
		why = status.WhyRefused
	}
	return status.ReasonMovedOff + id + " " + why
}

// tell has the router tell of e, as the newest of its events, numbered on
// from its base's.
func (w *world) tell(e status.Event) {
	e.ID = w.base.events + len(w.events) + 1
	w.events = append(w.events, e)
}

// seated has a new session of the model given start on the account with the
// given id at at, in the directory given, for the reason given: the router
// lists it, and counts it among those sent there.
func (w *world) seated(id, session, model, dir, reason string, at time.Time) {
	w.sessions = append(w.sessions, status.Session{ID: session, Assignments: []status.Assignment{assigned(model, id, dir, reason, at, at, "")}})
	w.sent[id]++
	w.started++
}

// seen has the router see a request of the session's model arrive at at.
func (w *world) seen(session, model string, at time.Time) {
	if a := w.seat(session, model); a != nil {
		a.LastSeen = at.UTC()
	}
}

// move moves the session's model to the account with the given id at at,
// for the reason given, and the router tells of it: counted, where a limit
// the scenario reached holds back the account it left, among the sessions
// that limit moved.
func (w *world) move(session, model, to string, at time.Time, reason string) {
	a := w.seat(session, model)
	from := a.Account
	a.Account, a.Reason, a.AssignedAt, a.LastSeen = to, reason, at.UTC(), at.UTC()
	w.sent[to]++
	e := status.Event{At: at, Kind: status.EventMoved, Session: session, Model: model, From: from, To: to, Reason: reason}
	if i, ok := w.holding(from, at); ok {
		e.ForcedBy, e.Limit = w.events[i].ID, w.events[i].ID
		w.counted(i, to)
	}
	w.tell(e)
}

// holding is where among the events the newest limit the scenario reached of
// the account with the given id is, reporting false where none holds it back
// at at.
func (w world) holding(id string, at time.Time) (int, bool) {
	if l, ok := w.limits[id]; !ok || !l.Until.After(at) {
		return 0, false
	}
	for i, e := range slices.Backward(w.events) {
		if e.Kind == status.EventLimit && e.Account == id {
			return i, true
		}
	}
	return 0, false
}

// counted counts a session moved to the account with the id to among those
// the limit the i-th event tells of has moved, which all went to one account
// while each went to the same.
func (w *world) counted(i int, to string) {
	limit := &w.events[i]
	if limit.Count == 0 || limit.To == to {
		limit.To = to
	} else {
		limit.To = ""
	}
	limit.Count++
}

// limit has the account with the given id reach its 5-hour limit at at: the
// window read as used in full, the account held back until it resets, and
// the router telling of it, the sessions it moves counted as they move.
func (w *world) limit(id string, at time.Time) {
	session, _ := w.lastRead(id, fiveHourKey)
	w.reading(session, 1, at)
	newest := w.base.limits
	for _, l := range w.limits {
		newest = max(newest, l.ID)
	}
	w.limits[id] = status.Limit{ID: newest + 1, Windows: []string{fiveHourKey}, Until: session.ResetsAt.UTC()}
	w.tell(status.Event{At: at, Kind: status.EventLimit, Account: id, Windows: []string{fiveHourKey}, Until: session.ResetsAt, Limit: newest + 1})
}

// lastRead is the account's window with the given key as last read: since
// the base, else as it ran as the scenario started; reporting false where
// it was never read.
func (w world) lastRead(id, key string) (readings.Reading, bool) {
	at := windowOf{account: id, key: key}
	if r, ok := w.read[at]; ok {
		return r, true
	}
	r, ok := w.base.latest[at]
	return r, ok
}

// reading has the router read the window r was read of at at, used as
// given, in whole hundredths, as Claude gives it.
func (w *world) reading(r readings.Reading, use float64, at time.Time) {
	w.readings.at(window{account: r.Account, key: r.Key, resets: r.ResetsAt}, at, use)
	w.read[windowOf{account: r.Account, key: r.Key}] = w.readings.read[len(w.readings.read)-1]
}

// answered has the router read the use of the account with the given id off
// an answer at at, which uses more of its windows the more it streams: of
// its 5-hour window, sessionPerChar a character, and of its week, a share of
// that. A window is read again only where its use, in whole hundredths, has
// changed, as the readings history keeps it; and not at all once it has
// reset.
func (w *world) answered(id string, chars int, at time.Time) {
	used := float64(chars) * sessionPerChar
	for _, share := range []struct {
		key  string
		used float64
	}{{fiveHourKey, used}, {weekKey, used * weekPerSession}} {
		last, ok := w.lastRead(id, share.key)
		if !ok || !last.ResetsAt.After(at) {
			continue
		}
		k := windowOf{account: id, key: share.key}
		w.used[k] += share.used
		if use := hundredths(min(w.base.latest[k].Utilization+w.used[k], 1)); use != last.Utilization {
			w.reading(last, use, at)
		}
	}
}

// How much of an account's windows an answer uses, as the scenarios have it:
// of its 5-hour window, a share for each character streamed, and of its
// week, a share of that.
const (
	sessionPerChar = 1.0 / 16_000_000
	weekPerSession = 1.0 / 8
)

// setPin has new sessions go to the best of the accounts given from at,
// which with move moves running sessions to them on their next requests,
// and the router tells of it, as the dashboard set it.
func (w *world) setPin(accounts []string, move bool, at time.Time) {
	w.pin = status.Pin{Accounts: slices.Clone(accounts), Since: at, Move: move}
	w.tell(status.Event{At: at, Kind: status.EventPin, Accounts: slices.Clone(accounts), Account: accounts[0], Move: move, By: byDashboard})
}

// unpin has every session routed on its merits from at, the router telling
// of it where there was a pin to clear.
func (w *world) unpin(at time.Time) {
	if w.pin.IsZero() {
		return
	}
	w.tell(status.Event{At: at, Kind: status.EventAuto, Accounts: w.pin.Accounts, By: byDashboard})
	w.pin = status.Pin{}
}

// pinSession gives the session its own pin to the account from at, which
// moves it there on its next request, and the router tells of it.
func (w *world) pinSession(session, account string, at time.Time) {
	w.pins[session] = ownPin{account: account, at: at}
	w.tell(status.Event{At: at, Kind: status.EventPin, Account: account, Session: session, By: byDashboard})
}

// unpinSession clears the session's own pin from at, the router telling of
// it where it had one.
func (w *world) unpinSession(session string, at time.Time) {
	was, ok := w.pins[session]
	if !ok {
		return
	}
	delete(w.pins, session)
	w.tell(status.Event{At: at, Kind: status.EventAuto, Account: was.account, Session: session, By: byDashboard})
}

// byDashboard is who the router's events say gave the orders a scenario's
// keys give: the dashboard.
const byDashboard = string(router.ByDashboard)

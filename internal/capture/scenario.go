package capture

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// Scenario is a stretch of the router's life, scripted to play through the
// dashboard in real time, as the capture tool's --scenario plays it for the
// README's demos: the router as it stands as the scenario starts, and its
// cues, what befalls it from then, each at an offset from the start; on a
// terminal of a size, in a theme. It plays the same way every time: what the
// router does is set by its cues, and by the orders keys give it, and by
// nothing else.
type Scenario struct {
	// Name is what the capture tool's --scenario calls it.
	Name string
	// Size is the terminal it's made for.
	Size watch.Size
	// start is when it starts, and setting the router as it stands then,
	// built afresh each time it's played.
	start   time.Time
	setting func(start time.Time) world
	cues    []cue
	// theme is what it's drawn in: nord unless InTheme says otherwise.
	theme theme.Theme
	// colourless draws it without colour, as NO_COLOR asks.
	colourless bool
}

// InTheme is the scenario drawn in t, as the one theme chosen.
func (s Scenario) InTheme(t theme.Theme) Scenario {
	s.theme = t
	return s
}

// WithoutColour is the scenario drawn without colour, as NO_COLOR asks.
func (s Scenario) WithoutColour() Scenario {
	s.colourless = true
	return s
}

// ScenarioNames lists every scenario's name, sorted.
func ScenarioNames() []string {
	var names []string
	for _, s := range scenarios(moment(time.Local)) {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	return names
}

// ScenarioNamed returns the scenario with the given name, played in the
// local time zone, on the fixtures' day, as moment fixes it. An empty or
// unknown name is an error that lists the scenarios.
func ScenarioNamed(name string) (Scenario, error) {
	return scenarioIn(name, time.Local)
}

// scenarioIn returns the scenario with the given name, played in the time
// zone given, as moment fixes it.
func scenarioIn(name string, loc *time.Location) (Scenario, error) {
	if name == "" {
		return Scenario{}, fmt.Errorf("name a scenario (available: %s)", strings.Join(ScenarioNames(), ", "))
	}
	all := scenarios(moment(loc))
	i := slices.IndexFunc(all, func(s Scenario) bool { return s.Name == name })
	if i < 0 {
		return Scenario{}, fmt.Errorf("unknown scenario %q (available: %s)", name, strings.Join(ScenarioNames(), ", "))
	}
	return all[i], nil
}

// world is the router as a scenario has it at a moment: its accounts, each
// with the sessions on it, the account new sessions go to, what it tells of
// lately, the newest first, its global pin, and each session's own pin, by
// the session's id.
type world struct {
	samples []sample
	best    string
	events  []status.Event
	pin     status.Pin
	pins    map[string]ownPin
}

// ownPin is a session's own pin: the account it names, and when it was given.
type ownPin struct {
	account string
	at      time.Time
}

// source is the router as the world has it at now, its status document built
// then, with its global pin; and each session as its own pin, alone, has it
// listed: pinned to the account the pin names, its model there pinned since
// the pin was given, and pinned nowhere once the pin is cleared.
func (w world) source(now time.Time) source {
	s := newSource(now, w.samples, w.destination(now), w.events...)
	s.doc.GeneratedAt, s.doc.Pin = now, w.pin
	for i := range s.sessions {
		pin := w.pins[s.sessions[i].ID]
		s.sessions[i].Pin = pin.account
		for j := range s.sessions[i].Assignments {
			a := &s.sessions[i].Assignments[j]
			a.Pinned, a.PinnedAt = pin.account != "" && a.Account == pin.account, time.Time{}
			if a.Pinned {
				a.PinnedAt = pin.at
			}
		}
	}
	return s
}

// sample is the account with the given id: nil where there's none.
func (w world) sample(id string) *sample {
	i := slices.IndexFunc(w.samples, func(s sample) bool { return s.id == id })
	if i < 0 {
		return nil
	}
	return &w.samples[i]
}

// seatOf finds the seat of the session's model: the account it's on, and its
// place among the account's seats, reporting false where it's on none.
func (w world) seatOf(session, model string) (*sample, int, bool) {
	for i := range w.samples {
		if j := slices.IndexFunc(w.samples[i].seats, func(s seat) bool { return s.session == session && s.model == model }); j >= 0 {
			return &w.samples[i], j, true
		}
	}
	return nil, 0, false
}

// room reports whether the account with the given id has room at at: no
// limit of its holds it back.
func (w world) room(id string, at time.Time) bool {
	s := w.sample(id)
	return s != nil && !s.held(at)
}

// held reports whether the sample's session limit holds it back at at.
func (s sample) held(at time.Time) bool {
	return !s.session.limited.IsZero() && s.session.resets.After(at)
}

// pinned is the account of those the global pin names that new sessions go
// to at at, reporting false where none has room: the best, where the pin
// names it and it has room, as the scenario names the best rather than
// scoring the accounts; else the first the pin names that has room.
func (w world) pinned(at time.Time) (string, bool) {
	if w.pin.Has(w.best) && w.room(w.best, at) {
		return w.best, true
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
	return w.best
}

// routed is where the router sends a request of the seat on the account from,
// at at, and why, where that's another: to the account its session's own pin
// names, where that has room; where a pin that moves running sessions doesn't
// name from, and the session has no pin of its own, to the global pin's, once,
// as the router moves a session only where it was put on its account before
// that pin was given; else, where from has no room, to where new sessions go.
func (w world) routed(s seat, from string, at time.Time) (string, string) {
	pin, own := w.pins[s.session]
	moving, pinning := w.pinned(at)
	switch {
	case own && pin.account != from && w.room(pin.account, at):
		return pin.account, status.ReasonPinned
	case !own && pinning && w.pin.Move && !w.pin.Has(from) && s.assigned.Before(w.pin.Since):
		return moving, status.ReasonMovedByPin
	case !w.room(from, at):
		return w.destination(at), status.ReasonMovedOff + from + " has no room"
	}
	return from, ""
}

// tell has the router tell of e, as the newest of its events.
func (w *world) tell(e status.Event) {
	w.events = slices.Insert(w.events, 0, e)
}

// seated puts the session's model on the account with the given id, as a new
// session, at at.
func (w *world) seated(id, session, model string, at time.Time) {
	s := w.sample(id)
	s.seats = append(s.seats, seat{session: session, model: model, reason: status.ReasonNew, assigned: at, seen: at})
}

// seen has the router see the session's model at work at at.
func (w *world) seen(session, model string, at time.Time) {
	if s, i, ok := w.seatOf(session, model); ok {
		s.seats[i].seen = at
	}
}

// move moves the session's model to the account with the given id at at,
// for the reason given, and the router tells of it: counted, where a limit of
// the account it left holds it back, among the sessions that limit moved.
func (w *world) move(session, model, to string, at time.Time, reason string) {
	from, i, ok := w.seatOf(session, model)
	if !ok {
		return
	}
	moved := from.seats[i]
	from.seats = slices.Delete(from.seats, i, i+1)
	moved.reason, moved.assigned, moved.seen = reason, at, at
	dest := w.sample(to)
	dest.seats = append(dest.seats, moved)
	e := status.Event{At: at, Kind: status.EventMoved, Session: session, Model: model, From: from.id, To: to, Reason: reason}
	if from.held(at) {
		e.Limit = forced
		w.counted(from.id, to)
	}
	w.tell(e)
}

// counted counts a session moved to the account with the id to among those
// the newest limit of the account with the id from has moved, which all went
// to one account while each went to the same.
func (w *world) counted(from, to string) {
	i := slices.IndexFunc(w.events, func(e status.Event) bool { return e.Kind == status.EventLimit && e.Account == from })
	if i < 0 {
		return
	}
	limit := &w.events[i]
	if limit.Count == 0 || limit.To == to {
		limit.To = to
	} else {
		limit.To = ""
	}
	limit.Count++
}

// limit has the account with the given id reach its session limit at at: its
// session read as used in full, the account held back until it resets, and
// the router telling of it, the sessions it moves counted as they move.
func (w *world) limit(id string, at time.Time) {
	s := w.sample(id)
	newest := 0
	for _, other := range w.samples {
		newest = max(newest, other.session.limit)
	}
	s.session.used, s.session.limited, s.session.limit, s.read = 1, at, newest+1, at
	w.tell(status.Event{At: at, Kind: status.EventLimit, Account: id, Windows: []string{fiveHourKey}, Until: s.session.resets, Limit: s.session.limit})
}

// answered has the router read the use of the account with the given id off
// an answer at at, which uses more of its windows the more it streams: of its
// session, sessionPerChar a character, and of its week, a share of that.
func (w *world) answered(id string, chars int, at time.Time) {
	s := w.sample(id)
	used := float64(chars) * sessionPerChar
	s.session.used = min(s.session.used+used, 1)
	s.week.used = min(s.week.used+used*weekPerSession, 1)
	s.read = at
}

// How much of an account's windows an answer uses, as the scenarios have it:
// of its session, a share for each character streamed, and of its week, a
// share of that.
const (
	sessionPerChar = 1.0 / 800_000
	weekPerSession = 1.0 / 8
)

// cue is something befalling a scenario's router at an offset from its
// start: a request of a session's model, which the router carries out as it
// would, as the cue plays it.
type cue struct {
	at   time.Duration
	play func(r *playing, at time.Time)
}

// offset is a cue's offset, given in seconds from the scenario's start.
func offset(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// asks is a request, the given seconds into the scenario, of a running
// session's model: the router sends it to the account the session is on, or,
// where that has no room, or the session's own pin names another, there,
// the session moving with it; and it's answered as a says.
func asks(seconds float64, session, model string, a answer) cue {
	return cue{at: offset(seconds), play: func(r *playing, at time.Time) { r.ask(at, session, model, a) }}
}

// starts is a new session's first request of its model, the given seconds
// into the scenario: the router sends it to the account new sessions go to,
// where the session starts, and it's answered as a says.
func starts(seconds float64, session, model string, a answer) cue {
	return cue{at: offset(seconds), play: func(r *playing, at time.Time) { r.start(at, session, model, a) }}
}

// reaches is a request, the given seconds into the scenario, of a running
// session's model, that reaches the session limit of the account it's on:
// refused there, the account held back till its session resets, the router
// sends it on to the account new sessions go to, the session moving with it,
// and it's answered there as a says.
func reaches(seconds float64, session, model string, a answer) cue {
	return cue{at: offset(seconds), play: func(r *playing, at time.Time) { r.reach(at, session, model, a) }}
}

// answer is how a request's answer comes: its first byte wait after the
// request goes out, then streaming for streams, chars characters of it.
type answer struct {
	wait, streams time.Duration
	chars         int
}

// The answers the scenarios' requests have: a brief one, quick to come, a
// steady one, and a long one, slower.
var (
	brief  = answer{wait: 700 * time.Millisecond, streams: 1500 * time.Millisecond, chars: 2600}
	steady = answer{wait: 900 * time.Millisecond, streams: 2600 * time.Millisecond, chars: 4200}
	long   = answer{wait: 1100 * time.Millisecond, streams: 4 * time.Second, chars: 6400}
)

// How the router tells of a request: of its answer's progress every
// progressEvery while it streams, as the router does at most; and of its
// refusal at its account's limit refuseAfter it goes out.
const (
	progressEvery = 250 * time.Millisecond
	refuseAfter   = 400 * time.Millisecond
)

// charsPerToken is how many characters of an answer make a token, as the
// dashboard estimates its tokens while it streams.
const charsPerToken = 4

// order is an order a key gave a scenario's router: when it was given, and
// what it changes of the router's world.
type order struct {
	at   time.Time
	give func(w *world)
}

// playing is a scenario as it plays: its world as it stands, what its request
// stream has told, what's still to happen, in the order it happens, and how
// many requests and happenings there have been.
type playing struct {
	world    world
	told     []router.StreamEvent
	agenda   []happening
	requests int
	queued   int
}

// happening is something to happen as a scenario plays, at a moment: seq
// orders those at the same moment as they were queued.
type happening struct {
	at  time.Time
	seq int
	do  func(r *playing, at time.Time)
}

// played is the scenario played until the moment given, or, where that's
// zero, to its end: its cues, and the orders given, each as it comes.
func (s Scenario) played(orders []order, until time.Time) playing {
	r := playing{world: s.setting(s.start)}
	for _, c := range s.cues {
		r.after(s.start.Add(c.at), c.play)
	}
	for _, o := range orders {
		r.after(o.at, func(r *playing, _ time.Time) { o.give(&r.world) })
	}
	for len(r.agenda) > 0 && (until.IsZero() || !r.agenda[0].at.After(until)) {
		h := r.agenda[0]
		r.agenda = r.agenda[1:]
		h.do(&r, h.at)
	}
	return r
}

// after queues what's to happen at at, after everything queued for then
// already.
func (r *playing) after(at time.Time, do func(r *playing, at time.Time)) {
	r.queued++
	h := happening{at: at, seq: r.queued, do: do}
	i, _ := slices.BinarySearchFunc(r.agenda, h, func(a, b happening) int {
		return cmp.Or(a.at.Compare(b.at), cmp.Compare(a.seq, b.seq))
	})
	r.agenda = slices.Insert(r.agenda, i, h)
}

// tell has the request stream tell of e.
func (r *playing) tell(e router.StreamEvent) {
	r.told = append(r.told, e)
}

// request is a new request of the session's model, on the account with the
// given id, numbered as the router numbers them, one after another.
func (r *playing) request(session, model, account string) request {
	r.requests++
	return request{id: fmt.Sprintf("%08d", 400+r.requests), session: session, model: model, account: account}
}

// ask carries out a request of a running session's model at at, as asks
// says: where it goes elsewhere, the router tells of the move before the
// request first goes upstream.
func (r *playing) ask(at time.Time, session, model string, a answer) {
	on, i, ok := r.world.seatOf(session, model)
	if !ok {
		panic(fmt.Sprintf("a scenario asks of %s's %s, which no account has", status.ShortID(session), model))
	}
	to, reason := r.world.routed(on.seats[i], on.id, at)
	req := r.request(session, model, to)
	if to != on.id {
		r.world.move(session, model, to, at, reason)
		moved := req.moved(on.id, reason, at)
		moved.Attempt = 0
		r.tell(moved)
	}
	r.world.seen(session, model, at)
	r.tell(req.told(router.StreamSent, at))
	r.respond(req, 1, at, a)
}

// start carries out a new session's first request at at, as starts says, the
// router telling of the session once its answer comes.
func (r *playing) start(at time.Time, session, model string, a answer) {
	to := r.world.destination(at)
	req := r.request(session, model, to)
	r.world.seated(to, session, model, at)
	r.tell(req.told(router.StreamSent, at))
	r.respond(req, 1, at, a)
	r.after(at.Add(a.wait), func(r *playing, at time.Time) {
		r.world.tell(status.Event{At: at, Kind: status.EventStarted, Account: to, Session: session, Model: model, Reason: status.ReasonNew})
	})
}

// reach carries out a request at at that reaches its account's session
// limit, as reaches says: the router tells of the limit, of the session's
// move, and of the request going upstream again, all as the refusal comes.
func (r *playing) reach(at time.Time, session, model string, a answer) {
	on, _, ok := r.world.seatOf(session, model)
	if !ok {
		panic(fmt.Sprintf("a scenario has %s's %s reach a limit, which no account has", status.ShortID(session), model))
	}
	from := on.id
	req := r.request(session, model, from)
	r.world.seen(session, model, at)
	r.tell(req.told(router.StreamSent, at))
	r.after(at.Add(refuseAfter), func(r *playing, at time.Time) {
		limited := req.told(router.StreamLimited, at)
		limited.Status = 429
		r.tell(limited)
		r.world.limit(from, at)
		reason := status.ReasonMovedOff + from + " hit its limit"
		again := req
		again.account = r.world.destination(at)
		r.world.move(session, model, again.account, at, reason)
		r.tell(again.moved(from, reason, at))
		sent := again.told(router.StreamSent, at)
		sent.Attempt = 2
		r.tell(sent)
		r.respond(again, 2, at, a)
	})
}

// respond has the answer to the request, on the attempt given, come as a
// says, from at: its first byte, which carries its account's use; its
// progress, every progressEvery; and its end, its tokens counted.
func (r *playing) respond(req request, attempt int, at time.Time, a answer) {
	first, done := at.Add(a.wait), at.Add(a.wait+a.streams)
	event := func(kind string, at time.Time) router.StreamEvent {
		e := req.told(kind, at)
		e.Attempt = attempt
		return e
	}
	r.after(first, func(r *playing, at time.Time) {
		r.tell(event(router.StreamFirst, at))
		r.world.answered(req.account, a.chars, at)
	})
	for t := first.Add(progressEvery); t.Before(done); t = t.Add(progressEvery) {
		chars := int(int64(a.chars) * int64(t.Sub(first)) / int64(a.streams))
		r.after(t, func(r *playing, at time.Time) {
			e := event(router.StreamProgress, at)
			e.Chars = chars
			r.tell(e)
		})
	}
	r.after(done, func(r *playing, at time.Time) {
		e := event(router.StreamDone, at)
		e.Status, e.Chars, e.Tokens = 200, a.chars, &quota.Tokens{Output: a.chars / charsPerToken}
		r.tell(e)
		r.world.seen(req.session, req.model, at)
	})
}

// moved is the request stream's event of the request's session moving from
// the account with the id from to the request's, at at, for the reason given.
func (q request) moved(from, reason string, at time.Time) router.StreamEvent {
	e := q.told(router.StreamMoved, at)
	e.From, e.To, e.Reason = from, q.account, reason
	return e
}

// flying are the requests in flight at at among those the events told of,
// as the router keeps them for a reader joining its stream then: each as it
// last went upstream, with its answer's first byte, how much of it has
// streamed, and the verdict on it, where they've come; the first sent first.
func flying(told []router.StreamEvent, at time.Time) []router.StreamEvent {
	flights := make(map[string]router.StreamEvent)
	for _, e := range told {
		if e.At.After(at) {
			break
		}
		f, ok := flights[e.Request]
		switch {
		case e.Kind == router.StreamSent:
			e.Kind, e.SentAt = router.StreamInFlight, e.At
			flights[e.Request] = e
		case e.Kind == router.StreamDone:
			delete(flights, e.Request)
		case !ok:
		case e.Kind == router.StreamFirst:
			f.FirstAt, f.Account = e.At, e.Account
			flights[e.Request] = f
		case e.Kind == router.StreamProgress:
			f.Chars = e.Chars
			flights[e.Request] = f
		case e.Kind == router.StreamLimited, e.Kind == router.StreamThrottled, e.Kind == router.StreamRefused:
			f.Verdict, f.Status = e.Kind, e.Status
			flights[e.Request] = f
		}
	}
	inFlight := make([]router.StreamEvent, 0, len(flights))
	for _, f := range flights {
		f.At = at.UTC()
		inFlight = append(inFlight, f)
	}
	slices.SortFunc(inFlight, func(a, b router.StreamEvent) int {
		return cmp.Or(a.SentAt.Compare(b.SentAt), cmp.Compare(a.Request, b.Request))
	})
	return inFlight
}

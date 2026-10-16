package capture

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// Scenario is a stretch of the router's life, scripted to play through the
// dashboard in real time, as the capture tool's --scenario plays it for the
// README's demos: one of the frames' worlds as the scenario starts, and its
// cues, what befalls the router from then, each at an offset from the start;
// on a terminal of a size, in a theme. It plays the same way every time: what
// the router does is set by its cues, and by the orders keys give it, and by
// nothing else.
type Scenario struct {
	// Name is what the capture tool's --scenario calls it.
	Name string
	// Size is the terminal it's made for.
	Size watch.Size
	// worlds are the worlds it's played over, and world the one it starts
	// in, from how long after their moment from says.
	worlds func() accountWorlds
	world  func(accountWorlds) accountWorld
	from   time.Duration
	cues   []cue
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

// ScenarioNames lists every scenario's name, sorted, building no world.
func ScenarioNames() []string {
	var names []string
	for _, s := range scenarios() {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	return names
}

// ScenarioNamed returns the scenario with the given name, played over the
// worlds of the frames' moment in the local time zone, as localMoment has it.
// An empty or unknown name is an error that lists the scenarios.
func ScenarioNamed(name string) (Scenario, error) {
	if name == "" {
		return Scenario{}, fmt.Errorf("name a scenario (available: %s)", strings.Join(ScenarioNames(), ", "))
	}
	all := scenarios()
	i := slices.IndexFunc(all, func(s Scenario) bool { return s.Name == name })
	if i < 0 {
		return Scenario{}, fmt.Errorf("unknown scenario %q (available: %s)", name, strings.Join(ScenarioNames(), ", "))
	}
	return all[i], nil
}

// --------------------------------------------------------- the base

// base is a scenario's world as it starts, built once, before its clock
// starts, as a year of readings takes a while: the world, kept, and its
// records, which are read in place as the scenario plays, never copied; the
// start; each of its windows as it runs then, the readings of it since it
// started and the last of them, which are all a document of it reads; the
// sessions it lists then, and their own pins; what its request stream had
// told by then; the account it names the best; and the cues, the scenario's
// and those that land what was in flight as it started. How many sessions
// it had sent in the last hour, its newest event's id, its newest limit's
// identity and the highest id its lines and stream use number what follows;
// and tells, how much its stream tells, played to its end, sizes a play's.
type base struct {
	world    accountWorld
	records  *records
	start    time.Time
	current  history
	latest   map[windowOf]readings.Reading
	listed   []status.Session
	pins     map[string]ownPin
	stream   []router.StreamEvent
	best     string
	cues     []cue
	sessions int
	events   int
	limits   int
	requests uint64
	tells    int
}

// windowOf names a window of an account: the account's id, and the window's
// key.
type windowOf struct {
	account, key string
}

// ownPin is a session's own pin: the account it names, and when it was given.
type ownPin struct {
	account string
	at      time.Time
}

// based is the scenario's base: its world of its worlds, as it stands from
// after their moment.
func (s Scenario) based() *base {
	worlds := s.worlds()
	w, start := s.world(worlds), worlds.now.Add(s.from)
	doc := w.document(start)
	b := &base{
		world: w, records: w.records(), start: start, best: doc.Best, sessions: doc.Sessions,
		stream: slices.DeleteFunc(slices.Clone(w.stream), func(e router.StreamEvent) bool { return e.At.After(start) }),
	}
	b.current, b.latest = windowsAsTheyRun(w.history)
	if w.routed != nil {
		b.listed = w.routed(start)
	}
	b.pins = make(map[string]ownPin)
	for _, l := range b.listed {
		if i := slices.IndexFunc(l.Assignments, func(a status.Assignment) bool { return a.Pinned }); i >= 0 {
			b.pins[l.ID] = ownPin{account: l.Pin, at: l.Assignments[i].PinnedAt}
		}
	}
	for _, e := range w.events {
		b.events = max(b.events, e.ID)
		if e.Kind == status.EventLimit {
			b.limits = max(b.limits, e.Limit)
		}
	}
	for _, l := range w.limits {
		b.limits = max(b.limits, l.ID)
	}
	for _, e := range b.stream {
		b.requests = max(b.requests, requestNumber(e.Request))
	}
	for _, f := range b.records.lines {
		b.requests = max(b.requests, requestNumber(f.Request))
	}
	b.cues = slices.Concat(landings(b.stream, start), s.cues)
	b.tells = len(b.played(nil, time.Time{}).told)
	return b
}

// windowsAsTheyRun are the readings of each window of h as it runs at the end of h,
// in the order they were read, and the last of each, by the window: what a
// document reads of a window, its trail starting afresh at each reset, as
// score.Noted has it.
func windowsAsTheyRun(h history) (history, map[windowOf]readings.Reading) {
	latest := make(map[windowOf]readings.Reading)
	for _, r := range h {
		latest[windowOf{account: r.Account, key: r.Key}] = r
	}
	var running history
	for _, r := range h {
		if r.ResetsAt.Equal(latest[windowOf{account: r.Account, key: r.Key}].ResetsAt) {
			running = append(running, r)
		}
	}
	return running, latest
}

// requestNumber is the router's request id as the number it counts from, as
// it gives them, in hex: 0 for one that isn't.
func requestNumber(id string) uint64 {
	n, err := strconv.ParseUint(id, 16, 64)
	if err != nil {
		return 0
	}
	return n
}

// landWithin is how long into a scenario the requests in flight as it
// starts take to end, one after another.
const landWithin = 3 * time.Second

// landings are cues that land each request in flight among those told as
// the scenario starts at start, the first sent first, the last by
// landWithin.
func landings(told []router.StreamEvent, start time.Time) []cue {
	flights := flying(told, start)
	var cues []cue
	for i, f := range flights {
		ends := landWithin * time.Duration(i+1) / time.Duration(len(flights))
		cues = append(cues, cue{play: func(r *playing, at time.Time) { r.land(f, at.Add(ends)) }})
	}
	return cues
}

// --------------------------------------------------------- the cues

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
// where that has no room, or a pin sends it elsewhere, there, the session
// moving with it; and it's answered as a says.
func asks(seconds float64, session, model string, a answer) cue {
	return cue{at: offset(seconds), play: func(r *playing, at time.Time) { r.ask(at, session, model, a) }}
}

// starts is a new session's first request of its model, the given seconds
// into the scenario, in the directory given: the router sends it to the
// account new sessions go to, where the session starts, and it's answered as
// a says.
func starts(seconds float64, session, model, dir string, a answer) cue {
	return cue{at: offset(seconds), play: func(r *playing, at time.Time) { r.start(at, session, model, dir, a) }}
}

// reaches is a request, the given seconds into the scenario, of a running
// session's model, that reaches the 5-hour limit of the account it's on:
// refused there, the account held back till its window resets, the router
// sends it on to the account new sessions go to as the refusal's pulse comes
// home, the session moving with it, and it's answered there as a says.
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
// steady one, a long one, slower, and a lengthy one, streaming a while.
var (
	brief   = answer{wait: 700 * time.Millisecond, streams: 1500 * time.Millisecond, chars: 2600}
	steady  = answer{wait: 900 * time.Millisecond, streams: 2600 * time.Millisecond, chars: 4200}
	long    = answer{wait: 1100 * time.Millisecond, streams: 4 * time.Second, chars: 6400}
	lengthy = answer{wait: 400 * time.Millisecond, streams: 14 * time.Second, chars: 12800}
)

// How the router tells of a request: of its answer's progress every
// progressEvery while it streams, as the router does at most; of its refusal
// at its account's limit refuseAfter it goes out, as the frames at
// personal's limit have it; and of its move elsewhere, and its going out
// there, retryAfter its refusal, as the refusal's pulse comes home, as those
// frames have it, 41e0 on personal still 178ms after, on side 620ms after.
const (
	progressEvery = 250 * time.Millisecond
	refuseAfter   = 300 * time.Millisecond
	retryAfter    = 540 * time.Millisecond
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

// --------------------------------------------------------- playing

// playing is a scenario as it plays: its world as it stands, what its request
// stream has told, what's still to happen, in the order it happens, and how
// many requests and happenings there have been.
type playing struct {
	world    world
	told     []router.StreamEvent
	agenda   []happening
	requests uint64
	queued   int
}

// happening is something to happen as a scenario plays, at a moment: seq
// orders those at the same moment as they were queued.
type happening struct {
	at  time.Time
	seq int
	do  func(r *playing, at time.Time)
}

// played is the scenario of the base played until the moment given, or,
// where that's zero, to its end: its cues, and the orders given, each as it
// comes, over its world as it started.
func (b *base) played(orders []order, until time.Time) playing {
	r := playing{world: b.startingOut(), told: append(make([]router.StreamEvent, 0, b.tells), b.stream...)}
	for _, c := range b.cues {
		r.after(b.start.Add(c.at), c.play)
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
// given id, numbered as the router numbers them, one after another, after
// every request its world's lines and stream number.
func (r *playing) request(session, model, account string) request {
	r.requests++
	return request{id: fmt.Sprintf("%08x", r.world.base.requests+r.requests), session: session, model: model, account: account}
}

// seat is the assignment of the session's model, panicking, as a scenario
// does what can't be, where its world lists none: saying what the scenario
// does of it, as does words it of the session's short id and the model.
func (r *playing) seat(session, model, does string) *status.Assignment {
	a := r.world.seat(session, model)
	if a == nil {
		panic(fmt.Sprintf(does+", which no account has", status.ShortID(session), model))
	}
	return a
}

// ask carries out a request of a running session's model at at, as asks
// says: where it goes elsewhere, the router tells of the move before the
// request first goes upstream.
func (r *playing) ask(at time.Time, session, model string, a answer) {
	on := *r.seat(session, model, "a scenario asks of %s's %s")
	to, reason := r.world.routed(session, on, at)
	req := r.request(session, model, to)
	if to != on.Account {
		r.world.move(session, model, to, at, reason)
		moved := req.moved(on.Account, reason, at)
		moved.Attempt = 0
		r.tell(moved)
	}
	r.world.seen(session, model, at)
	r.tell(req.told(router.StreamSent, at))
	r.respond(req, 1, at, a)
}

// start carries out a new session's first request at at, as starts says, the
// router telling of the session once its answer comes.
func (r *playing) start(at time.Time, session, model, dir string, a answer) {
	to, reason := r.world.destination(at), status.ReasonNew
	if _, pinned := r.world.pinned(at); pinned {
		reason = status.ReasonGlobalPin
	}
	req := r.request(session, model, to)
	r.world.seated(to, session, model, dir, reason, at)
	r.tell(req.told(router.StreamSent, at))
	r.respond(req, 1, at, a)
	r.after(at.Add(a.wait), func(r *playing, at time.Time) {
		r.world.tell(status.Event{At: at, Kind: status.EventStarted, Account: to, Session: session, Model: model, Reason: reason})
	})
}

// reach carries out a request at at that reaches its account's 5-hour
// limit, as reaches says: the router tells of the refusal, and of the limit,
// as the refusal comes; then of the session's move, and of the request going
// upstream again, as its pulse comes home.
func (r *playing) reach(at time.Time, session, model string, a answer) {
	from := r.seat(session, model, "a scenario has %s's %s reach a limit").Account
	req := r.request(session, model, from)
	r.world.seen(session, model, at)
	r.tell(req.told(router.StreamSent, at))
	r.after(at.Add(refuseAfter), func(r *playing, at time.Time) {
		limited := req.told(router.StreamLimited, at)
		limited.Status = 429
		r.tell(limited)
		r.world.limit(from, at)
		r.after(at.Add(retryAfter), func(r *playing, at time.Time) {
			reason := r.world.movedOff(from, at)
			again := req
			again.account = r.world.destination(at)
			r.world.move(session, model, again.account, at, reason)
			r.tell(again.moved(from, reason, at))
			r.tell(again.attempt(router.StreamSent, at, 2))
			r.respond(again, 2, at, a)
		})
	})
}

// land ends a request in flight as the scenario starts, f as the stream
// tells of it then, at ends: its answer's first byte coming a third of the
// way there, where it hasn't, and read off for its account's use; streaming
// a brief answer more, from what it had, till then.
func (r *playing) land(f router.StreamEvent, ends time.Time) {
	req := request{id: f.Request, session: f.Session, model: f.Model, account: f.Account}
	if f.FirstAt.IsZero() {
		wait := ends.Sub(f.At) / 3
		r.respond(req, f.Attempt, f.At, answer{wait: wait, streams: ends.Sub(f.At) - wait, chars: brief.chars})
		return
	}
	r.streams(req, f.Attempt, f.At, ends.Sub(f.At), f.Chars, brief.chars)
}

// respond has the answer to the request, on the attempt given, come as a
// says, from at: its first byte, which carries its account's use; then its
// stream, as streams has it.
func (r *playing) respond(req request, attempt int, at time.Time, a answer) {
	first := at.Add(a.wait)
	r.after(first, func(r *playing, at time.Time) {
		r.tell(req.attempt(router.StreamFirst, at, attempt))
		r.world.answered(req.account, a.chars, at)
	})
	r.streams(req, attempt, first, a.streams, 0, a.chars)
}

// streams has the answer to the request, on the attempt given, stream from
// from for d, chars characters more than the had it had: its progress every
// progressEvery, and its end, every character counted, its tokens with them.
func (r *playing) streams(req request, attempt int, from time.Time, d time.Duration, had, chars int) {
	done := from.Add(d)
	for t := from.Add(progressEvery); t.Before(done); t = t.Add(progressEvery) {
		sofar := had + int(int64(chars)*int64(t.Sub(from))/int64(d))
		r.after(t, func(r *playing, at time.Time) {
			e := req.attempt(router.StreamProgress, at, attempt)
			e.Chars = sofar
			r.tell(e)
		})
	}
	r.after(done, func(r *playing, at time.Time) {
		e := req.attempt(router.StreamDone, at, attempt)
		e.Status, e.Chars, e.Tokens = 200, had+chars, &quota.Tokens{Output: (had + chars) / charsPerToken}
		r.tell(e)
	})
}

// attempt is the request stream's event of the kind given of the request,
// at a time, on the attempt given.
func (q request) attempt(kind string, at time.Time, attempt int) router.StreamEvent {
	e := q.told(kind, at)
	e.Attempt = attempt
	return e
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

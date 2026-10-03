package watch

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
)

// How what travels the cords moves: a pulse runs along a cord in pulseFor;
// the shimmer down a cord whose answer streams moves a cell each
// shimmerStep; a request's answer, or the refusal that ended it, shows
// answeredFor once it ends; and a cord a move let go fades over looseFor.
const (
	pulseFor    = 540 * time.Millisecond
	shimmerStep = 80 * time.Millisecond
	answeredFor = 2 * time.Second
	looseFor    = 3 * time.Second
)

// charsPerToken is how many of the characters the stream counts of an
// answer make a token, as its tokens are estimated while it streams.
const charsPerToken = 4

// seenFor is how long the watch keeps when the stream last saw a seat: the
// router lists the sessions it has seen in the last hour, so one it lists
// after that it has seen since, as its own listing says.
const seenFor = time.Hour

// traffic is what the router's request stream has told of the routed
// requests, as the watch keeps it for the frame: what each request is doing
// on each account it went out on; when the stream last told of each seat on
// its account; the moves the stream told of, while they show; when the
// router's sessions were last listed, which ends a move's re-patch; and
// whether it's live, the stream having opened since the watch last forgot
// what it told. Its maps and moves are its own: what changes it copies them
// first.
type traffic struct {
	calls  map[callKey]call
	seen   map[dashboard.Plug]time.Time
	moves  []move
	listed time.Time
	live   bool
}

// callKey names a request on an account it went out on: its id, and the
// account's.
type callKey struct {
	request, account string
}

// call is what a request is doing on an account, as the stream last told of
// it: the seat it's of; what it's doing, and since when it has, at; when it
// went out; how many characters of its answer have streamed, and the tokens
// its closing usage counts, where exact is set; the upstream's answer that
// refused or throttled it, and whether that was its limit; when it ended
// there, zero while it's in flight; and whether a move brought it there,
// showing once shows has come.
type call struct {
	seat          dashboard.Seat
	doing         dashboard.Doing
	at, since     time.Time
	chars, tokens int
	exact         bool
	status        int
	limited       bool
	ended         time.Time
	brought       bool
	shows         time.Time
}

// move is a move the stream told of, as the frame draws it, and when it
// shows: at once, or once the refusal that moved it has bounced back along
// its cord.
type move struct {
	dashboard.Move
	shows time.Time
}

// took is the traffic once it has taken in the events heard, in turn, as
// hear takes each, held saying whether a limit holds the account with the
// given id back; tidied of what has ended by now.
func (t traffic) took(events []router.StreamEvent, held func(account string) bool, now time.Time) traffic {
	t.calls, t.seen, t.moves = cloned(t.calls), cloned(t.seen), slices.Clone(t.moves)
	for _, e := range events {
		t.hear(e, held)
	}
	return t.tidied(now)
}

// cloned is a copy of m to change, made where m is nil.
func cloned[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return make(map[K]V)
	}
	return maps.Clone(m)
}

// hear takes in the event e, of a request of a session's model, but the
// client's quota check's, which isn't drawn, noting the stream saw the seat
// then, on the account the request goes out on: in flight as the stream
// opens, gone out, its answer's first byte, its progress, its end, its
// limit, a refusal, its throttling, or its session's move, as each kind
// says. Gone out on an account, or done there, the request has left every
// other, as outOn says.
func (t *traffic) hear(e router.StreamEvent, held func(account string) bool) {
	if e.Check || e.Session == "" {
		return
	}
	plug := dashboard.Plug{Account: e.Account, Seat: seatTold(e)}
	t.seen[plug] = latest(t.seen[plug], e.At)
	switch e.Kind {
	case router.StreamInFlight:
		t.inFlight(e)
	case router.StreamSent:
		t.outOn(e.Request, e.Account, e.At)
		t.sent(e)
	case router.StreamFirst:
		t.told(e, func(c *call) { c.doing, c.at, c.brought = dashboard.Streaming, e.At, false })
	case router.StreamProgress:
		t.told(e, func(c *call) { c.chars = max(c.chars, e.Chars) })
	case router.StreamDone:
		t.outOn(e.Request, e.Account, e.At)
		t.told(e, func(c *call) { c.end(e) })
	case router.StreamLimited, router.StreamRefused:
		t.told(e, func(c *call) {
			c.doing, c.at, c.status, c.limited = dashboard.Refused, e.At, e.Status, e.Kind == router.StreamLimited
		})
	case router.StreamThrottled:
		t.told(e, func(c *call) { c.doing, c.at, c.status = dashboard.Throttled, e.At, e.Status })
	case router.StreamMoved:
		t.moved(e, held)
	}
}

// key names the request e tells of on the account it goes out on.
func key(e router.StreamEvent) callKey {
	return callKey{request: e.Request, account: e.Account}
}

// seatTold is the seat of the request e tells of: its session's model.
func seatTold(e router.StreamEvent) dashboard.Seat {
	return dashboard.Seat{Session: e.Session, Model: e.Model}
}

// inFlight takes in a request in flight as the stream opens: its answer
// streaming, where its first byte has come, else asking.
func (t *traffic) inFlight(e router.StreamEvent) {
	c := call{seat: seatTold(e), doing: dashboard.Asking, at: e.SentAt, since: e.SentAt}
	if !e.FirstAt.IsZero() {
		c.doing, c.at, c.chars = dashboard.Streaming, e.FirstAt, e.Chars
	}
	t.calls[key(e)] = c
}

// sent takes in a request gone out on an account: asking, from when it
// went, or where a move brought it there, from when that shows.
func (t *traffic) sent(e router.StreamEvent) {
	k := key(e)
	before := t.calls[k]
	c := call{seat: seatTold(e), doing: dashboard.Asking, at: e.At, since: e.At, brought: before.brought, shows: before.shows}
	if c.brought {
		c.at = latest(e.At, c.shows)
	}
	t.calls[k] = c
}

// told changes the call of the request e tells of, on the account it goes
// out on, as change says; a call left doing nothing, as a request that
// ended unanswered, is gone.
func (t *traffic) told(e router.StreamEvent, change func(*call)) {
	k := key(e)
	c := t.calls[k]
	c.seat = seatTold(e)
	change(&c)
	if c.doing == 0 {
		delete(t.calls, k)
		return
	}
	t.calls[k] = c
}

// outOn takes in the request with the given id being out on the account
// with the given id at at. A request is out on one account at a time, so
// its calls on every other end, those yet to, each once it's done with, as
// done says: a request that left an account without its own move told of,
// as when its session moved for another request of its, or by a pin, leaves
// nothing there.
func (t *traffic) outOn(request, account string, at time.Time) {
	for k, c := range t.calls {
		if k.request == request && k.account != account && c.ended.IsZero() {
			c.ended = c.done(at)
			t.calls[k] = c
		}
	}
}

// done is when the call is done with, its request gone on from it at at:
// then, or where it was refused, once its refusal has bounced back along its
// cord.
func (c call) done(at time.Time) time.Time {
	if c.doing == dashboard.Refused {
		return latest(at, c.at.Add(pulseFor))
	}
	return at
}

// end takes in the end of the call's request, as e tells: refused, it stays
// so; answered with success, its answer has just ended, its characters and
// its tokens as e counts them; otherwise there's nothing to show of it.
func (c *call) end(e router.StreamEvent) {
	c.ended = e.At
	switch {
	case c.doing == dashboard.Refused:
	case e.Status >= 200 && e.Status < 300:
		c.doing, c.at, c.chars = dashboard.Answered, e.At, max(c.chars, e.Chars)
		if e.Tokens != nil {
			c.tokens, c.exact = e.Tokens.Output, true
		}
	default:
		c.doing = 0
	}
}

// moved takes in a session's move e tells of, of a request's model: shown
// once the request is done with on the account it left, as done says; held
// where a limit moved it, as the refusal there was its limit, or held says a
// limit holds that account back; and the request brought where it went, for
// when it goes out there, as it does at once, or else gone with the call it
// left.
func (t *traffic) moved(e router.StreamEvent, held func(account string) bool) {
	left := t.calls[callKey{request: e.Request, account: e.From}]
	shows := left.done(e.At)
	t.moves = append(t.moves, move{Seat: seatTold(e), From: e.From, To: e.To, At: e.At, Reason: e.Reason, Held: left.limited || held(e.From), shows: shows})
	t.outOn(e.Request, e.To, e.At)
	t.calls[callKey{request: e.Request, account: e.To}] = call{seat: seatTold(e), brought: true, shows: shows, ended: shows}
}

// tidied is the traffic without what has ended by now: the calls whose
// requests ended answeredFor ago, the moves that no longer show, and when
// the stream saw each seat, seenFor ago.
func (t traffic) tidied(now time.Time) traffic {
	t.calls, t.seen = maps.Clone(t.calls), maps.Clone(t.seen)
	maps.DeleteFunc(t.calls, func(_ callKey, c call) bool { return c.over(now) })
	maps.DeleteFunc(t.seen, func(_ dashboard.Plug, at time.Time) bool { return now.Sub(at) >= seenFor })
	t.moves = slices.DeleteFunc(slices.Clone(t.moves), func(m move) bool { return m.over(now, t.listed) })
	return t
}

// over reports whether the call has nothing more to show at now: its request
// ended answeredFor ago.
func (c call) over(now time.Time) bool {
	return !c.ended.IsZero() && now.Sub(c.ended) >= answeredFor
}

// over reports whether the move shows no more at now, with the router's
// sessions last listed at listed: they've been listed since it, and a limit
// moved it, so the router's document's stubs stand for its cord, or its
// cord has faded.
func (m move) over(now, listed time.Time) bool {
	return listed.After(m.At) && (m.Held || now.Sub(m.shows) >= looseFor)
}

// at is the traffic as the frame draws it at now: live or not; what each
// seat is doing on each account, as shown says of the calls there, and what
// travels its cord; when the stream last saw each there; and each move once
// it shows, but those over.
func (t traffic) at(now time.Time) dashboard.Traffic {
	type keyed struct {
		key  callKey
		call call
	}
	shown := make(map[dashboard.Plug]keyed)
	for k, c := range t.calls {
		p := dashboard.Plug{Account: k.account, Seat: c.seat}
		if was, ok := shown[p]; c.doing != 0 && !c.over(now) && (!ok || c.shownOver(was.call, k.request, was.key.request)) {
			shown[p] = keyed{key: k, call: c}
		}
	}
	drawn := dashboard.Traffic{Live: t.live, Seen: t.seen}
	for p, s := range shown {
		if drawn.Calls == nil {
			drawn.Calls = make(map[dashboard.Plug]dashboard.Call)
		}
		drawn.Calls[p] = s.call.drawn(now)
	}
	for _, m := range t.moves {
		if !now.Before(m.shows) && !m.over(now, t.listed) {
			drawn.Moves = append(drawn.Moves, m.drawn(now, t.listed))
		}
	}
	return drawn
}

// shownOver reports whether a seat's call c, of the request with the id
// mine, is shown on its account over another of its calls there, of the
// request with the id theirs, as two of its requests are at once: one in
// flight over one that isn't; else the one that began doing what it's doing
// last; else, to settle it, the one whose id sorts last.
func (c call) shownOver(other call, mine, theirs string) bool {
	switch {
	case c.doing.InFlight() != other.doing.InFlight():
		return c.doing.InFlight()
	case !c.at.Equal(other.at):
		return c.at.After(other.at)
	default:
		return mine > theirs
	}
}

// drawn is the call as the frame draws it at now: what it's doing, its
// tokens, estimated at charsPerToken characters a token until its closing
// usage counts them, and what travels its cord. A pulse runs along it for
// pulseFor: out to the jack as its request goes; back to the call as its
// answer ends; and back, red, as its request is refused. While its answer
// streams, its shimmer moves on a step each shimmerStep.
func (c call) drawn(now time.Time) dashboard.Call {
	d := dashboard.Call{
		Doing: c.doing, Since: c.since, Tokens: c.chars / charsPerToken, Exact: c.exact,
		Status: c.status, New: c.brought && c.doing == dashboard.Asking,
	}
	if c.exact {
		d.Tokens = c.tokens
	}
	elapsed := now.Sub(c.at)
	d.Pulsing = elapsed >= 0 && elapsed < pulseFor
	d.Pulse.Along = progress(elapsed, pulseFor)
	switch c.doing {
	case dashboard.Answered:
		d.Pulse.Back = true
	case dashboard.Refused:
		d.Pulse.Back, d.Pulse.Red = true, true
	case dashboard.Streaming:
		d.Pulsing, d.Shimmer = false, int(max(elapsed, 0)/shimmerStep)
	case dashboard.Throttled:
		d.Pulsing = false
	}
	return d
}

// drawn is the move as the frame draws it at now, with the router's sessions
// last listed at listed: re-patching until they've been listed since it;
// and its cord, but where a limit moved it, faded as far as it has since it
// showed.
func (m move) drawn(now, listed time.Time) dashboard.Move {
	d := m.Move
	d.Listed = listed.After(m.At)
	if !d.Held {
		d.Fade = progress(now.Sub(m.shows), looseFor)
	}
	return d
}

// moving reports whether anything the traffic draws moves at now: a pulse
// running along a cord, a cord's shimmer while its answer streams, a move
// yet to show, or a cord a move let go, fading.
func (t traffic) moving(now time.Time) bool {
	for _, c := range t.calls {
		elapsed := now.Sub(c.at)
		if c.doing == dashboard.Streaming || (c.doing != 0 && c.doing != dashboard.Throttled && elapsed >= 0 && elapsed < pulseFor) {
			return true
		}
	}
	return slices.ContainsFunc(t.moves, func(m move) bool {
		return now.Before(m.shows) || (!m.Held && now.Sub(m.shows) < looseFor)
	})
}

// listedAt is the traffic once the router's sessions have been listed as
// they stood at at, which ends the re-patch of every move told of before.
func (t traffic) listedAt(at time.Time) traffic {
	t.listed = at
	return t
}

// afresh is the traffic as the watch forgets what the stream told of the
// requests, closing it, or opening it again: no longer live, but keeping
// when the stream last saw each seat, which holds all the same, and when
// the router's sessions were last listed.
func (t traffic) afresh() traffic {
	return traffic{seen: t.seen, listed: t.listed}
}

// latest is the later of two times.
func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

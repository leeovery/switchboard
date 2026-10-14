package watch

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// How what travels the cords moves: a pulse runs along a cord in pulseFor,
// as a request goes out, as its answer ends, and as it's refused, the move a
// refusal brings showing as that pulse comes home; the shimmer down a cord
// whose answer streams moves a cell as the clock passes each whole
// shimmerStep, every cord's in step; and a cord a move no limit forced let
// go fades over looseFor.
const (
	pulseFor    = 540 * time.Millisecond
	shimmerStep = 80 * time.Millisecond
	looseFor    = 3 * time.Second
)

// seenFor is how long the watch keeps a request the stream told of that's
// no longer in flight, and a move it told of: the router lists the sessions
// it has seen in the last IdleHour, so one it lists after that it has seen
// since, as its own listing says.
const seenFor = views.IdleHour

// traffic is what the router's request stream has told of the routed
// requests, as the watch keeps it for the frame: each request it told of,
// as the views read it, by its id; what each request is doing on each
// account it went out on, as the cords draw it; the moves the stream told
// of, for seenFor; the account the router's last listing of its
// sessions put each seat on, which shows each move there done, so the views
// no longer read it; whether it's live, the stream open and telling what's
// in flight, as the views trust it; and, the stream opened again, the
// requests it told of in flight before, by their ids, awaiting its first
// look, as join has it: nil while none does; and what hears the views'
// stream of it once as what it told changes, rather than at each frame,
// shared by every copy: nil where each look hears it afresh. Its maps and
// moves are its own: what changes it copies them first.
type traffic struct {
	requests map[string]views.Told
	calls    map[callKey]call
	moves    []move
	placed   map[seat]string
	live     bool
	awaiting map[string]bool
	heard    *views.Hearing
}

// seat is a session's requests of one model, each by its id.
type seat struct {
	session, model string
}

// callKey names a request on an account it went out on: its id, and the
// account's.
type callKey struct {
	request, account string
}

// call is what a request is doing on an account, as the stream last told of
// it: the seat it's of; what it's doing, and since when it has, at; whether
// the upstream refused it there for its limit; when it was done with there,
// zero while it's in flight; and whether a move brought it there, showing
// once shows has come.
type call struct {
	seat    seat
	doing   dashboard.Doing
	at      time.Time
	limited bool
	ended   time.Time
	brought bool
	shows   time.Time
}

// move is a move the stream told of: the seat it moved, from the account
// with one id to the account with another, when, and why, as the router's
// reason gives it; whether a listing of the router's sessions shows it done,
// and whether a limit moved it, so its cord hangs as the limit's stub rather
// than loose; the request that moved it, by its id; and when it shows: at
// once, or once the refusal that moved it has bounced back along its cord.
type move struct {
	seat         seat
	from, to     string
	at           time.Time
	reason       string
	listed, held bool
	request      string
	shows        time.Time
}

// took is the traffic once it has taken in the events heard, in turn, as
// join, tell and hear take each, held saying whether a limit holds the
// account with the given id back, each move among them done where the
// router's last listing shows it so; tidied of what has ended by now, as
// tidy has it.
func (t traffic) took(events []router.StreamEvent, held func(account string) bool, now time.Time) traffic {
	t.requests, t.calls, t.moves = cloned(t.requests), cloned(t.calls), slices.Clone(t.moves)
	t.awaiting = maps.Clone(t.awaiting)
	for _, e := range events {
		t.join(e)
		t.tell(e)
		t.hear(e, held)
	}
	t.moves = shownDone(t.moves, t.placed)
	t.tidy(now)
	return t
}

// cloned is a copy of m to change, made where m is nil.
func cloned[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return make(map[K]V)
	}
	return maps.Clone(m)
}

// hear takes in the event e, of a request of a session's model, but the
// client's quota check's, which isn't drawn, on the account the request goes
// out on: in flight as the stream opens, gone out, its answer's first byte,
// its end, its limit, a refusal, its throttling, or its session's move, as
// each kind says. Gone out on an account, or done there, the request has left
// every other, as outOn says.
func (t *traffic) hear(e router.StreamEvent, held func(account string) bool) {
	if e.Check || e.Session == "" {
		return
	}
	switch e.Kind {
	case router.StreamInFlight:
		t.inFlight(e)
	case router.StreamSent:
		t.outOn(e.Request, e.Account, e.At)
		t.sent(e)
	case router.StreamFirst:
		t.told(e, func(c *call) { c.first(e) })
	case router.StreamDone:
		t.outOn(e.Request, e.Account, e.At)
		t.told(e, func(c *call) { c.end(e) })
	case router.StreamLimited, router.StreamRefused:
		t.told(e, func(c *call) { c.doing, c.at, c.limited = dashboard.Refused, e.At, e.Kind == router.StreamLimited })
	case router.StreamThrottled:
		t.told(e, func(c *call) { c.doing, c.at = dashboard.Throttled, e.At })
	case router.StreamMoved:
		t.moved(e, held)
	}
}

// tell takes in what the event e tells of its request, as the views read
// it, whatever the request, Claude Code's quota check and a count of tokens
// among them, which the views keep apart: each event names the request
// afresh, and is when the stream last told of it, but a move, which tells
// nothing of the request itself. Another request than the one held under
// its id, as another says, is told of from nothing. In flight as the stream
// opens, the request is as e has it, but for when it was first sent, where
// that was told before; gone out again, it waits on another answer, the
// last one's verdict, first byte and characters gone; and the rest as each
// kind says.
func (t *traffic) tell(e router.StreamEvent) {
	if e.Kind == router.StreamMoved {
		return
	}
	r := t.requests[e.Request]
	if another(r, e) {
		r = views.Told{}
	}
	r.Request, r.Heard, r.Account = e.Request, e.At, cmp.Or(e.Account, r.Account)
	r.Session, r.Dir, r.Model = cmp.Or(e.Session, r.Session), cmp.Or(e.Dir, r.Dir), cmp.Or(e.Model, r.Model)
	r.Prompt, r.Class, r.AgentID = cmp.Or(e.Prompt, r.Prompt), cmp.Or(e.Class, r.Class), cmp.Or(e.AgentID, r.AgentID)
	r.Check, r.Count = r.Check || e.Check, r.Count || e.Count
	switch e.Kind {
	case router.StreamInFlight:
		r.First, r.Chars, r.Verdict, r.Status, r.Ended, r.Lost = e.FirstAt, e.Chars, verdictOf(e.Verdict), e.Status, time.Time{}, false
		if r.Sent.IsZero() || e.SentAt.Before(r.Sent) {
			r.Sent = e.SentAt
		}
	case router.StreamSent:
		r.First, r.Chars, r.Verdict, r.Status, r.Lost = time.Time{}, 0, 0, 0, false
		if r.Sent.IsZero() {
			r.Sent = e.At
		}
	case router.StreamFirst:
		r.First = e.At
	case router.StreamProgress:
		r.Chars = max(r.Chars, e.Chars)
	case router.StreamDone:
		r.Ended, r.Status, r.Chars = e.At, e.Status, max(r.Chars, e.Chars)
	case router.StreamLimited, router.StreamThrottled, router.StreamRefused:
		r.Verdict, r.Status = verdictOf(e.Kind), e.Status
	}
	t.requests[e.Request] = r
}

// another reports whether e tells of another request than r, the one held
// under its id, as a request is its id and when: a router gives each request
// an id of its own while it runs, and tells nothing of one once it has
// ended; of one lost, only that it's in flight still, as the stream opens
// again, as one lost while the router ran on is told of first. Anything else
// under its id is another run's request.
func another(r views.Told, e router.StreamEvent) bool {
	return !r.Ended.IsZero() || r.Lost && e.Kind != router.StreamInFlight
}

// verdictOf is the verdict an event of the stream's kind given tells of a
// request's answer, as the views read it: none for any other kind.
func verdictOf(kind string) views.Verdict {
	switch kind {
	case router.StreamLimited:
		return views.VerdictLimited
	case router.StreamThrottled:
		return views.VerdictThrottled
	case router.StreamRefused:
		return views.VerdictRefused
	}
	return 0
}

// key names the request e tells of on the account it goes out on.
func key(e router.StreamEvent) callKey {
	return callKey{request: e.Request, account: e.Account}
}

// seatTold is the seat of the request e tells of: its session's model.
func seatTold(e router.StreamEvent) seat {
	return seat{session: e.Session, model: e.Model}
}

// inFlight takes in a request in flight as the stream opens: its answer
// streaming, where its first byte has come, else asking.
func (t *traffic) inFlight(e router.StreamEvent) {
	c := call{seat: seatTold(e), doing: dashboard.Asking, at: e.SentAt}
	if !e.FirstAt.IsZero() {
		c.doing, c.at = dashboard.Streaming, e.FirstAt
	}
	t.calls[key(e)] = c
}

// sent takes in a request gone out on an account: asking, from when it
// went, or where a move brought it there, from when that shows.
func (t *traffic) sent(e router.StreamEvent) {
	k := key(e)
	before := t.calls[k]
	c := call{seat: seatTold(e), doing: dashboard.Asking, at: e.At, brought: before.brought, shows: before.shows}
	if c.brought {
		c.at = score.Later(e.At, c.shows)
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

// first takes in the first byte of the call's answer, as e tells: its answer
// streaming, but where the upstream refused it, as the router taps the
// answer it passes on, which keeps it refused.
func (c *call) first(e router.StreamEvent) {
	if c.doing != dashboard.Refused {
		c.doing, c.at, c.brought = dashboard.Streaming, e.At, false
	}
}

// done is when the call is done with, its request gone on from it at at:
// then, or where it was refused, once its refusal has bounced back along its
// cord.
func (c call) done(at time.Time) time.Time {
	if c.doing == dashboard.Refused {
		return score.Later(at, c.at.Add(pulseFor))
	}
	return at
}

// end takes in the end of the call's request, as e tells, done with as done
// says: refused, it stays so; answered with success, its answer has just
// ended, its pulse running home; otherwise there's nothing to show of it.
func (c *call) end(e router.StreamEvent) {
	c.ended = c.done(e.At)
	switch {
	case c.doing == dashboard.Refused:
	case e.Status >= 200 && e.Status < 300:
		c.doing, c.at = dashboard.Answered, e.At
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
	t.moves = append(t.moves, move{seat: seatTold(e), from: e.From, to: e.To, at: e.At, reason: e.Reason, held: left.limited || held(e.From), request: e.Request, shows: shows})
	t.outOn(e.Request, e.To, e.At)
	t.calls[callKey{request: e.Request, account: e.To}] = call{seat: seatTold(e), brought: true, shows: shows, ended: shows}
}

// showing is when the first of the moves yet to show at now shows, reporting
// false where none is yet to.
func (t traffic) showing(now time.Time) (time.Time, bool) {
	var next time.Time
	for _, m := range t.moves {
		if m.shows.After(now) && (next.IsZero() || m.shows.Before(next)) {
			next = m.shows
		}
	}
	return next, !next.IsZero()
}

// tidied is the traffic without what has ended by now, as tidy has it, its
// maps and moves copied first.
func (t traffic) tidied(now time.Time) traffic {
	t.requests, t.calls, t.moves = maps.Clone(t.requests), maps.Clone(t.calls), slices.Clone(t.moves)
	t.tidy(now)
	return t
}

// tidy lets go of what has ended by now: the requests no longer in flight
// that the stream told of last seenFor ago, the calls with nothing more to
// show, as over has it, and the moves told of seenFor ago. It changes t's
// maps and moves, which must be its own copies.
func (t *traffic) tidy(now time.Time) {
	maps.DeleteFunc(t.requests, func(_ string, r views.Told) bool { return !r.InFlight() && now.Sub(r.Heard) >= seenFor })
	maps.DeleteFunc(t.calls, func(_ callKey, c call) bool { return c.over(now) })
	t.moves = slices.DeleteFunc(t.moves, func(m move) bool { return m.over(now) })
}

// over reports whether the call has nothing more to show at now: done with
// there, and, answered, its pulse home since.
func (c call) over(now time.Time) bool {
	if c.ended.IsZero() {
		return false
	}
	home := c.ended
	if c.doing == dashboard.Answered {
		home = home.Add(pulseFor)
	}
	return !now.Before(home)
}

// shown reports whether the call shows at now: with more to show, and,
// brought by a move, once the move shows.
func (c call) shown(now time.Time) bool {
	return !c.over(now) && (!c.brought || !now.Before(c.shows))
}

// over reports whether the move is let go at now, seenFor since it: the
// views read it until then, what brought its model where it is, though a
// listing of the router's sessions has shown it done and its cord, held or
// faded, no longer hangs, until a line of the model's own takes over.
func (m move) over(now time.Time) bool {
	return now.Sub(m.at) >= seenFor
}

// showsAt reports whether the move shows at now: from when it shows, until
// it's over.
func (m move) showsAt(now time.Time) bool {
	return !now.Before(m.shows) && !m.over(now)
}

// at is the traffic as the frame draws it at now: what travels the cord of
// each session's call on each account, as onRow folds the calls shown
// there, each of whichever of its models; and the cords moves no limit
// forced let go, while they hang loose, as loose has them.
func (t traffic) at(now time.Time) dashboard.Traffic {
	rows := make(map[dashboard.Plug][]keyedCall)
	for k, c := range t.calls {
		if c.shown(now) {
			p := dashboard.Plug{Session: c.seat.session, Account: k.account}
			rows[p] = append(rows[p], keyedCall{request: k.request, call: c})
		}
	}
	var drawn dashboard.Traffic
	for p, calls := range rows {
		if drawn.Calls == nil {
			drawn.Calls = make(map[dashboard.Plug]dashboard.Call, len(rows))
		}
		drawn.Calls[p] = onRow(calls, now)
	}
	for _, m := range t.moves {
		if l, ok := m.loose(now); ok {
			drawn.Loose = append(drawn.Loose, l)
		}
	}
	return drawn
}

// keyedCall is a call with its request's id.
type keyedCall struct {
	request string
	call    call
}

// onRow is what travels a row's cord at now, its calls being those given,
// each of a request of the session's on the account, of whichever model, as
// its one cord draws them: what the busiest of them is doing, as Busier has
// it, and its shimmer, the one that began doing it last where they tie; and
// the pulse of the one that set its pulse running last. Where they tie, the
// request whose id sorts last.
func onRow(calls []keyedCall, now time.Time) dashboard.Call {
	slices.SortFunc(calls, func(a, b keyedCall) int {
		return cmp.Or(a.call.at.Compare(b.call.at), cmp.Compare(a.request, b.request))
	})
	var row dashboard.Call
	for _, k := range calls {
		d := k.call.drawn(now)
		if !row.Doing.Busier(d.Doing) {
			row.Doing, row.Shimmer = d.Doing, d.Shimmer
		}
		if d.Pulsing {
			row.Pulse, row.Pulsing = d.Pulse, true
		}
	}
	return row
}

// drawn is the call as the frame draws it at now: what it's doing, and what
// travels its cord. A pulse runs along it for pulseFor: out to the jack as
// its request goes; back to the call as its answer ends; and back, red, as
// its request is refused. While its answer streams, its shimmer moves on a
// step as the clock passes each whole shimmerStep.
func (c call) drawn(now time.Time) dashboard.Call {
	d := dashboard.Call{Doing: c.doing}
	switch elapsed := now.Sub(c.at); c.doing {
	case dashboard.Streaming:
		d.Shimmer = steps(c.at, now, shimmerStep)
	case dashboard.Asking, dashboard.Answered, dashboard.Refused:
		d.Pulsing = elapsed < pulseFor
		d.Pulse = dashboard.Pulse{Along: progress(elapsed, pulseFor), Back: c.doing != dashboard.Asking, Red: c.doing == dashboard.Refused}
	}
	return d
}

// steps counts the times the clock passes a whole step of step from from to
// to: none where to is before from.
func steps(from, to time.Time, step time.Duration) int {
	return max(int(to.UnixNano()/int64(step)-from.UnixNano()/int64(step)), 0)
}

// loose is the cord the move let go as the frame draws it at now, hanging
// loose from the jack its session left, faded as far as it has since the
// move showed, reporting false where it doesn't hang: a limit moved it, its
// cord hanging as the limit's stub instead; it's yet to show; or its cord
// has faded.
func (m move) loose(now time.Time) (dashboard.Loose, bool) {
	fade := progress(now.Sub(m.shows), looseFor)
	if m.held || now.Before(m.shows) || fade >= 1 {
		return dashboard.Loose{}, false
	}
	return dashboard.Loose{Session: m.seat.session, From: m.from, Fade: fade}, true
}

// listedAs is the traffic once the router has listed its sessions as given,
// putting each seat on an account, which shows each move that put it there
// done.
func (t traffic) listedAs(sessions []status.Session) traffic {
	t.placed = make(map[seat]string)
	for _, s := range sessions {
		for _, a := range s.Assignments {
			t.placed[seat{session: s.ID, model: a.Model}] = a.Account
		}
	}
	t.moves = shownDone(slices.Clone(t.moves), t.placed)
	return t
}

// shownDone are the moves, each listed that the listing putting each seat
// on the account placed says shows done: one that put its seat there, or
// one before a move of its seat that did, as a listing shows a seat only
// where it went last.
func shownDone(moves []move, placed map[seat]string) []move {
	done := make(map[seat]bool)
	for i := len(moves) - 1; i >= 0; i-- {
		if m := &moves[i]; done[m.seat] || placed[m.seat] == m.to {
			m.listed, done[m.seat] = true, true
		}
	}
	return moves
}

// afresh is the traffic as the watch forgets what the stream told of the
// requests, their router gone, as when the stream won't open, or another
// router answers: no longer live, each request in flight lost, as what the
// stream tells as it opens again tells of those still in flight afresh; but
// keeping the rest it told of, and the moves it told of, which hold all the
// same until a listing of the router's sessions shows them done, and where
// its last listing put each seat.
func (t traffic) afresh() traffic {
	requests := make(map[string]views.Told, len(t.requests))
	for id, r := range t.requests {
		r.Lost = r.Ended.IsZero()
		requests[id] = r
	}
	return traffic{requests: requests, moves: t.moves, placed: t.placed, heard: t.heard}
}

// unheard is the traffic once the watch no longer hears the stream, as it
// ends or closes: no longer live, so the views go by the router's listing,
// but keeping what it told, for its first look to settle as it opens again.
func (t traffic) unheard() traffic {
	t.live = false
	return t
}

// closed is the traffic once the watch closes the stream: unheard, and what
// each seat is doing on each account forgotten, as nothing tells of it again
// until the stream opens, telling of the requests in flight afresh.
func (t traffic) closed() traffic {
	t.calls = nil
	return t.unheard()
}

// reopened is the traffic as the stream opens again: what each seat is doing
// on each account forgotten, as what the stream tells as it opens tells of
// the requests in flight afresh; and the requests it told of in flight
// before awaiting its first look, as join has it, it's live once that has
// settled them, or at once where none was in flight.
func (t traffic) reopened() traffic {
	t.calls, t.awaiting = nil, nil
	for id, r := range t.requests {
		if !r.InFlight() {
			continue
		}
		if t.awaiting == nil {
			t.awaiting = make(map[string]bool)
		}
		t.awaiting[id] = true
	}
	t.live = t.awaiting == nil
	return t
}

// join takes in e, the stream opened again, as its first look: a request in
// flight as the watch joined it, told of again, awaits it no longer, and
// once none does, it has settled them all, as settle has it; any other event
// comes after the router has told of all those in flight as the watch
// joined, so it settles those still awaiting it. It changes t's awaiting,
// which must be its own copy.
func (t *traffic) join(e router.StreamEvent) {
	switch {
	case t.awaiting == nil:
	case e.Kind == router.StreamInFlight:
		delete(t.awaiting, e.Request)
		if len(t.awaiting) == 0 {
			t.settle()
		}
	default:
		t.settle()
	}
}

// settle ends the stream's first look, as it opened again: each request
// awaiting it, which it didn't tell of again, lost, as it ended untold, as
// one cut off as its router went away does; and the traffic live from then
// on. It changes t's requests, which must be its own copy.
func (t *traffic) settle() {
	for id := range t.awaiting {
		if r, ok := t.requests[id]; ok && r.InFlight() {
			r.Lost = true
			t.requests[id] = r
		}
	}
	t.awaiting, t.live = nil, true
}

// settled is the traffic once the stream, opened again, has told no more as
// its first look, as settle has it, where it's still awaited.
func (t traffic) settled() traffic {
	if t.awaiting == nil {
		return t
	}
	t.requests = maps.Clone(t.requests)
	t.settle()
	return t
}

// stream is the traffic as the views read it at now: live or not, the
// requests the stream told of, and its moves, each while it shows, as at
// draws it, so a frame never puts a session on an account its cords don't,
// and whether a listing of the router's sessions has shown it done; heard
// as the traffic hears it.
func (t traffic) stream(now time.Time) views.Stream {
	s := views.Stream{Live: t.live, Requests: t.requests}.HeardBy(t.heard)
	for _, m := range t.moves {
		if m.showsAt(now) {
			s.Moves = append(s.Moves, views.Moved{
				Request: m.request, Session: m.seat.session, Model: m.seat.model, From: m.from, To: m.to, Reason: m.reason, At: m.at, Listed: m.listed,
			})
		}
	}
	return s
}

// sentOf are the requests the events tell of going upstream, as the traffic
// has them, each once, in the order the events first tell of them going:
// each as it goes out, or in flight as the stream opens; never the client's
// quota check, or a count of tokens.
func (t traffic) sentOf(events []router.StreamEvent) []views.Told {
	var sent []views.Told
	told := make(map[string]bool)
	for _, e := range events {
		if _, ok := sentAt(e); ok && !told[e.Request] {
			told[e.Request] = true
			sent = append(sent, t.requests[e.Request])
		}
	}
	return sent
}

// sentAt is when the request e tells of went upstream, reporting false
// where e tells of none going, or of the client's quota check, or a count of
// tokens, which spend no quota.
func sentAt(e router.StreamEvent) (time.Time, bool) {
	switch {
	case e.Check, e.Count:
		return time.Time{}, false
	case e.Kind == router.StreamSent:
		return e.At, true
	case e.Kind == router.StreamInFlight:
		return e.SentAt, true
	}
	return time.Time{}, false
}

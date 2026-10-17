package views

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
)

// LedgerDay is the local day a view is built on, by its clock: when it
// starts, and its date, as the ledger files its lines under it and prices at
// it.
type LedgerDay struct {
	Start time.Time
	Date  string
}

// Today is the local day now falls on, in time.Local whatever now's time
// zone, as the ledger files its lines: the day every view that reads the
// ledger takes as its own.
func Today(now time.Time) LedgerDay {
	return LedgerDay{Start: dayfile.DayStart(now.Local(), 0), Date: dayfile.DateOf(now)}
}

// Holds reports whether t falls on the day or after it.
func (d LedgerDay) Holds(t time.Time) bool {
	return !t.Before(d.Start)
}

// story is what a session's requests tell, taken in in order, as inOrder
// has it, as add takes them: its first and last request; what names it; the
// directory it last named; its last request whose answer gave usage, and its
// last such turn of its own conversation; its requests on the day and their
// worth; each move a request of it made; and, of each of its threads, when
// its last request ended, and how long its writes to the cache last.
type story struct {
	prices ledger.Table
	day    LedgerDay

	// begun is set once it has taken in a request: first and last are none
	// till then.
	begun       bool
	first, last flight
	naming      naming
	dir         string
	// used and usedMain are each none, as metered says it's not, till a
	// request of each kind is taken in.
	used, usedMain heldRequest
	// classed is set once a request of it says its class, as a Claude Code
	// and a router that tell prompts apart have each say.
	classed  bool
	requests int
	worth    Worth
	moves    []move
	// ends are when each thread's last request so far ended, and lives how
	// long its writes to the cache last, as its last request that wrote to
	// the cache said, by its thread: a move of the thread's comes after them.
	ends  map[string]time.Time
	lives map[string]time.Duration
}

// newStory returns the story of a session before any of its requests is
// taken in, its requests on the day d counted, and moves priced by prices.
func newStory(prices ledger.Table, d LedgerDay) *story {
	return &story{prices: prices, day: d, ends: make(map[string]time.Time), lives: make(map[string]time.Duration)}
}

// storyOf returns the story of a session's requests, in order, as inOrder
// has it, its requests on the day d counted, and moves priced by prices.
func storyOf(requests []heldRequest, prices ledger.Table, d LedgerDay) *story {
	s := newStory(prices, d)
	for i := range requests {
		s.add(&requests[i])
	}
	return s
}

// cloned is a copy of s for what tells it on to change, sharing nothing it
// changes with s: its moves, and each thread's end and life, its own. What
// tells it on puts a move's cost and the request that rewrote it in their
// places, never changing either, so they're shared.
func (s *story) cloned() *story {
	c := *s
	c.moves = slices.Clone(s.moves)
	c.ends, c.lives = maps.Clone(s.ends), maps.Clone(s.lives)
	return &c
}

// told is s told on afresh, requests, its session's next, taken in, in
// order, as inOrder has it.
func (s *story) told(requests []heldRequest) *story {
	c := s.cloned()
	for i := range requests {
		c.add(&requests[i])
	}
	return c
}

// on is s, its requests priced, told on the day given, a later one than its
// own, none of whose requests arrived on it, afresh, as it would be told of
// the same requests priced on it: none of them is on the day, they're worth
// nothing, the requests it keeps are priced at the day's prices, and so is
// each move's cost, where its model's prices are others than they were on
// s's day, as SamePrices has it.
func (s *story) on(day LedgerDay) *story {
	c := s.cloned()
	was := c.day.Date
	c.day, c.requests, c.worth = day, 0, Worth{}
	for _, r := range []*heldRequest{&c.used, &c.usedMain} {
		if r.metered() {
			*r = r.pricedOn(c.prices, day.Date)
		}
	}
	for i := range c.moves {
		m := &c.moves[i]
		if m.rewrote == nil {
			continue
		}
		rewrote := m.rewrote.pricedOn(c.prices, day.Date)
		m.rewrote = &rewrote
		if !c.prices.SamePrices(m.model, was, day.Date) {
			m.cost = c.rewriteCost(m)
		}
	}
	return c
}

// isRequest reports whether l is a session's request: one of a session that
// ledger.Line.Counted counts, as every view counts requests, a line of no
// session being none of a session's.
func isRequest(l *ledger.Line) bool {
	return l.Counted() && l.Session != ""
}

// add takes in r, the session's next request.
func (s *story) add(r *heldRequest) {
	if !s.begun {
		s.begun, s.first = true, r.flight
	}
	s.last = r.flight
	if dir := r.dir.Value(); dir != "" {
		s.dir = dir
	}
	class, from := r.class.Value(), r.from.Value()
	s.classed = s.classed || class != ""
	main := class == ledger.ClassMain
	at := r.arrived()
	s.naming.see(r.model.Value(), r.account.Value(), class, at)
	if r.metered() {
		s.used = *r
		if main {
			s.usedMain = *r
		}
	}
	if s.day.Holds(at) {
		s.requests++
		s.worth.addSummed(r.worth, r.priced)
	}
	if from != "" {
		s.moves = append(s.moves, move{by: r.flight, tried: r.tried, at: at, model: r.model.Value(), from: from, to: r.account.Value(), reason: r.reason.Value()})
	}
	if main || class == "" && from != "" {
		s.rewrites(r)
	}
	thread := r.thread()
	s.ends[thread] = score.Later(s.ends[thread], r.ended())
	if r.life != 0 {
		s.lives[thread] = r.life
	}
}

// classCompaction is the class, as a line's hints give it, of a compaction
// of a session's conversation.
const classCompaction = "compaction"

// life returns how long the writes to the cache of the thread with the given
// name last: as its last request that wrote to the cache said, else an hour.
func (s *story) life(thread string) time.Duration {
	if life, ok := s.lives[thread]; ok {
		return life
	}
	return ledger.LongCache
}

// move is a request's move of its session from one account onto another:
// the request that made it, and the accounts it went out on that couldn't
// serve it; when it came, the model it asked for, as a session's models are
// routed apart, the account it left and the one it went to, and why, as the
// router said; and, settled once its conversation's first request after it
// came, where that went to the account it moved to, the tokens that request
// wrote to the cache, and what that cost, nil where that isn't known, or its
// cache would have run out anyway; and rewrote, that request, as it wrote
// the session's context again, nil where none has, or the cache would have
// run out anyway, for its cost to be priced again on another day.
type move struct {
	by                      flight
	tried                   []ledger.Tried
	at                      time.Time
	model, from, to, reason string
	settled                 bool
	written                 int
	cost                    *ledger.Picodollars
	rewrote                 *heldRequest
}

// rewrites settles each move of r's model that awaits it, r being a request
// of the session's own conversation, or one from before requests said their
// class that moved it: the conversation's first after the move, which wrote
// its context again on the account it went to, where it went to the account
// the move did. The move's written tokens are then its writes to the cache;
// r the request that wrote the context again, and the move's cost the
// writes' price, at the day's prices, left out where the table can't price
// all of it. Both are left out where the last request of its thread ended
// longer before r than the thread's writes to the cache last, as the cache
// would have run out by then anyway.
func (s *story) rewrites(r *heldRequest) {
	thread := r.thread()
	end, ok := s.ends[thread]
	warm := !ok || !r.arrived().After(end.Add(s.life(thread)))
	tokens, _ := r.spend.Tokens()
	model, account := r.model.Value(), r.account.Value()
	for i := range s.moves {
		m := &s.moves[i]
		if m.settled || m.model != model {
			continue
		}
		m.settled = true
		if m.to != account {
			continue
		}
		m.written = tokens.CacheWrite
		if !warm {
			continue
		}
		rewrote := *r
		m.rewrote = &rewrote
		m.cost = s.rewriteCost(m)
	}
}

// rewriteCost is what the request that rewrote the session's context, as the
// move m settled, cost to write to the cache, at the day's prices: nil where
// the table can't price all of it.
func (s *story) rewriteCost(m *move) *ledger.Picodollars {
	r := m.rewrote
	if w, priced := s.prices.CacheWriteSpend(m.model, r.geo.Value(), r.spend, s.day.Date); priced && len(w.Unpriced) == 0 {
		return &w.Cost
	}
	return nil
}

// toldOf is what the story tells of the request with the given id that
// arrived at at, of its session's: whether it's the session's first
// request; the move it made, nil where it made none; and whether it wrote
// the session's context again on the account a move took it to, as
// rewrites settles it. A request is one the story holds where it's the
// same request, as flight's is has it, so a line read apart from the
// story's own tells alike.
func (s *story) toldOf(id string, at time.Time) (first bool, made *move, rewrote bool) {
	first = s.begun && s.first.is(id, at)
	for i := range s.moves {
		m := &s.moves[i]
		if m.by.is(id, at) {
			made = m
		}
		rewrote = rewrote || m.rewrote != nil && m.rewrote.is(id, at)
	}
	return first, made, rewrote
}

// sameRequest reports whether the line l is of the request with the given
// id that arrived, or went upstream, at at, as its line or the request
// stream tells it: the same id, and at while l was in flight, as inFlightAt
// has it, as an id is unique only while the router that gave it runs.
func sameRequest(l *ledger.Line, id string, at time.Time) bool {
	return l.Request == id && inFlightAt(l, at)
}

// inFlightAt reports whether the request of the line l was in flight at at,
// as inFlight has it.
func inFlightAt(l *ledger.Line, at time.Time) bool {
	return inFlight(at, l.At, ended(l))
}

// inFlight reports whether a request that arrived and ended when given was
// in flight at at: from its arrival to its end. A line's TotalMS is cut to
// the millisecond, so it may have ended up to a millisecond after it says.
func inFlight(at, arrived, ended time.Time) bool {
	return !at.Before(arrived) && at.Before(ended.Add(time.Millisecond))
}

// moveOnto returns the move of the model with the given id that brought
// the session onto the account with the given id, where the model's last
// move came on the day, and was onto it: nil where it wasn't. A session's
// models are routed apart, so another's move tells nothing of how this one
// came there.
func (s *story) moveOnto(model, account string) *move {
	m := s.latestMove(model)
	if m == nil || !s.day.Holds(m.at) || m.to != account {
		return nil
	}
	return m
}

// latestMove returns the latest move of the model with the given id, of
// whatever day: nil where it never moved.
func (s *story) latestMove(model string) *move {
	for i, m := range slices.Backward(s.moves) {
		if m.model == model {
			return &s.moves[i]
		}
	}
	return nil
}

// moveCost returns what moving the session at now would cost, as Routing by
// hand says: the whole prompt of the last turn of its own conversation whose
// answer gave usage written again, as ledger.Table.Rewrite prices it for as
// long as its thread's writes to the cache last, so a side request, as a
// title, never stands in for its conversation. Of a session none of whose
// requests says its class, as from a Claude Code or a router from before
// they did, it's its last request whose answer gave usage. It reports false
// where that isn't known, and where the cache has run out since its thread's
// last request ended, as rewrites judges a move, which the session would
// write again on its next request anyway: unless flying says a request of
// its model is in flight, keeping its cache warm.
func (s *story) moveCost(now time.Time, flying func(model string) bool) (ledger.Picodollars, bool) {
	prompt, life, cold := s.moving()
	if prompt == nil {
		return 0, false
	}
	model := prompt.model.Value()
	if !flying(model) && !now.Before(cold) {
		return 0, false
	}
	return s.prices.RewriteSpend(model, prompt.geo.Value(), prompt.spend, life, s.day.Date)
}

// moving is what moveCost prices a move by: the prompt written again, nil
// where none is known; how long its thread's writes to the cache last; and
// the first moment its cache has run out since its thread's last request
// ended.
func (s *story) moving() (prompt *heldRequest, life time.Duration, cold time.Time) {
	prompt = &s.used
	if s.classed {
		prompt = &s.usedMain
	}
	if !prompt.metered() {
		return nil, 0, time.Time{}
	}
	thread := prompt.thread()
	life = s.life(thread)
	return prompt, life, s.ends[thread].Add(life + time.Nanosecond)
}

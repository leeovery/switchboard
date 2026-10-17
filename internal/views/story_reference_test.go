package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
)

// The story's fold over lines, as it stood before the story folded held
// requests, word for word but for its names: the reference the fold over
// held requests is held to, request by request, in the same order.

// lineStory is what a session's requests tell, taken in oldest first, as add
// takes them: its first and last request; what names it; the directory it
// last named; its last request whose answer gave usage, and its last such
// turn of its own conversation; its requests on the day and their worth;
// each move a request of it made; and, of each of its threads, when its last
// request ended, and how long its writes to the cache last.
type lineStory struct {
	prices ledger.Table
	day    LedgerDay

	first, last    *ledger.Line
	naming         naming
	dir            string
	used, usedMain *ledger.Line
	// classed is set once a request of it says its class, as a Claude Code
	// and a router that tell prompts apart have each say.
	classed  bool
	requests int
	worth    Worth
	moves    []lineMove
	// ends are when each thread's last request so far ended, and lives how
	// long its writes to the cache last, as its last request that wrote to
	// the cache said, by lineThreadOf: a move of the thread's comes after them.
	ends  map[string]time.Time
	lives map[string]time.Duration
}

// request is a session's request as the fold over lines took it in: its
// line, what it would have cost through the API, as ledger.Table.Summed
// gives it, and whether its answer gave usage.
type request struct {
	*ledger.Line
	worth   Worth
	metered bool
}

// requestsOf returns the requests among lines, a session's, oldest first, as
// isRequest says, each priced by prices at the local day with the given
// date.
func requestsOf(lines []ledger.Line, prices ledger.Table, date string) []request {
	var requests []request
	for i := range lines {
		if l := &lines[i]; isRequest(l) {
			_, metered := l.Tokens()
			requests = append(requests, request{Line: l, worth: summed(prices, l, date), metered: metered})
		}
	}
	return requests
}

// newLineStory returns the story of a session before any of its requests is
// taken in, its requests on the day d counted, and moves priced by prices.
func newLineStory(prices ledger.Table, d LedgerDay) *lineStory {
	return &lineStory{prices: prices, day: d, ends: make(map[string]time.Time), lives: make(map[string]time.Duration)}
}

func lineStoryOf(requests []request, prices ledger.Table, d LedgerDay) *lineStory {
	s := newLineStory(prices, d)
	for _, r := range requests {
		s.add(r)
	}
	return s
}

// add takes in r, the session's next request.
func (s *lineStory) add(r request) {
	l := r.Line
	if s.first == nil {
		s.first = l
	}
	s.last = l
	if l.Dir != "" {
		s.dir = l.Dir
	}
	s.classed = s.classed || l.Class != ""
	main := l.Class == ledger.ClassMain
	s.naming.see(l.Model, l.Account, l.Class, l.At)
	if r.metered {
		s.used = l
		if main {
			s.usedMain = l
		}
	}
	if s.day.Holds(l.At) {
		s.requests++
		s.worth.add(r.worth)
	}
	if l.From != "" {
		s.moves = append(s.moves, lineMove{by: l, at: l.At, model: l.Model, from: l.From, to: l.Account, reason: l.Reason})
	}
	if main || l.Class == "" && l.From != "" {
		s.rewrites(l)
	}
	thread := lineThreadOf(l)
	s.ends[thread] = score.Later(s.ends[thread], ended(l))
	if life, ok := l.CacheLife(); ok {
		s.lives[thread] = life
	}
}

// lineThreadOf returns the thread of a session's requests the request l is
// of, as its cache follows them: a subagent's, by its id; the session's own
// conversation, its compactions, which read it, among them; a side
// request's, as a title, by its class and model; and, of a line from before
// requests said their class, which tells no thread from another, its
// model's.
func lineThreadOf(l *ledger.Line) string {
	switch {
	case l.AgentID != "":
		return "agent " + l.AgentID
	case l.Class == ledger.ClassMain || l.Class == classCompaction:
		return "conversation"
	case l.Class == "":
		return "model " + l.Model
	}
	return l.Class + " " + l.Model
}

// life returns how long the writes to the cache of the thread with the given
// name last: as its last request that wrote to the cache said, else an hour.
func (s *lineStory) life(thread string) time.Duration {
	if life, ok := s.lives[thread]; ok {
		return life
	}
	return ledger.LongCache
}

// lineMove is a request's move of its session from one account onto
// another: the request that made it; when it came, the model it asked for,
// as a session's models are routed apart, the account it left and the one it
// went to, and why, as the router said; and, settled once its conversation's
// first request after it came, where that went to the account it moved to,
// the tokens that request wrote to the cache, and what that cost, nil where
// that isn't known, or its cache would have run out anyway; and rewrote,
// that request, as it wrote the session's context again, nil where none has,
// or the cache would have run out anyway.
type lineMove struct {
	by                      *ledger.Line
	at                      time.Time
	model, from, to, reason string
	settled                 bool
	written                 int
	cost                    *ledger.Picodollars
	rewrote                 *ledger.Line
}

// rewrites settles each move of l's model that awaits it, l being a request
// of the session's own conversation, or one from before requests said their
// class that moved it: the conversation's first after the move, which wrote
// its context again on the account it went to, where it went to the account
// the move did. The move's written tokens are then its writes to the cache;
// l the request that wrote the context again, and the move's cost the
// writes' price, at the day's prices, left out where the table can't price
// all of it. Both are left out where the last request of its thread, as
// lineThreadOf tells it, ended longer before l than the thread's writes to
// the cache last, as the cache would have run out by then anyway.
func (s *lineStory) rewrites(l *ledger.Line) {
	thread := lineThreadOf(l)
	end, ok := s.ends[thread]
	warm := !ok || !l.At.After(end.Add(s.life(thread)))
	tokens, _ := l.Tokens()
	for i := range s.moves {
		m := &s.moves[i]
		if m.settled || m.model != l.Model {
			continue
		}
		m.settled = true
		if m.to != l.Account {
			continue
		}
		m.written = tokens.CacheWrite
		if !warm {
			continue
		}
		m.rewrote = l
		if w, priced := s.prices.CacheWrite(l, s.day.Date); priced && len(w.Unpriced) == 0 {
			m.cost = &w.Cost
		}
	}
}

// toldOf is what the story tells of l, a line of its session's: whether
// it's the session's first request; the move it made, nil where it made
// none; and whether it wrote the session's context again on the account a
// move took it to, as rewrites settles it. A line is the request a story
// holds where it's the same request, as sameRequest has it, so a line read
// apart from the story's own tells alike.
func (s *lineStory) toldOf(l *ledger.Line) (first bool, made *lineMove, rewrote bool) {
	first = s.first != nil && sameRequest(s.first, l.Request, l.At)
	for i := range s.moves {
		m := &s.moves[i]
		if sameRequest(m.by, l.Request, l.At) {
			made = m
		}
		rewrote = rewrote || m.rewrote != nil && sameRequest(m.rewrote, l.Request, l.At)
	}
	return first, made, rewrote
}

// moveOnto returns the move of the model with the given id that brought
// the session onto the account with the given id, where the model's last
// move came on the day, and was onto it: nil where it wasn't. A session's
// models are routed apart, so another's move tells nothing of how this one
// came there.
func (s *lineStory) moveOnto(model, account string) *lineMove {
	m := s.latestMove(model)
	if m == nil || !s.day.Holds(m.at) || m.to != account {
		return nil
	}
	return m
}

// latestMove returns the latest move of the model with the given id, of
// whatever day: nil where it never moved.
func (s *lineStory) latestMove(model string) *lineMove {
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
func (s *lineStory) moveCost(now time.Time, flying func(model string) bool) (ledger.Picodollars, bool) {
	prompt, life, cold := s.moving()
	if prompt == nil {
		return 0, false
	}
	if !flying(prompt.Model) && !now.Before(cold) {
		return 0, false
	}
	return s.prices.Rewrite(prompt, life, s.day.Date)
}

// moving is what moveCost prices a move by: the prompt written again, nil
// where none is known; how long its thread's writes to the cache last; and
// the first moment its cache has run out since its thread's last request
// ended.
func (s *lineStory) moving() (prompt *ledger.Line, life time.Duration, cold time.Time) {
	prompt = s.used
	if s.classed {
		prompt = s.usedMain
	}
	if prompt == nil {
		return nil, 0, time.Time{}
	}
	thread := lineThreadOf(prompt)
	life = s.life(thread)
	return prompt, life, s.ends[thread].Add(life + time.Nanosecond)
}

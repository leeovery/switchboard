package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// day is the local day a view is built on, by its clock: when it starts, and
// its date, as the ledger files its lines under it and prices at it.
type day struct {
	start time.Time
	date  string
}

// dayOf returns the local day now falls on.
func dayOf(now time.Time) day {
	return day{start: dayfile.DayStart(now.Local(), 0), date: now.Local().Format(time.DateOnly)}
}

// holds reports whether t falls on the day or after it.
func (d day) holds(t time.Time) bool {
	return !t.Before(d.start)
}

// story is what a session's requests tell, taken in oldest first, as add
// takes them: its first and last request, its last that went to an account,
// and its last of its own conversation that did; the directory it last
// named; its last request whose answer gave usage, and its last such turn of
// its own conversation; its requests on the day and their worth; each move a
// request of it made; and, of each of its threads, when its last request
// ended, and how long its writes to the cache last.
type story struct {
	prices ledger.Table
	day    day

	first, last, lastRouted, lastMain *ledger.Line
	dir                               string
	used, usedMain                    *ledger.Line
	// classed is set once a request of it says its class, as a Claude Code
	// and a router that tell prompts apart have each say.
	classed  bool
	requests int
	worth    worthSum
	moves    []move
	// ends are when each thread's last request so far ended, and lives how
	// long its writes to the cache last, as its last request that wrote to
	// the cache said, by threadOf: a move of the thread's comes after them.
	ends  map[string]time.Time
	lives map[string]time.Duration
}

// newStory returns the story of a session before any of its requests is
// taken in, its requests on the day d counted, and moves priced by prices.
func newStory(prices ledger.Table, d day) *story {
	return &story{prices: prices, day: d, ends: make(map[string]time.Time), lives: make(map[string]time.Duration)}
}

// storyOf returns the story requests, a session's, oldest first, tell, on the
// day d, moves priced by prices.
func storyOf(requests []request, prices ledger.Table, d day) *story {
	s := newStory(prices, d)
	for _, r := range requests {
		s.add(r)
	}
	return s
}

// isRequest reports whether l is a session's request: a message of a session,
// one that spends quota, whether it went upstream or the router answered it
// itself, never a quota check or a count of tokens, which spend nothing.
func isRequest(l *ledger.Line) bool {
	return l.Kind == ledger.KindMessage && l.Session != ""
}

// add takes in r, the session's next request.
func (s *story) add(r request) {
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
	if l.Account != "" {
		s.lastRouted = l
		if main {
			s.lastMain = l
		}
	}
	if r.metered {
		s.used = l
		if main {
			s.usedMain = l
		}
	}
	if s.day.holds(l.At) {
		s.requests++
		s.worth.add(r.worth)
	}
	if l.From != "" {
		s.moves = append(s.moves, move{at: l.At, model: l.Model, from: l.From, to: l.Account, reason: l.Reason})
	}
	if main || l.Class == "" && l.From != "" {
		s.rewrites(l)
	}
	thread := threadOf(l)
	s.ends[thread] = latest(s.ends[thread], ended(l))
	if life, ok := l.CacheLife(); ok {
		s.lives[thread] = life
	}
}

// classCompaction is the class, as a line's hints give it, of a compaction
// of a session's conversation.
const classCompaction = "compaction"

// threadOf returns the thread of a session's requests the request l is of,
// as its cache follows them: a subagent's, by its id; the session's own
// conversation, its compactions, which read it, among them; a side
// request's, as a title, by its class and model; and, of a line from before
// requests said their class, which tells no thread from another, its
// model's.
func threadOf(l *ledger.Line) string {
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
func (s *story) life(thread string) time.Duration {
	if life, ok := s.lives[thread]; ok {
		return life
	}
	return ledger.LongCache
}

// conversation returns the session's last request of its own conversation
// that went to an account, where its requests say their class and one did,
// else its last request that went to one: the one whose account and model
// the List names, as a request the router answered itself went to none.
func (s *story) conversation() *ledger.Line {
	if s.classed && s.lastMain != nil {
		return s.lastMain
	}
	return s.lastRouted
}

// conversationModel returns the model of the session's last request of its
// own conversation that went to an account: "" where its requests don't say
// their class, or none did.
func (s *story) conversationModel() string {
	if !s.classed || s.lastMain == nil {
		return ""
	}
	return s.lastMain.Model
}

// move is a request's move of its session from one account onto another:
// when it came, the model it asked for, as a session's models are routed
// apart, the account it left and the one it went to, and why, as the router
// said; and, settled once its conversation's first request after it came,
// where that went to the account it moved to, the tokens that request wrote
// to the cache, and what that cost, nil where that isn't known, or its cache
// would have run out anyway.
type move struct {
	at                      time.Time
	model, from, to, reason string
	settled                 bool
	written                 int
	cost                    *ledger.Picodollars
}

// rewrites settles each move of l's model that awaits it, l being a request
// of the session's own conversation, or one from before requests said their
// class that moved it: the conversation's first after the move, which wrote
// its context again on the account it went to, where it went to the account
// the move did. The move's written tokens are then its writes to the cache,
// and its cost their price, at the day's prices, left out where the table
// can't price all of it, and where the last request of its thread, as
// threadOf tells it, ended longer before it than the thread's writes to the
// cache last, as the cache would have run out by then anyway.
func (s *story) rewrites(l *ledger.Line) {
	thread := threadOf(l)
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
		if w, priced := s.prices.CacheWrite(l, s.day.date); warm && priced && len(w.Unpriced) == 0 {
			m.cost = &w.Cost
		}
	}
}

// movedOnto returns the move of the model with the given id that brought
// the session onto the account with the given id, where the model's last
// move came on the day, and was onto it: nil where it wasn't. A session's
// models are routed apart, so another's move tells nothing of how this one
// came there.
func (s *story) movedOnto(model, account string) *Move {
	for _, m := range slices.Backward(s.moves) {
		if m.model != model {
			continue
		}
		if !s.day.holds(m.at) || m.to != account {
			return nil
		}
		return &Move{At: m.at.UTC(), From: m.from, Reason: m.reason, Cost: m.cost}
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
// where that isn't known, and where the cache has run out since that request
// ended, which the session would write again on its next request anyway:
// unless flying says a request of its model is in flight, keeping its cache
// warm.
func (s *story) moveCost(now time.Time, flying func(model string) bool) (ledger.Picodollars, bool) {
	prompt := s.used
	if s.classed {
		prompt = s.usedMain
	}
	if prompt == nil {
		return 0, false
	}
	life := s.life(threadOf(prompt))
	if !flying(prompt.Model) && now.Sub(ended(prompt)) > life {
		return 0, false
	}
	return s.prices.Rewrite(prompt, life, s.day.date)
}

// worthSum sums requests' worth exactly, and names, once each, the counts
// any of them leaves unpriced.
type worthSum struct {
	cost     ledger.Picodollars
	unpriced []string
}

// add adds w to the sum.
func (s *worthSum) add(w ledger.Worth) {
	s.cost += w.Cost
	for _, path := range w.Unpriced {
		if !slices.Contains(s.unpriced, path) {
			s.unpriced = append(s.unpriced, path)
		}
	}
	slices.Sort(s.unpriced)
}

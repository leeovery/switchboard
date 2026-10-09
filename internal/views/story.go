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

// story is what a session's lines tell, taken in oldest first, as add takes
// them: its first and last request, and its last of its own conversation;
// the directory it last named; its last request whose answer gave usage, and
// its last such turn of its own conversation; how long its writes to the
// cache last, as its last request that wrote to it says; its requests on the
// day and their worth; each move a request of it made; and when the last
// request of each of its threads ended.
type story struct {
	prices ledger.Table
	day    day

	first, last, lastMain *ledger.Line
	dir                   string
	used, usedMain        *ledger.Line
	// classed is set once a request of it says its class, as a Claude Code
	// and a router that tell prompts apart have each say.
	classed bool
	// life is how long its writes to the cache last: an hour until a request
	// of it wrote to the cache, as one that only read from it says nothing.
	life     time.Duration
	requests int
	worth    worthSum
	moves    []move
	// ends are when each thread's last request so far ended, by threadOf: a
	// move of the thread's comes after it.
	ends map[string]time.Time
}

// newStory returns the story of a session before any of its lines is taken
// in, its requests on the day d counted and priced by prices.
func newStory(prices ledger.Table, d day) *story {
	return &story{prices: prices, day: d, life: ledger.LongCache, ends: make(map[string]time.Time)}
}

// isRequest reports whether l is a session's request: a message of a session,
// one that spends quota, whether it went upstream or the router answered it
// itself, never a quota check or a count of tokens, which spend nothing.
func isRequest(l *ledger.Line) bool {
	return l.Kind == ledger.KindMessage && l.Session != ""
}

// add takes in l, the session's next line, passing over one that isn't a
// request, as isRequest says.
func (s *story) add(l *ledger.Line) {
	if !isRequest(l) {
		return
	}
	if s.first == nil {
		s.first = l
	}
	s.last = l
	if l.Dir != "" {
		s.dir = l.Dir
	}
	s.classed = s.classed || l.Class != ""
	main := l.Class == ledger.ClassMain
	if main {
		s.lastMain = l
	}
	if _, ok := l.Tokens(); ok {
		s.used = l
		if main {
			s.usedMain = l
		}
	}
	if s.day.holds(l.At) {
		s.requests++
		s.worth.add(s.prices.Summed(l, s.day.date))
	}
	if l.From != "" {
		s.moves = append(s.moves, s.moveOf(l))
	}
	thread := threadOf(l)
	s.ends[thread] = latest(s.ends[thread], ended(l))
	if life, ok := l.CacheLife(); ok {
		s.life = life
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

// conversation returns the session's last request of its own conversation,
// where its requests say their class and one is, else its last request: the
// one whose account and model the List names of one ended.
func (s *story) conversation() *ledger.Line {
	if s.classed && s.lastMain != nil {
		return s.lastMain
	}
	return s.last
}

// move is a request's move of its session from one account onto another:
// when it came, the model it asked for, as a session's models are routed
// apart, the account it left and the one it went to, why, as the router
// said, the tokens its request wrote to the cache, and what writing its
// context again on the account it went to cost, nil where that isn't known,
// or its cache would have run out anyway.
type move struct {
	at                      time.Time
	model, from, to, reason string
	written                 int
	cost                    *ledger.Picodollars
}

// moveOf returns the move l made: its cost its cache write, priced at the
// day's prices, left out where the table can't price all of it, and where
// the last request of its thread, as threadOf tells it, ended longer before
// it than the session's writes to the cache last, as the cache would have run
// out by then anyway.
func (s *story) moveOf(l *ledger.Line) move {
	tokens, _ := l.Tokens()
	m := move{at: l.At, model: l.Model, from: l.From, to: l.Account, reason: l.Reason, written: tokens.CacheWrite}
	if end, ok := s.ends[threadOf(l)]; ok && l.At.Sub(end) > s.life {
		return m
	}
	if w, ok := s.prices.CacheWrite(l, s.day.date); ok && len(w.Unpriced) == 0 {
		m.cost = &w.Cost
	}
	return m
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
// long as the session's writes to the cache last, so a side request, as a
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
	if prompt == nil || !flying(prompt.Model) && now.Sub(ended(prompt)) > s.life {
		return 0, false
	}
	return s.prices.Rewrite(prompt, s.life, s.day.date)
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

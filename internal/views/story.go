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
// them: its first and last request, the directory it last named, its last
// request whose answer gave usage, and its last such turn of its own
// conversation, its requests on the day and their worth, and each move a
// request of it made.
type story struct {
	prices ledger.Table
	day    day

	first, last *ledger.Line
	dir         string
	used        *ledger.Line
	usedMain    *ledger.Line
	// classed is set once a request of it says its class, as a Claude Code
	// and a router that tell prompts apart have each say.
	classed  bool
	requests int
	worth    worthSum
	moves    []move
	// lastOf is each model's last request so far, by its id: a move of the
	// model's comes after it.
	lastOf map[string]*ledger.Line
}

// newStory returns the story of a session before any of its lines is taken
// in, its requests on the day d counted and priced by prices.
func newStory(prices ledger.Table, d day) *story {
	return &story{prices: prices, day: d, lastOf: make(map[string]*ledger.Line)}
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
	if _, ok := l.Tokens(); ok {
		s.used = l
		if l.Class == ledger.ClassMain {
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
	s.lastOf[l.Model] = l
}

// move is a request's move of its session from one account onto another:
// when it came, the account it left and the one it went to, why, as the
// router said, and what writing its context again on the account it went to
// cost, nil where that isn't known, or its cache would have run out anyway.
type move struct {
	at               time.Time
	from, to, reason string
	cost             *ledger.Picodollars
}

// moveOf returns the move l made: its cost its cache write, priced at the
// day's prices, left out where the table can't price all of it, and where
// the model's last request came longer before it than its writes to the
// cache last, as the cache would have run out by then anyway.
func (s *story) moveOf(l *ledger.Line) move {
	m := move{at: l.At, from: l.From, to: l.Account, reason: l.Reason}
	if before := s.lastOf[l.Model]; before != nil && l.At.Sub(before.At) > before.CacheLife() {
		return m
	}
	if w, ok := s.prices.CacheWrite(l, s.day.date); ok && len(w.Unpriced) == 0 {
		m.cost = &w.Cost
	}
	return m
}

// movedOnto returns the move that brought the session onto the account with
// the given id, where its last move came on the day, and was onto it: nil
// where it wasn't.
func (s *story) movedOnto(account string) *Move {
	if len(s.moves) == 0 {
		return nil
	}
	m := s.moves[len(s.moves)-1]
	if !s.day.holds(m.at) || m.to != account {
		return nil
	}
	return &Move{At: m.at.UTC(), From: m.from, Reason: m.reason, Cost: m.cost}
}

// moveCost returns what moving the session now would cost, as Routing by
// hand says: the whole prompt of the last turn of its own conversation whose
// answer gave usage written again, as ledger.Table.Rewrite prices it, idle
// for as long as idle says, so a side request, as a title, never stands in for
// its conversation. Of a session none of whose requests says its class, as
// from a Claude Code or a router from before they did, it's its last request
// whose answer gave usage. It reports false where that isn't known, and where
// the session has idled longer than its cache lasts, which it would write
// again on its next request anyway.
func (s *story) moveCost(idle time.Duration) (ledger.Picodollars, bool) {
	prompt := s.used
	if s.classed {
		prompt = s.usedMain
	}
	if prompt == nil || idle > prompt.CacheLife() {
		return 0, false
	}
	return s.prices.Rewrite(prompt, s.day.date)
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

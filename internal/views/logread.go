package views

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// LogRead is a read of the ledger for the Log's Requests, grown as the
// ledger is read and never read again whole: the lines of the days the table
// spans, each priced once as it's taken in, at the prices of the day it's
// taken in on, and its row held, told again only as its session's story
// changes; and every line of their sessions, whatever day it's of, each
// session read once, which each session's story is told from. A row's story
// is never told from the table's lines alone, which would make a session
// that began the day before new today, and a move after a night's idle cost
// its context written again. What grows it makes it afresh, so a frame
// holding it never sees it change.
type LogRead struct {
	prices ledger.Table
	// price, where it's set, prices a line's request in place of worthOf's
	// summed, as a test counts the lines priced.
	price func(l *ledger.Line, date string) Worth
	// lines are the table's lines, each once, in the order taken in; held,
	// their places among lines, by their requests' ids.
	lines []*loggedLine
	held  map[string][]int
	// sessions are the sessions of the lines' requests, by their ids.
	sessions map[string]*loggedSession
	// written is the latest end of the lines.
	written time.Time
}

// loggedLine is a line of the table's, as the read holds it: the local day
// it's filed under, its arrival's; its row as its line alone tells it,
// priced; and its row, as its session's story tells the rest.
type loggedLine struct {
	line  ledger.Line
	day   LedgerDay
	facts RequestRow
	row   RequestRow
}

// loggedSession is a session of the table's requests, as the read holds it:
// its requests, oldest first, of whatever day; its story, told from them;
// and the places of its requests among the table's lines.
type loggedSession struct {
	requests []request
	story    *story
	lines    []int
}

// ReadLogHeld reads, through l, at now, the Log's Requests of held, the
// ledger's lines as Ledger's Today gives them, as LogRead.Grown takes them
// in. It reads the sessions of their requests once, every line of each,
// whatever day it's of, which says which of its requests is its first, the
// moves they made and what each cost; each line it prices by prices, at
// now's.
func ReadLogHeld(l LedgerSessions, held []ledger.Held, prices ledger.Table, now time.Time) LogRead {
	return LogRead{prices: prices}.Grown(l, held, now)
}

// Grown is r with held taken in at now: a tail of today's lines as Ledger's
// Today gives it, or every one of them again, as when it reads them afresh;
// or a day's, as Ledger's Day reads them, as the tables reach back a day,
// or a day is read again whole. Those of its lines r holds already are
// passed over, so a read costs what the ledger gained, never what r holds.
// Each line taken in is priced, at now's prices; the sessions of its
// requests r doesn't hold yet are read through l, all at once, and only
// those; and the stories of the sessions they're of are told again.
func (r LogRead) Grown(l LedgerSessions, held []ledger.Held, now time.Time) LogRead {
	fresh := r.unheld(held)
	if len(fresh) == 0 {
		return r
	}
	today := Today(now)
	grown := r.afresh()
	// touched are the sessions grown holds a copy of its own of, to change.
	touched := grown.read(l, fresh)
	for i := range fresh {
		line := &fresh[i]
		at := len(grown.lines)
		facts := lineFacts(line)
		facts.Worth = grown.worthOf(line, today.Date)
		logged := &loggedLine{line: *line, day: Today(line.At), facts: facts, row: facts}
		logged.row.toldBy(line, nil)
		grown.lines = append(grown.lines, logged)
		grown.held[line.Request] = append(slices.Clip(grown.held[line.Request]), at)
		if end := line.At.Add(time.Duration(line.TotalMS) * time.Millisecond); end.After(grown.written) {
			grown.written = end
		}
		if !isRequest(line) {
			continue
		}
		if !touched[line.Session] {
			grown.sessions[line.Session], touched[line.Session] = grown.sessions[line.Session].copied(), true
		}
		grown.sessions[line.Session].hold(line, at)
	}
	for id := range touched {
		grown.retell(id, today)
	}
	return grown
}

// Len is how many lines r holds: it changes only as r grows.
func (r LogRead) Len() int {
	return len(r.lines)
}

// WrittenTo is when the ledger last wrote of what r holds: the latest end of
// its lines, every kind's, each its arrival and its total_ms after; zero
// where it holds none. The Log not live is to it.
func (r LogRead) WrittenTo() time.Time {
	return r.written
}

// sessionStory is the story of the session with the given id, as r holds it
// told from every line of it, priced as r prices: one of no requests where r
// holds none of its lines. Every view r is read for tells a session from it,
// building none of its own.
func (r LogRead) sessionStory(id string) *story {
	if s := r.sessions[id]; s != nil && s.story != nil {
		return s.story
	}
	return newStory(r.prices, LedgerDay{})
}

// firstsOn are when each session's first line filed on the day d arrived,
// by the session's whole id, of the lines r holds: each that counts toward
// a session, as ledger.Line.CountedSession has it, so as the Log counts a
// session's lines.
func (r LogRead) firstsOn(d LedgerDay) map[string]time.Time {
	firsts := make(map[string]time.Time)
	for _, l := range r.lines {
		id := l.line.CountedSession()
		if id == "" || l.day.Date != d.Date {
			continue
		}
		if at, ok := firsts[id]; !ok || l.line.At.Before(at) {
			firsts[id] = l.line.At.UTC()
		}
	}
	return firsts
}

// worthOf is what the request l was worth at the prices of the local day
// with the given date, as summed has it.
func (r LogRead) worthOf(l *ledger.Line, date string) Worth {
	if r.price != nil {
		return r.price(l, date)
	}
	return summed(r.prices, l, date)
}

// heldLine is a line's request, as sameLine tells one from another: its id,
// and when it arrived.
type heldLine struct {
	request string
	at      int64
}

// unheld are the lines of held that r holds none of, as sameLine has it,
// each once, in the order given.
func (r LogRead) unheld(held []ledger.Held) []ledger.Line {
	var fresh []ledger.Line
	seen := make(map[heldLine]bool, len(held))
	for _, h := range held {
		line, key := h.Line, heldLine{request: h.Request, at: h.At.UnixNano()}
		if seen[key] || r.holds(&line) {
			continue
		}
		seen[key], fresh = true, append(fresh, line)
	}
	return fresh
}

// holds reports whether r holds the line l among the table's.
func (r LogRead) holds(l *ledger.Line) bool {
	return slices.ContainsFunc(r.held[l.Request], func(at int) bool { return sameLine(&r.lines[at].line, l) })
}

// sameLine reports whether a and b are the same request's line: of the same
// id, arriving at the same time, as an id is unique only while the router
// that gave it runs.
func sameLine(a, b *ledger.Line) bool {
	return a.Request == b.Request && a.At.Equal(b.At)
}

// afresh is a copy of r for what grows it to change, sharing nothing it
// changes with r: the sessions it holds are copied as they change, as copied
// copies one.
func (r LogRead) afresh() LogRead {
	r.lines = slices.Clone(r.lines)
	r.held = maps.Clone(r.held)
	r.sessions = maps.Clone(r.sessions)
	if r.held == nil {
		r.held = make(map[string][]int)
	}
	if r.sessions == nil {
		r.sessions = make(map[string]*loggedSession)
	}
	return r
}

// read reads, through l, the sessions of the requests among fresh that r
// doesn't hold, all at once, holding each the ledger gives that r doesn't,
// and returns those it holds, by their ids. It reads none where r holds
// every one.
func (r LogRead) read(l LedgerSessions, fresh []ledger.Line) map[string]bool {
	read := make(map[string]bool)
	var ids []string
	for i := range fresh {
		if id := fresh[i].Session; isRequest(&fresh[i]) && r.sessions[id] == nil && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return read
	}
	slices.Sort(ids)
	for id, lines := range l.Sessions(ids, nil) {
		if r.sessions[id] == nil {
			r.sessions[id], read[id] = &loggedSession{requests: requestsUnpriced(lines)}, true
		}
	}
	return read
}

// requestsUnpriced are the requests among lines, a session's, oldest first,
// as requestsOf has them, but unpriced: a story the Log tells its rows from
// says nothing of its requests' worth, which each row holds priced.
func requestsUnpriced(lines []ledger.Line) []request {
	var requests []request
	for i := range lines {
		l := &lines[i]
		if isRequest(l) {
			tokens, metered := l.Tokens()
			requests = append(requests, request{Line: l, tokens: tokens, metered: metered})
		}
	}
	return requests
}

// copied is a copy of s for what grows it to change, sharing nothing it
// changes with s: an empty session where s is nil.
func (s *loggedSession) copied() *loggedSession {
	if s == nil {
		return &loggedSession{}
	}
	return &loggedSession{requests: slices.Clip(s.requests), story: s.story, lines: slices.Clip(s.lines)}
}

// hold holds the request l, a line of the session's among the table's at
// the place given: among its requests, in the order they arrived, where it
// holds none of it, as sameLine has it, from the ledger's read of it.
func (s *loggedSession) hold(l *ledger.Line, at int) {
	s.lines = append(s.lines, at)
	i, _ := slices.BinarySearchFunc(s.requests, l.At, func(r request, at time.Time) int { return r.At.Compare(at) })
	for j := i; j < len(s.requests) && s.requests[j].At.Equal(l.At); j++ {
		if sameLine(s.requests[j].Line, l) {
			return
		}
	}
	for i < len(s.requests) && s.requests[i].At.Equal(l.At) {
		i++
	}
	tokens, metered := l.Tokens()
	s.requests = slices.Insert(s.requests, i, request{Line: l, tokens: tokens, metered: metered})
}

// retell tells the story of the session with the given id again, at the
// local day today's prices, and has each of its rows among the table's say
// what it tells, each afresh.
func (r LogRead) retell(id string, today LedgerDay) {
	s := r.sessions[id]
	s.story = storyOf(s.requests, r.prices, today)
	for _, at := range s.lines {
		told := *r.lines[at]
		told.row = told.facts
		told.row.toldBy(&told.line, s.story)
		r.lines[at] = &told
	}
}

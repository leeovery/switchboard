package views

import (
	"iter"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// CharsPerToken is how many of the characters the request stream counts of
// an answer make a token, as its tokens are estimated while it streams. The
// one count feeds both forms the views give it: its tokens so far, as "↓
// ~1.2k", and its characters, as "5.1k characters".
const CharsPerToken = 4

// Stream is what the router's request stream has told of the routed
// requests, as the watch heard it, for the views to read: the zero Stream is
// a watch that hears none, whose sessions are drawn as each look reads them.
// Its map and moves are its own: what changes it copies them first.
type Stream struct {
	// Live is set while what the stream tells is the watch's: from when it
	// opens until the watch forgets what it told.
	Live bool
	// Requests are the requests it told of, by their ids: each in flight;
	// and each it told of last within the hour a session runs for unseen,
	// ended or lost.
	Requests map[string]Told
	// Moves are the moves it told of within the hour a session runs for
	// unseen, the oldest first, each once it shows, a listing of the
	// router's sessions showing it done or not: what brought a model where
	// it is says so until a line of its own takes over, while only those no
	// listing has shown done put a model where it went, as unlisted has them.
	Moves []Moved
}

// unlisted are the moves s told of that no listing of the router's sessions
// has shown done, the oldest first: those that put a model where it went,
// as the router doesn't list it there yet.
func (s Stream) unlisted() []Moved {
	return slices.DeleteFunc(slices.Clone(s.Moves), func(m Moved) bool { return m.Listed })
}

// Told is a request as the request stream has told of it, by its id, each
// event filling it in: what a request's row in flight is drawn from, and
// what a session's requests are doing.
type Told struct {
	// Request is the router's id for it, unique while it runs.
	Request string
	// Session, Dir and Model are the session it belongs to, the directory
	// that session was started in, and the model it asks for: each "" where
	// it names none.
	Session, Dir, Model string
	// Account is the account it went out on last, or, once its client has an
	// answer, the one it came from.
	Account string
	// Prompt, Class and AgentID are the prompt it serves, its class, and the
	// agent that sent it, as its line will give them: each "" where it
	// carries none.
	Prompt, Class, AgentID string
	// Check is set where it's Claude Code's quota check, and Count where
	// it's a count of tokens: neither spends quota.
	Check, Count bool
	// Sent is when it first went upstream, as the stream told; and First when
	// the first byte of its answer came, since it last went: zero until it
	// has.
	Sent, First time.Time
	// Chars counts the characters of text, thinking and tools' input its
	// answer has streamed so far, or in all once it's done: zero while none
	// has, or none could be counted.
	Chars int
	// Verdict is the last the stream told of its answer on its account since
	// it last went out: none while there's none.
	Verdict Verdict
	// Status is the answer that gave its verdict, or, once it's done, what
	// its client was answered: zero where its client went first.
	Status int
	// Ended is when it ended, as the stream told: zero while it hasn't.
	Ended time.Time
	// Lost is set where it was in flight as the watch last forgot what the
	// stream told, and the stream, opened again, hasn't told of it since: it
	// ended untold, as one cut off as its router went away does.
	Lost bool
	// Heard is when the stream last told of it.
	Heard time.Time
}

// Verdict is the upstream's answer refusing a request on an account, or
// throttling it there, as the request stream tells of it.
type Verdict int

const (
	// VerdictLimited is a 429 at the account's limit: its session moves.
	VerdictLimited Verdict = iota + 1
	// VerdictThrottled is a 429 the router sends again on the account.
	VerdictThrottled
	// VerdictRefused is a 401 or a 403: its session moves.
	VerdictRefused
)

// Moved is a session's model moving to another account, as the request
// stream told of it: the request that moved it, the session and its model,
// the accounts it left and went to, why, as the router gave it, and when;
// and whether a listing of the router's sessions has shown it done, listing
// the model where it went, or where a move of it since went.
type Moved struct {
	Request, Session, Model string
	From, To, Reason        string
	At                      time.Time
	Listed                  bool
}

// SameRow reports whether t tells what o does of its request's row as its
// table is built: all but what the stream tells as its answer streams, its
// characters so far and when it last told of it, which a row in flight
// draws as it's drawn, as RequestRow.Live has it. Its stage is of the row,
// so sent, its first byte, a verdict and its end each tell another.
func (t Told) SameRow(o Told) bool {
	t.Chars, t.Heard, o.Chars, o.Heard = 0, time.Time{}, 0, time.Time{}
	return t == o
}

// InFlight reports whether the request is in flight, as the stream told:
// neither ended nor lost.
func (t Told) InFlight() bool {
	return t.Ended.IsZero() && !t.Lost
}

// Spends reports whether the request spends quota: neither Claude Code's
// quota check nor a count of tokens.
func (t Told) Spends() bool {
	return !t.Check && !t.Count
}

// countedSession is the session the request counts toward, as its line
// will by ledger.Line.CountedSession: none for Claude Code's quota check,
// sent under an id it never uses again.
func (t Told) countedSession() string {
	if t.Check {
		return ""
	}
	return t.Session
}

// arrivesWithin is how long before a request first went upstream it may
// have arrived: the router reads it and chooses its account as it comes,
// moments, which a body slow to come draws out.
const arrivesWithin = time.Minute

// FiledOn is the local day the request's line is filed on, as the ledger
// files a line, by when its request arrived; false where the stream told of
// no sending. The stream tells no arrival, only when it first went upstream,
// moments after, so it's the earliest day it may have arrived on: of one
// sent within arrivesWithin after midnight, the day before, which today's
// lines as the ledger's tail reads them never hold. Its line filed on the
// day it was sent after all comes with today's.
func (t Told) FiledOn() (LedgerDay, bool) {
	if t.Sent.IsZero() {
		return LedgerDay{}, false
	}
	return Today(t.Sent.Add(-arrivesWithin)), true
}

// filedAs reports whether l is the request's line: the same request, as
// sameRequest has it, by when it first went upstream.
func (t Told) filedAs(l *ledger.Line) bool {
	return sameRequest(l, t.Request, t.Sent)
}

// Of is what s told of the session with the given id alone: its requests,
// its moves, and whether s is live. Its page is built of it as of s whole,
// none of another session's telling of it, so what changes it alone builds
// the page again.
func (s Stream) Of(session string) Stream {
	of := Stream{Live: s.Live}
	for id, t := range s.Requests {
		if t.Session != session {
			continue
		}
		if of.Requests == nil {
			of.Requests = make(map[string]Told)
		}
		of.Requests[id] = t
	}
	for _, m := range s.Moves {
		if m.Session == session {
			of.Moves = append(of.Moves, m)
		}
	}
	return of
}

// known are the requests s told of that a view may take as s told them,
// each view reading them through it alone: every one while s is live; else
// those that ended, or were lost. What of a request in flight as s last told
// of it is unknown while s isn't live, as the watch keeps what s told as it
// stands while s is closed: whether it's in flight still, the account it went
// out on last, and what its answer has streamed. It's known again as s, open
// again, tells of it, or forgets it, which loses it, or its line comes.
func (s Stream) known() iter.Seq[Told] {
	return func(yield func(Told) bool) {
		for _, t := range s.Requests {
			if (s.Live || !t.InFlight()) && !yield(t) {
				return
			}
		}
	}
}

// sentOf is when the request whose line l is went upstream, as s told,
// reporting false where s told of no such request, as filedAs has it. A
// request's sending, once told, stands whatever s knows of it now, as known
// has it, so a row stays where it sat in flight as its line fills it in.
func (s Stream) sentOf(l *ledger.Line) (time.Time, bool) {
	t, told := s.Requests[l.Request]
	if !told || !t.filedAs(l) {
		return time.Time{}, false
	}
	return t.Sent, true
}

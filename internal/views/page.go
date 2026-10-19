package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// SessionPage is a session's page, as sessions <id> prints it: where it runs
// and why, each account it ran on and what it took of each, its moves, its
// turns, newest first, and its totals. Fields may be added to it, never
// renamed.
type SessionPage struct {
	// Session, Dir, Account, Model, Running, LastSeen, Models, State,
	// MoveCost and Ended are as the List has them.
	Session  string              `json:"session"`
	Dir      string              `json:"dir,omitempty"`
	Account  string              `json:"account,omitempty"`
	Model    string              `json:"model,omitempty"`
	Running  bool                `json:"running,omitempty"`
	LastSeen time.Time           `json:"last_seen,omitzero"`
	Models   []SessionModel      `json:"models,omitempty"`
	State    string              `json:"state,omitempty"`
	MoveCost *ledger.Picodollars `json:"move_cost,omitempty"`
	Ended    time.Time           `json:"ended,omitzero"`
	// Pin is its own pin, as the router lists it, or while no router lists
	// the sessions, as its last listing had it: "" where it has none, or
	// the router doesn't list it.
	Pin string `json:"pin,omitempty"`
	// Started is when its first request came, and Resumed the latest time it
	// came back after an IdleHour or more without one: zero where it never
	// did.
	Started time.Time `json:"started,omitzero"`
	Resumed time.Time `json:"resumed,omitzero"`
	// KeptUntil is, of one ended, the date of the last day the ledger keeps
	// the lines of its first day, as 2027-11-04: "" of one running, and where
	// the ledger keeps them for good.
	KeptUntil string           `json:"kept_until,omitempty"`
	Accounts  []SessionAccount `json:"accounts"`
	Moves     []SessionMove    `json:"moves"`
	Turns     []Turn           `json:"turns"`
	Totals    SessionTotals    `json:"totals"`
	// Activity is what it's doing, as the List has it. It's the views'
	// alone.
	Activity Activity `json:"-"`
	// Goes are where its models go now, as its page's row 7 names them: as
	// the router lists them, or while it doesn't answer, as it last listed
	// them, the moves the request stream told of since taken in, each with
	// why it's there and since when; where the router lists it not, as
	// before it does or once it has ended, each model's latest account, as
	// its lines and the stream give it, with the reason of its latest line.
	// It's the views' alone.
	Goes []SessionModel `json:"-"`
	// Auto is the account the router would send its next request to were
	// its own pin cleared, as autoOf has it: "" where the router can't say,
	// as while it doesn't answer. It's the views' alone.
	Auto string `json:"-"`
}

// SessionAccount is an account a session ran on: its first and last request
// there, how many it made, what they'd have cost through the API at today's
// prices, as Worth sums it, and the points of each
// of the account's windows they took, by its key, an estimate, as pointsOf
// shares them out: none where no window of it was read.
type SessionAccount struct {
	Account  string    `json:"account"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Requests int       `json:"requests"`
	Worth
	Points map[string]int `json:"points,omitempty"`
}

// SessionMove is a request's move of its session from one account onto
// another: when it came, the model it asked for, as a session's models are
// routed apart, why, as the router gave it, the tokens it wrote to the cache,
// and what writing its context again there cost, nil where its cache would
// have run out anyway, or that isn't known.
type SessionMove struct {
	At      time.Time           `json:"at"`
	Model   string              `json:"model"`
	From    string              `json:"from"`
	To      string              `json:"to"`
	Reason  string              `json:"reason"`
	Written int                 `json:"written"`
	Cost    *ledger.Picodollars `json:"cost,omitempty"`
	// Rewrote is set where its session's next request of its conversation
	// wrote its context again on the account it moved to, its cache not yet
	// run out, priced or not, as Cost isn't where the price table can't
	// price it. It's the views' alone.
	Rewrote bool `json:"-"`
}

// Turn is a prompt of a session and every request that serves it, as Claude
// Code tells them: its number, from 1 at the first to start; when its first
// request came, and when it ended, zero while it's still going; its requests;
// the tools its answers called, most first; the tokens it read from the
// cache, wrote to it and put out; what it'd have cost through the API at
// today's prices, as Worth sums it; and the
// accounts its own conversation's requests went to, in the order they did,
// one gone back to coming again, so a subagent on another account, its model
// routed apart, doesn't read as a move: those of its requests where it has
// none of its own conversation.
type Turn struct {
	Turn     int         `json:"turn"`
	Started  time.Time   `json:"started"`
	Ended    time.Time   `json:"ended,omitzero"`
	Requests int         `json:"requests"`
	Tools    []ToolCalls `json:"tools"`
	Read     int         `json:"read"`
	Written  int         `json:"written"`
	Out      int         `json:"out"`
	Worth
	Accounts []string `json:"accounts"`
}

// ToolCalls are a tool's calls, by its name, and how many times it was
// called.
type ToolCalls struct {
	Tool  string `json:"tool"`
	Times int    `json:"times"`
}

// SessionTotals sum a session: its turns, its requests, the share of the
// tokens it sent that the cache served, and what its requests would have
// cost through the API at today's prices, as Worth sums it.
type SessionTotals struct {
	Turns     int     `json:"turns"`
	Requests  int     `json:"requests"`
	FromCache float64 `json:"from_cache"`
	Worth
}

// PageSources are what a session's page is built from.
type PageSources struct {
	// Routed is the session as the router lists it: nil where it doesn't, as
	// without the router, or once the session has ended.
	Routed *status.Session
	// Unlisted is set where no router lists the sessions, as SessionSources'
	// Unlisted has it.
	Unlisted bool
	// Read holds the session's lines, and the requests of the accounts it
	// ran on, on its days.
	Read SessionsRead
	// Stream is what the request stream has told of its requests: zero
	// where it's unheard, as by the verbs.
	Stream Stream
	// Prices are the price table worth is priced by.
	Prices ledger.Table
	// Keep is how long the ledger keeps a day's lines, as [ledger] keep
	// says, as ledgerKept reads it: dayfile.Forever keeps them for good.
	Keep time.Duration
	Now  time.Time
}

// SessionsRead is a read of the ledger for a session's page, as ReadSessions
// reads it: the sessions it read, by each one's id, each resting as the read
// rests it, with its lines after it; every account's answers on each day it
// read, every session's, as takenAnswers holds them, for the points; and the
// ledger it read them through, to read a session again whole where its page
// can't be held on what it rests on.
type SessionsRead struct {
	day      LedgerDay
	sessions map[string]ledger.SessionFrom
	answers  takenAnswers
	l        LedgerSessions
}

// ReadSessions reads, through l, at now, the sessions a page may be of, as
// the ledger's Sessions reads them: today's, those running gives, as the
// router lists them, and those with the given ids, as one given in whole, of
// an earlier day. Those the ids given name, as each starts them, rest on
// their pages, as onPage has it, the page shown being of one of them; every
// other on its state.
func ReadSessions(l Ledger, running []status.Session, now time.Time, ids ...string) SessionsRead {
	read := readOn(l, Today(now))
	read.sessions = l.Sessions(append(runningIDs(running), ids...), pagedIfNamed(ids), read.answers.take)
	return read
}

// pagedIfNamed has a read of the ledger's sessions rest those the ids given
// name, as startedBy has it of each, on their pages, as onPage has it, and
// every other on its state.
func pagedIfNamed(ids []string) func(string) ledger.Rest {
	return func(id string) ledger.Rest {
		if slices.ContainsFunc(ids, func(given string) bool { return startedBy(id, given) }) {
			return ledger.RestOnPage
		}
		return ledger.RestOnState
	}
}

// ReadSession reads, through l, at now, the session with the given id
// alone, as its page reads it once its id is known whole: as ReadSessions
// reads it, but through the ledger's SessionsOf, so none of today's other
// sessions' days is read for it.
func ReadSession(l LedgerSessions, now time.Time, id string) SessionsRead {
	read := readOn(l, Today(now))
	read.sessions = l.SessionsOf([]string{id}, onPage, read.answers.take)
	return read
}

// whole is the session with the given id read again alone, through the
// ledger r read it through, on r's day, resting on nothing, every day's
// answers taken afresh: as a page that can't be held on what it rests on is
// read.
func (r SessionsRead) whole(id string) SessionsRead {
	read := readOn(r.l, r.day)
	read.sessions = r.l.SessionsOf([]string{id}, eachWhole, read.answers.take)
	return read
}

// readOn is a read, through l, on the day given, holding nothing yet.
func readOn(l LedgerSessions, day LedgerDay) SessionsRead {
	return SessionsRead{day: day, answers: make(takenAnswers), l: l}
}

// heldFrom is the horizon the page of the session with the given id, as r
// read it, is held behind at now, each of its requests after it held open,
// so each a stream tells of, by its pins, is taken in in its place: the end
// of the day of the state it rests on, which holds sealed each of its
// requests before then, or before every moment, of one resting on none; or
// earlier, where a request the pins tell of may have arrived, as
// pinnedSince has it, the session read whole where its state holds that
// sealed.
func (r SessionsRead) heldFrom(id string, now time.Time, pins []time.Time) instant {
	horizon := unsealed
	if base := r.sessions[id].Base; base != nil {
		horizon = endOfDay(dayOf(base.Date))
	}
	return earlier(horizon, pinnedSince(now, pins))
}

// eachWhole has a read of the ledger's sessions read each whole, resting on
// no state a digest holds: every line of each given, as the store reads
// again one resting on a state that won't restore.
func eachWhole(string) ledger.Rest { return ledger.RestWhole }

// onPage has a read of the ledger's sessions rest each on its page, as
// RestOnPage has it: on the newest state a digest holds of it whose chain
// of parts reads, as a session's page is restored from them.
func onPage(string) ledger.Rest { return ledger.RestOnPage }

// AskedPage is the page of the session with the given id as it's drawn
// asked to open, while its lines are read: its id, and its directory as the
// router lists it, of the sessions running given, as dir has it of a
// session it lists, none where it doesn't list it; nothing else is known
// till its lines are.
func AskedPage(id string, running []status.Session) SessionPage {
	page := SessionPage{Session: id}
	if s := Routed(running, id); s != nil {
		page.Dir = routedDir(*s)
	}
	return page
}

// Named returns the ids of the sessions one is named among, by as much of its
// id as is unique: those read with a request today, in the order their first
// requests today came, then those running gives, as the router lists them,
// each once. Today's lines are read whatever a session rests on.
func (r SessionsRead) Named(running []status.Session) []string {
	first := make(map[string]time.Time)
	for id, from := range r.sessions {
		if i := slices.IndexFunc(from.Lines, func(l ledger.Line) bool { return isRequest(&l) && r.day.Holds(l.At) }); i >= 0 {
			first[id] = from.Lines[i].At
		}
	}
	ids := slices.SortedFunc(maps.Keys(first), func(a, b string) int { return cmp.Or(first[a].Compare(first[b]), strings.Compare(a, b)) })
	for _, s := range running {
		if _, ok := first[s.ID]; !ok && !slices.Contains(ids, s.ID) {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// SessionPageOf builds the page of the session with the given id from src:
// what the router says of it, where it lists it; what the request stream
// told of its requests, those in flight among them; and what every line of
// it the ledger holds tells, whatever day each is of, with the requests of
// the accounts it ran on, on its days, for the points it took, as src's Read
// holds them. It's listed as the List lists it; and its newest turn goes on
// while its last answer called a tool till the session has ended, as its
// Activity says, so with no router, while the router's own rule keeps it.
// It reports false where the ledger holds no request of it, and the router
// doesn't list it. It's the page HoldPage holds, built once, but held open
// from the end of the state its read rests it on, or from before a request
// the stream tells of, as heldFrom has it, so it's read again whole only
// where that state holds sealed what the stream tells of.
func SessionPageOf(id string, src PageSources) (SessionPage, bool) {
	held := holdPage(id, src.Read, src.Prices, src.Keep, src.Read.heldFrom(id, src.Now, pinsOf(src.Stream)[id]), src.Now)
	return held.Page(PageLive{Routed: src.Routed, Unlisted: src.Unlisted, Stream: src.Stream, Now: src.Now})
}

// KeptUntil says until when the ledger keeps a day's lines, by the date of
// the last day it keeps them, as SessionPage's KeptUntil gives it, at now:
// "kept until 4 Nov 2027", the year named where it isn't now's, as "kept
// until 29 Dec"; and for none, "kept for good", as the ledger keeping them
// for good gives none.
func KeptUntil(date string, now time.Time) string {
	if date == "" {
		return "kept for good"
	}
	until, ok := dayfile.Civil(date)
	switch {
	case !ok:
		return "kept until " + prose.Clean(date)
	case until.Year() != now.Year():
		return "kept until " + prose.DayMonthYear(until)
	}
	return "kept until " + prose.DayMonth(until)
}

func ended(l *ledger.Line) time.Time {
	return l.At.Add(time.Duration(l.TotalMS) * time.Millisecond)
}

// idled reports whether a session's requests that came at prev and then at
// next had an idle gap between them: an IdleHour or more, after which the
// router chooses its account afresh, so it came back, and its story folds
// the gap.
func idled(prev, next time.Time) bool {
	return next.Sub(prev) >= IdleHour
}

func movesOf(s *story) []SessionMove {
	moves := make([]SessionMove, len(s.moves))
	for i, m := range s.moves {
		moves[i] = SessionMove{At: m.at.UTC(), Model: m.model, From: m.from, To: m.to, Reason: m.reason, Written: m.written, Cost: m.cost, Rewrote: m.rewrote != nil}
	}
	return moves
}

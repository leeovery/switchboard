package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// SessionPage is a session's page, as sessions <id> prints it: where it runs
// and why, each account it ran on and what it took of each, its moves, its
// turns, newest first, and its totals. Fields may be added to it, never
// renamed.
type SessionPage struct {
	// Session, Dir, Running, LastSeen, Models, State, MoveCost and Ended are
	// as the List has them.
	Session  string              `json:"session"`
	Dir      string              `json:"dir,omitempty"`
	Running  bool                `json:"running,omitempty"`
	LastSeen time.Time           `json:"last_seen,omitzero"`
	Models   []SessionModel      `json:"models,omitempty"`
	State    string              `json:"state,omitempty"`
	MoveCost *ledger.Picodollars `json:"move_cost,omitempty"`
	Ended    time.Time           `json:"ended,omitzero"`
	// Pin is its own pin, as the router lists it: "" where it has none, or
	// the router doesn't list it.
	Pin string `json:"pin,omitempty"`
	// Started is when its first request came, and Resumed the latest time it
	// came back after comeBack or more without one: zero where it never did.
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
}

// SessionAccount is an account a session ran on: its first and last request
// there, how many it made, what they'd have cost through the API at today's
// prices, Unpriced naming the counts that leaves out, and the points of each
// of the account's windows they took, by its key, an estimate, as pointsOf
// shares them out: none where no window of it was read.
type SessionAccount struct {
	Account  string             `json:"account"`
	From     time.Time          `json:"from"`
	To       time.Time          `json:"to"`
	Requests int                `json:"requests"`
	Worth    ledger.Picodollars `json:"worth"`
	Unpriced []string           `json:"unpriced,omitempty"`
	Points   map[string]int     `json:"points,omitempty"`
}

// SessionMove is a request's move of its session from one account onto
// another: when it came, why, as the router gave it, the tokens it wrote to
// the cache, and what writing its context again there cost, nil where its
// cache would have run out anyway, or that isn't known.
type SessionMove struct {
	At      time.Time           `json:"at"`
	From    string              `json:"from"`
	To      string              `json:"to"`
	Reason  string              `json:"reason"`
	Written int                 `json:"written"`
	Cost    *ledger.Picodollars `json:"cost,omitempty"`
}

// Turn is a prompt of a session and every request that serves it, as Claude
// Code tells them: its number, from 1 at the first to start; when its first
// request came, and when it ended, zero while it's still going; its requests;
// the tools its answers called, most first; the tokens it read from the
// cache, wrote to it and put out; what it'd have cost through the API at
// today's prices, Unpriced naming the counts that leaves out; and the
// accounts its requests went to, in the order they first did.
type Turn struct {
	Turn     int                `json:"turn"`
	Started  time.Time          `json:"started"`
	Ended    time.Time          `json:"ended,omitzero"`
	Requests int                `json:"requests"`
	Tools    []ToolCalls        `json:"tools"`
	Read     int                `json:"read"`
	Written  int                `json:"written"`
	Out      int                `json:"out"`
	Worth    ledger.Picodollars `json:"worth"`
	Unpriced []string           `json:"unpriced,omitempty"`
	Accounts []string           `json:"accounts"`
}

// ToolCalls are a tool's calls, by its name, and how many times it was
// called.
type ToolCalls struct {
	Tool  string `json:"tool"`
	Times int    `json:"times"`
}

// SessionTotals sum a session: its turns, its requests, the share of the
// tokens it sent that the cache served, and what its requests would have
// cost through the API at today's prices, Unpriced naming the counts that
// leaves out.
type SessionTotals struct {
	Turns     int                `json:"turns"`
	Requests  int                `json:"requests"`
	FromCache float64            `json:"from_cache"`
	Worth     ledger.Picodollars `json:"worth"`
	Unpriced  []string           `json:"unpriced,omitempty"`
}

// PageSources are what a session's page is built from.
type PageSources struct {
	// Routed is the session as the router lists it: nil where it doesn't, as
	// without the router, or once the session has ended.
	Routed *status.Session
	// Ledger reads the session's lines, and those of the accounts it ran on,
	// on its days.
	Ledger Ledger
	// Prices are the price table worth is priced by.
	Prices ledger.Table
	// Keep is how long the ledger keeps a day's lines, as [ledger] keep
	// says: dayfile.Forever keeps them for good.
	Keep time.Duration
	Now  time.Time
}

// comeBack is how long a session goes without a request for the next to
// bring it back, as after an idle hour the router chooses its account
// afresh.
const comeBack = time.Hour

// SessionPageOf builds the page of the session with the given id from src:
// what the router says of it, where it lists it, and what every line of it
// the ledger holds tells, whatever day each is of, with the lines of the
// accounts it ran on, on its days, for the points it took. It reports false
// where the ledger holds no request of it, and the router doesn't list it.
func SessionPageOf(id string, src PageSources) (SessionPage, bool) {
	today := dayOf(src.Now)
	lines := sessionLines(src.Ledger, id)
	requests := requestsOf(lines, src.Prices, today.date)
	if len(requests) == 0 && src.Routed == nil {
		return SessionPage{}, false
	}
	s := storyOf(lines, src.Prices, today)
	listed := listedFrom(id, s)
	if src.Routed != nil {
		listed.run(*src.Routed, s, src.Now)
	}
	page := SessionPage{
		Session: listed.Session, Dir: listed.Dir, Running: listed.Running, LastSeen: listed.LastSeen, Models: listed.Models,
		State: listed.State, MoveCost: listed.MoveCost, Ended: listed.Ended, Started: listed.Started,
		Resumed: resumedAt(requests), Accounts: accountsOf(requests), Moves: movesOf(s), Turns: turnsOf(requests, listed.Running),
	}
	if src.Routed != nil {
		page.Pin = src.Routed.Pin
	}
	if !page.Running && s.first != nil {
		page.KeptUntil, _ = dayfile.KeptUntil(dayOf(s.first.At).date, src.Keep)
	}
	points := pointsOf(src.Ledger, id, requests)
	for i := range page.Accounts {
		page.Accounts[i].Points = points[page.Accounts[i].Account]
	}
	page.Totals = totalsOf(requests, len(page.Turns))
	return page, true
}

// request is a session's request, as its page counts it: its line, what it
// would have cost through the API, as ledger.Table.Summed gives it, and the
// tokens its answer's usage counts, none where it gave none.
type request struct {
	*ledger.Line
	worth  ledger.Worth
	tokens quota.Tokens
}

// requestsOf returns the requests among lines, a session's, oldest first, as
// isRequest says, each priced by prices at the local day with the given
// date.
func requestsOf(lines []ledger.Line, prices ledger.Table, date string) []request {
	var requests []request
	for i := range lines {
		l := &lines[i]
		if !isRequest(l) {
			continue
		}
		tokens, _ := l.Tokens()
		requests = append(requests, request{Line: l, worth: prices.Summed(l, date), tokens: tokens})
	}
	return requests
}

// ended is when the request the line l holds ended: its arrival, and its
// whole time after it.
func ended(l *ledger.Line) time.Time {
	return l.At.Add(time.Duration(l.TotalMS) * time.Millisecond)
}

// resumedAt returns when requests, a session's, oldest first, last came
// back after comeBack or more without one: zero where they never did.
func resumedAt(requests []request) time.Time {
	var resumed time.Time
	for i := 1; i < len(requests); i++ {
		if requests[i].At.Sub(requests[i-1].At) >= comeBack {
			resumed = requests[i].At.UTC()
		}
	}
	return resumed
}

// accountsOf returns the accounts requests, a session's, oldest first, went
// to, in the order they first did, each with its first and last request
// there, their count and their worth: one the router answered itself went to
// none.
func accountsOf(requests []request) []SessionAccount {
	var accounts []SessionAccount
	var worth []worthSum
	for _, r := range requests {
		if r.Account == "" {
			continue
		}
		i := slices.IndexFunc(accounts, func(a SessionAccount) bool { return a.Account == r.Account })
		if i < 0 {
			i = len(accounts)
			accounts, worth = append(accounts, SessionAccount{Account: r.Account, From: r.At.UTC()}), append(worth, worthSum{})
		}
		accounts[i].To = r.At.UTC()
		accounts[i].Requests++
		worth[i].add(r.worth)
	}
	for i := range accounts {
		accounts[i].Worth, accounts[i].Unpriced = worth[i].cost, worth[i].unpriced
	}
	if accounts == nil {
		return []SessionAccount{}
	}
	return accounts
}

// movesOf returns each move the session's story tells, in the order they
// came.
func movesOf(s *story) []SessionMove {
	moves := make([]SessionMove, len(s.moves))
	for i, m := range s.moves {
		moves[i] = SessionMove{At: m.at.UTC(), From: m.from, To: m.to, Reason: m.reason, Written: m.written, Cost: m.cost}
	}
	return moves
}

// totalsOf sums requests, a session's, of which it told turns turns: the
// share of the tokens they sent that the cache served is their reads from it
// over their input, those reads and their writes to it together, none where
// they sent none.
func totalsOf(requests []request, turns int) SessionTotals {
	var worth worthSum
	var read, sent int
	for _, r := range requests {
		worth.add(r.worth)
		read += r.tokens.CacheRead
		sent += r.tokens.Input + r.tokens.CacheRead + r.tokens.CacheWrite
	}
	totals := SessionTotals{Turns: turns, Requests: len(requests), Worth: worth.cost, Unpriced: worth.unpriced}
	if sent > 0 {
		totals.FromCache = float64(read) / float64(sent)
	}
	return totals
}

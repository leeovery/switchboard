package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// Playback is a router's state played back from what the router recorded,
// the readings history and the request ledger, each taken in as the router
// took it in as it came, through the router's own state and sessions, so its
// document at a moment is the one the router would have built then, as far
// as those records tell: for cmd/forecasttool, which scores the weeks'
// forecasts against what happened. Nothing routes by it. What the records
// don't hold, it has none of: no account reaches a limit, nor has its token
// refused, as none has for a router that has just started, and no pin
// holds, as no history of the pin is kept. Every account configured has a
// usable token, though none is read, as a playback sends nothing. It isn't
// safe for concurrent use.
type Playback struct {
	state    *state
	sessions *sessions
	// taking is the readings history as taken up so far, as seed takes it up,
	// a reading at a time, from none.
	taking seeding
	// now is the time of the document asked for last, which the state's clock
	// reads; pruned is when the sessions were last pruned, as the state file
	// prunes them every pruneEvery.
	now, pruned time.Time
	// ends are the ends of the requests taken in that are yet to come, and
	// are to be taken in, soonest first, the first taken in first where they
	// end together.
	ends []requestEnd
}

// requestEnd is the end of a request taken in, as the proxy takes it in once
// the request is done: when; the request; the account whose answer its client
// had; the tokens its answer's usage gave, where weighs says the hold curve
// takes them in; and whether its session is forgotten then, as forgets says.
type requestEnd struct {
	at              time.Time
	req             Request
	account         string
	tokens          quota.Tokens
	weighs, forgets bool
}

// NewPlayback returns a playback of the accounts configured, as policy judges
// them, before anything is taken in.
func NewPlayback(configured config.Accounts, policy score.Policy) *Playback {
	p := &Playback{taking: newSeeding(nil, policy.Perishable)}
	held := make(accounts, len(configured))
	for i, a := range configured {
		held[i] = account{Account: a, secret: &secret{}}
	}
	clock := func() time.Time { return p.now }
	// No family is needed: a playback takes no answer's windows in, which a
	// model's family says which of count.
	p.state = newState(held, unpooled(), policy, nil, clock, func() {}, func() {})
	p.sessions = newSessions(clock, func() {}, func() {})
	return p
}

// Read takes in r, a reading of the readings history, as seed takes it up as
// the router starts, and as the window it reads now stands, as the state
// file would have kept it: started again at r's time where it shows the
// window reset by hand, as startedAgain has it. A reading of an account not
// configured is passed over, as seed passes it over. Readings are given in
// the order they were read, as the history gives them.
func (p *Playback) Read(r readings.Reading) {
	s := p.state
	s.mu.Lock()
	defer s.mu.Unlock()
	u, configured := s.usage[r.Account]
	if !configured {
		return
	}
	held := u.windows[r.Key]
	s.takeUpReading(p.taking, u, r)
	u.windows[r.Key] = startedAgain(held, r.Window(), r.At)
	u.readAt[r.Key], u.updated = r.At, r.At
}

// Line takes in the request the line of the request ledger is of, as the
// router took it in. As it arrived, its session is remembered on each
// account it went out on, in the order it did, as the scheduler remembers
// where it sent the session at each choice: a session comes to an account at
// its first request there, and stays on it while its requests go there. As
// it ended, its answer's tokens, where its request spends quota and its
// usage gave them, are taken into the hold curve by how long its session had
// been on its account by then, as the proxy weighs an answer; then its
// session is forgotten where the router forgot it: one new to the request,
// as the request wasn't answered with success, and one the router took the
// request's moves of back, as every account that judged it refused it.
// Lines are given in the order their requests arrived, as the ledger gives
// them, each after what was read before it.
func (p *Playback) Line(l ledger.Line) {
	p.until(l.At)
	req := Request{ID: l.Request, Session: l.Session, Model: l.Model, Check: l.Kind == ledger.KindCheck, Tried: attemptsOf(l.Tried)}
	fresh := p.wentOut(req, l)
	tokens, answered := l.Tokens()
	e := requestEnd{
		at: l.At.Add(time.Duration(l.TotalMS) * time.Millisecond), req: req, account: l.Account, tokens: tokens,
		weighs:  answered && l.Kind != ledger.KindCount,
		forgets: forgottenOnceDone(fresh, l.Status) || takenBack(req, l.Account),
	}
	if !e.weighs && !e.forgets {
		return
	}
	later := slices.IndexFunc(p.ends, func(o requestEnd) bool { return o.at.After(e.at) })
	if later < 0 {
		later = len(p.ends)
	}
	p.ends = slices.Insert(p.ends, later, e)
}

// wentOut remembers the session of req, the request of l, on each account it
// went out on, as the scheduler remembers it at each choice, all at once as
// it arrived, and reports whether its session was new to it, as the
// chooser's first choice for it says. It went out on each it was tried on,
// in order, then on the account whose answer its client had, where that
// isn't among them, as a request goes out on no account twice: one that is
// had its answer from an account it was tried on, none being left.
func (p *Playback) wentOut(req Request, l ledger.Line) (fresh bool) {
	accounts := req.tried()
	if _, tried := req.attempt(l.Account); !tried {
		accounts = append(accounts, l.Account)
	}
	for i, id := range accounts {
		if !req.remembered(id) {
			continue
		}
		found := p.sessions.lookup(req.key())
		if i == 0 {
			fresh = !found.assigned
		}
		p.sessions.remember(req, found.current, decision{account: id, reason: l.Reason}, l.At)
	}
	return fresh
}

// takenBack reports whether the router took back the moves of req, as it
// does once every account that judged a request refused it, none being left:
// the account whose answer its client had, account, is one it was tried on,
// as no account was left to try after it.
func takenBack(req Request, account string) bool {
	_, leftNone := req.attempt(account)
	return leftNone && req.refusedEverywhere()
}

// attemptsOf are the attempts tried gives, as the request had them.
func attemptsOf(tried []ledger.Tried) []Attempt {
	attempts := make([]Attempt, len(tried))
	for i, t := range tried {
		attempts[i] = Attempt{Account: t.Account, Why: t.Why}
	}
	return attempts
}

// Document is the status document the router would have built at at, with
// what's worked out of it, as Status gives it, but for what a playback has
// none of: of all it has taken in, and each answer that ended by then. at
// comes no earlier than the times of what was taken in before.
func (p *Playback) Document(at time.Time) status.Document {
	p.until(at)
	p.now = at
	return p.state.document().WorkedOut(p.state.policy)
}

// until takes in what comes by at: each request's end that has come by then,
// in the order they came, as end says, and the sessions pruned, as the state
// file prunes them, where that's due.
func (p *Playback) until(at time.Time) {
	for len(p.ends) > 0 && !p.ends[0].at.After(at) {
		e := p.ends[0]
		p.ends = p.ends[1:]
		p.end(e)
	}
	if at.Sub(p.pruned) >= pruneEvery {
		p.sessions.prune(at)
		p.pruned = at
	}
}

// end takes in e, a request's end, as the proxy does once the request is
// done: its answer weighed, then its session forgotten, each where e says.
func (p *Playback) end(e requestEnd) {
	if e.weighs {
		p.state.answered(e.tokens, p.sessions.onAccount(e.req.key(), e.account, e.at), e.at)
	}
	if e.forgets {
		p.sessions.forget(e.req)
	}
}

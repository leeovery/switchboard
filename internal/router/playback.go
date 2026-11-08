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
// refused, as none has for a router that has just started, and no global pin
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
	// ends are the answers to the requests taken in that are yet to end,
	// soonest first, the first taken in first where they end together.
	ends []answerEnd
}

// answerEnd is the answer to a request taken in, as the hold curve takes it in
// once it ends: when, the session and model the request was of, the account
// whose answer its client had, and the tokens its usage gave.
type answerEnd struct {
	at      time.Time
	of      key
	account string
	tokens  quota.Tokens
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
	p.state = newState(held, policy, nil, clock, func() {}, func() {})
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
// router took it in: as it arrived, its session remembered on the account
// whose answer its client had, as the scheduler remembers where it sent the
// session, which comes to an account at its first request there and stays
// on it while its requests go there; and as it ended, an answer's tokens,
// where its request spends quota and its usage gave them, taken into the
// hold curve by how long its session had been on its account by then, as the
// proxy weighs an answer. Lines are given in the order their requests
// arrived, as the ledger gives them, each after what was read before it.
func (p *Playback) Line(l ledger.Line) {
	p.until(l.At)
	req := Request{ID: l.Request, Session: l.Session, Model: l.Model, Check: l.Kind == ledger.KindCheck}
	if req.remembered(l.Account) {
		p.sessions.remember(req, p.sessions.lookup(req.key()).current, decision{account: l.Account, reason: l.Reason}, l.At)
	}
	tokens, answered := l.Tokens()
	if !answered || l.Kind == ledger.KindCount {
		return
	}
	e := answerEnd{at: l.At.Add(time.Duration(l.TotalMS) * time.Millisecond), of: req.key(), account: l.Account, tokens: tokens}
	later := slices.IndexFunc(p.ends, func(o answerEnd) bool { return o.at.After(e.at) })
	if later < 0 {
		later = len(p.ends)
	}
	p.ends = slices.Insert(p.ends, later, e)
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

// until takes in what comes by at: each answer that has ended by then, in
// the order they ended, and the sessions pruned, as the state file prunes
// them, where that's due.
func (p *Playback) until(at time.Time) {
	for len(p.ends) > 0 && !p.ends[0].at.After(at) {
		e := p.ends[0]
		p.ends = p.ends[1:]
		p.state.answered(e.tokens, p.sessions.onAccount(e.of, e.account, e.at), e.at)
	}
	if at.Sub(p.pruned) >= pruneEvery {
		p.sessions.prune(at)
		p.pruned = at
	}
}

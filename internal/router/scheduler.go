package router

import (
	"context"
	"time"
)

const (
	// probeWait bounds how long a choice waits for fresh usage, so a
	// session's first request never waits long.
	probeWait = 8 * time.Second
	// recheckWait bounds how long a choice that found no account with room
	// waits for those it probes again.
	recheckWait = 5 * time.Second
)

// scheduler is the router's Chooser. It keeps each session's requests of a
// model on one account, so their prompt cache stays warm, choosing afresh
// only for a new session, one idle past its cache's life, or one whose
// account has no room: then it chooses the account whose quota would be lost
// soonest unused, having probed the accounts whose usage is stale. See decide
// for the whole order.
type scheduler struct {
	// accounts are those requests can go out on.
	accounts accounts
	state    *state
	sessions *sessions
	probes   *probes
	now      func() time.Time
	emit     func(Event)
}

func (s *scheduler) Choose(ctx context.Context, req Request) Choice {
	d, was := s.decide(req)
	if d.afresh && s.probes.await(ctx, s.accounts, s.state.due, probeWait) {
		d, was = s.decide(req)
	}
	if d.noRoom && s.recheck(ctx, req) {
		d, was = s.decide(req)
	}
	if req.Session != "" {
		s.remember(req, was, d)
	}
	return Choice{Account: d.account, Reason: d.reason, NoRoom: d.noRoom}
}

// decide chooses on what's known now, and returns the session's assignment
// the choice was made on, zero for none.
func (s *scheduler) decide(req Request) (decision, assignment) {
	now := s.now()
	current, assigned, pin := s.sessions.lookup(key{session: req.Session, model: req.Model})
	return decide(situation{
		req:      req,
		now:      now,
		current:  current,
		assigned: assigned,
		pin:      pin,
		accounts: s.view(req, now),
	}), current
}

// view is what a choice for req knows at now: an account the request has
// been tried on has no room for it.
func (s *scheduler) view(req Request, now time.Time) view {
	return s.state.view(req.Model, now).without(req.tried())
}

// recheck probes again the accounts whose usage as last read leaves no room
// for req, once a minute at most, as a window may have reset with no traffic
// to show it. It reports whether it did.
func (s *scheduler) recheck(ctx context.Context, req Request) bool {
	full := s.view(req, s.now()).full()
	underway := s.probes.start(s.accounts.only(full), s.state.dueAgain)
	if len(underway) == 0 {
		return false
	}
	logger.Warn("no account has room; probing again", "accounts", accountsOf(underway))
	s.probes.wait(ctx, underway, recheckWait)
	return true
}

// remember notes where a session's request went, as its choice d says, made
// on the session's assignment as was, and logs and tells of a move. Should
// another request of the session have moved it since, the newer assignment
// stands, and the log says so: the request goes where d says all the same.
func (s *scheduler) remember(req Request, was assignment, d decision) {
	now := s.now()
	found, noted := s.sessions.remember(key{session: req.Session, model: req.Model}, was, req.Pin, d, now)
	switch {
	case !noted:
		logger.Info("session moved meanwhile; its newer assignment stands", "session", prefix(req.Session, sessionShown), "model", req.Model,
			"account", found.Account, "chosen", d.account)
	case found.Account != "" && found.Account != d.account:
		logger.Info("moved", "session", prefix(req.Session, sessionShown), "model", req.Model,
			"from", found.Account, "to", d.account, "reason", d.reason)
		s.emit(Moved{Session: req.Session, Model: req.Model, From: found.Account, To: d.account, Reason: d.reason,
			Forced: !s.view(req, now).room(found.Account)})
	}
}

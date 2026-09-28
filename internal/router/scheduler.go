package router

import (
	"context"
	"time"
)

// probeWait bounds how long a choice waits for fresh usage, so a session's
// first request never waits long.
const probeWait = 8 * time.Second

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
}

func (s *scheduler) Choose(ctx context.Context, req Request) Choice {
	d := s.decide(req)
	if d.afresh && s.probes.await(ctx, s.accounts, probeWait) {
		d = s.decide(req)
	}
	if req.Session != "" {
		s.remember(req, d)
	}
	return Choice{Account: d.account, Reason: d.reason}
}

// decide chooses on what's known now.
func (s *scheduler) decide(req Request) decision {
	now := s.now()
	current, assigned, pin := s.sessions.lookup(key{session: req.Session, model: req.Model})
	return decide(situation{
		req:      req,
		now:      now,
		current:  current,
		assigned: assigned,
		pin:      pin,
		accounts: s.state.view(req.Model, now),
	})
}

// remember notes where a session's request went, and logs a move.
func (s *scheduler) remember(req Request, d decision) {
	before := s.sessions.remember(key{session: req.Session, model: req.Model}, req.Pin, d, s.now())
	if before != "" && before != d.account {
		logger.Info("moved", "session", prefix(req.Session, sessionShown), "model", req.Model,
			"from", before, "to", d.account, "reason", d.reason)
	}
}

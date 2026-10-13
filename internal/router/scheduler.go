package router

import (
	"context"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// probeWait bounds how long a choice waits for fresh usage, so a
	// session's first request never waits long.
	probeWait = 8 * time.Second
	// recheckWait bounds how long a choice that found no account with room
	// waits for those it probes again.
	recheckWait = 5 * time.Second
	// beforeChoosing is what a choice's probes are for, as the log tells it.
	beforeChoosing = "before choosing"
)

// scheduler is the router's Chooser. It keeps each session's requests of a
// model on one account, so their prompt cache stays warm, and their reasoning
// with them where the model's thinking is bound to the account, choosing
// afresh only for a new session, one idle past its cache's life whose
// model's thinking isn't bound, or one whose account has no room: then it
// chooses the account whose quota would be lost soonest unused, having probed
// the accounts whose usage is stale. See decide for the whole order.
type scheduler struct {
	// accounts are every account configured, among which requests go out on
	// those with tokens.
	accounts accounts
	state    *state
	sessions *sessions
	probes   *probes
	now      func() time.Time
	emit     func(Event)
}

func (s *scheduler) Choose(ctx context.Context, req Request) Choice {
	d, on := s.decide(req)
	if d.afresh && s.probes.await(ctx, s.accounts.sendable(), s.state.due, probeWait, beforeChoosing) {
		d, on = s.decide(req)
	}
	if d.noRoom && s.recheck(ctx, req) {
		d, on = s.decide(req)
	}
	if d.passedOver != "" {
		notePressure(req, d)
	}
	c := Choice{Account: d.account, Reason: d.reason, NoRoom: d.noRoom, Reserved: d.reserved, Back: d.back}
	if req.Session != "" && d.account != "" && !req.Check {
		c.New, c.From = s.remember(on, d)
	}
	return c
}

func (s *scheduler) Answered(req Request, account, reason string) {
	if req.Check {
		return
	}
	a, ok := s.sessions.started(req.key())
	if !ok {
		return
	}
	if a.Account != account {
		a.Account, a.Reason = account, reason
	}
	s.emit(SessionStarted{Session: req.Session, Model: req.Model, Account: a.Account, Reason: a.Reason})
}

func (s *scheduler) Forget(req Request) (string, bool) {
	back, ok := s.sessions.forget(req)
	switch {
	case !ok:
	case back == "":
		logger.Debug("forgot a new session whose request went unanswered", "id", req.ID, "session", status.ShortID(req.Session), "model", req.Model)
	default:
		logger.Info("session back where it was before its request", "id", req.ID, "session", status.ShortID(req.Session), "model", req.Model, "account", back)
	}
	return back, ok
}

// decide chooses on what's known now, and returns the situation the choice
// was made in, as situation builds it.
func (s *scheduler) decide(req Request) (decision, situation) {
	now := s.now()
	found := s.sessions.lookup(req.key())
	on := found.situation(req, now, s.view(req, now))
	return decide(on), on
}

// auto returns the account the next request of model, of the session with the
// given id, would go out on at now were the session to have no pin of its own,
// as decide chooses it: the router's own rule for the session, which keeps it
// where it runs while that account has room and a move would cost it
// something. It chooses on what's known, probing no account and noting
// nothing in the log. It returns "" where it can't say: the session has no
// assignment for the model, or no account has room for the request, which
// would have the accounts probed again before it went out, if only for the
// upstream to refuse it.
func (s *scheduler) auto(id, model string, now time.Time) string {
	req := Request{Session: id, Model: model, Bound: s.sessions.bound(model)}
	found := s.sessions.lookup(req.key())
	if !found.assigned {
		return ""
	}
	found.given = false
	d := decide(found.situation(req, now, s.state.glance(model, now)))
	if d.noRoom {
		return ""
	}
	return d.account
}

// view is what a choice for req knows at now: an account the request has
// been tried on has no room for it.
func (s *scheduler) view(req Request, now time.Time) view {
	return s.state.view(req.Model, now).without(req.tried())
}

// notePressure notes in the log the account the choice d passed over as under
// pressure, and why, as the choice judged it: the rate its pressure window is
// being used at, and when, at that rate, it runs out, which comes before it
// resets.
func notePressure(req Request, d decision) {
	logger.Info("passed over under pressure", "id", req.ID, "account", d.passedOver, "rate", prose.Percent(d.pressure.Rate)+" an hour",
		"runs_out", d.pressure.RunsOut, "chosen", d.account)
}

// recheck probes again the accounts whose usage as last read leaves no room
// for req, once a minute at most, as a window may have reset with no traffic
// to show it. It reports whether it did.
func (s *scheduler) recheck(ctx context.Context, req Request) bool {
	full := s.view(req, s.now()).full()
	underway := s.probes.start(s.accounts.sendable().only(full), s.state.dueAgain)
	if len(underway) == 0 {
		return false
	}
	logger.Warn("no account has room; probing again", "accounts", accountsOf(underway))
	s.probes.wait(ctx, underway, recheckWait, beforeChoosing)
	return true
}

// remember notes where a session's request went, as its choice d says, made
// in the situation on, and logs and tells of a move. Should another request
// of the session have moved it since, the newer assignment stands, and the
// log says so: the request goes where d says all the same. It reports whether
// it gave the session its first account for the request's model, and returns
// the account it moved the session from, "" when it didn't move it.
func (s *scheduler) remember(on situation, d decision) (first bool, from string) {
	req, now := on.req, s.now()
	found, noted := s.sessions.remember(req, on.current, d, now)
	switch {
	case !noted:
		logger.Info("session moved meanwhile; its newer assignment stands", "session", status.ShortID(req.Session), "model", req.Model,
			"account", found.Account, "chosen", d.account)
	case found.Account != "" && found.Account != d.account:
		logger.Info("moved", "session", status.ShortID(req.Session), "model", req.Model,
			"from", found.Account, "to", d.account, "reason", d.reason)
		s.emit(Moved{Session: req.Session, Model: req.Model, From: found.Account, To: d.account, Reason: d.reason, Held: d.held, Limit: d.limit})
		from = found.Account
	}
	return noted && !on.assigned, from
}

package router

import (
	"context"
	"time"

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
	// due and dueAgain report whether an account wants probing before a
	// choice afresh, and again before one that found no account with room:
	// state's due and dueAgain.
	due, dueAgain func(id string, now time.Time) bool
	now           func() time.Time
	emit          func(Event)
}

// Choose decides again once it has looked for the probes a choice afresh
// waits on, and again once it has looked for accounts without room to probe
// again, whatever either found. A reading recorded between a decision and the
// look for what's due leaves nothing due, yet the decision lacked it. Nor
// will deciding again only where a reading came in do: a decision reads the
// clock as well as the readings, so a window that reset, or a limit or a
// refusal that lifted, while the look waited, up to probeWait or recheckWait,
// changes it with no reading taken in.
func (s *scheduler) Choose(ctx context.Context, req Request) Choice {
	d, on := s.decide(req)
	if d.afresh {
		s.probes.await(ctx, s.accounts.sendable(), s.due, probeWait, beforeChoosing)
		d, on = s.decideAgain(req, d)
	}
	if d.noRoom {
		s.recheck(ctx, req)
		d, on = s.decideAgain(req, d)
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
	left, back, ok := s.sessions.forget(req)
	attrs := []any{"id", req.ID, "session", status.ShortID(req.Session), "model", req.Model}
	switch {
	case !ok:
	case back.Account == "":
		logger.Debug("forgot a new session whose request went unanswered", attrs...)
	case back.Account != left.Account:
		logger.Info("session back where it was before its request", append(attrs, "account", back.Account)...)
	default:
		logger.Debug("session's assignment put back as it stood before its request",
			append(attrs, "account", back.Account, "reason", back.Reason, "assigned", back.AssignedAt, "last_seen", back.LastSeen)...)
	}
	return back.Account, ok
}

// decide chooses on what's known now, and returns the situation the choice
// was made in: the request pinned as its session's own pin has it, a pin given
// while the session runs passing over the one it was launched with.
func (s *scheduler) decide(req Request) (decision, situation) {
	now := s.now()
	found := s.sessions.lookup(req.key())
	on := situation{
		req:      req,
		now:      now,
		current:  found.current,
		assigned: found.assigned,
		pin:      found.global,
		accounts: s.view(req, now),
	}
	on.req.Pin, on.pinnedAt = found.pin(req.Pin)
	return decide(on), on
}

// decideAgain decides for req as decide does, and logs each part of the new
// decision that differs from was, the decision made before it.
func (s *scheduler) decideAgain(req Request, was decision) (decision, situation) {
	d, on := s.decide(req)
	if changed := d.changes(was); len(changed) > 0 {
		logger.Debug("decision overturned", append([]any{"id", req.ID, "session", status.ShortID(req.Session), "model", req.Model}, changed...)...)
	}
	return d, on
}

// changes returns, for the log, each part of d that tells where the request
// goes and why, the move it makes and what it passed over, that differs from
// was: under its own key as d has it, and that key with _before as was had it.
func (d decision) changes(was decision) []any {
	var attrs []any
	note := func(key string, differs bool, now, before any) {
		if differs {
			attrs = append(attrs, key, now, key+"_before", before)
		}
	}
	note("account", d.account != was.account, d.account, was.account)
	note("reason", d.reason != was.reason, d.reason, was.reason)
	note("no_room", d.noRoom != was.noRoom, d.noRoom, was.noRoom)
	note("reserved", d.reserved != was.reserved, d.reserved, was.reserved)
	note("back", !d.back.Equal(was.back), d.back, was.back)
	note("held", d.held != was.held, d.held, was.held)
	note("limit", d.limit != was.limit, d.limit, was.limit)
	note("passed_over", d.passedOver != was.passedOver, d.passedOver, was.passedOver)
	return attrs
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
	logger.Info("passed over under pressure", "id", req.ID, "account", d.passedOver, "rate", status.Percent(d.pressure.Rate)+" an hour",
		"runs_out", d.pressure.RunsOut, "chosen", d.account)
}

// recheck probes again the accounts whose usage as last read leaves no room
// for req, once a minute at most, as a window may have reset with no traffic
// to show it.
func (s *scheduler) recheck(ctx context.Context, req Request) {
	full := s.view(req, s.now()).full()
	underway := s.probes.start(s.accounts.sendable().only(full), s.dueAgain)
	if len(underway) == 0 {
		return
	}
	logger.Warn("no account has room; probing again", "accounts", accountsOf(underway))
	s.probes.wait(ctx, underway, recheckWait, beforeChoosing)
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

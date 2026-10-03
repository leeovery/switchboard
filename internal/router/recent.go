package router

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// maxEvents is how many of its events the router keeps for its status
// document: the newest.
const maxEvents = 50

// recent keeps what has happened lately, for the router's status document:
// its newest maxEvents events, in memory alone, so a restart starts them
// afresh, each with an id one more than the event's before it, from 1 as the
// router starts. It's kept whatever the notifications are set to. It hears
// the router's events without ever holding the router up, and looks at the
// accounts every lookEvery, and after each event it hears, for what no event
// tells of: an account coming under pressure, and one having room again.
// It's safe for concurrent use.
type recent struct {
	state    *state
	sessions *sessions
	now      func() time.Time
	// nudge is nudged as an event is heard, for run to look at the accounts
	// again.
	nudge chan struct{}

	mu sync.Mutex
	// kept are the events kept, the oldest first, and last is the id of the
	// newest.
	kept []happening
	last int

	// Only run's goroutine touches what follows.
	short ranOut
	// under holds, by id, whether each account was under pressure as last
	// looked at, and told when the window each was last told of as under
	// pressure in resets.
	under map[string]bool
	told  map[string]time.Time
}

func newRecent(state *state, sessions *sessions, now func() time.Time) *recent {
	return &recent{
		state:    state,
		sessions: sessions,
		now:      now,
		nudge:    make(chan struct{}, 1),
		short:    make(ranOut),
		under:    make(map[string]bool),
		told:     make(map[string]time.Time),
	}
}

// happening is an event kept, and for a limit's, the limit, as it comes to
// stand, which gives the event its windows, when it lifts, and the sessions
// it moved.
type happening struct {
	status.Event
	limit *limitMoves
}

// event is the happening as the status document gives it, a copy: a limit's
// as its limit now stands, with how many sessions it moved, and where, when
// they all went to one account.
func (h happening) event() status.Event {
	e := h.Event
	if h.limit != nil {
		e.Account, e.Windows, e.Until = h.limit.Account, h.limit.Windows, h.limit.Until
		e.Count = len(h.limit.sessions)
		if len(h.limit.to) == 1 {
			e.To = h.limit.to[0]
		}
	}
	e.Windows = slices.Clone(e.Windows)
	return e
}

// hear keeps what one of the router's events tells of, as it happens, and
// nudges run to look at the accounts again, never waiting for it.
func (r *recent) hear(e Event) {
	now := r.now().UTC()
	r.mu.Lock()
	r.take(e, now)
	r.mu.Unlock()
	select {
	case r.nudge <- struct{}{}:
	default:
	}
}

// take keeps what an event that happened at now tells of. r.mu must be held.
func (r *recent) take(e Event, now time.Time) {
	switch e := e.(type) {
	case SessionStarted:
		r.add(status.Event{Kind: status.EventStarted, Account: e.Account, Session: e.Session, Model: e.Model, Reason: e.Reason}, now)
	case LimitReached:
		r.reached(e, now)
	case Moved:
		r.moved(e, now)
	case Refused:
		r.add(status.Event{Kind: status.EventRefused, Account: e.Account, Until: e.Until, Status: e.Status, Family: e.Family}, now)
	case HealthChanged:
		r.add(status.Event{Kind: status.EventHealth, Reason: e.Reason}, now)
	case Primed:
		r.add(status.Event{Kind: status.EventPrimed, Account: e.Account, Windows: []string{e.Window}, Until: e.ResetsAt}, now)
	case RestartDue:
		r.add(status.Event{Kind: status.EventRestart, Reason: e.Reason}, now)
	}
}

// reached keeps a limit an account reached at now: a new one is news of its
// own, and one reached again while it holds joins the account's latest
// limit, unless that's no longer kept.
func (r *recent) reached(e LimitReached, now time.Time) {
	if !e.Again {
		r.keep(happening{Kind: status.EventLimit, limit: newLimitMoves(e)}, now)
		return
	}
	if h, ok := r.latestLimit(e.Account); ok {
		h.limit.join(e)
	}
}

// moved keeps a session's move at now, counting it among the sessions the
// latest limit of the account it left moved when the session had to leave,
// and that limit hadn't lifted by now; a move it counts names that limit's
// event.
func (r *recent) moved(e Moved, now time.Time) {
	move := status.Event{Kind: status.EventMoved, Session: e.Session, Model: e.Model, From: e.From, To: e.To, Reason: e.Reason}
	if h, ok := r.latestLimit(e.From); e.Forced && ok && now.Before(h.limit.Until) {
		h.limit.add(e)
		move.Limit = h.ID
	}
	r.add(move, now)
}

// latestLimit returns the latest limit event kept of the account with the
// given id, reporting false when none is kept. r.mu must be held.
func (r *recent) latestLimit(id string) (happening, bool) {
	for _, h := range slices.Backward(r.kept) {
		if h.limit != nil && h.limit.Account == id {
			return h, true
		}
	}
	return happening{}, false
}

// add keeps an event that happened at now, as keep does.
func (r *recent) add(e status.Event, now time.Time) {
	r.keep(happening{Event: e}, now)
}

// keep keeps a happening at now as the newest, with the next id, and drops
// the oldest once more than maxEvents are kept. r.mu must be held.
func (r *recent) keep(h happening, now time.Time) {
	r.last++
	h.ID, h.At = r.last, now
	r.kept = append(r.kept, h)
	if len(r.kept) > maxEvents {
		r.kept = slices.Delete(r.kept, 0, len(r.kept)-maxEvents)
	}
}

// events returns the events kept, the newest first.
func (r *recent) events() []status.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var events []status.Event
	for _, h := range slices.Backward(r.kept) {
		events = append(events, h.event())
	}
	return events
}

// run looks at the accounts, as look does, at once, then every lookEvery and
// whenever it's nudged, until ctx ends.
func (r *recent) run(ctx context.Context) {
	ticker := time.NewTicker(lookEvery)
	defer ticker.Stop()
	for {
		r.look()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-r.nudge:
		}
	}
}

// look keeps what the accounts, as they stand now, call for since the last
// look, as news says, judged as the status document judges them, with the
// global pin's accounts spending their reserves.
func (r *recent) look() {
	now := r.now()
	accounts, _ := r.state.statuses(now, r.sessions.globalPin().Accounts)
	news := r.news(accounts, r.state.standings(now))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range news {
		r.add(e, now.UTC())
	}
}

// news returns the events the accounts, as they stand, call for since they
// were last looked at, and remembers how they stand: each that has come under
// pressure, as pressed says, and each that has room again, as the
// notifications judge it. The first look at an account calls for none, as
// there's nothing yet to compare it with.
func (r *recent) news(accounts []status.Account, standings standings) []status.Event {
	var news []status.Event
	for _, a := range accounts {
		if r.pressed(a) {
			p := a.Pressure
			news = append(news, status.Event{Kind: status.EventPressure, Account: a.ID, Windows: []string{p.Window}, Until: p.RunsOut, Since: p.Since})
		}
	}
	for _, s := range standings {
		if r.short.roomAgain(s) {
			news = append(news, status.Event{Kind: status.EventRoom, Account: s.ID})
		}
	}
	return news
}

// pressed reports whether the account has come under pressure since it was
// last looked at, which is news once a reset of its pressure window at most,
// so a rate hovering at the line can't fill the events kept. One under
// pressure as it's first looked at is no news, and none until that window
// resets.
func (r *recent) pressed(a status.Account) bool {
	was, seen := r.under[a.ID]
	r.under[a.ID] = a.Pressure.Under
	if !a.Pressure.Under || was {
		return false
	}
	w, _ := a.Window(a.Pressure.Window)
	if r.told[a.ID].Equal(w.ResetsAt) {
		return false
	}
	r.told[a.ID] = w.ResetsAt
	return seen
}

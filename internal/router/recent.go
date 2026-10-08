package router

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
)

// maxEvents is how many of its events the router keeps for its status
// document: the newest.
const maxEvents = 50

// recent keeps what has happened lately, for the router's status document:
// its newest maxEvents events, in memory alone, so a restart starts them
// afresh, each with an id one more than the event's before it, from 1 as the
// router starts. It's kept whatever the notifications are set to. Once it's
// given the events' files, it files each event as it's kept, and again, whole,
// as it changes, those kept before then filed as they then stand. It hears the
// router's events without ever holding the router up, and looks at the
// accounts every lookEvery, and after each event it hears, for what no event
// tells of: an account coming under pressure, reaching its cap, and having
// room again; and has the router's health judged as it looks, so a turn is
// told of within a look of it. It's safe for concurrent use.
type recent struct {
	state    *state
	sessions *sessions
	now      func() time.Time
	// judge judges the router's health, telling of a turn, as it looks at
	// the accounts. It mustn't be called with mu held, as what it tells of
	// is heard here.
	judge func()
	// nudge is nudged as an event is heard, for run to look at the accounts
	// again.
	nudge chan struct{}

	mu sync.Mutex
	// kept are the events kept, the oldest first, and last is the id of the
	// newest.
	kept []happening
	last int
	// files is where the events are filed, nil until fileTo gives it, as
	// told by the router that started at started.
	files   *events.Writer
	started time.Time
	// limits holds, by account, the identity of the newest of its limits
	// heard of, so the news of a limit heard of before, whose event is no
	// longer kept, is told from that of one yet to be.
	limits map[string]int
	// caps holds, by id, how each account read stood at its reserve as last
	// looked at.
	caps map[string]capped

	// Only run's goroutine touches what follows.
	short ranOut
	// under holds, by id, whether each account read was under pressure as
	// last looked at, and told when the window each was last told of as
	// under pressure in resets.
	under map[string]bool
	told  map[string]time.Time
}

func newRecent(state *state, sessions *sessions, now func() time.Time) *recent {
	return &recent{
		state:    state,
		sessions: sessions,
		now:      now,
		judge:    func() {},
		nudge:    make(chan struct{}, 1),
		limits:   make(map[string]int),
		caps:     make(map[string]capped),
		short:    make(ranOut),
		under:    make(map[string]bool),
		told:     make(map[string]time.Time),
	}
}

// happening is an event kept, as the status document gives it, with what it
// takes to change it as what it tells of does: for a limit's, a cap's or a
// refusal's, the sessions it has moved; for a refusal's, the requests refused
// that it has joined; and for a move, what forced it, while the event it's to
// be counted in is yet to be told. It's marked from when it's kept, or
// changes, until it's filed as it then stands.
type happening struct {
	status.Event
	moves    moves
	requests []refusedUntil
	waits    awaiting
	marked   bool
}

// mark marks the happening as changed, for it to be filed again once the
// change is done.
func (h *happening) mark() {
	h.marked = true
}

// changes reports whether the event can still change at now: it began no
// more than events.ChangesFor days before.
func (h *happening) changes(now time.Time) bool {
	return !now.After(h.At.AddDate(0, 0, events.ChangesFor))
}

// event is the happening as the status document gives it, a copy.
func (h happening) event() status.Event {
	e := h.Event
	e.Windows, e.Accounts = slices.Clone(e.Windows), slices.Clone(e.Accounts)
	return e
}

// hear keeps what one of the router's events tells of, as it happens, and
// nudges run to look at the accounts again, never waiting for it.
func (r *recent) hear(e Event) {
	now := r.now().UTC()
	r.change(func() { r.take(e, now) })
	select {
	case r.nudge <- struct{}{}:
	default:
	}
}

// take keeps what an event that happened at now tells of. r.mu must be held.
func (r *recent) take(e Event, now time.Time) {
	switch e := e.(type) {
	case SessionStarted:
		r.add(status.Event{Kind: status.EventStarted, Account: e.Account, Session: bounded(e.Session), Model: bounded(e.Model), Reason: e.Reason}, now)
	case LimitReached:
		r.reached(e, now)
	case Moved:
		r.moved(e, now)
	case Refused:
		r.refused(e, now)
	case RefusalLifted:
		r.lifted(e, now)
	case HealthChanged:
		r.add(status.Event{Kind: status.EventHealth, Reason: e.Reason}, now)
	case Primed:
		r.add(status.Event{Kind: status.EventPrimed, Account: e.Account, Windows: []string{e.Window}, Until: e.ResetsAt}, now)
	case RestartDue:
		r.add(status.Event{Kind: status.EventRestart, Reason: e.Reason}, now)
	case Pinned:
		r.add(status.Event{Kind: status.EventPin, Accounts: e.Accounts, Account: e.Account, Session: bounded(e.Session), Move: e.Move, Force: e.Force, By: string(e.By)}, now)
	case Unpinned:
		r.add(status.Event{Kind: status.EventAuto, Accounts: e.Accounts, Account: e.Account, Session: bounded(e.Session), Force: e.Force, By: string(e.By)}, now)
	}
}

// add keeps an event that happened at now, as keep does.
func (r *recent) add(e status.Event, now time.Time) {
	r.keep(happening{Event: e}, now)
}

// keep keeps a happening at now as the newest, with the next id, marked to
// be filed, and drops the oldest once more than maxEvents are kept,
// returning the happening as it's kept. r.mu must be held.
func (r *recent) keep(h happening, now time.Time) *happening {
	r.last++
	h.ID, h.At = r.last, now
	h.mark()
	r.kept = append(r.kept, h)
	if len(r.kept) > maxEvents {
		r.kept = slices.Delete(r.kept, 0, len(r.kept)-maxEvents)
	}
	return &r.kept[len(r.kept)-1]
}

// find returns the newest event kept that is reports true of, reporting false
// when there's none. r.mu must be held.
func (r *recent) find(is func(h *happening) bool) (*happening, bool) {
	for i := range slices.Backward(r.kept) {
		if h := &r.kept[i]; is(h) {
			return h, true
		}
	}
	return nil, false
}

// byID returns the event kept with the given id, reporting false when it's
// no longer kept, or never was. r.mu must be held.
func (r *recent) byID(id int) (*happening, bool) {
	if len(r.kept) == 0 || id < r.kept[0].ID || id > r.last {
		return nil, false
	}
	return &r.kept[id-r.kept[0].ID], true
}

// change makes a change to the events kept, with r.mu held, then files each
// event the change marked, as it then stands, so an event's versions are
// filed in the order they're made.
func (r *recent) change(do func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	do()
	r.fileMarked()
}

// fileTo has the events filed to files from now on, as told by the router
// that started at started, filing at once those kept before, as they stand.
func (r *recent) fileTo(files *events.Writer, started time.Time) {
	r.change(func() { r.files, r.started = files, started.UTC() })
}

// fileMarked files each event kept that's marked, as it stands, and clears
// its mark, once there are files to file them to. r.mu must be held.
func (r *recent) fileMarked() {
	if r.files == nil {
		return
	}
	for i := range r.kept {
		if h := &r.kept[i]; h.marked {
			r.files.Note(events.Line{Event: h.event(), Run: r.started})
			h.marked = false
		}
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

// newest returns the id of the newest event kept, 0 before any is.
func (r *recent) newest() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
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
// look, as news and capping say, judged as the status document judges them,
// with the global pin's accounts spending their reserves; and has the
// router's health judged now, which tells of a turn as it's judged. The moves
// heard before it reads the accounts, still waiting for the cap or the
// refusal that forced them, wait no more, as unawaited says.
func (r *recent) look() {
	r.judge()
	now := r.now()
	pin := r.sessions.globalPin()
	heard := r.newest()
	accounts, _ := r.state.statuses(now, pin.Accounts)
	news := r.news(accounts, r.state.standings(now))
	r.change(func() {
		for _, e := range news {
			r.add(e, now.UTC())
		}
		r.capping(accounts, pin, now.UTC())
		r.unawaited(heard)
	})
}

// news returns the events the accounts, as they stand, call for since they
// were last looked at, and remembers how they stand: each that has come under
// pressure, as pressed says, and each that has room again, as the
// notifications judge it, with the windows that held it back. The first look
// at an account calls for none, as there's nothing yet to compare it with.
func (r *recent) news(accounts []status.Account, standings standings) []status.Event {
	var news []status.Event
	for _, a := range accounts {
		if r.pressed(a) {
			p := a.Pressure
			news = append(news, status.Event{Kind: status.EventPressure, Account: a.ID, Windows: []string{p.Window}, Until: p.RunsOut, Since: p.Since})
		}
	}
	for _, s := range standings {
		if windows, again := r.short.roomAgain(s); again {
			news = append(news, status.Event{Kind: status.EventRoom, Account: s.ID, Windows: windows})
		}
	}
	return news
}

// pressed reports whether the account has come under pressure since it was
// last looked at, which is news once a reset of its pressure window at most,
// so a rate hovering at the line can't fill the events kept. An account is
// first seen once it's read, as room news has it: one under pressure as it's
// first seen is no news, nor is it until that window resets.
func (r *recent) pressed(a status.Account) bool {
	if len(a.Windows) == 0 {
		delete(r.under, a.ID)
		return false
	}
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

package capture

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// source is the router as a fixture has it, at the moment the fixture draws:
// its status document, and its health check's answer, which says which
// router it is; the sessions it has routed lately, as GET /sessions lists
// them; its accounts' use of their windows over time, which GET /history
// answers from; and what its request stream tells, as GET /stream does. It's
// the watch's Source, every read giving the document, and an order a key
// gives changing nothing. Where it's gone, it stops answering then, by the
// fixture's clock: a read fails, mid-read, from then on. Read by a watch, it
// knows whether that watch has opened its stream before, and whether what
// the stream it opened last told is yet to be heard. Where it turns, it
// stands as another router does from a time on, as a fixture whose keys
// before its moment look at the router as it stood then has it.
type source struct {
	doc      status.Document
	health   router.Health
	sessions []status.Session
	trails   []trail
	stream   []router.StreamEvent
	now      time.Time
	gone     time.Time
	clock    func() time.Time
	joined   *atomic.Bool
	untold   *atomic.Bool
	turns    *turn
}

// turn is a source's turning to stand as next does, from at on.
type turn struct {
	at   time.Time
	next source
}

// errGone is a read of the router once it has gone: it took the read, and
// stopped answering before it gave the document.
var errGone = errors.New("the router stopped answering mid-read")

// routerPID is the process the fixtures' router runs as.
const routerPID = 41207

// routerOf is the router of the world w at now: its status document and its
// health check's answer, started as the run its events are of did, which
// are all usage reads of it; the history of its accounts' windows, from
// their readings; what its request stream tells; and the sessions it lists,
// where the world has them.
func routerOf(w accountWorld, now time.Time) source {
	doc := w.document(now)
	s := source{doc: doc, health: startedAt(routerRun(now)), trails: w.trails(doc, now), stream: w.stream, now: now}
	if w.routed != nil {
		s.sessions = w.routed(now)
	}
	return s
}

// then is the source standing as it does until at, and as next does from
// then on.
func (s source) then(at time.Time, next source) source {
	s.turns = &turn{at: at, next: next}
	return s
}

// standing is the source as it stands by its clock: as it turned, once it
// has, read by the same clock, what its stream told yet to be heard the
// same, but its stream opened afresh, so what it tells as it's first opened
// it tells whole.
func (s source) standing() source {
	if s.turns == nil || s.clock == nil || s.clock().Before(s.turns.at) {
		return s
	}
	next := s.turns.next
	next.clock, next.untold = s.clock, s.untold
	return next
}

// goneAt is the source, stopping answering at the time given.
func (s source) goneAt(t time.Time) source {
	s.gone = t
	return s
}

// by is the source as a watch reads it, by the clock given, which says
// whether it's gone: its stream not yet opened.
func (s source) by(clock func() time.Time) source {
	s.clock, s.joined, s.untold = clock, new(atomic.Bool), new(atomic.Bool)
	if s.turns != nil {
		turned := *s.turns
		turned.next.joined = new(atomic.Bool)
		s.turns = &turned
	}
	return s
}

// isGone reports whether the router has stopped answering, by the clock.
func (s source) isGone() bool {
	return !s.gone.IsZero() && !s.clock().Before(s.gone)
}

// startedAt is the health check's answer of the fixtures' router, started at
// the time given: healthy.
func startedAt(t time.Time) router.Health {
	return router.Health{OK: true, PID: routerPID, StartedAt: t}
}

// Read reads the router's document, whatever the read asks, with what's
// worked out of it, as the router gives it; once it's gone, it fails.
func (s source) Read(context.Context, status.Read) (status.Document, router.Health, error) {
	s = s.standing()
	if s.isGone() {
		return status.Document{}, router.Health{}, errGone
	}
	return s.doc.WorkedOut(claude.Policy()), s.health, nil
}

// Pin takes the order, and changes nothing.
func (s source) Pin(context.Context, []string, bool) error {
	return nil
}

// Unpin takes the order, and changes nothing.
func (s source) Unpin(context.Context) error {
	return nil
}

// PinSession takes the order, and changes nothing.
func (s source) PinSession(context.Context, string, string) error {
	return nil
}

// UnpinSession takes the order, and changes nothing.
func (s source) UnpinSession(context.Context, string) error {
	return nil
}

// RouterAnswers reports whether the document is the router's, and the router
// isn't gone.
func (s source) RouterAnswers(context.Context) bool {
	s = s.standing()
	return s.doc.Source == status.SourceRouter && !s.isGone()
}

// Sessions lists the sessions the router has routed lately, as GET /sessions
// does.
func (s source) Sessions(context.Context) ([]status.Session, error) {
	s = s.standing()
	return slices.Clone(s.sessions), nil
}

// History answers as GET /history does: every account's use of the window
// with the given key over its current length, a point each step from when it
// started; or, for an account whose window isn't running, no points.
func (s source) History(_ context.Context, window string, step time.Duration) (router.History, error) {
	if step <= 0 {
		return router.History{}, fmt.Errorf("step %s isn't more than 0", step)
	}
	s = s.standing()
	h := router.History{Window: window, Step: step.String()}
	for _, a := range s.doc.Accounts {
		held := router.AccountHistory{ID: a.ID}
		if i := slices.IndexFunc(s.trails, func(t trail) bool { return t.account == a.ID && t.window == window }); i >= 0 {
			held = s.trails[i].history(step, s.now)
		}
		h.Accounts = append(h.Accounts, held)
	}
	return h, nil
}

// Stream opens the router's request stream, as GET /stream does, and holds
// it open, telling nothing more, as a router does while nothing befalls its
// requests: the first time the watch reading it opens it, telling of the
// requests the fixture has in flight, and all that has befallen them; each
// time after, only of those still in flight then, by the clock, each as it
// stands, as a router tells a reader joining it, so what it told before, a
// move among it, isn't told again. A source no watch reads tells all of it
// each time.
func (s source) Stream(context.Context) (<-chan router.StreamEvent, error) {
	s = s.standing()
	told := s.stream
	if s.joined != nil && s.joined.Swap(true) {
		told = flying(s.stream, s.clock())
	}
	events := make(chan router.StreamEvent, len(told))
	for _, e := range told {
		events <- e
	}
	if s.untold != nil {
		s.untold.Store(len(told) > 0)
	}
	return events, nil
}

// await has the watch reading the source wait on the stream it opened last
// while what that told is yet to be heard, which the wait takes in at once,
// and holds the wait after, as the stream, open, tells nothing more: so the
// watch hears all it told, and listens on, never blocking the capture.
func (s source) await(wait tea.Cmd) tea.Cmd {
	if s.untold == nil || !s.untold.Swap(false) {
		return nil
	}
	return wait
}

// telling is the source with its request stream telling of the events
// given, in turn.
func (s source) telling(events ...router.StreamEvent) source {
	s.stream = events
	return s
}

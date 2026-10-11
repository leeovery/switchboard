package capture

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

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
// fixture's clock: a read fails, mid-read, from then on.
type source struct {
	doc      status.Document
	health   router.Health
	sessions []status.Session
	trails   []trail
	stream   []router.StreamEvent
	now      time.Time
	gone     time.Time
	clock    func() time.Time
}

// errGone is a read of the router once it has gone: it took the read, and
// stopped answering before it gave the document.
var errGone = errors.New("the router stopped answering mid-read")

// routerPID is the process the fixtures' router runs as.
const routerPID = 41207

// newSource is the router at now, started at noon, having read the samples
// readAgo before: healthy, routing every session on its merits, new sessions
// going to best, and telling of events, the newest first; the sessions on the
// samples; and the history of their windows.
func newSource(now time.Time, samples []sample, best string, events ...status.Event) source {
	sessions := sessionsOn(samples)
	doc := status.Document{
		GeneratedAt: ago(now, readAgo),
		Source:      status.SourceRouter,
		Best:        best,
		Prime:       priming(samples),
		Router:      status.Health{Healthy: true},
		Sessions:    len(sessions),
		Events:      numbered(events),
	}
	for _, s := range samples {
		doc.Accounts = append(doc.Accounts, s.account(now))
	}
	doc.Primary = status.PrimaryOf(doc.Accounts)
	return source{doc: doc, health: startedAtNoon(now), sessions: sessions, trails: trailsOf(samples, now), now: now}
}

// routerOf is the router of the world w at now: its status document and its
// health check's answer, started at noon, which are all usage reads of it;
// and the sessions it lists, where the world has them. It keeps no history
// and tells nothing on its stream.
func routerOf(w accountWorld, now time.Time) source {
	s := source{doc: w.document(now), health: startedAtNoon(now), now: now}
	if w.routed != nil {
		s.sessions = w.routed(now)
	}
	return s
}

// goneAt is the source, stopping answering at the time given.
func (s source) goneAt(t time.Time) source {
	s.gone = t
	return s
}

// by is the source, read by the clock given, which says whether it's gone.
func (s source) by(clock func() time.Time) source {
	s.clock = clock
	return s
}

// isGone reports whether the router has stopped answering, by the clock.
func (s source) isGone() bool {
	return !s.gone.IsZero() && !s.clock().Before(s.gone)
}

// startedAtNoon is the health check's answer of the fixtures' router, started
// at noon on the frames' day, in now's time zone: healthy.
func startedAtNoon(now time.Time) router.Health {
	return router.Health{OK: true, PID: routerPID, StartedAt: on(now, 1, 12, 0)}
}

// Read reads the router's document, whatever the read asks, with what's
// worked out of it, as the router gives it; once it's gone, it fails.
func (s source) Read(context.Context, status.Read) (status.Document, router.Health, error) {
	if s.isGone() {
		return status.Document{}, router.Health{}, errGone
	}
	return s.doc.WorkedOut(claude.Policy), s.health, nil
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
	return s.doc.Source == status.SourceRouter && !s.isGone()
}

// Sessions lists the sessions the router has routed lately, as GET /sessions
// does.
func (s source) Sessions(context.Context) ([]status.Session, error) {
	return slices.Clone(s.sessions), nil
}

// History answers as GET /history does: every account's use of the window
// with the given key over its current length, a point each step from when it
// started; or, for an account whose window isn't running, no points.
func (s source) History(_ context.Context, window string, step time.Duration) (router.History, error) {
	if step <= 0 {
		return router.History{}, fmt.Errorf("step %s isn't more than 0", step)
	}
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

// Stream opens the router's request stream, as GET /stream does: it tells of
// the requests the fixture has in flight, and what has befallen them, then
// ends, as a router ends it. The watch keeps what it told until it asks for
// it again, which a capture never has it do, as its timers never fire.
func (s source) Stream(context.Context) (<-chan router.StreamEvent, error) {
	events := make(chan router.StreamEvent, len(s.stream))
	for _, e := range s.stream {
		events <- e
	}
	close(events)
	return events, nil
}

// telling is the source with its request stream telling of the events
// given, in turn.
func (s source) telling(events ...router.StreamEvent) source {
	s.stream = events
	return s
}

// forced marks a move among the events a source is given as one a limit
// forced, which numbered counts in that limit.
const forced = -1

// numbered is events, the newest first, each with its id, as the router
// numbers them: rising by one an event. A move forced is counted in the
// newest limit, no newer than it, of the account it left, as the router
// counts it, naming that limit's event.
func numbered(events []status.Event) []status.Event {
	events = slices.Clone(events)
	for i := range events {
		events[i].ID = len(events) - i
	}
	for i, e := range events {
		if e.Limit != forced {
			continue
		}
		events[i].Limit = 0
		if j := slices.IndexFunc(events[i:], func(l status.Event) bool { return l.Kind == status.EventLimit && l.Account == e.From }); j >= 0 {
			events[i].Limit = events[i+j].ID
		}
	}
	return events
}

// priming is the router's priming schedule over the samples: their resets
// spread over 08:00-22:00, each primed daily at the time of day it's next
// primed, in the order they fall.
func priming(samples []sample) status.Prime {
	p := status.Prime{Day: "08:00-22:00", Window: fiveHourKey}
	for _, s := range samples {
		p.Slots = append(p.Slots, status.Slot{Account: s.id, At: s.prime.Format("15:04"), Next: s.prime})
	}
	slices.SortStableFunc(p.Slots, func(a, b status.Slot) int { return cmp.Compare(a.At, b.At) })
	return p
}

// sessionsOn are the sessions on the samples, as GET /sessions lists them:
// the one seen last first, each with the account each of its models goes to,
// the one used last first, and its own pin, where it has one.
func sessionsOn(samples []sample) []status.Session {
	var listed []status.Session
	for _, s := range samples {
		for _, seat := range s.seats {
			i := slices.IndexFunc(listed, func(l status.Session) bool { return l.ID == seat.session })
			if i < 0 {
				listed = append(listed, status.Session{ID: seat.session})
				i = len(listed) - 1
			}
			if seat.pinned {
				listed[i].Pin = s.id
			}
			listed[i].Assignments = append(listed[i].Assignments, seat.assignment(s.id))
		}
	}
	for _, l := range listed {
		slices.SortStableFunc(l.Assignments, seenLater)
	}
	slices.SortStableFunc(listed, func(a, b status.Session) int { return seenLater(a.Assignments[0], b.Assignments[0]) })
	return listed
}

// assignment is the seat as the router assigns it, to the account with the
// given id.
func (s seat) assignment(account string) status.Assignment {
	return status.Assignment{
		Model: s.model, Family: claude.Provider{}.Family(s.model), Account: account,
		Pinned: s.pinned, PinnedAt: s.pinnedAt, Reason: cmp.Or(s.reason, status.ReasonNew), AssignedAt: s.assigned, LastSeen: s.seen,
	}
}

// seenLater orders assignments the one seen last first.
func seenLater(a, b status.Assignment) int {
	return b.LastSeen.Compare(a.LastSeen)
}

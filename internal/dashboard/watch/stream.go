package watch

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// rejoinAfter is how long after the request stream ends, or fails to open,
// the watch asks for it again, doubled for each time in a row it has failed
// to open, up to rejoinMost.
const (
	rejoinAfter = time.Second
	rejoinMost  = 30 * time.Second
)

// joinFor is how long the watch waits, once the request stream has opened
// again, for its first look to tell of the requests in flight as it joined:
// the router tells of them as the stream opens, so where there are any they
// come at once, and where none has come in joinFor, none was in flight.
const joinFor = time.Second

// streaming is the watch's hold on the router's request stream: gen counts
// the times it has opened it, or closed it, so what an earlier time sends is
// dropped; cancel ends the one open or opening; events are what the one open
// tells; whether one is open, or opening, or due to be asked for again; how
// many times in a row it has failed to open; whether the router, the one
// whose health check said absentOf, is from before the stream, so has none;
// and how far today's lines are read, as it opened, for the traces it goes
// on from.
type streaming struct {
	gen                      int
	cancel                   context.CancelFunc
	events                   <-chan router.StreamEvent
	open, opening, rejoining bool
	fails                    int
	absent                   bool
	absentOf                 router.Health
	read                     ledger.Mark
}

// streamOpenedMsg is the request stream, opened, with what it tells, and
// today's lines read for the traces once it had, or why it couldn't be.
type streamOpenedMsg struct {
	gen    int
	events <-chan router.StreamEvent
	traced tracedLines
	err    error
}

// streamHeardMsg is what the request stream told, the first event that came
// and those already come behind it, and whether it ended after them.
type streamHeardMsg struct {
	gen    int
	events []router.StreamEvent
	ended  bool
}

// rejoinMsg is the request stream falling due to be asked for again.
type rejoinMsg struct {
	gen int
}

// joinedMsg is the request stream, opened again, joinFor on, when it has
// told of the requests in flight as the watch joined it, if any were.
type joinedMsg struct {
	gen int
}

// wantsStream reports whether the watch reads the router's request stream:
// while a session's page shows, Sessions, or the Overview with ROUTING or
// SESSIONS shown, whose traces, and what each session is doing, the stream
// tells; and the router answers, unless it's from before the stream.
func (m Model) wantsStream() bool {
	overview := m.view == dashboard.Overview && (m.sections.Has(dashboard.SectionRouting) || m.sections.Has(dashboard.SectionSessions))
	return (m.session != nil || m.view == dashboard.Sessions || overview) && m.answering() && !m.stream.absent
}

// tune opens the request stream when the watch wants it and has none open,
// opening or due to be asked for again; and closes it when the watch no
// longer wants it.
func (m Model) tune() (Model, tea.Cmd) {
	held := m.stream.open || m.stream.opening || m.stream.rejoining
	switch wanted := m.wantsStream(); {
	case wanted && !held:
		return m.openStream()
	case !wanted && held:
		return m.closeStream(), nil
	}
	return m, nil
}

// openStream opens the router's request stream, under its own context, and
// once it has, reads the requests today's lines tell of for the traces, as
// readTraces does, for the stream to go on from.
func (m Model) openStream() (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	m.stream.gen++
	m.stream.cancel, m.stream.opening = cancel, true
	gen, source, trace := m.stream.gen, m.cfg.Source, m.readTraces()
	open := func() tea.Msg {
		events, err := source.Stream(ctx)
		msg := streamOpenedMsg{gen: gen, events: events, err: err}
		if err == nil {
			msg.traced = trace()
		}
		return msg
	}
	return m, open
}

// closeStream closes the request stream, its traffic as closed has it: no
// longer live, and what each seat is doing forgotten, but keeping the
// requests it told of for the stream's first look to settle as it opens
// again; and how far the traces have read today's lines, for the stream to
// go on from then.
func (m Model) closeStream() Model {
	m.stream.end()
	m.stream = streaming{gen: m.stream.gen + 1, absent: m.stream.absent, absentOf: m.stream.absentOf, read: m.stream.read}
	m.traffic = m.traffic.closed()
	return m
}

// end ends the stream open or opening, where there's one.
func (s streaming) end() {
	if s.cancel != nil {
		s.cancel()
	}
}

// opened takes in the request stream opening, or why it couldn't. Open, its
// traffic is as reopened has it, what it tells as it opens telling of the
// requests in flight afresh, its first look to settle those it told of in
// flight before, by joinFor at the latest; and the watch counts the requests
// today's lines told of for the traces, and listens to it, what it draws
// live. Otherwise the watch forgets what it told before of the requests, as
// afresh has it, their router gone. From a router from before it, the watch
// goes without it until another router answers; else it asks for it again,
// later each time it fails in a row.
func (m Model) opened(msg streamOpenedMsg) (Model, tea.Cmd) {
	if msg.gen != m.stream.gen {
		return m, nil
	}
	m.stream.opening = false
	switch {
	case msg.err == nil:
		m.stream.open, m.stream.events, m.stream.fails = true, msg.events, 0
		m.traffic = m.traffic.reopened()
		m = m.traced(msg.traced)
		logger.Debug("reading the router's request stream")
		return m, tea.Batch(m.listen(), m.awaitJoin())
	case errors.Is(msg.err, router.ErrNoStream):
		m.traffic = m.traffic.afresh()
		m.stream.end()
		m.stream.cancel, m.stream.absent, m.stream.absentOf = nil, true, m.news.router
		logger.Info("the router is from before GET /stream: the sessions are drawn as each look reads them")
		return m, nil
	default:
		m.traffic = m.traffic.afresh()
		m.stream.end()
		m.stream.fails++
		logger.Debug("couldn't open the router's request stream", "error", msg.err, "fails", m.stream.fails)
		return m.rejoinIn(rejoinWait(m.stream.fails))
	}
}

// awaitJoin has the stream, opened again, taken to have told all it will of
// the requests in flight as the watch joined it once joinFor has passed,
// where any it told of in flight before await it.
func (m Model) awaitJoin() tea.Cmd {
	if m.traffic.awaiting == nil {
		return nil
	}
	return m.after(joinFor, joinedMsg{gen: m.stream.gen})
}

// joinedBy takes in joinFor passing since the stream opened again: open
// still, it has told all it will of the requests in flight as the watch
// joined it, so those it told of in flight before that it didn't tell of
// again are lost, as settled has it.
func (m Model) joinedBy(msg joinedMsg) Model {
	if msg.gen == m.stream.gen && m.stream.open {
		m.traffic = m.traffic.settled()
	}
	return m
}

// listen waits for what the open request stream tells next, as Await has
// the wait run.
func (m Model) listen() tea.Cmd {
	gen, events := m.stream.gen, m.stream.events
	wait := func() tea.Msg { return hear(gen, events) }
	if m.cfg.Await != nil {
		return m.cfg.Await(wait)
	}
	return wait
}

// hear waits for the next event the stream given tells, and returns it with
// those already come behind it, or that the stream ended.
func hear(gen int, events <-chan router.StreamEvent) streamHeardMsg {
	e, ok := <-events
	if !ok {
		return streamHeardMsg{gen: gen, ended: true}
	}
	heard := streamHeardMsg{gen: gen, events: []router.StreamEvent{e}}
	for {
		select {
		case e, ok := <-events:
			if !ok {
				heard.ended = true
				return heard
			}
			heard.events = append(heard.events, e)
		default:
			return heard
		}
	}
}

// heard takes in what the request stream told, the requests it told of going
// upstream counted for the traces, each number on the page it changed
// brightening, and listens on; or, where it ended, as a router ends it as it
// restarts, asks for it again after rejoinAfter, its traffic no longer live
// meanwhile, keeping what it told until it opens again, as unheard has it.
func (m Model) heard(msg streamHeardMsg) (Model, tea.Cmd) {
	if msg.gen != m.stream.gen || !m.stream.open {
		return m, nil
	}
	m = m.told(msg.events, m.now())
	if !msg.ended {
		return m, m.listen()
	}
	logger.Debug("the router's request stream ended: asking for it again")
	m.stream.end()
	m.stream.open, m.stream.events = false, nil
	m.traffic = m.traffic.unheard()
	return m.rejoinIn(rejoinAfter)
}

// told takes in the events the stream told, at now, as took does: those of
// the requests in flight as the watch joined it, as joined counts them, as
// part of the join's first look, brightening nothing; then the rest, keeping
// what the stream had told before them where they can change a number the
// page draws, as numbering says, for compared to brighten each they change
// once the message is taken in.
func (m Model) told(events []router.StreamEvent, now time.Time) Model {
	if n := joined(events); n > 0 {
		m, events = m.took(events[:n], now), events[n:]
	}
	if m.numbering(events) {
		m.toldBefore = &toldSoFar{traffic: m.traffic, traces: m.traces}
	}
	return m.took(events, now)
}

// toldSoFar is what the request stream has told, as the watch keeps it: what
// each seat is doing, and the requests the traces count.
type toldSoFar struct {
	traffic traffic
	traces  views.Traces
}

// joined is how many of the events lead them telling of the requests in
// flight as the watch joined the stream, which opens with one for each.
func joined(events []router.StreamEvent) int {
	if n := slices.IndexFunc(events, func(e router.StreamEvent) bool { return e.Kind != router.StreamInFlight }); n >= 0 {
		return n
	}
	return len(events)
}

// took takes in the events the stream told, at now: each request told of,
// and what each seat is doing, for the traffic, and the requests going
// upstream, for the traces.
func (m Model) took(events []router.StreamEvent, now time.Time) Model {
	m.traffic = m.traffic.took(events, m.held(now), now)
	m.traces = m.traces.Told(m.traffic.sentOf(events), now)
	return m
}

// numbering reports whether the events can change a number the page shown
// draws: where one sends a request, which the traces count, or moves
// a seat, which re-patches the sessions each account counts, and the page is
// one that draws what they change, as StreamNumbered says. An answer's
// progress, its end and the rest change none, as the dashboard's tests hold
// the traffic to changing no number but by its moves.
func (m Model) numbering(events []router.StreamEvent) bool {
	return slices.ContainsFunc(events, func(e router.StreamEvent) bool {
		_, sent := sentAt(e)
		return sent || e.Kind == router.StreamMoved
	}) && dashboard.Frame{View: m.view, Sections: m.sections, Session: m.session.frame()}.StreamNumbered()
}

// rejoinIn has the request stream asked for again once wait has passed.
func (m Model) rejoinIn(wait time.Duration) (Model, tea.Cmd) {
	m.stream.rejoining, m.stream.cancel = true, nil
	return m, m.after(wait, rejoinMsg{gen: m.stream.gen})
}

// rejoined takes in the request stream falling due to be asked for again,
// which tune then does, where the watch still wants it.
func (m Model) rejoined(msg rejoinMsg) Model {
	if msg.gen == m.stream.gen {
		m.stream.rejoining = false
	}
	return m
}

// rejoinWait is how long to wait to ask for the request stream again once
// it has failed to open fails times in a row: rejoinAfter, doubled for each
// failure before, up to rejoinMost.
func rejoinWait(fails int) time.Duration {
	wait := rejoinAfter
	for i := 1; i < fails && wait < rejoinMost; i++ {
		wait *= 2
	}
	return min(wait, rejoinMost)
}

// answeredAgain takes in a look finding the router whose health check said
// from: where it's another router than the one before, as one restarted is,
// its stream is asked for at once, whether the last one's was due to be
// asked for again, having failed to open, or ended, what it told of the
// requests forgotten, as they went with its router; or gone without, the
// last router being from before it.
func (m Model) answeredAgain(from router.Health) Model {
	switch {
	case m.stream.absent && !from.Same(m.stream.absentOf):
		m.stream.absent = false
	case m.stream.rejoining && m.news.another(from):
		m.stream.rejoining, m.stream.fails = false, 0
		m.stream.gen++
		m.traffic = m.traffic.afresh()
	}
	return m
}

// held reports whether the limit the router saw the account with the given id
// reach holds it at now, as the moment of the document on screen then has it,
// as moment says, asked for the first time it's asked.
func (m Model) held(now time.Time) func(id string) bool {
	look := sync.OnceValue(func() status.Moment { return m.moment(now) })
	return func(id string) bool {
		a, ok := look().Account(id)
		return ok && a.LimitHolds
	}
}

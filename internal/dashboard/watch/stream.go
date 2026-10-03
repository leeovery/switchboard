package watch

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
)

// rejoinAfter is how long after the request stream ends, or fails to open,
// the watch asks for it again, doubled for each time in a row it has failed
// to open, up to rejoinMost.
const (
	rejoinAfter = time.Second
	rejoinMost  = 30 * time.Second
)

// streaming is the watch's hold on the router's request stream: gen counts
// the times it has opened it, or closed it, so what an earlier time sends is
// dropped; cancel ends the one open or opening; events are what the one open
// tells; whether one is open, or opening, or due to be asked for again; how
// many times in a row it has failed to open; and whether the router, the one
// whose health check said absentOf, is from before the stream, so has none.
type streaming struct {
	gen                      int
	cancel                   context.CancelFunc
	events                   <-chan router.StreamEvent
	open, opening, rejoining bool
	fails                    int
	absent                   bool
	absentOf                 router.Health
}

// streamOpenedMsg is the request stream, opened, with what it tells, or why
// it couldn't be.
type streamOpenedMsg struct {
	gen    int
	events <-chan router.StreamEvent
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

// wantsStream reports whether the watch reads the router's request stream:
// while Sessions shows, or the Accounts view with a card flipped, and the
// router answers, unless it's from before the stream.
func (m Model) wantsStream() bool {
	shows := m.view == dashboard.Sessions || (m.view == dashboard.Accounts && len(m.flipped) > 0)
	return shows && m.answering() && !m.stream.absent
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

// openStream opens the router's request stream, under its own context.
func (m Model) openStream() (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	m.stream.gen++
	m.stream.cancel, m.stream.opening = cancel, true
	gen, source := m.stream.gen, m.cfg.Source
	return m, func() tea.Msg {
		events, err := source.Stream(ctx)
		return streamOpenedMsg{gen: gen, events: events, err: err}
	}
}

// closeStream closes the request stream, forgetting what it told of the
// requests, which is past telling once the watch no longer hears it.
func (m Model) closeStream() Model {
	m.stream.end()
	m.stream = streaming{gen: m.stream.gen + 1, absent: m.stream.absent, absentOf: m.stream.absentOf}
	m.traffic = m.traffic.afresh()
	return m
}

// end ends the stream open or opening, where there's one.
func (s streaming) end() {
	if s.cancel != nil {
		s.cancel()
	}
}

// opened takes in the request stream opening, or why it couldn't, forgetting
// what it told before of the requests: what it tells as it opens tells of
// those in flight afresh. Open, the watch listens to it, what it draws live.
// From a router from before it, the watch goes without it until another
// router answers. Otherwise it asks for it again, later each time it fails
// in a row.
func (m Model) opened(msg streamOpenedMsg) (Model, tea.Cmd) {
	if msg.gen != m.stream.gen {
		return m, nil
	}
	m.stream.opening = false
	m.traffic = m.traffic.afresh()
	switch {
	case msg.err == nil:
		m.stream.open, m.stream.events, m.stream.fails = true, msg.events, 0
		m.traffic.live = true
		logger.Debug("reading the router's request stream")
		return m, m.listen()
	case errors.Is(msg.err, router.ErrNoStream):
		m.stream.end()
		m.stream.cancel, m.stream.absent, m.stream.absentOf = nil, true, m.news.router
		logger.Info("the router is from before GET /stream: the cords and the cards' backs draw the sessions as each look reads them")
		return m, nil
	default:
		m.stream.end()
		m.stream.fails++
		logger.Debug("couldn't open the router's request stream", "error", msg.err, "fails", m.stream.fails)
		return m.rejoinIn(rejoinWait(m.stream.fails))
	}
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

// heard takes in what the request stream told, and listens on; or, where it
// ended, as a router ends it as it restarts, asks for it again after
// rejoinAfter, keeping what it told until it opens again.
func (m Model) heard(msg streamHeardMsg) (Model, tea.Cmd) {
	if msg.gen != m.stream.gen || !m.stream.open {
		return m, nil
	}
	m.traffic = m.traffic.took(msg.events, m.held, m.now())
	if !msg.ended {
		return m, m.listen()
	}
	logger.Debug("the router's request stream ended: asking for it again")
	m.stream.end()
	m.stream.open, m.stream.events = false, nil
	return m.rejoinIn(rejoinAfter)
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
// asked for again, having failed to open, or gone without, the last router
// being from before it.
func (m Model) answeredAgain(from router.Health) Model {
	switch {
	case m.stream.absent && !from.Same(m.stream.absentOf):
		m.stream.absent = false
	case m.stream.rejoining && m.news.another(from):
		m.stream.rejoining, m.stream.fails = false, 0
		m.stream.gen++
	}
	return m
}

// held reports whether a limit holds the account with the given id back, as
// the document on screen has it.
func (m Model) held(id string) bool {
	a, ok := m.doc.Account(id)
	return ok && a.Limit.Holds(m.now())
}

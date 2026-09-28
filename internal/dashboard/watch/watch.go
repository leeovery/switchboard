// Package watch keeps the dashboard on screen. It reads the status document as
// each read falls due, redraws as the clock moves, eases each bar to its new
// reading, and posts a desktop notification when an account has room again or
// a window passes 90%. The model does no I/O of its own: it's handed its
// source, its clock and its notifier, so tests drive it as a terminal would.
package watch

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// Source reads the status document the dashboard shows.
type Source interface {
	Fetch(ctx context.Context) (status.Document, error)
}

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(message string) error
}

// Config is what a watch is given.
type Config struct {
	Source   Source
	Notifier Notifier
	// Now reads the wall clock.
	Now func() time.Time
	// Interval is the longest the dashboard goes between reads.
	Interval time.Duration
	// Policy is the provider's say in which windows leave an account without
	// room.
	Policy score.Policy
}

const (
	// keys says what the keys do, at the end of the footer.
	keys = "r refresh · q quit"
	// topMargin is the blank lines above the frame.
	topMargin = 1
)

// Model is a watch's state, as Bubble Tea runs it. Build one with New.
type Model struct {
	ctx context.Context
	cfg Config
	// after delivers msg once d has passed. Tests replace it, to hold each
	// timer until they fire it.
	after func(d time.Duration, msg tea.Msg) tea.Cmd

	width, height int

	doc status.Document
	// updated is when doc arrived: zero until one has.
	updated time.Time
	// next is when the next read is due.
	next     time.Time
	fetching bool
	// failed says why the last read failed, if it did.
	failed string
	// failures counts the reads that have failed in a row, in whole or in
	// part.
	failures int

	// chain numbers the live chain of ticks: a tick from an earlier one is
	// dropped.
	chain int
	// ease moves each bar to doc's reading, and framing is set while frames
	// run to draw it.
	ease     easing
	framing  bool
	readings readings
}

// fetchedMsg is what a read found.
type fetchedMsg struct {
	doc status.Document
	err error
}

// tickMsg wakes the model to redraw, and to read again once that's due.
type tickMsg struct {
	chain int
}

// frameMsg draws the next frame of the bars easing.
type frameMsg struct{}

// New returns a model that reads cfg's source at once, and whenever a read
// falls due after, under ctx.
func New(ctx context.Context, cfg Config) Model {
	return Model{ctx: ctx, cfg: cfg, after: after, fetching: true}
}

// after delivers msg once d has passed.
func after(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// Init starts the first read and the ticks.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetch(), m.armTick(m.now()))
}

// Update takes in a message: a key, a resize, a read, a tick or a frame. The
// view is drawn again after each.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		return m.pressed(msg)
	case fetchedMsg:
		return m.fetched(msg)
	case tickMsg:
		return m.ticked(msg)
	case frameMsg:
		return m.framed()
	}
	return m, nil
}

// View draws the dashboard full screen, as the document stands at the clock's
// time, over a footer saying when it was read and will be next.
func (m Model) View() tea.View {
	v := tea.View{AltScreen: true}
	if m.width <= 0 {
		// The terminal's size arrives just after the start.
		return v
	}
	now := m.now()
	frame := dashboard.Render(m.shown(now), now, dashboard.Options{
		Width:  m.width,
		Height: max(m.height-topMargin, 0),
		Color:  true,
		Footer: m.footer(now),
	})
	v.SetContent(strings.Repeat("\n", topMargin) + frame)
	return v
}

// pressed acts on a key: r reads now, and q or ctrl+c quits.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "r", "R":
		return m.refresh()
	case "q", "Q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// refresh starts a read, unless one is under way.
func (m Model) refresh() (Model, tea.Cmd) {
	if m.fetching {
		return m, nil
	}
	m.fetching = true
	return m, m.fetch()
}

// fetch reads the source.
func (m Model) fetch() tea.Cmd {
	ctx, source := m.ctx, m.cfg.Source
	return func() tea.Msg {
		doc, err := source.Fetch(ctx)
		return fetchedMsg{doc: doc, err: err}
	}
}

// fetched takes in a read: the document to show from now on, or why there's
// none. After a read that failed, in whole or in part, the next comes sooner
// than the interval, backing off as the failures run on; either way, a window
// on screen that resets brings it on.
func (m Model) fetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	m.fetching = false
	var wait time.Duration
	m.failures, wait = backoff(m.failures, msg.err != nil || incomplete(msg.doc), m.cfg.Interval)
	var cmd tea.Cmd
	if msg.err != nil {
		m.failed = msg.err.Error()
	} else {
		m, cmd = m.show(msg.doc, now)
	}
	m.next = nextFetch(m.doc, now, wait)
	return m, cmd
}

// show puts doc, read at now, on screen: it posts what the change calls for,
// eases the bars to it from where they stand, and starts a new chain of ticks
// at its pace, as it may count seconds where the last didn't, or stop.
func (m Model) show(doc status.Document, now time.Time) (Model, tea.Cmd) {
	notify := m.notify(m.readings.alerts(doc, now, m.cfg.Policy))
	m.readings = m.readings.with(doc, now)
	m.ease = easing{from: utilizations(m.shown(now)), start: now}
	m.doc, m.updated, m.failed = doc, now, ""
	m.chain++
	m, frames := m.startFrames()
	return m, tea.Batch(notify, m.armTick(now), frames)
}

// shown is the document as it's drawn at now, its bars part way along their
// easing.
func (m Model) shown(now time.Time) status.Document {
	return m.ease.apply(m.doc, now)
}

// ticked reads again once a read is due. Every tick redraws, as every message
// does, so countdowns and the clock stay live.
func (m Model) ticked(msg tickMsg) (tea.Model, tea.Cmd) {
	if msg.chain != m.chain {
		return m, nil
	}
	now := m.now()
	var fetch tea.Cmd
	if !now.Before(m.next) {
		m, fetch = m.refresh()
	}
	return m, tea.Batch(fetch, m.armTick(now))
}

// armTick arms the live chain's next tick. Ticks come at least every minute,
// never as one long timer to the next read: a timer's clock stops while a Mac
// sleeps, so one set for half an hour before the lid closes would fire half an
// hour after it opens. Each tick reads the wall clock instead.
func (m Model) armTick(now time.Time) tea.Cmd {
	return m.after(tickDelay(m.doc, now), tickMsg{chain: m.chain})
}

// startFrames starts drawing frames while the bars ease, unless none moves or
// frames are already running, which carry on through the new easing.
func (m Model) startFrames() (Model, tea.Cmd) {
	if m.framing || !m.ease.moves(m.doc) {
		return m, nil
	}
	m.framing = true
	return m, m.after(frameEvery, frameMsg{})
}

// framed arms the next frame, until the easing is done.
func (m Model) framed() (tea.Model, tea.Cmd) {
	if m.ease.done(m.now()) {
		m.framing = false
		return m, nil
	}
	return m, m.after(frameEvery, frameMsg{})
}

// notify posts the messages in turn.
func (m Model) notify(messages []string) tea.Cmd {
	if len(messages) == 0 {
		return nil
	}
	notifier := m.cfg.Notifier
	return func() tea.Msg {
		for _, message := range messages {
			// A full-screen view has nowhere to say a notification failed.
			_ = notifier.Notify(message)
		}
		return nil
	}
}

// footer says when the document was read and will be next, that a read is
// under way, or why the last one failed; then what the keys do.
func (m Model) footer(now time.Time) string {
	var state string
	switch {
	case m.fetching && m.updated.IsZero():
		state = "reading usage…"
	case m.fetching:
		state = "refreshing…"
	case m.failed != "":
		state = "couldn't read usage: " + m.failed + " · next " + hourMinute(now, m.next)
	default:
		state = "updated " + hourMinute(now, m.updated) + " · next " + hourMinute(now, m.next)
	}
	return state + " · " + keys
}

// now reads the wall clock without its monotonic reading, which stops while a
// Mac sleeps: kept, it would put every deadline off by the time spent asleep.
func (m Model) now() time.Time {
	return m.cfg.Now().Round(0)
}

// hourMinute shows t in now's time zone, such as "13:51".
func hourMinute(now, t time.Time) string {
	return t.In(now.Location()).Format("15:04")
}

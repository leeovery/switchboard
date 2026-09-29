// Package watch keeps the dashboard on screen. While the router answers, it
// reads the router's status document every few seconds, and takes keys that
// tell the router where to send sessions, leaving desktop notifications to the
// router. While it doesn't, it probes every account as each read falls due,
// and posts its own notifications, as the config asks, when an account has
// room again or a window passes the warning. Probing as asked while the
// router answers, it leaves them to the router all the same. It redraws as
// the clock moves, and eases each bar to its new reading. Beyond its log, the
// model does no I/O of its own: it's handed its source, its clock and its
// notifier, so tests drive it as a terminal would.
package watch

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// logger notes what a watch does: a full-screen view leaves nowhere else to
// say what went wrong.
var logger = logs.For("watch")

// ErrNoRouter is what a read that doesn't probe fails with when the router
// doesn't answer.
var ErrNoRouter = errors.New("the router isn't answering")

// Source is where the dashboard reads the status document: the router, while
// it answers, else probing every account. It also tells the router where to
// send sessions.
type Source interface {
	// Read reads the document as r asks.
	Read(ctx context.Context, r Read) (status.Document, error)
	// Pin has the router send every new session to the account with the
	// given id, and with move, every running session too.
	Pin(ctx context.Context, account string, move bool) error
	// Unpin has the router route every session on its merits again.
	Unpin(ctx context.Context) error
	// RouterAnswers reports whether the router answers: while it does, it
	// posts the desktop notifications, even as the source probes as asked.
	RouterAnswers(ctx context.Context) bool
}

// A Read is what a read of the source asks for.
type Read struct {
	// Refresh, when the router answers, has it first probe the accounts it
	// hasn't read for this long; zero takes its document as it stands.
	Refresh time.Duration
	// Probe, when the router doesn't answer, builds the document by probing
	// every account instead. Without it, such a read fails with ErrNoRouter.
	Probe bool
}

// full reports whether the read brings every account up to date: the router
// refreshes those it hasn't read lately, or every account is probed.
func (r Read) full() bool {
	return r.Probe && r.Refresh > 0
}

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(message string) error
}

// Config is what a watch is given.
type Config struct {
	Source   Source
	Notifier Notifier
	// Notifications says which notifications the dashboard posts while it
	// probes without the router: room again, and a window passing the
	// warning. The others are the router's alone, as only it sees limits
	// reached and sessions moved.
	Notifications config.Notifications
	// Now reads the wall clock.
	Now func() time.Time
	// Interval is the longest the dashboard goes between full reads: ones
	// that probe, or have the router refresh what it hasn't read lately.
	Interval time.Duration
	// Policy is the provider's say in which windows leave an account without
	// room.
	Policy score.Policy
	// Size is what to draw at until the terminal gives its size, and in place
	// of a width or height it gives as zero, which means it doesn't know.
	Size Size
}

// Size is a terminal's size in cells.
type Size struct {
	Width, Height int
}

// or is s with each dimension it has as zero, which means it isn't known,
// taken from known.
func (s Size) or(known Size) Size {
	return Size{Width: cmp.Or(s.Width, known.Width), Height: cmp.Or(s.Height, known.Height)}
}

// topMargin is the blank lines above the frame.
const topMargin = 1

// Model is a watch's state, as Bubble Tea runs it. Build one with New.
type Model struct {
	ctx context.Context
	cfg Config
	// after delivers msg once d has passed. Tests replace it, to hold each
	// timer until they fire it.
	after func(d time.Duration, msg tea.Msg) tea.Cmd

	// size is what the frame is drawn at: the terminal's, as far as it's
	// known.
	size Size

	doc status.Document
	// updated is when doc arrived: zero until one has.
	updated time.Time
	// plan is when the next reads are due, and what they ask for.
	plan plan
	// fetching is set while a read is under way, and loud while the footer
	// tells of it.
	fetching, loud bool
	// again asks for a look at the router's document as soon as the read
	// under way lands.
	again bool
	// failed says why the last read failed, if it did.
	failed string
	// lost is when the router stopped answering, while the dashboard has
	// probed since.
	lost time.Time

	// ordering is set while the router carries out an order a key gave.
	ordering bool
	// note says what the last key did, till noteUntil.
	note      string
	noteUntil time.Time

	// chain numbers the live chain of ticks: a tick from an earlier one is
	// dropped.
	chain int
	// ease moves each bar to doc's reading, and framing is set while frames
	// run to draw it.
	ease     easing
	framing  bool
	readings readings
}

// fetchedMsg is what a read that asked for read found.
type fetchedMsg struct {
	read Read
	doc  status.Document
	err  error
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
	return Model{ctx: ctx, cfg: cfg, after: after, size: cfg.Size, plan: plan{interval: cfg.Interval}, fetching: true, loud: true}
}

// after delivers msg once d has passed.
func after(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// Init starts the first read, a full one, and the ticks.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetch(m.plan.full()), m.armTick(m.now()))
}

// Update takes in a message: a key, a resize, a read, the router carrying
// out an order, a tick or a frame. The view is drawn again after each, which
// is all a note's lapsing asks.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resized(msg)
	case tea.KeyPressMsg:
		return m.pressed(msg)
	case fetchedMsg:
		return m.fetched(msg)
	case orderedMsg:
		return m.ordered(msg)
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
	now := m.now()
	frame := dashboard.Render(m.shown(now), now, dashboard.Options{
		Width:  m.size.Width,
		Height: max(m.size.Height-topMargin, 0),
		Color:  true,
		Footer: m.footer(now),
	})
	v := tea.NewView(strings.Repeat("\n", topMargin) + frame)
	v.AltScreen = true
	return v
}

// resized takes in the size the terminal gives, keeping the size drawn at for
// a width or height it gives as zero.
func (m Model) resized(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	given := Size{Width: msg.Width, Height: msg.Height}
	m.size = given.or(m.size)
	if m.size == given {
		return m, nil
	}
	// Bubble Tea cuts every frame to the size in the last size message it
	// passed on, so the model passes on the size it draws at. Proven under a
	// pseudo-terminal giving 0×0: without this, the screen stayed blank.
	size := tea.WindowSizeMsg{Width: m.size.Width, Height: m.size.Height}
	return m, func() tea.Msg { return size }
}

// read reads the source as r asks, unless a read is under way.
func (m Model) read(r Read) (Model, tea.Cmd) {
	if m.fetching {
		return m, nil
	}
	m.fetching, m.loud = true, r.full()
	return m, m.fetch(r)
}

// fetch reads the source as r asks.
func (m Model) fetch(r Read) tea.Cmd {
	ctx, source := m.ctx, m.cfg.Source
	return func() tea.Msg {
		doc, err := source.Read(ctx, r)
		return fetchedMsg{read: r, doc: doc, err: err}
	}
}

// fetched takes in a read: the document to show from now on, or why there's
// none, and plans the next read by it. A question after the router that found
// it gone changes nothing. Each read starts a new chain of ticks at the pace
// it calls for, and lets a look at the router's document asked for while it
// was under way go ahead.
func (m Model) fetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	m.fetching, m.loud = false, false
	var shown tea.Cmd
	switch {
	case errors.Is(msg.err, ErrNoRouter):
		m.plan = m.plan.missed(now)
	case msg.err != nil:
		m.failed = msg.err.Error()
		m.plan = m.plan.failed(m.doc, now)
		logger.Warn("usage read failed", "error", msg.err, "next", m.plan.due)
	default:
		m.plan = m.plan.landed(msg.read, msg.doc, now)
		m, shown = m.show(msg.doc, now)
		logRead(msg, m.plan.due)
	}
	m.chain++
	var again tea.Cmd
	if m.again {
		m.again = false
		m, again = m.reread()
	}
	return m, tea.Batch(shown, m.armTick(now), again)
}

// logRead notes a read: where it came from, the accounts it read and those it
// couldn't, and when the next full read is due. A look at the router's
// document as it stands, every few seconds, is noted only at debug.
func logRead(msg fetchedMsg, next time.Time) {
	var read, failed []string
	for _, a := range msg.doc.Accounts {
		if wasRead(a) {
			read = append(read, a.ID)
		} else {
			failed = append(failed, a.ID)
		}
	}
	level := slog.LevelInfo
	if routed(msg.doc) && msg.read.Refresh == 0 {
		level = slog.LevelDebug
	}
	logger.Log(context.Background(), level, "usage read", "source", msg.doc.Source,
		"read", strings.Join(read, ","), "failed", strings.Join(failed, ","), "next", next)
}

// show puts doc, read at now, on screen: it posts what the change calls for,
// unless the router is there to post its own, as nothing is to be told twice;
// follows the router as it goes and comes back; and eases the bars to doc
// from where they stand.
func (m Model) show(doc status.Document, now time.Time) (Model, tea.Cmd) {
	var post tea.Cmd
	if !routed(doc) {
		post = m.post(m.readings.alerts(doc, now, m.cfg.Policy, m.cfg.Notifications), probedAsAsked(doc))
	}
	m.readings = m.readings.with(doc, now)
	m = m.follow(doc, now)
	m.ease = easing{from: utilizations(m.shown(now)), start: now}
	m.doc, m.updated, m.failed = doc, now, ""
	m, frames := m.startFrames()
	return m, tea.Batch(post, frames)
}

// follow notes where doc, read at now, came from, against where the document
// on screen did: the router lost when it stops answering, and found again.
func (m Model) follow(doc status.Document, now time.Time) Model {
	switch was, is := m.routed(), routed(doc); {
	case was && !is:
		m.lost = now
		logger.Warn("the router stopped answering; probing directly", "router", doc.Fallback.Router, "reason", doc.Fallback.Reason)
	case !was && is:
		m.lost = time.Time{}
		logger.Info("reading the router")
	}
	return m
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
	var read tea.Cmd
	if r, ok := m.plan.at(now, m.routed()); ok {
		m, read = m.read(r)
	}
	return m, tea.Batch(read, m.armTick(now))
}

// armTick arms the live chain's next tick: just past the next second while a
// countdown shows seconds, else just past the next minute, or, reading the
// router, just past the next look at its document when that comes sooner.
// Ticks never come as one long timer to the next read: a timer's clock stops
// while a Mac sleeps, so one set for half an hour before the lid closes would
// fire half an hour after it opens. Each tick reads the wall clock instead.
func (m Model) armTick(now time.Time) tea.Cmd {
	delay := tickDelay(m.doc, now)
	if until := m.plan.next.Sub(now); m.routed() && until > 0 {
		delay = min(delay, until+tickSlack)
	}
	return m.after(delay, tickMsg{chain: m.chain})
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

// post posts the alerts in turn, noting each in the log. Of a document probed
// as asked, it asks after the router first, and posts none while it answers,
// as the router posts its own.
func (m Model) post(alerts []notify.Notice, asked bool) tea.Cmd {
	if len(alerts) == 0 {
		return nil
	}
	ctx, source, notifier := m.ctx, m.cfg.Source, m.cfg.Notifier
	return func() tea.Msg {
		if asked && source.RouterAnswers(ctx) {
			for _, a := range alerts {
				logger.Debug("notification left to the router", "account", a.Account, "news", a.News)
			}
			return nil
		}
		for _, a := range alerts {
			if err := notifier.Notify(a.Message); err != nil {
				logger.Warn("notification failed", "account", a.Account, "news", a.News, "error", err)
				continue
			}
			logger.Info("notification", "account", a.Account, "news", a.News)
		}
		return nil
	}
}

// footer says since when the router hasn't answered, while the dashboard
// probes for want of it; then what the last key did, while that's news, or
// else how reading goes; and last what the keys do.
func (m Model) footer(now time.Time) string {
	parts := []string{m.state(now), m.keys()}
	if !m.lost.IsZero() {
		parts = append([]string{"no router since " + status.TimeOfDay(now, m.lost)}, parts...)
	}
	return strings.Join(parts, " · ")
}

// state says what the last key did, while that's news; else that a read is
// under way, why the last one failed, or when the document was read, and,
// while it's probed, when it will be next.
func (m Model) state(now time.Time) string {
	switch {
	case now.Before(m.noteUntil):
		return m.note
	case m.fetching && m.updated.IsZero():
		return "reading usage…"
	case m.fetching && m.loud:
		return "refreshing…"
	case m.failed != "":
		return "couldn't read usage: " + m.failed + " · next " + status.TimeOfDay(now, m.plan.due)
	case m.routed():
		return "updated " + status.TimeOfDay(now, m.updated)
	default:
		return "updated " + status.TimeOfDay(now, m.updated) + " · next " + status.TimeOfDay(now, m.plan.due)
	}
}

// routed reports whether the document on screen is the router's.
func (m Model) routed() bool {
	return routed(m.doc)
}

// routed reports whether doc is the router's.
func routed(doc status.Document) bool {
	return doc.Source == status.SourceRouter
}

// probedAsAsked reports whether doc was built by probing as asked, rather
// than for want of the router, which may be running all the same: one probed
// for want of it says why.
func probedAsAsked(doc status.Document) bool {
	return !routed(doc) && doc.Fallback == (status.Fallback{})
}

// now reads the wall clock without its monotonic reading, which stops while a
// Mac sleeps: kept, it would put every deadline off by the time spent asleep.
func (m Model) now() time.Time {
	return m.cfg.Now().Round(0)
}

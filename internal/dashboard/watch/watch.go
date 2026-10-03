// Package watch keeps the dashboard on screen. While the router answers, it
// reads the router's status document every few seconds, and its history with
// each full read, picks out the events each look finds new, and takes keys
// that tell the router where to send sessions, leaving desktop notifications
// to the router. While it doesn't, it probes every account as each read falls
// due, keeps the readings it sees for the charts, and posts its own
// notifications, as the config asks, when an account has room again or a
// window passes the warning. Probing as asked while the router answers, it
// leaves them to the router all the same. It redraws as the clock moves, every
// second, and eases each bar to its new reading. Beyond its log, the model
// does no I/O of its own: it's handed its source, its clock, its notifier and
// where it keeps its preferences, so tests drive it as a terminal would.
package watch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// logger notes what a watch does: a full-screen view leaves nowhere else to
// say what went wrong.
var logger = logs.For("watch")

// ErrNoRouter is what a read that doesn't probe fails with when the router
// doesn't answer.
var ErrNoRouter = errors.New("the router isn't answering")

// Source is where the dashboard reads the status document: the router, while
// it answers, else probing every account. It also tells the router where to
// send sessions, and gives its history.
type Source interface {
	// Read reads the document as r asks, and says which router gave it, as
	// its health check answered: zero for a document built by probing.
	Read(ctx context.Context, r Read) (status.Document, router.Health, error)
	// Pin has the router send every new session to the best of the accounts
	// with the given ids, and with move, every running session on another
	// account too.
	Pin(ctx context.Context, accounts []string, move bool) error
	// Unpin has the router route every session on its merits again.
	Unpin(ctx context.Context) error
	// PinSession has the router send every request of the session with the
	// given id to the account with the given id from its next request on,
	// as pin --session does.
	PinSession(ctx context.Context, session, account string) error
	// UnpinSession has the router clear the session's own pin, routing it on
	// its merits from its next request on, as pin auto --session does.
	UnpinSession(ctx context.Context, session string) error
	// RouterAnswers reports whether the router answers: while it does, it
	// posts the desktop notifications, even as the source probes as asked.
	RouterAnswers(ctx context.Context) bool
	// History gives every account's use of the window with the given key
	// over its current length, a point each step, as the router's GET
	// /history does, failing with router.ErrNoHistory, wrapped, where the
	// router is from before it.
	History(ctx context.Context, window string, step time.Duration) (router.History, error)
	// Sessions lists the sessions the router has routed in the last hour,
	// the one seen last first, as its GET /sessions does.
	Sessions(ctx context.Context) ([]status.Session, error)
	// Stream opens the router's request stream, as its GET /stream does,
	// which tells of what befalls each routed request as it happens, until
	// ctx ends or the router ends it, as the channel's closing says; failing
	// with router.ErrNoStream, wrapped, where the router is from before it.
	Stream(ctx context.Context) (<-chan router.StreamEvent, error)
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

// Fresh is the read r asks for, which usage --refresh asks for too: the
// router first refreshes every account it hasn't read in the last minute, the
// least it waits between probes of one, and every one that can take no
// request anyway, or, without the router, every account is probed.
func Fresh() Read {
	return Read{Refresh: freshFor, Probe: true}
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
	// After delivers msg once d has passed on the clock, for the ticks that
	// redraw as it moves and the frames that ease the bars: nil takes a real
	// timer.
	After func(d time.Duration, msg tea.Msg) tea.Cmd
	// Await has wait run, the command that waits for what the router's
	// request stream tells next, which blocks until it tells something: nil
	// runs it as any command is run. A test holds it, to tell the stream's
	// events in turn.
	Await func(wait tea.Cmd) tea.Cmd
	// Interval is the longest the dashboard goes between full reads: ones
	// that probe, or have the router refresh what it hasn't read lately.
	Interval time.Duration
	// Policy is the provider's say in which windows leave an account without
	// room.
	Policy score.Policy
	// Size is what to draw at until the terminal gives its size, and in place
	// of a width or height it gives as zero, which means it doesn't know.
	Size Size
	// Choice is the theme, or the pair of themes, the user chose, and Pair
	// the themes it draws in: what the dashboard is drawn in from the start.
	Choice theme.Choice
	Pair   theme.Pair
	// Themes are where the theme picker finds the themes to pick from, and
	// keeps the user's pick: nil draws the dashboard without colour, as
	// NO_COLOR asks, and leaves t doing nothing.
	Themes Themes
	// View is the view the preferences kept, which the watch opens on where
	// it's one there is, Featured the window they keep the cards featuring,
	// and Chart the style they keep the cards drawing it in; Prefs keeps
	// each as it changes: nil keeps nothing.
	View     dashboard.View
	Featured dashboard.Feature
	Chart    dashboard.Chart
	Prefs    Prefs
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

// Model is a watch's state, as Bubble Tea runs it. Build one with New.
type Model struct {
	ctx context.Context
	cfg Config
	// after delivers msg once d has passed: Config.After, else a real timer.
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
	// probed since, or shows its last document.
	lost time.Time

	// ordering is set while the router carries out an order a key gave.
	ordering bool
	// note says what the last key did, till noteUntil.
	note      string
	noteUntil time.Time

	// chain numbers the live chain of ticks: a tick from an earlier one is
	// dropped.
	chain int
	// ease moves each bar to doc's reading; framing is set while frames run
	// to draw what moves, and frameDue is when the next is due.
	ease     easing
	framing  bool
	frameDue time.Time
	readings readings

	// choice is the theme or pair the user chose, and pair the themes it
	// draws in; showing is the theme the screen is drawn in, the one in
	// force or the one the picker's cursor is on.
	choice   theme.Choice
	pair     theme.Pair
	showing  theme.Theme
	backdrop backdrop
	picker   picker

	// views are the views there are, in tab's order, and view the one shown.
	views []dashboard.View
	view  dashboard.View
	// news is what the looks have seen of the router's events, and changes
	// of the cards' states.
	news    news
	changes changes
	// history is how the accounts' windows have been used, and trails the
	// charts' share of it, of the document on screen.
	history history
	trails  dashboard.History
	// sessions are the sessions the router listed with the document on
	// screen, for the cards' dots and Sessions' calls: nil where it listed
	// none. order is the order Sessions' calls ran in as they were listed,
	// which they keep from look to look.
	sessions []status.Session
	order    dashboard.Order
	// stream is the watch's hold on the router's request stream, and traffic
	// what the stream has told of the routed requests.
	stream  streaming
	traffic traffic
	// featured is which window every card features, chart the style every
	// card draws it in, and span how far ahead Runway looks.
	featured dashboard.Feature
	chart    dashboard.Chart
	span     dashboard.Span
	// scroll is how many rows the view shown is scrolled down by, where its
	// cards, or its lanes, don't fit.
	scroll int
	// focus is the account whose card has the focus, "" while none has it;
	// flipped are the accounts whose cards are flipped, by id, a set no
	// model changes once it's made; and selected is the session picked out
	// on the back of the card with the focus, zero while none is.
	focus    string
	flipped  map[string]bool
	selected dashboard.Seat
	// helping is set while the help is open over the view.
	helping bool
}

// fetchedMsg is what a read that asked for read found, and which router gave
// it, with the sessions it listed and when it asked for them, listed: zero
// where it listed none.
type fetchedMsg struct {
	read     Read
	doc      status.Document
	router   router.Health
	sessions []status.Session
	listed   time.Time
	err      error
}

// tickMsg wakes the model to redraw, and to read again once that's due.
type tickMsg struct {
	chain int
}

// frameMsg draws the next frame of what moves on screen.
type frameMsg struct{}

// New returns a model that reads cfg's source at once, and whenever a read
// falls due after, under ctx.
func New(ctx context.Context, cfg Config) Model {
	views := dashboard.Views()
	m := Model{
		ctx: ctx, cfg: cfg, after: cfg.After, size: cfg.Size, plan: plan{interval: cfg.Interval}, fetching: true, loud: true,
		choice: cfg.Choice, pair: cfg.Pair, views: views, view: opening(cfg.View, views), featured: cfg.Featured, chart: cfg.Chart,
	}
	if m.after == nil {
		m.after = after
	}
	return m
}

// after delivers msg once d has passed, on a real timer.
func after(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// Init starts the first read, a full one, and the ticks, and in colour, asks
// the terminal what its background is (OSC 11), the screen staying blank for
// theme.AnswerWithin at most, so as not to flash one theme then the other.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetch(m.plan.full()), m.armTick(m.now())}
	if m.coloured() {
		cmds = append(cmds, tea.RequestBackgroundColor, m.after(theme.AnswerWithin, unansweredMsg{}))
	}
	return tea.Batch(cmds...)
}

// Update takes in a message, as take says, then has the watch read the
// router's request stream while it wants it, and only then, and draw frames
// while anything on screen moves. The view is drawn again after each, which
// is all a note's lapsing asks.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.take(msg)
	tuned, stream := next.(Model).tune()
	tuned, frames := tuned.startFrames(tuned.now())
	return tuned, tea.Batch(cmd, stream, frames)
}

// take takes in a message: a key, the wheel, a resize, the terminal's
// background, the themes listed for the picker, or the choice kept as it
// stands, a read, the router's history, the router carrying out an order,
// the request stream opening, telling of requests or falling due to be asked
// for again, a tick or a frame.
func (m Model) take(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resized(msg)
	case tea.KeyPressMsg:
		return m.pressed(msg)
	case tea.MouseWheelMsg:
		return m.wheeled(msg), nil
	case tea.BackgroundColorMsg:
		return m.answered(msg.Color), nil
	case unansweredMsg:
		return m.noAnswer(), nil
	case listedMsg:
		return m.listed(msg)
	case askMsg:
		return m.asked(msg), nil
	case keptMsg:
		return m.kept(msg), nil
	case fetchedMsg:
		return m.fetched(msg)
	case historyMsg:
		m.history = m.history.answered(msg)
		m.trails = m.history.drawn(m.doc)
		return m, nil
	case orderedMsg:
		return m.ordered(msg)
	case streamOpenedMsg:
		return m.opened(msg)
	case streamHeardMsg:
		return m.heard(msg)
	case rejoinMsg:
		return m.rejoined(msg), nil
	case tickMsg:
		return m.ticked(msg)
	case frameMsg:
		return m.framed()
	}
	return m, nil
}

// View draws the dashboard full screen, as the document stands at the clock's
// time, the help over its middle and the theme picker over its right while
// they're open. In colour, it's drawn once the terminal has said what its
// background is, or had its time to, so it never shows one theme then
// another: in the theme shown, its canvas painted on every cell, and set as
// the terminal's background too, as background says. While the cards scroll,
// it asks for the wheel, which takes the terminal's own selecting with the
// mouse, so it asks for it then alone.
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.coloured() && !m.backdrop.settled {
		return v
	}
	now := m.now()
	doc, f := m.shown(now), m.frame(now)
	f.Keys = m.keys()
	if m.helping {
		f.Help = m.helpKeys()
	}
	lines := f.Draw(doc, now)
	if m.picker.open {
		lines = m.picker.drawn(m.choice).Over(lines, m.size.Width, f.Look)
	}
	v.SetContent(strings.Join(lines, "\n"))
	v.BackgroundColor = m.background(f.Look)
	if f.Scrolling(doc, now).Most > 0 {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// frame is the dashboard's frame as the model stands at now, but for the
// keys its footer lists: at the size drawn at, in the look, showing the view
// shown, with what the watch knows of the router, its events and its
// history, the window the cards feature and the style they draw it in, how
// far ahead Runway looks, the sessions listed, what the request stream tells
// of their requests and the order Sessions' calls keep, how far the view is
// scrolled, the card with the focus, the cards flipped and the session
// picked out, the keys that work on a card's sessions, the note a key left,
// and how reading goes.
func (m Model) frame(now time.Time) dashboard.Frame {
	return dashboard.Frame{
		Width: m.size.Width, Height: m.size.Height, Look: m.look(),
		Views: m.views, View: m.view,
		Lost: m.lost, Outdated: m.history.outdated, Fresh: m.news.faded(now), Changed: m.changes.faded(now), History: m.trails,
		Featured: m.featured, Chart: m.chart, Span: m.span,
		Sessions: m.sessions, Traffic: m.traffic.at(now), Order: m.order, Scroll: m.scroll,
		Focus: m.focus, Flipped: m.flipped, Selected: m.selected, Patch: m.patch(),
		Note: m.noted(now), Status: m.status(now),
		Policy: m.cfg.Policy,
	}
}

// resized takes in the size the terminal gives, keeping the size drawn at for
// a width or height it gives as zero, and closes the theme picker should it
// no longer fit.
func (m Model) resized(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	given := Size{Width: msg.Width, Height: msg.Height}
	m.size = given.or(m.size)
	m, closed := m.fitPicker()
	if m.size == given {
		return m, closed
	}
	// Bubble Tea cuts every frame to the size in the last size message it
	// passed on, so the model passes on the size it draws at. Proven under a
	// pseudo-terminal giving 0×0: without this, the screen stayed blank.
	size := tea.WindowSizeMsg{Width: m.size.Width, Height: m.size.Height}
	return m, tea.Batch(closed, func() tea.Msg { return size })
}

// read reads the source as r asks, unless a read is under way.
func (m Model) read(r Read) (Model, tea.Cmd) {
	if m.fetching {
		return m, nil
	}
	m.fetching, m.loud = true, r.full()
	return m, m.fetch(r)
}

// fetch reads the source as r asks, as fetchFrom reads it, on the model's
// clock.
func (m Model) fetch(r Read) tea.Cmd {
	ctx, source, now := m.ctx, m.cfg.Source, m.now
	return func() tea.Msg {
		return fetchFrom(ctx, source, r, now)
	}
}

// fetchFrom reads the source as r asks, and from a router, the sessions it
// lists, for the cards' dots and Sessions' calls, stamped with when now says
// it asked for them, as a move told of after is one they don't list yet: a
// router that can't list them leaves them out.
func fetchFrom(ctx context.Context, source Source, r Read, now func() time.Time) fetchedMsg {
	doc, from, err := source.Read(ctx, r)
	msg := fetchedMsg{read: r, doc: doc, router: from, err: err}
	if err != nil || !routed(doc) {
		return msg
	}
	listed := now()
	sessions, err := source.Sessions(ctx)
	if err != nil {
		logger.Debug("couldn't list the router's sessions", "error", err)
		return msg
	}
	msg.sessions, msg.listed = sessions, listed
	return msg
}

// fetched takes in a read: the document to show from now on, or why there's
// none, and plans the next read by it. A question after the router, or a
// look at its document, that found it gone only notes the router lost, as
// lose does. Each read starts a new chain of ticks at the pace it calls for,
// and lets a look at the router's document asked for while it was under way
// go ahead.
func (m Model) fetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	m.fetching, m.loud = false, false
	var shown tea.Cmd
	switch {
	case errors.Is(msg.err, ErrNoRouter):
		m.plan = m.plan.missed(now)
		m = m.lose(now)
	case msg.err != nil:
		m.failed = msg.err.Error()
		m.plan = m.plan.failed(m.doc, now)
		logger.Warn("usage read failed", "error", msg.err, "next", m.plan.due)
	default:
		m.plan = m.plan.landed(msg.read, msg.doc, now)
		m, shown = m.show(msg, now)
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

// show puts the document a read found at now on screen, with the sessions
// the router listed, as they stood as it was asked for them, Sessions' calls
// keeping the order they ran in; or where it couldn't list them, those it
// listed last; and none while probing. It posts what the change calls for,
// unless the router is there to post its own, as nothing is to be told
// twice; follows the router as it goes and comes back; notes the events new
// to it, and the readings it gives; asks the router for its history with a
// full read, and as the router answers again, or another router does, as one
// restarted; keeps the focus, the cards flipped and the selection where
// they still are; and eases the bars to it from where they stand.
func (m Model) show(msg fetchedMsg, now time.Time) (Model, tea.Cmd) {
	doc := msg.doc
	var post, asked tea.Cmd
	if !routed(doc) {
		post = m.post(m.readings.alerts(doc, now, m.cfg.Policy, m.cfg.Notifications), probedAsAsked(doc))
	}
	ask := routed(doc) && (msg.read.full() || !m.answering() || m.news.another(msg.router))
	if routed(doc) {
		m = m.answeredAgain(msg.router)
	}
	m.readings = m.readings.with(doc, now)
	m = m.follow(doc, now)
	m.news = m.news.looked(doc, msg.router, now)
	m.changes = m.changes.looked(doc, msg.router, now, m.cfg.Policy)
	m.history = m.history.saw(doc)
	m.ease = easing{from: utilizations(m.shown(now)), start: now}
	m.doc, m.updated, m.failed = doc, now, ""
	if !msg.listed.IsZero() || !routed(doc) {
		m.order = m.frame(now).OrderOf(doc)
		m.sessions = msg.sessions
		m.traffic = m.traffic.listedAt(cmp.Or(msg.listed, now))
	}
	m = m.stillThere()
	m.trails = m.history.drawn(doc)
	if ask {
		m, asked = m.askHistory(doc)
	}
	return m, tea.Batch(post, asked)
}

// follow notes where doc, read at now, came from, against where the document
// on screen did: the router lost when it stops answering, unless a look lost
// it first, and found again.
func (m Model) follow(doc status.Document, now time.Time) Model {
	switch was, is := m.routed(), routed(doc); {
	case was && !is:
		m.lost = cmp.Or(m.lost, now)
		logger.Warn("the router stopped answering; probing directly", "router", doc.Fallback.Router, "reason", doc.Fallback.Reason)
	case is && (!was || !m.lost.IsZero()):
		m.lost = time.Time{}
		logger.Info("reading the router")
	}
	return m
}

// lose notes a look at the router's document, at now, finding the router
// gone: its document stays on screen, the footer saying since when there's
// been no router, until it answers again, or a full read probes.
func (m Model) lose(now time.Time) Model {
	if !m.routed() || !m.lost.IsZero() {
		return m
	}
	m.lost = now
	logger.Warn("the router stopped answering; showing its last document")
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
	m.traffic = m.traffic.tidied(now)
	var read tea.Cmd
	if r, ok := m.plan.at(now, m.routed()); ok {
		m, read = m.read(r)
	}
	return m, tea.Batch(read, m.armTick(now))
}

// armTick arms the live chain's next tick: just past the next second, or,
// reading the router, just past the next look at its document when that
// comes sooner.
// Ticks never come as one long timer to the next read: a timer's clock stops
// while a Mac sleeps, so one set for half an hour before the lid closes would
// fire half an hour after it opens. Each tick reads the wall clock instead.
func (m Model) armTick(now time.Time) tea.Cmd {
	delay := tickDelay(now)
	if until := m.plan.next.Sub(now); m.routed() && until > 0 {
		delay = min(delay, until+tickSlack)
	}
	return m.after(delay, tickMsg{chain: m.chain})
}

// startFrames starts drawing frames while anything on screen moves at now,
// unless they're running already at the pace what moves calls for: frames
// running at a falling hourglass's pace, as something starts moving
// smoothly, have one drawn at once.
func (m Model) startFrames(now time.Time) (Model, tea.Cmd) {
	switch {
	case m.framing && m.dueInTime(now):
		return m, nil
	case !m.framing && !m.moving(now):
		return m, nil
	}
	return m.armFrame(now)
}

// dueInTime reports whether the frame due next comes soon enough for what
// moves at now: within frameEvery while anything moves smoothly, else
// whenever it's armed.
func (m Model) dueInTime(now time.Time) bool {
	return !m.smoothly(now) || !m.frameDue.After(now.Add(frameEvery))
}

// armFrame arms the next frame, as nextFrame has it after now.
func (m Model) armFrame(now time.Time) (Model, tea.Cmd) {
	delay := m.nextFrame(now)
	m.framing, m.frameDue = true, now.Add(delay)
	return m, m.after(delay, frameMsg{})
}

// framed arms the next frame, while anything on screen moves. A frame that
// comes before the one due, armed before it at a slower pace, is dropped.
func (m Model) framed() (tea.Model, tea.Cmd) {
	switch now := m.now(); {
	case now.Before(m.frameDue):
		return m, nil
	case !m.moving(now):
		m.framing = false
		return m, nil
	default:
		return m.armFrame(now)
	}
}

// nextFrame is how long after now the next frame is drawn: frameEvery while
// anything moves smoothly; else, while only the sand falls in a card's
// hourglass, once it has fallen a grain more.
func (m Model) nextFrame(now time.Time) time.Duration {
	if m.smoothly(now) {
		return frameEvery
	}
	return dashboard.FallStep - now.Sub(now.Truncate(dashboard.FallStep))
}

// moving reports whether anything on screen moves at now: smoothly, or the
// sand falling in a card's hourglass, as Falling says, wherever one shows.
func (m Model) moving(now time.Time) bool {
	return m.smoothly(now) || m.frame(now).Falling(m.shown(now), now)
}

// smoothly reports whether anything on screen moves smoothly at now, frame
// by frame: a bar easing to its reading, the highlight on an event, or on a
// card's state, fading back, or what travels the cords, as travelling says.
func (m Model) smoothly(now time.Time) bool {
	return (m.ease.moves(m.doc) && !m.ease.done(now)) || m.news.faded(now) != nil || m.changes.faded(now) != nil || m.travelling(now)
}

// travelling reports whether what travels the cords moves at now, as the
// request stream tells, on Sessions' switchboard, the only place it's drawn.
// The cards' backs and Sessions' plain list are drawn again as the stream
// tells of more, and as the clock ticks, drawing nothing new while nothing
// happens.
func (m Model) travelling(now time.Time) bool {
	return m.view == dashboard.Sessions && m.traffic.moving(now) && m.frame(now).Switchboard(m.shown(now), now)
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

// status says how reading goes, at the footer's right: that the first read,
// or a full read, is under way; why the last read failed, and when the next
// is due; since when there's been no router, while its last document stays
// on screen; or how long ago the document was read, and, while it's probed,
// when it will be next.
func (m Model) status(now time.Time) string {
	switch {
	case m.fetching && m.updated.IsZero():
		return "reading usage…"
	case m.fetching && m.loud:
		return "refreshing…"
	case m.failed != "":
		return "couldn't read usage: " + m.failed + " · next " + status.TimeOfDay(now, m.plan.due)
	case m.routed() && !m.lost.IsZero():
		return "no router since " + status.TimeOfDay(now, m.lost)
	case m.routed():
		return "read " + ago(now, m.updated)
	default:
		return "read " + ago(now, m.updated) + " · next " + status.TimeOfDay(now, m.plan.due)
	}
}

// ago says how long before now t was: in seconds, such as "4s ago", within a
// minute, and from then as status.Countdown counts, such as "2m ago".
func ago(now, t time.Time) string {
	if d := now.Sub(t); d < time.Minute {
		return fmt.Sprintf("%ds ago", max(int(d/time.Second), 0))
	}
	return status.Countdown(t, now) + " ago"
}

// noted is what the last key did, while that's news at now: "" once it isn't.
func (m Model) noted(now time.Time) string {
	if now.Before(m.noteUntil) {
		return m.note
	}
	return ""
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

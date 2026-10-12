// Package watch keeps the dashboard on screen. While the router answers, it
// reads the router's status document every few seconds, and its history with
// each full read, picks out the events each look finds new, and takes the
// routing card's orders of where to send sessions, leaving desktop
// notifications to the router. While it doesn't, it probes every account as
// each read falls due, keeps the readings it sees for the charts, and posts
// its own notifications, as the config asks, when an account has room again
// or a window passes the warning. Probing as asked while the router answers,
// it leaves them to the router all the same. It redraws as the clock moves,
// every second, and eases each bar to its new reading. Beyond its log, the
// model does no I/O of its own: it's handed its source, its clock, its
// notifier, where it keeps its preferences, and the readers of the request
// ledger, the readings history and the router's events, so tests drive it as
// a terminal would.
package watch

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// logger notes what a watch does: a full-screen view leaves nowhere else to
// say what went wrong.
var logger = logs.For("watch")

// Source is where the dashboard reads the status document: the router, while
// it answers, else probing every account. It also tells the router where to
// send sessions, and gives its history.
type Source interface {
	// Read reads the document as r asks, and says which router gave it, as
	// its health check answered: zero for a document built by probing.
	Read(ctx context.Context, r status.Read) (status.Document, router.Health, error)
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

// full reports whether the read brings every account up to date: the router
// refreshes those it hasn't read lately, or every account is probed.
func full(r status.Read) bool {
	return r.Probe && r.Refresh > 0
}

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(message string) error
}

// Config is what a watch is given.
type Config struct {
	Source Source
	// ProbeOnly is set where the source probes every account as asked, as
	// --probe has it, never reading the router's document: there's no router
	// for a read that fails to have lost.
	ProbeOnly bool
	Notifier  Notifier
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
	// View is the tab the preferences kept, which the watch opens on where
	// it's one there is, Featured the window they keep the cards featuring,
	// and Chart the style they keep the cards drawing it in; Prefs keeps
	// each as it changes: nil keeps nothing.
	View     dashboard.View
	Featured dashboard.Feature
	Chart    dashboard.Chart
	Prefs    Prefs
	// Ledger, Readings and Events read the request ledger, the readings
	// history and the router's events where they lie: the watch opens none
	// of their files itself.
	Ledger   views.Ledger
	Readings views.Readings
	Events   views.Events
	// Prices are the price table a move's cost is worked out at, the
	// config's prices in place of any of the table's.
	Prices ledger.Table
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
	// fetching is set while a read is under way.
	fetching bool
	// again asks for a look at the router's document as soon as the read
	// under way lands.
	again bool
	// lost is when the router stopped answering, while the dashboard has
	// probed since, shows its last document, or has read nothing since.
	lost time.Time
	// failed is why the last read failed, while it's the last: "" once one
	// lands, and for a look that found the router gone.
	failed string

	// ordering is set while the router carries out an order the routing
	// card gave.
	ordering bool
	// note says what the last key did, till noteUntil.
	note      string
	noteUntil time.Time

	// chain numbers the live chain of ticks: a tick from an earlier one is
	// dropped.
	chain int
	// ease moves each bar to doc's reading. frames numbers the frames armed
	// to draw what moves, a frame armed before the last being dropped;
	// framing is set while the last is yet to come, due at frameDue.
	ease     easing
	frames   int
	framing  bool
	frameDue time.Time
	readings lastReads

	// choice is the theme or pair the user chose, and pair the themes it
	// draws in; showing is the theme the screen is drawn in, the one in
	// force or the one the picker's cursor is on.
	choice   theme.Choice
	pair     theme.Pair
	showing  theme.Theme
	backdrop backdrop
	picker   picker

	// view is the tab shown.
	view dashboard.View
	// news is what the looks have seen of the router's events, and changes
	// of the cards' states.
	news    news
	changes changes
	// history is how the accounts' windows have been used, and trails the
	// charts' share of it, of the document on screen.
	history history
	trails  dashboard.Trails
	// sessions are the sessions the router listed with the document on
	// screen, for the cards' dots and Sessions' calls: nil where it listed
	// none. listedBy is the router that listed them, as its health check
	// said; and order is the order Sessions' calls ran in as they were
	// listed, which they keep from look to look.
	sessions []status.Session
	listedBy router.Health
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
	// and flipped are the accounts whose cards are flipped, by id, a set no
	// model changes once it's made.
	focus   string
	flipped map[string]bool

	// open is the card open over the page, if any: the routing card, its
	// picker as routing holds it, or help, scrolled as help holds it. page is
	// the page as it stood when the card opened, which stands still behind
	// it.
	open    opened
	routing routing
	help    helping
	page    frozen
}

// opened is which card is open over the page.
type opened int

const (
	// nothingOpen is no card: the page is live.
	nothingOpen opened = iota
	// routingOpen is the routing card.
	routingOpen
	// helpOpen is the page's help.
	helpOpen
)

// frozen is the page as it stood when a card opened over it: its frame and
// the document it was drawn of, at the moment it was drawn. Only its size
// and its look follow the terminal's while the card is open, so nothing
// behind the card moves, the clock included.
type frozen struct {
	frame dashboard.Frame
	doc   status.Document
	at    time.Time
}

// fetchedMsg is what a read that asked for read found, and which router gave
// it, with the sessions it listed, where listed says it listed them.
type fetchedMsg struct {
	read     status.Read
	doc      status.Document
	router   router.Health
	sessions []status.Session
	listed   bool
	err      error
}

// tickMsg wakes the model to redraw, and to read again once that's due.
type tickMsg struct {
	chain int
}

// frameMsg draws the next frame of what moves on screen: the frame numbered
// gen as it was armed.
type frameMsg struct {
	gen int
}

// New returns a model that reads cfg's source at once, and whenever a read
// falls due after, under ctx.
func New(ctx context.Context, cfg Config) Model {
	m := Model{
		ctx: ctx, cfg: cfg, after: cfg.After, size: cfg.Size, plan: plan{interval: cfg.Interval}, fetching: true,
		choice: cfg.Choice, pair: cfg.Pair, view: opening(cfg.View), featured: cfg.Featured, chart: cfg.Chart,
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
// router's request stream while it wants it, and only then, price the moves
// the routing card would make while it wants them, and draw frames while
// anything on screen moves. The view is drawn again after each, which is all
// a note's lapsing asks.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.take(msg)
	tuned, stream := next.(Model).tune()
	tuned, costs := tuned.costing()
	tuned, frames := tuned.startFrames(tuned.now())
	return tuned, tea.Batch(cmd, stream, costs, frames)
}

// take takes in a message: a key, a click, the wheel, a resize, the
// terminal's background, the themes listed for the picker, or the choice
// kept as it stands, a read, the router's history, the router carrying out
// an order, the sessions listed to price the routing card's moves, the
// request stream opening, telling of requests or falling due to be asked for
// again, a tick or a frame.
func (m Model) take(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resized(msg)
	case tea.KeyPressMsg:
		return m.pressed(msg)
	case tea.MouseClickMsg:
		return m.clicked(msg)
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
	case costedMsg:
		return m.costed(msg), nil
	case streamOpenedMsg:
		return m.opened(msg)
	case streamHeardMsg:
		return m.heard(msg)
	case rejoinMsg:
		return m.rejoined(msg), nil
	case tickMsg:
		return m.ticked(msg)
	case frameMsg:
		return m.framed(msg), nil
	}
	return m, nil
}

// View draws the dashboard full screen, as screen has it at the clock's
// time, the theme picker over its right while it's open. In colour, it's
// drawn once the terminal has said what its background is, or had its time
// to, so it never shows one theme then another: in the theme shown, its
// canvas painted on every cell, and set as the terminal's background too, as
// background says. The mouse is on while it runs, for clicks and the wheel,
// so the terminal's own selecting takes its modifier.
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.coloured() && !m.backdrop.settled {
		return v
	}
	f, doc, at := m.screen(m.now())
	lines := f.Draw(doc, at)
	if m.picker.open {
		lines = m.picker.drawn(m.choice).Over(lines, m.size.Width, f.Look)
	}
	v.SetContent(strings.Join(lines, "\n"))
	v.BackgroundColor = m.background(f.Look)
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// pressed acts on a key: ctrl+c quits, whatever's open. While a card is
// open, its keys alone act, as routingKey and helpKey take them. Else q
// quits; while the theme picker is open, it takes every other key; and
// otherwise p opens the routing card, ? opens the page's help, R refreshes,
// and the rest are the page's, as pageKey takes them.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := key.String(); {
	case k == "ctrl+c":
		return m, tea.Quit
	case m.open == routingOpen:
		return m.routingKey(k)
	case m.open == helpOpen:
		return m.helpKey(k)
	case k == "q":
		return m, tea.Quit
	case m.picker.open:
		return m.pickerKey(k)
	case k == "p":
		return m.openRouting()
	case k == "?":
		return m.openHelp(), nil
	case k == "R":
		return m.refresh()
	}
	return m.pageKey(key)
}

// refresh reads now, as R asks: having the router refresh what it hasn't
// read in the last minute, and what can take no request, or probing when it
// doesn't answer.
func (m Model) refresh() (Model, tea.Cmd) {
	logger.Debug("refresh key pressed", "already_reading", m.fetching)
	return m.read(status.Fresh())
}

// screen is what's on screen at now: the frame drawn, and the document and
// the moment it's drawn of. With a card open, it's the page frozen as the
// card opened, at the size drawn at now and in the look as it stands, the
// card over it; else the page as the model stands at now, its key line
// listing the keys that work.
func (m Model) screen(now time.Time) (dashboard.Frame, status.Document, time.Time) {
	if m.open == nothingOpen {
		return m.live(now), m.doc, now
	}
	f := m.page.frame
	f.Width, f.Height, f.Look = m.size.Width, m.size.Height, m.look()
	f.Card = m.card(now)
	return f, m.page.doc, m.page.at
}

// live is the page as the model stands at now: its frame, its key line
// listing the keys that work.
func (m Model) live(now time.Time) dashboard.Frame {
	f := m.frame(now)
	f.KeyLine = m.keyLine(now)
	return f
}

// card is the card open over the page, as the model stands at now: the
// routing card, its figures read afresh, or help.
func (m Model) card(now time.Time) dashboard.Opened {
	if m.open == routingOpen {
		return m.routingCard(now)
	}
	return m.helpCard()
}

// cover opens the card given over the page, which stands still behind it as
// it stands now.
func (m Model) cover(card opened) Model {
	now := m.now()
	m.open, m.page = card, frozen{frame: m.live(now), doc: m.doc, at: now}
	return m
}

// uncover closes the card open over the page, which is live again.
func (m Model) uncover() Model {
	m.open, m.page = nothingOpen, frozen{}
	return m
}

// frame is the dashboard's frame as the model stands at now, but for its key
// line and any card over it: at the size drawn at, in the look, showing the
// tab shown, with what the watch knows of the router, its events and its
// history, the window the cards feature and the style they draw it in, how
// far ahead Runway looks, the sessions listed, what the request stream tells
// of their requests and the order Sessions' calls keep, how far the view is
// scrolled, the card with the focus and the cards flipped.
func (m Model) frame(now time.Time) dashboard.Frame {
	return dashboard.Frame{
		Width: m.size.Width, Height: m.size.Height, Look: m.look(),
		View: m.view,
		Lost: m.lost, Failed: m.failure(), Outdated: m.history.outdated, Fresh: m.news.faded(now), Changed: m.changes.faded(now), Trails: m.trails, Eased: m.eased(now),
		Featured: m.featured, Chart: m.chart, Span: m.span,
		Sessions: m.sessions, Traffic: m.traffic.at(now), Order: m.order, Scroll: m.scroll,
		Focus: m.focus, Flipped: m.flipped,
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
func (m Model) read(r status.Read) (Model, tea.Cmd) {
	if m.fetching {
		return m, nil
	}
	m.fetching = true
	return m, m.fetch(r)
}

// fetch reads the source as r asks, as fetchFrom reads it.
func (m Model) fetch(r status.Read) tea.Cmd {
	ctx, source := m.ctx, m.cfg.Source
	return func() tea.Msg {
		return fetchFrom(ctx, source, r)
	}
}

// fetchFrom reads the source as r asks, and from a router, the sessions it
// lists, for the cards' dots and Sessions' calls: a router that can't list
// them leaves them out.
func fetchFrom(ctx context.Context, source Source, r status.Read) fetchedMsg {
	doc, from, err := source.Read(ctx, r)
	msg := fetchedMsg{read: r, doc: doc, router: from, err: err}
	if err != nil || !routed(doc) {
		return msg
	}
	sessions, err := source.Sessions(ctx)
	if err != nil {
		logger.Debug("couldn't list the router's sessions", "error", err)
		return msg
	}
	msg.sessions, msg.listed = sessions, true
	return msg
}

// fetched takes in a read: the document to show from now on, or why there's
// none, and plans the next read by it. A read that fails notes why, and the
// router lost, as lose does: a question after the router, or a look at its
// document, that found it gone does nothing more. Each read starts a new
// chain of ticks at the pace it calls for, and lets a look at the router's
// document asked for while it was under way go ahead.
func (m Model) fetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	m.fetching = false
	var shown tea.Cmd
	switch {
	case errors.Is(msg.err, status.ErrNoRouter):
		m.plan = m.plan.missed(now)
		m = m.lose(now)
	case msg.err != nil:
		m.plan = m.plan.failed(m.doc, now)
		m.failed = redact.Text(msg.err.Error())
		logger.Warn("usage read failed", "error", msg.err, "next", m.plan.due)
		m = m.lose(now)
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
// the router listed, Sessions' calls keeping the order they ran in, each
// move they show done no longer re-patching them; or where it couldn't list
// them, those it listed last, while the router that listed them answers, as
// a router restarted or replaced lists its own; and none while probing. It
// posts what the change calls for, unless the router is there to post its
// own, as nothing is to be told twice; follows the router as it goes and
// comes back; notes the events new to it, and the readings it gives; asks
// the router for its history with a full read, and as the router answers
// again, or another router does, as one restarted; keeps the focus and the
// cards flipped where they still are; and eases the bars to it from where
// they stand.
func (m Model) show(msg fetchedMsg, now time.Time) (Model, tea.Cmd) {
	doc := msg.doc
	var post, asked tea.Cmd
	if !routed(doc) {
		post = m.post(m.readings.alerts(doc, now, m.cfg.Policy, m.cfg.Notifications), probedAsAsked(doc))
	}
	ask := routed(doc) && (full(msg.read) || !m.answering() || m.news.another(msg.router))
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
	switch {
	case msg.listed || !routed(doc):
		m.order = m.frame(now).OrderOf(doc)
		m.sessions, m.listedBy = msg.sessions, msg.router
		m.traffic = m.traffic.listedAs(msg.sessions)
	case !msg.router.Same(m.listedBy):
		m.sessions = nil
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

// lose notes a read at now that failed, as the router not answering it:
// while its document is on screen, which stays there, the status line
// saying since when the router hasn't answered, until it answers again, or a
// full read probes; and before any document, the status line saying that
// alone, unless the source probes as asked, with no router to lose. A read
// that fails while probing says nothing of the router: the status line says
// why it failed instead.
func (m Model) lose(now time.Time) Model {
	switch {
	case !m.lost.IsZero():
		return m
	case m.routed():
		logger.Warn("the router stopped answering; showing its last document")
	case m.updated.IsZero() && !m.cfg.ProbeOnly:
		logger.Warn("the router isn't answering, and nothing has been read")
	default:
		return m
	}
	m.lost = now
	return m
}

// failure is why the last read failed, and when the next is due, for the
// status line: zero where it didn't fail.
func (m Model) failure() dashboard.Failure {
	if m.failed == "" {
		return dashboard.Failure{}
	}
	return dashboard.Failure{Reason: m.failed, Next: m.plan.due}
}

// shown is the document as its bars stand at now, part way along their
// easing.
func (m Model) shown(now time.Time) status.Document {
	return m.ease.apply(m.doc, now)
}

// eased are how much of each window the bars fill at now, by its account
// and key, while they ease to the document's readings: nil once they're
// there.
func (m Model) eased(now time.Time) map[dashboard.Ref]float64 {
	if m.ease.done(now) || !m.ease.moves(m.doc) {
		return nil
	}
	eased := make(map[dashboard.Ref]float64)
	for _, a := range m.shown(now).Accounts {
		for _, w := range a.Windows {
			eased[dashboard.Ref{Account: a.ID, Window: w.Key}] = w.Utilization
		}
	}
	return eased
}

// ticked reads again once a read is due. Every tick redraws, as every message
// does, so countdowns and the clock stay live.
func (m Model) ticked(msg tickMsg) (tea.Model, tea.Cmd) {
	if msg.chain != m.chain {
		return m, nil
	}
	now := m.now()
	m.traffic = m.traffic.tidied(now)
	m.changes = m.changes.ticked(m.doc, now, m.cfg.Policy)
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

// stepSlack sets a frame drawn for what moves a step at a time just past the
// step it waits for, so the wall clock it reads, which can run a little
// behind the timer that brings it, has turned over.
const stepSlack = time.Millisecond

// startFrames arms the next frame, while anything on screen moves at now,
// as nextFrame has it, unless the one armed already comes as soon. One armed
// to come within frameEvery stands without asking nextFrame, which lays a
// whole frame out.
func (m Model) startFrames(now time.Time) (Model, tea.Cmd) {
	if m.framing && !m.frameDue.After(now.Add(frameEvery)) {
		return m, nil
	}
	next, moving := m.nextFrame(now)
	if !moving || m.framing && !m.frameDue.After(now.Add(next)) {
		return m, nil
	}
	m.frames++
	m.framing, m.frameDue = true, now.Add(next)
	return m, m.after(next, frameMsg{gen: m.frames})
}

// framed takes in a frame: the one armed last, which startFrames follows
// with the next while anything still moves; or one armed before something
// moved sooner, which is dropped. Neither is held to the wall clock, which
// can read a little behind the timer that brought it.
func (m Model) framed(msg frameMsg) Model {
	if msg.gen == m.frames {
		m.framing = false
	}
	return m
}

// nextFrame is how long after now what moves on screen next changes, and
// whether anything does: nothing while a card is open, as the page behind it
// stands still; else frameEvery while anything moves frame by frame, a bar
// easing to its reading, the highlight on an event, or on a card's state,
// fading back, or what travels the cords, as Motion says; else as soon as
// the shimmer down a cord steps on, or the sand in a card's hourglass falls
// a grain, on the clock's steps of each, or a move yet to show re-patches
// Sessions' calls. The cards' backs and Sessions' plain list are drawn again
// as the stream tells of more, and as the clock ticks.
func (m Model) nextFrame(now time.Time) (time.Duration, bool) {
	if m.open != nothingOpen {
		return 0, false
	}
	moving := m.frame(now).Motion(m.doc, now)
	if moving.Smooth || m.ease.moves(m.doc) && !m.ease.done(now) || m.news.faded(now) != nil || m.changes.faded(now) != nil {
		return frameEvery, true
	}
	var next []time.Duration
	if moving.Shimmer {
		next = append(next, untilStep(now, shimmerStep)+stepSlack)
	}
	if moving.Falling {
		next = append(next, untilStep(now, dashboard.FallStep)+stepSlack)
	}
	if shows, ok := m.traffic.showing(now); ok && m.view == dashboard.Sessions {
		next = append(next, shows.Sub(now)+stepSlack)
	}
	if len(next) == 0 {
		return 0, false
	}
	return slices.Min(next), true
}

// untilStep is how long after now the clock passes its next whole step of
// step.
func untilStep(now time.Time, step time.Duration) time.Duration {
	return step - time.Duration(now.UnixNano()%int64(step))
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

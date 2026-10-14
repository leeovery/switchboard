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
	// WindowName names a window by its key wherever the Overview names one,
	// as "5-hour" or "Fable week": nil names it by its label in the
	// document.
	WindowName func(key string) string
	// Accounts are the configured accounts, whose plans History prices its
	// worth against, and WeekStarts the day its calendar weeks start on.
	Accounts   config.Accounts
	WeekStarts time.Weekday
	// WeekWindow is the key of the window a week long every model shares,
	// the accounts' own weeks, which History's Weeks counts; and Family reads
	// a model's family from its id, for one the version table doesn't name.
	WeekWindow string
	Family     func(model string) string
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
	// it's one there is; Sections the Overview's sections they keep shown;
	// By how they keep its windows grouped, else as the config's [usage] by
	// has it; History what they keep History's views showing, as
	// dashboard.HistoryKept reads it; and Log the Log's columns they keep
	// shown, as dashboard.LogKept reads them. Prefs keeps each as it
	// changes: nil keeps nothing.
	View     dashboard.View
	Sections dashboard.Sections
	By       dashboard.By
	History  dashboard.HistoryPage
	Log      dashboard.LogTicks
	Prefs    Prefs
	// AccountsShown is what the preferences keep Accounts showing, as
	// dashboard.AccountsKept reads it, which Prefs keeps as it changes.
	AccountsShown dashboard.AccountsShown
	// TraceScale is the requests a minute that fill a trace's height on the
	// Overview: 0 for the live rule, as the dashboard has it, the busiest
	// column of every trace on the page, never less than 8.
	TraceScale float64
	// Ledger, Readings and Events read the request ledger, the readings
	// history and the router's events where they lie: the watch opens none
	// of their files itself.
	Ledger   views.Ledger
	Readings views.Readings
	Events   views.Events
	// LedgerKeep is how long the ledger keeps a day's lines, as [ledger]
	// keep gives it, which says the Log's oldest day, as views.OldestKept
	// reads it, and to when an ended session's page says its lines are kept,
	// as views.HoldPage reads it: dayfile.Forever keeps them for good, and
	// none, as a Config that doesn't say, config.DefaultLedgerKeep, as the
	// router keeps them.
	LedgerKeep time.Duration
	// Prices are the price table a move's cost is worked out at, the
	// config's prices in place of any of the table's.
	Prices ledger.Table
	// Home is the user's home directory, which a session's page shows a
	// directory under as ~: "" shows it whole.
	Home string
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

	// doc is the document on screen, and asRead what the watch reads of it
	// as it was read.
	doc    status.Document
	asRead asRead
	// taking is the message Update is taking in, while it takes it: zero
	// outside Update.
	taking taking
	// updated is when doc arrived: zero until one has.
	updated time.Time
	// plan is when the next reads are due, and what they ask for.
	plan plan
	// fetching is set while a read is under way.
	fetching bool
	// again asks for a look at the router's document as soon as the read
	// under way lands.
	again bool
	// lost is when the router was first found not answering, while the
	// dashboard shows its last document, or has read nothing since: zero once
	// a read lands.
	lost time.Time
	// failed is why the last read failed, while it's the last: "" once one
	// lands, and for a look that found the router gone.
	failed string

	// ordering is set while the router carries out an order the routing
	// card gave.
	ordering bool
	// note says what the last key did, till noteUntil: zero while it's held
	// for a card covering the page, as noting holds it.
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
	// of the accounts' states; numbers what reads and the request stream
	// changed of the numbers on the page.
	news    news
	changes changes
	numbers numbers
	// history is how the accounts' windows have been used, and trails the
	// charts' share of it, of the document on screen.
	history history
	trails  dashboard.Trails
	// sessions are the sessions the router listed with the document on
	// screen, for the sessions running and Sessions' calls: nil where it
	// listed none. listedBy is the router that listed them, as its health
	// check said; and order is the order Sessions' calls ran in as they were
	// listed, which they keep from look to look.
	sessions []status.Session
	listedBy router.Health
	order    dashboard.Order
	// stream is the watch's hold on the router's request stream, traffic
	// what the stream has told of the routed requests, and traces the
	// requests the Overview's traces count, of today's lines and the stream.
	stream  streaming
	traffic traffic
	traces  views.Traces
	// toldBefore is what the stream had told before the message taken in
	// told what can change a number the page draws, for compared to compare
	// the numbers drawn of it with those drawn since: nil where it told
	// nothing that can.
	toldBefore *toldSoFar
	// sections are the Overview's sections shown, and by how its windows are
	// grouped; span is how far ahead Runway looks.
	sections dashboard.Sections
	by       dashboard.By
	span     dashboard.Span
	// layout is the Overview's layout as it was last made, kept from look to
	// look, as keepLayout keeps it, so it changes only on an input the page
	// acts on, a key, a click or the wheel, a resize, or a section's shape
	// changing.
	layout dashboard.Layout
	// scroll is how many rows the view shown is scrolled down by, where it
	// doesn't fit, or on the Overview, how many accounts USAGE is, within its
	// section.
	scroll int
	// focus is the account the Overview's focus is on, which GRAPHS follows:
	// the first, in the config's order, till ↑ and ↓ move it, and "" while
	// the document has none.
	focus string
	// accounts is what the watch holds of Accounts: what it shows, and its
	// page, built of the days past holds.
	accounts accountsTab
	// past is what the watch holds of History: its page, the ledger's days
	// it's built of, and the reads of them, which Accounts shares.
	past historyTab
	// session is the session's page open, drawn in place of the view: nil
	// while none is. opening counts the pages asked to open, each opening
	// once its read lands, but one asked for before the last; and pending is
	// the id of the last, while its read waits for the ledger's tail's.
	session *sessionShown
	opening int
	pending string
	// log is what the watch holds of the Log, and tail today's lines of the
	// ledger as its readers follow them.
	log  logTab
	tail ledgerTail

	// open is the card open over the page, if any: the routing card, its
	// picker as routing holds it, or help, scrolled as help holds it. page is
	// the page as it stood when the card opened, which stands still behind
	// it.
	open    opened
	routing routing
	help    helping
	page    frozen

	// shown is what render made of the screen once the last message was
	// taken in.
	shown shown
}

// shown is the screen as the last message left it, for View to show, and
// what moves on it, which has frames drawn: the live page, its sections built
// once at the moment the message was taken in, as at says, which View draws
// at that moment; and where render drew it, to know what moves, as drawn
// says, the view drawn, which View shows rather than drawing it again.
// Behind a card, and on a screen blank till the terminal says what its
// background is, there's no page, and View draws the screen itself.
type shown struct {
	page   *dashboard.Frame
	at     time.Time
	moving dashboard.Motion
	view   tea.View
	drawn  bool
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
	// logCardOpen is the details of the Log's line chosen, and logPanelOpen
	// the panel of what its table shows.
	logCardOpen
	logPanelOpen
	// turnCardOpen is the Log's details of the request chosen of a session's
	// turn open, over its page.
	turnCardOpen
)

// frozen is the page as it stood when a card opened over it, or the Log's
// details last stepped to another line, the table scrolling as the lit line
// follows: its frame and the document it was drawn of, at the moment it was
// drawn. Only its size and its look follow the terminal's while the card is
// open, so nothing behind the card moves, the clock included, but at a key.
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
		choice: cfg.Choice, pair: cfg.Pair, view: opening(cfg.View), sections: cfg.Sections, by: cfg.By,
		accounts: accountsTab{shown: cfg.AccountsShown}, past: historyTab{page: cfg.History, starts: cfg.WeekStarts},
		log: openingLog(cfg.Log),
	}
	m.accounts.shown.Accounts, m.past.page.Accounts = cfg.Accounts, cfg.Accounts
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

// Update takes in a message, at the one time it came, as taking has it, as
// take says, building the session's page open afresh, as relived says, and
// closing a session's card it leaves no p for, as closing says, then has
// the watch read the router's request stream while it wants it, and the
// ledger's days for History, Accounts, a session's page, the Log and its
// find as they want them, History's and Accounts' pages built of them as
// historyBuilt and accountsBuilt have it, Detail's selection following its
// sessions, as followed has it, and the Log's page built of what it holds,
// and only then, price the moves the routing card would make while it wants
// them.
// Then, of the page as it stands at that moment, its Overview's sections
// built once for each to share, it keeps the Overview's layout, but for a
// frame, which takes nothing in to lay it out afresh by; brightens the
// numbers what the stream told changed, as compared has it; and only then,
// so the screen and its frames see the layout the page will draw and the
// numbers it brightens, shows the page, drawing frames while anything on it
// moves. The view is drawn again after each, which is all a note's lapsing
// asks.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	now := m.now()
	m.taking = taking{at: now, moment: m.doc.HoldAt(now, m.cfg.Policy)}
	next, cmd := m.take(msg)
	tuned, closed := next.(Model).relived(msg, now).closing()
	tuned, stream := tuned.tune()
	tuned, past := tuned.readHistory()
	tuned = tuned.historyBuilt().accountsBuilt().followed(msg)
	tuned, paged := tuned.readPage()
	tuned, logged := tuned.readLog()
	tuned, found := tuned.readFind()
	tuned, costs := tuned.costing()
	page := tuned.frame(now).Built(tuned.doc, now)
	if _, frame := msg.(frameMsg); !frame {
		tuned = tuned.keepLayout(page, now)
	}
	tuned = tuned.compared(page, now)
	tuned, frames := tuned.startFrames(page, now)
	tuned.taking = taking{}
	return tuned, tea.Batch(cmd, closed, stream, past, paged, logged, found, costs, frames)
}

// taking is a message as Update takes it in: when it came, which every look
// within it reads as now, as now has it, so they're all of one moment; and
// the moment of the document on screen then, worked out the first time a
// look asks, for every look at it then to share, as frame and moment have
// it, the document a read finds taking it over.
type taking struct {
	at     time.Time
	moment *status.HeldMoment
}

// keepLayout keeps the Overview's layout as page, built at now, lays it out,
// for the looks after: as it was kept, but laid out afresh where an input
// the page acts on let it go, as acting does, on a resize, or as a section's
// shape changes, as LayoutOf has it.
func (m Model) keepLayout(page dashboard.Frame, now time.Time) Model {
	m.layout = page.LayoutOf(m.doc, now)
	return m
}

// laidOut is page, built at now, as the model stands since: its Overview
// laid out as kept, and its numbers as bright as they are at now, neither of
// which its sections draw.
func (m Model) laidOut(page dashboard.Frame, now time.Time) dashboard.Frame {
	page.Layout, page.Brightened = m.layout, m.numbers.brightened(now)
	return page
}

// acting is m as the page acts on an input, its Overview's layout let go,
// to be laid out afresh: whichever input asks, a key, a click or the wheel,
// so the page is laid out alike whichever way it's asked.
func (m Model) acting() Model {
	m.layout = dashboard.Layout{}
	return m
}

// shownOf is the screen page, built at now, shows, for View to draw: the
// live page as the model stands since; but no page behind a card, nor on a
// screen blank till the terminal says what its background is.
func (m Model) shownOf(page dashboard.Frame, now time.Time) shown {
	if m.open != nothingOpen || m.blank() {
		return shown{}
	}
	live := m.laidOut(page, now)
	return shown{page: &live, at: now}
}

// shownPage is the live page shown, as it's drawn: its key line listing the
// keys that work on it as the message was taken in.
func (m Model) shownPage() dashboard.Frame {
	page := *m.shown.page
	page.KeyLine = m.keyLine(page, m.shown.at)
	return page
}

// render works out what moves on the page shown, for frames to be drawn
// while anything does. Where anything it draws may be settling, it draws the
// screen once to know, and keeps it for View to show; else what moves needs
// no drawing, and View draws the page. Nothing moves where there's no page.
func (m Model) render() Model {
	switch page, at := m.shown.page, m.shown.at; {
	case page == nil:
	case !page.MaySettle():
		m.shown.moving = page.Motion(m.doc, at)
	default:
		live := m.shownPage()
		lines, moving := live.Rendered(m.doc, at)
		m.shown.view, m.shown.moving, m.shown.drawn = m.viewOf(lines, live.Look), moving, true
	}
	return m
}

// take takes in a message: a key, a click, the wheel, a resize, the
// terminal's background, the themes listed for the picker, or the choice
// kept as it stands, a read, the router's history, the ledger's days read
// for History, the router carrying out an order, the sessions listed to
// price the routing card's moves, the request stream opening, telling of
// requests or falling due to be asked for again, a tick, a frame, or a
// session's page read.
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
		m.trails = m.history.drawn(m.moment(m.now()))
		return m, nil
	case filedMsg:
		m.history = m.history.filedAs(msg)
		m.trails = m.history.drawn(m.moment(m.now()))
		return m, nil
	case yearReadMsg:
		return m.yearRead(msg), nil
	case sinceMsg:
		return m.sinceRead(msg), nil
	case tailedMsg:
		return m.tailed(msg), nil
	case logReadMsg:
		return m.logRead(msg), nil
	case findReadMsg:
		return m.findRead(msg), nil
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
	case joinedMsg:
		return m.joinedBy(msg), nil
	case tickMsg:
		return m.ticked(msg)
	case frameMsg:
		return m.framed(msg), nil
	case sessionReadMsg:
		return m.sessionRead(msg), nil
	}
	return m, nil
}

// View draws the dashboard full screen as the last message left it: as
// render drew it, where it did; else its live page, at the moment the
// message was taken in, its sections as they were built then; else, as
// behind a card, as screen has it at the clock's time. In colour, it's drawn
// once the terminal has said what its background is, or had its time to, so
// it never shows one theme then another.
func (m Model) View() tea.View {
	switch {
	case m.shown.drawn:
		return m.shown.view
	case m.shown.page != nil:
		live := m.shownPage()
		return m.viewOf(live.Draw(m.doc, m.shown.at), live.Look)
	case m.blank():
		return blankView()
	}
	f, doc, at := m.screen(m.now())
	return m.viewOf(f.Draw(doc, at), f.Look)
}

// blank reports whether the screen is blank: in colour, till the terminal
// has said what its background is, or had its time to.
func (m Model) blank() bool {
	return m.coloured() && !m.backdrop.settled
}

// blankView is the screen full and blank.
func blankView() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	return v
}

// viewOf is the screen of the lines drawn in look, full, the theme picker
// over its right while it's open: in the theme shown, its canvas painted on
// every cell, and set as the terminal's background too, as background says.
// The mouse is on while it runs, for clicks and the wheel, so the terminal's
// own selecting takes its modifier.
func (m Model) viewOf(lines []string, look dashboard.Look) tea.View {
	if m.picker.open {
		lines = m.picker.drawn(m.choice).Over(lines, m.size.Width, look)
	}
	v := blankView()
	v.SetContent(strings.Join(lines, "\n"))
	v.BackgroundColor = m.background(look)
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// pressed acts on a key: ctrl+c quits, whatever's open. While a card is
// open, its keys alone act, as routingKey, helpKey, logCardKey, logPanelKey
// and turnCardKey take them, as do the find's while the Log's is open. Else q
// quits; while the theme picker is open, it takes every other key; and
// otherwise p opens the routing card, ? opens the page's help, R refreshes,
// and the rest are a session's page's, while one is open, as sessionKey
// takes them, else the page's, as pageKey takes them.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := key.String(); {
	case k == "ctrl+c":
		return m, tea.Quit
	case m.open == routingOpen:
		return m.routingKey(k)
	case m.open == helpOpen:
		return m.helpKey(k)
	case m.open == logCardOpen:
		return m.logCardKey(k)
	case m.open == logPanelOpen:
		return m.logPanelKey(k)
	case m.open == turnCardOpen:
		return m.turnCardKey(k)
	case m.finding():
		return m.logFindKey(k)
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
	case m.session != nil:
		return m.sessionKey(key)
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
	f.KeyLine = m.keyLine(f, now)
	return f
}

// card is the card open over the page, as the model stands at now: the
// routing card, its figures read afresh; the Log's details or its panel; the
// Log's details of a session's turn open's request; or help.
func (m Model) card(now time.Time) dashboard.Opened {
	switch m.open {
	case routingOpen:
		return m.routingCard(now)
	case logCardOpen:
		return m.logCard()
	case logPanelOpen:
		return m.logPanel(now)
	case turnCardOpen:
		return m.turnCard()
	}
	return m.helpCard()
}

// cover opens the card given over the page, which stands still behind it as
// it stands now, dropping a session's page asked to open, as unasked has it,
// so no card opens over one page to stand over another.
func (m Model) cover(card opened) Model {
	now := m.now()
	m.open, m.page = card, frozen{frame: m.live(now), doc: m.doc, at: now}
	return m.unasked()
}

// uncover closes the card open over the page, which is live again, and
// starts the time of a note held while it was covered, as seen has it.
func (m Model) uncover() (Model, tea.Cmd) {
	m.open, m.page = nothingOpen, frozen{}
	return m.seen()
}

// frame is the dashboard's frame as the model stands at now, but for its key
// line and any card over it: at the size drawn at, in the look, showing the
// tab shown, with what the watch knows of the router, its events and its
// history, the numbers brightening, the Overview's sections shown, how its
// windows are grouped, the requests its traces count and its layout as
// kept, how far ahead Runway looks, the sessions listed, what the request
// stream tells of their requests and the order Sessions' calls keep, how far
// the view is scrolled, the account the focus is on, and what Accounts, as
// standing has it, and History show. It holds the one moment of the
// document at now, as At has it, for its bars eased to it and every look at
// it to share: the message's, as taking has it, where now is when the
// message came.
func (m Model) frame(now time.Time) dashboard.Frame {
	f := dashboard.Frame{
		Width: m.size.Width, Height: m.size.Height, Look: m.look(),
		View: m.view,
		Lost: m.lostShown(), Failed: m.failure(), Outdated: m.history.outdated, Fresh: m.news.faded(now), Changed: m.changes.faded(now), StateChanged: m.changes.wordFaded(now), TurnedOut: m.changes.turnedOut(now), Trails: m.trails,
		Brightened: m.numbers.brightened(now), Sections: m.sections, By: m.by, Traces: m.traces, TraceScale: m.cfg.TraceScale, Layout: m.layout, Span: m.span,
		Sessions: m.sessions, Stream: m.traffic.stream(now), Traffic: m.traffic.at(now), Order: m.order, Scroll: m.scroll,
		Focus:      m.focus,
		Policy:     m.cfg.Policy,
		WindowName: m.cfg.WindowName,
		WeekWindow: m.cfg.WeekWindow,
		Accounts:   m.accounts.shown,
		History:    m.past.page,
		Session:    m.session.frame(),
		Log:        m.log.page,
	}.Holding(m.taking.moment).At(m.doc, now)
	if m.view == dashboard.Accounts {
		f.Accounts = m.accounts.standing(f)
	}
	f.Eased = m.ease.eased(f, m.doc, now)
	return f
}

// resized takes in the size the terminal gives, keeping the size drawn at for
// a width or height it gives as zero; and closes the theme picker should it
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

// fetch reads the source as r asks, as fetchFrom reads it, asked now.
func (m Model) fetch(r status.Read) tea.Cmd {
	ctx, source, policy, asked := m.ctx, m.cfg.Source, m.cfg.Policy, m.now()
	return func() tea.Msg {
		return fetchFrom(ctx, source, r, policy, asked)
	}
}

// fetchFrom reads the source as r asks, and from a router, as the moment of
// what it read as it was asked says, as policy judges it, the sessions it
// lists, for the sessions running and Sessions' calls: a router that can't
// list them leaves them out.
func fetchFrom(ctx context.Context, source Source, r status.Read, policy score.Policy, asked time.Time) fetchedMsg {
	doc, from, err := source.Read(ctx, r)
	msg := fetchedMsg{read: r, doc: doc, router: from, err: err}
	if err != nil || !doc.At(asked, policy).Routed {
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
// router lost, as lose does. Before any document, from a source that asks the
// router first, as awaitsRouter has it, the status line says the latest
// read's news either way: a question after the router, or a look at its
// document, that found it gone has no why of its own, and clears the last
// read's, the status line saying the router isn't answering, since it was
// first found gone; and a read that failed for a reason of its own has the
// status line say that reason, as lostShown has it. Elsewhere a read that
// found the router gone leaves the last read's why standing, as nothing new
// was found. Each read starts a new chain of ticks at the pace it calls for,
// lets a look at the router's document asked for while it was under way go
// ahead, and as any look does, asks for the ledger's days since History's
// were read, as readHistory reads them while History or Accounts shows.
func (m Model) fetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	m.fetching, m.past.due, m.tail.due = false, true, true
	var shown tea.Cmd
	switch {
	case errors.Is(msg.err, status.ErrNoRouter):
		m.plan = m.plan.missed(now)
		if m.awaitsRouter() {
			m.failed = ""
		}
		m = m.lose(now)
	case msg.err != nil:
		m.plan = m.plan.failed(m.doc, now)
		m.failed = redact.Text(msg.err.Error())
		logger.Warn("usage read failed", "error", msg.err, "next", m.plan.due)
		m = m.lose(now)
	default:
		look := msg.doc.At(now, m.cfg.Policy)
		m.plan = m.plan.landed(msg.read, msg.doc, look.Routed, now)
		m, shown = m.show(msg, look)
		logRead(msg, look, m.plan.due)
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
// couldn't, as look has them, and when the next full read is due. A look at
// the router's document as it stands, every few seconds, is noted only at
// debug.
func logRead(msg fetchedMsg, look status.Moment, next time.Time) {
	var read, failed []string
	for _, a := range look.Accounts {
		if a.Read {
			read = append(read, a.ID)
		} else {
			failed = append(failed, a.ID)
		}
	}
	level := slog.LevelInfo
	if look.Routed && msg.read.Refresh == 0 {
		level = slog.LevelDebug
	}
	logger.Log(context.Background(), level, "usage read", "source", sourceOf(look),
		"read", strings.Join(read, ","), "failed", strings.Join(failed, ","), "next", next)
}

// show puts the document a read found on screen, as look has it at its moment,
// which every look at it within the message shares, as taking has it, with
// the sessions the router listed, Sessions' calls keeping the order they ran
// in, each move they show done no longer re-patching them; or where it
// couldn't list them, those it listed last, while the router that listed them
// answers, as a router restarted or replaced lists its own; and none while
// probing. It posts what the change calls for, unless the router is there to
// post its own, as nothing is to be told twice; follows the router as it goes
// and comes back; notes the events new to it, and the readings it gives; asks
// the router for its history with a full read, and as the router answers
// again, or another router does, as one restarted, or probing, reads the
// readings history's files with the first full read since probing began; keeps
// the focus where its account still is, else puts it on the first; eases the
// bars to it from where they stand; and brightens each number on the page it
// changed.
func (m Model) show(msg fetchedMsg, look status.Moment) (Model, tea.Cmd) {
	doc, now, read := msg.doc, look.Now, originOf(look, msg.router)
	was := m.numbersDrawn(now)
	var post, asked tea.Cmd
	if !look.Routed {
		post = m.post(m.readings.alerts(look, read, m.cfg.Notifications), probedAsAsked(look))
	}
	ask := look.Routed && (full(msg.read) || !m.answering() || m.news.another(msg.router))
	if look.Routed {
		m = m.answeredAgain(msg.router)
		m.history.filedSince = false
	}
	m.readings = m.readings.with(look, read)
	m = m.follow(look)
	m.news = m.news.looked(look, msg.router)
	m.changes = m.changes.looked(look, msg.router)
	m.history = m.history.saw(look)
	m.ease = easing{from: m.ease.standing(m.frame(now), m.doc, now), start: now}
	m.doc, m.asRead, m.taking.moment, m.updated, m.failed = doc, asReadOf(look), look.Held(), now, ""
	switch {
	case msg.listed || !look.Routed:
		m.order = m.frame(now).OrderOf(doc)
		m.sessions, m.listedBy = msg.sessions, msg.router
		m.traffic = m.traffic.listedAs(msg.sessions)
	case !msg.router.Same(m.listedBy):
		m.sessions = nil
	}
	m = m.stillThere(look)
	m.trails = m.history.drawn(look)
	m = m.lookedLog(look, msg.router)
	m.numbers = m.numbers.looked(was, m.numbersDrawn(now), read, now)
	switch {
	case ask:
		m, asked = m.askHistory(look)
	case !look.Routed && full(msg.read) && !m.history.filedSince:
		m, asked = m.readFiled(look)
	}
	return m, tea.Batch(post, asked)
}

// follow notes where the document look is a moment of came from, against
// where the document on screen did, as the router stops answering or answers
// again; and, a document having landed from whatever source, that the router
// is lost no more.
func (m Model) follow(look status.Moment) Model {
	switch was, is := m.routed(), look.Routed; {
	case was && !is:
		logger.Warn("the router stopped answering; probing directly", "router", look.Fallback.Router, "reason", look.Fallback.Reason)
	case is && (!was || !m.lost.IsZero()):
		logger.Info("reading the router")
	}
	m.lost = time.Time{}
	return m
}

// lose notes a read at now that failed, as the router not answering it:
// while its document is on screen, which stays there, the status line
// saying since when the router hasn't answered, until a read lands, as a read
// then asks the router first, failing only where it doesn't answer; and
// before any document, the status line saying that alone, unless the source
// probes as asked, with no router to lose, or a read has failed for a reason
// of its own, as a probe can, which the status line says instead. A read that
// fails while probing says nothing of the router: the status line says why
// it failed.
func (m Model) lose(now time.Time) Model {
	switch {
	case !m.lost.IsZero():
		return m
	case m.routed():
		logger.Warn("the router stopped answering; showing its last document")
	case m.awaitsRouter() && m.failed == "":
		logger.Warn("the router isn't answering, and nothing has been read")
	default:
		return m
	}
	m.lost = now
	return m
}

// lostShown is when the router was lost, for the status line: zero before
// any document while the latest read failed for a reason of its own, which
// the status line says instead, keeping when the router was first found gone
// for a later read that finds it gone again.
func (m Model) lostShown() time.Time {
	if m.awaitsRouter() && m.failed != "" {
		return time.Time{}
	}
	return m.lost
}

// awaitsRouter reports whether nothing has been read, from a source that asks
// the router first: a read that then finds the router gone has the status
// line say it isn't answering.
func (m Model) awaitsRouter() bool {
	return m.updated.IsZero() && !m.cfg.ProbeOnly
}

// failure is why the last read failed, and when the next is due, for the
// status line: zero where it didn't fail.
func (m Model) failure() dashboard.Failure {
	if m.failed == "" {
		return dashboard.Failure{}
	}
	return dashboard.Failure{Reason: m.failed, Next: m.plan.due}
}

// ticked reads again once a read is due. Every tick redraws, as every message
// does, so countdowns and the clock stay live.
func (m Model) ticked(msg tickMsg) (tea.Model, tea.Cmd) {
	if msg.chain != m.chain {
		return m, nil
	}
	now := m.now()
	m.traffic = m.traffic.tidied(now)
	m.changes = m.changes.ticked(m.moment(now))
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

// startFrames shows the screen of page, built at now, as shownOf has it, in
// place of whatever a message before left, and arms the next frame, while
// anything on screen moves at now, as nextFrame has it once render has worked
// out what does, unless the one armed already comes as soon, as one armed to
// come within frameEvery does, which asks nothing of render.
func (m Model) startFrames(page dashboard.Frame, now time.Time) (Model, tea.Cmd) {
	m.shown = m.shownOf(page, now)
	if m.framing && !m.frameDue.After(now.Add(frameEvery)) {
		return m, nil
	}
	m = m.render()
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
// stands still; else frameEvery while anything moves frame by frame on the
// screen render drew at now: what travels the cords, or on the page, a bar
// easing to its reading, or a number brightened, or the cue on an event, an
// account's state or a window's words, fading back; else as soon as the
// shimmer down a cord steps on, on the clock's steps of it, or a move yet to
// show re-patches Sessions' calls. Sessions' plain list is drawn again as
// the stream tells of more, and as the clock ticks.
func (m Model) nextFrame(now time.Time) (time.Duration, bool) {
	if m.open != nothingOpen {
		return 0, false
	}
	moving := m.shown.moving
	if moving.Smooth {
		return frameEvery, true
	}
	var next []time.Duration
	if moving.Shimmer {
		next = append(next, untilStep(now, shimmerStep)+stepSlack)
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
	return m.asRead.routed
}

// asRead is what the watch reads of a document as it was read, which holds
// whatever the time: whether it's the router's, how many accounts it has,
// and whether a pin stands.
type asRead struct {
	routed   bool
	accounts int
	pinned   bool
}

// asReadOf is what the watch reads of the document look is a moment of, as
// it was read.
func asReadOf(look status.Moment) asRead {
	return asRead{routed: look.Routed, accounts: len(look.Accounts), pinned: !look.Pin.IsZero()}
}

// probedAsAsked reports whether the document look is a moment of was built by
// probing as asked, rather than for want of the router, which may be running
// all the same: one probed for want of it says why.
func probedAsAsked(look status.Moment) bool {
	return !look.Routed && look.Fallback == (status.Fallback{})
}

// sourceOf names where the document look is a moment of came from, as its log
// says: the router, probing, or "" before any document.
func sourceOf(look status.Moment) string {
	switch {
	case look.Routed:
		return status.SourceRouter
	case look.Probed:
		return status.SourceProbe
	default:
		return ""
	}
}

// now is the time the message Update is taking in came, as taking has it;
// else it reads the wall clock without its monotonic reading, which stops
// while a Mac sleeps: kept, it would put every deadline off by the time spent
// asleep.
func (m Model) now() time.Time {
	if !m.taking.at.IsZero() {
		return m.taking.at
	}
	return m.cfg.Now().Round(0)
}

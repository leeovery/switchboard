package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionShown is what the watch holds of a session's page while it's open:
// the session's id; its page as its lines build it, held between looks, as
// views.HeldPage holds it, and how many reads of the ledger's tail, and of
// its turn open's lines, have changed it; when each day it awaits a line on
// was last read for it; the page as the frame shows it, built afresh of
// what's held as relived builds it, what it was last built of, none before
// it's built, and until when that stands by the clock, as views.BuiltPage's
// Until has it; the Log's details card over it, while it's open; and how
// many turns it has opened, for a read of the lines of one opened before
// the last to open none.
type sessionShown struct {
	id      string
	held    views.HeldPage
	taken   int
	awaited awaitedReads
	shown   dashboard.SessionShown
	built   *pageBasis
	until   time.Time
	card    turnCarding
	turns   int
	// asked is the open that asked for it, from 1, as sessionReadMsg's
	// asked has it: 0 while its read is under way, as it loads.
	asked int
	// turnWaited is set once the read of the lines of its turn open has
	// been waited on LoadingAfter, as turnLoading has it; and wholeAsked
	// once a read of the ledger's tail was asked for at once, its session
	// to be read whole, as relived has it, till a page is built again.
	turnWaited bool
	wholeAsked bool
}

// sessionReadMsg is a read of the ledger for a session's page, the asked'th
// asked to open: the page of the session with the given id, as its lines
// build it, held.
type sessionReadMsg struct {
	asked int
	id    string
	held  views.HeldPage
}

// frame is the page s holds as the frame draws it: nil while none is open.
func (s *sessionShown) frame() *dashboard.SessionShown {
	if s == nil {
		return nil
	}
	shown := s.shown
	return &shown
}

// openSession opens the page of the session with the given id, as a click on
// its row does, once its lines are read, as readAsked reads them: till then
// the tab it opens over shows, and acts, as it is, but where they've taken
// LoadingAfter, when the page shows loading, as pageLoading has it.
func (m Model) openSession(id string) (Model, tea.Cmd) {
	m.opening, m.pending = m.opening+1, id
	m, read := m.readAsked()
	return m, tea.Batch(read, m.after(LoadingAfter, pageLoadingMsg{asked: m.opening, id: id}))
}

// readAsked reads the lines of the page asked to open last, as readSession
// reads them, once no read of the ledger's tail is under way, holding the
// tail's reads off till it lands: so the mark never moves past where its
// read of today's lines ends, the page yet to take what lies between. What
// it reads past the mark comes again at the tail's next read, and the page
// takes each line once. Till then it waits.
func (m Model) readAsked() (Model, tea.Cmd) {
	if m.pending == "" || m.tail.reading {
		return m, nil
	}
	id := m.pending
	m.pending, m.tail = "", m.tail.wholeAsked()
	return m, m.readSession(m.opening, id)
}

// readSession reads, as a command, the page the asked'th open asked for: the
// lines of the session with the given id, and every account's requests on
// its days, as views.ReadPage reads them through the ledger at now, pinned
// as the stream tells, priced at the config's prices and kept as [ledger]
// keep keeps them, as HoldPage reads it; none, where the watch reads no
// ledger.
func (m Model) readSession(asked int, id string) tea.Cmd {
	l, prices, keep, now := m.cfg.Ledger, m.cfg.Prices, m.cfg.LedgerKeep, m.now()
	stream := m.traffic.stream(now)
	return func() tea.Msg {
		if l == nil {
			return sessionReadMsg{asked: asked, id: id, held: views.HoldPage(id, views.SessionsRead{}, prices, keep, now)}
		}
		return sessionReadMsg{asked: asked, id: id, held: views.ReadPage(l, id, stream, prices, keep, now)}
	}
}

// sessionRead takes in a page's read, the tail's reads no longer held off
// by it, which opens it, in place of its page loading, but one of an open
// asked for before the last, or dropped since, as unasked has it: its
// session taken into the store, as heldWith has it, the page holding it as
// the store does where that's newer, as views.HeldPage's Held has it; the
// page is built as relived builds it, its directory shown under the home the
// watch is given.
func (m Model) sessionRead(msg sessionReadMsg) Model {
	m.tail = m.tail.wholeLanded()
	m.arrived = max(m.arrived, msg.asked)
	if msg.asked == m.opening {
		m = m.heldWith(msg.held.Found())
		held := msg.held.Joined(m.held)
		m.session = &sessionShown{id: msg.id, held: held, shown: dashboard.SessionShown{Home: m.cfg.Home}, asked: msg.asked}
	}
	return m
}

// readPage has the watch read for a session's page as it wants: the page
// asked to open last, as readAsked reads it; and while one is open, at each
// look, what the ledger gained since the tail's mark, as readTail reads it
// for every reader, once no other read of the ledger is under way.
func (m Model) readPage() (Model, tea.Cmd) {
	switch {
	case m.pending != "":
		return m.readAsked()
	case m.session == nil || m.cfg.Ledger == nil || !m.tail.due || m.tail.busy() || m.log.reading:
		return m, nil
	}
	return m.readTail(m.now())
}

// pageFold is what a read of the ledger's tail does for the session's page
// open as it's asked: the page held, of the session with the given id, is
// rolled on to today, its session, its points among it, held as the store
// the read feeds holds it, the days before today the read reads again
// whole, late, among what it feeds it, and its turn open taking in what the
// read read. Awaited is when each day the page awaits a line on was last
// read for it, as the read leaves it.
type pageFold struct {
	id      string
	held    views.HeldPage
	late    []string
	awaited awaitedReads
}

// pageTaken is the page held of the session with the given id, as a read
// of the ledger's tail found it, and whether that changed its page, as
// views.HeldPage's On has it.
type pageTaken struct {
	id      string
	held    views.HeldPage
	changed bool
}

// pageFold is what a read of the ledger's tail at now does for the session's
// page open, nil where none is: its late days those of the days the tail
// missed, as missed has them, it holds not every line of, as Unread has
// them; and those it awaits a line on, as Awaited has them, whose lines
// could have come since they were last read, as due has them, as the Log's
// are; each once, the oldest first.
func (m Model) pageFold(now time.Time) *pageFold {
	s := m.session
	if s == nil || s.asked == 0 {
		return nil
	}
	live := m.pageLive(s.id, now)
	due, awaited := s.awaited.due(s.held.Awaited(live), live.Stream, now, m.tail.at)
	late := slices.Concat(due, s.held.Unread(m.tail.missed(views.Today(now))))
	return &pageFold{id: s.id, held: s.held, late: slices.Compact(slices.Sorted(slices.Values(late))), awaited: awaited}
}

// pageAwaiting is m with the session's page open noting when each day it
// awaits a line on was last read for it, as the read of the tail f folds
// leaves it: none where no page is open.
func (m Model) pageAwaiting(f *pageFold) Model {
	if f == nil {
		return m
	}
	s := *m.session
	s.awaited = f.awaited
	m.session = &s
	return m
}

// taken is the page held as a read of the ledger's tail on the day given
// finds it, rolled on to it.
func (f pageFold) taken(today views.LedgerDay) *pageTaken {
	t := pageTaken{id: f.id}
	t.held, t.changed = f.held.On(today.Date)
	return &t
}

// pageTook is m with the page held as a read of the ledger's tail grew it,
// where the session's page open is still of its session: one closed since
// takes nothing in. It holds the turn open the page open holds, the lines
// the read read taken into it, as views.HeldPage's TurnOf has it, as the
// page the read grew may be the page from before its turn opened; and its
// session as the store the read fed holds it, as Held has it. Its page is
// built afresh where they changed it, and each number on it the lines taken
// in change brightens, as a look's changes do: the points of the windows
// the session took, which only its lines move.
func (m Model) pageTook(t pageTaken, read tailLines) Model {
	if m.session == nil || m.session.id != t.id {
		return m
	}
	now := m.now()
	was := m.numbersDrawn(now)
	s := *m.session
	held, fed := t.held.TurnOf(m.session.held, read.all()...).Held(m.held)
	s.held = held
	if t.changed || fed {
		s.taken++
	}
	m.session = &s
	m = m.relived(t, now)
	m.numbers = m.numbers.changed(was, m.numbersDrawn(now), now)
	return m
}

// relived is m with the session's page open built afresh at now, but for a
// frame, which takes nothing in: of what it holds of the session's lines, of
// the router's listing of it, as pageLive has it, and of what the request
// stream told, as the views build it, its turn open's requests among it,
// made again only as what it's made of changes, as pageBasis has it, or as
// the clock reaches when it was built until; its turn open numbered as
// views.BuiltPage's Open has it, chosen, and saying its requests load only
// once turnLoading has waited on them; its turn chosen as following has it.
// What streams on between, a row in flight and the answer it's
// doing, is drawn as it stands at each frame. A page that can't be built, of
// no request and listed by no router, stays as it was last built, and
// doesn't open where it never was; one whose session is to be read whole,
// as views.BuiltPage's ReadsWhole has it, stays as it was last built too,
// a read of the ledger's tail asked for at once, but once till a page is
// built again. One loading has nothing to build yet.
func (m Model) relived(msg tea.Msg, now time.Time) Model {
	if _, frame := msg.(frameMsg); frame || m.session == nil || m.session.asked == 0 {
		return m
	}
	s := *m.session
	live := m.pageLive(s.id, now)
	live.Stream = live.Stream.Of(s.id)
	if basis := m.pageBasis(live); !s.built.same(basis) || !now.Before(s.until) {
		built, ok := s.held.Build(live)
		if built.ReadsWhole {
			m.tail.due = m.tail.due || !s.wholeAsked
			s.wholeAsked = true
			if s.shown.Page.Session == "" {
				s.shown = dashboard.SessionShown{Page: views.AskedPage(s.id, m.sessions), Home: m.cfg.Home, Loading: true}
			}
			m.session = &s
			return m
		}
		s.wholeAsked = false
		if !ok && s.shown.Page.Session == "" {
			m.session = nil
			return m
		}
		if ok {
			s.shown.Page, s.shown.Story, s.shown.Requests, s.shown.Loading = built.Page, built.Story, built.Requests, false
			s.shown.RequestsLoading = built.RequestsLoading && s.turnWaited
			if built.Open != 0 {
				s.shown.Open, s.shown.Turn = built.Open, built.Open
			}
		}
		s.built, s.until = &basis, built.Until
	}
	s.shown = following(m.session.shown, s.shown, m.turnsScrolling(s.shown))
	m.session = &s
	return m
}

// pageBasis is what a session's page is made of: the lines it holds, as
// taken counts the reads of the ledger's tail that changed its page; what
// it's built of beside them, as views.PageLive's Same compares it, kept as
// Kept keeps it; and its turn open.
type pageBasis struct {
	taken int
	live  views.PageLive
	open  int
}

// same reports whether b is o, the page made of one as of the other.
func (b *pageBasis) same(o pageBasis) bool {
	return b != nil && b.taken == o.taken && b.live.Same(o.live) && b.open == o.open
}

// pageBasis is what the session's page open is made of, of live, what it's
// built of beside its lines.
func (m Model) pageBasis(live views.PageLive) pageBasis {
	s := m.session
	return pageBasis{taken: s.taken, live: live.Kept(), open: s.shown.Open}
}

// pageLive is what a session's page is built of at now beside its lines, as
// Sessions' List is, as listLive has it: the session with the given id as
// the router lists it, or while it doesn't answer, as its last listing had
// it; whether it doesn't; and what the request stream told.
func (m Model) pageLive(id string, now time.Time) views.PageLive {
	return views.PageLive{Routed: views.Routed(m.sessions, id), Unlisted: !m.answering(), Stream: m.traffic.stream(now), Now: now}
}

// turnsScrolling is how far the turns of the page shown scroll, laid out at
// the size m draws at, as TurnsScrolling has it, of the page alone.
func (m Model) turnsScrolling(shown dashboard.SessionShown) dashboard.Scrolling {
	return dashboard.Frame{Width: m.size.Width, Height: m.size.Height, Session: &shown}.TurnsScrolling()
}

// following is the page shown, built afresh, is, as it was before, was: its
// turn chosen the one chosen before, by its number; but the newest, where
// the newest was chosen, or none, with no turn open, so a page follows its
// newest turn till another is chosen, or where the one chosen is no more;
// kept in view, its turns scrolling as s says.
func following(was, is dashboard.SessionShown, s dashboard.Scrolling) dashboard.SessionShown {
	i := turnAt(is, is.Turn)
	if i < 0 || was.Open == 0 && turnAt(was, was.Turn) <= 0 {
		i = 0
	}
	return choosing(is, i, s)
}

// turnAt is the index of the turn numbered n among the page's turns, newest
// first: -1 where it has none such.
func turnAt(shown dashboard.SessionShown, n int) int {
	return slices.IndexFunc(shown.Page.Turns, func(t views.Turn) bool { return t.Turn == n })
}

// choosing is the page shown with its turn at index i, newest first, chosen,
// or the nearest there is, scrolled no further than keeps it in view, with
// the rows under it the page keeps with it, as Kept has them, its turns
// scrolling as s says: none chosen of a page with no turns.
func choosing(shown dashboard.SessionShown, i int, s dashboard.Scrolling) dashboard.SessionShown {
	turns := shown.Page.Turns
	if len(turns) == 0 {
		shown.Turn, shown.Scroll = 0, 0
		return shown
	}
	i = min(max(i, 0), len(turns)-1)
	shown.Turn = turns[i].Turn
	shown.Scroll = s.Keeping(shown.Scroll, i, shown.Kept())
	return shown
}

// sessionKey acts on a key while a session's page is open, as pressed hands
// it on: with a turn open, its keys first, as turnKey has them; esc goes
// back where the page was opened from, its tab as it was left; tab and
// shift-tab show the tab after or before that one, leaving the page, as
// pageKey has them; ↑ and ↓ choose the turn before and after, newest first,
// as moveTurn has it; enter opens the turn chosen, as openTurn has it; j,
// k, PgDn and PgUp scroll the turns, or the turn open's requests, as
// scrollSession has it; t opens the theme picker; and w shows and hides each
// model's own window, as toggleModels has it. The rest do nothing,
// the tab's own keys among them. While it loads, its keys are loadingKey's.
func (m Model) sessionKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.session.shown.Loading {
		return m.loadingKey(key)
	}
	k := key.String()
	if by, ok := scrollKey(k); ok {
		return m.scrollSession(by), nil
	}
	if m.session.shown.Open != 0 {
		if next, cmd, ok := m.turnKey(k); ok {
			return next, cmd
		}
	}
	switch k {
	case "esc":
		return m.closeSession(), nil
	case "tab", "shift+tab":
		return m.pageKey(key)
	case "up":
		return m.moveTurn(-1), nil
	case "down":
		return m.moveTurn(1), nil
	case "enter":
		return m.openTurn()
	case "t":
		return m.openPicker()
	case "w":
		return m.toggleModels(), nil
	}
	return m, nil
}

// loadingKey acts on a key while a session's page loads: esc goes back where
// it was opened from, dropping its read, as closeSession has it; tab and
// shift-tab leave it for the tab after or before, as pageKey has them; and t
// opens the theme picker. The rest do nothing till it's loaded.
func (m Model) loadingKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		return m.closeSession(), nil
	case "tab", "shift+tab":
		return m.pageKey(key)
	case "t":
		return m.openPicker()
	}
	return m, nil
}

// closeSession closes the session's page open, the tab it was opened over
// showing again as it was left, and drops one asked to open, as unasked has
// it.
func (m Model) closeSession() Model {
	m.session = nil
	return m.unasked()
}

// unasked is m with no session's page asked to open: one whose lines are
// still being read opens nothing once they are, as sessionRead has it, and
// one whose read waits is never read.
func (m Model) unasked() Model {
	m.opening, m.pending = m.opening+1, ""
	return m
}

// moveTurn chooses the turn by turns on from the one chosen, newest first, as
// ↑ and ↓ do, no further than the first and the last.
func (m Model) moveTurn(by int) Model {
	return m.turnChosen(turnAt(m.session.shown, m.session.shown.Turn) + by)
}

// turnChosen is m with the turn at index i of its page's, newest first, chosen,
// as choosing has it.
func (m Model) turnChosen(i int) Model {
	s := *m.session
	s.shown = choosing(s.shown, i, m.turnsScrolling(s.shown))
	m.session = &s
	return m
}

// scrollTurns scrolls the page's turns by, as j and k do, no further than
// they go, the turn chosen kept in view: the one nearest it in view, where
// it scrolls out.
func (m Model) scrollTurns(by scrollStep) Model {
	s := *m.session
	turns := m.turnsScrolling(s.shown)
	s.shown.Scroll = by.from(s.shown.Scroll, turns)
	s.shown = choosing(s.shown, inViewAt(turnAt(s.shown, s.shown.Turn), s.shown.Scroll, turns), turns)
	m.session = &s
	return m
}

// clickTurn acts on a click on the row of the turn numbered n: with a turn
// open, it closes it, as esc does, and on another turn, chooses it; else on
// the turn chosen, it opens it, as enter does, and on another, it chooses
// it.
func (m Model) clickTurn(n int) (tea.Model, tea.Cmd) {
	if open := m.session.shown.Open; open != 0 {
		m = m.closeTurn()
		if n == open {
			return m, nil
		}
	} else if n == m.session.shown.Turn {
		return m.pressed(namedPresses["enter"])
	}
	if i := turnAt(m.session.shown, n); i >= 0 {
		return m.turnChosen(i), nil
	}
	return m, nil
}

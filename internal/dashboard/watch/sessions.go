package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionsTab is what the watch holds of Sessions: what it shows, its
// Switchboard and its List as the last look built them; whether the List is
// held, in the store, from its first read, as a view naming the sessions
// running first shows, as namesSessions has it, for the run, fed by the
// ledger's tail; whether that read is under way; when each day the store
// awaits a line on was last read for it; and how many moves the views read
// as the last look built Sessions, for a frame to build it afresh only as a
// move comes to show.
type sessionsTab struct {
	shown   dashboard.SessionsShown
	listed  bool
	reading bool
	awaited awaitedReads
	moves   int
}

// listReadMsg is the List's first read, the sessions it lists held as a
// store of their own, as views.HeldSessions holds them, for the watch's to
// take them in.
type listReadMsg struct {
	held views.HeldSessions
}

// sessionsLooked is m with Sessions built afresh at the look msg takes, while
// it shows with no session's page over it: the calls of the sessions
// running, as views.CallsOf has them, of the List held; who sits where on
// the Switchboard, kept from the look before, as views.Patch keeps it, the
// stubs hanging as views.StubsOf has them; the List, as the List held
// builds it, once it's held; and the choice following its session, as
// Following has it, kept in view where it was, as revealing has it. A frame
// takes nothing in, so builds nothing, but where a move the views read
// comes to show at it, putting its session on the account it went to.
func (m Model) sessionsLooked(msg tea.Msg) Model {
	if m.view != dashboard.Sessions || m.session != nil {
		return m
	}
	now := m.now()
	s := m.traffic.stream(now)
	if _, frame := msg.(frameMsg); frame && len(s.Moves) == m.sessionsTab.moves {
		return m
	}
	sighted := m.sessionsTab.shown.Chosen != (dashboard.Plug{}) && chosenShows(m.frame(now), m.doc, now, m.scroll)
	mo := m.moment(now)
	t := m.sessionsTab
	if t.listed {
		t.shown.List = m.held.List(m.listLive(now, s))
	}
	t.shown.Calls = views.CallsOf(mo, m.listLive(now, s), m.held)
	t.shown.Patched = views.Patch(t.shown.Patched, t.shown.Calls, views.StubsOf(mo))
	t.shown = t.shown.Following(accountIDs(mo))
	t.moves = len(s.Moves)
	m.sessionsTab = t
	if sighted {
		m = m.revealing(now)
	}
	return m
}

// listLive is what the List is built of at now beside its lines: the
// sessions the router lists, or while it doesn't answer, as it last listed
// them, as the Switchboard draws them then; whether it doesn't; and what the
// request stream told, s.
func (m Model) listLive(now time.Time, s views.Stream) views.ListLive {
	return views.ListLive{Running: m.sessions, Unlisted: m.unlisted(), Stream: s, Now: now}
}

// unlisted reports whether no router lists the sessions, those the watch
// holds its last listing: the one value every view's sessions are listed by.
func (m Model) unlisted() bool {
	return !m.answering()
}

// readSessions has the watch read the ledger for the List as it wants,
// while what it shows names the sessions running, as namesSessions has it:
// the List's first read, as readList reads it; and once it's held, at each
// look, what the ledger gained since the tail's mark, as readTail reads it
// for every reader, once no other read of the ledger is under way. While
// nothing reads the ledger, nothing.
func (m Model) readSessions() (Model, tea.Cmd) {
	switch t := m.sessionsTab; {
	case !m.namesSessions() || m.cfg.Ledger == nil || t.reading:
		return m, nil
	case !t.listed:
		return m.readList()
	case !m.tail.due || m.tail.busy() || m.log.reading || m.pending != "":
		return m, nil
	}
	return m.readTail(m.now())
}

// namesSessions reports whether what the watch shows names the sessions
// running, as the List held names each: Sessions, the Overview with
// SESSIONS shown, and Accounts' Detail.
func (m Model) namesSessions() bool {
	switch m.view {
	case dashboard.Sessions:
		return true
	case dashboard.Overview:
		return m.sections.Has(dashboard.SectionSessions)
	}
	return m.detailShown()
}

// readList reads the List, as a command, once: a store of its own fed as
// the List wants, which reads whole every session it lists, of the router's
// listing, priced at the config's prices. It waits till no read of the
// ledger's tail is under way, and holds the tail's reads off till it lands,
// as a session's page's read does, so the mark never moves past where its
// read of today's lines ends, the store yet to take what lies between.
func (m Model) readList() (Model, tea.Cmd) {
	if m.tail.reading {
		return m, nil
	}
	l, prices, now := m.cfg.Ledger, m.cfg.Prices, m.now()
	want := views.ListWanted(m.sessions, m.traffic.stream(now))
	m.sessionsTab.reading, m.tail = true, m.tail.wholeAsked()
	return m, func() tea.Msg {
		held, _ := views.HoldSessions(prices).Fed(l, views.Taken{}, want, now)
		return listReadMsg{held: held}
	}
}

// listRead takes in the List's first read, its sessions taken into the
// watch's store, held for the run from then on, the tail's reads no longer
// held off by it.
func (m Model) listRead(msg listReadMsg) Model {
	m.tail = m.tail.wholeLanded()
	m = m.heldWith(msg.held)
	m.sessionsTab.reading, m.sessionsTab.listed = false, true
	return m
}

// heldWith is m with the sessions found, held of a reader's read of its
// own, taken into the store, as views.HeldSessions' With takes them: the
// one way a reader's read joins the store, in Update. Those it takes while
// a read of the ledger's tail is under way are kept apart too, for the
// store the read fed to take as it lands, as tailed has it.
func (m Model) heldWith(found views.HeldSessions) Model {
	if m.tail.reading {
		m.merged = m.merged.With(found.Without(m.held))
	}
	m.held = m.held.With(found)
	return m
}

// heldFold is what a read of the ledger's tail does for the store: it takes
// in the lines of the days before today it reads again whole, late, the
// oldest first, then what today's gained, wanting what want wants, as
// views.HeldSessions' Fed feeds it. Awaited is when each day the store
// awaits a line on was last read for it, as the read leaves it.
type heldFold struct {
	held    views.HeldSessions
	want    views.SessionsWanted
	late    []string
	awaited awaitedReads
}

// heldFold is what a read of the ledger's tail at now does for the store,
// nil while nothing wants it nor it holds anything: what it wants, the
// List's sessions once its first read has landed, and the session of the
// page open, once it's loaded, as views.SessionsWanted's Paging has it, so
// it lets go of what nobody wants, reading no day again for a store nothing
// wants; its late days those of the days the tail missed, as missed has
// them, it holds not every line of, as Unread has them; and, of the List's,
// those it awaits a line on, as Awaited has them, whose lines could have
// come since they were last read, as due has them, as the Log's are, the
// page's own being the page's; each once, the oldest first.
func (m Model) heldFold(now time.Time) *heldFold {
	t := m.sessionsTab
	paged := m.session != nil && m.session.asked != 0
	f := &heldFold{held: m.held, awaited: t.awaited}
	switch {
	case !t.listed && !paged && m.held.Len() == 0:
		return nil
	case !t.listed && !paged:
		return f
	}
	live := m.listLive(now, m.traffic.stream(now))
	var due []string
	if t.listed {
		f.want = views.ListWanted(live.Running, live.Stream)
		due, f.awaited = t.awaited.due(m.held.Awaited(live), live.Stream, now)
	}
	if paged {
		f.want = f.want.Paging(m.session.held, live.Stream)
	}
	late := slices.Concat(due, m.held.Unread(m.tail.missed(views.Today(now))))
	f.late = slices.Compact(slices.Sorted(slices.Values(late)))
	return f
}

// heldAwaiting is m with the store noting when each day it awaits a line on
// was last read for it, as the read of the tail f folds leaves it: none
// while nothing wants it.
func (m Model) heldAwaiting(f *heldFold) Model {
	if f != nil {
		m.sessionsTab.awaited = f.awaited
	}
	return m
}

// fed is the store fed, through l, at now, what a read of the ledger's tail
// read, as the fold says: today's gain, and the late days it reads again.
func (f heldFold) fed(l views.LedgerSessions, read tailLines, now time.Time) *views.HeldSessions {
	days := make(map[string][]ledger.Held, len(f.late))
	for _, date := range f.late {
		days[date] = read.days[date]
	}
	held, _ := f.held.Fed(l, views.Took(read.gained, days), f.want, now)
	return &held
}

// noSessionChosen is why enter opens nothing on Sessions while no session
// is chosen.
const noSessionChosen = "no session is chosen"

// sessionsSteps are the ways Sessions' keys move: back, -1, or on, 1.
var sessionsSteps = map[string]int{"[": -1, "]": 1, "up": -1, "down": 1}

// sessionsKey acts on a key of Sessions', reporting false for a key it
// doesn't take, or on another tab, and the command it starts, if any: [ and
// ] turn the sub-tab, as Turned has it; ↑ and ↓ choose a row, as Choosing
// has it; enter opens the page of the session chosen, as openSession opens
// it, and while none is, as the key line draws it faint, says why in the key
// line's place; and esc deselects, as Unchosen has it. A key that
// changes what's shown shows it as showSessions has it.
func (m Model) sessionsKey(key string) (Model, tea.Cmd, bool) {
	if m.view != dashboard.Sessions {
		return m, nil, false
	}
	s := m.sessionsTab.shown
	var next dashboard.SessionsShown
	var changed bool
	switch key {
	case "[", "]":
		next, changed = s.Turned(sessionsSteps[key], m.boardAccounts())
	case "up", "down":
		next, changed = s.Choosing(sessionsSteps[key], m.boardAccounts())
	case "enter":
		if !s.Opens() {
			noted, cmd := m.noting(noSessionChosen)
			return noted, cmd, true
		}
		opened, cmd := m.openSession(s.Chosen.Session)
		return opened, cmd, true
	case "esc":
		next, changed = s.Unchosen()
	default:
		return m, nil, false
	}
	if changed {
		m = m.showSessions(next)
	}
	return m, nil, true
}

// boardAccounts are the ids of the accounts whose lines the Switchboard
// draws, in their order: the document's at now, as accountIDs has them.
func (m Model) boardAccounts() []string {
	return accountIDs(m.moment(m.now()))
}

// accountIDs are the ids of the accounts of the moment mo, in its order.
func accountIDs(mo status.Moment) []string {
	ids := make([]string, len(mo.Accounts))
	for i, a := range mo.Accounts {
		ids[i] = a.ID
	}
	return ids
}

// showSessions shows Sessions as given: another sub-tab from the view's top,
// the row chosen kept in view, as revealing has it. It keeps what the
// preferences keep of it, where that's changed, for the next watch to open
// with: the sub-tab.
func (m Model) showSessions(s dashboard.SessionsShown) Model {
	was := m.sessionsTab.shown
	if s.Shown != was.Shown {
		m.scroll = 0
	}
	m.sessionsTab.shown = s
	m = m.revealing(m.now())
	if kept := s.Kept(); kept != was.Kept() {
		m.keepPrefs("what Sessions shows", func(prefs *theme.Prefs) { prefs.Sessions = kept }, "sessions", kept)
	}
	return m
}

// revealing is m with Sessions' view scrolled no further than shows the row
// chosen on the page m draws at now, as revealed has it: where it is, with
// none chosen.
func (m Model) revealing(now time.Time) Model {
	if m.sessionsTab.shown.Chosen == (dashboard.Plug{}) {
		return m
	}
	f := m.frame(now)
	m.scroll = revealed(m.scroll, f.Scrolling(m.doc, now), func(offset int) bool { return chosenShows(f, m.doc, now, offset) })
	return m
}

// chosenShows reports whether the row chosen on Sessions shows on the page f
// draws of doc at now, scrolled to offset: whether a click there would land
// on it, as Targets lays the page out, whichever form draws it.
func chosenShows(f dashboard.Frame, doc status.Document, now time.Time, offset int) bool {
	f.Scroll = offset
	chosen := f.SessionsTab.Chosen
	return slices.ContainsFunc(f.Targets(doc, now), func(t dashboard.Target) bool {
		return t.On == dashboard.OnSession && t.Session == chosen.Session && t.Account == chosen.Account
	})
}

// revealed is how far a view scrolled to at, scrolling as s says, is scrolled
// to show what shows says it shows at each offset: where it is, where it
// shows there; else at the offset nearest it that shows it, the one above
// where two are as near; and where none does, where it is.
func revealed(at int, s dashboard.Scrolling, shows func(offset int) bool) int {
	from := min(max(at, 0), s.Most)
	for d := range s.Most + 1 {
		if up := from - d; up >= 0 && shows(up) {
			return up
		}
		if down := from + d; d > 0 && down <= s.Most && shows(down) {
			return down
		}
	}
	return at
}

// sessionsClick acts on a click on Sessions, reporting false for one it
// doesn't take, or on another tab, or under a session's page: on a sub-tab,
// it shows it, as Showing has it; and on a call or a row of the List, it
// chooses it, and on the one chosen, opens its session's page, as
// openSession has it. Each opens as enter does, but never through the key,
// which the theme picker takes while it's open. The page acts as acting has
// it.
func (m Model) sessionsClick(t dashboard.Target) (tea.Model, tea.Cmd, bool) {
	if m.view != dashboard.Sessions || m.session != nil {
		return m, nil, false
	}
	s := m.sessionsTab.shown
	switch t.On {
	case dashboard.OnSubTab:
		if next, ok := s.Showing(t.SubTab, m.boardAccounts()); ok {
			return m.acting().showSessions(next), nil, true
		}
		return m, nil, true
	case dashboard.OnSession:
		row := dashboard.Plug{Session: t.Session, Account: t.Account}
		if row == s.Chosen {
			opened, cmd := m.openSession(t.Session)
			return opened, cmd, true
		}
		return m.acting().showSessions(s.ChosenOn(row)), nil, true
	}
	return m, nil, false
}

package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// accountsTab is what the watch holds of Accounts: what it shows; the
// period's page, built of the days History's reads hold, and what it was
// built of, as builtOf has it; that page stood at a moment, and what it was
// stood at, as accountsBuilt stands it; and the sessions Detail's SESSIONS
// listed last, by id, in their order, for its selection to follow, as
// followed has it.
type accountsTab struct {
	shown   dashboard.AccountsShown
	page    *views.AccountsPage
	of      builtOf
	stood   *views.AccountsPage
	at      *stoodAt
	running []string
}

// stoodAt is what a page was stood at: the moment, held, and its time; and
// when it stands otherwise, as the page's Until has it.
type stoodAt struct {
	moment      *status.HeldMoment
	from, until time.Time
}

// stoodTill is what a page stood at the moment mo, till until, was stood at.
func stoodTill(mo status.Moment, until time.Time) *stoodAt {
	return &stoodAt{moment: mo.Held(), from: mo.Now, until: until}
}

// holds reports whether a page stood at s stands at now, of doc, as policy
// judges it, as it was stood: of the one document, judged alike, as the
// moment it was stood at is, from then till it stands otherwise. nil holds
// none.
func (s *stoodAt) holds(doc status.Document, now time.Time, policy score.Policy) bool {
	return s != nil && s.moment.Of(doc, s.from, policy) && !now.Before(s.from) && now.Before(s.until)
}

// builtOf is what Accounts' page was built of: the days held, as historyTab
// counts them as they change, the period, and the local day it was built on.
type builtOf struct {
	held   int
	period views.AccountsPeriod
	day    string
}

// accountsBuilt is m with Accounts' page built afresh at now of the last 12
// months' days held, as views.Past.Accounts builds it over the period shown,
// where those days, the period or the day have changed since it was built,
// while Accounts shows: once for each, never a frame at a time. Each look
// reads today's days afresh, as readHistory reads them, so the page follows
// them from look to look; and past midnight it moves on to the new day
// before the next look reads it. It's stood at the moment of the message
// taken in, as views.AccountsPage.At stands it, where it's built afresh, the
// document or the policy is another, or the page stands otherwise since, as
// its Until has it: never a frame at a time.
func (m Model) accountsBuilt() Model {
	if m.view != dashboard.Accounts || !m.past.last.held() {
		return m
	}
	now := m.now()
	of := builtOf{held: m.past.held, period: m.accounts.shown.Period, day: dayfile.DateOf(now)}
	if m.accounts.page == nil || m.accounts.of != of {
		page := m.past.last.past.Since(nil, now).Accounts(of.period)
		m.accounts.page, m.accounts.of, m.accounts.at = &page, of, nil
	}
	if !m.accounts.at.holds(m.doc, now, m.cfg.Policy) {
		mo := m.moment(now)
		m.accounts.stood = new(m.accounts.page.At(mo))
		m.accounts.at = stoodTill(mo, m.accounts.page.Until(mo))
	}
	return m
}

// standing is Accounts as the frame f shows it: its page as accountsBuilt
// last stood it, none till the days are read; Compare's columns scrolled to
// show its focus at f's size, as CompareFirst has them, and Detail's list
// to show the account it shows, as DetailListScroll has it, each from as far
// along as it was left, so a frame smaller than the one it was scrolled in
// still shows it.
func (t accountsTab) standing(f dashboard.Frame) dashboard.AccountsShown {
	s := t.shown
	s.Page = t.stood
	s.First, s.ListScroll = f.CompareFirst(s.Focus), f.DetailListScroll(s.Account())
	return s
}

// showAccounts shows Accounts as given: another sub-tab or period from the
// view's top; Compare's columns and Detail's list as far along as the frame
// scrolls them to show the focus and the account shown, as standing has
// them. It keeps what the preferences keep of it, where that's changed, for
// the next watch to open with: the sub-tab and the period, its page built
// afresh as accountsBuilt has it.
func (m Model) showAccounts(s dashboard.AccountsShown) Model {
	was := m.accounts.shown
	if s.View() != was.View() || s.Period != was.Period {
		m.scroll = 0
	}
	m.accounts.shown = s
	drawn := m.frame(m.now()).Accounts
	m.accounts.shown.First, m.accounts.shown.ListScroll = drawn.First, drawn.ListScroll
	if kept := s.Kept(); kept != was.Kept() {
		m.keepPrefs("what Accounts shows", func(prefs *theme.Prefs) { prefs.Accounts = kept }, "accounts", kept)
	}
	return m
}

// detailShown reports whether Accounts shows Detail.
func (m Model) detailShown() bool {
	return m.view == dashboard.Accounts && m.accounts.shown.View() == dashboard.AccountsDetail
}

// sessionsOn are the ids of the sessions running at the moment mo on the
// account Detail shows, of those the router listed, sessions, as the request
// stream s has moved and seen them since, in its SESSIONS' order, as
// views.RunningOn has them: none of all accounts, which none runs on alone,
// nor while Accounts shows no Detail.
func (m Model) sessionsOn(mo status.Moment, sessions []status.Session, s views.Stream) []string {
	if !m.detailShown() {
		return nil
	}
	var ids []string
	for _, r := range views.RunningOn(views.RunningOf(mo, sessions, s, m.sessionsTab.list()), m.accounts.shown.Account()) {
		ids = append(ids, r.Session)
	}
	return ids
}

// running are the ids of the sessions Detail's SESSIONS lists at now, as
// sessionsOn has them, as the request stream has moved and seen them.
func (m Model) running(now time.Time) []string {
	return m.sessionsOn(m.moment(now), m.sessions, m.traffic.stream(now))
}

// runningIn are the ids of the sessions Detail's SESSIONS lists on the frame
// f drawn at now, as sessionsOn has them, at its moment, of its sessions and
// its stream.
func (m Model) runningIn(f dashboard.Frame, now time.Time) []string {
	return m.sessionsOn(f.Moment(m.doc, now), f.Sessions, f.Stream)
}

// detailsSessions reports whether Detail shows an account that sessions run
// on at now, of those the router listed, whose SESSIONS say what each is
// doing, as the request stream tells it: never all accounts, which none
// runs on alone.
func (m Model) detailsSessions() bool {
	return m.detailShown() && len(m.sessionsOn(m.moment(m.now()), m.sessions, views.Stream{})) > 0
}

// followed is m with Detail's SESSIONS' selection following the sessions
// running on the account it shows, as Following has it, from those it listed
// last to those it lists now, kept for the next: while Detail shows, but for
// a frame, which takes nothing in; while it doesn't, the selection holds as
// it was, for the run.
func (m Model) followed(msg tea.Msg) Model {
	if _, frame := msg.(frameMsg); frame || !m.detailShown() {
		return m
	}
	is := m.running(m.now())
	m.accounts.shown, m.accounts.running = m.accounts.shown.Following(m.accounts.running, is), is
	return m
}

// accountsSteps are the ways Accounts' keys move: back, -1, or on, 1.
var accountsSteps = map[string]int{"[": -1, "]": 1, "<": -1, ">": 1, "left": -1, "right": 1, "up": -1, "down": 1}

// accountsKey acts on a key of Accounts', reporting false for a key it
// doesn't take, or on another tab, and the command it starts, if any: [ and
// ] turn the sub-tab, and < and > step the period, as Turned and Stepped have
// them; ← and → move Compare's focus, or Detail's ↑ and ↓ between its list
// and its SESSIONS, as Moved has it; ↑ and ↓ choose in Detail, as Choosing
// has it; enter opens what's focused or selected, as entered has it; and esc
// ends Compare's focus, as Unfocused has it. A key that changes what's shown
// shows it as showAccounts has it. Where Compare draws no focus, as on a
// phone, its focus keys do nothing, as focusless has it.
func (m Model) accountsKey(key string) (Model, tea.Cmd, bool) {
	if m.shownView() != dashboard.Accounts {
		return m, nil, false
	}
	if m.focusless(key) {
		return m, nil, true
	}
	s, now, by := m.accounts.shown, m.now(), accountsSteps[key]
	var next dashboard.AccountsShown
	var changed bool
	switch key {
	case "[", "]":
		next, changed = s.Turned(by)
	case "<", ">":
		next, changed = s.Stepped(by)
	case "left", "right":
		next, changed = s.Moved(by, m.running(now))
	case "up", "down":
		next, changed = s.Choosing(by, m.running(now))
	case "enter":
		opened, cmd := m.entered(m.running(now))
		return opened, cmd, true
	case "esc":
		next, changed = s.Unfocused()
	default:
		return m, nil, false
	}
	if changed {
		m = m.showAccounts(next)
	}
	return m, nil, true
}

// comparing reports whether Accounts shows Compare with a focus for ← and →
// to move, enter to open and esc to end: never on a phone, whose Compare
// draws none, as dashboard.Phone tells one.
func (m Model) comparing() bool {
	return m.shownView() == dashboard.Accounts && m.accounts.shown.View() == dashboard.AccountsCompare && !dashboard.Phone(m.size.Width)
}

// focusKeys are Compare's focus keys: those that move its focus, open the
// column it's on and end it.
var focusKeys = map[string]bool{"left": true, "right": true, "enter": true, "esc": true}

// focusless reports whether key is one of Compare's focus keys, pressed on
// Compare where it draws no focus, as comparing has it.
func (m Model) focusless(key string) bool {
	return focusKeys[key] && m.accounts.shown.View() == dashboard.AccountsCompare && !m.comparing()
}

// entered is m with enter pressed on Accounts, running being the sessions
// running on the account Detail shows, by id, in its SESSIONS' order: where
// Opens says there's nothing to open, as the key line draws enter faint, it
// says why in the key line's place, as unopened has it; else on Compare, the
// column focused opened in Detail, as Opened has it, and on Detail, from its
// list as from its SESSIONS, the page of the session selected, as Selection
// has it, opened as openSession opens it.
func (m Model) entered(running []string) (Model, tea.Cmd) {
	s := m.accounts.shown
	if !s.Opens(running) {
		if why := unopened(s); why != "" {
			return m.noting(why)
		}
		return m, nil
	}
	if next, opened := s.Opened(); opened {
		return m.showAccounts(next), nil
	}
	return m.openSession(s.Selection(running))
}

// unopened is why enter opens nothing on Accounts as s shows it, where it
// doesn't: no column of Compare focused; all accounts chosen in Detail's
// list, which no session runs on alone; or an account no session runs on.
// It's "" with one account, whose key line has no enter while none runs on
// it.
func unopened(s dashboard.AccountsShown) string {
	switch {
	case s.View() == dashboard.AccountsCompare:
		return "no account is focused"
	case s.Alone():
		return ""
	case s.Account() == "":
		return "all accounts lists no sessions"
	}
	return "no session is running on this account"
}

// accountsClick acts on a click on Accounts, reporting false for one it
// doesn't take, or on another tab, or under a session's page: on a sub-tab,
// it shows it, as Showing has it; on a Compare column's heads, it focuses
// it, and on the one focused, opens it in Detail, as FocusedOn has it; on an
// account in Detail's list, it chooses it; and on a session in its SESSIONS,
// it selects it, and on the one selected, opens its page, as openSession
// has it (settled in the plan), each as enter opens it. The page acts as
// acting has it.
func (m Model) accountsClick(t dashboard.Target) (tea.Model, tea.Cmd, bool) {
	if m.shownView() != dashboard.Accounts {
		return m, nil, false
	}
	s := m.accounts.shown
	var next dashboard.AccountsShown
	switch t.On {
	case dashboard.OnSubTab:
		next, _ = s.Showing(t.SubTab)
	case dashboard.OnColumn:
		next = s.FocusedOn(t.Account)
	case dashboard.OnAccount:
		next = s.ChosenOn(t.Account)
	case dashboard.OnSession:
		if t.Session == s.Selection(m.running(m.now())) {
			opened, cmd := m.openSession(t.Session)
			return opened, cmd, true
		}
		next = s.SelectedOn(t.Session)
	default:
		return m, nil, false
	}
	return m.acting().showAccounts(next), nil, true
}

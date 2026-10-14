package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// accountsTab is what the watch holds of Accounts: what it shows; the
// period's page, built of the days History's reads hold, and what it was
// built of, as builtOf has it; and the sessions Detail's SESSIONS listed
// last, by id, in their order, for its selection to follow, as followed has
// it.
type accountsTab struct {
	shown   dashboard.AccountsShown
	page    *views.AccountsPage
	of      builtOf
	running []string
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
// before the next look reads it.
func (m Model) accountsBuilt() Model {
	if m.view != dashboard.Accounts || !m.past.last.held() {
		return m
	}
	now := m.now()
	of := builtOf{held: m.past.held, period: m.accounts.shown.Period, day: dayfile.DateOf(now)}
	if m.accounts.page != nil && m.accounts.of == of {
		return m
	}
	page := m.past.last.past.Since(nil, now).Accounts(of.period)
	m.accounts.page, m.accounts.of = &page, of
	return m
}

// standing is Accounts as the frame shows it at the moment mo: its page, as
// it was built, stood there, as views.AccountsPage.At stands it; none till
// the days are read.
func (t accountsTab) standing(mo status.Moment) dashboard.AccountsShown {
	s := t.shown
	if t.page != nil {
		s.Page = new(t.page.At(mo))
	}
	return s
}

// showAccounts shows Accounts as given: another sub-tab or period from the
// view's top; Compare's columns scrolled to its focus, at the size drawn
// at, as CompareFirst has them. It keeps what the preferences keep of it,
// where that's changed, for the next watch to open with: the sub-tab and the
// period, its page built afresh as accountsBuilt has it.
func (m Model) showAccounts(s dashboard.AccountsShown) Model {
	was := m.accounts.shown
	if s.View() != was.View() || s.Period != was.Period {
		m.scroll = 0
	}
	m.accounts.shown = s
	if s.View() == dashboard.AccountsCompare && s.Focused {
		m.accounts.shown.First = m.frame(m.now()).CompareFirst(s.Focus)
	}
	if kept := s.Kept(); kept != was.Kept() {
		m.keepPrefs("what Accounts shows", func(prefs *theme.Prefs) { prefs.Accounts = kept }, "accounts", kept)
	}
	return m
}

// detailShown reports whether Accounts shows Detail.
func (m Model) detailShown() bool {
	return m.view == dashboard.Accounts && m.accounts.shown.View() == dashboard.AccountsDetail
}

// sessionsOn are the ids of the sessions running at now on the account
// Detail shows, of those the router listed, as the request stream s has
// moved and seen them since, in its SESSIONS' order, as views.RunningOn has
// them: none of all accounts, which none runs on alone, nor while Accounts
// shows no Detail.
func (m Model) sessionsOn(now time.Time, s views.Stream) []string {
	if !m.detailShown() {
		return nil
	}
	var ids []string
	for _, r := range views.RunningOn(views.RunningOf(m.moment(now), m.sessions, s), m.accounts.shown.Account()) {
		ids = append(ids, r.Session)
	}
	return ids
}

// running are the ids of the sessions Detail's SESSIONS lists at now, as
// sessionsOn has them, as the request stream has moved and seen them.
func (m Model) running(now time.Time) []string {
	return m.sessionsOn(now, m.traffic.stream(now))
}

// detailsSessions reports whether Detail shows an account that sessions run
// on at now, of those the router listed, whose SESSIONS say what each is
// doing, as the request stream tells it: never all accounts, which none
// runs on alone.
func (m Model) detailsSessions() bool {
	return m.detailShown() && len(m.sessionsOn(m.now(), views.Stream{})) > 0
}

// followed is m with Detail's SESSIONS' selection following the sessions
// running on the account it shows, as Following has it, from those it listed
// last to those it lists now, kept for the next: while Detail shows; while
// it doesn't, the selection holds as it was, for the run.
func (m Model) followed() Model {
	if !m.detailShown() {
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
// has it; and enter opens, on Compare, the column focused in Detail, as
// Opened has it, and on Detail, the page of the session selected, as
// openSelected has it. A key that changes what's shown shows it as
// showAccounts has it.
func (m Model) accountsKey(key string) (Model, tea.Cmd, bool) {
	if m.view != dashboard.Accounts {
		return m, nil, false
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
		if s.View() == dashboard.AccountsDetail {
			opened, cmd := m.openSelected(m.running(now))
			return opened, cmd, true
		}
		next, changed = s.Opened()
	default:
		return m, nil, false
	}
	if changed {
		m = m.showAccounts(next)
	}
	return m, nil, true
}

// openSelected opens the page of the session Detail's SESSIONS has selected
// of those running, as Selection has it, as enter does there and from its
// list, as openSession opens it: nothing where none runs on the account it
// shows.
func (m Model) openSelected(running []string) (Model, tea.Cmd) {
	if id := m.accounts.shown.Selection(running); id != "" {
		return m.openSession(id)
	}
	return m, nil
}

// accountsClick acts on a click on Accounts, reporting false for one it
// doesn't take, or on another tab, or under a session's page: on a sub-tab,
// it shows it, as Showing has it; on a Compare column's heads, it focuses
// it; on an account in Detail's list, it chooses it; and on a session in its
// SESSIONS, it selects it. On the column focused, or the session selected,
// it opens it, as enter does (settled in the plan). The page acts as acting
// has it.
func (m Model) accountsClick(t dashboard.Target) (tea.Model, tea.Cmd, bool) {
	if m.view != dashboard.Accounts || m.session != nil {
		return m, nil, false
	}
	s := m.accounts.shown
	var next dashboard.AccountsShown
	var opens bool
	switch t.On {
	case dashboard.OnSubTab:
		next, _ = s.Showing(t.SubTab)
	case dashboard.OnColumn:
		next, opens = s.FocusedOn(t.Account), s.Focused && s.Focus == t.Account
	case dashboard.OnAccount:
		next = s.ChosenOn(t.Account)
	case dashboard.OnSession:
		next, opens = s.SelectedOn(t.Session), t.Session == s.Selection(m.running(m.now()))
	default:
		return m, nil, false
	}
	if opens {
		opened, cmd := m.pressed(namedPresses["enter"])
		return opened, cmd, true
	}
	return m.acting().showAccounts(next), nil, true
}

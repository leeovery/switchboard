package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// accountsTab is what the watch holds of Accounts: what it shows; and the
// period's page, built of the days History's reads hold, and what it was
// built of, as builtOf has it.
type accountsTab struct {
	shown dashboard.AccountsShown
	page  *views.AccountsPage
	of    builtOf
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

// showAccounts shows Accounts as given, and keeps what the preferences keep
// of it, where that's changed, for the next watch to open with: the sub-tab
// and the period, its page built afresh as accountsBuilt has it.
func (m Model) showAccounts(s dashboard.AccountsShown) Model {
	was := m.accounts.shown.Kept()
	m.accounts.shown = s
	if kept := s.Kept(); kept != was {
		m.keepPrefs("what Accounts shows", func(prefs *theme.Prefs) { prefs.Accounts = kept }, "accounts", kept)
	}
	return m
}

// detailsSessions reports whether Detail shows an account that sessions run
// on at now, of those the router listed, whose SESSIONS say what each is
// doing, as the request stream tells it: never all accounts, which none
// runs on alone.
func (m Model) detailsSessions() bool {
	s := m.accounts.shown
	if m.view != dashboard.Accounts || s.View() != dashboard.AccountsDetail {
		return false
	}
	now := m.now()
	return len(views.RunningOn(views.RunningOf(m.moment(now), m.sessions, views.Stream{}), s.Account())) > 0
}

// accountsKey acts on a key of Accounts', reporting false for a key it
// doesn't take, or on another tab, and the command it starts, if any: none
// yet.
func (m Model) accountsKey(string) (Model, tea.Cmd, bool) {
	return m, nil, false
}

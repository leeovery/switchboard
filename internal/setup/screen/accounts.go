package screen

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// reviewed takes in what Review answered, as status.reviewAnswered has it,
// the accounts read again, where a change was waiting on it: at Accounts,
// the step done once the accounts are first read, failed where they
// couldn't be, and its chosen row following the account it was on each
// time they're read, as following has it; read first with no account, a
// first run, the add card open at once, as there's nothing to set up
// without one.
func (m Model) reviewed(msg reviewMsg) Model {
	was := m.status.review
	m.status = m.status.reviewAnswered(msg)
	m.rereading = m.rereading && msg.ask < m.status.asked
	if m.walk.step != dashboard.SetupAccounts || !m.status.reviewed {
		return m
	}
	if !m.walk.done {
		var err error
		if m.status.review == nil {
			err = msg.err
		}
		m.walk = m.walk.finished(err)
	}
	if was == nil && m.status.review != nil && len(m.status.review.Accounts) == 0 {
		m.firstRun, m.add = true, addOpened()
	}
	return m.following(was).madeChosen(msg)
}

// madeChosen is m, once the accounts are read again since a token was saved,
// as msg reads them, the account it was saved for forgotten, and, where
// they're read, its row chosen, kept in view: the account added, or the one
// whose token was replaced.
func (m Model) madeChosen(msg reviewMsg) Model {
	if m.madeFor == "" || m.rereading {
		return m
	}
	id := m.madeFor
	m.madeFor = ""
	if msg.err != nil || m.status.review == nil {
		return m
	}
	if i := slices.IndexFunc(m.status.review.Accounts, func(a setup.Reviewed) bool { return a.ID == id }); i >= 0 {
		m.chosen = i
		m = m.chosenInView()
	}
	return m
}

// settling reports whether a change to the accounts is outstanding: p's,
// being written, or any, made, while the accounts aren't yet read again. The
// account keys and enter wait for it, so each acts on the accounts as every
// change has left them, never on a row a removal has taken out.
func (m Model) settling() bool {
	return m.marking > 0 || m.rereading
}

// following is m with its row chosen on the account it was on as the
// accounts were read before, was, wherever it's read now; or where it's gone,
// the one read after it, else the last, so + add an account is chosen only
// where it was, or no account is left. The page scrolls to the row chosen
// only where its row has moved, so a page scrolled past it, as to the
// notes, stays where it is, as far as it still goes. Read for the first
// time, the first row is chosen, in view.
func (m Model) following(was *setup.Review) Model {
	if was == nil {
		return m.chose(0)
	}
	at := m.chosen
	switch now := m.status.review; {
	case now == nil || at >= len(was.Accounts):
		at = m.rows() - 1
	default:
		id := was.Accounts[at].ID
		if i := slices.IndexFunc(now.Accounts, func(a setup.Reviewed) bool { return a.ID == id }); i >= 0 {
			at = i
		} else {
			at = max(min(at, len(now.Accounts)-1), 0)
		}
	}
	if at == m.chosen {
		return m.scrolled(0, 0)
	}
	m.chosen = at
	return m.chosenInView()
}

// rows is how many rows the Accounts page chooses between: an account's
// each, and + add an account.
func (m Model) rows() int {
	if m.status.review == nil {
		return 1
	}
	return len(m.status.review.Accounts) + 1
}

// adding reports whether + add an account is the row chosen.
func (m Model) adding() bool {
	return m.chosen == m.rows()-1
}

// choosing is how far the key moves the Accounts page's row chosen,
// reporting false for a key that doesn't, or off that page: ↑ and k a row
// up, ↓ and j a row down.
func (m Model) choosing(key string) (by int, ok bool) {
	if m.walk.step != dashboard.SetupAccounts {
		return 0, false
	}
	switch key {
	case "up", "k":
		return -1, true
	case "down", "j":
		return 1, true
	}
	return 0, false
}

// chose is m with the row by rows on from the one chosen chosen, no further
// than the first or the last, kept in view.
func (m Model) chose(by int) Model {
	m.chosen = min(max(m.chosen+by, 0), m.rows()-1)
	return m.chosenInView()
}

// chosenInView is m with the Accounts page scrolled as little as keeps its
// row chosen in view, as dashboard.Setup.ScrolledToChosen has it.
func (m Model) chosenInView() Model {
	if m.walk.step == dashboard.SetupAccounts {
		m.scroll = m.page().ScrolledToChosen(m.size.Width, m.size.Height)
	}
	return m
}

// accountsEntered acts on enter at the Accounts page, once its accounts are
// read, but while a change is settling, which it waits for: on + add an
// account, it opens the add card, so it never moves on with no account; on
// an account's row, it moves on, changing nothing, but where two accounts or
// more have none the primary, which waits for one.
func (m Model) accountsEntered() (tea.Model, tea.Cmd) {
	switch {
	case m.settling():
	case m.adding():
		m.add = addOpened()
	case !m.status.review.Unmarked:
		return m.movedOn()
	}
	return m, nil
}

// accountsPage is what the Accounts page has of its own, as dashboard.Setup
// draws it: whether it's a first run, the row chosen, the card open over
// it, where one is, and what the step failed at, where it did. Its
// accounts, as the status line has them, and why the step failed are the
// summary's.
func (m Model) accountsPage() *dashboard.SetupAccountsPage {
	p := &dashboard.SetupAccountsPage{FirstRun: m.firstRun, Chosen: m.chosen, Failed: m.accountsFailed}
	if m.add != nil {
		p.Add = m.add.drawn()
	}
	if m.edit != nil {
		p.Edit = m.edit.drawn()
	}
	if m.remove != nil {
		p.Remove = m.remove.drawn()
	}
	if m.token != nil {
		p.Token = m.token.drawn(m.now())
	}
	if m.paste != nil {
		p.Paste = m.paste.drawn()
	}
	return p
}

// accountKeys are the keys that act on the account chosen, as accountKey
// has them.
var accountKeys = []string{"e", "r", "p", "t"}

// chosenAccount is the account whose row is chosen, once the Accounts step
// has read them, while it hasn't failed and no change is settling: false
// where none is, as where + add an account is chosen.
func (m Model) chosenAccount() (setup.Reviewed, bool) {
	review := m.status.review
	if m.walk.step != dashboard.SetupAccounts || !m.walk.done || m.walk.failed != nil || m.settling() || review == nil || m.chosen >= len(review.Accounts) {
		return setup.Reviewed{}, false
	}
	return review.Accounts[m.chosen], true
}

// accountKey acts on the account a, chosen, as key says: e opens its edit
// card; r, the card asking before it's removed; p makes it the primary, as
// madePrimary has it; and t makes it a new token, saved in its old one's
// place, compared with the accounts read, as making has it, once no
// account's check is out.
func (m Model) accountKey(key string, a setup.Reviewed) (tea.Model, tea.Cmd) {
	switch key {
	case "e":
		m.edit = editing(a)
	case "r":
		m.remove = &removeCard{id: a.ID}
	case "p":
		return m.madePrimary(a)
	case "t":
		if m.status.checksOut() {
			return m, nil
		}
		return m.making(setup.TokenFor{Account: config.NewAccount{ID: a.ID}, Organizations: m.organizations()})
	}
	return m, nil
}

// organizations are those of the accounts read, as setup has them, which a
// token made is compared with: none where none are read. A token's making
// waits while any account's check is out (see checksOut), so it's compared
// with every one the run will know.
func (m Model) organizations() setup.Organizations {
	if m.status.review == nil {
		return nil
	}
	return m.status.review.Organizations()
}

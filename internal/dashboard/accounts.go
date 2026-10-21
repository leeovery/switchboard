package dashboard

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// AccountsView is one of Accounts' views, its sub-tabs: Compare, the
// default, and Detail.
type AccountsView int

const (
	// AccountsCompare sets the accounts side by side, a column each, with
	// all accounts last.
	AccountsCompare AccountsView = iota
	// AccountsDetail shows one account in full, or all of them, chosen from
	// a list.
	AccountsDetail
)

// accountsViews are Accounts' sub-tabs' names, in the heading row's order.
var accountsViews = []string{"Compare", "Detail"}

// AccountsShown is Accounts as the watch gives it: the sub-tab shown and the
// period both count over, which the preferences keep; Compare's focus and
// the columns it shows; Detail's choice, its list's scroll, and the session
// selected in its SESSIONS, which hold for the run alone; and the period's
// page, built of the ledger's days.
type AccountsShown struct {
	// Shown is the sub-tab shown, as View has it, and Period the period
	// both count over.
	Shown  AccountsView
	Period views.AccountsPeriod
	// Accounts are the configured accounts, in the config's order: Compare's
	// columns and Detail's list.
	Accounts config.Accounts
	// Focused is set once ← or → has put Compare's focus on a column, and
	// Focus is its account, by id, "" for all accounts; First is the place
	// among the accounts of the first Compare shows, as its columns scroll
	// with the focus.
	Focused bool
	Focus   string
	First   int
	// Chosen is the account Detail's list has chosen, by id, "" for all
	// accounts, as it opens; and ListScroll how many of its rows are
	// scrolled out of view above, as the list scrolls with the choice.
	Chosen     string
	ListScroll int
	// InSessions is set while ↑ and ↓ move through Detail's SESSIONS, → having
	// taken them there from the list; and Selected is the session selected
	// there, by id, "" for the first.
	InSessions bool
	Selected   string
	// Page is the period's page, stood at the moment drawn, as
	// views.AccountsPage.At stands it: nil till the days are read.
	Page *views.AccountsPage
}

// AccountsKept is Accounts as the preferences kept it: the sub-tab and the
// period, each by its name. What names neither, as a later build's, shows
// as Accounts opens: Compare, this week.
func AccountsKept(kept theme.AccountsPrefs) AccountsShown {
	s := AccountsShown{Shown: AccountsView(placeAmong(kept.View, lowerNames(accountsViews)))}
	if period := views.AccountsPeriod(kept.Period); period.Known() {
		s.Period = period
	}
	return s
}

// Kept is what the preferences keep of Accounts, as AccountsKept reads it:
// the sub-tab shown and the period, never what's chosen.
func (s AccountsShown) Kept() theme.AccountsPrefs {
	return theme.AccountsPrefs{View: lowerNames(accountsViews)[s.Shown], Period: string(s.Period)}
}

// Alone reports whether there's one account, which all accounts would only
// repeat: Compare goes, Detail showing it.
func (s AccountsShown) Alone() bool {
	return len(s.Accounts) == 1
}

// View is the sub-tab drawn: Shown, but Detail where there's one account.
func (s AccountsShown) View() AccountsView {
	if s.Alone() {
		return AccountsDetail
	}
	return s.Shown
}

// Account is the account Detail shows, by id: the one there is, where
// there's one; else Chosen, "" for all accounts.
func (s AccountsShown) Account() string {
	if s.Alone() {
		return s.Accounts[0].ID
	}
	return s.Chosen
}

// heading is Accounts' heading row: its name and its sub-tabs, the one drawn
// picked out, Detail alone where there's one account; and at its right, the
// period between < and >.
func (s AccountsShown) heading() heading {
	h := heading{name: strings.ToUpper(Accounts.Title()), subTabs: accountsViews, shown: int(s.View()), period: s.Period.Name()}
	if s.Alone() {
		h.subTabs, h.shown = accountsViews[AccountsDetail:], 0
	}
	return h
}

// accountsHeading is the frame's heading row on Accounts, as its heading has
// it, and after [ ], on Compare, which of its accounts it shows, in the forms
// compareState says it in.
func (f Frame) accountsHeading() heading {
	h := f.Accounts.heading()
	if f.Accounts.View() != AccountsCompare {
		return h
	}
	if forms := f.compareState(); len(forms) > 0 {
		h.state, h.shorter = forms[0], forms[1:]
	}
	return h
}

// cueState picks out width cells from x along row y, the state in a word of
// the account with the given id, as Accounts says it, while it's newly
// changed, as far faded back as the frame's StateChanged says, as cue picks
// them out: never for a change ROUTING alone sees, as an open account going
// idle.
func (f Frame) cueState(c *canvas, id string, x, y, width int) {
	if fade, ok := f.StateChanged[id]; ok {
		cue(c, f.Look, x, y, width, fade)
	}
}

// accountsForm is Accounts' view shown as its form draws it: what it draws
// from row top; how far it scrolls, scrolled to each offset; what of it
// scrolls, from column x along row y, width cells wide and rows tall; the
// zones a click acts on; how much of each bar it draws it fills, as Filled
// has them; and, of Detail, the cells it draws a day of BY DAY in. Each is
// nothing where it's nil: it draws nothing, never scrolls, its view scrolls
// whole, nothing takes a click, it fills no bar, and its days are the
// fewest cells.
type accountsForm struct {
	draw      func(c *canvas, top int)
	scrolling func(offset int) Scrolling
	scrolls   func() (x, y, width, rows int)
	targets   func() []Target
	filled    func() map[Ref]float64
	days      func() int
}

// accountsForm is the form Accounts' view shown is drawn in, of doc at now:
// as Built built it, where it built it of doc at now, else afresh.
func (f Frame) accountsForm(doc status.Document, now time.Time) accountsForm {
	if f.built != nil && f.built.at.Of(doc, now, f.Policy) {
		return f.built.accounts()
	}
	return f.buildAccountsForm(doc, now)
}

// buildAccountsForm builds the form Accounts' view shown is drawn in, of doc
// at now: a phone's, as phoneAccounts has it; else Compare's, as compareForm
// has it, or Detail's, as detailForm has it.
func (f Frame) buildAccountsForm(doc status.Document, now time.Time) accountsForm {
	switch {
	case f.phone():
		return f.phoneAccounts(doc, now)
	case f.Accounts.View() == AccountsCompare:
		return f.compareForm(doc, now)
	}
	return f.detailForm(doc, now)
}

// accounts draws Accounts' view shown of doc at now from row top, as its
// form draws it.
func (f Frame) accounts(c *canvas, doc status.Document, now time.Time, top int) {
	if form := f.accountsForm(doc, now); form.draw != nil {
		form.draw(c, top)
	}
}

// accountsScrolling is how far Accounts' view shown of doc at now scrolls,
// scrolled to each offset given, as its form says: nowhere where it doesn't
// say.
func (f Frame) accountsScrolling(doc status.Document, now time.Time) func(offset int) Scrolling {
	if form := f.accountsForm(doc, now); form.scrolling != nil {
		return form.scrolling
	}
	return func(int) Scrolling { return Scrolling{} }
}

// accountsScrolls is what of Accounts' view shown of doc at now scrolls, as
// its form says: else the view's rows, across the frame.
func (f Frame) accountsScrolls(doc status.Document, now time.Time) (x, y, width, rows int) {
	return f.accountsForm(doc, now).scrolled(f)
}

// scrolled is what of the form scrolls in the frame f, as its scrolls says:
// else the view's rows, across the frame.
func (form accountsForm) scrolled(f Frame) (x, y, width, rows int) {
	if form.scrolls != nil {
		return form.scrolls()
	}
	return 0, f.top(), f.Width, f.viewRows()
}

// detailDays are the cells Detail draws a day of BY DAY in, as the form it's
// built in has them, so help and Detail read the one choice of its form;
// dayCellsLeast, the stack's, where the frame isn't built, or its form
// doesn't say.
func (f Frame) detailDays() int {
	if f.built == nil {
		return dayCellsLeast
	}
	if form := f.built.accounts(); form.days != nil {
		return form.days()
	}
	return dayCellsLeast
}

// accountsTargets are the zones of Accounts' view shown of doc at now a
// click acts on, as its form lays them out.
func (f Frame) accountsTargets(doc status.Document, now time.Time) []Target {
	if form := f.accountsForm(doc, now); form.targets != nil {
		return form.targets()
	}
	return nil
}

// accountsFilled are how much of each bar Accounts' view shown of doc at now
// draws it fills, as Filled has them, as its form says.
func (f Frame) accountsFilled(doc status.Document, now time.Time) map[Ref]float64 {
	if form := f.accountsForm(doc, now); form.filled != nil {
		return form.filled()
	}
	return nil
}

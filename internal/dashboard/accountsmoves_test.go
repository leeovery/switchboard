package dashboard

import (
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// tenConfigured are ten accounts, acct-1 to acct-10, more than Compare shows
// at 160 columns.
func tenConfigured() config.Accounts {
	accounts := make(config.Accounts, 10)
	for i := range accounts {
		accounts[i] = config.Account{ID: fmt.Sprintf("acct-%d", i+1)}
	}
	return accounts
}

// Sessions running on work, by id, in SESSIONS' order.
var workRunning = []string{"d28c", "7f3a", "9e21"}

// [ and ] move between Compare and Detail, round; a click picks one, but the
// one shown. With one account, Detail alone is drawn, and neither moves.
func TestAccountsSubTabsTurnRound(t *testing.T) {
	s := AccountsShown{Accounts: threeConfigured}
	for _, step := range []struct {
		by   int
		want AccountsView
	}{{-1, AccountsDetail}, {-1, AccountsCompare}, {1, AccountsDetail}, {1, AccountsCompare}} {
		var ok bool
		if s, ok = s.Turned(step.by); !ok || s.Shown != step.want {
			t.Errorf("turned %d to %d, %v; want %d", step.by, s.Shown, ok, step.want)
		}
	}
	if _, ok := s.Showing(int(AccountsCompare)); ok {
		t.Error("Showing() the sub-tab shown reports a change, want none")
	}
	if got, ok := s.Showing(int(AccountsDetail)); !ok || got.Shown != AccountsDetail {
		t.Errorf("Showing(Detail) = %d, %v; want Detail shown", got.Shown, ok)
	}
	if _, ok := s.Showing(len(accountsViews)); ok {
		t.Error("Showing() a sub-tab there isn't reports a change, want none")
	}
	one := AccountsShown{Accounts: threeConfigured[:1]}
	for _, by := range []int{-1, 1} {
		if got, ok := one.Turned(by); ok || got.View() != AccountsDetail {
			t.Errorf("with one account, turned %d to %d, %v; want Detail, unmoved", by, got.View(), ok)
		}
	}
	if _, ok := one.Showing(0); ok {
		t.Error("with one account, a click on Detail, the sub-tab there is, reports a change, want none")
	}
}

// < steps to a shorter period and > to a longer, today the shortest and the
// last 12 weeks the longest, where neither steps further.
func TestAccountsPeriodsStepAndStopAtTheEnds(t *testing.T) {
	s := AccountsShown{Accounts: threeConfigured}
	var periods []string
	var stops int
	for _, by := range []int{-1, -1, 1, 1, 1, 1} {
		var ok bool
		if s, ok = s.Stepped(by); !ok {
			stops++
		}
		periods = append(periods, s.Period.Name())
	}
	want := []string{"today", "today", "this week", "30 days", "12 weeks", "12 weeks"}
	if !slices.Equal(periods, want) || stops != 2 {
		t.Errorf("stepped through %q, stopping %d times; want %q, stopping at each end", periods, stops, want)
	}
}

// ← and → move Compare's focus along its columns, its accounts' then all
// accounts', no further than either end: from none, → puts it on the first
// column shown, and ← on all accounts'. On Detail they do nothing to it.
func TestArrowsMoveComparesFocusAlongItsColumns(t *testing.T) {
	ten := tenConfigured()
	tests := []struct {
		name    string
		shown   AccountsShown
		by      int
		want    string
		changed bool
	}{
		{name: "→ from none", shown: AccountsShown{Accounts: threeConfigured}, by: 1, want: "work", changed: true},
		{name: "→ from none, scrolled along", shown: AccountsShown{Accounts: ten, First: 4}, by: 1, want: "acct-5", changed: true},
		{name: "← from none", shown: AccountsShown{Accounts: threeConfigured}, by: -1, want: "", changed: true},
		{name: "→ along", shown: AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "personal"}, by: 1, want: "side", changed: true},
		{name: "→ onto all accounts", shown: AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "side"}, by: 1, want: "", changed: true},
		{name: "→ at all accounts", shown: AccountsShown{Accounts: threeConfigured, Focused: true}, by: 1, want: ""},
		{name: "← back from all accounts", shown: AccountsShown{Accounts: threeConfigured, Focused: true}, by: -1, want: "side", changed: true},
		{name: "← at the first", shown: AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "work"}, by: -1, want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := tt.shown.Moved(tt.by, nil)
			if !got.Focused || got.Focus != tt.want || changed != tt.changed {
				t.Errorf("Moved(%d) focuses %v on %q, changed %v; want %q, changed %v", tt.by, got.Focused, got.Focus, changed, tt.want, tt.changed)
			}
		})
	}
	detail := AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured}
	if got, _ := detail.Moved(-1, nil); got.Focused {
		t.Error("← on Detail focused a Compare column, want none")
	}
}

// enter opens the column Compare's focus is on in Detail, its account chosen
// there, or all accounts, in the list, the session selected there the
// first; with no column focused, or on Detail, it does nothing.
func TestEnterOpensComparesFocusedColumnInDetail(t *testing.T) {
	was := AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "side", Chosen: "work", InSessions: true, Selected: "d28c"}
	got, ok := was.Opened()
	if !ok || got.View() != AccountsDetail || got.Chosen != "side" || got.InSessions || got.Selected != "" || got.Focus != "side" {
		t.Errorf("Opened() = %+v, %v; want Detail of side, in its list, its first session selected, Compare still focused on side", got, ok)
	}
	all := AccountsShown{Accounts: threeConfigured, Focused: true, Chosen: "work"}
	if got, _ := all.Opened(); got.Chosen != "" {
		t.Errorf("all accounts' column opened %q in Detail, want all accounts", got.Chosen)
	}
	for _, s := range []AccountsShown{{Accounts: threeConfigured}, {Shown: AccountsDetail, Accounts: threeConfigured, Focused: true}} {
		if got, ok := s.Opened(); ok || got.Shown != s.Shown {
			t.Errorf("Opened() of %+v = %+v, %v; want nothing", s, got, ok)
		}
	}
}

// enter opens something where Compare has a column focused, or Detail a
// session selected, from its list as from its SESSIONS, with one account too;
// else nothing.
func TestEnterOpensWhereAColumnIsFocusedOrASessionSelected(t *testing.T) {
	tests := []struct {
		name    string
		shown   AccountsShown
		running []string
		want    bool
	}{
		{name: "Compare, no column focused", shown: AccountsShown{Accounts: threeConfigured}},
		{name: "Compare, a column focused", shown: AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "side"}, want: true},
		{name: "Compare, sessions running on the account Detail chose", shown: AccountsShown{Accounts: threeConfigured, Chosen: "work"}, running: workRunning},
		{name: "Detail of all accounts", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured}},
		{name: "Detail's list, sessions running", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work"}, running: workRunning, want: true},
		{name: "Detail's SESSIONS", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", InSessions: true}, running: workRunning, want: true},
		{name: "Detail of an account none runs on", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "side"}},
		{name: "Detail, focused on Compare before", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Focused: true, Chosen: "side"}},
		{name: "one account, sessions running", shown: AccountsShown{Accounts: threeConfigured[:1]}, running: workRunning, want: true},
		{name: "one account, none running", shown: AccountsShown{Accounts: threeConfigured[:1]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.shown.Opens(tt.running); got != tt.want {
				t.Errorf("Opens(%q) = %v, want %v", tt.running, got, tt.want)
			}
		})
	}
}

// esc ends Compare's focus, its columns staying where they are; with no
// column focused, or on Detail, it does nothing.
func TestEscEndsComparesFocusWhereTheColumnsAre(t *testing.T) {
	focused := AccountsShown{Accounts: tenConfigured(), Focused: true, Focus: "acct-7", First: 1}
	if got, ok := focused.Unfocused(); !ok || got.Focused || got.Focus != "" || got.First != 1 {
		t.Errorf("Unfocused() = %+v, %v; want no focus, the columns from the 2nd still", got, ok)
	}
	for _, s := range []AccountsShown{
		{Accounts: threeConfigured},
		{Shown: AccountsDetail, Accounts: threeConfigured, Focused: true, Focus: "side"},
	} {
		if got, ok := s.Unfocused(); ok || got.Focused != s.Focused {
			t.Errorf("Unfocused() of %+v = %+v, %v; want nothing", s, got, ok)
		}
	}
}

// With no accounts, Compare draws no columns, so ← and → focus none, a click
// on all accounts' heads focuses nothing, and enter opens nothing.
func TestComparesFocusFindsNoColumnWithNoAccounts(t *testing.T) {
	var s AccountsShown
	for _, by := range []int{-1, 1} {
		if got, ok := s.Moved(by, nil); ok || got.Focused {
			t.Errorf("Moved(%d) = %+v, %v; want no focus", by, got, ok)
		}
	}
	if got := s.FocusedOn(""); got.Focused || got.Shown != AccountsCompare {
		t.Errorf("FocusedOn(all accounts) = %+v, want no focus, on Compare", got)
	}
	if s.Opens(nil) {
		t.Error("enter opens a column, want none")
	}
}

// ↑ and ↓ on Detail choose an account in its list, all accounts first, no
// further than either end, a choice of another account selecting its first
// session; in its SESSIONS, and with one account, which has no list, they
// select a session, no further than either end; on Compare, nothing.
func TestUpAndDownChooseAnAccountInDetailsListOrASessionInItsSESSIONS(t *testing.T) {
	list := AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Selected: "7f3a"}
	tests := []struct {
		name     string
		shown    AccountsShown
		by       int
		chosen   string
		selected string
		changed  bool
	}{
		{name: "↓ from all accounts", shown: list, by: 1, chosen: "work", changed: true},
		{name: "↑ at all accounts", shown: list, by: -1, chosen: "", selected: "7f3a"},
		{name: "↓ at the last", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "side"}, by: 1, chosen: "side"},
		{name: "↑ to all accounts", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", Selected: "7f3a"}, by: -1, chosen: "", changed: true},
		{name: "↓ in SESSIONS, from the first", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", InSessions: true}, by: 1, chosen: "work", selected: "7f3a", changed: true},
		{name: "↓ in SESSIONS, at the last", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", InSessions: true, Selected: "9e21"}, by: 1, chosen: "work", selected: "9e21"},
		{name: "↑ in SESSIONS", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", InSessions: true, Selected: "9e21"}, by: -1, chosen: "work", selected: "7f3a", changed: true},
		{name: "↓ with one account", shown: AccountsShown{Accounts: threeConfigured[:1]}, by: 1, selected: "7f3a", changed: true},
		{name: "↑ with one account, at the first, none selected yet", shown: AccountsShown{Accounts: threeConfigured[:1]}, by: -1, selected: "d28c", changed: true},
		{name: "↑ with one account, at the first", shown: AccountsShown{Accounts: threeConfigured[:1], Selected: "d28c"}, by: -1, selected: "d28c"},
		{name: "↓ on Compare", shown: AccountsShown{Accounts: threeConfigured, Selected: "7f3a"}, by: 1, selected: "7f3a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := tt.shown.Choosing(tt.by, workRunning)
			if got.Chosen != tt.chosen || got.Selected != tt.selected || changed != tt.changed {
				t.Errorf("Choosing(%d) chose %q, selected %q, changed %v; want %q, %q, %v", tt.by, got.Chosen, got.Selected, changed, tt.chosen, tt.selected, tt.changed)
			}
			if got.InSessions != tt.shown.InSessions {
				t.Errorf("Choosing(%d) put ↑↓ in SESSIONS %v, want them where they were, %v", tt.by, got.InSessions, tt.shown.InSessions)
			}
		})
	}
}

// → takes ↑ and ↓ from Detail's list into its SESSIONS, where sessions run
// on the account it shows, putting the selection on the session it lands on,
// so it stays with it as SESSIONS reorder; ← takes them back, the selection
// held; and with one account, which has no list, neither does anything.
func TestRightTakesUpAndDownIntoDetailsSESSIONSAndLeftBack(t *testing.T) {
	work := AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work"}
	inSessions := AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", InSessions: true, Selected: "7f3a"}
	tests := []struct {
		name     string
		shown    AccountsShown
		by       int
		running  []string
		in       bool
		selected string
		changed  bool
	}{
		{name: "→ with sessions", shown: work, by: 1, running: workRunning, in: true, selected: "d28c", changed: true},
		{name: "→ with none", shown: work, by: 1},
		{name: "→ in SESSIONS", shown: inSessions, by: 1, running: workRunning, in: true, selected: "7f3a"},
		{name: "← from SESSIONS", shown: inSessions, by: -1, running: workRunning, selected: "7f3a", changed: true},
		{name: "← in the list", shown: work, by: -1, running: workRunning},
		{name: "→ with one account", shown: AccountsShown{Accounts: threeConfigured[:1]}, by: 1, running: workRunning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := tt.shown.Moved(tt.by, tt.running)
			if got.InSessions != tt.in || got.Selected != tt.selected || changed != tt.changed || got.Chosen != tt.shown.Chosen {
				t.Errorf("Moved(%d) put ↑↓ in SESSIONS %v, selected %q, changed %v, chose %q; want %v, %q, %v, %q",
					tt.by, got.InSessions, got.Selected, changed, got.Chosen, tt.in, tt.selected, tt.changed, tt.shown.Chosen)
			}
		})
	}
}

// While a session runs on the account Detail shows, one is selected, from its
// list as from its SESSIONS: the one selected, where it still runs, else the
// first; none where none runs.
func TestOneSessionIsSelectedWhileAnyRuns(t *testing.T) {
	tests := []struct {
		name     string
		selected string
		running  []string
		want     string
	}{
		{name: "the one selected", selected: "9e21", running: workRunning, want: "9e21"},
		{name: "none selected yet", running: workRunning, want: "d28c"},
		{name: "the one selected ended", selected: "c61b", running: workRunning, want: "d28c"},
		{name: "none running", selected: "9e21"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (AccountsShown{Selected: tt.selected}).Selection(tt.running); got != tt.want {
				t.Errorf("Selection() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The session selected ending gives the selection to the one in its place
// now, the first listed after it still running, by id, however the rest
// moved since, else to the last; with none left, ↑ and ↓ go back to the
// list. One still running keeps it.
func TestTheSelectedSessionEndingSelectsTheOneInItsPlace(t *testing.T) {
	five := []string{"d28c", "c61b", "7f3a", "db8a", "9c2e"}
	tests := []struct {
		name       string
		selected   string
		was, is    []string
		want       string
		inSessions bool
	}{
		{name: "one in the middle ends", selected: "7f3a", was: workRunning, is: []string{"d28c", "9e21"}, want: "9e21", inSessions: true},
		{name: "the last ends", selected: "9e21", was: workRunning, is: []string{"d28c", "7f3a"}, want: "7f3a", inSessions: true},
		{name: "the third of five ends as the rest reorder", selected: "7f3a", was: five, is: []string{"9c2e", "db8a", "c61b", "d28c"}, want: "db8a", inSessions: true},
		{name: "the third of five ends, those after it with it", selected: "7f3a", was: five, is: []string{"c61b", "d28c"}, want: "d28c", inSessions: true},
		{name: "one not listed before ends", selected: "41e0", was: five, is: []string{"c61b", "d28c"}, want: "c61b", inSessions: true},
		{name: "it moves up as another ends", selected: "9e21", was: workRunning, is: []string{"7f3a", "9e21"}, want: "9e21", inSessions: true},
		{name: "the first, as none was moved to, ends", was: workRunning, is: []string{"7f3a", "9e21"}, inSessions: true},
		{name: "none is left", selected: "7f3a", was: workRunning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", InSessions: true, Selected: tt.selected}
			got := s.Following(tt.was, tt.is)
			if got.Selected != tt.want || got.InSessions != tt.inSessions {
				t.Errorf("Following() selected %q, ↑↓ in SESSIONS %v; want %q, %v", got.Selected, got.InSessions, tt.want, tt.inSessions)
			}
		})
	}
}

// A click puts Compare's focus on a column, chooses an account in Detail's
// list, the list taking ↑ and ↓ again, or selects a session in its
// SESSIONS, taking ↑ and ↓ there; each only where it's drawn.
func TestAClickPutsTheFocusOrTheChoiceWhereItLands(t *testing.T) {
	compare := AccountsShown{Accounts: threeConfigured}
	if got := compare.FocusedOn("personal"); !got.Focused || got.Focus != "personal" {
		t.Errorf("a click on personal's heads focused %v on %q, want personal", got.Focused, got.Focus)
	}
	if got := compare.FocusedOn(""); !got.Focused || got.Focus != "" {
		t.Errorf("a click on all accounts' heads focused %v on %q, want all accounts", got.Focused, got.Focus)
	}
	focused := AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "side"}
	if got := focused.FocusedOn("side"); got.View() != AccountsDetail || got.Chosen != "side" {
		t.Errorf("a click on side's heads, focused, showed %d of %q, want Detail of side", got.View(), got.Chosen)
	}

	detail := AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work", InSessions: true, Selected: "7f3a"}
	if got := detail.ChosenOn("side"); got.Chosen != "side" || got.InSessions || got.Selected != "" {
		t.Errorf("a click on side chose %q, ↑↓ in SESSIONS %v, selected %q; want side, in the list, its first session", got.Chosen, got.InSessions, got.Selected)
	}
	if got := detail.ChosenOn("work"); got.Chosen != "work" || got.InSessions || got.Selected != "7f3a" {
		t.Errorf("a click on work, chosen, chose %q, ↑↓ in SESSIONS %v, selected %q; want work, in the list, 7f3a still", got.Chosen, got.InSessions, got.Selected)
	}
	if got := (AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "work"}).SelectedOn("9e21"); got.Selected != "9e21" || !got.InSessions {
		t.Errorf("a click on 9e21 selected %q, ↑↓ in SESSIONS %v; want 9e21, there", got.Selected, got.InSessions)
	}
	if got := (AccountsShown{Accounts: threeConfigured[:1]}).SelectedOn("9e21"); got.Selected != "9e21" || got.InSessions {
		t.Errorf("with one account, a click on 9e21 selected %q, ↑↓ in SESSIONS %v; want 9e21, with no list to leave", got.Selected, got.InSessions)
	}

	if got := detail.FocusedOn("side"); got.Focused {
		t.Error("on Detail, a click focused a Compare column, want none")
	}
	if got := compare.ChosenOn("side"); got.Chosen != "" {
		t.Errorf("on Compare, a click chose %q in Detail's list, want nothing", got.Chosen)
	}
	if got := compare.SelectedOn("9e21"); got.Selected != "" {
		t.Errorf("on Compare, a click selected %q, want nothing", got.Selected)
	}
}

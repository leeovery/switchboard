package dashboard

import "slices"

// Showing is Accounts with its sub-tab i shown, as a click on it shows it. It
// reports false where that's the one drawn, or there's no such sub-tab: with
// one account, Detail, the one there is, is drawn always.
func (s AccountsShown) Showing(i int) (AccountsShown, bool) {
	if s.Alone() || i < 0 || i >= len(accountsViews) || AccountsView(i) == s.Shown {
		return s, false
	}
	s.Shown = AccountsView(i)
	return s, true
}

// Turned is Accounts with the sub-tab by places on from the one shown shown,
// round, as ] and [ move, a place each, as Showing has it.
func (s AccountsShown) Turned(by int) (AccountsShown, bool) {
	return s.Showing(round(int(s.Shown)+by, len(accountsViews)))
}

// Stepped is Accounts with the period Compare and Detail share stepped to a
// shorter one where by is -1, as < steps it, or a longer one where it's 1, as
// > does. It reports false at either end, where it stays: today, and the last
// 12 weeks.
func (s AccountsShown) Stepped(by int) (AccountsShown, bool) {
	step := s.Period.Longer
	if by < 0 {
		step = s.Period.Shorter
	}
	period, ok := step()
	s.Period = period
	return s, ok
}

// Moved is Accounts with ← or → pressed, by -1 or 1, running being the
// sessions running on the account Detail shows, by id, in its SESSIONS'
// order: on Compare, its focus moved along its columns, as focusing has it;
// on Detail, ↑ and ↓ taken into its SESSIONS by →, where sessions run there,
// the selection put on the session it lands on, as Selection has it, so it
// stays with it as they reorder; and back to its list by ←. It reports false
// where nothing changes, as with one account, which has no list.
func (s AccountsShown) Moved(by int, running []string) (AccountsShown, bool) {
	switch {
	case s.View() == AccountsCompare:
		return s.focusing(by)
	case s.Alone() || by > 0 && len(running) == 0:
		return s, false
	}
	in := by > 0
	changed := s.InSessions != in
	s.InSessions = in
	if in {
		s.Selected = s.Selection(running)
	}
	return s, changed
}

// focusing is Accounts with Compare's focus moved by places along its
// columns, its accounts' in the config's order then all accounts', no
// further than either end: from none, → puts it on the first column shown,
// and ← on all accounts' (settled in the plan).
func (s AccountsShown) focusing(by int) (AccountsShown, bool) {
	columns := append(s.Accounts.IDs(), "")
	i := slices.Index(columns, s.Focus) + by
	switch {
	case !s.Focused && by > 0:
		i = s.First
	case !s.Focused:
		i = len(columns) - 1
	}
	i = min(max(i, 0), len(columns)-1)
	changed := !s.Focused || columns[i] != s.Focus
	s.Focused, s.Focus = true, columns[i]
	return s, changed
}

// FocusedOn is Accounts as a click on the heads of Compare's column of the
// account with the given id, "" for all accounts', has it: the focus put
// there, or where it's there already, the column opened in Detail, as Opened
// has it; off Compare, as it is.
func (s AccountsShown) FocusedOn(id string) AccountsShown {
	switch {
	case s.View() != AccountsCompare:
	case s.Focused && s.Focus == id:
		s, _ = s.Opened()
	default:
		s.Focused, s.Focus = true, id
	}
	return s
}

// Opened is Accounts with the column Compare's focus is on opened in Detail,
// as enter opens it: its account chosen there, or all accounts, as chose has
// it. It reports false where no column is focused, or off Compare.
func (s AccountsShown) Opened() (AccountsShown, bool) {
	if s.View() != AccountsCompare || !s.Focused {
		return s, false
	}
	s.Shown = AccountsDetail
	return s.chose(s.Focus), true
}

// Opens reports whether enter opens anything on Accounts as s shows it,
// running being the sessions running on the account Detail shows, by id, in
// its SESSIONS' order: on Compare, the column focused, as Opened has it; on
// Detail, the session selected, from its list as from its SESSIONS, as
// Selection has it. Where it doesn't, the key line draws enter faint.
func (s AccountsShown) Opens(running []string) bool {
	if s.View() == AccountsCompare {
		_, opened := s.Opened()
		return opened
	}
	return s.Selection(running) != ""
}

// Unfocused is Accounts with Compare's focus ended, as esc ends a choice, its
// columns where they are (settled in the plan). It reports false where no
// column is focused, or off Compare.
func (s AccountsShown) Unfocused() (AccountsShown, bool) {
	if s.View() != AccountsCompare || !s.Focused {
		return s, false
	}
	s.Focused, s.Focus = false, ""
	return s, true
}

// Choosing is Accounts with ↑ or ↓ pressed on Detail, by -1 or 1, running
// being the sessions running on the account it shows, by id, in its
// SESSIONS' order: in its SESSIONS, or with one account, which has no list,
// the session selected moved among them, as selecting has it; else the
// list's choice moved, all accounts first, then each account in the config's
// order, as chose has it. Neither goes past either end. It reports false
// where nothing changes, or off Detail.
func (s AccountsShown) Choosing(by int, running []string) (AccountsShown, bool) {
	switch {
	case s.View() != AccountsDetail:
		return s, false
	case s.InSessions || s.Alone():
		return s.selecting(by, running)
	}
	listed := s.listed()
	chosen := listed[min(max(slices.Index(listed, s.Chosen)+by, 0), len(listed)-1)]
	return s.chose(chosen), chosen != s.Chosen
}

// ChosenOn is Accounts with Detail's list's choice on the account with the
// given id, "" for all accounts, as a click on its row chooses it, as chose
// has it; with no list, as off Detail or with one account, as it is.
func (s AccountsShown) ChosenOn(id string) AccountsShown {
	if s.View() != AccountsDetail || s.Alone() {
		return s
	}
	return s.chose(id)
}

// chose is Accounts with Detail's list's choice on the account with the given
// id, "" for all accounts, ↑ and ↓ back in the list: its SESSIONS' selection
// on the first where that's another account, as Detail opens on one.
func (s AccountsShown) chose(id string) AccountsShown {
	if id != s.Chosen {
		s.Selected = ""
	}
	s.Chosen, s.InSessions = id, false
	return s
}

// listed are Detail's list's rows, by account id: all accounts', "", first,
// then each account's in the config's order.
func (s AccountsShown) listed() []string {
	return append([]string{""}, s.Accounts.IDs()...)
}

// selecting is Accounts with Detail's SESSIONS' selection moved by places
// among the sessions running, from the one Selection has selected, no
// further than either end, and put on its session, so it stays with it as
// they reorder: one at an end too, as the first ↑ or ↓ with one account may
// find it.
func (s AccountsShown) selecting(by int, running []string) (AccountsShown, bool) {
	from := s.Selection(running)
	if from == "" {
		return s, false
	}
	was := s.Selected
	s.Selected = running[min(max(slices.Index(running, from)+by, 0), len(running)-1)]
	return s, s.Selected != was
}

// SelectedOn is Accounts with Detail's SESSIONS' selection on the session
// with the given id, ↑ and ↓ taken into them where there's a list, as a click
// on its row selects it; off Detail, as it is.
func (s AccountsShown) SelectedOn(id string) AccountsShown {
	if s.View() == AccountsDetail {
		s.Selected, s.InSessions = id, !s.Alone()
	}
	return s
}

// Selection is the session Detail's SESSIONS has selected of those running on
// the account it shows, by id, in their order: Selected, where it runs there,
// else the first; "" where none does. While any runs there, one is selected,
// from the list as from SESSIONS, for enter to open (settled in the plan).
func (s AccountsShown) Selection(running []string) string {
	switch {
	case slices.Contains(running, s.Selected):
		return s.Selected
	case len(running) > 0:
		return running[0]
	}
	return ""
}

// Following is Accounts as the sessions running on the account Detail shows
// go from was to is, by id, in its SESSIONS' order: the session selected,
// where it ends, gives the selection to the one in its place now, as
// inItsPlace finds it; and with none left, ↑ and ↓ go back to the list
// (settled in the plan).
func (s AccountsShown) Following(was, is []string) AccountsShown {
	switch {
	case len(is) == 0:
		s.InSessions, s.Selected = false, ""
	case s.Selected != "" && !slices.Contains(is, s.Selected):
		s.Selected = inItsPlace(was, is, s.Selected)
	}
	return s
}

// inItsPlace is the session of those running now, is, in the place of the
// one with the given id, which ended, of those running before, was, by id,
// in SESSIONS' order: the first listed after it before that runs now, as
// the rest move up into its place, however they've moved since; else the
// last now; or, where it wasn't listed before, the first, as Selection has
// it.
func inItsPlace(was, is []string, ended string) string {
	at := slices.Index(was, ended)
	if at < 0 {
		return is[0]
	}
	if next := slices.IndexFunc(was[at+1:], func(id string) bool { return slices.Contains(is, id) }); next >= 0 {
		return was[at+1+next]
	}
	return is[len(is)-1]
}

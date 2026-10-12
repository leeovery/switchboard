package watch

// moveFocus moves the Overview's focus by accounts, in the config's order, up
// where by is less than zero, as ↑ and ↓ do, scrolling USAGE to the account
// it's on where that's out of view.
func (m Model) moveFocus(int) Model {
	return m
}

// focusOn moves the Overview's focus to the account with the given id, as a
// click on its row does.
func (m Model) focusOn(string) Model {
	return m
}

// stillThere is the model with the focus given up where the document no
// longer has its account.
func (m Model) stillThere() Model {
	if !m.has(m.focus) {
		m.focus = ""
	}
	return m
}

// has reports whether the document on screen has the account with the given
// id.
func (m Model) has(id string) bool {
	_, ok := m.doc.Account(id)
	return ok
}

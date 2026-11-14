package dashboard

// accountsTokens is Accounts' Tokens, as the frame has it: its rows the
// dial's period's, as its Rows has them, led by the whole period's total,
// split as v has it, filling the view to its foot.
func (f Frame) accountsTokens() tokensTable {
	s := f.Accounts
	return tokensTable{
		page: s.Tokens, rows: f.Period.Rows(), period: f.Period, split: s.TokensSplit, accounts: s.Accounts, fills: true,
		width: f.Width, viewRows: f.viewRows(), scroll: f.Scroll,
	}
}

// tokensForm is Tokens on Accounts as its form draws it: drawn as its table
// draws it, scrolling as it does, a row at a time, its rows under its heads
// what scrolls. It's nothing till its page is built.
func (f Frame) tokensForm() accountsForm {
	if f.Accounts.Tokens == nil {
		return accountsForm{}
	}
	t, top := f.accountsTokens(), f.top()
	return accountsForm{
		draw:      t.draw,
		scrolling: t.scrolling(top),
		scrolls: func() (x, y, width, rows int) {
			placed, _ := t.placed(top)
			return 0, placed.first, f.Width, placed.rows
		},
	}
}

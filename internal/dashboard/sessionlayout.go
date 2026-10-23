package dashboard

// A session's page keeps rows for leastTurns turns at the least, beside its
// accounts, which give way to a row saying how many more there are before
// they do; and an open turn shows mostRequests of its requests at a time.
const (
	leastTurns   = 3
	mostRequests = 5
)

// The rows a session's story takes: under the wide chrome, from its heading
// to its axis, and folded to its heading alone; and under a phone's, from
// its label to its axis, as it has no heading.
const (
	wideStoryRows   = phoneStoryRows + 1
	foldedStoryRows = 1
	phoneStoryRows  = 1 + storyColumnRows + 2
)

// rowsBesideTurns are the rows a session's page keeps under its accounts
// besides its turns' own: the blank row and TURNS' heading under them, the
// blank row and the turns' column heads under that, the row saying how many
// more turns there are, and the key line.
const rowsBesideTurns = 6

// sessionLayout is how a session's page lays out its rows at the frame's
// size: its heading and its story's heading in the wide chrome's form, or
// a phone's, as chromeWide has it; its chrome alone, where it's too short
// for the rest; its story whole, or else, wide, folded to its heading, and
// on a phone left out; and from what row each part starts, and how many
// rows its accounts, its turns and an open turn's requests take.
type sessionLayout struct {
	wide, chrome, story bool
	// heading is the heading's row, now the first of the two that say what
	// it's doing, and storyAt the story's first.
	heading, now, storyAt int
	// accountsAt is the first account's row; accounts how many of them have
	// rows; and more how many are left out, said on the row after them.
	accountsAt, accounts, more int
	// turnsHeading is TURNS' row, turnsAt the newest turn's, turns how many
	// rows the turns take, and moreTurns the row saying how many more there
	// are.
	turnsHeading, turnsAt, turns, moreTurns int
	// requests is how many of an open turn's requests show at a time: as
	// many as the turns' rows hold under it, its heads and its └ line, five
	// at the most and one at the least.
	requests int
}

// pageLayout is how the page of the session open lays out its rows, as
// sessionLayout lays them out, heading itself as chromeWide has it.
func (f Frame) pageLayout() sessionLayout {
	return f.sessionLayout(len(f.Session.Page.Accounts), f.chromeWide())
}

// sessionLayout lays out the page of a session that ran on the accounts
// given, at the frame's size, heading itself in the wide chrome's form where
// wide says, else a phone's: its story whole where its fixed rows, one
// account and leastTurns turns fit, else folded, under the wide chrome, or
// left out, under a phone's, where they fit then, else the chrome alone. Its
// accounts each take a row while leastTurns turns fit beside them, else as
// many as leave a row saying how many more, and the turns the rest.
func (f Frame) sessionLayout(accounts int, wide bool) sessionLayout {
	l := sessionLayout{wide: wide, heading: f.headingY()}
	l.now = l.heading + 2
	l.storyAt = l.now + 3
	whole, folded := wideStoryRows, foldedStoryRows
	if !l.wide {
		whole, folded = phoneStoryRows, 0
	}
	l.story = l.room(f.Height, l.accountsAfter(whole)) >= 1+leastTurns
	storyRows := whole
	if !l.story {
		storyRows = folded
	}
	l.accountsAt = l.accountsAfter(storyRows)
	room := l.room(f.Height, l.accountsAt)
	if room < 1+leastTurns {
		return sessionLayout{wide: l.wide, chrome: true}
	}
	l.accounts = accounts
	if accounts+leastTurns > room {
		l.accounts = room - leastTurns - 1
		l.more = accounts - l.accounts
	}
	l.turnsHeading = l.accountsAt + l.accounts + min(l.more, 1) + 1
	l.turnsAt = l.turnsHeading + 3
	l.moreTurns = f.Height - 2
	l.turns = l.moreTurns - l.turnsAt
	l.requests = min(max(l.turns-3, 1), mostRequests)
	return l
}

// accountsAfter is the row a session's accounts start at, below a story of
// the rows given and the blank row after it, or right after its now rows,
// where it has none.
func (l sessionLayout) accountsAfter(storyRows int) int {
	if storyRows == 0 {
		return l.storyAt
	}
	return l.storyAt + storyRows + 1
}

// room is how many rows a session's page the given height has for its
// accounts and its turns together, its accounts starting at the row given:
// all but rowsBesideTurns.
func (l sessionLayout) room(height, accountsAt int) int {
	return height - accountsAt - rowsBesideTurns
}

// chromeWide reports whether a session's page heads itself as the wide
// chrome does, its heading and its story's heading in their wide forms,
// rather than a phone's, its story labelled alone: where the chrome's tabs
// share the title's row. The page's own chrome follows the page's, so it
// reads the breakpoint where the chrome has drawn it, rather than choosing
// again.
func (f Frame) chromeWide() bool {
	_, tabsRow, _ := f.tabsAt()
	return tabsRow == 0
}

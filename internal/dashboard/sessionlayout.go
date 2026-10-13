package dashboard

// sessionWideFrom is the width a session's page draws its wide form from:
// where its widest fixed row, the newest turn's ↑ asking, ends. Narrower, it
// draws a phone's.
const sessionWideFrom = 122

// A session's page keeps rows for leastTurns turns at the least, beside its
// accounts, which give way to a row saying how many more there are before
// they do; and an open turn shows mostRequests of its requests at a time.
const (
	leastTurns   = 3
	mostRequests = 5
)

// The rows a session's story takes: the wide page's, from its heading to its
// axis, and folded to its heading alone; and a phone's, from its label to
// its axis, as it has no heading.
const (
	wideStoryRows   = 9
	foldedStoryRows = 1
	phoneStoryRows  = 7
)

// sessionLayout is how a session's page lays out its rows at the frame's
// size: in its wide form, or a phone's; its chrome alone, where it's too
// short for the rest; its story whole, or else, wide, folded to its heading,
// and on a phone left out; and from what row each part starts, and how many
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

// sessionLayout lays out the page of a session that ran on the accounts
// given, at the frame's size: its story whole where its fixed rows, one
// account and leastTurns turns fit, else folded, wide, or left out, on a
// phone, where they fit then, else the chrome alone. Its accounts each
// take a row while leastTurns turns fit beside them, else as many as leave
// a row saying how many more, and the turns the rest.
func (f Frame) sessionLayout(accounts int) sessionLayout {
	l := sessionLayout{wide: f.Width >= sessionWideFrom, heading: f.headingY()}
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
	l.turnsAt = l.turnsHeading + 2
	if l.wide {
		l.turnsAt++
	}
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
// all but those of the blank row and TURNS' heading under them, the blank
// row, and wide, the column heads under that, the row saying how many more
// turns there are, and the key line.
func (l sessionLayout) room(height, accountsAt int) int {
	under := 5
	if l.wide {
		under++
	}
	return height - accountsAt - under
}

package dashboard

import "github.com/leeovery/switchboard/internal/dashboard/flex"

// A session's page keeps rows for turnsNatural turns, as many as a turn
// opened takes at its natural, its row, its heads, requestsNatural of its
// requests and its foot, till its story's last row of columns has given
// way; for leastTurns till its story and its accounts have; and for one at
// the least.
const (
	requestsNatural = 5
	turnsNatural    = 1 + 1 + requestsNatural + 1
	leastTurns      = 3
)

// The rows a session's story takes besides its columns: under the wide
// chrome, its heading, a move's label, its ribbon and its axis; under a
// phone's, the same but its heading, as it has none; and folded, under the
// wide chrome, its heading alone, and under a phone's, none.
const (
	wideStoryRows   = 1 + phoneStoryRows
	phoneStoryRows  = 3
	foldedStoryRows = 1
)

// sessionPart is one of a session's page's parts down the frame, from its
// heading to the row saying how many more turns there are, in the order
// they're drawn.
type sessionPart int

const (
	sessionHeadingPart sessionPart = iota
	underHeadingPart
	sessionNowPart
	underNowPart
	storyPart
	storyRowPart
	underStoryPart
	stayRowsPart
	overTurnsPart
	turnsHeadingPart
	underTurnsHeadingPart
	turnHeadsPart
	newestTurnPart
	turnsLowPart
	turnsHighPart
	turnsSparePart
	moreTurnsPart
)

// The turns a session's page's parts give way in as its rows run short:
// the story's last column row; the turns past leastTurns; the story,
// folded to its heading, or left out; the accounts, to a row saying how
// many; the turns past one; and the blank rows, one at a time, the one
// under TURNS' heading first and the one under the page's heading last.
const (
	storyRowGivesWay = iota + 1
	turnsHighGiveWay
	storyGivesWay
	staysGiveWay
	turnsLowGiveWay
	underTurnsHeadingGivesWay
	overTurnsGivesWay
	underStoryGivesWay
	underNowGivesWay
	underHeadingGivesWay
)

// sessionLayout is how a session's page lays out its rows at the frame's
// size: its heading and its story's heading in the wide chrome's form, or
// a phone's, as chromeWide has it; its chrome alone, where it's too short
// for the rest; its story whole, or else, wide, folded to its heading, and
// on a phone left out; and from what row each part starts, and how many
// rows its story's columns, its accounts and its turns take.
type sessionLayout struct {
	wide, chrome, story bool
	// heading is the heading's row, now the first of the two that say what
	// it's doing, storyAt the story's first, and body how many rows its
	// columns take.
	heading, now, storyAt, body int
	// accountsAt is the first account's row; accounts how many of them have
	// rows; and more how many are left out, said on the row after them.
	accountsAt, accounts, more int
	// turnsHeading is TURNS' row, heads its column heads', turnsAt the
	// newest turn's, turns how many rows the turns take, and moreTurns the
	// row saying how many more there are.
	turnsHeading, heads, turnsAt, turns, moreTurns int
}

// pageLayout is how the page of the session open lays out its rows, as
// sessionLayout lays them out, heading itself as chromeWide has it.
func (f Frame) pageLayout() sessionLayout {
	return f.sessionLayout(len(f.Session.Page.Accounts), f.chromeWide())
}

// sessionLayout lays out the page of a session that ran on the accounts
// given, at the frame's size, heading itself in the wide chrome's form where
// wide says, else a phone's, its parts where flex lays them from its
// heading to the row over the key line, as sessionParts has them; or its
// chrome alone, where even their leasts don't fit.
func (f Frame) sessionLayout(accounts int, wide bool) sessionLayout {
	heading := f.headingY()
	at, fits := flex.Lay(f.Height-1-heading, sessionParts(accounts, wide)...)
	if !fits {
		return sessionLayout{wide: wide, chrome: true}
	}
	row := func(p sessionPart) int { return heading + at[p].At }
	l := sessionLayout{
		wide: wide, story: !at[storyPart].Left, heading: heading, now: row(sessionNowPart), storyAt: row(storyPart),
		accountsAt: row(stayRowsPart), turnsHeading: row(turnsHeadingPart), heads: row(turnHeadsPart),
		turnsAt: row(newestTurnPart), moreTurns: row(moreTurnsPart),
		turns: at[newestTurnPart].Size + at[turnsLowPart].Size + at[turnsHighPart].Size + at[turnsSparePart].Size,
	}
	if l.story {
		l.body = storyColumnRows - 1 + at[storyRowPart].Size
	}
	l.accounts = at[stayRowsPart].Size
	if l.accounts < accounts {
		l.accounts = max(l.accounts-1, 0)
		l.more = accounts - l.accounts
	}
	return l
}

// storyLeast are the rows a story whole takes but its columns' last, the
// first row of the page to give way.
func storyLeast(wide bool) int {
	besides := phoneStoryRows
	if wide {
		besides = wideStoryRows
	}
	return besides + storyColumnRows - 1
}

// sessionParts are a session's page's parts down the frame, as flex lays
// them, of a session that ran on the accounts given, under the wide chrome
// where wide says, else a phone's: each fixed, but the story's last column
// row, the story, its accounts and its turns, which give way in the turns
// storyRowGivesWay to underHeadingGivesWay name, and the turns past
// turnsNatural, which take what's left. Under a phone's chrome the story is
// left out with the blank row under it, rather than folded.
func sessionParts(accounts int, wide bool) []flex.Part {
	fixed := func(rows int) flex.Part { return flex.Part{Least: rows, Natural: rows} }
	blank := func(givesWay int) flex.Part { return flex.Part{Least: 1, Natural: 1, Leaves: givesWay} }
	story, underStory := flex.Part{Least: storyLeast(wide), Natural: storyLeast(wide), Leaves: storyGivesWay}, blank(underStoryGivesWay)
	if wide {
		story.Kept = foldedStoryRows
	} else {
		underStory.Leaves = storyGivesWay
	}
	return []flex.Part{
		sessionHeadingPart:    fixed(1),
		underHeadingPart:      blank(underHeadingGivesWay),
		sessionNowPart:        fixed(2),
		underNowPart:          blank(underNowGivesWay),
		storyPart:             story,
		storyRowPart:          {Natural: 1, Narrows: storyRowGivesWay},
		underStoryPart:        underStory,
		stayRowsPart:          {Least: min(accounts, 1), Natural: accounts, Narrows: staysGiveWay},
		overTurnsPart:         blank(overTurnsGivesWay),
		turnsHeadingPart:      fixed(1),
		underTurnsHeadingPart: blank(underTurnsHeadingGivesWay),
		turnHeadsPart:         fixed(1),
		newestTurnPart:        fixed(1),
		turnsLowPart:          {Natural: leastTurns - 1, Narrows: turnsLowGiveWay},
		turnsHighPart:         {Natural: turnsNatural - leastTurns, Narrows: turnsHighGiveWay},
		turnsSparePart:        {Grows: true},
		moreTurnsPart:         fixed(1),
	}
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

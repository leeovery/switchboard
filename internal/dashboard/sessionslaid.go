package dashboard

import (
	"cmp"
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/views"
)

// The Switchboard's columns as the design fixes them at their widest, at 160
// columns and wider: a call's id at the first column, cut to callID, a cell,
// and its directory, cut to callDir; a cell, then what it's doing, in
// callDoing with the gap after it; then where it came from, "▸ from ", in
// fromLong cells, or "▸ " alone, in fromShort, and the account, cut to
// fromAccount; plugGap cells, then the plugs. The bends' room runs from the
// column after the plugs to the first bend, bendFromJacks left of the jacks,
// clear by bendClear of a cord hanging loose looseCells long; and the lines
// start lineGap after the jacks, their bars lineBar and their words
// lineWords, or lineShortWords in usage's short form, besides lineBeside of label,
// percentage and gaps.
const (
	callID         = 4
	callDir        = 12
	callDoing      = 13
	fromLong       = 7
	fromShort      = 2
	fromAccount    = 8
	plugGap        = 2
	looseCells     = 9
	bendClear      = 3
	bendFromJacks  = looseCells + bendClear
	lineGap        = 2
	lineBar        = 32
	lineWords      = 22
	lineShortWords = 14
	lineBeside     = 20
)

// The fewest cells each part narrows to, as the narrowing table has them:
// the bends' room, the lines' bars, first and last, the calls' directory, and
// the account after ▸. Bent cords turn down bendsApart apart, or closer; a
// stub is stubCells long, each next a cell shorter, down to stubLeast.
const (
	leastRoom    = 9
	leastWideBar = 16
	leastDir     = 6
	leastBar     = 8
	leastAccount = 6
	bendsApart   = 3
	stubCells    = 7
	stubLeast    = 1
)

// boardColumns are where the Switchboard's parts sit across a frame, as the
// narrowing table gives them for its width: a call's parts, its id at the
// first column; the plugs' column, the bends' room's columns after it, the
// jacks' column and the column the lines start at; and the cells of the
// lines' bars and words, the words in usage's short form where shortWords
// says.
type boardColumns struct {
	callColumns
	plug, room, jacks int
	lines             int
	bar, words        int
	shortWords        bool
}

// The turns the Switchboard's parts give way in as it narrows, in the order
// the design's narrowing table has them: the lines' bars to leastWideBar, the
// calls' directory to leastDir, ▸'s from dropped, the lines' words in usage's
// short form, the lines' bars to leastBar, and the account after ▸ to
// leastAccount.
const (
	barsToWide = iota + 1
	dirToLeast
	fromDropped
	wordsShortened
	barsToLeast
	accountToLeast
)

// The Switchboard's parts across a frame, in boardParts' order.
const (
	boardID = iota
	boardDir
	boardDoing
	boardFrom
	boardAccount
	boardRoom
	boardJacks
	boardBeside
	boardBarsPast
	boardBars
	boardWords
)

// boardParts are the Switchboard's parts across a frame, as flex lays them:
// a call's id, its directory a cell on, what it's doing a cell on, ▸ and its
// from, and the account it came from; the bends' room, plugGap cells on,
// which takes what's left; the jacks' bendFromJacks; and the lines, lineGap
// cells on: their label, percentage and gaps, their bars, those past
// leastWideBar and those up to it each narrowing in a turn of its own, and
// their words.
var boardParts = []flex.Part{
	boardID:       {Least: callID, Natural: callID},
	boardDir:      {Least: leastDir, Natural: callDir, Narrows: dirToLeast, Gap: 1},
	boardDoing:    {Least: callDoing, Natural: callDoing, Gap: 1},
	boardFrom:     {Least: fromLong, Natural: fromLong, Leaves: fromDropped, Kept: fromShort},
	boardAccount:  {Least: leastAccount, Natural: fromAccount, Narrows: accountToLeast},
	boardRoom:     {Least: leastRoom, Natural: leastRoom, Grows: true, Gap: plugGap},
	boardJacks:    {Least: bendFromJacks, Natural: bendFromJacks},
	boardBeside:   {Least: lineBeside, Natural: lineBeside, Gap: lineGap},
	boardBarsPast: {Natural: lineBar - leastWideBar, Narrows: barsToWide},
	boardBars:     {Least: leastBar, Natural: leastWideBar, Narrows: barsToLeast},
	boardWords:    {Least: lineWords, Natural: lineWords, Leaves: wordsShortened, Kept: lineShortWords},
}

// boardColumnsOf are the Switchboard's columns at width, its parts laid as
// boardParts declares them, and whether they fit: under 100 columns they
// don't.
func boardColumnsOf(width int) (boardColumns, bool) {
	s, fits := flex.Lay(width, boardParts...)
	return boardColumns{
		id:         s[boardID].At,
		dir:        s[boardDir].Size,
		doing:      s[boardDoing].At,
		from:       s[boardFrom].At,
		account:    s[boardAccount].Size,
		fromShort:  s[boardFrom].Left,
		plug:       s[boardRoom].At,
		room:       s[boardRoom].Size,
		jacks:      s[boardJacks].At + s[boardJacks].Size,
		lines:      s[boardBeside].At,
		bar:        s[boardBarsPast].Size + s[boardBars].Size,
		words:      s[boardWords].Size,
		shortWords: s[boardWords].Left,
	}, fits
}

// firstBend is the column the first bent cord turns down at: the bends'
// room's last.
func (k boardColumns) firstBend() int {
	return k.plug + k.room
}

// boardLines are the Switchboard's lines as its layout reads them: the
// accounts', by their ids, in the config's order; and how many windows each
// draws a row of, as usage --by account draws them.
type boardLines struct {
	accounts []string
	windows  int
}

// boardLayout is the Switchboard laid out at a frame's size, as sessionsLaid
// lays it: its columns; whether it draws lanes in place of cords; the
// compaction step its lines take, 1 to 5; its lines, in the config's order;
// its calls, top to bottom; the row its key starts on, -1 where it gives way,
// at step 5; and the rows its calls and lines take. Rows count from the
// view's top.
type boardLayout struct {
	columns boardColumns
	lanes   bool
	step    int
	lines   []laidLine
	calls   []laidCall
	keyAt   int
	rows    int
}

// laidLine is an account's line laid out: its account, by its id, and its
// line as the patch keeps it; its first row and how many it takes, a jack a
// row, grown a row for each jack plugged past its own; the first of its
// account's rows, its group's first call's or its own, whichever is higher,
// which it scrolls from; and the stubs hanging from its jacks. Its group
// starts on groupStart, where it has one. Its account's rows run from first
// to its own last, as end has it, as each of its calls sits on its jack's
// row or above it.
type laidLine struct {
	account    string
	patched    views.PatchedLine
	top, rows  int
	first      int
	stubs      []laidStub
	groupStart int
}

// end is the row after its account's last: its own last's.
func (ln laidLine) end() int {
	return ln.top + ln.rows
}

// tall is how many rows its account takes, from its first to its last.
func (ln laidLine) tall() int {
	return ln.end() - ln.first
}

// laidStub is a cord a limit let go, hanging from the jack on row: cells
// long, to the jack's left.
type laidStub struct {
	row, cells int
}

// laidCall is a call laid out: its session, by its id, and the account its
// row is on, by its; and its cord, from its plug on its row to its jack's
// row, bent where those differ.
type laidCall struct {
	session, account string
	cord             cord
}

// bent reports whether the call's cord turns down to its jack.
func (c laidCall) bent() bool {
	return c.cord.from != c.cord.to
}

// boardKeyRows are the rows the key takes, and boardKeyGap the blank rows over it.
const (
	boardKeyRows = 2
	boardKeyGap  = 2
)

// compactedStep is the compaction step whose lines scroll with their calls,
// the key giving way.
const compactedStep = 5

// sessionsLaid lays the Switchboard out width columns wide, in a view rows
// tall, with who sits where as patched keeps it, on the lines given. Its
// calls are grouped by the account they're on, in the lines' order, a blank
// row after each group, an empty group taking none. It takes the first
// compaction step whose lines end boardKeyGap blank rows and the key's
// boardKeyRows above the view's last row, else step 5, step 4's lines
// scrolling with no key, each account's group starting below the line
// above, as layLines has it. Each line follows the one before at its step's
// gap, or lower, to meet its group, so every cord runs right, then down; and
// grows a row for each jack its calls and stubs plug past its rows. Bent
// cords turn down in the order of their jacks' rows, then their calls', the
// first at the bends' room's last column, each next bendsApart further left,
// or 2, or 1, where the room holds them no further apart. It draws lanes in
// place of cords where its parts don't fit, as under 100 columns, and
// wherever the bent cords outnumber the bends' room's columns.
func sessionsLaid(width, rows int, patched views.Patched, lines boardLines) boardLayout {
	k, fits := boardColumnsOf(width)
	l := boardLayout{columns: k, lanes: !fits, keyAt: -1}
	for l.step = 1; l.step < compactedStep; l.step++ {
		l.layLines(patched, lines)
		if l.rows+boardKeyGap+boardKeyRows <= rows {
			l.keyAt = l.rows + boardKeyGap
			return l.bend()
		}
	}
	l.layLines(patched, lines)
	return l.bend()
}

// layLines lays out the calls and the lines at l's step, and the rows they
// take: each group of calls from the row after the group above's blank row,
// and each line from its step's gap after the line above, or lower, as laid
// has it. At step 5, where the lines scroll an account at a time, a group
// starts no higher than the row after the line above, so each account's
// rows, from its first to its line's last, as laidLine has them, start
// below the account above's.
func (l *boardLayout) layLines(patched views.Patched, lines boardLines) {
	l.calls, l.lines = nil, nil
	group, next, end := 0, 0, 0
	for _, account := range lines.accounts {
		p := patched.Line(account)
		if l.step == compactedStep {
			group = max(group, end)
		}
		ln := laidLine{account: account, patched: p, groupStart: group}
		for i, c := range p.Calls {
			l.calls = append(l.calls, laidCall{session: c.Session, account: account, cord: cord{plug: l.columns.plug, from: group + i}})
		}
		if len(p.Calls) > 0 {
			group += len(p.Calls) + 1
		}
		end = ln.laid(l.step, lines.windows, next)
		next = end + l.gapAfter()
		l.lines = append(l.lines, ln)
	}
	l.rows = end
	l.plugged()
}

// laid lays the line out at the step given, of windows windows, from row next
// or lower, so each of its calls is on its jack's row or above it, and
// returns the row after it.
func (ln *laidLine) laid(step, windows, next int) int {
	ln.top, ln.rows = next, lineRows(step, windows)
	for i, c := range ln.patched.Calls {
		ln.top = max(ln.top, ln.groupStart+i-c.Jack)
		ln.rows = max(ln.rows, c.Jack+1)
	}
	ln.first = ln.top
	if len(ln.patched.Calls) > 0 {
		ln.first = min(ln.top, ln.groupStart)
	}
	ln.stubs = nil
	for i, jack := range ln.patched.Stubs {
		ln.rows = max(ln.rows, jack+1)
		ln.stubs = append(ln.stubs, laidStub{row: ln.top + jack, cells: max(stubCells-i, stubLeast)})
	}
	return ln.top + ln.rows
}

// lineRows are the rows a line of windows windows takes at a step, before it
// grows: at step 1, its name row and a row a window, a blank row between
// windows; at step 2, without those blank rows; at step 3, two rows; and at
// steps 4 and 5, one.
func lineRows(step, windows int) int {
	switch step {
	case 1:
		return max(2*windows, 1)
	case 2:
		return 1 + windows
	case 3:
		return 2
	}
	return 1
}

// gapAfter is the blank rows after each line at l's step: two at step 1, one
// at step 2, and none from step 3.
func (l boardLayout) gapAfter() int {
	switch l.step {
	case 1:
		return 2
	case 2:
		return 1
	}
	return 0
}

// plugged has each call's cord end on its jack's row, on its line.
func (l *boardLayout) plugged() {
	i := 0
	for _, ln := range l.lines {
		for _, c := range ln.patched.Calls {
			l.calls[i].cord.to = ln.top + c.Jack
			i++
		}
	}
}

// bend is l with its bent cords' bends set, as sessionsLaid has them, or
// drawing lanes, where the bends' room can't hold them a column apart.
func (l boardLayout) bend() boardLayout {
	var bent []int
	for i, c := range l.calls {
		if c.bent() {
			bent = append(bent, i)
		}
	}
	slices.SortStableFunc(bent, func(a, b int) int {
		return cmp.Or(cmp.Compare(l.calls[a].cord.to, l.calls[b].cord.to), cmp.Compare(l.calls[a].cord.from, l.calls[b].cord.from))
	})
	apart, ok := bendsApartIn(l.columns.room, len(bent))
	if !ok {
		l.lanes = true
		return l
	}
	for j, i := range bent {
		l.calls[i].cord.bend = l.columns.firstBend() - j*apart
	}
	return l
}

// bendsApartIn is how far apart n bent cords turn down in a bends' room of
// room columns: bendsApart, or closer, a column at the least, reporting
// false where even that doesn't hold them.
func bendsApartIn(room, n int) (int, bool) {
	for apart := bendsApart; apart >= 1; apart-- {
		if (n-1)*apart <= room-1 {
			return apart, true
		}
	}
	return 0, false
}

// line is the line of the account with the given id, reporting false where
// the layout has none.
func (l boardLayout) line(account string) (laidLine, bool) {
	i := slices.IndexFunc(l.lines, func(ln laidLine) bool { return ln.account == account })
	if i < 0 {
		return laidLine{}, false
	}
	return l.lines[i], true
}

// jackRow is the row of the jack given of the line of the account with the
// given id, reporting false where the layout has no such line.
func (l boardLayout) jackRow(account string, jack int) (int, bool) {
	ln, ok := l.line(account)
	return ln.top + jack, ok
}

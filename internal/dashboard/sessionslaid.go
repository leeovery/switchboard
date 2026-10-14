package dashboard

import (
	"cmp"
	"slices"

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

// boardColumnsOf are the Switchboard's columns at width, as the narrowing
// table has them: at their widest, the bends' room taking what's left; and
// where that leaves it short of leastRoom, each part in turn giving way, as
// narrowing has them, until it has its leastRoom, or no part has more to
// give, as under 100 columns, which draws lanes.
func boardColumnsOf(width int) boardColumns {
	k := boardColumns{dir: callDir, account: fromAccount, bar: lineBar, words: lineWords}
	k.room = width - k.fixed()
	for _, give := range narrowing {
		if k.room >= leastRoom {
			break
		}
		give(&k, leastRoom-k.room)
		k.room = width - k.fixed()
	}
	return k.placed()
}

// narrowing are the parts that give way as the Switchboard narrows, in the
// order the design's table has them, each given the cells the bends' room
// lacks: the lines' bars to leastWideBar, a cell for each column lost; the
// calls' directory to leastDir; ▸'s from dropped, freeing its cells at once;
// the lines' words in usage's short form, freeing theirs at once; the lines'
// bars to leastBar; and the account after ▸ to leastAccount. What a part
// frees at once beyond what's lacking goes to the bends' room, which gives it
// back a column at a time, as each next part waits till it falls short again.
var narrowing = []func(k *boardColumns, lacking int){
	func(k *boardColumns, lacking int) { k.bar = narrowed(k.bar, leastWideBar, lacking) },
	func(k *boardColumns, lacking int) { k.dir = narrowed(k.dir, leastDir, lacking) },
	func(k *boardColumns, _ int) { k.fromShort = true },
	func(k *boardColumns, _ int) { k.shortWords, k.words = true, lineShortWords },
	func(k *boardColumns, lacking int) { k.bar = narrowed(k.bar, leastBar, lacking) },
	func(k *boardColumns, lacking int) { k.account = narrowed(k.account, leastAccount, lacking) },
}

// narrowed is cells less what's lacking, but no fewer than least.
func narrowed(cells, least, lacking int) int {
	return max(cells-lacking, least)
}

// fixed are the columns every part but the bends' room takes: the calls to
// their plugs, the jacks' bendFromJacks, the gap to the lines, and the lines.
func (k boardColumns) fixed() int {
	return k.plugAt() + bendFromJacks + lineGap + lineBeside + k.bar + k.words
}

// plugAt is the plugs' column: after the id, its directory, what it's doing
// and where it came from, plugGap cells on.
func (k boardColumns) plugAt() int {
	from := fromLong
	if k.fromShort {
		from = fromShort
	}
	return callID + 1 + k.dir + 1 + callDoing + from + k.account + plugGap
}

// placed is k with its parts' columns worked out from their cells.
func (k boardColumns) placed() boardColumns {
	k.doing = callID + 1 + k.dir + 1
	k.from = k.doing + callDoing
	k.plug = k.plugAt()
	k.jacks = k.plug + k.room + bendFromJacks
	k.lines = k.jacks + lineGap
	return k
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
// starts on groupStart, where it has one.
type laidLine struct {
	account    string
	patched    views.PatchedLine
	top, rows  int
	first      int
	stubs      []laidStub
	groupStart int
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
// scrolling with no key. Each line follows the one before at its step's gap,
// or lower, to meet its group, so every cord runs right, then down; and grows
// a row for each jack its calls and stubs plug past its rows. Bent cords turn
// down in the order of their jacks' rows, then their calls', the first at
// the bends' room's last column, each next bendsApart further left, or 2, or
// 1, where the room holds them no further apart. It draws lanes in place of
// cords where no part has more to give, as under 100 columns, and wherever
// the bent cords outnumber the bends' room's columns.
func sessionsLaid(width, rows int, patched views.Patched, lines boardLines) boardLayout {
	k := boardColumnsOf(width)
	l := boardLayout{columns: k, lanes: k.room < leastRoom, keyAt: -1}
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
// take.
func (l *boardLayout) layLines(patched views.Patched, lines boardLines) {
	l.calls, l.lines = nil, nil
	row := 0
	for _, account := range lines.accounts {
		p := patched.Line(account)
		l.lines = append(l.lines, laidLine{account: account, patched: p, groupStart: row})
		for _, c := range p.Calls {
			l.calls = append(l.calls, laidCall{session: c.Session, account: account, cord: cord{plug: l.columns.plug, from: row}})
			row++
		}
		if len(p.Calls) > 0 {
			row++
		}
	}
	next := 0
	for i := range l.lines {
		next = l.lines[i].laid(l.step, lines.windows, next) + l.gapAfter()
	}
	l.rows = 0
	if n := len(l.lines); n > 0 {
		l.rows = l.lines[n-1].top + l.lines[n-1].rows
	}
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

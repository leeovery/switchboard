package dashboard

import (
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// numbered is how many accounts have a number that picks them on the routing
// card: the first nine.
const numbered = 9

// routingSpare is how many of the frame's rows the routing card leaves
// spare, at its tallest, its rows scrolling within it past that. It lays out
// at least the row the cursor is on, so on a terminal too short for that and
// its fixed rows it's taller, up to the page's height; on one shorter than
// that, its body is cut over its keys, as Card.shown has it, the cursor's
// row with it where it falls below the cut.
const routingSpare = 4

// The fewest rows the routing card keeps for its outcome, new sessions' and
// a session's, which keep room for their longest, so picking, which changes
// how long it is, never moves the card.
const (
	outcomeRows        = 4
	sessionOutcomeRows = 2
)

// The parts of a row of the routing card, from its content's start: the
// mark and the number, from choiceName, before the name; the name, a
// label's column by Rule 2; nameGap on, the state, or auto's description,
// as wide as stateCells has it; stateGap on, the windows, the second
// choiceUsesApart from the first, as widestUses has them; and endGap on, at
// the content's end, the reason or the sessions running, as endLeast has
// it.
const (
	choiceName      = 2
	choiceNameAt    = choiceName + 2
	stateGap        = 4
	choiceUsesApart = 11
	endGap          = 2
)

// What holds an account back from new sessions until a time, as closedBy
// says it: a limit, or its own cap.
const (
	limitHolds = "at its limit"
	capHolds   = "at its cap"
)

// stateCells are the cells of a row's state: the widest word stateWords
// says.
func stateCells() int {
	return max(RoutingRow{Open: true}.stateWords().width(), RoutingRow{}.stateWords().width())
}

// widestHeld is the widest reason the router gives with a time, as closedBy
// says it: a limit's, back another day, as "at its limit till Sun 15:54".
func widestHeld() string {
	var now time.Time
	another := now.AddDate(0, 0, 1)
	return slices.MaxFunc([]string{till(limitHolds, another, now), till(capHolds, another, now)}, func(a, b string) int {
		return ansi.StringWidth(a) - ansi.StringWidth(b)
	})
}

// widestRefused is the widest reason a refused token gives, as closedBy says
// it: refused for the widest family the ledger knows, with a status code of
// three digits, as every HTTP status is, as "token refused (403, sonnet)".
func widestRefused() string {
	refusals := make([]string, len(ledger.Families()))
	for i, family := range ledger.Families() {
		refusals[i] = tokenRefused(status.Refusal{Status: http.StatusForbidden, Family: family})
	}
	return slices.MaxFunc(refusals, func(a, b string) int { return ansi.StringWidth(a) - ansi.StringWidth(b) })
}

// tokenRefused is the reason an account whose token the upstream refused
// gives, as "token refused (401)".
func tokenRefused(r status.Refusal) string {
	return "token " + r.Answer()
}

// endLeast is the cells of a row's end at its least: the widest of the
// reasons that end can't be cut short of, a time's, as widestHeld has it,
// and a refusal's, as widestRefused has it.
func endLeast() int {
	return max(ansi.StringWidth(widestHeld()), ansi.StringWidth(widestRefused()))
}

// widestUses are the cells of the rows' windows' use, as windowsWords says
// it, each window as it's named at its widest, 100%.
func (r RoutingCard) widestUses() int {
	widest := 0
	for _, row := range r.Rows {
		for i := range row.Windows {
			row.Windows[i].Figure = Figure{Used: 1, Read: true}
		}
		widest = max(widest, row.windowsWords().width())
	}
	return widest
}

// oneLineNameKept is the name's least on one line, where its natural is as
// many or more: so one line keeps a name of up to 11 cells whole.
const oneLineNameKept = 11

// narrowGap is the cells a stacked row's gaps narrow to, once, where they
// don't fit with their own, before its name narrows.
const narrowGap = 1

// choiceColumns are where a row's parts lie in the card's content, as
// columns lays them: the name's cells, and where the state and the windows
// start; the cells between the end and what's before it; and whether the
// rows are stacked, an account's windows on a row of their own under it.
type choiceColumns struct {
	name, state, windows, endGap int
	stacked                      bool
}

// rowParts are a row's parts on one line, as flex lays them: the mark and
// number; the name, a label's column by Rule 2, the one part that narrows,
// on one line to no fewer than oneLineNameKept of its natural; the state;
// the windows; and the end.
func (r RoutingCard) rowParts() []flex.Part {
	use := r.widestUses()
	name := labelColumn(r.widestName())
	name.Least = max(name.Least, min(name.Natural, oneLineNameKept))
	return []flex.Part{
		{Least: choiceNameAt, Natural: choiceNameAt},
		name,
		{Least: stateCells(), Natural: stateCells(), Gap: nameGap},
		{Least: use, Natural: use, Gap: stateGap},
		{Least: endLeast(), Natural: endLeast(), Gap: endGap},
	}
}

// widestName is the name column's natural: the widest of the accounts'
// names, as widestNamed has it.
func (r RoutingCard) widestName() int {
	var names []string
	for _, row := range r.Rows {
		if row.Account != "" {
			names = append(names, row.Name)
		}
	}
	return widestNamed(names)
}

// natural is the card's natural width, its edges included: its rows' parts
// on one line, each at its natural, cardInset in from either edge.
func (r RoutingCard) natural() int {
	s, _ := flex.Lay(math.MaxInt, r.rowParts()...)
	last := s[len(s)-1]
	return last.At + last.Size + 2*cardInset
}

// columns are where the rows' parts lie in width cells: on one line, where
// they fit, the name narrowing first; else stacked, as stackedNames lays
// the name's row, the windows on a row of their own from the state's
// column, or as near it as they still fit whole, so a window's use is never
// cut. The parts are the card's, never what a row says, so every row is
// laid out alike.
func (r RoutingCard) columns(width int) choiceColumns {
	parts := r.rowParts()
	if s, oneLine := flex.Lay(width, parts...); oneLine {
		return choiceColumns{name: s[1].Size, state: s[2].At, windows: s[3].At, endGap: endGap}
	}
	s, gap := r.stackedNames(width, parts[0], parts[1])
	uses := parts[3]
	uses.Gap = 0
	w, _ := flex.Lay(width, flex.Part{Natural: s[2].At}, uses)
	return choiceColumns{name: s[1].Size, state: s[2].At, windows: w[1].At, endGap: gap, stacked: true}
}

// stackedNames are where a stacked row's mark and number, name and what
// follows it lie in width cells, and the gap before its end, whitespace
// giving way before words: the name whole with gaps of nameGap and endGap;
// where that doesn't fit, the gaps at narrowGap, once, and the name whole
// where that fits, then narrowing toward labelLeast; then what follows it
// narrowing, auto's description cut first, then the end.
func (r RoutingCard) stackedNames(width int, key, name flex.Part) ([]flex.Span, int) {
	whole := name
	whole.Least = min(whole.Natural, whole.Most)
	if s, fits := flex.Lay(width, key, whole, r.afterName(nameGap, endGap)); fits {
		return s, endGap
	}
	name.Least, name.Narrows = labelLeast, 1
	after := r.afterName(narrowGap, narrowGap)
	after.Least, after.Narrows = stateCells()+narrowGap+endLeast(), 2
	s, _ := flex.Lay(width, key, name, after)
	return s, narrowGap
}

// afterName is what follows a stacked row's name, gap cells on, as flex
// lays it: the state, then the end at its least, end cells after it; or
// auto's description at its natural, as autoNatural has it, where that's
// wider.
func (r RoutingCard) afterName(gap, end int) flex.Part {
	cells := max(stateCells()+end+endLeast(), r.autoNatural())
	return flex.Part{Least: cells, Natural: cells, Gap: gap}
}

// RoutingCard is the routing card's data, which the watch builds afresh at
// each read while the card is open, so its figures follow them: what it
// routes, and the session, on a session's card; its rows; what's picked, the
// row the cursor is on and whether m is ticked; the sessions m would move,
// or the session, and what rebuilding their caches costs; and why the router
// refused the last apply.
type RoutingCard struct {
	// Mode is what the card routes, and Session the session a session's card
	// routes: zero on new sessions'.
	Mode    RoutingMode
	Session RoutingSession
	// Rows are 0 auto's row, then an account's each, in the config's
	// order, as RoutingRows builds them.
	Rows []RoutingRow
	// Picked are the ids of the accounts picked, in the config's order: none
	// picks auto.
	Picked []string
	// Unsent are the ids of the accounts picked that applying the card
	// leaves out of the pin it sends, as the router refuses a pin to one
	// with no usable token: none where it sends every one. What the card
	// says applying it does names none of them.
	Unsent []string
	// Cursor is the row the cursor is on, its index in Rows.
	Cursor int
	// Move is set while m is ticked, to move the sessions running elsewhere
	// to the accounts picked too.
	Move bool
	// Moving counts the sessions that would move: those running on an
	// account not picked, without a pin of their own. Cost is what
	// rebuilding their caches costs, or on a session's card, the session's
	// moving to another account: nil where none is known.
	Moving int
	Cost   *ledger.Picodollars
	// Refused is the router's reason for refusing the last apply: "" while
	// it hasn't.
	Refused string
}

// RoutingMode is what a routing card routes: new sessions, its zero value,
// or one session.
type RoutingMode int

const (
	// NewSessions is new sessions' card, ROUTING · NEW SESSIONS, which every
	// page but a session's opens, ticking the accounts their pin names.
	NewSessions RoutingMode = iota
	// ThisSession is a session's card, ROUTING · THIS SESSION, which its own
	// page opens while it runs, picking the one account its own pin names,
	// or auto.
	ThisSession
)

// RoutingSession is the session a session's card routes, as its page has
// it: its id; the directory it runs in; the home directory, which the row
// shows it under as ~, as its page's heading does, "" showing it whole; the
// account it runs on; and the account the router would send its next
// request to were its own pin cleared, "" where the router can't say.
type RoutingSession struct {
	ID, Dir, Home, On, Auto string
}

// RoutingRow is a row of the routing card: auto's, or an account's.
type RoutingRow struct {
	// Account is the account's id: "" on auto's row.
	Account string
	// Key is the number key that picks the row, "0" on auto's, an account's
	// place in the config: "" for an account past the ninth.
	Key string
	// Name is the account's name.
	Name string
	// Open is set where the account takes new sessions now; else Reason says
	// why it's closed, as "at its cap till 17:10", and Capped whether its own
	// cap closed it, in amber, rather than Anthropic, in red.
	Open   bool
	Reason string
	Capped bool
	// Windows are the use of the first two windows every model shares, its
	// week and its 5-hour, as the moment's SharedWindows lists them.
	Windows [2]RoutingWindow
	// Running counts its sessions running.
	Running int
	// Pickable is set where it can be picked: not where it can take no
	// session, held at its limit, its token refused, or with no token.
	Pickable bool
}

// Figure is a window's use as the routing card shows it, from 0 to 1, its
// level coloured as usage colours it: none where Read says the window
// hasn't been read, and faded where Stale says its reading is stale, the
// account's token refused or its last read failed.
type Figure struct {
	Used        float64
	Read, Stale bool
}

// RoutingWindow is a window of an account's row on the routing card: its
// key; its name, as a sentence has it, as "week" or "5-hour"; and its use:
// none of a row with no window in its place.
type RoutingWindow struct {
	Key, Name string
	Figure
}

// RoutingRows are the routing card's rows of the moment m, with the sessions
// the router listed, as the request stream s has moved and seen them since:
// 0 auto's, then an account's each, in the config's order, each with its
// state, as closedBy says, its windows' use, and its sessions running, as
// views.RunningCountsOf counts SESSIONS' rows for ROUTING too.
func RoutingRows(m status.Moment, sessions []status.Session, s views.Stream) []RoutingRow {
	rows := []RoutingRow{{Key: "0", Name: "auto", Open: true, Pickable: true}}
	running := views.RunningCountsOf(m, sessions, s)
	shared := m.SharedWindows()
	for i, a := range m.Accounts {
		row := RoutingRow{Account: a.ID, Name: name(a), Pickable: pickable(a), Running: running[a.ID]}
		for j, key := range shared[:min(len(shared), len(row.Windows))] {
			w, _ := a.Window(key)
			row.Windows[j] = RoutingWindow{Key: key, Name: status.InProse(status.NameOfKey(key)), Figure: figureOf(w)}
		}
		row.Reason, row.Capped = closedBy(a, m.Now)
		row.Open = row.Reason == ""
		if i < numbered {
			row.Key = strconv.Itoa(i + 1)
		}
		rows = append(rows, row)
	}
	return rows
}

// pickable reports whether the account can be picked as it stands: it has a
// usable token, not refused, and no limit holds back its every request.
func pickable(a status.AccountNow) bool {
	return a.TokenSet && !a.TokenRefused && !limited(a)
}

// closedBy says why the account takes no new session as it stands, its times
// in now's time zone, and whether its own cap closed it rather than
// Anthropic: no usable token; a limit holding back its every request; its
// token refused; or its cap holding it back from the router's own choices.
// Of a limit or its cap, it says till it's back, as its Back has it. It's ""
// where the account is open.
func closedBy(a status.AccountNow, now time.Time) (reason string, capped bool) {
	switch {
	case !a.TokenSet:
		return "no token", false
	case limited(a):
		return till(limitHolds, a.Back, now), false
	case a.TokenRefused:
		return tokenRefused(a.Refusal), false
	case a.CapHolds:
		return till(capHolds, a.Back, now), true
	}
	return "", false
}

// till follows what holds an account back with when it's back, at t, as
// status.Till says it, as in "at its cap till 17:10": what alone where that's
// unknown.
func till(what string, t time.Time, now time.Time) string {
	if until := status.Till(now, t); until != "" {
		return what + " " + until
	}
	return what
}

// figureOf is the use of an account's window as it stands, as usage reads it:
// none where its bar draws empty, as where the account has no usable token or
// nothing is read of the window; else its use, stale or not.
func figureOf(w status.WindowNow) Figure {
	if drawsEmpty(w.Kind) {
		return Figure{}
	}
	return Figure{Used: w.Used, Read: true, Stale: w.Kind == status.WindowStale}
}

// sharedWeek is the account's week every model shares, as it stands; or,
// reporting false, where it has none, one it hasn't reported, as Window has
// it.
func sharedWeek(a status.AccountNow) (status.WindowNow, bool) {
	return windowWhere(a, func(w status.WindowNow) bool { return w.Shared && quota.WeekLong(w.Key) })
}

// windowWhere is the first of the account's windows that is, as it stands;
// or, reporting false, where none is, one it hasn't reported, as Window has
// it.
func windowWhere(a status.AccountNow, is func(status.WindowNow) bool) (status.WindowNow, bool) {
	if i := slices.IndexFunc(a.Windows, is); i >= 0 {
		return a.Windows[i], true
	}
	return a.Window("")
}

// card is the routing card on the frame: titled as m stands; its opening
// line; its rows, those that fit in view, the one the cursor is on lit; m's
// row; the outcome; and its keys, a blank row between each. It's as tall as
// all its rows, up to the frame's height less routingSpare, past which its
// rows scroll, the one the cursor is on kept in view.
func (r RoutingCard) card(f Frame) Card {
	width := f.cardWidth(r.natural())
	content := width - 2*cardInset
	at := r.columns(content)
	above := slices.Concat(r.opening(content), []line{nil}, r.sessionRows(at, content))
	below := slices.Concat(r.moveRows(), r.outcome(content))
	rows, lit, scroll := r.inView(at, content, f.bodyRoom(routingSpare, true)-len(above)-len(below))
	lit.first += len(above)
	return Card{title: r.title(), width: width, body: slices.Concat(above, rows, below), keys: r.keys(content), scroll: scroll, lit: lit}
}

// title is the card's title: what it routes, running sessions too while m
// is ticked.
func (r RoutingCard) title() string {
	switch {
	case r.Mode == ThisSession:
		return "ROUTING · THIS SESSION"
	case r.Move:
		return "ROUTING · NEW AND RUNNING SESSIONS"
	}
	return "ROUTING · NEW SESSIONS"
}

// The card's opening line: new sessions', as m stands, clear and ticked; and
// a session's.
const (
	pickNew     = "Pick the accounts new sessions go to."
	pickRunning = "Pick the accounts new and running sessions go to."
	pickSession = "Pick the account this session goes to."
)

// opening is the line saying what the card is for, wrapped to width cells:
// on new sessions' card, in as many rows as its longer form takes, so
// ticking m never moves the card.
func (r RoutingCard) opening(width int) []line {
	if r.Mode == ThisSession {
		return line{{pickSession, secondaryInk}}.wrap(width)
	}
	said := pickNew
	if r.Move {
		said = pickRunning
	}
	return atLeast(line{{said, secondaryInk}}.wrap(width), len(line{{pickRunning, secondaryInk}}.wrap(width)))
}

// sessionRows are the session's row and a blank row after it, on a
// session's card alone, as sessionRow lays it out, its path, as the rows
// are laid out, on one line alone.
func (r RoutingCard) sessionRows(at choiceColumns, width int) []line {
	if r.Mode != ThisSession {
		return nil
	}
	return []line{r.sessionRow(width, !at.stacked), nil}
}

// sessionRow is the session's row, width cells wide: its id, as the
// dashboard shows ids, bold; its directory's name, muted; where pathed, the
// directory, as pathIn has it; and at its end, runs on and the account it
// runs on, bold, where that's known. Where it runs is said here alone,
// never marked on its row below.
func (r RoutingCard) sessionRow(width int, pathed bool) line {
	s := r.Session
	var on line
	if s.On != "" {
		on = line{{"runs on ", mutedInk}, {r.names([]string{s.On})[0], titleInk}}
	}
	room := width - on.width() - 2
	row := line{{status.ShownID(s.ID), titleInk}}
	if name := status.DirName(s.Dir, max(room-row.width()-2, 1)); name != "" {
		row = slices.Concat(row, line{spaces(2), {name, mutedInk}})
	}
	if path := pathIn(s.Dir, s.Home, room-row.width()-2); pathed && path != "" {
		row = slices.Concat(row, line{spaces(2), {path, secondaryInk}})
	}
	return atEnd(row, on, width, endGap)
}

// pathIn is the directory dir in room cells, the home directory given as ~,
// cut from its start, its name kept, as a session's page heads it: none
// where cutting it would leave no more than its name, which the row says
// already.
func pathIn(dir, home string, room int) string {
	if room < ansi.StringWidth(status.DirName(dir, math.MaxInt))+2 {
		return ""
	}
	return status.DirPath(dir, home, room)
}

// atLeast is lines with blank lines after them to rows, or as many as it
// has, where that's more.
func atLeast(lines []line, rows int) []line {
	return append(lines, make([]line, max(rows-len(lines), 0))...)
}

// cardOut is the form of the line saying how many of the card's rows are out
// of view, over those shown or under them, as in "▼ 3 more".
var cardOut = outForm{inks: subtleOut}

// inView are the card's rows as they show in room lines, as choice lays each
// out, the lines of them the cursor's row takes, and how far they're
// scrolled: all of them where they fit; else those around the cursor, with
// "▲ 2 more" over them while some are above, and "▼ 3 more" under them while
// some are below, in all room's lines, so stacked rows of two lines never
// move the card as the cursor does.
func (r RoutingCard) inView(at choiceColumns, width, room int) ([]line, bodyRows, cardScroll) {
	choices := make([][]line, len(r.Rows))
	heights := make([]int, len(r.Rows))
	for i, row := range r.Rows {
		choices[i] = r.choice(row, at, width)
		heights[i] = len(choices[i])
	}
	cursor := min(max(r.Cursor, 0), len(heights)-1)
	first, end := scrolledTo(heights, cursor, room)
	var lines []line
	var lit bodyRows
	if first > 0 {
		lines = append(lines, outOfView(first, 0, width, cardOut))
	}
	for i := first; i < end; i++ {
		if i == cursor {
			lit = bodyRows{first: len(lines), n: heights[i]}
		}
		lines = append(lines, choices[i]...)
	}
	if end < len(choices) {
		lines = append(lines, outOfView(0, len(choices)-end, width, cardOut))
	}
	if first > 0 || end < len(choices) {
		lines = atLeast(lines, room)
	}
	return lines, lit, cardScroll{above: first, below: len(choices) - end}
}

// scrolledTo is which of the choices, each heights[i] lines tall, show in room
// lines, the one at cursor among them, from first to end: those from the
// top, where the cursor is among those that fit over a line saying how many
// are below; else those up to the cursor, as many as fit under a line saying
// how many are above, and over one saying how many are below, where any
// are. The cursor's shows, however few lines there are.
func scrolledTo(heights []int, cursor, room int) (first, end int) {
	if fitting(heights, room) == len(heights) {
		return 0, len(heights)
	}
	if end = fitting(heights, room-1); cursor < end {
		return 0, end
	}
	end, room = cursor+1, room-1
	if end < len(heights) {
		room--
	}
	for first = end; first > 0 && heights[first-1] <= room; first-- {
		room -= heights[first-1]
	}
	return min(first, cursor), end
}

// fitting is how many of the heights given fit in room lines, from the
// first.
func fitting(heights []int, room int) int {
	for i, h := range heights {
		if room -= h; room < 0 {
			return i
		}
	}
	return len(heights)
}

// choice is the row as the card lays it out, width cells wide, its parts in
// the columns given: its mark, its number and name, then auto's
// description, or an account's state, its windows' use and at the end its
// reason or sessions running. Stacked, an account's windows take a row of
// their own under it.
func (r RoutingCard) choice(row RoutingRow, at choiceColumns, width int) []line {
	head := line{r.mark(row), spaces(choiceName - 1), {padded(row.Key, 2), dimInk}, {prose.Cut(row.Name, at.name), titleInk}}
	if row.Account == "" {
		return []line{padTo(head, at.state, r.autoChooses(width-at.state))}
	}
	named := padTo(head, at.state, row.stateWords())
	windows := row.windowsWords()
	if at.stacked {
		return []line{atEnd(named, row.sideWords(), width, at.endGap), padTo(nil, at.windows, windows).fit(width)}
	}
	return []line{atEnd(padTo(named, at.windows, windows), row.sideWords(), width, at.endGap)}
}

// stateWords are an account's row's state: open, or closed in its reason's
// ink.
func (row RoutingRow) stateWords() line {
	if row.Open {
		return line{{"open", positiveInk}}
	}
	return line{{"closed", reasonInk(row)}}
}

// windowsWords are the row's windows' use as the card says it, as
// figureWords has each, the second choiceUsesApart cells from the first: none
// of a row with no window in its place.
func (row RoutingRow) windowsWords() line {
	var words line
	for i, w := range row.Windows {
		if w.Name != "" {
			words = padTo(words, i*choiceUsesApart, figureWords(w.Name, w.Figure))
		}
	}
	return words
}

// autoChooses is what auto's row says of it, in room cells: the router
// chooses; on a session's card, where the router says, the account it would
// choose for the session, for now, those last words the first to go where
// they don't fit.
func (r RoutingCard) autoChooses(room int) line {
	return firstFit(room, r.autoForms(r.Session.Auto)...)
}

// autoForms are what auto's row can say of it, choosing the account with
// the id given, the fullest first: on a session's card, the account, named
// as a label's column names it, labelMost cells at the most, for now, then
// the account alone; else, or where it's none, the router chooses.
func (r RoutingCard) autoForms(id string) []line {
	chooses := line{{"the router chooses", mutedInk}}
	if r.Mode != ThisSession || id == "" {
		return []line{chooses}
	}
	named := slices.Concat(chooses, line{{": ", mutedInk}, {asLabelled(r.names([]string{id})[0]), titleInk}})
	return []line{slices.Concat(named, line{{" for now", mutedInk}}), named}
}

// autoNatural is the cells of auto's description at its fullest, whichever
// account the router chooses: of the card's accounts, the widest it says.
func (r RoutingCard) autoNatural() int {
	widest := r.autoForms("")[0].width()
	for _, row := range r.Rows {
		if row.Account != "" {
			widest = max(widest, r.autoForms(row.Account)[0].width())
		}
	}
	return widest
}

// mark is the row's mark: picked, or not, where it can be, as the card's mode
// marks them; and none where it can't, unless it's picked, as a pin set from
// the command line can pick it.
func (r RoutingCard) mark(row RoutingRow) span {
	picked, unpicked := r.Mode.marks()
	switch {
	case r.picks(row):
		return span{picked, keyInk}
	case row.Pickable:
		return span{unpicked, dimInk}
	default:
		return spaces(1)
	}
}

// marks are a row's marks, picked and not: new sessions' card ticks boxes, ■
// and □; a session's picks one, ● and ○.
func (m RoutingMode) marks() (picked, unpicked string) {
	if m == ThisSession {
		return "●", "○"
	}
	return "■", "□"
}

// picks reports whether the row is picked: auto's while no account is.
func (r RoutingCard) picks(row RoutingRow) bool {
	if row.Account == "" {
		return len(r.Picked) == 0
	}
	return slices.Contains(r.Picked, row.Account)
}

// reasonInk is what a closed account's state and reason are drawn in: amber
// where its own cap closed it, red where Anthropic did.
func reasonInk(row RoutingRow) ink {
	if row.Capped {
		return warningInk
	}
	return errorInk
}

// sideWords are what the row says at its end: a closed account's reason; an
// open one's sessions running, where it has any.
func (row RoutingRow) sideWords() line {
	switch {
	case !row.Open:
		return line{{row.Reason, reasonInk(row)}}
	case row.Running > 0:
		return line{{strconv.Itoa(row.Running), strongInk}, {" running", mutedInk}}
	default:
		return nil
	}
}

// figureWords are a window's use as the card says it, after its name: the
// share bold, coloured by its level as usage colours it.
func figureWords(window string, f Figure) line {
	return slices.Concat(line{{window + " ", mutedInk}}, usageBar{used: f.Used, empty: !f.Read, stale: f.Stale}.percentage())
}

// padTo is l with blanks after it to column, then after; l, then after,
// where l reaches column already.
func padTo(l line, column int, after line) line {
	return slices.Concat(l, line{spaces(column - l.width())}, after)
}

// atEnd is l, cut to width cells, with end at its end, cut short where it
// must be to keep gap cells clear of l: then gap cells after l, as a cut
// can let go of the spaces before its ellipsis and end short of the edge.
func atEnd(l, end line, width, gap int) line {
	l = l.fit(width)
	at := l.width() + gap
	cut := end.fit(width - at)
	if cut.width() == end.width() {
		at = width - cut.width()
	}
	return padTo(l, at, cut)
}

// moveRows are the rows between the card's choices and its outcome: m's, a
// blank row either side, on new sessions' card; a blank row on a session's.
func (r RoutingCard) moveRows() []line {
	if r.Mode == ThisSession {
		return []line{nil}
	}
	return []line{nil, r.moveRow(), nil}
}

// moveRow is m's row: its box, ticked or not, and what it does.
func (r RoutingCard) moveRow() line {
	box := span{"□", dimInk}
	if r.Move {
		box = span{"■", keyInk}
	}
	return line{box, spaces(1), {"m", keyInk}, {"  also move the sessions running", secondaryInk}}
}

// outcome is what applying the card would do, or why the router refused the
// last apply, in red, wrapped to width cells, in the rows outcomeRoom keeps
// for it: cut short where it's longer, as a refusal can be.
func (r RoutingCard) outcome(width int) []line {
	said := []line{{{prose.Clean(r.Refused), errorInk}}}
	if r.Refused == "" {
		said = r.sentences()
	}
	var lines []line
	for _, sentence := range said {
		lines = append(lines, sentence.wrap(width)...)
	}
	room := r.outcomeRoom(width)
	return atLeast(cutTo(lines, room, width), room)
}

// The widest count of sessions moving, and cost of moving them, the card
// keeps room to say: past them, its outcome is cut to the room kept.
const (
	widestMoving = "9999"
	widestCost   = "$9999.99"
)

// outcomeRoom is how many rows the card keeps for its outcome, width cells
// wide: outcomeRows, or as many as the longest it can say takes, whatever's
// picked. What it says of any pick is widest's words, but for some left out
// and some shorter: fewer names, each with "or" before it and a comma or a
// full stop after it at most, wherever it stands in the list; "it's" for
// "both are"; "session" for "sessions". As wrap fills a row before it starts
// the next, fewer and shorter words never take more rows. A session's card
// keeps as many as sessionRoom says.
func (r RoutingCard) outcomeRoom(width int) int {
	if r.Mode == ThisSession {
		return r.sessionRoom(width)
	}
	var names []string
	for _, row := range r.Rows {
		if row.Account != "" {
			names = append(names, "or "+row.Name+",")
		}
	}
	return max(outcomeRows, wrappedRows(width,
		"New sessions go to "+strings.Join(names, " ")+" When both are full, the router chooses.",
		"The "+widestMoving+" sessions running elsewhere moves there too.",
		"Rebuilding their caches costs ≈ "+widestCost+".",
	), wrappedRows(width, autoChooses))
}

// sessionRoom is how many rows a session's card keeps for its outcome, width
// cells wide: sessionOutcomeRows, or as many as its move to the account of
// the widest name, and what that costs, take. What it says of any pick is
// those words, but for some left out and some shorter: "stays on" for "moves
// to", a narrower name, a full stop for " on its next request.".
func (r RoutingCard) sessionRoom(width int) int {
	widest := ""
	for _, row := range r.Rows {
		if row.Account != "" && ansi.StringWidth(row.Name) > ansi.StringWidth(widest) {
			widest = row.Name
		}
	}
	return max(sessionOutcomeRows, wrappedRows(width,
		"This session moves to "+widest+" on its next request.",
		"Rebuilding its cache costs ≈ "+widestCost+".",
	), wrappedRows(width, sessionFollows))
}

// wrappedRows is how many rows the sentences given take, each wrapped to
// width cells.
func wrappedRows(width int, sentences ...string) int {
	rows := 0
	for _, said := range sentences {
		rows += len(line{{said, secondaryInk}}.wrap(width))
	}
	return rows
}

// cutTo is lines cut to rows: where there are more, the last row kept runs
// on into the rest, cut short with an ellipsis at width cells.
func cutTo(lines []line, rows, width int) []line {
	if len(lines) <= rows {
		return lines
	}
	last := lines[rows-1]
	for _, l := range lines[rows:] {
		last = slices.Concat(last, line{spaces(1)}, l)
	}
	return append(lines[:rows-1:rows-1], last.fit(width))
}

// What the card says applying it does with auto picked: on new sessions'
// card, and on a session's.
const (
	autoChooses    = "The router chooses an account for each new session."
	sessionFollows = "This session follows the router."
)

// sentences say what applying the card changes, a sentence each, built of
// its parts: on a session's card, as sessionSentences say; else where new
// sessions go; then, with m ticked and an account picked, the sessions that
// move there too, where any would, and what rebuilding their caches costs,
// where that's known and isn't nothing.
func (r RoutingCard) sentences() []line {
	if r.Mode == ThisSession {
		return r.sessionSentences()
	}
	sends := r.sends()
	if len(sends) == 0 {
		return []line{{{autoChooses, secondaryInk}}}
	}
	said := []line{slices.Concat(
		line{{"New sessions go to ", secondaryInk}}, either(r.names(sends), titleInk, secondaryInk),
		line{{". When " + whenFull(len(sends)) + " full, the router chooses.", secondaryInk}},
	)}
	if !r.Move || r.Moving == 0 {
		return said
	}
	sessions, caches := " sessions running elsewhere move there too.", "Rebuilding their caches costs "
	if r.Moving == 1 {
		sessions, caches = " session running elsewhere moves there too.", "Rebuilding its cache costs "
	}
	said = append(said, line{{"The ", secondaryInk}, {strconv.Itoa(r.Moving), titleInk}, {sessions, secondaryInk}})
	return append(said, r.costs(caches)...)
}

// sessionSentences say what applying a session's card changes, a sentence
// each, built of its parts: with auto picked, or nothing picked it sends,
// that the session follows the router; with the account it runs on picked,
// that it stays there; else that it moves to the account picked, and what
// rebuilding its cache costs, where that's known and isn't nothing.
func (r RoutingCard) sessionSentences() []line {
	sends := r.sends()
	if len(sends) == 0 {
		return []line{{{sessionFollows, secondaryInk}}}
	}
	named := span{r.names(sends[:1])[0], titleInk}
	if sends[0] == r.Session.On {
		return []line{{{"This session stays on ", secondaryInk}, named, {".", secondaryInk}}}
	}
	moves := line{{"This session moves to ", secondaryInk}, named, {" on its next request.", secondaryInk}}
	return append([]line{moves}, r.costs("Rebuilding its cache costs ")...)
}

// costs is the sentence saying what rebuilding the caches of the sessions
// that move costs, after its opening words, where that's known and isn't
// nothing: none otherwise.
func (r RoutingCard) costs(opening string) []line {
	if r.Cost == nil || *r.Cost <= 0 {
		return nil
	}
	return []line{{{opening, secondaryInk}, {"≈ " + r.Cost.Cents(), alertInk}, {".", secondaryInk}}}
}

// sends are the ids of the accounts applying the card pins, in the config's
// order: those picked, but those it leaves unsent.
func (r RoutingCard) sends() []string {
	return slices.DeleteFunc(slices.Clone(r.Picked), func(id string) bool { return slices.Contains(r.Unsent, id) })
}

// whenFull says, of as many accounts as are picked, that they're full:
// "it's", "both are", or "all are".
func whenFull(picked int) string {
	switch picked {
	case 1:
		return "it's"
	case 2:
		return "both are"
	default:
		return "all are"
	}
}

// names are the names of the accounts with the ids given, as the card's
// rows name them, or where none does, their ids, cleaned.
func (r RoutingCard) names(ids []string) []string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = prose.Clean(id)
		if j := slices.IndexFunc(r.Rows, func(row RoutingRow) bool { return row.Account == id }); j >= 0 {
			names[i] = r.Rows[j].Name
		}
	}
	return names
}

// keys are the card's keys row, as many of them as fit width cells: ↑↓ move
// the first to go, then m move running, as m's row names its key, then the
// numbers, which nothing else says pick; enter apply and esc cancel always
// kept. The numbers name the accounts there are, up to the ninth, and tick
// on new sessions' card, or pick on a session's, which has no m.
func (r RoutingCard) keys(width int) []Key {
	move := Key{Key: "↑↓", Does: "move"}
	tick := Key{Key: "space 0-" + strconv.Itoa(min(max(len(r.Rows)-1, 0), numbered)), Does: "tick"}
	running := []Key{{Key: "m", Does: "move running"}}
	if r.Mode == ThisSession {
		tick.Does, running = "pick", nil
	}
	kept := []Key{{Key: "enter", Does: "apply"}, {Key: "esc", Does: "cancel"}}
	for _, keys := range [][]Key{slices.Concat([]Key{move, tick}, running, kept), slices.Concat([]Key{tick}, running, kept), slices.Concat([]Key{tick}, kept)} {
		if keyRow(keys).width() <= width {
			return keys
		}
	}
	return kept
}

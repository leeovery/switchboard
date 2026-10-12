package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// numbered is how many accounts have a number that picks them on the routing
// card: the first nine.
const numbered = 9

// routingSpare is how many of the frame's rows the routing card leaves
// spare, at its tallest: past that, its rows scroll within it.
const routingSpare = 4

// outcomeRows are the fewest rows the routing card keeps for its outcome,
// which keeps room for its longest, so picking, which changes how long it
// is, never moves the card.
const outcomeRows = 4

// The columns the routing card's rows lay their parts in, from its
// content's start: the mark; the name, its number before it; the state, or
// auto's description; the 5-hour window, then the week, a phone's under the
// name from the state's column; and at the content's end, the reason or the
// sessions running.
const (
	choiceName    = 2
	choiceState   = 16
	choiceSession = 26
	choiceWeek    = 39
)

// RoutingCard is the routing card's data, which the watch builds afresh at
// each read while the card is open, so its figures follow them: what it
// routes; its rows; what's picked, the row the cursor is on and whether m is
// ticked; the sessions m would move and what rebuilding their caches costs;
// and why the router refused the last apply.
type RoutingCard struct {
	// Mode is what the card routes.
	Mode RoutingMode
	// Rows are 0 auto's row, then an account's each, in the config's
	// order, as RoutingRows builds them.
	Rows []RoutingRow
	// Picked are the ids of the accounts picked, in the config's order: none
	// picks auto.
	Picked []string
	// Cursor is the row the cursor is on, its index in Rows.
	Cursor int
	// Move is set while m is ticked, to move the sessions running elsewhere
	// to the accounts picked too.
	Move bool
	// Moving counts the sessions that would move: those running on an
	// account not picked, without a pin of their own. Cost is what
	// rebuilding their caches costs: nil where none is known.
	Moving int
	Cost   *ledger.Picodollars
	// Refused is the router's reason for refusing the last apply: "" while
	// it hasn't.
	Refused string
}

// RoutingMode is what a routing card routes: new sessions, its zero value.
// A session's card, on its own page, joins it with that page.
type RoutingMode int

// NewSessions is new sessions' card, ROUTING · NEW SESSIONS, which every
// page opens.
const NewSessions RoutingMode = 0

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
	// Session and Week are its 5-hour window's use and its week's.
	Session, Week Figure
	// Running counts its sessions running.
	Running int
	// Pickable is set where it can be picked: not where it can take no
	// session, held at its limit, its token refused, or with no token.
	Pickable bool
}

// Figure is a window's use as the routing card shows it, from 0 to 1, its
// level coloured as usage colours it: none where Read says the window
// hasn't been read, and faded where Stale says it's as last read, the
// account's token refused or its last read failed.
type Figure struct {
	Used        float64
	Read, Stale bool
}

// RoutingRows are the routing card's rows of doc at now, by policy, with
// the sessions the router listed: 0 auto's, then an account's each, in the
// config's order, each with its state, as closedBy says, its windows' use,
// and its sessions running.
func RoutingRows(doc status.Document, sessions []status.Session, now time.Time, policy score.Policy) []RoutingRow {
	rows := []RoutingRow{{Key: "0", Name: "auto", Open: true, Pickable: true}}
	for i, a := range doc.Accounts {
		row := RoutingRow{
			Account: a.ID, Name: name(a), Pickable: pickable(a, now, policy),
			Session: figureOf(doc, a, policy.Started, now), Week: figureOf(doc, a, sharedWeek(policy), now),
			Running: running(sessions, a.ID, now),
		}
		row.Reason, row.Capped = closedBy(doc, a, now, policy)
		row.Open = row.Reason == ""
		if i < numbered {
			row.Key = strconv.Itoa(i + 1)
		}
		rows = append(rows, row)
	}
	return rows
}

// pickable reports whether the account can be picked at now: it has a
// usable token, not refused, and no limit holds back its every request.
func pickable(a status.Account, now time.Time, policy score.Policy) bool {
	return a.TokenSet && !a.TokenRefused(now) && !limited(a, now, policy)
}

// closedBy says why doc's account a takes no new session at now, and
// whether its own cap closed it rather than Anthropic: no usable token; a
// limit holding back its every request, till it's back; its token refused;
// or its cap holding it back from the router's own choices, till the
// windows at it reset. It's "" where the account is open.
func closedBy(doc status.Document, a status.Account, now time.Time, policy score.Policy) (reason string, capped bool) {
	hold, held := a.Held(now, policy)
	switch {
	case !a.TokenSet:
		return "no token", false
	case held && hold.Every:
		return till("at its limit", hold.Until, now), false
	case a.TokenRefused(now):
		return "token " + a.Refused.Answer(), false
	case doc.CapHolds(a, now, policy):
		back, _ := score.LetGo(a.Windows, a.Reserve, policy.IsShared, now)
		return till("at its cap", back, now), true
	}
	return "", false
}

// till follows what holds an account back with when it's back, at t, as in
// "at its cap till 17:10": what alone where that's unknown.
func till(what string, t time.Time, now time.Time) string {
	if t.IsZero() {
		return what
	}
	return what + " till " + status.Dated(now, t)
}

// running counts the sessions listed that run on the account with the given
// id at now, as its card on the Overview counts them, without what the
// request stream tells.
func running(sessions []status.Session, id string, now time.Time) int {
	return len(Frame{}.busy(sessions, id, now))
}

// figureOf is doc's account a's use of its window with the given key at now,
// as usage reads it: none where the account has no usable token or nothing
// is read of the window; as last read where that's stale.
func figureOf(doc status.Document, a status.Account, key string, now time.Time) Figure {
	switch st := doc.WindowStateOf(a, key, now); st.Kind {
	case status.WindowTokenless, status.WindowUnread:
		return Figure{}
	default:
		return Figure{Used: st.Used, Read: true, Stale: st.Kind == status.WindowStale}
	}
}

// sharedWeek is the key of the week every model shares, as policy says: ""
// where it names none.
func sharedWeek(policy score.Policy) string {
	i := slices.IndexFunc(policy.Shared, quota.WeekLong)
	if i < 0 {
		return ""
	}
	return policy.Shared[i]
}

// card is the routing card on the frame: titled as m stands; its opening
// line; its rows, those that fit in view, the one the cursor is on lit; m's
// row; the outcome; and its keys, a blank row between each. It's as tall as
// all its rows, up to the frame's height less routingSpare, past which its
// rows scroll, the one the cursor is on kept in view.
func (r RoutingCard) card(f Frame) Card {
	width := f.cardWidth(routingWidth)
	content := width - 2*cardInset
	above := slices.Concat(r.opening(content), []line{nil})
	below := slices.Concat([]line{nil, r.moveRow(), nil}, r.outcome(content))
	rows, lit, scroll := r.inView(f.phone(), content, f.bodyRoom(routingSpare, true)-len(above)-len(below))
	lit.first += len(above)
	return Card{title: r.title(), width: width, body: slices.Concat(above, rows, below), keys: r.keys(content), scroll: scroll, lit: lit}
}

// title is the card's title: what it routes, running sessions too while m
// is ticked.
func (r RoutingCard) title() string {
	if r.Move {
		return "ROUTING · NEW AND RUNNING SESSIONS"
	}
	return "ROUTING · NEW SESSIONS"
}

// The card's opening line, as m stands: clear, and ticked.
const (
	pickNew     = "Pick the accounts new sessions go to."
	pickRunning = "Pick the accounts new and running sessions go to."
)

// opening is the line saying what the card is for, wrapped to width cells,
// in as many rows as its longer form takes, so ticking m never moves the
// card.
func (r RoutingCard) opening(width int) []line {
	said := pickNew
	if r.Move {
		said = pickRunning
	}
	return atLeast(line{{said, secondaryInk}}.wrap(width), len(line{{pickRunning, secondaryInk}}.wrap(width)))
}

// atLeast is lines with blank lines after them to rows, or as many as it
// has, where that's more.
func atLeast(lines []line, rows int) []line {
	return append(lines, make([]line, max(rows-len(lines), 0))...)
}

// inView are the card's rows as they show in room lines, as choice lays each
// out, the lines of them the cursor's row takes, and how far they're
// scrolled: all of them where they fit; else those around the cursor, with
// "▲ 2 more" over them while some are above, and "▼ 3 more" under them while
// some are below, in all room's lines, so a phone's rows of two lines never
// move the card as the cursor does.
func (r RoutingCard) inView(phone bool, width, room int) ([]line, bodyRows, cardScroll) {
	choices := make([][]line, len(r.Rows))
	heights := make([]int, len(r.Rows))
	for i, row := range r.Rows {
		choices[i] = r.choice(row, phone, width)
		heights[i] = len(choices[i])
	}
	cursor := min(max(r.Cursor, 0), len(heights)-1)
	first, end := scrolledTo(heights, cursor, room)
	var lines []line
	var lit bodyRows
	if first > 0 {
		lines = append(lines, line{{"▲ " + strconv.Itoa(first) + " more", dimInk}})
	}
	for i := first; i < end; i++ {
		if i == cursor {
			lit = bodyRows{first: len(lines), n: heights[i]}
		}
		lines = append(lines, choices[i]...)
	}
	if end < len(choices) {
		lines = append(lines, line{{"▼ " + strconv.Itoa(len(choices)-end) + " more", dimInk}})
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

// choice is the row as the card lays it out, width cells wide: its mark,
// its number and name, then auto's description, or an account's state, its
// windows' use and at the end its reason or sessions running. On a phone, an
// account's windows take a row of their own under it, from the state's
// column.
func (r RoutingCard) choice(row RoutingRow, phone bool, width int) []line {
	head := line{r.mark(row), spaces(choiceName - 1), {padded(row.Key, 2), dimInk}, {truncate(row.Name, choiceState-choiceName-3), titleInk}}
	if row.Account == "" {
		return []line{padTo(head, choiceState, line{{"the router chooses", mutedInk}}).fit(width)}
	}
	state := line{{"open", positiveInk}}
	if !row.Open {
		state = line{{"closed", reasonInk(row)}}
	}
	named := padTo(head, choiceState, state)
	windows := padTo(figureWords("5-hour", row.Session), choiceWeek-choiceSession, figureWords("week", row.Week))
	if phone {
		return []line{atEnd(named, row.sideWords(), width), padTo(nil, choiceState, windows).fit(width)}
	}
	return []line{atEnd(padTo(named, choiceSession, windows), row.sideWords(), width)}
}

// mark is the row's box: ■ where it's picked, □ where it can be, and none
// where it can't, unless it's picked, as a pin set from the command line
// can pick it.
func (r RoutingCard) mark(row RoutingRow) span {
	switch {
	case r.picks(row):
		return span{"■", keyInk}
	case row.Pickable:
		return span{"□", dimInk}
	default:
		return spaces(1)
	}
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
// must be to keep two cells clear of l.
func atEnd(l, end line, width int) line {
	l = l.fit(width)
	end = end.fit(width - l.width() - 2)
	return padTo(l, width-end.width(), end)
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
	said := []line{{{status.Clean(r.Refused), errorInk}}}
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
// the next, fewer and shorter words never take more rows.
func (r RoutingCard) outcomeRoom(width int) int {
	var names []string
	for _, row := range r.Rows {
		if row.Account != "" {
			names = append(names, "or "+row.Name+",")
		}
	}
	widest := []string{
		"New sessions go to " + strings.Join(names, " ") + " When both are full, the router chooses.",
		"The " + widestMoving + " sessions running elsewhere moves there too.",
		"Rebuilding their caches costs ≈ " + widestCost + ".",
	}
	rows := 0
	for _, said := range widest {
		rows += len(line{{said, secondaryInk}}.wrap(width))
	}
	return max(outcomeRows, rows, len(line{{autoChooses, secondaryInk}}.wrap(width)))
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

// autoChooses is what the card says applying it does with auto picked.
const autoChooses = "The router chooses an account for each new session."

// sentences say what applying the card changes, a sentence each, built of
// its parts: where new sessions go; then, with m ticked and an account
// picked, the sessions that move there too, where any would, and what
// rebuilding their caches costs, where that's known and isn't nothing.
func (r RoutingCard) sentences() []line {
	if len(r.Picked) == 0 {
		return []line{{{autoChooses, secondaryInk}}}
	}
	said := []line{slices.Concat(
		line{{"New sessions go to ", secondaryInk}}, either(r.names(r.Picked), titleInk, secondaryInk),
		line{{". When " + whenFull(len(r.Picked)) + " full, the router chooses.", secondaryInk}},
	)}
	if !r.Move || r.Moving == 0 {
		return said
	}
	sessions, caches := " sessions running elsewhere move there too.", "Rebuilding their caches costs "
	if r.Moving == 1 {
		sessions, caches = " session running elsewhere moves there too.", "Rebuilding its cache costs "
	}
	said = append(said, line{{"The ", secondaryInk}, {strconv.Itoa(r.Moving), titleInk}, {sessions, secondaryInk}})
	if r.Cost != nil && *r.Cost > 0 {
		said = append(said, line{{caches, secondaryInk}, {"≈ " + r.Cost.Cents(), alertInk}, {".", secondaryInk}})
	}
	return said
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
		names[i] = status.Clean(id)
		if j := slices.IndexFunc(r.Rows, func(row RoutingRow) bool { return row.Account == id }); j >= 0 {
			names[i] = r.Rows[j].Name
		}
	}
	return names
}

// keys are the card's keys row, as many of them as fit width cells: ↑↓ move
// the first to go, then m move running, as m's row names its key, then the
// numbers, which nothing else says pick; enter apply and esc cancel always
// kept. The numbers name the accounts there are, up to the ninth.
func (r RoutingCard) keys(width int) []Key {
	move, running := Key{Key: "↑↓", Does: "move"}, Key{Key: "m", Does: "move running"}
	tick := Key{Key: "space 0-" + strconv.Itoa(min(max(len(r.Rows)-1, 0), numbered)), Does: "tick"}
	kept := []Key{{Key: "enter", Does: "apply"}, {Key: "esc", Does: "cancel"}}
	for _, keys := range [][]Key{slices.Concat([]Key{move, tick, running}, kept), slices.Concat([]Key{tick, running}, kept), slices.Concat([]Key{tick}, kept)} {
		if keyRow(keys).width() <= width {
			return keys
		}
	}
	return kept
}

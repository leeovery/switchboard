package dashboard

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// Where the heading's slots sit in a row, as the frames place them: ROUTER,
// NEW SESSIONS GO TO and ROOM LEFT start at these columns, and COMING UP
// comingAfter cells after ROOM LEFT's start, or comingGap cells after its
// rooms summed, whichever is further.
const (
	routerAt    = 1
	sessionsAt  = 26
	roomAt      = 58
	comingAfter = 48
	comingGap   = 2
)

const (
	// slotLines are the lines under each slot's label.
	slotLines = 3
	// roomLabel is the cells before ROOM LEFT's bars, which its rows'
	// labels take.
	roomLabel = 6
	// The cells a bar of ROOM LEFT takes, from the most, for a few accounts,
	// to the least, for many, and the cells all of them take at most until
	// they're the least.
	mostBarCells  = 8
	leastBarCells = 3
	barsCells     = 40
	// comingTail is the cells at the end of a line of COMING UP that its
	// countdown takes, with the gap before it.
	comingTail = 12
	// comingThings is how many things COMING UP lists.
	comingThings = 3
)

var (
	// emptyRoomInk is a bar of ROOM LEFT's empty cells: the border, faint.
	emptyRoomInk = ink{token: theme.Border, fade: 0.3}
	nextInk      = ink{token: theme.AccentMode, bold: true}
	unhealthyInk = ink{token: theme.StateDestructive, bold: true}
)

// roomGlyph fills a cell of a bar of ROOM LEFT, whether it has room or not,
// as its colour tells; emptyGlyph is an empty cell drawn without colour.
const (
	roomGlyph  = "▆"
	emptyGlyph = "░"
)

// heading draws the heading from row y, as wide as the frame, on the rows
// headingRows says it takes: a single line with one account; else its four
// slots, in a row from slotsInRowFrom columns, in two rows of two from
// phoneUnder, and under it, in a phone's two lines. Before anything is read,
// the slots are their labels alone, and the phone's lines blank.
func (f Frame) heading(c *canvas, doc status.Document, now time.Time, y int) {
	switch {
	case len(doc.Accounts) == 1:
		f.oneLine(c, doc, now, y)
	case f.phone():
		if len(doc.Accounts) > 0 {
			f.twoLines(c, doc, now, y)
		}
	case f.Width < slotsInRowFrom:
		f.twoRows(c, doc, now, y)
	default:
		f.inRow(c, doc, now, y)
	}
}

// headingRows is how many rows the heading of n accounts takes, as heading
// draws it.
func (f Frame) headingRows(n int) int {
	switch {
	case n == 1:
		return 1
	case f.phone():
		return 2
	case f.Width < slotsInRowFrom:
		return 2 * (1 + slotLines)
	default:
		return 1 + slotLines
	}
}

// inRow draws the four slots side by side.
func (f Frame) inRow(c *canvas, doc status.Document, now time.Time, y int) {
	router := sessionsAt - routerAt - 1
	f.routerSlot(c, doc, now, routerAt, y, []int{router, router, roomAt - routerAt - 2})
	f.sessionsSlot(c, doc, now, sessionsAt, y, roomAt-sessionsAt-2)
	f.roomSlot(c, doc, now, roomAt, y)
	f.comingSlot(c, doc, now, comingAt(roomAt, len(doc.Accounts)), y)
}

// comingAt is the column COMING UP starts at, of n accounts' ROOM LEFT at x:
// comingAfter cells after ROOM LEFT's start, or comingGap cells after its
// rooms summed, whichever is further.
func comingAt(x, n int) int {
	return max(x+comingAfter, summedEnd(x, n)+comingGap)
}

// twoRows draws ROUTER beside NEW SESSIONS GO TO, and under them, ROOM LEFT
// beside COMING UP, the slots on the right in a column. ROUTER's last line
// runs on under NEW SESSIONS GO TO's, which has none.
func (f Frame) twoRows(c *canvas, doc status.Document, now time.Time, y int) {
	right := comingAt(routerAt, len(doc.Accounts))
	router := right - routerAt - 2
	f.routerSlot(c, doc, now, routerAt, y, []int{router, router, f.edge() - routerAt})
	f.sessionsSlot(c, doc, now, right, y, f.edge()-right)
	y += 1 + slotLines
	f.roomSlot(c, doc, now, routerAt, y)
	f.comingSlot(c, doc, now, right, y)
}

// twoLines draws the heading as a phone has it: how the router is, its
// sessions and its routing; and where new sessions go, with the rooms summed
// at its right.
func (f Frame) twoLines(c *canvas, doc status.Document, now time.Time, y int) {
	c.line(margin, y, together(routerSays(doc, f.Lost, now), " · ").fit(f.edge()-margin))
	room := line{{"room ", dimInk}, {"5h ", mutedInk}, {f.roomSum(doc, now, f.sessionRoom), titleInk}, {"  wk ", mutedInk}, {f.roomSum(doc, now, f.weekRoom), titleInk}}
	start := c.right(f.edge(), y+1, room)
	c.line(margin, y+1, slices.Concat(line{{"new → ", mutedInk}}, newSessionsGo(doc, false)).fit(start-margin-2))
}

// oneLine draws the heading of one account, which has none to choose
// between: how the router is, its sessions and its priming, and at the
// right, the room the account has left.
func (f Frame) oneLine(c *canvas, doc status.Document, now time.Time, y int) {
	a := doc.Accounts[0]
	session, week := status.Percent(f.sessionRoom(a, now)), status.Percent(f.weekRoom(a, now))
	room := line{{"ROOM LEFT   ", labelInk}, {"5h ", mutedInk}, {session, titleInk}, {"   week ", mutedInk}, {week, titleInk}}
	label, sep := line{{"ROUTER", labelInk}, spaces(3)}, status.Separator
	if f.phone() {
		room = line{{"room ", dimInk}, {"5h ", mutedInk}, {session, titleInk}, {"  wk ", mutedInk}, {week, titleInk}}
		label, sep = nil, " · "
	}
	start := c.right(f.edge(), y, room)
	says := slices.Concat(label, together(routerSays(doc, f.Lost, now), sep))
	c.line(margin, y, says.fit(start-margin-2))
}

// routerSlot draws ROUTER at x from row y, its lines as wide as given, as
// routerLines lays them out.
func (f Frame) routerSlot(c *canvas, doc status.Document, now time.Time, x, y int, widths []int) {
	c.line(x, y, line{{"ROUTER", labelInk}})
	for i, l := range routerLines(doc, f.Lost, now, widths) {
		c.line(x, y+1+i, l)
	}
}

// sessionsSlot draws NEW SESSIONS GO TO at x from row y, its lines width
// cells wide: the account new sessions go to, and how many could take one,
// as the router chooses among them, as status.Choosable has them.
func (f Frame) sessionsSlot(c *canvas, doc status.Document, now time.Time, x, y, width int) {
	c.line(x, y, line{{"NEW SESSIONS GO TO", labelInk}})
	c.line(x, y+1, newSessionsGo(doc, true).fit(width))
	if _, ok := doc.Account(doc.Best); ok {
		among := status.Choosable(f.Policy, doc.Accounts, doc.Pin.Accounts, now)
		open := fmt.Sprintf("%d of %d open", len(among), len(doc.Accounts))
		c.line(x, y+2, line{{open, mutedInk}}.fit(width))
	}
}

// roomSlot draws ROOM LEFT at x from row y: a row each for the 5-hour window
// and the week, a bar per account, in their order, filled to the share it
// has left, and the rooms summed; and under the bars, their accounts'
// places.
func (f Frame) roomSlot(c *canvas, doc status.Document, now time.Time, x, y int) {
	c.line(x, y, line{{"ROOM LEFT, IN ACCOUNTS", labelInk}})
	n := len(doc.Accounts)
	if n == 0 {
		return
	}
	cells := barCells(n)
	rows := []struct {
		label string
		room  func(status.Account, time.Time) float64
	}{{"5h", f.sessionRoom}, {"week", f.weekRoom}}
	for i, row := range rows {
		bar := line{{row.label, mutedInk}, spaces(roomLabel - len(row.label))}
		for _, a := range doc.Accounts {
			bar = append(append(bar, f.roomBar(row.room(f.easing(a), now), cells)...), spaces(1))
		}
		sum := line{spaces(1), {f.roomSum(doc, now, row.room), titleInk}, {" of " + strconv.Itoa(n), mutedInk}}
		c.line(x, y+1+i, slices.Concat(bar, sum))
	}
	var places strings.Builder
	for i := range n {
		fmt.Fprintf(&places, "%-*d", cells+1, i+1)
	}
	c.line(x+roomLabel, y+1+len(rows), line{{strings.TrimRight(places.String(), " "), faintInk}})
}

// comingSlot draws COMING UP at x from row y: the next things to happen,
// soonest first, each with its time, its account, what happens, and at the
// frame's right, how long until it.
func (f Frame) comingSlot(c *canvas, doc status.Document, now time.Time, x, y int) {
	c.line(x, y, line{{"COMING UP", labelInk}})
	f.comingLines(c, upcoming(doc, now, f.Policy), now, x, y+1, slotLines)
}

// comingLines draws up to n of the things coming up from row y, their words
// from x, each after its time, in a column, timeGap after it, and their
// countdowns ending at the frame's right.
func (f Frame) comingLines(c *canvas, items []happening, now time.Time, x, y, n int) {
	items = items[:min(n, len(items))]
	column := 0
	for _, h := range items {
		column = max(column, len(status.When(now, h.at)))
	}
	for i, h := range items {
		c.right(f.edge(), y+i, line{{status.Until(now, h.at), h.ink}})
		at := fmt.Sprintf("%-*s", column, status.When(now, h.at)) + f.timeGap()
		words := line{{at, secondaryInk}, {h.name, strongInk}, {h.what, h.ink}}
		c.line(x, y+i, words.fit(f.edge()-comingTail-x))
	}
}

// newSessionsGo says where new sessions go: the account, numbered by its
// place where numbered says; that no account has room; or that nothing has
// been read of any to tell. It's nil when there are no accounts.
func newSessionsGo(doc status.Document, numbered bool) line {
	best, ok := doc.Account(doc.Best)
	switch {
	case ok && numbered:
		return line{{"▲ ", nextInk}, {strconv.Itoa(place(doc, best.ID)) + " ", dimInk}, {name(best), nextInk}}
	case ok:
		return line{{"▲ ", nextInk}, {name(best), nextInk}}
	case len(doc.Accounts) == 0:
		return nil
	case !slices.ContainsFunc(doc.Accounts, read):
		return line{{"nothing read yet", dimInk}}
	default:
		return line{{"no account has room right now", errorInk}}
	}
}

// read reports whether the account's usage has been read.
func read(a status.Account) bool {
	return !a.FetchedAt.IsZero()
}

// routerSays is what ROUTER says, a part each, as a line has it: how the
// router is; its sessions, and how it routes, or with one account, how it
// primes; then why it's unhealthy and a restart it has due, each starting
// where there are lines of their own. Probing, it says so, and why the router
// wasn't read. It's nil while nothing has been read.
func routerSays(doc status.Document, lost, now time.Time) []chunk {
	switch {
	case doc.Source == status.SourceRouter:
		p := fromRouter(doc, lost, now)
		return []chunk{fresh(p.health), fresh(p.sessions), {text: p.routing}, fresh(p.reason), fresh(p.restart)}
	case doc.Source != status.SourceProbe:
		return nil
	case doc.Fallback.Router == status.RouterNotRunning:
		return []chunk{fresh(line{{"○ probing", dimInk}}), fresh(line{{"router not running", dimInk}})}
	case doc.Fallback.Router == status.RouterUnhealthy:
		return []chunk{fresh(line{{"○ probing", dimInk}}), fresh(line{{because("router unhealthy", doc.Fallback.Reason), errorInk}})}
	default:
		return []chunk{fresh(line{{"○ probing, as asked", dimInk}})}
	}
}

// said is what ROUTER says of a router: how it is, its sessions and how it
// routes, why it's unhealthy, and a restart it has due, each nil where it
// says nothing.
type said struct {
	health, sessions, routing, reason, restart line
}

// fromRouter is what ROUTER says of the router whose document doc is, as
// routerSays has it: while its document stays on screen, the router not
// answering since lost, that there's been no router since then, and the rest
// as its document had it, why it was unhealthy included.
func fromRouter(doc status.Document, lost, now time.Time) said {
	p := said{sessions: line{{status.SessionCount(doc.Sessions), mutedInk}}}
	switch {
	case !lost.IsZero():
		p.health = line{{"○ no router since " + status.TimeOfDay(now, lost), dimInk}}
	case doc.Router.Healthy:
		p.health = line{{"● ", positiveInk}, {"healthy", titleInk}}
	default:
		p.health = line{{"● ", errorInk}, {"unhealthy", unhealthyInk}}
	}
	if routes := routes(doc); routes != "" {
		p.routing = line{{routes, mutedInk}}
	}
	if reason := status.Clean(doc.Router.Reason); !doc.Router.Healthy && reason != "" {
		p.reason = line{{reason, errorInk}}
	}
	if doc.Restart.Due() {
		p.restart = line{{"restart due (" + status.Clean(doc.Restart.Reason) + ")", warningInk}}
	}
	return p
}

// routerLines lays out what ROUTER says of the router whose document doc is
// on lines as wide as widths says, a line for each width at most: how the
// router is; under it, its sessions and how it routes, the routing following
// on their line where it fits, else on the last where that's free, else cut
// short there; and on the last line, a restart it has due, which always
// shows, else why it's unhealthy. With a restart due, why it's unhealthy
// follows how it is instead, wrapping onto the lines under it where it must,
// as wrapped wraps it, its sessions and routing following where there's room
// left. Probing, it's what routerSays says, flowed onto the lines.
func routerLines(doc status.Document, lost, now time.Time, widths []int) []line {
	if doc.Source != status.SourceRouter || len(widths) < 3 {
		return flow(routerSays(doc, lost, now), widths, " · ")
	}
	p := fromRouter(doc, lost, now)
	last := len(widths) - 1
	ending := p.restart
	if ending == nil {
		ending = p.reason
	}
	routed := []chunk{fresh(p.sessions), {text: p.routing}}
	switch {
	case ending == nil:
		return flow(slices.Concat([]chunk{fresh(p.health)}, routed), widths, " · ")
	case p.restart != nil && p.reason != nil:
		lines := wrapped(p.health, " · ", p.reason[0], widths[0], last)
		n, rest := len(lines), together(routed, " · ")
		if follow := slices.Concat(lines[n-1], line{{" · ", mutedInk}}, rest); follow.width() <= widths[n-1] {
			lines[n-1] = follow
		} else if n < last {
			lines = append(lines, rest.fit(widths[n]))
		}
		return append(filled(lines, last), ending.fit(widths[last]))
	default:
		lines := slices.Concat([]line{p.health.fit(widths[0])}, flow(routed, widths[1:last], " · "))
		return append(filled(lines, last), ending.fit(widths[last]))
	}
}

// filled is lines with blank lines after them, n lines at least.
func filled(lines []line, n int) []line {
	for len(lines) < n {
		lines = append(lines, nil)
	}
	return lines
}

// routes says how the router routes new sessions: "auto", or to the accounts
// its pin names, as in "pinned to work and side". With one account, there's
// nothing to route between, and it says how the router primes it, as in
// "priming 08:00–22:00", or nothing without priming.
func routes(doc status.Document) string {
	switch {
	case len(doc.Accounts) == 1 && doc.Prime.Day == "":
		return ""
	case len(doc.Accounts) == 1:
		return "priming " + strings.ReplaceAll(status.Clean(doc.Prime.Day), "-", "–")
	case doc.Pin.IsZero():
		return "auto"
	default:
		return "pinned to " + names(doc, doc.Pin.Accounts)
	}
}

// because follows what with why, cleaned, where there's a why.
func because(what, why string) string {
	if why = status.Clean(why); why == "" {
		return what
	}
	return what + ": " + why
}

// takes reports whether the account can take a request of every model at
// now: it has a usable token, and room in every window every model shares,
// and no limit or refused token holds it back.
func (f Frame) takes(a status.Account, now time.Time) bool {
	return a.TokenSet && !limited(a, now, f.Policy) && !a.TokenRefused(now) && score.Available(a.Windows, 0, f.Policy.IsShared, now)
}

// sessionRoom is the share of its 5-hour window the account has left at now,
// or none while it can take no request.
func (f Frame) sessionRoom(a status.Account, now time.Time) float64 {
	if !f.takes(a, now) {
		return 0
	}
	return roomOf(a, f.Policy.Started, now)
}

// weekRoom is the share of its week the account has left at now.
func (f Frame) weekRoom(a status.Account, now time.Time) float64 {
	return roomOf(a, f.Policy.Perishable, now)
}

// roomOf is the share the account has left at now of its window with the
// given key: all of it once it has reset since it was read, and none of a
// window it hasn't been read to have.
func roomOf(a status.Account, key string, now time.Time) float64 {
	w, ok := a.Window(key)
	switch {
	case !ok:
		return 0
	case w.ResetBy(now):
		return 1
	default:
		return min(max(1-w.Utilization, 0), 1)
	}
}

// roomSum is the rooms the accounts of doc have left at now, as room gives
// each, summed as accounts' worth, such as "1.3".
func (f Frame) roomSum(doc status.Document, now time.Time, room func(status.Account, time.Time) float64) string {
	var sum float64
	for _, a := range doc.Accounts {
		sum += room(a, now)
	}
	return fmt.Sprintf("%.1f", sum)
}

// roomBar is a bar of ROOM LEFT, cells long, filled to the share left: its
// empty cells faint, or without colour, in emptyGlyph.
func (f Frame) roomBar(left float64, cells int) line {
	filled := int(math.Round(left * float64(cells)))
	empty := span{strings.Repeat(roomGlyph, cells-filled), emptyRoomInk}
	if !f.Look.coloured {
		empty.text = strings.Repeat(emptyGlyph, cells-filled)
	}
	return line{{strings.Repeat(roomGlyph, filled), accentInk}, empty}
}

// barCells is how many cells each of n accounts' bars of ROOM LEFT takes:
// fewer as accounts are added, so the slot stays put, down to the least.
func barCells(n int) int {
	return max(leastBarCells, min(mostBarCells, barsCells/max(n, 1)-1))
}

// barsEnd is the column after the last of the bars of n accounts' ROOM LEFT
// at x, a cell before the rooms summed.
func barsEnd(x, n int) int {
	return x + roomLabel + n*(barCells(n)+1) - 1
}

// summedEnd is the column after n accounts' ROOM LEFT at x, its rooms summed
// at their widest, as in " 12.0 of 12".
func summedEnd(x, n int) int {
	most := strconv.Itoa(n)
	return barsEnd(x, n) + 1 + len(" "+most+".0 of "+most)
}

// easing is the account a as its bars are drawn: each window's use as far
// as the bars have eased to it, as eased has it.
func (f Frame) easing(a status.Account) status.Account {
	if f.Eased == nil {
		return a
	}
	windows := slices.Clone(a.Windows)
	for i, w := range windows {
		windows[i].Utilization = f.eased(a.ID, w)
	}
	a.Windows = windows
	return a
}

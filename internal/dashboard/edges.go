package dashboard

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
)

// The badges on a card's top edge: the flipped card's, the primary
// account's, the pin's on each account the global pin names, and the next's
// on the account new sessions go to.
const (
	flippedBadge = "sessions"
	primaryBadge = "◆ primary"
	pinBadge     = "● pinned"
	nextBadge    = "▲ next"
)

// edging is how a card's edges are drawn: their corners, the rule along them
// and the sides down them, and the ink they're in.
type edging struct {
	topLeft, topRight, bottomLeft, bottomRight, across, down string
	ink                                                      ink
}

// lightEdges and heavyEdges are a card's edges: rounded and light, as every
// card has, or heavy, as the card with the focus has.
var (
	lightEdges = edging{topLeft: "╭", topRight: "╮", bottomLeft: "╰", bottomRight: "╯", across: "─", down: "│"}
	heavyEdges = edging{topLeft: "┏", topRight: "┓", bottomLeft: "┗", bottomRight: "┛", across: "━", down: "┃"}
)

// in is the edges drawn in the ink k.
func (e edging) in(k ink) edging {
	e.ink = k
	return e
}

// rule is the rule along an edge, n cells long.
func (e edging) rule(n int) string {
	return strings.Repeat(e.across, max(n, 0))
}

// topEdge draws a card's top edge width cells wide from x along row y, as e
// edges it: the account's place and name, and at the right its badges. The
// name is cut to leave room for every badge but the flipped card's, so a pin
// or the next account coming or going never moves it; but on a card too
// narrow to leave it leastName cells so, to leave room for those shown. Of
// those, leavingOff leaves off what doesn't fit: on a narrow card, first to
// give the name leastName cells; then to fit beside the name.
func topEdge(c *canvas, fc face, x, y, width int, e edging) {
	tail := func(badges []span) line {
		if len(badges) == 0 {
			return line{{e.across + e.topRight, e.ink}}
		}
		return slices.Concat(line{{" ", e.ink}}, badgeLine(badges, e.ink), line{{" " + e.across + e.topRight, e.ink}})
	}
	shown := fc.badges()
	every := tail([]span{{primaryBadge, primaryInk}, {pinBadge, pinBadgeInk}, {nextBadge, badgeInk}})
	place := strconv.Itoa(fc.place) + " "
	opening := e.topLeft + e.across + " "
	fixed := ansi.StringWidth(opening+place) + 2
	room := width - fixed - every.width()
	if room < leastName {
		shown = leavingOff(shown, func(badges []span) bool { return width-fixed-tail(badges).width() >= leastName })
		room = width - fixed - tail(shown).width()
	}
	lead := line{{opening, e.ink}, {place, dimInk}, {truncate(name(fc.account), max(room, 1)), titleInk}, {" ", e.ink}}
	shown = leavingOff(shown, func(badges []span) bool { return lead.width()+tail(badges).width() <= width })
	edge(c, x, y, width, lead, tail(shown), e)
}

// leastName is the fewest cells a card's top edge leaves its account's name
// beside every badge before it leaves it the room beside those shown alone.
const leastName = 8

// leavingOff is the badges, with ◆ primary left off, then ▲ next, until fit
// reports that they fit: ● pinned, the pin's only mark on a card, and a
// flipped card's sessions are never left off.
func leavingOff(badges []span, fit func([]span) bool) []span {
	for _, off := range []string{primaryBadge, nextBadge} {
		if fit(badges) {
			break
		}
		badges = slices.DeleteFunc(slices.Clone(badges), func(b span) bool { return b.text == off })
	}
	return badges
}

// badges are the badges on the card's top edge, in order: the flipped
// card's, the primary's, the pin's, and the next account's.
func (fc face) badges() []span {
	var badges []span
	for _, b := range []struct {
		on    bool
		badge span
	}{
		{fc.flipped, span{flippedBadge, keyInk}}, {fc.account.Primary, span{primaryBadge, primaryInk}},
		{fc.pinned, span{pinBadge, pinBadgeInk}}, {fc.next, span{nextBadge, badgeInk}},
	} {
		if b.on {
			badges = append(badges, b.badge)
		}
	}
	return badges
}

// badgeLine is the badges in a line, a dot in the border's ink between each.
func badgeLine(badges []span, border ink) line {
	var l line
	for i, b := range badges {
		if i > 0 {
			l = append(l, span{" · ", border})
		}
		l = append(l, b)
	}
	return l
}

// bottomEdge draws a card's bottom edge width cells wide from x along row
// y, as e edges it: a dot for each session on the account, lit while busy,
// as many as fit, an ellipsis standing for the rest, and at the right how
// many sessions it has.
func bottomEdge(c *canvas, fc face, x, y, width int, e edging) {
	count := span{status.SessionCount(fc.sessions), mutedInk}
	if fc.sessions == 0 {
		count.ink = dimInk
	}
	tail := line{{" ", e.ink}, count, {" " + e.across + e.bottomRight, e.ink}}
	lead := line{{e.bottomLeft, e.ink}}
	if len(fc.busy) > 0 {
		lead = line{{e.bottomLeft + e.across + " ", e.ink}}
		dots, room := fc.busy, width-lead.width()-tail.width()-1
		if 2*len(dots) > room {
			dots = dots[:max(room/2-1, 0)]
		}
		for _, lit := range dots {
			lead = append(lead, sessionDot(lit), spaces(1))
		}
		if len(dots) < len(fc.busy) {
			lead = append(lead, span{ellipsis, dimInk}, spaces(1))
		}
	}
	edge(c, x, y, width, lead, tail, e)
}

// sessionDot is a session's dot: lit while it's busy, else hollow and dim.
func sessionDot(lit bool) span {
	if lit {
		return span{"●", positiveInk}
	}
	return span{"○", dimInk}
}

// edge draws a card's edge width cells wide from x along row y: lead at its
// left, tail at its right, and e's rule between.
func edge(c *canvas, x, y, width int, lead, tail line, e edging) {
	end := c.line(x, y, lead.fit(width))
	start := x + width - tail.width()
	if start < end {
		return
	}
	c.text(end, y, e.rule(start-end), e.ink)
	c.line(start, y, tail)
}

// sides draws a card's sides from x along row y, width cells apart, as e
// edges it.
func sides(c *canvas, x, y, width int, e edging) {
	c.text(x, y, e.down, e.ink)
	c.text(x+width-1, y, e.down, e.ink)
}

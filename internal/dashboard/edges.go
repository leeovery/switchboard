package dashboard

import (
	"slices"
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
)

// The badges on a card's top edge: the primary account's, the pin's on
// each account the global pin names, and the next's on the account new
// sessions go to.
const (
	primaryBadge = "◆ primary"
	pinBadge     = "● pinned"
	nextBadge    = "▲ next"
)

// The corners of a card's edges, with the rule and the gap beside them.
const (
	topLead    = "╭─ "
	topTail    = " ─╮"
	bottomLead = "╰─ "
	bottomTail = " ─╯"
)

// topEdge draws a card's top edge width cells wide from x along row y, in
// the border's ink: the account's place and name, and at the right its
// badges. The name is cut to leave room for every badge, so none coming or
// going ever moves it; but on a card too narrow to leave it leastName cells
// so, to leave room for those shown.
func topEdge(c *canvas, fc face, x, y, width int, border ink) {
	tail := line{{"─╮", border}}
	if shown := badgeLine(fc.account.Primary, fc.pinned, fc.next, border); len(shown) > 0 {
		tail = slices.Concat(line{{" ", border}}, shown, line{{topTail, border}})
	}
	every := slices.Concat(line{{" ", border}}, badgeLine(true, true, true, border), line{{topTail, border}})
	place := strconv.Itoa(fc.place) + " "
	fixed := ansi.StringWidth(topLead+place) + 2
	room := width - fixed - every.width()
	if room < leastName {
		room = width - fixed - tail.width()
	}
	lead := line{{topLead, border}, {place, dimInk}, {truncate(name(fc.account), max(room, 1)), titleInk}, {" ", border}}
	edge(c, x, y, width, lead, tail, border)
}

// leastName is the fewest cells a card's top edge leaves its account's name
// beside every badge before it leaves it the room beside those shown alone.
const leastName = 8

// badgeLine is a card's badges, those given, in order, a dot in the border's
// ink between each: the primary's, the pin's, and the next account's.
func badgeLine(primary, pinned, next bool, border ink) line {
	var l line
	for _, b := range []struct {
		on    bool
		badge span
	}{{primary, span{primaryBadge, primaryInk}}, {pinned, span{pinBadge, pinBadgeInk}}, {next, span{nextBadge, badgeInk}}} {
		switch {
		case !b.on:
			continue
		case len(l) > 0:
			l = append(l, span{" · ", border})
		}
		l = append(l, b.badge)
	}
	return l
}

// bottomEdge draws a card's bottom edge width cells wide from x along row
// y, in the border's ink: a dot for each session on the account, lit while
// busy, as many as fit, an ellipsis standing for the rest, and at the right
// how many sessions it has.
func bottomEdge(c *canvas, fc face, x, y, width int, border ink) {
	count := span{status.SessionCount(fc.sessions), mutedInk}
	if fc.sessions == 0 {
		count.ink = dimInk
	}
	tail := line{{" ", border}, count, {bottomTail, border}}
	lead := line{{"╰", border}}
	if len(fc.busy) > 0 {
		lead = line{{bottomLead, border}}
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
	edge(c, x, y, width, lead, tail, border)
}

// sessionDot is a session's dot: lit while it's busy, else hollow and dim.
func sessionDot(lit bool) span {
	if lit {
		return span{"●", positiveInk}
	}
	return span{"○", dimInk}
}

// edge draws a card's edge width cells wide from x along row y: lead at its
// left, tail at its right, and a rule in the border's ink between.
func edge(c *canvas, x, y, width int, lead, tail line, border ink) {
	end := c.line(x, y, lead.fit(width))
	start := x + width - tail.width()
	if start < end {
		return
	}
	c.text(end, y, rule(start-end), border)
	c.line(start, y, tail)
}

// sides draws a card's sides from x along row y, width cells apart, in the
// border's ink.
func sides(c *canvas, x, y, width int, border ink) {
	c.text(x, y, "│", border)
	c.text(x+width-1, y, "│", border)
}

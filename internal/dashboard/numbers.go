package dashboard

import (
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/status"
)

// NumberKind is which of a page's figures a number is.
type NumberKind int

const (
	// NumberUsed is how much of a window is used, as its percentage says:
	// an account's, or the pool's.
	NumberUsed NumberKind = iota + 1
	// NumberAllowance is an account's allowance in a window, as its words
	// lead with it: "19%/day", or the room itself, "34%".
	NumberAllowance
	// NumberRoom is the room left in a window, as in "1.5": the pool's, or a
	// lone account's.
	NumberRoom
	// NumberSessions are the sessions on an account, as ROUTING counts them,
	// or on the pool's accounts, summed.
	NumberSessions
	// NumberRate is an account's requests of the last minute, as in
	// "15/min", or the pool's accounts', summed.
	NumberRate
	// NumberStates are how many of the pool's accounts stand as the words
	// they're counted in say, as in "2 open".
	NumberStates
	// NumberTokens are how many tokens a session's answer has streamed, as
	// in "↓ ~1.2k".
	NumberTokens
	// NumberRouted are the sessions the router has routed in the last hour,
	// as the status line counts them.
	NumberRouted
)

// Number names a number the page draws, which brightens as it changes:
// which figure it is; the account it's of, by id, "" for the pool's or the
// router's own; and what of the account it's of, where it's of a part of
// it: a window, by its key; a session, by its id; or, of the pool's states,
// the words they're counted in.
type Number struct {
	Kind    NumberKind
	Account string
	Of      string
}

// numbered is the ink as the number n's, which the page brightens as it
// changes.
func (k ink) numbered(n Number) ink {
	k.number = unique.Make(n)
	return k
}

// numbered is the span as the number n.
func (s span) numbered(n Number) span {
	s.ink = s.ink.numbered(n)
	return s
}

// numbered is the line, every span of it, as the number n.
func (l line) numbered(n Number) line {
	numbered := make(line, len(l))
	for i, s := range l {
		numbered[i] = s.numbered(n)
	}
	return numbered
}

// numbered is the number the cell is part of, reporting false where it's
// part of none.
func (c cell) numbered() (Number, bool) {
	if c.number == (unique.Handle[Number]{}) {
		return Number{}, false
	}
	return c.number.Value(), true
}

// Numbers are the numbers the frame's page draws of doc at now that brighten
// as they change, by the number each is, each as its cells read: those on
// screen alone, never one scrolled out of view, in a section folded, or in a
// row left off for want of room.
func (f Frame) Numbers(doc status.Document, now time.Time) map[Number]string {
	return f.page(doc, now).numbers()
}

// brightening reports whether any number the frame's page draws of doc at
// now brightens still, as its Brightened says.
func (f Frame) brightening(doc status.Document, now time.Time) bool {
	if len(f.Brightened) == 0 {
		return false
	}
	for n := range f.Numbers(doc, now) {
		if f.Brightened[n] > 0 {
			return true
		}
	}
	return false
}

// numbers are the numbers drawn on the canvas, by the number each is, each
// as its cells read along its row.
func (c *canvas) numbers() map[Number]string {
	drawn := make(map[Number]string)
	for _, row := range c.cells {
		for _, here := range row {
			if n, ok := here.numbered(); ok {
				drawn[n] += here.glyph
			}
		}
	}
	return drawn
}

// brighten lights each number drawn on the canvas as far as brightened says
// it brightens, by the number it is: its cells' colours mixed that far toward
// text.primary, after their own, as a lit cord's are; or, where the look has
// no colour, bold while it brightens at all.
func (c *canvas) brighten(brightened map[Number]float64, look Look) {
	if len(brightened) == 0 {
		return
	}
	for _, row := range c.cells {
		for x := range row {
			here := &row[x]
			n, ok := here.numbered()
			switch bright := brightened[n]; {
			case !ok || bright <= 0:
			case look.coloured:
				here.ink.glow = max(here.ink.glow, bright)
			default:
				here.ink.bold = true
			}
		}
	}
}

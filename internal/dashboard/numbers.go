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
	// an account's, or the pool's, as History's Weeks gives this week's
	// peak.
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
	// NumberRate is requests of the last minute, as in "15/min": an
	// account's, the pool's accounts' summed, or a session's, of its account.
	NumberRate
	// NumberStates are how many of the pool's accounts stand as the words
	// they're counted in say, as in "2 open".
	NumberStates
	// NumberAccountStates are how many of every account stand as the words
	// they're counted in say, as all accounts' head counts them on Accounts:
	// a figure of its own, as all accounts is never the pool.
	NumberAccountStates
	// NumberAllUsed is how much of a window every account has used, as all
	// accounts' percentage on Accounts says: a figure of its own, as all
	// accounts is never the pool.
	NumberAllUsed
	// NumberAllRoom is the room every account has left in a window, to each
	// one's limit, as all accounts' words on Detail say it, as in "1.2": a
	// figure of its own, as all accounts is never the pool.
	NumberAllRoom
	// NumberRouted are the sessions the router has routed in the last hour,
	// as the status line counts them.
	NumberRouted
	// NumberYear is a figure of History's Year a look can change within a
	// day: one of LAST 12 MONTHS', or a calendar year's, or of BY MONTH's
	// month so far, which Of names as its label does, after the month's, as
	// 2026-10, and after the version shown's name, its Account the account
	// shown.
	NumberYear
	// NumberShareOfUse is a family's share of what History's Days weighs,
	// its eight weeks' worth, as in "59%".
	NumberShareOfUse
	// NumberLimitsHit are the limits of a window an account hit in History's
	// Days, its eight weeks.
	NumberLimitsHit
	// NumberTokens is a figure of Tokens' period still running, by its
	// column, as "input" or "against".
	NumberTokens
	// NumberTokensSplit is a figure of a row Tokens splits its period still
	// running into: of an account, or of a model version, by its id before
	// the column, as "claude-opus-5-5 input", or of both.
	NumberTokensSplit
	// NumberMoved are the sessions an event of the Log's Events moved, as
	// its words count them, which Of names by the event's key.
	NumberMoved
	// NumberPeriod is a figure of the period Accounts counts over, a look
	// reading today's days: of a row of Compare's groups or THIS WEEK, which
	// Of names as its label does, as "requests", every figure of the row one
	// number; or a version's share of use, after its name, as "Opus 5.5
	// share". Its Account is the account's id, "" for all accounts'.
	NumberPeriod
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
	k.mark = unique.Make(mark{number: n})
	return k
}

// unnumbered is the ink as no number's.
func (k ink) unnumbered() ink {
	k.mark = unique.Handle[mark]{}
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
	if c.mark == (unique.Handle[mark]{}) {
		return Number{}, false
	}
	n := c.mark.Value().number
	return n, n != Number{}
}

// StreamNumbered reports whether the frame's page may draw a number the
// request stream changes: the Overview's, with ROUTING or SESSIONS shown,
// whose rates count the requests it tells of going out, and whose sessions
// its moves re-patch. No other page draws one, a session's page over the
// Overview among them, as the tests hold each to.
func (f Frame) StreamNumbered() bool {
	return f.View == Overview && f.Session == nil && (f.Sections.Has(SectionRouting) || f.Sections.Has(SectionSessions))
}

// Numbers are the numbers the frame's page draws of doc at now that brighten
// as they change, by the number each is, each as its cells read: those on
// screen alone, never one scrolled out of view, in a section folded, or in a
// row left off for want of room.
func (f Frame) Numbers(doc status.Document, now time.Time) map[Number]string {
	f = f.At(doc, now)
	return f.page(doc, now).numbers()
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

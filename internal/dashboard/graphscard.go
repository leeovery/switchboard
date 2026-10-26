package dashboard

import (
	"math"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// roomFade is how far a burn-down card's room left fades into the canvas:
// 55% of its window's ink shows.
const roomFade = 0.45

// burnCard is a window of an account as a Graphs burn-down card charts it at
// now, worked out once, as burnCardOf has it: the window's span, its
// readings and now, as GRAPHS charts them; the ink its state gives it; the
// room ahead, from the room it has now, by the projection the moment goes
// by; when it runs out; whether its cap or its limit holds it, and since
// when; its account's reserve; whether it has nothing to draw; and how far
// it fades.
type burnCard struct {
	window chart
	ink    ink
	ahead  func(time.Time) (float64, bool)
	// out is when the window runs out at its pace, before its reset: zero
	// where it doesn't.
	out time.Time
	// held is set where its cap or its limit holds it at now, and heldSince
	// is when that hold began, which the caller fills from Runway's lanes:
	// zero where nothing tells, and the card draws no ✕.
	held      bool
	heldSince time.Time
	// reserve is the room left at its account's reserve, the reserve itself,
	// drawn where reserveHolds.
	reserve      float64
	reserveHolds bool
	// empty is set where the moment gives the window no span, as of one not
	// read, not started, lapsed while a limit still holds it, or of an
	// account without a usable token: its chart is empty but for now's line,
	// over its length from now.
	empty bool
	// fade is how far its marks fade into the canvas: a stale reading's and
	// an account out of the pool's, compounded where both hold.
	fade float64
}

// burnCardOf is account a's window with the given key as its burn-down card
// charts it at now, from its trail: over its span, as GRAPHS charts it, as
// chartOf has it; in the ink its state gives it, as windowInk has it; the
// room ahead at a time as the moment's RoomAhead has it, from the room it
// has now, never worked out again; its run-out, where its Kind runs out, at
// Out; its reserve; and its fade, as accountFade has it. A window the moment
// gives no span is charted over its length, as its key gives it, from now,
// empty, as spanFromNow has it. When a hold began is the caller's to fill.
func burnCardOf(a status.AccountNow, key string, trail Trail, now time.Time) burnCard {
	w, _ := a.Window(key)
	cd := burnCard{
		window: chartOf(a, shownWindow{key: key}, trail, now, BurnDown),
		ink:    windowInk(a, w),
		ahead: func(t time.Time) (float64, bool) {
			return a.RoomAhead(key, t)
		},
		held:         w.Kind == status.WindowLimited || w.Kind == status.WindowCapped,
		reserve:      a.Reserve,
		reserveHolds: a.ReserveHolds,
		fade:         accountFade(a, w.Kind == status.WindowStale),
	}
	if w.RunsOut {
		cd.out = w.Out.At
	}
	if !cd.window.spanned {
		cd.spanFromNow(key)
	}
	return cd
}

// spanFromNow charts the card, empty, over its window's length, as its key
// gives it, from now, so now falls in its first column: where the key gives
// none, it stays without a span, and draws nothing.
func (cd *burnCard) spanFromNow(key string) {
	cd.window.length, cd.window.spanned = quota.Length(key)
	cd.window.start, cd.window.readings, cd.empty = cd.window.now, nil, true
}

// windowInk is the ink of account a's window w by the state it's in, as
// Runway's lanes colour a state: state.destructive where its limit holds it
// or its token is refused; accent.attention where it runs out at its pace
// before its reset, or its cap holds it; else the ramp's first stop.
func windowInk(a status.AccountNow, w status.WindowNow) ink {
	switch {
	case w.Kind == status.WindowLimited || a.TokenRefused:
		return errorInk
	case w.Kind == status.WindowRunsOut || w.Kind == status.WindowCapped:
		return warningInk
	}
	return lastingInk
}

// faded is k as the card draws it: faded the card's fade more into the
// canvas, drawn faint where the look can't fade it.
func (cd burnCard) faded(k ink) ink {
	if cd.fade > 0 {
		k.fade, k.dim = 1-(1-k.fade)*(1-cd.fade), true
	}
	return k
}

// draw draws the card's chart and its axis in the bounds given: the chart
// on every row but the last, a column a cell over the window's span, and
// its axis on the last. In order, each on the cells left empty but ✕: the
// room left before now, as fill draws it; where it's heading, as heading
// draws it; its reserve, as reserveDots draws it; now's line; and ✕, as
// cross draws it, over what it falls on. An empty card has now's line and
// its axis alone, whatever holds its window; a window whose span isn't
// known draws nothing.
func (cd burnCard) draw(c *canvas, look Look, at bounds) {
	h := at.rows - 1
	if h < 1 || at.width < 1 || !cd.window.spanned {
		return
	}
	p := plot{x: at.x, floor: at.y + h - 1, width: at.width, h: h, now: cd.window.nowColumn(at.width)}
	if !cd.empty {
		cd.fill(c, look, p, cd.roomInk(roomFade))
		cd.heading(p).draw(c, p)
		cd.reserveDots(p).draw(c, p)
	}
	nowDown(c, p)
	if !cd.empty {
		cd.cross(c, p)
	}
	cd.axis(c, at.x, at.y+h, at.width)
}

// spark draws the card's window as GLANCE's sparkline, width cells from x
// along row y: a row of eighths, a column a cell over its span; before now's
// column, the room left, as fill fills it, in the window's ink; from now's,
// the room it has ahead, as aheadFill fills it, in that ink as aheadInk
// fades it; and ✕, as cross draws it, over what it falls on. An empty card
// draws nothing, nor one whose span isn't known.
func (cd burnCard) spark(c *canvas, look Look, x, y, width int) {
	if width < 1 || !cd.window.spanned || cd.empty {
		return
	}
	p := plot{x: x, floor: y, width: width, h: 1, now: cd.window.nowColumn(width)}
	room := cd.roomInk(0)
	cd.fill(c, look, p, room)
	cd.aheadFill(c, look, p, aheadInk(room))
	cd.cross(c, p)
}

// roomInk is the ink the card's room is drawn in: its window's, faded as
// far into the canvas as given, and further as the card is.
func (cd burnCard) roomInk(fade float64) ink {
	return cd.faded(ink{token: cd.ink.token, fade: fade})
}

// fill fills each column of p before now's to the room its middle had, as
// the last reading by then left it, as roomEighths counts it, in k: none
// where nothing was read by then.
func (cd burnCard) fill(c *canvas, look Look, p plot, k ink) {
	for col := range p.now {
		used, ok := cd.window.usedAt(cd.window.readAt(col, p.width))
		if eighths := roomEighths(1-used, p.h); ok && eighths > 0 {
			fillUp(c, look, p.x+col, p.floor, eighths, func(int) ink { return k })
		}
	}
}

// aheadFill fills each column of p from now's to the room its middle has
// ahead, as roomAhead has it, as roomEighths counts it, in k: none where
// nothing ahead is known.
func (cd burnCard) aheadFill(c *canvas, look Look, p plot, k ink) {
	for col := p.now; col < p.width; col++ {
		room, ok := cd.roomAhead((float64(col) + 0.5) / float64(p.width))
		if eighths := roomEighths(room, p.h); ok && eighths > 0 {
			fillUp(c, look, p.x+col, p.floor, eighths, func(int) ink { return k })
		}
	}
}

// roomAhead is the room the window has ahead a share of the way through its
// span, as the card's ahead has it, now's where that has passed, reporting
// false where nothing ahead is known.
func (cd burnCard) roomAhead(share float64) (float64, bool) {
	return cd.ahead(score.Later(cd.window.at(share), cd.window.now))
}

// roomEighths are the eighths of a column h rows tall that room, a share of
// the whole, fills: round(room × h × 8), one at least where any is left.
func roomEighths(room float64, h int) int {
	if room <= 0 {
		return 0
	}
	return max(halfEven(settled(min(room, 1)*float64(8*h))), 1)
}

// settled is x, a count of cells or dots worked out of a share of a window,
// taken to the nearest whole or half where it's within score.Tolerance of it,
// so a share's float error, as 1 − (1 − 0.05)'s, 0.050000000000000044,
// never tips a rounding at its edge.
func settled(x float64) float64 {
	if near := math.Round(2*x) / 2; math.Abs(x-near) <= score.Tolerance {
		return near
	}
	return x
}

// heading is where the window's heading on p, as braille in its ink: a dot
// at each half column from now's column to its reset, at the room the half
// column's middle has ahead, as the card's ahead has it, now's where that
// middle has passed, on the row footRow puts it on; none where nothing
// ahead is known.
func (cd burnCard) heading(p plot) dots {
	d := newDots(p.h)
	k := cd.faded(cd.ink)
	for px := 2 * p.now; px < 2*p.width; px++ {
		if room, ok := cd.roomAhead((float64(px) + 0.5) / float64(2*p.width)); ok {
			d.dot(px, d.footRow(room), k, 0)
		}
	}
	return d
}

// reserveDots is the account's reserve on p, where it holds, as braille: on
// the row of dots footRow puts its room on, the right-hand dot of every
// other column from the first, viz.reserve drawn faint.
func (cd burnCard) reserveDots(p plot) dots {
	d := newDots(p.h)
	if !cd.reserveHolds {
		return d
	}
	k, row := cd.faded(capDotInk), d.footRow(cd.reserve)
	for col := 0; col < p.width; col += 2 {
		d.dot(2*col+1, row, k, 0)
	}
	return d
}

// footRow is the row of dots, of the chart's 4h from its top, that room, a
// share of the chart, falls on: ⌈room × 4h⌉ − 1 from its foot, the foot's
// at the least, the product settled first.
func (d dots) footRow(room float64) int {
	rows := 4 * d.h
	return rows - 1 - max(int(math.Ceil(settled(min(room, 1)*float64(rows))))-1, 0)
}

// nowDown draws now's line, │ in text.faint, down p's now column, on the
// cells left empty.
func nowDown(c *canvas, p plot) {
	for r := range p.h {
		if here := c.at(p.x+p.now, p.floor-r); here != nil && *here == blank {
			c.text(p.x+p.now, p.floor-r, nowLine, faintInk)
		}
	}
}

// cross draws ✕ on p, over what it falls on, on the row footRow puts the
// floor the window meets on, the room it has ahead, as the moment has it,
// where it runs out, or now, where a hold keeps it: where its cap or its
// limit holds it, in the column the hold began, where that's told; else,
// where it runs out, in that column, now's at the soonest.
func (cd burnCard) cross(c *canvas, p plot) {
	at, after := cd.out, p.now
	if cd.held {
		at, after = cd.heldSince, 0
	}
	if at.IsZero() {
		return
	}
	floor, known := cd.ahead(score.Later(cd.out, cd.window.now))
	if !known {
		return
	}
	col := max(min(cd.timeline(p.width).Column(at), p.width-1), after, 0)
	row := newDots(p.h).footRow(floor) / 4
	c.text(p.x+col, p.floor-p.h+1+row, outMark, cd.faded(outInk))
}

// timeline is the card's span across width columns, in now's time zone, as
// its axis and its ✕ place a time: each column a whole share of it, to the
// nanosecond.
func (cd burnCard) timeline(width int) views.Timeline {
	start := cd.window.start.In(cd.window.now.Location())
	return views.Timeline{Start: start, End: start.Add(cd.window.length), Columns: width}
}

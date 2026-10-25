package dashboard

import (
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// accountHues are how many hues the pool's accounts take in turn,
// viz.account.1 to viz.account.4.
const accountHues = 4

// aheadFade is how far POOL's stack fades into the canvas from now on, where
// it's heading: 50% of it shows.
const aheadFade = 0.5

// poolKeyApart is what goes between the items of POOL's key.
const poolKeyApart = 4

// What POOL's key says of what it draws: where the stack's heading; and,
// without colour, what its one stack is.
const (
	poolHeadingMeans = " faint: heading, at its recent rate"
	poolTotalMeans   = " the pool's room; its accounts by colour alone"
	poolNowMeans     = " now"
	poolMark         = "█"
)

// poolNowInk is now's line down POOL's chart.
var poolNowInk = ink{token: theme.TextPrimary}

// poolStacked is the pool's stack as Graphs' POOL draws it in a look: each of
// its accounts' room stacked from the foot, in its hue, or, in a look without
// colour, the pool's total alone.
type poolStacked struct {
	stack views.PoolStack
	look  Look
}

// stackPart is what of a column of POOL's chart a part of its stack fills:
// its eighths from the foot, from and to, and its hue.
type stackPart struct {
	from, to int
	hue      hue
}

// accountHue is the hue of the account at place i among the pool's, from 0:
// viz.account.1 to viz.account.4 for the first four; each after takes the
// hue four before it, mixed theme.OlderFade toward the canvas, as an older
// version takes its family's, the ninth no further than the fifth.
func accountHue(i int) hue {
	h := hue{token: theme.VizAccount1 + theme.Token(i%accountHues)}
	if i >= accountHues {
		h.fade = theme.OlderFade
	}
	return h
}

// fadedBy is h faded by more into the canvas, as well as its own fade.
func (h hue) fadedBy(more float64) hue {
	h.fade = 1 - (1-h.fade)*(1-more)
	return h
}

// keyRows are the rows POOL's key takes width cells wide, as key flows it.
func (p poolStacked) keyRows(width int) int {
	return len(p.key(width))
}

// rows are the rows POOL takes width cells wide, its chart h rows tall: the
// chart's, its axis's and its key's.
func (p poolStacked) rows(width, h int) int {
	return h + runwayAxisRows + p.keyRows(width)
}

// draw draws POOL from x down from row y, width cells wide, its chart h rows
// tall, and nothing outside that: the chart, a column a cell across the
// stack's timeline, as column fills it, after now's faded aheadFade, and
// now's line, │ in text.primary, down now's column through every row; under
// it, Runway's week's axis over the same timeline, as timeAxis draws it; and
// under that, its key, as key flows it.
func (p poolStacked) draw(c *canvas, x, y, width, h int) {
	tl := p.stack.Timeline
	if tl.Columns <= 0 || width < 1 || h < 1 {
		return
	}
	in := c.within(bounds{x: x, y: y, width: width, rows: p.rows(width, h)})
	floor, now := y+h-1, tl.NowColumn()
	for col := range tl.Columns {
		if col != now {
			p.column(in, x+col, floor, h, p.parts(col, h, col > now && now >= 0))
		}
	}
	if now >= 0 && now < tl.Columns {
		for r := range h {
			in.text(x+now, floor-r, nowLine, poolNowInk)
		}
	}
	timeAxis(in, tl, views.RunwayWeek, x, floor+1)
	for i, l := range p.key(width) {
		in.line(x, floor+1+runwayAxisRows+i, l)
	}
}

// parts are the parts of the stack in its column col, of a chart h rows
// tall, from the foot: of n accounts, each account's round(room × 8h / n)
// eighths, none where its room then isn't known, in the stack's order, in its
// hue, faded aheadFade where it's ahead; in a look without colour, the sum of
// those eighths alone. None reach past the chart's top.
func (p poolStacked) parts(col, h int, ahead bool) []stackPart {
	n, top := len(p.stack.Accounts), 8*h
	var parts []stackPart
	at := 0
	for i, a := range p.stack.Accounts {
		e := min(stackEighths(a.Rooms[col], h, n), top-at)
		if e <= 0 {
			continue
		}
		hu := accountHue(i)
		if ahead {
			hu = hu.fadedBy(aheadFade)
		}
		parts = append(parts, stackPart{from: at, to: at + e, hue: hu})
		at += e
	}
	if !p.look.coloured && at > 0 {
		return []stackPart{{to: at}}
	}
	return parts
}

// stackEighths are the eighths of a chart h rows tall an account's room
// fills, of n accounts' stacked: round(room × 8h / n), the product settled
// first; none where its room isn't known.
func stackEighths(r views.AccountRoom, h, n int) int {
	if !r.Known || n < 1 {
		return 0
	}
	return halfEven(settled(min(max(r.Room, 0), 1) * float64(8*h) / float64(n)))
}

// column draws the parts of a column of the stack at x, up from row floor,
// each cell as stackCell draws it.
func (p poolStacked) column(c *canvas, x, floor, h int, parts []stackPart) {
	for r := range h {
		if glyph, k, ok := stackCell(p.look, parts, 8*r); ok {
			c.text(x, floor-r, glyph, k)
		}
	}
}

// stackCell is the glyph and ink of the cell of a column whose eighths start
// at lo, of the parts given, stacked from the foot, and whether any falls in
// it. A part alone in it fills it to where it ends, as filledCell fills it.
// Where two or more meet in it: under the stack's top, the lowest's eighths
// in its hue on the highest's as their ground, any between giving way; at the
// stack's top, the stack's eighths in the hue of the part with the most of
// them, the lowest of those even, so the stack's height is never more nor
// less than its parts'.
func stackCell(look Look, parts []stackPart, lo int) (string, ink, bool) {
	var inside []stackPart
	for _, s := range parts {
		if s.from < lo+8 && s.to > lo {
			inside = append(inside, stackPart{from: max(s.from, lo), to: min(s.to, lo+8), hue: s.hue})
		}
	}
	if len(inside) == 0 {
		return "", ink{}, false
	}
	first, last := inside[0], inside[len(inside)-1]
	top := last.to - lo
	switch {
	case len(inside) == 1:
		glyph, k := filledCell(look, top, first.hue.ink())
		return glyph, k, true
	case top < 8:
		return levels[top], widest(inside).hue.ink(), true
	}
	k := first.hue.ink()
	k.on = last.hue
	return levels[first.to-lo], k, true
}

// widest is the part with the most eighths of those given, the lowest of
// those even.
func widest(parts []stackPart) stackPart {
	most := parts[0]
	for _, s := range parts[1:] {
		if s.to-s.from > most.to-most.from {
			most = s
		}
	}
	return most
}

// key is POOL's key width cells wide, its items poolKeyApart apart, flowed onto
// as many rows as it takes, an item never broken: each account's █ in its
// hue and its name, muted; a █ faded as the stack ahead is, in the last
// account's hue, and what that means; and │ and now. Without colour, as the
// accounts can't be told apart, a █ and what the stack is, then │ and now.
func (p poolStacked) key(width int) []line {
	return wrapped(spaces(poolKeyApart), width, p.keyItems())
}

// keyItems are the items of POOL's key, in its order, as key has them.
func (p poolStacked) keyItems() []line {
	now := line{{nowLine, poolNowInk}, {poolNowMeans, mutedInk}}
	if !p.look.coloured {
		return []line{{{poolMark, ink{}}, {poolTotalMeans, mutedInk}}, now}
	}
	var items []line
	for i, a := range p.stack.Accounts {
		items = append(items, line{{poolMark, accountHue(i).ink()}, {" " + labelled(a.Label, a.ID), mutedInk}})
	}
	if n := len(p.stack.Accounts); n > 0 {
		items = append(items, line{{poolMark, accountHue(n - 1).fadedBy(aheadFade).ink()}, {poolHeadingMeans, mutedInk}})
	}
	return append(items, now)
}

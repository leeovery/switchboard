package dashboard

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// FallStep is how often an hourglass's stream falls a grain while its
// account is busy, on the clock's steps of it: a watch need draw it no
// oftener.
const FallStep = 125 * time.Millisecond

// grainCycle is how many grains down an hourglass's stream each line of its
// grains has a gap, the lines side by side a grain out of step.
const grainCycle = 3

// quadrants are the glyphs of a cell's grains, by which of them are drawn:
// its top left, top right, bottom left and bottom right, as bits 1, 2, 4
// and 8.
var quadrants = [16]string{" ", "▘", "▝", "▀", "▖", "▌", "▞", "▛", "▗", "▚", "▐", "▜", "▄", "▙", "▟", "█"}

// grain is what a grain of an hourglass holds, the most telling last.
type grain int

const (
	// outside is beyond the glass.
	outside grain = iota
	// empty is inside the glass, with no sand in it.
	empty
	// capped is the glass's caps, top and bottom.
	capped
	// sand is the sand above: the room left.
	sand
	// piled is the pile below: what's used.
	piled
	// falling is the stream falling between them.
	falling
)

// glass is an hourglass: its grains, row by row, two a cell each way, and
// which rows its bulbs take, the top's from its cap down to its neck, and
// the bottom's from its neck down to its foot.
type glass struct {
	grains      [][]grain
	top, bottom []int
}

// glassCells is how many cells wide an hourglass rows tall is drawn: two a
// row, a cell being about twice as tall as it's wide, so it stands about as
// wide as it's tall, and two more, so a short one still has bulbs either
// side of its neck.
func glassCells(rows int) int {
	return 2*rows + 2
}

// newGlass is an hourglass cells wide and rows tall, empty: a cap along its
// top and its foot, where it's 4 rows tall or more, and two bulbs between
// them, the top narrowing to its neck, two grains wide, and the bottom
// widening from it again, each a grain inside its caps.
func newGlass(cells, rows int) glass {
	g := glass{grains: make([][]grain, 2*rows)}
	for y := range g.grains {
		g.grains[y] = make([]grain, 2*cells)
	}
	first, last := 0, len(g.grains)-1
	if rows >= 4 {
		g.fill(first, capped)
		g.fill(last, capped)
		first, last = first+1, last-1
	}
	half := (last - first + 1) / 2
	for i := range half {
		width := bulbWidth(i, half, 2*cells-2)
		g.span(first+i, width)
		g.span(last-i, width)
		g.top = append(g.top, first+i)
		g.bottom = append(g.bottom, last-half+1+i)
	}
	return g
}

// bulbWidth is how many grains wide a bulb half rows tall is in its row i,
// counting from its widest, most at most: curving in to its neck, two grains
// wide, as an hourglass's bulbs do, and an even number, so it stays centred.
func bulbWidth(i, half, most int) int {
	if half < 2 {
		return most
	}
	toNeck := float64(half-1-i) / float64(half-1)
	return 2 + 2*int(math.Round(math.Sqrt(toNeck)*float64(most-2)/2))
}

// fill fills the glass's row y, every grain of it, with gr.
func (g glass) fill(y int, gr grain) {
	for x := range g.grains[y] {
		g.grains[y][x] = gr
	}
}

// span makes the middle width grains of the glass's row y its inside.
func (g glass) span(y, width int) {
	from := (len(g.grains[y]) - width) / 2
	for x := from; x < from+width; x++ {
		g.grains[y][x] = empty
	}
}

// inside counts the grains inside the glass in the rows given.
func (g glass) inside(rows []int) int {
	n := 0
	for _, y := range rows {
		for _, gr := range g.grains[y] {
			if gr == empty {
				n++
			}
		}
	}
	return n
}

// pour pours sand into the empty glass: used of it, a share, piled in its
// bottom bulb from its foot up, and the rest in its top bulb, which holds as
// many grains, settling from its neck up, so each grain is in one or the
// other; each as a level, the last of it in the middle of its row.
func (g glass) pour(used float64) {
	piles := grainsOf(used, g.inside(g.bottom))
	g.settle(g.top, sand, g.inside(g.top)-piles)
	g.settle(g.bottom, piled, piles)
}

// settle fills n grains of the bulb whose rows are given, top to bottom,
// with gr: its lowest row first, and on up, each row from its middle out.
func (g glass) settle(bulb []int, gr grain, n int) {
	for i, at := range g.order(slices.Backward(bulb)) {
		if i < n {
			g.grains[at[1]][at[0]] = gr
		}
	}
}

// grainsOf is how many grains share of whole is, to the nearest.
func grainsOf(share float64, whole int) int {
	return int(math.Round(min(max(share, 0), 1) * float64(whole)))
}

// order is where the glass's grains inside it are in the rows given, as x
// and y, row by row in turn, each row's from its middle out.
func (g glass) order(rows func(func(int, int) bool)) [][2]int {
	var grains [][2]int
	for _, y := range rows {
		var row [][2]int
		for x, gr := range g.grains[y] {
			if gr == empty {
				row = append(row, [2]int{x, y})
			}
		}
		middle := float64(len(g.grains[y])) / 2
		slices.SortStableFunc(row, func(a, b [2]int) int {
			return cmp.Compare(math.Abs(float64(a[0])+0.5-middle), math.Abs(float64(b[0])+0.5-middle))
		})
		grains = append(grains, row...)
	}
	return grains
}

// stream lets a stream wide grains across, but never wider than the glass,
// fall down the middle of the glass's bottom bulb, through what's empty of
// it, onto the pile, phase grains further down than at rest: a line of
// grains down each of its columns, a gap in it every grainCycle grains,
// each line a grain out of step with the one beside it.
func (g glass) stream(wide, phase int) {
	wide = min(wide, len(g.grains[0]))
	from := (len(g.grains[0]) - wide) / 2
	for i, y := range g.bottom {
		for x := from; x < from+wide; x++ {
			if g.grains[y][x] == empty && mod(i-phase+x%2, grainCycle) != grainCycle-1 {
				g.grains[y][x] = falling
			}
		}
	}
}

// mod is a modulo b, never negative.
func mod(a, b int) int {
	return (a%b + b) % b
}

// cell are the grains of the glass's cell at x along row y: its top left,
// top right, bottom left and bottom right.
func (g glass) cell(x, y int) [4]grain {
	top, bottom := g.grains[2*y], g.grains[2*y+1]
	return [4]grain{top[2*x], top[2*x+1], bottom[2*x], bottom[2*x+1]}
}

// quadrant is the glyph of a cell's grains of which is says are drawn.
func quadrant(grains [4]grain, is func(grain) bool) string {
	bits := 0
	for i, gr := range grains {
		if is(gr) {
			bits |= 1 << i
		}
	}
	return quadrants[bits]
}

// hourglass draws the window as an hourglass rows tall in a chart width
// cells wide from x along row y, standing over the chart's column for now,
// so the axis under it reads when: its top bulb the room left, settled in
// the state's colour faded halfway; its bottom bulb the pile of what's used;
// and between them, where there's sand left to fall, the stream, as wide as
// streamWide has it, in the state's colour, falling while the account is
// busy and still while it isn't; the glass itself faint. Held at its limit,
// all its sand is piled below. Once the window resets, the glass turns over,
// all its sand above again, still, at the chart's start; and lapsed, it
// stands there full, dim, saying when it starts. One whose reset isn't known
// has no length to stand it on.
func (f Frame) hourglass(c *canvas, p plot, x, y, width, rows int) {
	from, cells := p.glassAt(width, rows)
	switch {
	case cells == 0:
	case p.lapsed:
		f.drawGlass(c, p.glass(cells, rows), x, y, sandInks(faintLevelInk))
		across(c, x+cells, y+rows/2, width-cells, "full · "+p.starts)
	case p.spanned:
		f.drawGlass(c, p.glass(cells, rows), x+from, y, sandInks(ink{token: p.tone, fade: projectionFade}))
	}
}

// glassAt is where the window's hourglass rows tall stands in a chart width
// cells wide: from its column from, cells wide, over the column for now
// while the window runs, else at the chart's start; cells none where there's
// no room for one.
func (p plot) glassAt(width, rows int) (from, cells int) {
	cells = min(glassCells(rows), width-width%2)
	if cells < 2 || rows < 1 {
		return 0, 0
	}
	if p.running() {
		from = min(max(p.nowColumn(width)-cells/2, 0), width-cells)
	}
	return from, cells
}

// falling reports whether the sand falls in an hourglass on screen in the
// Accounts view of doc at now: the cards draw hourglasses, and one not
// flipped has its stream falling, its account busy, where any of its glass
// shows, scrolled into view, and not under the help.
func (f Frame) falling(doc status.Document, now time.Time) bool {
	if f.Chart != Hourglass {
		return false
	}
	faces, shown := f.faces(doc, now)
	top := f.top(doc)
	l := f.layOut(doc, now, len(shown), top)
	s := sight{top: top, offset: f.scrolled(l), rows: l.view, help: f.helpCovers()}
	inside, rows := l.width-2*(padding+1), l.density.chart
	for i, fc := range faces {
		if !fc.hasFeatured || fc.flipped || !fc.chart.falls() {
			continue
		}
		x, y := l.at(i, l.density)
		from, cells := fc.chart.glassAt(inside, rows)
		if s.shows(x+padding+1+from, y+l.density.offset(chartRows, l.bars), cells, rows) {
			return true
		}
	}
	return false
}

// running reports whether the window is running at now: neither lapsed nor
// reset since it was read.
func (p plot) running() bool {
	return !p.lapsed && p.now.Before(p.window.ResetsAt)
}

// glass is the window's hourglass cells wide and rows tall, its sand poured
// as the window stands at now: what's used piled below, and the rest above,
// all of it once the window has reset or lapsed, and none while it's held at
// its limit; and its stream falling between them, where it has one.
func (p plot) glass(cells, rows int) glass {
	g := newGlass(cells, rows)
	used := min(max(p.window.Utilization, 0), 1)
	switch {
	case !p.running():
		used = 0
	case p.held:
		used = 1
	}
	g.pour(used)
	if wide := p.streamWide(); wide > 0 {
		g.stream(wide, p.phase())
	}
	return g
}

// streamWide is how many grains wide the window's stream falls: none where
// it has no sand left above but what its account's reserve keeps there;
// where it's been used lately, two, and two more each time its recent rate
// passes again the fastest it could be used and still last to its reset,
// six at most; else two while its account is busy, and none while it
// isn't.
func (p plot) streamWide() int {
	switch {
	case !p.running() || p.held || p.window.Utilization >= p.floor-score.Tolerance:
		return 0
	case p.rate > score.Tolerance:
		lasting, _ := p.lasting()
		perHour := lasting * float64(time.Hour/rateSpan)
		return 2 * min(max(int(math.Ceil(p.rate/perHour)), 1), 3)
	case p.busy:
		return 2
	default:
		return 0
	}
}

// falls reports whether the window's stream falls at now: its account is
// busy, with sand left to fall.
func (p plot) falls() bool {
	return p.busy && p.streamWide() > 0
}

// phase is how far the window's stream has fallen at now, in grains, round
// its cycle: on with the clock while it falls, else at rest.
func (p plot) phase() int {
	if !p.falls() {
		return 0
	}
	return int(p.now.UnixNano() / int64(FallStep) % grainCycle)
}

// glassInk is an hourglass's glass, its caps and its inside: the bars'
// track faded halfway, as faint as a track's shade, so the dimmest sand
// shows against it.
var glassInk = ink{token: theme.VizTrack, fade: projectionFade}

// sandInks are the inks an hourglass's grains are drawn in, its sand, above
// and piled below, in the ink given: its glass faint, and its stream in the
// sand's colour, unfaded.
func sandInks(sanded ink) map[grain]ink {
	stream := sanded
	stream.fade = 0
	return map[grain]ink{empty: glassInk, capped: glassInk, sand: sanded, piled: sanded, falling: stream}
}

// drawGlass draws the glass from x along row y, a cell at a time, in the
// inks given, as glyph has each cell.
func (f Frame) drawGlass(c *canvas, g glass, x, y int, inks map[grain]ink) {
	for row := range len(g.grains) / 2 {
		for col := range len(g.grains[0]) / 2 {
			if glyph, k, ok := g.glyph(col, row, inks, f.Look.blends()); ok {
				c.text(x+col, y+row, glyph, k)
			}
		}
	}
}

// glyph is the glyph of the glass's cell at col along row, and its ink, in a
// look that blends its colours or not, as blendedGlyph and plainGlyph have
// it, reporting false where the cell holds nothing of the glass.
func (g glass) glyph(col, row int, inks map[grain]ink, blends bool) (string, ink, bool) {
	grains := g.cell(col, row)
	if slices.Max(grains[:]) == outside {
		return "", ink{}, false
	}
	if blends {
		return blendedGlyph(grains, inks)
	}
	return plainGlyph(grains, inks)
}

// blendedGlyph is a cell's grains as a quadrant block in a look that blends
// its colours, and its ink: its most telling grains in their ink, on the
// ink the rest of it is drawn in, where that's one ink, all within the
// glass; else on the canvas, with any more of its sand or its stream, but
// never the glass, whose ink isn't theirs. A cell of glass alone is its
// block.
func blendedGlyph(grains [4]grain, inks map[grain]ink) (string, ink, bool) {
	fore := slices.Max(grains[:])
	k := inks[fore]
	switch back, ok := beneath(grains, fore, inks); {
	case fore <= capped:
		return quadrant(grains, func(gr grain) bool { return gr == empty || gr == capped }), k, true
	case ok:
		k.on = back.hue()
		return quadrant(grains, func(gr grain) bool { return gr == fore }), k, true
	default:
		return quadrant(grains, func(gr grain) bool { return gr >= sand }), k, true
	}
}

// plainGlyph is a cell's grains as a look that can't blend its colours draws
// them, without the faded surface a blended cell stands on, as a bar's
// projection is drawn in such a look: its sand, its stream and the glass's
// caps as a quadrant block, in the ink of the most telling of them; else,
// where half of it or more is the glass's inside, the bars' track; else
// nothing, reporting false.
func plainGlyph(grains [4]grain, inks map[grain]ink) (string, ink, bool) {
	fore := slices.Max(grains[:])
	switch {
	case fore > empty:
		return quadrant(grains, func(gr grain) bool { return gr > empty }), inks[fore], true
	case count(grains, empty) >= 2:
		return trackCell, trackInk, true
	default:
		return "", ink{}, false
	}
}

// beneath is the ink a cell's grains are drawn in but those holding fore, as
// inks has them, reporting false where they're drawn in more than one, any
// of them is outside the glass, or there are none.
func beneath(grains [4]grain, fore grain, inks map[grain]ink) (ink, bool) {
	var back ink
	found := false
	for _, gr := range grains {
		switch {
		case gr == fore:
		case gr == outside || found && inks[gr] != back:
			return ink{}, false
		default:
			back, found = inks[gr], true
		}
	}
	return back, found
}

// count counts the grains that hold gr.
func count(grains [4]grain, gr grain) int {
	n := 0
	for _, g := range grains {
		if g == gr {
			n++
		}
	}
	return n
}

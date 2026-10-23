package dashboard

import (
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// storyColumnRows is how many rows a session's story's columns take at the
// most, at every width; the page gives the last back where its rows run
// short, as sessionLayout has it.
const storyColumnRows = 5

// storyLit is how far the turn chosen's columns are mixed into the canvas:
// 95% of their colour shows.
const storyLit = 0.05

// The glyphs a story draws: a move's marker; what a fold draws in its middle
// three columns; and a stretch's ends.
const (
	moveMarker   = "┊"
	foldDrawn    = "╱ ╱"
	stretchStart = "├"
	stretchEnd   = "┤"
)

// The story's heading, and what it says after it.
const (
	storyHeading = "STORY"
	storySays    = "requests a minute"
)

// storyRuleShort is how many cells short of the frame's edge the story's
// heading's rule ends.
const storyRuleShort = 2

// moveLabelGap is how many cells a move's label keeps between its words and
// its time, and before the label to its right.
const moveLabelGap = 2

// leastLabel is the fewest cells a move's label cut to fit takes, its time
// and marker among them: narrower, it's left out, its marker alone.
const leastLabel = 12

// sessionStory draws the story of a session's page in the rows its layout
// gives it, the turn chosen lit, as STORY draws it: under the wide chrome,
// its heading, then the label of a move, its columns in the rows the layout
// gives them, its ribbon and its axis, or its heading alone where it's
// folded; under a phone's, the same without its heading, or nothing where
// it's left out. It names each account as the moment m names it. chosen is
// the zero Turn where none is.
func (f Frame) sessionStory(c *canvas, m status.Moment, l sessionLayout, story views.Story, chosen views.Turn) {
	switch {
	case l.chrome:
		return
	case l.wide && !l.story:
		f.storyHeading(c, l.storyAt, f.noRoom())
	case l.wide:
		f.storyHeading(c, l.storyAt, line{{storySays, faintInk}})
		f.drawStory(c, m, bounds{y: l.storyAt + 1, width: f.Width, rows: l.body}, story, chosen)
	case l.story:
		f.drawStory(c, m, bounds{y: l.storyAt, width: f.Width, rows: l.body}, story, chosen)
	}
}

// storyHeading draws the story's heading along row y: STORY in text.subtle,
// what it says headingGap cells on, and a cell after that a rule in border to
// storyRuleShort cells short of the frame's edge.
func (f Frame) storyHeading(c *canvas, y int, says line) {
	x := c.line(0, y, line{{storyHeading, dimInk}})
	x = c.line(x+headingGap, y, says)
	c.text(x+1, y, rule(f.Width-storyRuleShort-x-1), borderInk)
}

// drawStory draws the story across at's width from its top row: the label
// of a move on it, its columns in at's rows under that, the ribbon under
// them and the axis under the ribbon, its accounts named as the moment m
// names them.
func (f Frame) drawStory(c *canvas, m status.Moment, at bounds, story views.Story, chosen views.Turn) {
	cols := story.Columns(f.Width)
	if len(cols.Rates) == 0 {
		return
	}
	d := storyDrawn{StoryColumns: cols, end: story.End, label: at.y, in: bounds{y: at.y + 1, width: at.width, rows: at.rows}, m: m}
	d.eighths = f.storyEighths(c, d)
	f.storyColumns(c, d, d.lit(chosen))
	d.folds(c)
	d.markers(c)
	for _, l := range d.labels(f.Width) {
		c.line(l.x, d.label, l.said)
	}
	d.ribbon(c)
	d.axis(c)
}

// storyDrawn is a session's story laid across the frame's width, as it's
// drawn: its columns; its end, whose time zone its times are said in; the row
// of its label, and the bounds of its columns; each column's eighths, at its
// scale; and the moment its accounts are named as, as named names them.
type storyDrawn struct {
	views.StoryColumns
	end     time.Time
	label   int
	in      bounds
	eighths []int
	m       status.Moment
}

// storyEighths are the eighths each of the story's columns fills, at the
// scale the traces are drawn at: the frame's TraceScale, where it pins one,
// else the busiest column's, never less than leastScale.
func (f Frame) storyEighths(c *canvas, d storyDrawn) []int {
	scale := f.traceScale([]tracing{{on: c, cells: d.Rates, at: d.in}})
	eighths := make([]int, len(d.Rates))
	for i, v := range d.Rates {
		eighths[i] = traceEighths(v, scale, d.in.rows)
	}
	return eighths
}

// lit reports whether the column given is of the turn chosen: from its
// first request's to its end's, or the newest while it's going. None is
// where no turn is chosen.
func (d storyDrawn) lit(chosen views.Turn) func(column int) bool {
	if chosen.Turn == 0 {
		return func(int) bool { return false }
	}
	first, last := d.At(chosen.Started), len(d.Rates)-1
	if !chosen.Ended.IsZero() {
		last = d.At(chosen.Ended)
	}
	return func(column int) bool { return column >= first && column <= last }
}

// storyColumns paints the story's columns but a fold's middle three, as the
// traces paint theirs, those lit in accent.key mixed with the canvas so 95%
// of it shows.
func (f Frame) storyColumns(c *canvas, d storyDrawn, lit func(column int) bool) {
	fill, lane := traceInks(false)
	shining := ink{token: theme.AccentKey, fade: storyLit}
	for i, eighths := range d.eighths {
		if d.foldsMiddle(i) {
			continue
		}
		k := fill
		if lit(i) {
			k = shining
		}
		f.paintColumn(c, i, d.in, eighths, k, lane)
	}
}

// foldsMiddle reports whether the column given is one of a fold's middle
// three, which draw it.
func (d storyDrawn) foldsMiddle(column int) bool {
	return d.Folded(column) && d.Folded(column-1) && d.Folded(column+1)
}

// folds draws each fold's ╱ ╱ in text.faint, in its middle three columns, on
// each of the columns' rows.
func (d storyDrawn) folds(c *canvas) {
	for _, first := range d.Folds {
		for y := d.in.y; y < d.in.y+d.in.rows; y++ {
			c.text(first+1, y, foldDrawn, faintInk)
		}
	}
}

// markers draws each move's marker, ┊ in accent.attention, in its column on
// the label's row, and on each of the columns' rows its column leaves empty,
// or holds its floor alone.
func (d storyDrawn) markers(c *canvas) {
	floor := d.in.y + d.in.rows - 1
	for _, m := range d.Moves {
		c.text(m.Column, d.label, moveMarker, warningInk)
		filled := (d.eighths[m.Column] + 7) / 8
		for y := d.label + 1; y <= floor-filled; y++ {
			c.text(m.Column, y, moveMarker, warningInk)
		}
	}
}

// moveLabel is a move's label on a story: the column it starts at, and what
// it says.
type moveLabel struct {
	x    int
	said line
}

// labels are the labels of the story's moves, width cells wide, placed the
// newest first, as labelOf places each, ending moveLabelGap cells before the
// label to its right, or the marker where that one's left out; and left out
// where the label to its right covers its marker.
func (d storyDrawn) labels(width int) []moveLabel {
	var labels []moveLabel
	limit := width
	for _, m := range slices.Backward(d.Moves) {
		if m.Column >= limit {
			continue
		}
		l, ok := d.labelOf(m, limit)
		if !ok {
			limit = m.Column - moveLabelGap
			continue
		}
		labels, limit = append(labels, l), l.x-moveLabelGap
	}
	return labels
}

// labelOf is the label of the move m, ending before the column limit: in
// its wide words where they fit whole, else its short words where they do,
// at its marker's left first; else the move's names, as moveNames cuts
// them, on whichever side has more room, where that leaves it leastLabel
// cells at least.
func (d storyDrawn) labelOf(m views.StoryMove, limit int) (moveLabel, bool) {
	at := prose.TimeOfDay(d.end, m.At)
	rest := moveLabelGap + ansi.StringWidth(at) + 1 + ansi.StringWidth(moveMarker)
	left, right := m.Column+1-rest, limit-m.Column-rest
	for _, form := range []views.MoveForm{views.MoveWide, views.MoveShort} {
		words := d.said(m.Said(form), form)
		switch {
		case words.width() <= left:
			return moveLabelAt(m.Column, words, at, true), true
		case words.width() <= right:
			return moveLabelAt(m.Column, words, at, false), true
		}
	}
	if max(left, right)+rest < leastLabel {
		return moveLabel{}, false
	}
	names := moveNames(d.named(m.From), d.named(m.To), max(left, right), secondaryInk)
	return moveLabelAt(m.Column, names, at, left >= right), true
}

// moveLabelAt is the label saying words of a move at the time given, its
// marker at column: ending at it, "words  14:31 ┊", where left; else
// starting at it, "┊ 14:31  words".
func moveLabelAt(column int, words line, at string, left bool) moveLabel {
	if !left {
		return moveLabel{x: column, said: slices.Concat(line{{moveMarker + " " + at, warningInk}, spaces(moveLabelGap)}, words)}
	}
	said := slices.Concat(words, line{spaces(moveLabelGap), {at + " " + moveMarker, warningInk}})
	return moveLabel{x: column + 1 - said.width(), said: said}
}

// said is a move's label's words drawn, said in form: its accounts by their
// names, in text.secondary, its figures in accent.attention, bold in the
// wide words, and the rest in text.muted.
func (d storyDrawn) said(words status.Words, form views.MoveForm) line {
	figure := alertInk
	if form == views.MoveShort {
		figure = warningInk
	}
	l := make(line, len(words))
	for i, p := range words {
		l[i] = span{p.Text, mutedInk}
		switch p.Kind {
		case status.PartAccount:
			l[i] = span{d.named(p.Text), secondaryInk}
		case status.PartFigure:
			l[i].ink = figure
		}
	}
	return l
}

// named is the account with the id given's name as the story says it: as
// the moment names it, cut as a label's column cuts it, so it reads as it
// does on the rest of the page.
func (d storyDrawn) named(id string) string {
	return asLabelled(named(d.m, id))
}

// ribbon draws each stretch on the row under the columns, ├, ─ and ┤ in
// text.faint from its first column to its last, its account's name, as
// named names it, centred in it in text.secondary, a space either side,
// where its inside has room for it, the odd cell left of it.
func (d storyDrawn) ribbon(c *canvas) {
	y := d.in.y + d.in.rows
	for _, s := range d.Stretches {
		inside, name := s.Last-s.First-1, " "+d.named(s.Account)+" "
		cells := ansi.StringWidth(name)
		if cells > inside {
			c.text(s.First, y, stretchStart+rule(inside)+stretchEnd, faintInk)
			continue
		}
		before := (inside - cells + 1) / 2
		c.line(s.First, y, line{{stretchStart + rule(before), faintInk}, {name, secondaryInk},
			{rule(inside-before-cells) + stretchEnd, faintInk}})
	}
}

// axis draws the axis's labels on the row under the ribbon, each at its
// column, in text.faint, but now or ended, in text.subtle.
func (d storyDrawn) axis(c *canvas) {
	y := d.in.y + d.in.rows + 1
	for _, a := range d.Axis {
		k := faintInk
		if a.End {
			k = dimInk
		}
		c.text(a.Column, y, a.Text, k)
	}
}

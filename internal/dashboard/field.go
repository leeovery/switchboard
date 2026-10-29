package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/dashboard/graphemes"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// Field is a form's text field as it's drawn: what's typed in it, and while
// it's the one typed in, as Typing has it, where its cursor is, in glyphs
// from its start, as a terminal draws them; and From, the first glyph its
// box shows of a value wider than it, as InView last left it, so the view
// holds still while the cursor moves within it.
type Field struct {
	Value  string
	Cursor int
	From   int
	Typing bool
}

// textField is a form's row of a text field: its name; the field; the hint
// after it; and what's wrong with what's typed in it, said under it, where
// anything is.
type textField struct {
	name, hint, wrong string
	field             Field
}

// The cells of a field's row: its name's, before its box; its box's, a cell
// before its value, then its value's room; and the cells between the box
// and the hint.
const (
	fieldNameCells = 9
	fieldBoxCells  = 24
	fieldHintGap   = 3
)

// The parts of a field's row, as fieldParts lays them out.
const (
	fieldName = iota
	fieldBox
	fieldHint
)

// The inks of a field typed in: its value, bold text.primary, on its box's
// bg.subtle; and its cursor, a block of text.primary, the rune it's on, if
// any, in the box's colour.
var (
	typingInk = ink{token: theme.TextPrimary, bold: true, on: hue{token: theme.BgSubtle}}
	caretInk  = ink{token: theme.BgSubtle, bold: true, on: hue{token: theme.TextPrimary}}
)

// fieldParts are a field's row's parts, across width cells: its name; its
// box; and fieldHintGap cells on, its hint, taking the rest, or giving way
// where there's none.
func fieldParts(width int) []flex.Span {
	spans, _ := flex.Lay(width, []flex.Part{
		fieldName: {Least: fieldNameCells, Natural: fieldNameCells},
		fieldBox:  {Least: fieldBoxCells, Natural: fieldBoxCells},
		fieldHint: {Least: 1, Grows: true, Leaves: 1, Gap: fieldHintGap},
	}...)
	return spans
}

// draw draws the field's row from x along row y, width cells wide, as
// fieldParts lays it out: its name in text.subtle; its value a cell into its
// box, in text.secondary, or typed in, as typed draws it; and its hint in
// text.faint. Under it, from its value's column, what's wrong with its value,
// in state.destructive, wrapped onto as many rows as it takes. It returns the
// row after the last it drew on.
func (f textField) draw(c *canvas, x, y, width int) int {
	spans := fieldParts(width)
	name, box, hint := spans[fieldName], spans[fieldBox], spans[fieldHint]
	c.line(x+name.At, y, line{{f.name, dimInk}}.fit(name.Size))
	at, room := x+box.At+1, box.Size-1
	if f.field.Typing {
		c.surface(x+box.At, y, box.Size, typingInk.on)
		c.line(at, y, f.field.typed(room))
	} else {
		c.line(at, y, line{{f.field.Value, secondaryInk}}.fit(room))
	}
	if !hint.Left {
		c.line(x+hint.At, y, line{{f.hint, faintInk}}.fit(hint.Size))
	}
	next := y + 1
	for _, l := range f.wrongLines(width) {
		c.line(at, next, l)
		next++
	}
	return next
}

// wrongLines are the lines saying what's wrong with the field's value, as
// draw draws them under its row width cells wide, wrapped from its value's
// column: none where nothing is.
func (f textField) wrongLines(width int) []line {
	if f.wrong == "" {
		return nil
	}
	return line{{f.wrong, errorInk}}.wrap(width - valueAt(width))
}

// valueAt is the column a field's value starts at, of a row width cells
// wide, as fieldParts lays it out: a cell into its box.
func valueAt(width int) int {
	return fieldParts(width)[fieldBox].At + 1
}

// fieldRoom is the cells a field's box has for its value, a cell in.
const fieldRoom = fieldBoxCells - 1

// InView is f with From moved as little as brings its cursor into view in
// its box, as typed draws it, so the view holds still while the cursor moves
// within it: back to the cursor, where it's before From; on, where it's past
// the box's end; and back as far as the value from there to its end still
// fits, so the box leaves no room blank that the value could fill.
func (f Field) InView() Field {
	f.From = f.typedView().first(fieldRoom)
	return f
}

// typedView is the field as typed draws it: its glyphs, and its cursor and
// From, each kept within them.
func (f Field) typedView() fieldView {
	var gs []grapheme
	for g, cells := range graphemes.In(f.Value) {
		gs = append(gs, grapheme{text: g, cells: cells})
	}
	cursor := min(max(f.Cursor, 0), len(gs))
	return fieldView{glyphs: gs, cursor: cursor, from: min(max(f.From, 0), cursor)}
}

// fieldView is a field's value as it's drawn typed in: its glyphs; where its
// cursor is; and the first glyph shown, as the view last stood.
type fieldView struct {
	glyphs       []grapheme
	cursor, from int
}

// caret is the glyph the cursor is drawn on: the one it's at, or a blank,
// after the last, or in place of one that takes no cell, as a stray
// combining mark, so the cursor shows.
func (v fieldView) caret() grapheme {
	if v.cursor < len(v.glyphs) && v.glyphs[v.cursor].width() > 0 {
		return v.glyphs[v.cursor]
	}
	return grapheme{text: " ", cells: 1}
}

// cells is how many cells the glyphs from i to j take as they're drawn, the
// cursor's as caret has it.
func (v fieldView) cells(i, j int) int {
	n := 0
	for k := i; k < j; k++ {
		if k == v.cursor {
			n += v.caret().width()
		} else {
			n += v.glyphs[k].width()
		}
	}
	return n
}

// cutBefore is the cell the ellipsis takes, marking what's cut, before the
// glyphs shown from from: none from the first.
func cutBefore(from int) int {
	return min(from, 1)
}

// shows reports whether the cursor is in view in room cells with the
// glyphs shown from from: it and those before it, and either the rest, or a
// cell for the ellipsis marking them cut.
func (v fieldView) shows(from, room int) bool {
	used := cutBefore(from) + v.cells(from, v.cursor) + v.caret().width()
	rest := v.cells(min(v.cursor+1, len(v.glyphs)), len(v.glyphs))
	return used+rest <= room || used+1 <= room
}

// fits reports whether the glyphs from from to the last, and the blank the
// cursor takes after it, fit in room cells.
func (v fieldView) fits(from, room int) bool {
	return cutBefore(from)+v.cells(from, len(v.glyphs))+1 <= room
}

// first is the first glyph shown in room cells, as InView moves it.
func (v fieldView) first(room int) int {
	from := v.from
	for from < v.cursor && !v.shows(from, room) {
		from++
	}
	for from > 0 && v.fits(from-1, room) {
		from--
	}
	return from
}

// typed is the field's value as it's drawn typed in, in no more than room
// cells, its view as InView moves it: an ellipsis, where glyphs before it
// are cut; the glyphs from its first shown; the cursor, in caretInk, on the
// glyph it's on, or after the last, on a blank; then as many glyphs after
// it as fit whole, and where they don't all, an ellipsis marking the rest
// cut; each but the cursor in typingInk.
func (f Field) typed(room int) line {
	v := f.typedView()
	from, caret := v.first(room), v.caret()
	var l line
	if from > 0 {
		l = line{{prose.Ellipsis, typingInk}}
	}
	l = append(l, span{spelt(v.glyphs[from:v.cursor]), typingInk}, span{caret.text, caretInk})
	after := min(v.cursor+1, len(v.glyphs))
	used := cutBefore(from) + v.cells(from, v.cursor) + caret.width()
	if used+v.cells(after, len(v.glyphs)) <= room {
		return append(l, span{spelt(v.glyphs[after:]), typingInk})
	}
	to := after
	for to < len(v.glyphs) && used+v.glyphs[to].width()+1 <= room {
		used += v.glyphs[to].width()
		to++
	}
	return append(l, span{spelt(v.glyphs[after:to]), typingInk}, span{prose.Ellipsis, typingInk})
}

// spelt is the text of gs, one after another.
func spelt(gs []grapheme) string {
	var b strings.Builder
	for _, g := range gs {
		b.WriteString(g.text)
	}
	return b.String()
}

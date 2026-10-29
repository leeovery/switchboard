package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/dashboard/graphemes"
	"github.com/leeovery/switchboard/internal/theme"
)

// Field is a form's text field as it's drawn: what's typed in it, and while
// it's the one typed in, as Typing has it, where its cursor is, in glyphs
// from its start, as a terminal draws them.
type Field struct {
	Value  string
	Cursor int
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
	if f.wrong != "" {
		for _, l := range (line{{f.wrong, errorInk}}).wrap(x + width - at) {
			c.line(at, next, l)
			next++
		}
	}
	return next
}

// typed is the field's value as it's drawn typed in, in no more than room
// cells: the cursor, in caretInk, on the glyph it's on, or after the last,
// on a blank; then as many glyphs before it as fit, then after it, whole,
// in typingInk. A glyph that takes no cell, as a stray combining mark, is
// drawn as a blank under the cursor, so the cursor shows.
func (f Field) typed(room int) line {
	var gs []grapheme
	for g, cells := range graphemes.In(f.Value) {
		gs = append(gs, grapheme{text: g, cells: cells})
	}
	cursor := min(max(f.Cursor, 0), len(gs))
	caret := grapheme{text: " ", cells: 1}
	if cursor < len(gs) && gs[cursor].width() > 0 {
		caret = gs[cursor]
	}
	used, from := caret.width(), cursor
	for from > 0 && used+gs[from-1].width() <= room {
		from--
		used += gs[from].width()
	}
	to := min(cursor+1, len(gs))
	for to < len(gs) && used+gs[to].width() <= room {
		used += gs[to].width()
		to++
	}
	return line{{spelt(gs[from:cursor]), typingInk}, {caret.text, caretInk}, {spelt(gs[min(cursor+1, to):to]), typingInk}}
}

// spelt is the text of gs, one after another.
func spelt(gs []grapheme) string {
	var b strings.Builder
	for _, g := range gs {
		b.WriteString(g.text)
	}
	return b.String()
}

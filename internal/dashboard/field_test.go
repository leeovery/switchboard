package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

// dayField is the field Your day, holding 08:00-23:00, as frame 7 has it,
// with the changes given.
func dayField(changes ...func(*textField)) textField {
	f := textField{name: "Your day", hint: "start and end, as HH:MM-HH:MM", field: Field{Value: "08:00-23:00", Cursor: 11}}
	for _, change := range changes {
		change(&f)
	}
	return f
}

// fieldDrawn is the field drawn from column 2 along row 0, on a canvas width
// cells wide and 4 rows tall, and the row after it.
func fieldDrawn(f textField, width int) (*canvas, int) {
	c := newCanvas(width, 4)
	return c, f.draw(c, 2, 0, width-2)
}

func typing(f *textField) { f.field.Typing = true }

func TestAFieldsRowSetsItsNameValueAndHintInColumns(t *testing.T) {
	for _, tt := range []struct {
		name  string
		field textField
		want  string
	}{
		{name: "not typed in", field: dayField(), want: "  Your day  08:00-23:00               start and end, as HH:MM-HH:MM"},
		{name: "typed in, the cursor after its value", field: dayField(typing), want: "  Your day  08:00-23:00               start and end, as HH:MM-HH:MM"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, next := fieldDrawn(tt.field, 160)
			if got := rowOf(c, 0); got != tt.want {
				t.Errorf("the field's row reads\n%q\nwant\n%q: its name from column 2, its value 10 cells on, its hint 26 cells past that", got, tt.want)
			}
			if next != 1 {
				t.Errorf("the field returns row %d, want 1, the row after it", next)
			}
		})
	}
}

func TestAFieldNotTypedInIsDrawnPlain(t *testing.T) {
	c, _ := fieldDrawn(dayField(), 160)
	for _, check := range []struct{ what, why string }{
		{"its name", inksFrom(c, 2, 0, 8, dimInk)},
		{"the cell before its value", inksFrom(c, 11, 0, 1, ink{})},
		{"its value", inksFrom(c, 12, 0, 11, secondaryInk)},
		{"the cells after its value", inksFrom(c, 23, 0, 15, ink{})},
		{"its hint", inksFrom(c, 38, 0, 29, faintInk)},
	} {
		if check.why != "" {
			t.Errorf("%s: %s", check.what, check.why)
		}
	}
}

func TestAFieldTypedInIsBoldOnItsBoxWithACursor(t *testing.T) {
	box := ink{on: hue{token: theme.BgSubtle}}
	c, _ := fieldDrawn(dayField(typing), 160)
	for _, check := range []struct{ what, why string }{
		{"its name", inksFrom(c, 2, 0, 8, dimInk)},
		{"its box's first cell, before its value", inksFrom(c, 11, 0, 1, box)},
		{"its value, bold text.primary on bg.subtle", inksFrom(c, 12, 0, 11, typingInk)},
		{"the cursor after it, a block of text.primary", inksFrom(c, 23, 0, 1, caretInk)},
		{"the rest of its box, 24 cells from its first", inksFrom(c, 24, 0, 11, box)},
		{"the cells between its box and its hint", inksFrom(c, 35, 0, 3, ink{})},
		{"its hint", inksFrom(c, 38, 0, 29, faintInk)},
	} {
		if check.why != "" {
			t.Errorf("%s: %s", check.what, check.why)
		}
	}
	if caretInk.on.token != theme.TextPrimary {
		t.Errorf("the cursor is on %v, want text.primary", caretInk.on.token)
	}
}

func TestAFieldsCursorMidValueIsOnTheRuneItsAt(t *testing.T) {
	c, _ := fieldDrawn(dayField(typing, func(f *textField) { f.field.Cursor = 2 }), 160)
	if got := rowOf(c, 0); !strings.HasPrefix(got, "  Your day  08:00-23:00 ") {
		t.Errorf("the field's row reads %q, want its value whole, nothing added for the cursor", got)
	}
	for _, check := range []struct{ what, why string }{
		{"the value before the cursor", inksFrom(c, 12, 0, 2, typingInk)},
		{"the cursor, on the colon", inksFrom(c, 14, 0, 1, caretInk)},
		{"the value after the cursor", inksFrom(c, 15, 0, 8, typingInk)},
		{"the cell after the value, the box's", inksFrom(c, 23, 0, 1, ink{on: hue{token: theme.BgSubtle}})},
	} {
		if check.why != "" {
			t.Errorf("%s: %s", check.what, check.why)
		}
	}
}

func TestAnEmptyFieldTypedInHasItsCursorFirst(t *testing.T) {
	c, _ := fieldDrawn(dayField(typing, func(f *textField) { f.field = Field{Typing: true} }), 160)
	if got := inkAt(c, 12, 0); got != caretInk {
		t.Errorf("the value's first cell is in %+v, want the cursor", got)
	}
	if got, want := rowOf(c, 0), "  Your day                            start and end, as HH:MM-HH:MM"; got != want {
		t.Errorf("the field's row reads\n%q\nwant\n%q", got, want)
	}
}

// A value of 30 cells, wider than the 23 its box holds.
func TestAValueWiderThanItsRoom(t *testing.T) {
	const wide = "abcdefghijklmnopqrstuvwxyz0123"
	tests := []struct {
		name   string
		field  Field
		value  string
		cursor int
	}{
		{name: "not typed in: cut, its end an ellipsis", field: Field{Value: wide}, value: "abcdefghijklmnopqrstuv…", cursor: -1},
		{name: "typed in, its cursor at the end: its start cut and marked, its last 21 cells, then the cursor", field: Field{Value: wide, Cursor: 30, Typing: true}, value: "…jklmnopqrstuvwxyz0123 ", cursor: 22},
		{name: "typed in, its cursor at the start: its first 22 cells, the cursor on the first, its end cut and marked", field: Field{Value: wide, Cursor: 0, Typing: true}, value: "abcdefghijklmnopqrstuv…", cursor: 0},
		{name: "typed in, its cursor mid-way and in view from the start", field: Field{Value: wide, Cursor: 10, Typing: true}, value: "abcdefghijklmnopqrstuv…", cursor: 10},
		{name: "typed in, its cursor past the view from the start: moved on as little as shows it, either end marked", field: Field{Value: wide, Cursor: 25, Typing: true}, value: "…fghijklmnopqrstuvwxyz…", cursor: 21},
		{name: "typed in, its view from the tenth, its cursor within it: held where it was", field: Field{Value: wide, Cursor: 12, From: 9, Typing: true}, value: "…jklmnopqrstuvwxyz0123 ", cursor: 4},
		{name: "typed in, its view from the tenth, its cursor before it: moved back to the cursor", field: Field{Value: wide, Cursor: 4, From: 9, Typing: true}, value: "…efghijklmnopqrstuvwxy…", cursor: 1},
		{name: "typed in, its view from past the cursor's end: moved back to show it", field: Field{Value: "abc", Cursor: 3, From: 9, Typing: true}, value: "abc                    ", cursor: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := fieldDrawn(dayField(func(f *textField) { f.field = tt.field }), 160)
			row := []rune(c.row(0, Look{}))
			if got := string(row[12:35]); got != tt.value {
				t.Errorf("the value's 23 cells read %q, want %q", got, tt.value)
			}
			if got := string(row[35:38]); got != "   " {
				t.Errorf("the cells before the hint read %q, want them blank", got)
			}
			if tt.cursor >= 0 {
				if got := inkAt(c, 12+tt.cursor, 0); got != caretInk {
					t.Errorf("cell %d of the value is in %+v, want the cursor", tt.cursor, got)
				}
			}
		})
	}
}

// InView moves a field's view as little as shows its cursor: held still
// while the cursor moves within it; on, where the cursor's past its end;
// back to the cursor, where it's before its start; and back as far as the
// value from there to its end fits, so no room's left blank that it could
// fill.
func TestAFieldsViewHoldsStillWhileItsCursorMovesWithinIt(t *testing.T) {
	const wide = "abcdefghijklmnopqrstuvwxyz0123"
	tests := []struct {
		name  string
		field Field
		from  int
	}{
		{name: "a value that fits, from its first", field: Field{Value: "08:00-23:00", Cursor: 11}, from: 0},
		{name: "the cursor at the end of a wide value: moved on to show it", field: Field{Value: wide, Cursor: 30}, from: 9},
		{name: "the cursor moved back within the view: held", field: Field{Value: wide, Cursor: 9, From: 9}, from: 9},
		{name: "the cursor a glyph before the view: moved back a glyph", field: Field{Value: wide, Cursor: 8, From: 9}, from: 8},
		{name: "the cursor at the start: from it", field: Field{Value: wide, Cursor: 0, From: 9}, from: 0},
		{name: "the cursor moved on within the view: held", field: Field{Value: wide, Cursor: 21, From: 0}, from: 0},
		{name: "the cursor moved on past the view: on as little as shows it", field: Field{Value: wide, Cursor: 22, From: 0}, from: 2},
		{name: "the value cut short, its view past it: moved back to fill the box", field: Field{Value: wide[:24], Cursor: 24, From: 9}, from: 3},
		{name: "a From past the cursor, as of a value cleared: from the cursor, its first", field: Field{From: 9}, from: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.field.InView().From; got != tt.from {
				t.Errorf("InView().From = %d, want %d", got, tt.from)
			}
		})
	}
}

// A value of 15 of one glyph, typed in, its cursor at the start, mid-way
// and at the end: its box 24 cells, from column 11, whatever the glyph's
// cells; the cursor on a whole glyph, or after the last, inside it; the
// cells after it blank up to the hint, at column 38.
func TestAFieldTypedInLaysItsGlyphsOutInCells(t *testing.T) {
	glyphs := []struct {
		name, text string
		cells      int
	}{
		{name: "a CJK character", text: "漢", cells: 2},
		{name: "an emoji", text: "😀", cells: 2},
		{name: "an emoji and its presentation", text: "❤\uFE0F", cells: 2},
		{name: "a flag", text: "🇬🇧", cells: 2},
		{name: "a family, joined", text: "👨\u200d👩\u200d👧", cells: 2},
		{name: "a letter and a combining accent", text: "e\u0301", cells: 1},
	}
	box := hue{token: theme.BgSubtle}
	for _, g := range glyphs {
		for _, cursor := range []int{0, 7, 15} {
			t.Run(fmt.Sprintf("%s, the cursor at %d", g.name, cursor), func(t *testing.T) {
				field := Field{Value: strings.Repeat(g.text, 15), Cursor: cursor, Typing: true}
				c, _ := fieldDrawn(dayField(func(f *textField) { f.field = field }), 160)
				var caret []int
				for x := 11; x < 35; x++ {
					switch k := inkAt(c, x, 0); {
					case k == caretInk:
						caret = append(caret, x)
					case k.on != box:
						t.Errorf("column %d is on %+v, want the box's bg.subtle", x, k.on)
					}
				}
				want := g.cells
				if cursor == 15 {
					want = 1
				}
				if len(caret) != want || caret[0] < 12 {
					t.Fatalf("the cursor is on columns %v, want %d of them, in the box past its first", caret, want)
				}
				if glyph := c.cells[0][caret[0]].glyph; cursor < 15 && glyph != g.text || cursor == 15 && glyph != " " {
					t.Errorf("the cursor is on %q, want the glyph it's at whole, or a blank after the last", glyph)
				}
				for x := 35; x < 38; x++ {
					if k := inkAt(c, x, 0); k != (ink{}) || c.cells[0][x].glyph != " " {
						t.Errorf("column %d holds %q in %+v, want it blank, between the box and the hint", x, c.cells[0][x].glyph, k)
					}
				}
				if row := []rune(c.row(0, Look{})); !strings.HasSuffix(string(row), "start and end, as HH:MM-HH:MM") {
					t.Errorf("the row reads %q, want the hint at its end", string(row))
				}
				if got := inkAt(c, 38, 0); got != faintInk || c.cells[0][38].glyph != "s" {
					t.Errorf("column 38 holds %q in %+v, want the hint's first", c.cells[0][38].glyph, got)
				}
			})
		}
	}
}

// The cursor reversed where the look has no colour for text.primary as a
// surface, without colour and in the terminal theme; else on text.primary's
// colour.
func TestAFieldsCursorShowsInEveryLook(t *testing.T) {
	nord := builtin(t, "nord")
	tests := []struct {
		name     string
		look     Look
		reversed bool
	}{
		{name: "without colour", look: NoColour(), reversed: true},
		{name: "in the terminal theme", look: Screen(builtin(t, theme.Terminal)), reversed: true},
		{name: "in nord", look: Screen(nord)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.look.reversed(caretInk); got != tt.reversed {
				t.Errorf("the cursor is reversed: %v, want %v", got, tt.reversed)
			}
			c, _ := fieldDrawn(dayField(typing), 160)
			if got := strings.Contains(c.row(0, tt.look), "\x1b[7m"); got != tt.reversed {
				t.Errorf("the row is drawn reversed somewhere: %v, want %v", got, tt.reversed)
			}
			if !tt.reversed && !sameColor(tt.look.surface(caretInk), nord.Colour(theme.TextPrimary)) {
				t.Errorf("the cursor is on %v, want text.primary's colour", tt.look.surface(caretInk))
			}
		})
	}
}

func TestAStrayMarkUnderTheCursorShowsTheCursor(t *testing.T) {
	c, _ := fieldDrawn(dayField(func(f *textField) { f.field = Field{Value: "\u0301ab", Cursor: 0, Typing: true} }), 160)
	if got := inkAt(c, 12, 0); got != caretInk {
		t.Errorf("the value's first cell is in %+v, want the cursor", got)
	}
}

func TestWhatsWrongWithAFieldsValueIsSaidUnderIt(t *testing.T) {
	const wrong = `prime.day "8-9": must be two times of day, HH:MM, joined by -, such as 08:00-23:00`
	t.Run("on the row under it, from its value's column, in state.destructive", func(t *testing.T) {
		c, next := fieldDrawn(dayField(func(f *textField) { f.wrong = wrong }), 160)
		if got, want := rowOf(c, 1), blanks(12)+wrong; got != want {
			t.Errorf("the row under the field reads\n%q\nwant\n%q", got, want)
		}
		if why := inksFrom(c, 12, 1, 4, errorInk); why != "" {
			t.Error(why)
		}
		if next != 2 {
			t.Errorf("the field returns row %d, want 2, the row after what's wrong", next)
		}
	})
	t.Run("wrapped where it's wider than the row", func(t *testing.T) {
		c, next := fieldDrawn(dayField(func(f *textField) { f.wrong = wrong }), 70)
		if got, want := rowOf(c, 1), blanks(12)+`prime.day "8-9": must be two times of day, HH:MM, joined`; got != want {
			t.Errorf("the first row under the field reads\n%q\nwant\n%q", got, want)
		}
		if got, want := rowOf(c, 2), blanks(12)+"by -, such as 08:00-23:00"; got != want {
			t.Errorf("the second row under the field reads\n%q\nwant\n%q", got, want)
		}
		if next != 3 {
			t.Errorf("the field returns row %d, want 3", next)
		}
	})
}

func TestAFieldsHintGivesWayWhereTheresNoRoom(t *testing.T) {
	c, _ := fieldDrawn(dayField(), 2+9+24+3)
	if got, want := rowOf(c, 0), "  Your day  08:00-23:00"; got != want {
		t.Errorf("the field's row reads %q, want %q, its hint left off", got, want)
	}
	c, _ = fieldDrawn(dayField(), 2+9+24+3+10)
	if got, want := rowOf(c, 0), "  Your day  08:00-23:00               start and…"; got != want {
		t.Errorf("the field's row reads %q, want %q, its hint cut to its room", got, want)
	}
}

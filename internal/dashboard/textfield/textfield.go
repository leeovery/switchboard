// Package textfield is a text field's editing, for a screen that runs a form
// in Bubble Tea: what's typed in it, and where its cursor is, moved and
// edited by the keys a terminal sends, and by a paste, a character at a time
// as the terminal draws characters, a grapheme, never part of one. How it's
// drawn is the dashboard's.
package textfield

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/graphemes"
)

// Text is what's typed in a field, and where its cursor is: before the
// grapheme of its value it's at, or after the last, at their count. The zero
// Text is an empty field.
type Text struct {
	value  string
	cursor int
}

// Of is a field holding value, its cursor after the last of it.
func Of(value string) Text {
	return Text{value: value, cursor: len(charactersOf(value))}
}

// Value is what's typed in the field.
func (t Text) Value() string {
	return t.value
}

// Cursor is where the field's cursor is, in graphemes from its start.
func (t Text) Cursor() int {
	return t.cursor
}

// Pressed is the field once key is pressed in it, reporting false for a key
// it doesn't take: ← and → move the cursor a grapheme, home and end to the
// start and the end; backspace takes out the grapheme before the cursor, and
// delete the one after; and a key that types, without ctrl or alt, puts in
// what it types, as Pasted does. A key it takes that changes nothing, as ←
// at the start, is taken all the same.
func (t Text) Pressed(key tea.KeyPressMsg) (Text, bool) {
	gs := charactersOf(t.value)
	switch key.String() {
	case "left":
		return t.at(gs, t.cursor-1), true
	case "right":
		return t.at(gs, t.cursor+1), true
	case "home":
		return t.at(gs, 0), true
	case "end":
		return t.at(gs, len(gs)), true
	case "backspace":
		if t.cursor == 0 {
			return t, true
		}
		return joined(gs[:t.cursor-1], "", gs[t.cursor:]), true
	case "delete":
		if t.cursor == len(gs) {
			return t, true
		}
		return joined(gs[:t.cursor], "", gs[t.cursor+1:]), true
	}
	if key.Text == "" || key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) {
		return t, false
	}
	return t.Pasted(key.Text), true
}

// Pasted is the field with text put in at its cursor, the cursor after it,
// all of it but what kept leaves out.
func (t Text) Pasted(text string) Text {
	gs := charactersOf(t.value)
	return joined(gs[:t.cursor], strings.Map(kept, text), gs[t.cursor:])
}

// at is the field, its value the graphemes gs, with its cursor at i, kept
// within it.
func (t Text) at(gs []string, i int) Text {
	t.cursor = min(max(i, 0), len(gs))
	return t
}

// joined is the field holding the graphemes before, then put, then the
// graphemes after, its cursor after put, counted as the graphemes fall once
// joined, as an accent put after a letter joins it.
func joined(before []string, put string, after []string) Text {
	upTo := strings.Join(before, "") + put
	return Text{value: upTo + strings.Join(after, ""), cursor: len(charactersOf(upTo))}
}

// charactersOf are text's characters as a terminal draws them, as
// graphemes has them.
func charactersOf(text string) []string {
	var gs []string
	for g := range graphemes.In(text) {
		gs = append(gs, g)
	}
	return gs
}

// kept is r where the field keeps it, and none otherwise, as strings.Map
// drops a rune mapped below zero: every rune but those that would disturb
// the terminal or the line, every control, an escape, a tab and a line
// break among them, the line and paragraph separators, and the controls
// that turn the text's direction. Joiners, tags, variation selectors and
// the rest are kept wherever they come: what a value may hold besides is
// that value's own rule, held where it's checked.
func kept(r rune) rune {
	if unicode.Is(unicode.Cc, r) || unicode.In(r, unicode.Zl, unicode.Zp) || unicode.Is(unicode.Bidi_Control, r) {
		return -1
	}
	return r
}

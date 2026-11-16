// Package graphemes is what one character is, as a terminal draws it: a
// grapheme, an accented letter, an emoji and its presentation, a flag or a
// family joined, and the cells it takes. The dashboard's canvas draws by it
// and a text field edits by it, so the two never disagree on where one
// character ends.
package graphemes

import (
	"iter"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// In are text's characters, in order, each with the cells it takes: one,
// two where it's wide, or none, as a stray combining mark. Ranging over
// them allocates nothing.
func In(text string) iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		for text != "" {
			g, width := first(text)
			text = text[len(g):]
			if !yield(g, min(width, 2)) {
				return
			}
		}
	}
}

// first is text's first character and the cells it takes. Most of what the
// dashboard draws is plain: a printable ASCII character, a line or a block
// of a bar where it's a cell, which nothing but a character past them can
// join, as an accent or a joiner does. Such a one followed by another, or by
// nothing, is a character alone, told without the cluster rules, which take
// many times longer.
func first(text string) (string, int) {
	r, size := utf8.DecodeRuneInString(text)
	if plain(r) && (size == len(text) || plainNext(text[size:])) {
		return text[:size], 1
	}
	return ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
}

// plainNext reports whether text starts with a character no character
// before it joins: any ASCII byte, or a plain character.
func plainNext(text string) bool {
	if text[0] < utf8.RuneSelf {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text)
	return plain(r)
}

// plain reports whether r is a character of one cell that joins nothing
// after it: a printable ASCII one, a space to a tilde, or one of the box
// drawing characters or block elements, U+2500 to U+259F, that the cluster
// rules give a cell, as boxOneCell has them.
func plain(r rune) bool {
	return ' ' <= r && r <= '~' || boxFirst <= r && r <= boxLast && boxOneCell[r-boxFirst]
}

// boxFirst and boxLast are the first and last of the box drawing characters
// and block elements.
const (
	boxFirst = '─'
	boxLast  = '▟'
)

// boxOneCell are the box drawing characters and block elements, from
// boxFirst, that the cluster rules give a cell. Most are East Asian
// Ambiguous, two cells where ansi is set so, by RUNEWIDTH_EASTASIAN as it
// starts, so they're asked of it rather than assumed.
var boxOneCell = func() (one [boxLast - boxFirst + 1]bool) {
	for r := boxFirst; r <= boxLast; r++ {
		_, width := ansi.FirstGraphemeCluster(string(r), ansi.GraphemeWidth)
		one[r-boxFirst] = width == 1
	}
	return one
}()

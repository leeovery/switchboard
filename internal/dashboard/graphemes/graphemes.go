// Package graphemes is what one character is, as a terminal draws it: a
// grapheme, an accented letter, an emoji and its presentation, a flag or a
// family joined, and the cells it takes. The dashboard's canvas draws by it
// and a text field edits by it, so the two never disagree on where one
// character ends.
package graphemes

import (
	"iter"

	"github.com/charmbracelet/x/ansi"
)

// In are text's characters, in order, each with the cells it takes: one,
// two where it's wide, or none, as a stray combining mark. Ranging over
// them allocates nothing.
func In(text string) iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		for text != "" {
			g, width := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
			text = text[len(g):]
			if !yield(g, min(width, 2)) {
				return
			}
		}
	}
}

package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// ellipsis marks where text was cut short, or what wasn't shown.
const ellipsis = "…"

// span is a run of text in one ink.
type span struct {
	text string
	ink  ink
}

// line is one line of a frame, its spans laid left to right. Layout works on
// lines rather than strings so that widths are measured on the text alone and
// colour is decided only when the frame is drawn.
type line []span

// width is how many cells the line takes.
func (l line) width() int {
	n := 0
	for _, s := range l {
		n += ansi.StringWidth(s.text)
	}
	return n
}

// draw writes the line to b, its inks drawn as look draws them.
func (l line) draw(b *strings.Builder, look Look) {
	for _, s := range l {
		b.WriteString(look.render(s.text, s.ink))
	}
}

// fit cuts the line to width cells, ending it with an ellipsis where it's cut.
func (l line) fit(width int) line {
	if l.width() <= width {
		return l
	}
	if width < 1 {
		return nil
	}
	var kept line
	for _, s := range l {
		if w := ansi.StringWidth(s.text); w < width {
			kept = append(kept, s)
			width -= w
			continue
		}
		return append(kept, span{ellipsize(s.text, width), s.ink})
	}
	return kept
}

// wrap breaks the line at its spaces onto as many lines width cells wide as
// it takes, each word in its ink, and a word wider than width cut short.
func (l line) wrap(width int) []line {
	var lines []line
	var row line
	for i, w := range l.words() {
		switch {
		case i == 0:
			row = w.word
		case row.width()+1+w.word.width() <= width:
			row = slices.Concat(row, line{w.gap}, w.word)
		default:
			lines, row = append(lines, row.fit(width)), w.word
		}
	}
	if row != nil {
		lines = append(lines, row.fit(width))
	}
	return lines
}

// worded is a word of a line, its spans' parts up to a space, and the space
// before it, in its ink.
type worded struct {
	word line
	gap  span
}

// words are the line's words, as wrap breaks it between them: a word may
// run across spans, as a name in its ink and the full stop after it.
func (l line) words() []worded {
	var words []worded
	var w worded
	for _, s := range l {
		for i, part := range strings.Split(s.text, " ") {
			if i > 0 && len(w.word) > 0 {
				words, w = append(words, w), worded{}
			}
			if i > 0 {
				w.gap = span{" ", s.ink}
			}
			if part != "" {
				w.word = append(w.word, span{part, s.ink})
			}
		}
	}
	if len(w.word) > 0 {
		words = append(words, w)
	}
	return words
}

// leastShown is the fewest cells of words cut short worth showing: where
// fewer would show, they're left off.
const leastShown = 5

// fitWhole cuts the line to width cells by leaving off its last spans, each
// whole, as many as don't fit; where even its first doesn't fit alone, that
// alone, cut short as fit cuts it.
func (l line) fitWhole(width int) line {
	for n := len(l); n > 0; n-- {
		if l[:n].width() <= width {
			return l[:n]
		}
	}
	return l[:min(len(l), 1)].fit(width)
}

// spread puts left and right at either end of width cells, cutting left to
// keep a space between them.
func spread(left, right line, width int) line {
	left = left.fit(width - right.width() - 1)
	return slices.Concat(left, line{spaces(width - left.width() - right.width())}, right)
}

func spaces(n int) span {
	return span{text: strings.Repeat(" ", max(n, 0))}
}

// padded is text with blanks after it to fill cells cells, as a column has
// it: as it is where it fills them already.
func padded(text string, cells int) string {
	return text + strings.Repeat(" ", max(cells-ansi.StringWidth(text), 0))
}

// rule is a horizontal border n cells long.
func rule(n int) string {
	return strings.Repeat("─", max(n, 0))
}

// truncate cuts text to width cells, ending it with an ellipsis where it's
// cut.
func truncate(text string, width int) string {
	switch {
	case ansi.StringWidth(text) <= width:
		return text
	case width < 1:
		return ""
	default:
		return ellipsize(text, width)
	}
}

// ellipsize cuts text to leave room in width cells for the ellipsis it ends
// with.
func ellipsize(text string, width int) string {
	return strings.TrimRight(ansi.Truncate(text, width-ansi.StringWidth(ellipsis), ""), " ") + ellipsis
}

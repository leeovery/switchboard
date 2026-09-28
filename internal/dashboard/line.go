package dashboard

import (
	"slices"
	"strings"
	"unicode"

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

// draw writes the line to b, in its inks when color is set.
func (l line) draw(b *strings.Builder, color bool) {
	for _, s := range l {
		if color {
			b.WriteString(s.ink.render(s.text))
		} else {
			b.WriteString(s.text)
		}
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

// spread puts left and right at either end of width cells, cutting left to
// keep a space between them.
func spread(left, right line, width int) line {
	left = left.fit(width - right.width() - 1)
	return slices.Concat(left, line{spaces(width - left.width() - right.width())}, right)
}

func spaces(n int) span {
	return span{text: strings.Repeat(" ", max(n, 0))}
}

// rule is a horizontal border n cells long.
func rule(n int) string {
	return strings.Repeat("─", max(n, 0))
}

// clean makes text fit to lay out on one line: control characters, which
// would move the cursor or restyle what follows, become spaces, and each run
// of spaces becomes one.
func clean(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
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

// wrap breaks text into lines of at most width cells, between words where it
// can, and keeps at most maxLines of them: the last ends in an ellipsis when
// the text runs on past it.
func wrap(text string, width, maxLines int) []string {
	var lines []string
	for word := range strings.FieldsSeq(text) {
		if last := len(lines) - 1; last >= 0 && ansi.StringWidth(lines[last])+1+ansi.StringWidth(word) <= width {
			lines[last] += " " + word
			continue
		}
		lines = append(lines, breakWord(word, width)...)
	}
	if len(lines) > maxLines {
		rest := strings.Join(lines[maxLines-1:], " ")
		lines = append(lines[:maxLines-1], truncate(rest, width))
	}
	return lines
}

// breakWord splits a word into pieces of at most width cells.
func breakWord(word string, width int) []string {
	var pieces []string
	for ansi.StringWidth(word) > width {
		piece := ansi.Truncate(word, width, "")
		if piece == "" {
			// A character wider than the whole width; truncating the line
			// is all that can be done with it.
			break
		}
		pieces = append(pieces, piece)
		word = word[len(piece):]
	}
	return append(pieces, word)
}

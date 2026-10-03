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

// fitHead cuts the line to width cells as fit does, but keeps its first span
// whole where that fits by itself: what follows is cut short, or left off
// where fewer than leastShown cells of it would show.
func (l line) fitHead(width int) line {
	if l.width() <= width || len(l) == 0 {
		return l
	}
	left := width - l[:1].width()
	switch {
	case left < 0:
		return l.fit(width)
	case left < leastShown:
		return l[:1]
	default:
		return slices.Concat(l[:1], l[1:].fit(left))
	}
}

// leastShown is the fewest cells of a line's tail fitHead shows, cut short.
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

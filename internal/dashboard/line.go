package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
)

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

// fit cuts the line, words, to width cells, ending it with an ellipsis where
// it's cut, as fitParts cuts words.
func (l line) fit(width int) line {
	return fitParts(width, wordsPart(l))
}

// inked is the line as it's drawn, each run of one ink a span, however its
// spans were split.
func (l line) inked() line {
	var runs line
	for _, s := range l {
		if n := len(runs); n > 0 && runs[n-1].ink == s.ink {
			runs[n-1].text += s.text
			continue
		}
		runs = append(runs, s)
	}
	return runs
}

// elided is the line, a figure, as an ellipsis alone, in its first span's ink
// but as no number: the figure it stands for isn't shown, so its changes
// can't be.
func (l line) elided() line {
	return line{{prose.Ellipsis, l[0].ink.unnumbered()}}
}

// part is a part of a line fitParts cuts: words, which may be cut part-way;
// or a figure, never cut part-way, as $1,234,5… would read as a smaller sum,
// marked as the number it is, where it's one, only where it's drawn whole.
type part struct {
	said   line
	figure bool
	number Number
}

// wordsPart is the line as words.
func wordsPart(said line) part {
	return part{said: said}
}

// figurePart is the line as a figure, the number n, or none.
func figurePart(said line, n Number) part {
	return part{said: said, figure: true, number: n}
}

// drawn is the part drawn whole: a figure marked as its number, where it's
// one; words as they are.
func (p part) drawn() line {
	if !p.figure || p.number == (Number{}) {
		return p.said
	}
	return p.said.numbered(p.number)
}

// fitParts is the parts, one after another, cut to width cells. Where they
// run past it, words are cut, ending with an ellipsis; a figure is drawn
// whole where it fits what's left, the rest left off where it fills it,
// else as an ellipsis alone, as elided has it. In 14 cells, "4.6× its $1,800
// month" cuts to "4.6× its $1,8…" as words, but to "4.6× its …" with $1,800 a
// figure; in 15, to "4.6× its $1,800", the figure filling them.
func fitParts(width int, parts ...part) line {
	room, cutting := width, partsWidth(parts) > width
	if cutting && width < 1 {
		return nil
	}
	var kept line
	for _, p := range pieces(parts) {
		w := p.said.width()
		switch {
		case !cutting || w < room:
			kept, room = append(kept, p.drawn()...), room-w
		case !p.figure:
			return append(kept, span{prose.CutShort(p.said[0].text, room), p.said[0].ink})
		case w == room:
			return append(kept, p.drawn()...)
		default:
			return append(kept, p.said.elided()...)
		}
	}
	return kept
}

// partsWidth is how many cells the parts take.
func partsWidth(parts []part) int {
	n := 0
	for _, p := range parts {
		n += p.said.width()
	}
	return n
}

// pieces are the parts as fitParts weighs them: each figure whole, and words
// a span at a time, each a part of one span, so each is cut in its own ink.
func pieces(parts []part) []part {
	var split []part
	for _, p := range parts {
		if p.figure {
			split = append(split, p)
			continue
		}
		for _, s := range p.said {
			split = append(split, wordsPart(line{s}))
		}
	}
	return split
}

// wordsIn is text as words, in the ink given.
func wordsIn(text string, k ink) part {
	return wordsPart(line{{text, k}})
}

// figureIn is text as a figure, in the ink given, as no number.
func figureIn(text string, k ink) part {
	return figurePart(line{{text, k}}, Number{})
}

// joinedParts are the groups of parts one after another, apart between each
// two, as joined joins lines: a group with nothing in it left out.
func joinedParts(apart span, groups ...[]part) []part {
	var joined []part
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		if joined != nil {
			joined = append(joined, wordsPart(line{apart}))
		}
		joined = append(joined, g...)
	}
	return joined
}

// wrap breaks the line at its spaces onto as many lines width cells wide as
// it takes, each word in its ink, and a word wider than width cut short.
func (l line) wrap(width int) []line {
	return wrapParts(width, wordsPart(l))
}

// wrapParts breaks the parts at the spaces in their words onto as many lines
// width cells wide as it takes, a figure never broken: each line cut as
// fitParts cuts it, where a word, or a figure, is wider than width.
func wrapParts(width int, parts ...part) []line {
	var lines []line
	var row []part
	for i, w := range partWords(parts) {
		switch {
		case i == 0:
			row = w.word
		case partsWidth(row)+1+partsWidth(w.word) <= width:
			row = slices.Concat(row, []part{wordsPart(line{w.gap})}, w.word)
		default:
			lines, row = append(lines, fitParts(width, row...)), w.word
		}
	}
	if row != nil {
		lines = append(lines, fitParts(width, row...))
	}
	return lines
}

// worded is a word of parts, its words' spans up to a space and any figure
// among them, and the space before it, in its ink.
type worded struct {
	word []part
	gap  span
}

// partWords are the parts' words, as wrapParts breaks them between: a word may
// run across spans, as a name in its ink and the full stop after it, and a
// figure is a word whole, or part of one, spaces and all.
func partWords(parts []part) []worded {
	var words []worded
	var w worded
	for _, p := range parts {
		if p.figure {
			w.word = append(w.word, p)
			continue
		}
		for _, s := range p.said {
			for i, piece := range strings.Split(s.text, " ") {
				if i > 0 && len(w.word) > 0 {
					words, w = append(words, w), worded{}
				}
				if i > 0 {
					w.gap = span{" ", s.ink}
				}
				if piece != "" {
					w.word = append(w.word, wordsIn(piece, s.ink))
				}
			}
		}
	}
	if len(w.word) > 0 {
		words = append(words, w)
	}
	return words
}

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

// firstFit is the first of forms, each shorter than the one before, that fits
// in width cells; where none does, the last, cut short.
func firstFit(width int, forms ...line) line {
	for _, l := range forms {
		if l.width() <= width {
			return l
		}
	}
	return forms[len(forms)-1].fit(width)
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

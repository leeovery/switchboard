package dashboard

import (
	"slices"
	"strings"
)

// helpChrome is the rows of help's card around its body: its top and bottom
// edges, and the blank row and keys row over its foot.
const helpChrome = 4

// Help is the page's help, open over it, as the watch gives it: every key
// that works on the page, those the key line leaves out among them, worded
// as the key line words them; and how many rows it's scrolled down by. The
// page's name, for HELP · <PAGE>, and its parts come of the frame: of the
// views milestone 5 built, the glyphs their key explains.
type Help struct {
	Keys   []Key
	Scroll int
}

// card is the help card of the frame's page: its keys first, as rows
// keysApart apart, then its parts, as helpParts has them. It's as tall as
// they are, up to the frame's height less two rows; past that, it shows
// those Scroll rows down, no further than its last row, and j k scroll
// joins esc close in its keys row.
func (h Help) card(f Frame) Card {
	width := f.cardWidth(helpWidth)
	body := slices.Concat(keyRows(h.Keys, width-2*cardInset), f.helpParts())
	shown := min(max(f.Height-2-helpChrome, 0), len(body))
	from := min(max(h.Scroll, 0), len(body)-shown)
	keys := []Key{{Key: "esc", Does: "close"}}
	if shown < len(body) {
		keys = slices.Insert(keys, 0, Key{Key: "j k", Does: "scroll"})
	}
	return Card{
		title:  "HELP · " + strings.ToUpper(f.View.Title()),
		width:  width,
		body:   body[from : from+shown],
		keys:   keys,
		scroll: cardScroll{above: from, below: len(body) - from - shown},
	}
}

// keyRows are the keys as rows of help lists them, as the key line words
// them and keysApart apart, as many to a row as fit width cells.
func keyRows(keys []Key, width int) []line {
	var rows []line
	var row line
	for _, k := range keys {
		words := k.words()
		switch {
		case len(row) == 0:
			row = words
		case row.width()+keysApart+words.width() <= width:
			row = slices.Concat(row, line{spaces(keysApart)}, words)
		default:
			rows = append(rows, row)
			row = words
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// helpParts are the parts of the page help explains after its keys, each a
// blank row and then its rows: of the views milestone 5 built, each group of
// the glyphs their key explains, a glyph to a row, drawn as the view draws
// it, then what it means; the page's glyphs in one column, glyphsGap cells
// wider than the widest of them.
func (f Frame) helpParts() []line {
	groups := f.glyphs()
	column := 0
	for _, group := range groups {
		for _, g := range group {
			column = max(column, g.drawn.width())
		}
	}
	var rows []line
	for _, group := range groups {
		rows = append(rows, nil)
		for _, g := range group {
			rows = append(rows, slices.Concat(g.drawn, line{spaces(column - g.drawn.width() + glyphsGap), {g.means, secondaryInk}}))
		}
	}
	return rows
}

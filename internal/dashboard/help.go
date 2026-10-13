package dashboard

import (
	"slices"
	"strings"
)

// Help is the page's help, open over it, as the watch gives it: every key
// that works on the page, those the key line leaves out among them, worded
// as the key line words them; and how many rows it's scrolled down by. The
// page's name, for HELP · <PAGE>, and its parts come of the frame, as
// helpName and helpParts have them.
type Help struct {
	Keys   []Key
	Scroll int
}

// card is the help card of the frame's page: its keys first, as rows
// keysApart apart, then its parts, as helpRows lays them out. It's as tall
// as they are, up to the frame's height less two rows; past that, it shows
// those Scroll rows down, no further than its last row, and j k scroll
// joins esc close in its keys row.
func (h Help) card(f Frame) Card {
	width := f.cardWidth(helpWidth)
	inside := width - 2*cardInset
	body := slices.Concat(keyRows(h.Keys, inside), helpRows(f.helpParts(), inside))
	shown := min(max(f.bodyRoom(2, true), 0), len(body))
	from := min(max(h.Scroll, 0), len(body)-shown)
	keys := []Key{{Key: "esc", Does: "close"}}
	if shown < len(body) {
		keys = slices.Insert(keys, 0, Key{Key: "j k", Does: "scroll"})
	}
	return Card{
		title:  "HELP · " + strings.ToUpper(f.helpName()),
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

// helpPart is a part of the page help explains after its keys: its heading,
// where it has one, as a section's name; what it says of the part, a
// paragraph a line; and the glyphs it explains.
type helpPart struct {
	heading string
	says    []line
	glyphs  []glyph
}

// helpParts are the parts of the page help explains after its keys: of a
// session's page, as sessionHelp has them; of the Overview, a part a
// section, in the page's order, as overviewHelp has them; of History, as
// historyHelp has them; of the other views built, a part a group of the
// glyphs their key explains.
func (f Frame) helpParts() []helpPart {
	switch {
	case f.Session != nil:
		return sessionHelp()
	case f.View == Overview:
		return overviewHelp()
	case f.View == History:
		return f.historyHelp()
	}
	var parts []helpPart
	for _, group := range f.glyphs() {
		parts = append(parts, helpPart{glyphs: group})
	}
	return parts
}

// helpRows are the parts as help's rows, width cells wide, each part a blank
// row and then its rows: its heading, in text.subtle; what it says, each
// paragraph wrapped to width; and its glyphs, a glyph to a row, drawn as the
// page draws it, then what it means in text.secondary, wrapped under itself.
// Every part's glyphs share one column, glyphsGap cells wider than the
// widest of them.
func helpRows(parts []helpPart, width int) []line {
	column := 0
	for _, part := range parts {
		for _, g := range part.glyphs {
			column = max(column, g.drawn.width()+glyphsGap)
		}
	}
	var rows []line
	for _, part := range parts {
		rows = append(rows, nil)
		if part.heading != "" {
			rows = append(rows, line{{part.heading, dimInk}})
		}
		for _, said := range part.says {
			rows = append(rows, said.wrap(width)...)
		}
		for _, g := range part.glyphs {
			rows = append(rows, glyphRows(g, column, width)...)
		}
	}
	return rows
}

// glyphRows are a glyph's rows in help, width cells wide: the glyph, drawn
// as the page draws it, then from column what it means, in text.secondary,
// wrapped under itself.
func glyphRows(g glyph, column, width int) []line {
	meaning := line{{g.means, secondaryInk}}
	wrapped := []line{meaning}
	if column+meaning.width() > width {
		wrapped = meaning.wrap(width - column)
	}
	var rows []line
	for i, means := range wrapped {
		lead := line{spaces(column)}
		if i == 0 {
			lead = append(slices.Clone(g.drawn), spaces(column-g.drawn.width()))
		}
		rows = append(rows, slices.Concat(lead, means))
	}
	return rows
}

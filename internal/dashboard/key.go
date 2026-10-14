package dashboard

import (
	"strings"
)

// glyphsGap is the blank cells between a glyph help explains and what it
// means, after the widest of them.
const glyphsGap = 3

// forTheKey says the key to the glyphs is behind ?, where the view has no
// room to show it.
var forTheKey = line{{"?", keyInk}, {" for the key", dimInk}}

// glyph is a glyph of the view, drawn as the view draws it, and what it
// means.
type glyph struct {
	drawn line
	means string
}

// glyphs are the glyphs the key explains of the view shown. Of Runway, its
// legend's. Of Sessions, its switchboard's and its requests', as
// switchboardGlyphs has them. A tab not built yet has none.
func (f Frame) glyphs() [][]glyph {
	switch f.View {
	case Runway:
		return [][]glyph{f.legendGlyphs()}
	case Sessions:
		return switchboardGlyphs()
	default:
		return nil
	}
}

// switchboardGlyphs are the glyphs of Sessions, as it draws them, in two
// groups: its switchboard's, a cord from a call to its account's line, a free
// jack, and a cord hanging loose from one, as a move lets it go; then what a
// call's row says of its request: asking, its answer streaming back, refused,
// throttled, and moved here.
func switchboardGlyphs() [][]glyph {
	return [][]glyph{
		{
			{drawn: line{{plugGlyph + strings.Repeat(heavyCord.along, 2) + pluggedJack, cordInk(1, true)}}, means: "a session's cord to its account"},
			{drawn: line{{freeJack, freeJackInk}}, means: "a free jack"},
			{drawn: line{{strings.Repeat(hangingGlyph, 2), dimInk}, {freeJack, freeJackInk}}, means: "a cord a move let go"},
		},
		{
			{drawn: line{{"↑", nameInk}}, means: "asking"},
			{drawn: line{{"↓", titleInk}}, means: "streaming back, ~ its tokens so far"},
			{drawn: line{{refusedJack, exhaustedInk}}, means: "a limit or a refusal"},
			{drawn: line{{"…", dimInk}}, means: "throttled"},
			{drawn: line{{"↪", nameInk}}, means: "moved here"},
		},
	}
}

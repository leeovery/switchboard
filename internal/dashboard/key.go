package dashboard

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

// glyphs are the glyphs the key explains of the view shown: of Runway, its
// legend's. A tab not built yet has none.
func (f Frame) glyphs() [][]glyph {
	if f.Shown() == Runway {
		return [][]glyph{f.legendGlyphs()}
	}
	return nil
}

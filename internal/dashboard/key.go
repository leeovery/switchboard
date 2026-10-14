package dashboard

// glyphsGap is the blank cells between a glyph help explains and what it
// means, after the widest of them.
const glyphsGap = 3

// glyph is a glyph of the view, drawn as the view draws it, and what it
// means.
type glyph struct {
	drawn line
	means string
}

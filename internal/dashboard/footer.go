package dashboard

import "slices"

// Key is a key the footer lists, and what it does, as "m" and "move". One
// Always listed is listed wherever there's room for it, as q is, the rest
// dropping from the end of the list where they don't all fit.
type Key struct {
	Key, Does string
	Always    bool
}

// words are the key and what it does, as the footer lists them.
func (k Key) words() line {
	return line{{k.Key, keyInk}, {" " + k.Does, mutedInk}}
}

// The blank cells between the footer's keys, and on a phone, fewer.
const (
	keysGap      = 3
	phoneKeysGap = 2
)

// footer draws the footer on row y: at its right, how reading goes; and at
// its left, what the last key did while that's news, else the keys that
// work, in their order, as many as fit, as fitted says.
func (f Frame) footer(c *canvas, y int) {
	gap := keysGap
	if f.phone() {
		gap = phoneKeysGap
	}
	limit := f.edge()
	if f.Status != "" {
		limit = c.right(f.edge(), y, line{{f.Status, dimInk}}.fit(f.edge()-margin)) - gap
	}
	if f.Note != "" {
		c.line(margin, y, line{{f.Note, noteInk}}.fit(limit-margin))
		return
	}
	x := margin
	for i, k := range fitted(f.Keys, limit-margin, gap) {
		if i > 0 {
			x += gap
		}
		x = c.line(x, y, k.words())
	}
}

// fitted are the keys of keys that fit in room cells, gap apart, each whole,
// in their order: those Always listed, and as many of the rest as fit beside
// them, dropping from the end; or, where those Always listed don't fit alone,
// as many of them as do.
func fitted(keys []Key, room, gap int) []Key {
	keys = slices.Clone(keys)
	for len(keys) > 0 && listedWidth(keys, gap) > room {
		last := len(keys) - 1
		for i, k := range slices.Backward(keys) {
			if !k.Always {
				last = i
				break
			}
		}
		keys = slices.Delete(keys, last, last+1)
	}
	return keys
}

// listedWidth is how many cells keys take, listed gap apart.
func listedWidth(keys []Key, gap int) int {
	cells := 0
	for i, k := range keys {
		if i > 0 {
			cells += gap
		}
		cells += k.words().width()
	}
	return cells
}

package dashboard

import (
	"cmp"
	"slices"
)

// The blank cells the key line sets its keys apart by: within a group,
// after one, and on a phone's line, between any two.
const (
	keysInGroup = 2
	KeysAfter   = 4
	keysApart   = 3
)

// Quit is q, which quits, as the key line and help list it.
var Quit = Key{Key: "q", Does: "quit"}

// KeyLine is the key line, the page's last row: the keys that work where
// they are, in their groups, or a note in their place.
type KeyLine struct {
	// Groups are the keys in their groups, in order: tab views first, then
	// the page's own, then p pin where a pin can be set, and ? help, q quit
	// last.
	Groups [][]Key
	// Lead is the blank cells after the first group, tab views: three, or
	// five on the Overview; and After those after each group past it: five
	// on the Overview, or where it's 0, KeysAfter. Within a group, keys are
	// keysInGroup apart.
	Lead, After int
	// Phone are the keys a phone's line keeps between the first group, tab
	// views, which leads it while it fits, and ? and q quit, which end it,
	// as the page words them there; and PhoneHelp is ?'s word there: "keys",
	// or on the Overview, "keys and toggles". A line without it, as the
	// theme picker's, is laid out on a phone as on a wider terminal.
	Phone     []Key
	PhoneHelp string
	// PhoneKept are the keys a phone's line keeps after Phone's, last to
	// give way, as the dial's n while a past period shows; where
	// there are any, tab views gives way after Phone's, not before, the one
	// exception to the phone's order.
	PhoneKept []Key
	// Narrower are the wide line's narrower forms, each its groups as Groups
	// has them, tried in their order where the one before doesn't fit whole,
	// before a phone's line takes its place: as a page of toggles leaves out
	// its keys kept to a line wider than a phone's, a group at a time, or as
	// a tab with the dial leaves p pin out.
	Narrower [][][]Key
	// Note says what the last key did, or why it couldn't, in the line's
	// place, while that's news: "" otherwise.
	Note string
}

// Key is a key the key line lists, or a card's keys row, and what it does, as
// "p" and "pin"; and where it says how something's set, its State, as b's
// "window", after "by". One Off can't act now, as p while the router doesn't
// answer: it's drawn faint. One Hidden toggles a section that's hidden: its
// key is drawn as any other's, and its word faint.
type Key struct {
	Key, Does, State string
	Off, Hidden      bool
}

// words are the key and what it does, as a key line lists them: the key in
// bold accent.key, its word muted, faint where it's hidden, and a colon and
// its state after it, the state in text.secondary; or all of it faint where
// it's off.
func (k Key) words() line {
	word := " " + k.Does
	if k.State != "" {
		word += ": "
	}
	switch {
	case k.Off:
		return line{{k.Key + word + k.State, faintInk}}
	case k.Hidden:
		return line{{k.Key, keyInk}, {word + k.State, faintInk}}
	case k.State == "":
		return line{{k.Key, keyInk}, {word, mutedInk}}
	}
	return line{{k.Key, keyInk}, {word, mutedInk}, {k.State, secondaryInk}}
}

// keyAt is a key as the key line lays it out: the key, and the column its
// words start at.
type keyAt struct {
	key Key
	x   int
}

// spaced is a key as the key line sets it, and the blank cells before it.
type spaced struct {
	key Key
	gap int
}

// keyLine draws the key line on its row, as keyLineRow has it, from column
// 0: its note, while it has one, else its keys, as keysAt lays them out.
func (f Frame) keyLine(c *canvas) {
	y, keys, ok := f.keyLineRow()
	if !ok {
		return
	}
	if note := f.KeyLine.Note; note != "" {
		c.line(0, y, line{{note, noteInk}}.fit(f.Width))
		return
	}
	for _, k := range keys {
		c.line(k.x, y, k.key.words())
	}
}

// keyLineRow is the row the key line is on, the frame's last, and the keys
// it shows there, as keysAt lays them out: none while a note takes their
// place. It reports false where that row isn't under the title rows, which
// leaves the line no row of its own.
func (f Frame) keyLineRow() (y int, keys []keyAt, ok bool) {
	y = f.Height - 1
	switch {
	case y < f.titleRows():
		return y, nil, false
	case f.KeyLine.Note != "":
		return y, nil, true
	}
	return y, f.keysAt(), true
}

// keysAt are the key line's keys as it lays them out from column 0, each
// where its words start, as many as fit whole in the frame's width: on a
// phone, or wherever the wide line doesn't fit whole even without its spare
// groups, as phone chooses them, else as fitting sets them apart.
func (f Frame) keysAt() []keyAt {
	keys := f.KeyLine.fitting(f.Width)
	if f.KeyLine.PhoneHelp != "" && (f.phone() || widthOf(keys) > f.Width) {
		keys = f.KeyLine.phone(f.Width)
	}
	var laid []keyAt
	x := 0
	for _, k := range keys {
		end := x + k.gap + k.key.words().width()
		if end > f.Width {
			break
		}
		laid = append(laid, keyAt{key: k.key, x: x + k.gap})
		x = end
	}
	return laid
}

// wide are the groups given in order, as the line sets its own: Lead cells
// after the first, as many as After has after each past it, and keysInGroup
// apart within a group.
func (l KeyLine) wide(groups [][]Key) []spaced {
	var keys []spaced
	gap := 0
	for i, group := range groups {
		if len(group) == 0 {
			continue
		}
		for _, k := range group {
			keys = append(keys, spaced{key: k, gap: gap})
			gap = keysInGroup
		}
		gap = cmp.Or(l.After, KeysAfter)
		if i == 0 {
			gap = l.Lead
		}
	}
	return keys
}

// fitting is the line as wide sets it, width cells wide: where it doesn't
// fit whole, the first of its Narrower forms that does, else the last.
func (l KeyLine) fitting(width int) []spaced {
	keys := l.wide(l.Groups)
	for _, groups := range l.Narrower {
		if widthOf(keys) <= width {
			break
		}
		keys = l.wide(groups)
	}
	return keys
}

// phone is the line as a phone width cells wide has it, keysApart apart:
// the first group, tab views, leading it; the page's Phone keys, then its
// PhoneKept; then ?, its word PhoneHelp, and q quit. Where that's wider than
// the phone, the first group goes first, then the page's Phone keys from the
// last; but where it keeps any, the Phone keys go first, then the first
// group; then the kept from the last, till it fits or only ? and q quit are
// left.
func (l KeyLine) phone(width int) []spaced {
	var lead []Key
	if len(l.Groups) > 0 {
		lead = l.Groups[0]
	}
	page, kept := l.Phone, l.PhoneKept
	ends := []Key{{Key: "?", Does: l.PhoneHelp}, Quit}
	for {
		keys := evenly(slices.Concat(lead, page, kept, ends))
		switch {
		case widthOf(keys) <= width:
			return keys
		case len(lead) > 0 && (len(kept) == 0 || len(page) == 0):
			lead = nil
		case len(page) > 0:
			page = page[:len(page)-1]
		case len(kept) > 0:
			kept = kept[:len(kept)-1]
		default:
			return keys
		}
	}
}

// evenly are the keys, keysApart apart.
func evenly(keys []Key) []spaced {
	laid := make([]spaced, len(keys))
	for i, k := range keys {
		laid[i] = spaced{key: k, gap: keysApart}
	}
	if len(laid) > 0 {
		laid[0].gap = 0
	}
	return laid
}

// widthOf is how many cells the keys take, set apart as they are.
func widthOf(keys []spaced) int {
	n := 0
	for _, k := range keys {
		n += k.gap + k.key.words().width()
	}
	return n
}

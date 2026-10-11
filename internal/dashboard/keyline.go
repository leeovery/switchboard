package dashboard

// The blank cells the key line sets its keys apart by: within a group, and
// after one.
const (
	keysInGroup = 2
	keysAfter   = 4
)

// KeyLine is the key line, the page's last row: the keys that work where
// they are, in their groups, or a note in their place.
type KeyLine struct {
	// Groups are the keys in their groups, in order: tab views first, then
	// the page's own, then p pin where a pin can be set, and ? help, q quit
	// last.
	Groups [][]Key
	// Lead is the blank cells after the first group, tab views: three, or
	// five on the Overview. The rest are keysInGroup apart within a group,
	// and keysAfter after one.
	Lead int
	// Phone are the keys a phone's line keeps between tab views, which leads
	// it while it fits, and ? and q quit, which end it, as the page words
	// them there; and PhoneHelp is ?'s word there: "keys", or on the
	// Overview, "keys and toggles".
	Phone     []Key
	PhoneHelp string
	// Note says what the last key did, or why it couldn't, in the line's
	// place, while that's news: "" otherwise.
	Note string
}

// Key is a key the key line lists, or a card's keys row, and what it does, as
// "p" and "pin". One Off can't act now, as p while the router doesn't
// answer: it's drawn faint.
type Key struct {
	Key, Does string
	Off       bool
}

// words are the key and what it does, as a key line lists them: the key in
// bold accent.key and its word muted, or both faint where it's off.
func (k Key) words() line {
	if k.Off {
		return line{{k.Key + " " + k.Does, faintInk}}
	}
	return line{{k.Key, keyInk}, {" " + k.Does, mutedInk}}
}

// keyAt is a key as the key line lays it out: the key, and the column its
// words start at.
type keyAt struct {
	key Key
	x   int
}

// keyLine draws the key line on the frame's last row, where that's under
// the title row: its note, while it has one, else its keys, as keysAt lays
// them out.
func (f Frame) keyLine(c *canvas) {
	y := f.Height - 1
	if y < f.titleRows() {
		return
	}
	if note := f.KeyLine.Note; note != "" {
		c.line(margin, y, line{{note, noteInk}}.fit(f.edge()-margin))
		return
	}
	for _, k := range f.keysAt() {
		c.line(k.x, y, k.key.words())
	}
}

// keysAt are the key line's keys as it lays them out from the margin, each
// where its words start: its groups in order, Lead cells after the first,
// keysInGroup apart within a group and keysAfter after one, as many as fit
// whole.
func (f Frame) keysAt() []keyAt {
	var laid []keyAt
	x, gap := margin, 0
	for i, group := range f.KeyLine.Groups {
		for j, k := range group {
			if x+gap+k.words().width() > f.edge() {
				return laid
			}
			x += gap
			laid = append(laid, keyAt{key: k, x: x})
			x += k.words().width()
			gap = keysInGroup
			if j == len(group)-1 {
				gap = keysAfter
			}
		}
		if i == 0 && len(group) > 0 {
			gap = f.KeyLine.Lead
		}
	}
	return laid
}

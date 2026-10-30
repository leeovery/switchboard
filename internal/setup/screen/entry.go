package screen

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/textfield"
)

// entry is a text field as the screen holds it: what's typed in it; the
// first glyph its box shows, as dashboard.Field.InView keeps it, so its view
// holds still while the cursor moves within it; and whether it's hidden, as
// a token's is, its view kept as its dots draw it.
type entry struct {
	text   textfield.Text
	from   int
	hidden bool
}

// entryOf is a field holding value, its cursor after the last of it, in
// view.
func entryOf(value string) entry {
	return entry{}.holding(textfield.Of(value))
}

// value is what's typed in the field.
func (e entry) value() string {
	return e.text.Value()
}

// field is the field as dashboard.Field draws it, typed in as typing says.
func (e entry) field(typing bool) dashboard.Field {
	return dashboard.Field{Value: e.text.Value(), Cursor: e.text.Cursor(), From: e.from, Typing: typing, Hidden: e.hidden}
}

// holding is the field holding text, its view moved as little as keeps its
// cursor in it.
func (e entry) holding(text textfield.Text) entry {
	e.text = text
	e.from = e.field(true).InView().From
	return e
}

// pressed is the field once key is pressed in it, as textfield.Text.Pressed
// has it, reporting false for a key it doesn't take.
func (e entry) pressed(key tea.KeyPressMsg) (entry, bool) {
	text, ok := e.text.Pressed(key)
	if !ok {
		return e, false
	}
	return e.holding(text), true
}

// pasted is the field with text pasted in at its cursor, as
// textfield.Text.Pasted has it.
func (e entry) pasted(text string) entry {
	return e.holding(e.text.Pasted(text))
}

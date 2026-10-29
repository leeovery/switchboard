package dashboard

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// SetupEditCard is an account's edit card, open over the Accounts page, as
// it stands: the account's id; its fields, Label and Reserve as typed, Plan
// the plan chosen, one of config.PlanChoices, and Primary whether yes is
// chosen; the field chosen; and where setup refused a value, why, as Why
// says, under the field Refused names.
type SetupEditCard struct {
	ID             string
	Label, Reserve Field
	Plan           string
	Primary        bool
	Chosen         setup.Field
	Refused        setup.Field
	Why            string
}

// SetupRemoveCard is the card asking before an account is removed, open over
// the Accounts page: the account's id, and why setup refused to remove it,
// where it did.
type SetupRemoveCard struct {
	ID, Refused string
}

// setupCardWidth is the natural width of setup's cards, frame 6's, their
// edges included.
const setupCardWidth = 80

// What the edit card says: its fields' names and their hints.
const (
	labelName   = "Label"
	planName    = "Plan"
	planHint    = "used by History to value usage"
	primaryName = "Primary"
	primaryHint = "the account the Claude apps are signed in to"
	reserveName = "Reserve"
	reserveHint = "usage kept back for the Claude apps"
)

// Primary's choices, and the edit card's keys and the remove card's.
var (
	primaryChoices = []string{"no", "yes"}
	editKeys       = []Key{{Key: "tab", Does: "next"}, {Key: "←→", Does: "select"}, {Key: "enter", Does: "save"}, {Key: "esc", Does: "cancel"}}
	removeKeys     = []Key{{Key: "enter", Does: "remove"}, {Key: "esc", Does: "keep it"}}
)

// What the remove card says of the account it would remove, after its id.
const (
	removeSays  = " is taken out of the config, and its token file deleted, so no requests are routed to it."
	removeKeeps = "Its Claude subscription is left as it is."
)

// setupCard is a card of setup's, as the dashboard's Card draws it, but over
// the page as it is, unfaded; and its fields, each drawn over the row of its
// body it's on.
type setupCard struct {
	card   Card
	fields []cardField
}

// cardField is a field of a card's, on the row of its body given.
type cardField struct {
	row   int
	field textField
}

// card is the card open over the page of the step showing: the Accounts
// page's edit card, or its remove card; nil where none is.
func (s Setup) card(width int) *setupCard {
	if s.At != SetupAccounts || s.Accounts == nil {
		return nil
	}
	cardWidth, _ := cardAcross(width, setupCardWidth)
	switch a := s.Accounts; {
	case a.Edit != nil:
		return a.Edit.card(cardWidth)
	case a.Remove != nil:
		return a.Remove.card(cardWidth)
	}
	return nil
}

// draw draws the card over what's drawn on c, as Card.draw draws it, then
// each of its fields its body shows, over its row.
func (k setupCard) draw(c *canvas) {
	k.card.draw(c)
	x, y, _, _ := k.card.box(c.width, c.height())
	shown := len(k.card.shown(c.height()))
	for _, f := range k.fields {
		if f.row < shown {
			f.field.draw(c, x+cardInset, y+2+f.row, k.card.width-2*cardInset)
		}
	}
}

// card is the edit card, width cells wide: EDIT and the account's id in its
// top edge; then a blank row apart, its fields, Label, Plan and its hint,
// Primary and its hint, and Reserve, its hint after it, each refusal under
// the field it's of; and its keys.
func (e SetupEditCard) card(width int) *setupCard {
	content := width - 2*cardInset
	k := &setupCard{card: Card{title: "EDIT " + strings.ToUpper(e.ID), width: width, keys: editKeys}}
	k.text(e.textField(setup.LabelField, labelName, "", e.Label), content)
	k.card.body = append(k.card.body, nil)
	plans, primary := config.PlanChoices(), 0
	if e.Primary {
		primary = 1
	}
	k.card.body = append(k.card.body,
		optionsRow(content, planName, planNames(plans), slices.Index(plans, e.Plan), e.Chosen == setup.PlanField),
		underChoice(content, planHint),
		nil,
		optionsRow(content, primaryName, primaryChoices, primary, e.Chosen == setup.PrimaryField),
		underChoice(content, primaryHint),
		nil,
	)
	k.text(e.textField(setup.ReserveField, reserveName, reserveHint, e.Reserve), content)
	return k
}

// textField is the card's text field of the field given, named and hinted as
// given, typed in where it's the one chosen, saying why setup refused its
// value, where it did.
func (e SetupEditCard) textField(of setup.Field, name, hint string, f Field) textField {
	field := textField{name: name, hint: hint, field: f}
	field.field.Typing = e.Chosen == of
	if e.Refused == of {
		field.wrong = e.Why
	}
	return field
}

// text puts f on the card's next row of its body, content cells wide, and
// the rows under it saying what's wrong with its value, where anything is.
func (k *setupCard) text(f textField, content int) {
	k.fields = append(k.fields, cardField{row: len(k.card.body), field: f})
	k.card.body = append(k.card.body, make([]line, 1+len(f.wrongLines(content)))...)
}

// card is the remove card, width cells wide: REMOVE and the account's id in
// its top edge; what removing it does, and what it leaves; why setup
// refused to remove it, where it did; and its keys.
func (r SetupRemoveCard) card(width int) *setupCard {
	k := &setupCard{card: Card{title: "REMOVE " + strings.ToUpper(r.ID), width: width, keys: removeKeys}}
	content := width - 2*cardInset
	says := line{{r.ID, titleInk}, {removeSays, secondaryInk}}
	k.card.body = append(says.wrap(content), line{{removeKeeps, mutedInk}})
	if r.Refused != "" {
		k.card.body = append(append(k.card.body, nil), line{{r.Refused, errorInk}}.wrap(content)...)
	}
	return k
}

// choiceMarkInk is the ink of the mark before each of a choice's options.
var choiceMarkInk = ink{token: theme.AccentKey}

// optionsRow is a form's row of a choice, width cells wide: its name, in
// text.subtle, then from its value's column, laid out as a text field's,
// its options, four cells apart, ■ before the one chosen, in text.primary,
// bold where the choice is the field chosen, and □ before the rest, muted,
// each mark in accent.key.
func optionsRow(width int, name string, options []string, chosen int, active bool) line {
	l := line{{name, dimInk}}.fit(fieldParts(width)[fieldName].Size)
	l = append(l, spaces(valueAt(width)-l.width()))
	for i, option := range options {
		if i > 0 {
			l = append(l, spaces(choicesApart))
		}
		mark, k := "□ ", mutedInk
		if i == chosen {
			mark, k = "■ ", ink{token: theme.TextPrimary, bold: active}
		}
		l = append(l, span{mark, choiceMarkInk}, span{option, k})
	}
	return l
}

// choicesApart is the cells between a choice's options.
const choicesApart = 4

// underChoice is a choice's hint, on the row under it, width cells wide,
// from its value's column, faint.
func underChoice(width int, hint string) line {
	return line{spaces(valueAt(width)), {hint, faintInk}}
}

// planNames are the plans as an edit offers them: each by its name, as
// setup.PlanName has it, or for none, not set.
func planNames(plans []string) []string {
	names := make([]string, len(plans))
	for i, p := range plans {
		names[i] = setup.PlanName(p)
		if names[i] == "" {
			names[i] = notSet
		}
	}
	return names
}

package dashboard

import (
	"strings"
	"time"
)

// SetupPasteCard is the card pasting an account's token, open over the
// Accounts page in the token card's place, as it stands: the account's id;
// the token as it's pasted, which the card draws hidden, whatever it's
// given; whether it's being checked and saved, when it takes no key; and
// why setup refused it, where it did.
type SetupPasteCard struct {
	ID       string
	Token    Field
	Checking bool
	Why      string
}

// What the paste card says of its own: what it asks for; what setup does
// with it; its field's name and hint; and how a token copied off setup's
// screen is taken.
const (
	pasteAsks   = "Paste the token you made with claude setup-token"
	pasteSays   = "Setup checks it with the API, then saves it, a year from today."
	pasteName   = "Token"
	pasteHint   = "shown as dots"
	pasteJoined = "A token copied across lines is joined up, the card's edges left out."
)

// pasteKeys are the paste card's keys.
var pasteKeys = []Key{{Key: "enter", Does: "save"}, cancelKey}

// card is the paste card at now, width cells wide, laid out as the token
// card's frame 2: PASTE A TOKEN and the account's id in its top edge; what
// it asks for, bold text.primary, from tokenSaysAt, or while it's checked,
// the spinner before it and that it's checked; what setup does with it, in
// text.secondary; a row on, its field, from tokenSaysAt, typed in till it's
// checked, drawn hidden, why setup refused it under it; a row on, how a
// token copied across lines is taken, muted; and its keys, none while it's
// checked.
func (p SetupPasteCard) card(width int, now time.Time) *setupCard {
	content := width - 2*cardInset
	b := tokenBody{width: content}
	if p.Checking {
		b.underWay(now, tokenChecking)
	} else {
		b.says(line{{pasteAsks, titleInk}})
	}
	b.says(line{{pasteSays, secondaryInk}}).gap()
	k := &setupCard{card: Card{title: "PASTE A TOKEN · " + strings.ToUpper(p.ID), width: width, body: b.lines}}
	token := p.Token
	token.Typing, token.Hidden = !p.Checking, true
	k.text(textField{name: pasteName, hint: pasteHint, wrong: p.Why, field: token}, tokenSaysAt, content)
	b.lines = k.card.body
	k.card.body = b.gap().says(line{{pasteJoined, mutedInk}}).lines
	if !p.Checking {
		k.card.keys = pasteKeys
	}
	return k
}

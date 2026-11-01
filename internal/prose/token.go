package prose

import (
	"strings"
	"time"
)

// TokenEnding is a token's year ending, as Accounts and the notifications
// tell of it: Until, when it's up, told by its date in Now's time zone; Up,
// set once it is; and DaysLeft, the days left of it at Now, 0 on its last
// day, as status.TokenYear.DaysLeft counts them, where it isn't.
type TokenEnding struct {
	Now, Until time.Time
	Up         bool
	DaysLeft   int
}

// TokenPart is a part of what's told of a token's year ending: its Text, and
// what it is, as its Kind has it.
type TokenPart struct {
	Text string
	Kind TokenPartKind
}

// TokenPartKind is what a part of what's told of a token's year ending is.
type TokenPartKind int

const (
	// TokenWords are plain words.
	TokenWords TokenPartKind = iota
	// TokenWarning is what warns: the days left, or that the year's up.
	TokenWarning
	// TokenCommand is the command that makes a new token.
	TokenCommand
)

// newToken is what Told says makes a new token, after the command.
const newToken = " makes a new one"

// tokenCommand is the command that makes a new token.
const tokenCommand = "switchboard setup"

// News is the year's ending, as a notification's news tells it: "token
// expires 17 Nov 2026 · 21 days left", "· last day" on its last day, or
// "token expired 17 Nov 2026" once it's up.
func (e TokenEnding) News() []TokenPart {
	if e.Up {
		return []TokenPart{{"token ", TokenWords}, {expiresVerb(true), TokenWarning}, {" " + DayMonthYear(e.Until.In(e.Now.Location())), TokenWords}}
	}
	return []TokenPart{{"token " + Expires(e.Now, e.Until, false) + " · ", TokenWords}, {DaysLeft(e.DaysLeft), TokenWarning}}
}

// Told is the year's ending told whole, as News tells it, and that
// switchboard setup makes a new token: "token expires 17 Nov 2026 · 21 days
// left — switchboard setup makes a new one".
func (e TokenEnding) Told() []TokenPart {
	return append(e.News(), TokenPart{" — ", TokenWords}, TokenPart{tokenCommand, TokenCommand}, TokenPart{newToken, TokenWords})
}

// Short is the year's ending told short, without its date: "token: 21 days
// left", "token: last day", or "token expired".
func (e TokenEnding) Short() []TokenPart {
	if e.Up {
		return []TokenPart{{"token ", TokenWords}, {expiresVerb(true), TokenWarning}}
	}
	return []TokenPart{{"token: ", TokenWords}, {DaysLeft(e.DaysLeft), TokenWarning}}
}

// TokenSaid is the parts' text, one after another.
func TokenSaid(parts []TokenPart) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

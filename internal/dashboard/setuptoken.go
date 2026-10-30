package dashboard

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// SetupTokenCard is the token card, open over the Accounts page, making an
// account's token, as it stands: the account's id; where the making has got;
// and, waiting, how long it's waited, how the sign-in address opened, as
// setup words it, whether in a private window, and whether it didn't open,
// when Address is it, as it's given, to paste; or, ended, how it ended,
// nothing saved, as setup words it.
type SetupTokenCard struct {
	ID       string
	Stage    TokenStage
	Waited   time.Duration
	Opening  string
	Private  bool
	Unopened bool
	Address  string
	Unsaved  setup.Unsaved
}

// TokenStage is where a token's making has got, as the token card tells it.
type TokenStage int

const (
	// TokenStarting is claude setup-token starting, its sign-in address not
	// yet ready.
	TokenStarting TokenStage = iota
	// TokenWaiting is it waiting for the sign-in's approval in the browser.
	TokenWaiting
	// TokenStopping is it being stopped, as esc asked, before it said it
	// made the token.
	TokenStopping
	// TokenChecking is the token made, being checked with the API and saved,
	// however setup's left.
	TokenChecking
	// TokenUnsaved is the making ended, nothing saved, as Unsaved says.
	TokenUnsaved
)

// spins reports whether the card's stage has its spinner moving: every one
// but the making's end.
func (t TokenStage) spins() bool {
	return t != TokenUnsaved
}

// tokenSaysAt is how many cells into the token card's content what it says
// starts, its mark in the cells before.
const tokenSaysAt = 5

// What the token card says, at each stage, of its own.
const (
	tokenStarting     = "Starting claude setup-token"
	tokenStartingSays = "Setup opens its sign-in page in the browser once it's ready."
	tokenWaiting      = "Waiting for you to approve it in the browser"
	tokenPrivate      = "Close any other private window first: they all share one sign-in."
	tokenSaves        = "Setup saves the token automatically. There's nothing to copy."
	tokenStopping     = "Stopping claude setup-token"
	tokenStoppingSays = "Nothing is saved, unless it had already created the token."
	tokenChecking     = "Checking the token and saving it"
	tokenCheckingSays = "claude setup-token created it. Setup checks it with the API, then saves it, a year from today."
	tokenKept         = "It's saved however you leave setup now, as it lasts a year on the account."
)

// The token card's keys.
var (
	reopenKey   = Key{Key: "o", Does: "reopen window"}
	copyAddress = Key{Key: "c", Does: "copy the address"}
	pasteKey    = Key{Key: "p", Does: "paste a token"}
	cancelKey   = Key{Key: "esc", Does: "cancel"}
	tryAgainKey = Key{Key: "enter", Does: "try again"}
)

// card is the token card at now, width cells wide: NEW TOKEN and the
// account's id in its top edge; then where the making has got, from
// tokenSaysAt, its mark before it, as body has it; and its keys.
func (t SetupTokenCard) card(width int, now time.Time) *setupCard {
	content := width - 2*cardInset
	return &setupCard{card: Card{title: "NEW TOKEN · " + strings.ToUpper(t.ID), width: width, body: t.body(now, content), keys: t.keys()}}
}

// body is what the card says at now, content cells wide, as its stage has
// it: under way, its spinner and what's under way, then what it says of
// it; ended, its mark and how it ended, laid out as frame 10's.
func (t SetupTokenCard) body(now time.Time, content int) []line {
	b := tokenBody{width: content}
	switch t.Stage {
	case TokenStarting:
		b.underWay(now, tokenStarting).gap().says(line{{tokenStartingSays, mutedInk}})
	case TokenWaiting:
		t.waiting(&b, now)
	case TokenStopping:
		b.underWay(now, tokenStopping).gap().says(line{{tokenStoppingSays, mutedInk}})
	case TokenChecking:
		b.underWay(now, tokenChecking).gap().says(line{{tokenCheckingSays, secondaryInk}}).gap().says(line{{tokenKept, mutedInk}})
	case TokenUnsaved:
		t.unsaved(&b)
	}
	return b.lines
}

// waiting is what the card says while setup-token waits for the sign-in's
// approval, at now, as frame 2 has it: its spinner, the wait, and at its
// right how long it's waited, faint; then how the address opened, where it
// says, in text.secondary; for a private window, that its others share its
// sign-in, in accent.attention; the address to paste, where it didn't open,
// made safe to show, in accent.mode, broken where it's wider than the card,
// never cut, as it's to be copied whole; and that setup saves the token
// itself, muted.
func (t SetupTokenCard) waiting(b *tokenBody, now time.Time) {
	b.lines = append(b.lines, spread(waiting(now, tokenWaiting), line{{prose.Stopwatch(t.Waited), faintInk}}, b.width))
	b.gap()
	if t.Opening != "" {
		b.says(line{{t.Opening, secondaryInk}}).gap()
	}
	if t.Private {
		b.says(line{{tokenPrivate, warningInk}}).gap()
	}
	if t.Unopened {
		for _, l := range wholeLines(prose.Visible(t.Address), b.width-tokenSaysAt) {
			b.said(l)
		}
		b.gap()
	}
	b.says(line{{tokenSaves, mutedInk}})
}

// unsaved is what the card says of a making that saved nothing, as frame 10
// has it: its mark, bold, and what came of it, bold text.primary; under it
// what that means, in text.secondary; a row on, what setup-token showed, each
// line after │ in text.faint, muted, broken where it's wider than the card;
// and a row on, what to do now, muted.
func (t SetupTokenCard) unsaved(b *tokenBody) {
	u := t.Unsaved
	b.lines = append(b.lines, line{markOf(u.Mark), spaces(tokenSaysAt - 1), {u.Headline, titleInk}}.fit(b.width))
	b.says(line{{u.Says, secondaryInk}})
	if len(u.Shown) > 0 {
		b.gap()
	}
	for _, shown := range u.Shown {
		for part := range strings.SplitSeq(ansi.Wrap(shown, b.width-tokenSaysAt-ansi.StringWidth(shownBar), ""), "\n") {
			b.said(line{{shownBar, faintInk}, {part, mutedInk}})
		}
	}
	b.gap().says(line{{u.Then, mutedInk}})
}

// shownBar leads each line of what setup-token showed.
const shownBar = "│ "

// keys are the card's keys at its stage: starting, pasting a token and
// cancelling; waiting, reopening the window, or where the address didn't
// open, copying it, then the same; ended, trying again, where it does, then
// the same; and none while it's being stopped or the token's checked, as
// neither waits on a key.
func (t SetupTokenCard) keys() []Key {
	switch t.Stage {
	case TokenStarting:
		return []Key{pasteKey, cancelKey}
	case TokenWaiting:
		if t.Unopened {
			return []Key{copyAddress, pasteKey, cancelKey}
		}
		return []Key{reopenKey, pasteKey, cancelKey}
	case TokenUnsaved:
		if t.Unsaved.Again {
			return []Key{tryAgainKey, pasteKey, cancelKey}
		}
		return []Key{pasteKey, cancelKey}
	}
	return nil
}

// tokenBody is the token card's body as it's laid out, width cells wide.
type tokenBody struct {
	width int
	lines []line
}

// underWay lays out what's under way at now: the spinner, then the words.
func (b *tokenBody) underWay(now time.Time, words string) *tokenBody {
	b.lines = append(b.lines, waiting(now, words).fit(b.width))
	return b
}

// gap lays out a blank row.
func (b *tokenBody) gap() *tokenBody {
	b.lines = append(b.lines, nil)
	return b
}

// says lays out l from tokenSaysAt, wrapped onto as many rows as it takes.
func (b *tokenBody) says(l line) *tokenBody {
	for _, part := range l.wrap(b.width - tokenSaysAt) {
		b.said(part)
	}
	return b
}

// said lays out l, a row of it, from tokenSaysAt.
func (b *tokenBody) said(l line) {
	b.lines = append(b.lines, append(line{spaces(tokenSaysAt)}, l...))
}

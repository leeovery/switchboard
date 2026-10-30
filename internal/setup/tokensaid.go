package setup

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setuptoken"
)

// Opening says how the sign-in address opened, at AddressReady, as the token
// card says it under its wait: in a private window, as the browser is
// signed in to another account, to sign in there; not opened, to open the
// address shown, signed in to the account; "" where it opened in the browser
// as it is, as there's nothing more to say.
func (m Making) Opening() string {
	switch {
	case m.Step != AddressReady || m.Opened == setuptoken.OpenedInBrowser:
		return ""
	case m.Opened == setuptoken.OpenedPrivately:
		return "Your browser is signed in to " + m.SignedIn + ", so setup opened " + privateWindow(m.Browser) + ". Sign in to " + m.For + " there, then approve."
	case m.SignedIn == "":
		return "Setup couldn't open your browser. Open this address in it, signed in to " + m.For + ", then approve:"
	}
	return "Your browser is signed in to " + m.SignedIn + ", and setup couldn't open " + privateWindow(m.Browser) + ". Open this address in one, sign in to " + m.For + " there, then approve:"
}

// privateWindow names a private window of the browser, "a private Firefox
// window", or of none named, "a private window".
func privateWindow(browser string) string {
	if browser == "" {
		return "a private window"
	}
	return "a private " + browser + " window"
}

// Unsaved is how the token card tells a token's making that saved nothing,
// laid out as frame 10's: its mark, and what came of it; what that means;
// what setup-token showed, a line each, where the card shows any; what to do
// now; and whether enter tries again.
type Unsaved struct {
	Mark     Mark
	Headline string
	Says     string
	Shown    []string
	Then     string
	Again    bool
}

// shownLast is how many of the last lines setup-token showed the token card
// shows, where it made no token: those that say why.
const shownLast = 4

// thens are what to do once a making has saved nothing: try again, where
// nothing's made, and that try signed in to the other subscription, or once
// Claude Code's installed; and where a token's made, which lasts a year on
// the account, paste it, once it's copied, and pasteOnceRight, where what
// kept it from being saved is to be put right first.
type thens struct {
	tryAgain, signedIn, installed string
	paste, pasteOnceRight         string
}

// The token card tries again with enter and pastes with p; once setup's
// screen has gone, there's neither until setup is run again.
var (
	onTheCard = thens{
		tryAgain:       "Try again, or paste a token you made with claude setup-token.",
		signedIn:       "Try again, signed in to the other subscription, or paste a token you made with claude setup-token.",
		installed:      "Install Claude Code, then try again, or paste a token you made with claude setup-token.",
		paste:          "Then press p to paste it.",
		pasteOnceRight: "It lasts a year on the account, so copy it, then press p to paste it once that's put right.",
	}
	onceGone = thens{
		tryAgain:       "Run switchboard setup again to try again, or to paste a token you made with claude setup-token.",
		signedIn:       "Run switchboard setup again to try again, signed in to the other subscription, or to paste a token you made with claude setup-token.",
		installed:      "Install Claude Code, then run switchboard setup again to try again, or to paste a token you made with claude setup-token.",
		paste:          "Then run switchboard setup again, and press p on its token card to paste it.",
		pasteOnceRight: "It lasts a year on the account, so copy it, then once that's put right, run switchboard setup again, and press p on its token card to paste it.",
	}
)

// Unsaved is how the token card tells the making's end, where it saved
// nothing, as Result says, each in words of its own: setup-token stopping
// with an error, what it showed last; the API refusing the token; the token
// another account's subscription's, which it names; no Claude Code to run
// setup-token with; the token made, but unread, all setup-token showed, the
// token in it, to copy and paste, as it lasts a year on the account, so
// enter makes no other; and the token made, but not saved, why, and the
// token, to copy and paste, so enter makes no other, or, where setup never
// read it, nothing to copy, enter trying again. Any other end is the zero
// Unsaved.
func (t Tokened) Unsaved() Unsaved {
	return t.unsaved(onTheCard)
}

// Printed is the making's end, where it saved nothing, as Unsaved tells it,
// made safe to show, a line each, for setup's caller to print once its
// screen has gone: what came of it; what that means; what setup-token
// showed, each line set in by two spaces, the token in it to copy; and what
// to do now, which is to run setup again, as there's no card to try again
// or paste on. Any other end prints nothing.
func (t Tokened) Printed() []string {
	u := t.unsaved(onceGone).Clean()
	lines := make([]string, 0, len(u.Shown)+3)
	for _, line := range slices.Concat([]string{u.Headline, u.Says}, indented(u.Shown), []string{u.Then}) {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// indented are the lines, each set in by two spaces.
func indented(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = "  " + line
	}
	return out
}

// Clean is u made safe to show: what setup-token showed keeping its
// spacing, as the token in it is to be copied.
func (u Unsaved) Clean() Unsaved {
	u.Headline, u.Says, u.Then = prose.Clean(u.Headline), prose.Clean(u.Says), prose.Clean(u.Then)
	u.Shown = slices.Clone(u.Shown)
	for i, line := range u.Shown {
		u.Shown[i] = prose.Visible(line)
	}
	return u
}

// unsaved is Unsaved, saying what to do now as then has it.
func (t Tokened) unsaved(then thens) Unsaved {
	failed := Unsaved{Mark: Failed, Then: then.tryAgain, Again: true}
	switch t.Result {
	case TokenNotMade:
		failed.Headline, failed.Says = "The token wasn't created", "claude setup-token stopped with an error. What it showed:"
		failed.Shown = trimmed(t.Shown[max(len(t.Shown)-shownLast, 0):])
	case TokenRefused:
		failed.Headline, failed.Says = "The token was refused", "The API refused the token, so it isn't saved."
	case TokenSameAccount:
		failed.Headline = "The token is for " + t.SameAs + "'s subscription"
		failed.Says = sameSubscription(t.SameAs) + "."
		failed.Then = then.signedIn
	case NoClaudeCode:
		failed.Headline, failed.Says, failed.Then = "There's no Claude Code to create the token with", t.Why, then.installed
	case TokenUnread:
		return Unsaved{
			Mark: Warning, Headline: "The token was created, but setup couldn't read it",
			Says:  "It lasts a year on the account, so copy it from what claude setup-token showed:",
			Shown: trimmed(t.Shown), Then: then.paste,
		}
	case TokenNotSaved:
		kept := Unsaved{Mark: Warning, Headline: "The token was created, but setup couldn't save it", Says: t.Why, Shown: t.Shown, Then: then.pasteOnceRight}
		if len(t.Shown) == 0 {
			kept.Then, kept.Again = then.tryAgain, true
		}
		return kept
	default:
		return Unsaved{}
	}
	return failed
}

// sameSubscription says the account named has the token's subscription
// already, so it isn't saved.
func sameSubscription(id string) string {
	return id + " has that subscription already, so the token isn't saved"
}

// Refusal is why the paste card refuses the token pasted, said under its
// field, where its paste saved nothing, as Result says: what's pasted no
// token, the API refusing it, or not answering for it, as PasteToken words
// each; and another account's subscription, which it names. Any other end is
// no refusal: "".
func (t Tokened) Refusal() string {
	switch t.Result {
	case TokenNotGiven, TokenRefused, TokenUnanswered:
		return t.Why
	case TokenSameAccount:
		return sameSubscription(t.SameAs)
	}
	return ""
}

// trimmed are the lines without the blanks round each, as setup-token sets
// its text in from its screen's edge.
func trimmed(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(l)
	}
	return out
}

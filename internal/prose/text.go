package prose

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Separator sets apart the parts of a line that says several things, such as
// where the usage came from: wider than the dot within an account's title, so
// the title reads as one part.
const Separator = "  ·  "

// HistoryMoving says the history waits on its move onto the accounts' ids,
// which the router makes once, as it first starts on a version that makes
// them: as the status, the dashboard and the verbs that read the history say
// it, each followed by what holds it up, as the Move constants and
// MoveFailed say it.
const HistoryMoving = "the history waits on its move onto the accounts' new ids"

// What holds the move onto the accounts' ids up, said after HistoryMoving,
// but for the API, as MoveWaitsAPI says, and a failure, as MoveFailed says:
// the router making it; the router to restart onto the new ids its config
// holds; no router answering; the router not saying how it stands; and the
// router running being of a version from before ids were made.
const (
	MoveRunning      = "the router is making it"
	MoveWaitsRestart = "the router makes the rest once it restarts onto the new ids its config holds"
	MoveWaitsRouter  = "no router answers to make it: start it with switchboard service install (or switchboard serve)"
	MoveWaitsUntold  = "the router didn't say how it stands in time, as when it's busy or restarting"
	MoveWaitsUpgrade = "the router running is of a version from before the ids, and makes it once it restarts on this one"
)

// MoveWaitsAPI says the move onto the accounts' ids waits for the API to name
// the organization of the token of the account given, by the id it had,
// which makes its id, why saying why it hasn't: of the accounts' tokens
// alike, where none's given.
func MoveWaitsAPI(account, why string) string {
	if account == "" {
		return "the API hasn't named the organizations of the accounts' tokens, which make their ids, so the router asks again a while later"
	}
	return "the API hasn't named the organization of " + Clean(account) + "'s token, which makes its id, so the router asks again a while later: " + Clean(why)
}

// MoveFailed says the move onto the accounts' ids failed, why says why, and
// the router tries it again.
func MoveFailed(why string) string {
	return "the router couldn't make it, and tries again in a minute: " + Clean(why)
}

// An account's token said to be another subscription's, as the router finds
// it probing the account, which takes no requests while it holds that token:
// ForeignToken in full, "its token is another subscription's";
// ForeignTokenAfterName after the account's name, as in "work's token is
// another subscription's"; and ForeignTokenShort where a few cells hold it,
// "wrong token", no wider than the 11 cells Runway's label column keeps for
// a short word at its narrowest, which never widens for one.
const (
	ForeignToken          = "its " + foreignToken
	ForeignTokenAfterName = "'s " + foreignToken
	ForeignTokenShort     = "wrong token"
)

// foreignToken is what ForeignToken and ForeignTokenAfterName say after whose
// token it is.
const foreignToken = "token is another subscription's"

// Tokenless says, short, why an account takes no requests for want of a token
// of its own: "no token", or, where the token it holds is another
// subscription's, as foreign says, ForeignTokenShort's "wrong token".
func Tokenless(foreign bool) string {
	if foreign {
		return ForeignTokenShort
	}
	return "no token"
}

// AddAsOwn is the other way to put right an account whose token is another
// subscription's, said after giving it its own: that subscription added as
// an account of its own.
const AddAsOwn = "or add that subscription as an account of its own"

// AccountsToken is the command that gives the account with the given id its
// token, as in "switchboard accounts token work".
func AccountsToken(id string) string {
	return "switchboard accounts token " + Clean(id)
}

// ForeignTokenFix says the token of the account with the given id is another
// subscription's, as ForeignToken says, and what puts it right: "its token is
// another subscription's: give it its own with switchboard accounts token
// work, or add that subscription as an account of its own".
func ForeignTokenFix(id string) string {
	return ForeignToken + ": give it its own with " + AccountsToken(id) + ", " + AddAsOwn
}

// Version words a release's version as it's shown, such as v0.2.10: a
// version that isn't a release's number, such as a build's "dev", as it is.
func Version(version string) string {
	if version == "" || version[0] < '0' || version[0] > '9' {
		return version
	}
	return "v" + version
}

// Clean makes text safe to show on one line of a terminal: control
// characters, which would move the cursor or restyle what follows, become
// spaces, and each run of spaces becomes one.
func Clean(text string) string {
	return strings.Join(strings.Fields(Visible(text)), " ")
}

// Visible is text made safe to show on one line of a terminal, each control
// character a space, as Clean makes it, but with its spacing kept, so text
// to copy, such as a line for a startup file, shows as it is copied.
func Visible(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
}

// Quoted shows text the user gave, such as a label, in double quotes, set
// apart from the words around it: as Clean makes it, but for its quote marks
// and backslashes, escaped by a backslash, so where it ends reads plainly,
// and each character drawn as nothing, as Unseen has it, that does nothing
// that shows, as MapUnseen has it, shown as its escape, as \u200B, so what it
// holds shows. A bidirectional control is among them, so it can't reorder
// the rest of the line, and so is a direction mark, which a label keeps but
// whose part in it can't be seen. A private-use character shows as it is,
// and so does what makes an emoji, a keycap, a variant or a flag, or shapes
// a joining script, unless an escape stands before it, as an escape owns
// nothing after it.
func Quoted(text string) string {
	return `"` + MapUnseen(quoteEscaper.Replace(Clean(text)), escaped) + `"`
}

// quoteEscaper escapes the quote marks and backslashes in text Quoted shows.
var quoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// The words that join a list's last item to the rest: And, as List runs
// items together, "a, b and c"; and Or, as ListOr runs together what's
// chosen between, "a, b or c".
const (
	And = " and "
	Or  = " or "
)

// Joiner is what goes before the item at i of n run together as a list, its
// last joined by last, as And or Or: nothing before the first, last before
// the last, and ", " before any other.
func Joiner(i, n int, last string) string {
	switch i {
	case 0:
		return ""
	case n - 1:
		return last
	}
	return ", "
}

// List runs items together as a list: "a", "a and b", "a, b and c", or ""
// for none.
func List(items []string) string {
	return listed(items, And)
}

// ListOr runs items together as a list of what's chosen between: "a", "a or
// b", "a, b or c", or "" for none.
func ListOr(items []string) string {
	return listed(items, Or)
}

// listed runs items together as a list, each after its Joiner, the last
// joined by last.
func listed(items []string, last string) string {
	var b strings.Builder
	for i, item := range items {
		b.WriteString(Joiner(i, len(items), last))
		b.WriteString(item)
	}
	return b.String()
}

// ListOf names the things a noun names, each of a kind its name gives, as
// List runs the names together: the noun alone for none, as "limit"; "5-hour
// limit" for one; and "5-hour and week limits" for more, a noun whose plural
// takes an s.
func ListOf(names []string, noun string) string {
	switch len(names) {
	case 0:
		return noun
	case 1:
		return names[0] + " " + noun
	}
	return List(names) + " " + noun + "s"
}

// Truncate keeps the first n characters of s.
func Truncate(s string, n int) string {
	if runes := []rune(s); len(runes) > n {
		return string(runes[:n])
	}
	return s
}

// TruncateBytes keeps as much of s as n bytes hold, ending at the end of a
// character.
func TruncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Ellipsis stands for what's cut, as TruncateBytesFront, Cut, CutShort and
// CutFront cut it, or what isn't shown.
const Ellipsis = "…"

// Cut cuts text to the cells given, as a terminal draws it, ending it with an
// ellipsis where it's cut, as CutShort cuts it: text as it is where it fits,
// and "" where no cell is given.
func Cut(text string, cells int) string {
	if ansi.StringWidth(text) <= cells {
		return text
	}
	return CutShort(text, cells)
}

// CutShort cuts text short to the cells given, as a terminal draws it, ending
// it with an ellipsis, the spaces before that let go, whether or not it fits,
// as where what follows it won't: "" where no cell is given.
func CutShort(text string, cells int) string {
	if cells < 1 {
		return ""
	}
	return strings.TrimRight(ansi.Truncate(text, cells-ansi.StringWidth(Ellipsis), ""), " ") + Ellipsis
}

// CutFront cuts text to the cells given, as a terminal draws it, from its
// start, keeping its end, an ellipsis ahead of it where it's cut: text as it
// is where it fits, and "" where no cell is given.
func CutFront(text string, cells int) string {
	switch width := ansi.StringWidth(text); {
	case width <= cells:
		return text
	case cells < 1:
		return ""
	default:
		return Ellipsis + ansi.TruncateLeft(text, width-(cells-ansi.StringWidth(Ellipsis)), "")
	}
}

// TruncateBytesFront keeps as much of the end of s as n bytes hold with an
// ellipsis ahead of it, standing for what's cut, starting at the start of a
// character: s as it is where it fits, and "" where the ellipsis doesn't.
func TruncateBytesFront(s string, n int) string {
	switch {
	case len(s) <= n:
		return s
	case n < len(Ellipsis):
		return ""
	}
	start := len(s) - (n - len(Ellipsis))
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return Ellipsis + s[start:]
}

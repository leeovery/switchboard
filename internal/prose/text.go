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

// Clean makes text safe to show on one line of a terminal: control
// characters, which would move the cursor or restyle what follows, become
// spaces, and each run of spaces becomes one.
func Clean(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

// Quoted shows text the user gave, such as a label, in double quotes, set
// apart from the words around it: as Clean makes it, but for its quote marks
// and backslashes, escaped by a backslash, so where it ends reads plainly,
// and each character drawn as nothing, as Unseen has it, that plays no part
// in a sequence, shown as its escape, as \u200B, so what it holds shows. A
// bidirectional control is among them, so it can't reorder the rest of the
// line. A joiner within an emoji's sequence, a variation selector, a flag's
// tags and a private-use character show as they are.
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

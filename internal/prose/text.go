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

// List runs items together as a list: "a", "a and b", "a, b and c", or ""
// for none.
func List(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	last := len(items) - 1
	return strings.Join(items[:last], ", ") + " and " + items[last]
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

// ellipsis stands for what TruncateBytesFront, Cut and CutFront cut.
const ellipsis = "…"

// Cut cuts text to the cells given, as a terminal draws it, ending it with an
// ellipsis where it's cut, the spaces before that let go: text as it is where
// it fits, and "" where no cell is given.
func Cut(text string, cells int) string {
	switch {
	case ansi.StringWidth(text) <= cells:
		return text
	case cells < 1:
		return ""
	}
	return strings.TrimRight(ansi.Truncate(text, cells-ansi.StringWidth(ellipsis), ""), " ") + ellipsis
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
		return ellipsis + ansi.TruncateLeft(text, width-(cells-ansi.StringWidth(ellipsis)), "")
	}
}

// TruncateBytesFront keeps as much of the end of s as n bytes hold with an
// ellipsis ahead of it, standing for what's cut, starting at the start of a
// character: s as it is where it fits, and "" where the ellipsis doesn't.
func TruncateBytesFront(s string, n int) string {
	switch {
	case len(s) <= n:
		return s
	case n < len(ellipsis):
		return ""
	}
	start := len(s) - (n - len(ellipsis))
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return ellipsis + s[start:]
}

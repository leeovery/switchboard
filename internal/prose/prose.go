// Package prose words what switchboard tells people the same way wherever it
// tells it: a list run together as English runs one, a count as briefly as a
// row has room for, or in full, its thousands set apart, and text cut short.
package prose

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Tokens counts n tokens as briefly as a row has room for, as the Log counts
// them: as they are under a thousand; in thousands to a tenth under ten
// thousand, as "5.4k", and whole under a million, as "958k"; then in millions
// to a tenth, as "5.7M" or "16.1M". Each form is for what rounds to it, so
// 9,960 is "10k", and 999,500 is "1.0M", a half rounded up.
func Tokens(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case rounded(n, 100) < 100:
		return tenths(rounded(n, 100), "k")
	case rounded(n, 1000) < 1000:
		return strconv.Itoa(rounded(n, 1000)) + "k"
	}
	return tenths(rounded(n, 100_000), "M")
}

// Characters counts n characters as the Log counts an answer's so far: in
// thousands to a tenth at every size, as "0.8k", "5.1k", "14.2k" or "958.3k",
// a half rounded up, as Tokens rounds its tenths.
func Characters(n int) string {
	return tenths(rounded(n, 100), "k")
}

// rounded is how many of unit n is, to the nearest, a half rounded up.
func rounded(n, unit int) int {
	return (n + unit/2) / unit
}

// tenths writes a count of tenths of a unit, to a tenth, as "5.4k".
func tenths(n int, unit string) string {
	return strconv.Itoa(n/10) + "." + strconv.Itoa(n%10) + unit
}

// Times says how many times something happened, as a sentence does: "once",
// "twice", then "3 times".
func Times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return fmt.Sprint(n) + " times"
}

// numberWords are the numbers Number words, from none to ten.
var numberWords = []string{"none", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}

// Number words n as a sentence counts things: a word to ten, as "three" or
// "none", then figures, as "11".
func Number(n int) string {
	if n >= 0 && n < len(numberWords) {
		return numberWords[n]
	}
	return fmt.Sprint(n)
}

// Counted counts n of the thing a noun names, one or many, as "1 week" or
// "1,240 requests", its thousands set apart as Grouped sets them: a noun
// whose plural takes an s, as every one counted does.
func Counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return Grouped(n) + " " + noun + "s"
}

// Grouped writes n with its thousands set apart by commas, as "1,240".
func Grouped(n int) string {
	digits := strconv.Itoa(n)
	sign := ""
	if n < 0 {
		sign, digits = "-", digits[1:]
	}
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return sign + b.String()
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

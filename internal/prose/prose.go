// Package prose words what switchboard tells people the same way wherever it
// tells it: a list run together as English runs one, a count as briefly as a
// row has room for, and text cut short.
package prose

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// countUnits are the units Count counts in from a thousand, each a thousand
// of the one before.
var countUnits = []string{"k", "M", "B"}

// Count counts n, as of tokens, as briefly as a row has room for: as it is
// under a thousand, then in thousands, millions or billions, the least it
// rounds to under a thousand of, or billions past them, to a tenth under ten,
// as "1.2k" or "9.9M", or whole, as "34k" or "120M".
func Count(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	of, unit := float64(n)/1000, 0
	for of >= 999.5 && unit < len(countUnits)-1 {
		of, unit = of/1000, unit+1
	}
	if of < 9.95 {
		return fmt.Sprintf("%.1f%s", of, countUnits[unit])
	}
	return fmt.Sprintf("%.0f%s", of, countUnits[unit])
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

// ellipsis stands for what TruncateBytesFront cuts.
const ellipsis = "…"

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

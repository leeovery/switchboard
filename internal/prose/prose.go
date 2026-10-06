// Package prose words what switchboard tells people the same way wherever it
// tells it: a list run together as English runs one, and text cut short.
package prose

import (
	"strings"
	"unicode/utf8"
)

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

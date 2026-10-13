package prose

import (
	"fmt"
	"strconv"
	"strings"
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

// SessionCount counts sessions in words, such as "3 sessions", "1 session" or
// "no sessions".
func SessionCount(n int) string {
	switch n {
	case 0:
		return "no sessions"
	case 1:
		return "1 session"
	default:
		return fmt.Sprintf("%d sessions", n)
	}
}

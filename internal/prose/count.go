package prose

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Tokens counts n tokens as briefly as a row has room for, as the Log counts
// them: as they are under a thousand; in thousands to a tenth under ten
// thousand, as "5.4k", and whole under a million, as "958k"; in millions to a
// tenth under a billion, as "5.7M" or "16.1M"; then in billions to a tenth
// under ten billion, as "1.5B", and whole past them, as "12B". Each form is
// for what rounds to it, so 9,960 is "10k", 999,500 is "1.0M" and
// 999,950,000 is "1.0B", a half rounded up.
func Tokens(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case rounded(n, 100) < 100:
		return inTenths(rounded(n, 100), "k")
	case rounded(n, 1000) < 1000:
		return strconv.Itoa(rounded(n, 1000)) + "k"
	case rounded(n, 100_000) < 10_000:
		return inTenths(rounded(n, 100_000), "M")
	case rounded(n, 100_000_000) < 100:
		return inTenths(rounded(n, 100_000_000), "B")
	}
	return strconv.Itoa(rounded(n, 1_000_000_000)) + "B"
}

// TokensWidest is the widest Tokens says of any count under a hundred
// thousand billion, past which "100000B" is wider: "999.9M".
func TokensWidest() string {
	return Tokens(999_900_000)
}

// Characters counts n characters as the Log counts an answer's so far: in
// thousands to a tenth at every size, as "0.8k", "5.1k", "14.2k" or "958.3k",
// a half rounded up, as Tokens rounds its tenths.
func Characters(n int) string {
	return inTenths(rounded(n, 100), "k")
}

// rounded is how many of unit n is, to the nearest, a half rounded up.
func rounded(n, unit int) int {
	return (n + unit/2) / unit
}

// inTenths writes a count of tenths of a unit, to a tenth, as "5.4k".
func inTenths(n int, unit string) string {
	return strconv.Itoa(n/10) + "." + strconv.Itoa(n%10) + unit
}

// Millions writes a count of tokens as History's Tokens counts them: under
// 100 million in millions to one place, as "4.2M"; then in whole millions, as
// "615M"; and from a billion in billions to two places, as "1.06B".
func Millions(n int) string {
	millions := float64(n) / 1e6
	switch {
	case math.Round(millions*10) < 1000:
		return fmt.Sprintf("%.1fM", millions)
	case math.Round(millions) < 1000:
		return fmt.Sprintf("%.0fM", millions)
	}
	return fmt.Sprintf("%.2fB", millions/1000)
}

// Thousands writes n as a phone's key has it: as it is under a thousand; from
// a thousand, in thousands to as many places as it takes, as "1k" or "1.25k".
func Thousands(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return strconv.FormatFloat(float64(n)/1000, 'f', -1, 64) + "k"
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

// CountedOr counts n of the thing a noun names as Counted counts it, as "4
// moves" or "1,240 sessions", or says none as zero words it, as "none" or "no
// sessions", where there are none.
func CountedOr(n int, noun, zero string) string {
	if n == 0 {
		return zero
	}
	return Counted(n, noun)
}

// SessionCount counts sessions in words, as CountedOr counts them: "3
// sessions", "1 session" or "no sessions".
func SessionCount(n int) string {
	return CountedOr(n, "session", "no sessions")
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

package prose

import (
	"fmt"
	"strings"
	"unicode"
)

// The characters that shape what's beside them, and those of a flag's
// sequence, by their code points.
const (
	zeroWidthNonJoiner = '\u200c'
	zeroWidthJoiner    = '\u200d'
	blackFlag          = '\U0001f3f4'
	firstTag           = '\U000e0020'
	cancelTag          = '\U000e007f'
)

// Unseen reports whether r is drawn as nothing: Unicode's default ignorable
// code points, the format characters, the variation selectors and the others
// it names, but for whitespace, and for the prepended concatenation marks,
// such as the Arabic number signs, which draw.
func Unseen(r rune) bool {
	return (unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) ||
		unicode.Is(unicode.Variation_Selector, r)) &&
		!unicode.IsSpace(r) && !unicode.Is(unicode.Prepended_Concatenation_Mark, r)
}

// Reorders reports whether r is a bidirectional embedding, override or
// isolate, U+202A to U+202E and U+2066 to U+2069, which can reorder the rest
// of a line it's shown in.
func Reorders(r rune) bool {
	return (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
}

// DirectionMark reports whether r is a direction mark: left-to-right,
// right-to-left or Arabic letter.
func DirectionMark(r rune) bool {
	return r == '\u200e' || r == '\u200f' || r == '\u061c'
}

// MapUnseen returns text with each unseen character that plays no part in a
// sequence, as inSequence has it, replaced by what replace makes of it: ""
// to drop it. Every other character is kept as it is.
func MapUnseen(text string, replace func(r rune) string) string {
	runes := []rune(text)
	var b strings.Builder
	for i, r := range runes {
		if Unseen(r) && !inSequence(runes, i) {
			b.WriteString(replace(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// inSequence reports whether the character at i of runes plays its part in
// a sequence with what's beside it: a joiner, zero-width or not, between a
// character it can follow and one that's seen; a variation selector after a
// character it can follow; and a tag of a flag's sequence, after a black
// flag and the tags before it, up to the one that cancels them.
func inSequence(runes []rune, i int) bool {
	r := runes[i]
	switch {
	case r == zeroWidthJoiner || r == zeroWidthNonJoiner:
		return i > 0 && i+1 < len(runes) && follows(runes[i-1]) && !unicode.IsSpace(runes[i+1]) && !Unseen(runes[i+1])
	case unicode.Is(unicode.Variation_Selector, r):
		return i > 0 && follows(runes[i-1])
	case r >= firstTag && r <= cancelTag:
		j := i - 1
		for j >= 0 && runes[j] >= firstTag && runes[j] < cancelTag {
			j--
		}
		return j >= 0 && runes[j] == blackFlag
	}
	return false
}

// follows reports whether a joiner or a variation selector can follow r:
// a character that's seen and isn't a space, or a variation selector or a
// tag, which end what they're part of.
func follows(r rune) bool {
	if unicode.IsSpace(r) {
		return false
	}
	return !Unseen(r) || unicode.Is(unicode.Variation_Selector, r) || (r >= firstTag && r <= cancelTag)
}

// escaped is r as an escape, as \u200B, so it shows as what it is.
func escaped(r rune) string {
	if r > 0xffff {
		return fmt.Sprintf(`\U%08X`, r)
	}
	return fmt.Sprintf(`\u%04X`, r)
}

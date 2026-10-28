package prose

import (
	"fmt"
	"strings"
	"unicode"
)

// The characters that make the sequences an unseen character can be kept
// in, by their code points.
const (
	zeroWidthNonJoiner = '\u200c'
	zeroWidthJoiner    = '\u200d'
	emojiPresentation  = '\ufe0f'
	keycap             = '\u20e3'
	blackFlag          = '\U0001f3f4'
	firstTag           = '\U000e0020'
	lastTag            = '\U000e007e'
	cancelTag          = '\U000e007f'
	firstModifier      = '\U0001f3fb'
	lastModifier       = '\U0001f3ff'
	// mostFlagTags is the most tags a subdivision's flag names it by.
	mostFlagTags = 8
)

// unjoined are the scripts that never shape with joiners: a joiner between
// two of their letters does nothing that shows.
var unjoined = []*unicode.RangeTable{
	unicode.Latin, unicode.Greek, unicode.Cyrillic, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul,
}

// unselected are the scripts whose letters have no variants a variation
// selector picks.
var unselected = []*unicode.RangeTable{unicode.Latin, unicode.Greek, unicode.Cyrillic}

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

// MapUnseen returns text with each unseen character that does nothing to
// show, as a sequence owns one, replaced by what replace makes of it: ""
// to drop it, the character itself to keep it, or other text, such as an
// escape, to show in its place. Every other character is kept as it is.
//
// An unseen character is kept only where it does something that shows,
// judged left to right on what's kept, so the character before one is the
// last kept, never one dropped, and after text shown in one's place, nothing
// is owned:
//   - a zero-width joiner between two pictographs, after a symbol, an emoji
//     modifier or an emoji presentation selector kept after one, and before
//     a symbol; and, as a non-joiner is, between two letters or marks of a
//     script that shapes with joiners;
//   - a variation selector after a symbol, punctuation beyond ASCII, a
//     letter outside Latin, Greek and Cyrillic, such as an ideograph, or a
//     keycap's #, * or digit, as the keycap follows;
//   - a subdivision flag's tags: a black flag, one to eight tags and the tag
//     that cancels them, whole.
func MapUnseen(text string, replace func(r rune) string) string {
	runes := []rune(text)
	flagged := flagTags(runes)
	var b strings.Builder
	var kept recent
	for i, r := range runes {
		if !Unseen(r) || flagged[i] || kept.owns(r, after(runes, i)) {
			b.WriteRune(r)
			kept.add(r)
			continue
		}
		s := replace(r)
		b.WriteString(s)
		switch s {
		case string(r):
			kept.add(r)
		case "":
		default:
			kept.add(shown)
		}
	}
	return b.String()
}

// shown stands, among the characters kept, for text replace put in place of
// an unseen character, such as an escape: seen, it ends any sequence, and
// owns nothing after it.
const shown rune = 0

// recent is the last two characters kept, last first, shown standing too for
// none, as at the start, which owns nothing after it alike.
type recent struct {
	last, before rune
}

// add has r kept last.
func (k *recent) add(r rune) {
	k.last, k.before = r, k.last
}

// owns reports whether the unseen r, kept after k's characters and before
// next, does something that shows.
func (k recent) owns(r, next rune) bool {
	switch {
	case r == zeroWidthJoiner:
		return k.afterPictograph() && unicode.IsSymbol(next) || joins(k.last) && joins(next)
	case r == zeroWidthNonJoiner:
		return joins(k.last) && joins(next)
	case selector(r):
		return selects(k.last, next)
	}
	return false
}

// afterPictograph reports whether k's last kept ends a pictograph: a symbol
// or an emoji modifier, or an emoji presentation selector kept after one.
func (k recent) afterPictograph() bool {
	pictograph := func(r rune) bool {
		return unicode.IsSymbol(r) || (r >= firstModifier && r <= lastModifier)
	}
	return pictograph(k.last) || k.last == emojiPresentation && pictograph(k.before)
}

// joins reports whether r is a letter or a mark of a script that shapes
// with joiners.
func joins(r rune) bool {
	return (unicode.IsLetter(r) || unicode.IsMark(r)) && !Unseen(r) && !unicode.In(r, unjoined...)
}

// selector reports whether r is a variation selector, U+FE00 to U+FE0F or
// U+E0100 to U+E01EF.
func selector(r rune) bool {
	return (r >= '\ufe00' && r <= '\ufe0f') || (r >= '\U000e0100' && r <= '\U000e01ef')
}

// selects reports whether a variation selector after last, and before next,
// picks a variant of last: a symbol, punctuation beyond ASCII, a letter
// outside Latin, Greek and Cyrillic, or a keycap's #, * or digit, the
// keycap next.
func selects(last, next rune) bool {
	switch {
	case unicode.IsSymbol(last):
		return true
	case unicode.IsPunct(last):
		return last > unicode.MaxASCII || (last == '#' || last == '*') && next == keycap
	case unicode.IsLetter(last):
		return !unicode.In(last, unselected...)
	}
	return last >= '0' && last <= '9' && next == keycap
}

// flagTags marks the tags of runes, and the tags that cancel them, that
// make a subdivision's flag: after a black flag, one to mostFlagTags tags,
// then the cancel tag.
func flagTags(runes []rune) []bool {
	flagged := make([]bool, len(runes))
	for i, r := range runes {
		if r != blackFlag {
			continue
		}
		end := i + 1
		for end < len(runes) && runes[end] >= firstTag && runes[end] <= lastTag {
			end++
		}
		if tags := end - i - 1; tags < 1 || tags > mostFlagTags || end == len(runes) || runes[end] != cancelTag {
			continue
		}
		for j := i + 1; j <= end; j++ {
			flagged[j] = true
		}
	}
	return flagged
}

// after is the character after the one at i of runes, 0 at the end.
func after(runes []rune, i int) rune {
	if i+1 < len(runes) {
		return runes[i+1]
	}
	return 0
}

// escaped is r as an escape, as \u200B, so it shows as what it is.
func escaped(r rune) string {
	if r > 0xffff {
		return fmt.Sprintf(`\U%08X`, r)
	}
	return fmt.Sprintf(`\u%04X`, r)
}

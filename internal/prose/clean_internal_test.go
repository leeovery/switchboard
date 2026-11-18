package prose

import (
	"strings"
	"testing"
	"unicode"
)

// strung are runes, each a string of its own.
func strung(runes ...rune) []string {
	pieces := make([]string, len(runes))
	for i, r := range runes {
		pieces[i] = string(r)
	}
	return pieces
}

// eachOf calls check with every string of at most most pieces, each one of
// pieces, the empty string among them.
func eachOf(pieces []string, most int, check func(string)) {
	var walk func(prefix string, left int)
	walk = func(prefix string, left int) {
		check(prefix)
		if left == 0 {
			return
		}
		for _, p := range pieces {
			walk(prefix+p, left-1)
		}
	}
	walk("", most)
}

// Plain ASCII Clean leaves as it is, judged a byte at a time, is what the
// whole rule makes of it, and so is any other ASCII.
func TestCleansASCIIPathAgreesWithTheWholeRule(t *testing.T) {
	ascii := strung('a', '~', ' ', '\t', '\n', 0, 0x7f)
	eachOf(ascii, 5, func(s string) {
		if got, want := Clean(s), tidy(s); got != want {
			t.Errorf("Clean(%q) = %q, want %q, as the whole rule makes it", s, got, want)
		}
	})
}

// Whatever it's given, Clean leaves one tidy line, holding no control, no
// bidirectional control, no prepended concatenation mark, no character drawn
// as nothing that does nothing that shows, and no direction mark at its
// edges, nor anywhere in text holding no letter written right to left; and
// nothing it would clean again.
func TestCleanLeavesOneTidyLineWhateverItsGiven(t *testing.T) {
	pieces := append(strung(
		'a', ' ', 1, '1', 0xa0,
		0x200b, 0x200d, 0xfe0f, 0x20e3, 0x202e,
		0x200f, 0x061c, 0x0600,
		0x1f600, 0x2764, 0x0915, 0x094d, 0x1f3f4, 0xe0067, 0x05d0,
	), "\xff")
	eachOf(pieces, 4, func(s string) {
		once := Clean(s)
		switch {
		case once != strings.TrimSpace(once) || strings.Contains(once, "  "):
			t.Errorf("Clean(%+q) = %+q, not one tidy line", s, once)
		case strings.ContainsFunc(once, unicode.IsControl) || strings.ContainsFunc(once, Reorders) || strings.ContainsFunc(once, Prepends):
			t.Errorf("Clean(%+q) = %+q, holding a control, a bidirectional control or a prepended mark", s, once)
		case MapUnseen(once, keptIfAMark) != once:
			t.Errorf("Clean(%+q) = %+q, holding a character drawn as nothing that does nothing that shows", s, once)
		case strings.TrimFunc(once, DirectionMark) != once:
			t.Errorf("Clean(%+q) = %+q, a direction mark at its edge", s, once)
		case strings.ContainsFunc(once, DirectionMark) && !strings.ContainsFunc(once, rightToLeft):
			t.Errorf("Clean(%+q) = %+q, a direction mark in text holding no letter written right to left", s, once)
		}
		if twice := Clean(once); twice != once {
			t.Errorf("Clean(Clean(%+q)) = %+q, want %+q, as Clean left it", s, twice, once)
		}
	})
}

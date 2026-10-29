package graphemes_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard/graphemes"
)

// glyph is a character and the cells it takes.
type glyph struct {
	text  string
	cells int
}

// all are text's characters, as In gives them.
func all(text string) []glyph {
	var gs []glyph
	for g, cells := range graphemes.In(text) {
		gs = append(gs, glyph{g, cells})
	}
	return gs
}

func TestInGivesEachCharacterWholeWithItsCells(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []glyph
	}{
		{name: "nothing", text: ""},
		{name: "letters, a cell each", text: "ab", want: []glyph{{"a", 1}, {"b", 1}}},
		{name: "a CJK character, two cells", text: "a漢", want: []glyph{{"a", 1}, {"漢", 2}}},
		{name: "an emoji and its presentation, one", text: "❤\uFE0Fb", want: []glyph{{"❤\uFE0F", 2}, {"b", 1}}},
		{name: "a flag, one", text: "🇬🇧", want: []glyph{{"🇬🇧", 2}}},
		{name: "a family joined, one", text: "👨\u200d👩\u200d👧", want: []glyph{{"👨\u200d👩\u200d👧", 2}}},
		{name: "a letter and its accent, one", text: "e\u0301x", want: []glyph{{"e\u0301", 1}, {"x", 1}}},
		{name: "a stray mark, no cells", text: "\u0301a", want: []glyph{{"\u0301", 0}, {"a", 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := all(tt.text); !slices.Equal(got, tt.want) {
				t.Errorf("In(%q) gives %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestInStopsWhereItsAskedTo(t *testing.T) {
	var got []string
	for g := range graphemes.In("abc") {
		got = append(got, g)
		if g == "b" {
			break
		}
	}
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("ranged till b, In gave %q", got)
	}
}

func TestRangingOverInAllocatesNothing(t *testing.T) {
	const text = "Priming starts each account's 5-hour window, 漢字 and 👨\u200d👩\u200d👧"
	var cells int
	if allocs := testing.AllocsPerRun(20, func() {
		for _, n := range graphemes.In(text) {
			cells += n
		}
	}); allocs != 0 {
		t.Errorf("ranging over In made %v allocations, want none", allocs)
	}
}

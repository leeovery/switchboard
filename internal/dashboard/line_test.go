package dashboard

import (
	"reflect"
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{name: "fits", text: "Work", width: 4, want: "Work"},
		{name: "cut", text: "Personal", width: 5, want: "Pers…"},
		{name: "cut after a space", text: "Work team", width: 6, want: "Work…"},
		{name: "wide characters", text: "東京都", width: 4, want: "東…"},
		{name: "room for the ellipsis alone", text: "Work", width: 1, want: "…"},
		{name: "no room", text: "Work", width: 0, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.text, tt.width); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

func TestFit(t *testing.T) {
	name := line{{"Switch", nameInk}, {"board", dimInk}}
	tests := []struct {
		name  string
		line  line
		width int
		want  line
	}{
		{name: "fits", line: name, width: 11, want: name},
		{name: "cut inside a span, keeping its ink", line: name, width: 8, want: line{{"Switch", nameInk}, {"b…", dimInk}}},
		{name: "cut at the end of a span", line: name, width: 6, want: line{{"Switc…", nameInk}}},
		{name: "no room", line: name, width: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.line.fit(tt.width); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fit(%d) = %q, want %q", tt.width, got.plain(), tt.want.plain())
			}
		})
	}
}

// A figure is drawn whole where it fits, else as an ellipsis alone, in its
// first span's ink, never cut part-way; marked as its number only where it's
// drawn whole, as an ellipsis is no number.
func TestFitPartsDrawsAFigureWholeOrAnEllipsis(t *testing.T) {
	worth := line{{"$1,234,567", secondaryInk}, {"+", faintInk}}
	n := Number{Kind: NumberYear, Of: "worth"}
	tests := []struct {
		name   string
		figure part
		width  int
		want   line
	}{
		{name: "fits", figure: figurePart(worth, Number{}), width: 11, want: worth},
		{name: "a cell short", figure: figurePart(worth, Number{}), width: 10, want: line{{"…", secondaryInk}}},
		{name: "numbered, fitting", figure: figurePart(worth, n), width: 11, want: worth.numbered(n)},
		{name: "numbered, a cell short", figure: figurePart(worth, n), width: 10, want: line{{"…", secondaryInk}}},
		{name: "numbered beforehand, a cell short", figure: figurePart(worth.numbered(n), Number{}), width: 10, want: line{{"…", secondaryInk}}},
		{name: "no room", figure: figurePart(worth, Number{}), width: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(tt.width, tt.figure); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fitParts(%d) = %+v, want %+v", tt.width, got, tt.want)
			}
		})
	}
}

// Parts are cut as fit cuts a line, ending with an ellipsis where they're
// cut, but that a figure is never cut part-way: drawn whole where it fits
// what's left, the parts after it left off where it fills it, else an
// ellipsis alone; words may cut.
func TestFitPartsCutsWordsButNeverAFigurePartWay(t *testing.T) {
	ratio, words, price, unit := span{"4.6×", boldPositiveInk}, span{" its ", dimInk}, span{"$1,800", dimInk}, span{" month", dimInk}
	parts := []part{figurePart(line{ratio}, Number{}), wordsPart(line{words}), figurePart(line{price}, Number{}), wordsPart(line{unit})}
	tests := []struct {
		name  string
		width int
		want  line
	}{
		{name: "fits", width: 21, want: line{ratio, words, price, unit}},
		{name: "its last words cut", width: 18, want: line{ratio, words, price, {" m…", dimInk}}},
		{name: "its last words cut to an ellipsis", width: 16, want: line{ratio, words, price, {"…", dimInk}}},
		{name: "a figure filling what's left, the words after it left off", width: 15, want: line{ratio, words, price}},
		{name: "a figure a cell short", width: 14, want: line{ratio, words, {"…", dimInk}}},
		{name: "a figure that doesn't fit", width: 12, want: line{ratio, words, {"…", dimInk}}},
		{name: "words cut", width: 7, want: line{ratio, {" i…", dimInk}}},
		{name: "the first figure filling it", width: 4, want: line{ratio}},
		{name: "the first figure a cell short", width: 3, want: line{{"…", boldPositiveInk}}},
		{name: "no room", width: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(tt.width, parts...); !reflect.DeepEqual(got, tt.want) || got.width() > tt.width {
				t.Errorf("fitParts(%d) = %+v, want %+v", tt.width, got, tt.want)
			}
		})
	}
}

// A part's number marks it only where it's a figure drawn whole: never the
// words around it, nor the ellipsis they're cut to.
func TestFitPartsMarksOnlyAFigureDrawnWhole(t *testing.T) {
	n := Number{Kind: NumberTokens, Of: "against"}
	ratio, words, price, unit := span{"4.6×", boldPositiveInk}, span{" its ", dimInk}, span{"$1,800", dimInk}, span{" month", dimInk}
	parts := []part{figurePart(line{ratio}, n), {said: line{words}, number: n}, figurePart(line{price}, n), {said: line{unit}, number: n}}
	tests := []struct {
		name  string
		width int
		want  line
	}{
		{name: "fits", width: 21, want: line{ratio.numbered(n), words, price.numbered(n), unit}},
		{name: "its last words cut to an ellipsis", width: 16, want: line{ratio.numbered(n), words, price.numbered(n), {"…", dimInk}}},
		{name: "a figure as an ellipsis", width: 14, want: line{ratio.numbered(n), words, {"…", dimInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(tt.width, parts...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fitParts(%d) = %+v, want %+v", tt.width, got, tt.want)
			}
		})
	}
}

func TestFitWholeLeavesOffWholeSpans(t *testing.T) {
	says := line{{"runs out ~16:05", exhaustedInk}, {"  ·  back 17:10", mutedInk}, {", as it resets", mutedInk}}
	tests := []struct {
		name  string
		width int
		want  line
	}{
		{name: "fits", width: 44, want: says},
		{name: "its last left off", width: 43, want: says[:2]},
		{name: "all but its first left off", width: 20, want: says[:1]},
		{name: "its first cut short, where alone it doesn't fit", width: 9, want: line{{"runs out…", exhaustedInk}}},
		{name: "no room", width: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := says.fitWhole(tt.width); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fitWhole(%d) = %q, want %q", tt.width, got.plain(), tt.want.plain())
			}
		})
	}
}

func TestSpread(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		want        string
	}{
		{name: "apart", left: "Session", right: "37%", want: "Session  37%"},
		{name: "left cut to keep a space", left: "Fable week", right: "104%", want: "Fable…  104%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spread(line{{tt.left, secondaryInk}}, line{{tt.right, secondaryInk}}, 12)
			if got.plain() != tt.want {
				t.Errorf("spread(%q, %q, 12) = %q, want %q", tt.left, tt.right, got.plain(), tt.want)
			}
		})
	}
}

func TestWrapBreaksALineAtItsSpaces(t *testing.T) {
	sentence := line{{"New sessions go to ", secondaryInk}, {"personal", titleInk}, {". When it's full, the router chooses.", secondaryInk}}
	tests := []struct {
		name  string
		l     line
		width int
		want  []string
	}{
		{name: "that fits, as it is", l: sentence, width: 76, want: []string{"New sessions go to personal. When it's full, the router chooses."}},
		{name: "a word running across spans kept whole", l: sentence, width: 27, want: []string{"New sessions go to", "personal. When it's full,", "the router chooses."}},
		{name: "a word wider than the line, cut short", l: line{{"a reasonably-long-word", secondaryInk}}, width: 10, want: []string{"a", "reasonabl…"}},
		{name: "nothing, no lines", l: nil, width: 10, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range tt.l.wrap(tt.width) {
				got = append(got, l.plain())
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("wrap(%d) = %q, want %q", tt.width, got, tt.want)
			}
		})
	}
	if got := sentence.wrap(27)[1]; got[0] != (span{"personal", titleInk}) || got[1] != (span{".", secondaryInk}) {
		t.Errorf("the second line leads %+v, want each part of a word in its own ink", got[:2])
	}
}

// plain is the line's text without its inks.
func (l line) plain() string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.text)
	}
	return b.String()
}

package dashboard

import (
	"math"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// usd is an amount of dollars, as the ledger counts money.
func usd(dollars float64) *ledger.Picodollars {
	p := ledger.Picodollars(math.Round(dollars * 1e12))
	return &p
}

// A worth is written to the dollar, its thousands set apart, in the ink its
// row gives it: a faint + after it where part is unpriced, unpriced where
// all is, and a dash for none.
func TestAWorthIsWrittenToTheDollar(t *testing.T) {
	k := mutedInk
	tests := []struct {
		name  string
		worth views.Worth
		want  line
	}{
		{name: "priced", worth: views.Worth{Amount: usd(634.5)}, want: line{{"$635", k}}},
		{name: "a thousand or more", worth: views.Worth{Amount: usd(27479.5)}, want: line{{"$27,480", k}}},
		{name: "part unpriced", worth: views.Worth{Amount: usd(184), Unpriced: []string{"no_usage"}}, want: line{{"$184", k}, {"+", faintInk}}},
		{name: "wholly unpriced", worth: views.Worth{Unpriced: []string{"claude-opus-9"}}, want: line{{"unpriced", k}}},
		{name: "nothing", worth: views.Worth{}, want: line{noFigure}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worthSaid(tt.worth, k); !slices.Equal(got, tt.want) {
				t.Errorf("worthSaid(%+v) = %+v, want %+v", tt.worth, got, tt.want)
			}
		})
	}
}

// Against its price is to a tenth, a half to the even one, bold, in
// state.positive at 1× or more as it's written, and accent.attention under; a
// dash where there's none.
func TestAgainstItsPriceIsColouredAsItsWritten(t *testing.T) {
	tests := []struct {
		ratio *float64
		want  line
	}{
		{ratio: new(3.8166), want: line{{"3.8×", boldPositiveInk}}},
		{ratio: new(0.96), want: line{{"1.0×", boldPositiveInk}}},
		{ratio: new(0.94), want: line{{"0.9×", alertInk}}},
		{ratio: new(0.25), want: line{{"0.2×", alertInk}}},
		{ratio: new(1.25), want: line{{"1.2×", boldPositiveInk}}},
		{ratio: new(0.75), want: line{{"0.8×", alertInk}}},
		{want: line{noFigure}},
	}
	for _, tt := range tests {
		if got := againstSaid(tt.ratio); !slices.Equal(got, tt.want) {
			t.Errorf("againstSaid(%v) = %+v, want %+v", tt.ratio, got, tt.want)
		}
	}
}

// A share is a whole percentage, in the ink its figure gives it; a dash
// where it isn't known.
func TestAShareIsAWholePercentage(t *testing.T) {
	if got, want := shareSaid(new(0.214), strongInk), (line{{"21%", strongInk}}); !slices.Equal(got, want) {
		t.Errorf("a share of 0.214 reads %+v, want %+v", got, want)
	}
	if got, want := shareSaid(nil, strongInk), (line{noFigure}); !slices.Equal(got, want) {
		t.Errorf("an unknown share reads %+v, want %+v", got, want)
	}
}

// What History says as views words it is drawn in its ink, each figure among
// it a figure, so a wrap never breaks it, and the rest words.
func TestWordsViewsSayAreDrawnTheirFiguresFigures(t *testing.T) {
	got := saidParts(views.ShortOutOf(6, 7), mutedInk)
	want := []part{wordsIn("short on ", mutedInk), figureIn("6", mutedInk), wordsIn(" of ", mutedInk), figureIn("7", mutedInk)}
	if !slices.EqualFunc(got, want, func(a, b part) bool { return a.figure == b.figure && slices.Equal(a.said, b.said) }) {
		t.Errorf("short on 6 of 7 is drawn as %+v, want %+v", got, want)
	}
}

// A family is named with its first letter capital, cleaned, after its dot
// in its hue, or text.muted for one without a hue of its own.
func TestAFamilyIsNamedAfterItsDot(t *testing.T) {
	tests := []struct {
		family string
		want   line
	}{
		{family: "opus", want: line{{"●", ink{token: theme.VizFamily1}}, {" Opus", dimInk}}},
		{family: "claude-unknown-9", want: line{{"●", ink{token: theme.TextMuted}}, {" Claude-unknown-9", dimInk}}},
		{family: "\x1b[2Jécho", want: line{{"●", ink{token: theme.TextMuted}}, {" [2Jécho", dimInk}}},
		{family: "élan", want: line{{"●", ink{token: theme.TextMuted}}, {" Élan", dimInk}}},
	}
	for _, tt := range tests {
		if got := familyDot(tt.family, dimInk); !slices.Equal(got, tt.want) {
			t.Errorf("familyDot(%q) = %+v, want %+v", tt.family, got, tt.want)
		}
	}
}

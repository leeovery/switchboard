package views_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// figures are the figures among words, in order.
func figures(w status.Words) []string {
	var said []string
	for _, p := range w {
		if p.Kind == status.PartFigure {
			said = append(said, p.Text)
		}
	}
	return said
}

// Weeks' phrases are said once, for History's Weeks and history's text
// alike: their counts figures, the dashboard's to draw as figures, and a
// count of none in words.
func TestWeeksPhrasesSayTheirCountsAsFigures(t *testing.T) {
	tests := []struct {
		name    string
		said    status.Words
		want    string
		figures []string
	}{
		{name: "the headline", said: views.Headline(3, views.AboutRight), want: "three is about right", figures: []string{"three"}},
		{name: "short on some", said: views.ShortOn(6), want: "short on 6", figures: []string{"6"}},
		{name: "short on none", said: views.ShortOn(0), want: "short on none"},
		{name: "short on some of the weeks", said: views.ShortOfWeeks(3, 7), want: "short on 3 of 7 weeks", figures: []string{"3", "7"}},
		{name: "short on none of a week", said: views.ShortOfWeeks(0, 1), want: "short on none of 1 week", figures: []string{"1"}},
		{name: "short on some of the last weeks", said: views.ShortOfTheLastWeeks(3, 7), want: "short on 3 of the last 7 weeks", figures: []string{"3", "7"}},
		{name: "short on some out of the weeks", said: views.ShortOutOf(6, 7), want: "short on 6 of 7", figures: []string{"6", "7"}},
		{name: "short on none out of the weeks", said: views.ShortOutOf(0, 7), want: "short on none"},
		{name: "some out of a count", said: views.OutOf(0, 7), want: "0 of 7", figures: []string{"0", "7"}},
		{name: "a limit hit", said: views.LimitHit(1, 1), want: "limit hit 1 of 1 week", figures: []string{"1", "1"}},
		{name: "limits hit, their thousands set apart", said: views.LimitHit(1200, 1300), want: "limit hit 1,200 of 1,300 weeks", figures: []string{"1,200", "1,300"}},
		{name: "left at a reset", said: views.LeftOnAverage(0.09), want: "9% left at a reset, on average", figures: []string{"9%"}},
		{name: "a count of a thing", said: views.CountedWords(1240, "request"), want: "1,240 requests", figures: []string{"1,240"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.said.String(); got != tt.want {
				t.Errorf("said %q, want %q", got, tt.want)
			}
			if got := figures(tt.said); !slices.Equal(got, tt.figures) {
				t.Errorf("%q has the figures %q, want %q", tt.want, got, tt.figures)
			}
		})
	}
}

// The busiest day is named by its date, and the longest run counted in
// days, each nothing where there's none, for its reader to mark.
func TestHistorysDayAndRunAreSaidOnceNothingWhereThereIsNone(t *testing.T) {
	tests := []struct {
		name, got, want string
	}{
		{name: "a day", got: views.DateNamed("2026-09-22"), want: "Tue 22 Sep"},
		{name: "no day", got: views.DateNamed("")},
		{name: "a date that isn't one", got: views.DateNamed("2026-13-45")},
		{name: "a run of a day", got: views.DaysInARow(1), want: "1 day"},
		{name: "a run of days", got: views.DaysInARow(58), want: "58 days"},
		{name: "no run", got: views.DaysInARow(0)},
		{name: "the one to drop", got: views.ToDrop("side"), want: "side is the one to drop"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

package views

import (
	"slices"
	"testing"
)

// The key rounds each cut on its own: to 10, a cut under 10 to 1, under 1 to
// a tenth, under a tenth to a hundredth, a half up; and where two that
// differ would then read the same, or out of order, each of them a step
// finer, till none do. Cuts that are the same stay the same.
func TestHoursKeyCuts(t *testing.T) {
	tests := []struct {
		name string
		cuts []float64
		want []string
	}{
		{name: "to 10", cuts: []float64{12, 48, 93}, want: []string{"10", "50", "90"}},
		{name: "to 10, a half up", cuts: []float64{15, 45, 1234.5}, want: []string{"20", "50", "1,230"}},
		{name: "one under 10 to 1, the rest to 10", cuts: []float64{3, 47, 92}, want: []string{"3", "50", "90"}},
		{name: "each under its step to the next", cuts: []float64{0.4, 6, 23}, want: []string{"0.4", "6", "20"}},
		{name: "a pair 10 merges to 1, the rest to 10", cuts: []float64{12, 14, 80}, want: []string{"12", "14", "80"}},
		{name: "a pair 10 merges to 1, the third left", cuts: []float64{12, 14, 87}, want: []string{"12", "14", "90"}},
		{name: "all under 10", cuts: []float64{1.5, 3.25, 7.75}, want: []string{"2", "3", "8"}},
		{name: "under 1, a half up", cuts: []float64{0.25, 0.5, 1.25}, want: []string{"0.3", "0.5", "1"}},
		{name: "to a hundredth where a tenth merges two", cuts: []float64{0.25, 1.0 / 3, 0.5}, want: []string{"0.25", "0.33", "0.5"}},
		{name: "out of order at 10, both to 1", cuts: []float64{15, 17, 20.25}, want: []string{"15", "17", "20"}},
		{name: "a pair finer step by step to a hundredth", cuts: []float64{15.25, 46.0 / 3, 40}, want: []string{"15.25", "15.33", "40"}},
		{name: "the same cuts kept the same", cuts: []float64{24, 24, 24}, want: []string{"20", "20", "20"}},
		{name: "none", cuts: []float64{}, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hoursKeyCuts(tt.cuts); !slices.Equal(got, tt.want) {
				t.Errorf("hoursKeyCuts(%v) = %q, want %q", tt.cuts, got, tt.want)
			}
		})
	}
}

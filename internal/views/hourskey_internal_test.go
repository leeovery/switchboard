package views

import (
	"slices"
	"testing"
)

// The key rounds the cuts to 10; to 1 where rounding to 10 would make two
// the same, or round one under 10; and to a tenth, then a hundredth, where
// averages under a few requests need it to keep every bound distinct and
// above none.
func TestHoursKeyCuts(t *testing.T) {
	tests := []struct {
		name string
		cuts []float64
		want []string
	}{
		{name: "to 10", cuts: []float64{12, 48, 93}, want: []string{"10", "50", "90"}},
		{name: "to 10, a half up", cuts: []float64{15, 45, 1234.5}, want: []string{"20", "50", "1,230"}},
		{name: "to 1 where 10 merges two", cuts: []float64{12, 14, 93}, want: []string{"12", "14", "93"}},
		{name: "to 1 where one is under 10", cuts: []float64{3, 24, 60}, want: []string{"3", "24", "60"}},
		{name: "all under 10", cuts: []float64{1.5, 3.25, 7.75}, want: []string{"2", "3", "8"}},
		{name: "to a tenth where one is under 1", cuts: []float64{0.25, 0.5, 1.25}, want: []string{"0.3", "0.5", "1.3"}},
		{name: "to a hundredth where a tenth merges two", cuts: []float64{0.25, 1.0 / 3, 0.5}, want: []string{"0.25", "0.33", "0.50"}},
		{name: "the same cuts kept the same", cuts: []float64{24, 24, 24}, want: []string{"20", "20", "20"}},
		{name: "none", cuts: []float64{}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hoursKeyCuts(tt.cuts); !slices.Equal(got, tt.want) {
				t.Errorf("hoursKeyCuts(%v) = %q, want %q", tt.cuts, got, tt.want)
			}
		})
	}
}

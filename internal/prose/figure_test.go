package prose_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

func TestPercent(t *testing.T) {
	tests := []struct {
		utilization float64
		whole       int
		want        string
	}{
		{utilization: 0, whole: 0, want: "0%"},
		{utilization: 0.004, whole: 0, want: "0%"},
		{utilization: 0.23, whole: 23, want: "23%"},
		{utilization: 0.125, whole: 12, want: "12%"},
		{utilization: 0.625, whole: 62, want: "62%"},
		{utilization: 0.996, whole: 100, want: "100%"},
		{utilization: 1.04, whole: 104, want: "104%"},
	}
	for _, tt := range tests {
		if got := prose.WholePercent(tt.utilization); got != tt.whole {
			t.Errorf("WholePercent(%v) = %d, want %d", tt.utilization, got, tt.whole)
		}
		if got := prose.Percent(tt.utilization); got != tt.want {
			t.Errorf("Percent(%v) = %q, want %q", tt.utilization, got, tt.want)
		}
	}
}

// A window used whole is said "100%", as wide as Percent says any use to its
// limit.
func TestPercentWholeIsAWindowUsedWhole(t *testing.T) {
	if got := prose.PercentWhole(); got != prose.Percent(1) {
		t.Errorf("PercentWhole = %q, want %q", got, prose.Percent(1))
	}
	for thousandths := range 1001 {
		if said := prose.Percent(float64(thousandths) / 1000); len(said) > len(prose.PercentWhole()) {
			t.Errorf("Percent(%v) = %q, wider than PercentWhole's %q", float64(thousandths)/1000, said, prose.PercentWhole())
		}
	}
}

// A figure to a tenth rounds a half to the even one, as every figure does,
// so 0.25 is 0.2 and 0.75 is 0.8; a sliver under nothing is 0.0, never -0.0.
// A ratio that rounds to 100 or more is written whole, 99.95 as 100×.
func TestAFigureToATenthRoundsAHalfToTheEvenOne(t *testing.T) {
	tests := []struct {
		x       float64
		nearest float64
		tenths  string
		ratio   string
	}{
		{x: 0, nearest: 0, tenths: "0.0", ratio: "0.0×"},
		{x: 1.5, nearest: 1.5, tenths: "1.5", ratio: "1.5×"},
		{x: 3.8166, nearest: 3.8, tenths: "3.8", ratio: "3.8×"},
		{x: 0.96, nearest: 1, tenths: "1.0", ratio: "1.0×"},
		{x: 0.94, nearest: 0.9, tenths: "0.9", ratio: "0.9×"},
		{x: 0.25, nearest: 0.2, tenths: "0.2", ratio: "0.2×"},
		{x: 0.75, nearest: 0.8, tenths: "0.8", ratio: "0.8×"},
		{x: 1.25, nearest: 1.2, tenths: "1.2", ratio: "1.2×"},
		{x: -0.04, nearest: 0, tenths: "0.0", ratio: "0.0×"},
		{x: 99.94, nearest: 99.9, tenths: "99.9", ratio: "99.9×"},
		{x: 99.95, nearest: 100, tenths: "100.0", ratio: "100×"},
		{x: 100, nearest: 100, tenths: "100.0", ratio: "100×"},
		{x: 123.4, nearest: 123.4, tenths: "123.4", ratio: "123×"},
		{x: 1234.5, nearest: 1234.5, tenths: "1234.5", ratio: "1234×"},
	}
	for _, tt := range tests {
		if got := prose.NearestTenth(tt.x); got != tt.nearest {
			t.Errorf("NearestTenth(%v) = %v, want %v", tt.x, got, tt.nearest)
		}
		if got := prose.Tenths(tt.x); got != tt.tenths {
			t.Errorf("Tenths(%v) = %q, want %q", tt.x, got, tt.tenths)
		}
		if got := prose.Ratio(tt.x); got != tt.ratio {
			t.Errorf("Ratio(%v) = %q, want %q", tt.x, got, tt.ratio)
		}
	}
}

// A figure to places is written to them, its thousands set apart.
func TestAFigureGroupedToPlacesSetsItsThousandsApart(t *testing.T) {
	tests := []struct {
		x      float64
		places int
		want   string
	}{
		{x: 90, places: 0, want: "90"},
		{x: 1240, places: 0, want: "1,240"},
		{x: 0.3, places: 1, want: "0.3"},
		{x: 0.30000000000000004, places: 1, want: "0.3"},
		{x: 0.5, places: 2, want: "0.50"},
		{x: 12345.67, places: 2, want: "12,345.67"},
		{x: -0.5, places: 1, want: "-0.5"},
		{x: -1234.5, places: 1, want: "-1,234.5"},
	}
	for _, tt := range tests {
		if got := prose.GroupedTo(tt.x, tt.places); got != tt.want {
			t.Errorf("GroupedTo(%v, %d) = %q, want %q", tt.x, tt.places, got, tt.want)
		}
	}
}

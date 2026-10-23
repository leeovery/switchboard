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

// A figure to a tenth rounds a half to the even one, as every figure does,
// so 0.25 is 0.2 and 0.75 is 0.8; a sliver under nothing is 0.0, never -0.0.
// A window used whole is said "100%", the widest of its use to its limit.
func TestPercentWholeIsAWindowUsedWhole(t *testing.T) {
	if got := prose.PercentWhole(); got != "100%" || len(got) < len(prose.Percent(0.995)) {
		t.Errorf("PercentWhole = %q, want %q, as wide as any use to its limit", got, "100%")
	}
}

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

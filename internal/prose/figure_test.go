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

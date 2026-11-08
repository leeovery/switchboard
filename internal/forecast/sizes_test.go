package forecast

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestSizes(t *testing.T) {
	tests := []struct {
		name  string
		plans []string
		want  []float64
	}{
		{"no accounts", nil, []float64{}},
		{"a pro week is 1", []string{"pro"}, []float64{1}},
		{"a max week is 5", []string{"max"}, []float64{5}},
		{"an ultra week is 20", []string{"ultra"}, []float64{20}},
		{"every plan, in the order given", []string{"ultra", "pro", "max", "pro"}, []float64{20, 1, 5, 1}},
		{"a plan not known makes every account 1", []string{"ultra", "", "max"}, []float64{1, 1, 1}},
		{"a plan of another name makes every account 1", []string{"max", "ultra", "team"}, []float64{1, 1, 1}},
		{"no plan known, every account 1", []string{"", ""}, []float64{1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sizes(tt.plans); !slices.Equal(got, tt.want) {
				t.Errorf("Sizes(%q) = %v, want %v", tt.plans, got, tt.want)
			}
		})
	}
}

func TestSizesGoByTheConfigsPlanNames(t *testing.T) {
	plans := []string{config.PlanPro, config.PlanMax, config.PlanUltra}
	want := []float64{1, 5, 20}
	if got := Sizes(plans); !slices.Equal(got, want) {
		t.Errorf("Sizes(%q) = %v, want %v", plans, got, want)
	}
}

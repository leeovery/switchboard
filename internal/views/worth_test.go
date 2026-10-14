package views_test

import (
	"math"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// usd is an amount of dollars, as the ledger counts money.
func usd(dollars float64) *ledger.Picodollars {
	p := ledger.Picodollars(math.Round(dollars * 1e12))
	return &p
}

// A worth to the cent is the Log's: under half a cent but more than none
// reads "<$0.01", not as nothing; unpriced where all is; a + after it where
// part is.
func TestAWorthIsSaidToTheCent(t *testing.T) {
	tests := []struct {
		name  string
		worth views.Worth
		want  string
	}{
		{name: "priced", worth: views.Worth{Amount: usd(0.15)}, want: "$0.15"},
		{name: "half a cent, rounded up", worth: views.Worth{Amount: usd(0.005)}, want: "$0.01"},
		{name: "under half a cent", worth: views.Worth{Amount: usd(0.0049)}, want: "<$0.01"},
		{name: "a millionth of a cent", worth: views.Worth{Amount: usd(1e-8)}, want: "<$0.01"},
		{name: "priced at nothing", worth: views.Worth{Amount: usd(0)}, want: "$0.00"},
		{name: "nothing", worth: views.Worth{}, want: "$0.00"},
		{name: "a thousand or more", worth: views.Worth{Amount: usd(1234.5)}, want: "$1234.50"},
		{name: "part unpriced", worth: views.Worth{Amount: usd(17.856), Unpriced: []string{"no_usage"}}, want: "$17.86+"},
		{name: "under half a cent, part unpriced", worth: views.Worth{Amount: usd(0.001), Unpriced: []string{"no_usage"}}, want: "<$0.01+"},
		{name: "wholly unpriced", worth: views.Worth{Unpriced: []string{"claude-opus-9"}}, want: "unpriced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.worth.Cents(); got != tt.want {
				t.Errorf("Cents() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A worth to the dollar is History's: its thousands set apart, a + after it
// where part is unpriced, unpriced where all is, and nothing for none, which
// each surface says its own way.
func TestAWorthIsSaidToTheDollar(t *testing.T) {
	tests := []struct {
		name  string
		worth views.Worth
		want  string
	}{
		{name: "priced", worth: views.Worth{Amount: usd(634.5)}, want: "$635"},
		{name: "a thousand or more", worth: views.Worth{Amount: usd(27479.5)}, want: "$27,480"},
		{name: "part unpriced", worth: views.Worth{Amount: usd(184), Unpriced: []string{"no_usage"}}, want: "$184" + views.PartUnpriced},
		{name: "wholly unpriced", worth: views.Worth{Unpriced: []string{"claude-opus-9"}}, want: "unpriced"},
		{name: "nothing", worth: views.Worth{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.worth.Dollars(); got != tt.want {
				t.Errorf("Dollars() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A plan's price a month is to the dollar, its thousands set apart; a plan
// whose price isn't known says none.
func TestAPlansPriceAMonthSetsItsThousandsApart(t *testing.T) {
	tests := []struct {
		name string
		plan views.PlanCost
		want string
	}{
		{name: "a plan's", plan: views.PlanCost{Price: usd(200)}, want: "$200/mo"},
		{name: "a thousand or more", plan: views.PlanCost{Price: usd(2000)}, want: "$2,000/mo"},
		{name: "not known", plan: views.PlanCost{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.Monthly(); got != tt.want {
				t.Errorf("Monthly() = %q, want %q", got, tt.want)
			}
		})
	}
	if got := views.PerMonth(*usd(1000)); got != "$1,000/mo" {
		t.Errorf("PerMonth($1,000) = %q, want $1,000/mo", got)
	}
}

// A week's peak is a whole percentage, a week still running's marked as so
// far.
func TestAWeeksPeakIsMarkedSoFarWhileItsWeekRuns(t *testing.T) {
	tests := []struct {
		share float64
		open  bool
		want  string
	}{
		{share: 0.6, want: "60%"},
		{share: 1, want: "100%"},
		{share: 0.34, open: true, want: "34%…"},
	}
	for _, tt := range tests {
		if got := views.Peak(tt.share, tt.open); got != tt.want {
			t.Errorf("Peak(%v, %v) = %q, want %q", tt.share, tt.open, got, tt.want)
		}
	}
}

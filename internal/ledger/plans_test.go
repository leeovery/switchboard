package ledger_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

func TestEachPlanIsPricedAMonthAndSizedInPros(t *testing.T) {
	tests := []struct {
		id, name string
		size     int
		from     string
		monthly  ledger.Picodollars
	}{
		{id: "pro", name: "Pro", size: 1, from: "2023-09-07", monthly: dollars(20)},
		{id: "max5x", name: "Max 5x", size: 5, from: "2025-04-09", monthly: dollars(100)},
		{id: "max20x", name: "Max 20x", size: 20, from: "2025-04-09", monthly: dollars(200)},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			plan, ok := ledger.Pricing.Plan(tt.id)
			if !ok || plan.Name != tt.name || plan.Size != tt.size || plan.Price(today) != tt.monthly {
				t.Fatalf("Plan(%s) = %+v, %v, want %s, the size of %d Pros, at %s a month", tt.id, plan, ok, tt.name, tt.size, tt.monthly.Cents())
			}
			if len(plan.Prices) != 1 || plan.Prices[0].From != tt.from {
				t.Errorf("Plan(%s) is priced %+v, want its price from the day it launched, %s", tt.id, plan.Prices, tt.from)
			}
		})
	}
	if got := len(ledger.Pricing.Plans); got != len(tests) {
		t.Errorf("the table prices %d plans, want %d", got, len(tests))
	}
}

func TestEveryPlanTheConfigNamesIsPriced(t *testing.T) {
	for _, id := range config.Plans {
		if _, ok := ledger.Pricing.Plan(id); !ok {
			t.Errorf("Plan(%s) isn't priced, want every plan an account can be on priced", id)
		}
	}
}

func TestAPlanTheTableDoesntKnowIsntPriced(t *testing.T) {
	for _, id := range []string{"team", "Max 5x", "MAX5X", ""} {
		if plan, ok := ledger.Pricing.Plan(id); ok {
			t.Errorf("Plan(%q) = %+v, want no plan", id, plan)
		}
	}
}

func TestAPlanIsPricedAtTheDayAskedFor(t *testing.T) {
	// A plan whose price rose the day before today.
	plan := ledger.Plan{ID: "pro", Prices: []ledger.PlanPrice{{From: "2023-09-07", Monthly: dollars(20)}, {From: "2026-10-06", Monthly: dollars(25)}}}
	tests := []struct {
		name string
		date string
		want ledger.Picodollars
	}{
		{name: "a day before the rise", date: date, want: dollars(20)},
		{name: "the day the rise took effect", date: "2026-10-06", want: dollars(25)},
		{name: "today, since the rise", date: today, want: dollars(25)},
		{name: "a day before its first price, at that", date: "2023-01-01", want: dollars(20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plan.Price(tt.date); got != tt.want {
				t.Errorf("Price(%s) = %s, want %s", tt.date, got.Cents(), tt.want.Cents())
			}
		})
	}
}

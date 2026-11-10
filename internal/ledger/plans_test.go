package ledger_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

func TestEachPlanIsPricedAMonthAndSizedInProWeeks(t *testing.T) {
	tests := []struct {
		id, name string
		size     int64
		from     string
		monthly  ledger.Picodollars
	}{
		{id: "pro", name: "Pro", size: 1, from: "2023-09-07", monthly: dollars(20)},
		{id: "max", name: "Max", size: 5, from: "2025-04-09", monthly: dollars(100)},
		{id: "ultra", name: "Ultra", size: 10, from: "2025-04-09", monthly: dollars(200)},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			plan, ok := ledger.Pricing.Plan(tt.id)
			if !ok || plan.Name != tt.name || plan.Size != tt.size || plan.Price(today) != tt.monthly {
				t.Fatalf("Plan(%s) = %+v, %v, want %s, the size of %d Pro weeks, at %s a month", tt.id, plan, ok, tt.name, tt.size, tt.monthly.Cents())
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
	for _, id := range []string{"team", "Max", "ULTRA", "max5x", "max20x", ""} {
		if plan, ok := ledger.Pricing.Plan(id); ok {
			t.Errorf("Plan(%q) = %+v, want no plan", id, plan)
		}
	}
}

func TestAccountsAreSizedByTheirPlansOrThePlanMostAreOn(t *testing.T) {
	tests := []struct {
		name  string
		plans []string
		want  []int64
		usual int64
	}{
		{name: "no accounts", plans: nil, want: []int64{}, usual: 1},
		{name: "a pro week is 1", plans: []string{"pro"}, want: []int64{1}, usual: 1},
		{name: "a max week is 5", plans: []string{"max"}, want: []int64{5}, usual: 5},
		{name: "an ultra week is 10", plans: []string{"ultra"}, want: []int64{10}, usual: 10},
		{name: "every plan, in the order given", plans: []string{"ultra", "pro", "max", "pro"}, want: []int64{10, 1, 5, 1}, usual: 1},
		{name: "a plan not known is the plan most are on", plans: []string{"max", "", "ultra", "max"}, want: []int64{5, 5, 10, 5}, usual: 5},
		{name: "a plan of another name is the plan most are on", plans: []string{"pro", "team", "pro", "ultra"}, want: []int64{1, 1, 1, 10}, usual: 1},
		{name: "a plan not known, where two tie, is the larger", plans: []string{"ultra", "", "max"}, want: []int64{10, 10, 5}, usual: 10},
		{name: "plans not known beside one alone", plans: []string{"", "max", ""}, want: []int64{5, 5, 5}, usual: 5},
		{name: "no plan known, every account 1", plans: []string{"", "max20x"}, want: []int64{1, 1}, usual: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, usual := ledger.Pricing.Sizes(tt.plans)
			if !slices.Equal(got, tt.want) || usual != tt.usual {
				t.Errorf("Sizes(%q) = %v, %d, want %v, %d", tt.plans, got, usual, tt.want, tt.usual)
			}
		})
	}
}

func TestAPlansMostIsTheMostItHasCostOnAnyDay(t *testing.T) {
	tests := []struct {
		name   string
		prices []ledger.PlanPrice
		want   ledger.Picodollars
	}{
		{name: "one price", prices: []ledger.PlanPrice{{From: "2023-09-07", Monthly: dollars(20)}}, want: dollars(20)},
		{name: "a rise", prices: []ledger.PlanPrice{{From: "2023-09-07", Monthly: dollars(20)}, {From: "2026-10-06", Monthly: dollars(25)}}, want: dollars(25)},
		{name: "a fall", prices: []ledger.PlanPrice{{From: "2023-09-07", Monthly: dollars(200)}, {From: "2026-10-06", Monthly: dollars(150)}}, want: dollars(200)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (ledger.Plan{ID: "pro", Prices: tt.prices}).Most(); got != tt.want {
				t.Errorf("Most() = %s, want %s", got.Cents(), tt.want.Cents())
			}
		})
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

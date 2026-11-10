package ledger

import (
	"cmp"
	"slices"
)

// Plan is a plan a subscription is on, as the price table prices it.
type Plan struct {
	// ID names it as the config does, as ultra.
	ID string
	// Name names it as the views show it, as Ultra.
	Name string
	// Size is how big its week is, in Pro weeks, as Anthropic publishes each
	// plan's weekly usage against Pro's: what the forecast and History's
	// replay of a week weigh an account on it by, as Sizes has them.
	Size int64
	// Prices are what it costs a month, each from the day it took effect,
	// oldest first.
	Prices []PlanPrice
}

// PlanPrice is what a plan costs a month, from the day it took effect.
type PlanPrice struct {
	// From is the date of the day it took effect, as 2025-04-09.
	From    string
	Monthly Picodollars
}

func (p PlanPrice) effective() string { return p.From }

// Plan returns the plan the config names by the id given, reporting false
// for one the table doesn't price.
func (t Table) Plan(id string) (Plan, bool) {
	i := slices.IndexFunc(t.Plans, func(p Plan) bool { return p.ID == id })
	if i < 0 || len(t.Plans[i].Prices) == 0 {
		return Plan{}, false
	}
	return t.Plans[i], true
}

// Sizes are the sizes, in Pro weeks, of accounts on the plans with the ids
// given, in their order, as the config names each: each one's plan's size,
// else, where the table doesn't know its plan, as where it isn't known, that
// of the plan most of them are on, the larger of two that tie, else, where
// none is known, 1; and that most usual size.
func (t Table) Sizes(plans []string) (sizes []int64, usual int64) {
	sizes = make([]int64, len(plans))
	known := make(map[int64]int)
	for i, id := range plans {
		if plan, ok := t.Plan(id); ok {
			sizes[i] = plan.Size
			known[plan.Size]++
		}
	}
	usual = 1
	for size, n := range known {
		if n > known[usual] || n == known[usual] && size > usual {
			usual = size
		}
	}
	for i := range sizes {
		if sizes[i] == 0 {
			sizes[i] = usual
		}
	}
	return sizes, usual
}

// Price returns what the plan costs a month on the local day with the given
// date, as inEffect finds it. The plan must have a price, as those Plan
// returns do.
func (p Plan) Price(date string) Picodollars {
	return inEffect(p.Prices, date).Monthly
}

// Most returns the most the plan has cost a month, on any day: whatever
// day a period is priced at, its price is no more. The plan must have a
// price, as those Plan returns do.
func (p Plan) Most() Picodollars {
	return slices.MaxFunc(p.Prices, func(a, b PlanPrice) int { return cmp.Compare(a.Monthly, b.Monthly) }).Monthly
}

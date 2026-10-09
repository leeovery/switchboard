package ledger

import "slices"

// Plan is a plan a subscription is on, as the price table prices it.
type Plan struct {
	// ID names it as the config does, as max5x.
	ID string
	// Name names it as the views show it, as Max 5x.
	Name string
	// Size is its size in Pros, which History's replay of a week weighs an
	// account on it by.
	Size int
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

// Price returns what the plan costs a month on the local day with the given
// date, as inEffect finds it. The plan must have a price, as those Plan
// returns do.
func (p Plan) Price(date string) Picodollars {
	return inEffect(p.Prices, date).Monthly
}

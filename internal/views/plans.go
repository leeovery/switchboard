package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

// share is a part of a month, num over den: what a plan costs over a period,
// as a part of its monthly price.
type share struct{ num, den int64 }

// The shares of a month Accounts and History price their periods at: a day
// 12/365 of a month, a week 12/52, 30 days or a calendar month a month, and
// 12 weeks twelve weeks'.
var (
	aWeek       = share{12, 52}
	aMonth      = share{1, 1}
	twelveWeeks = share{12 * 12, 52}
)

// days is the share of a month n days are.
func days(n int) share {
	return share{12 * int64(n), 365}
}

// prorate returns the part s of monthly, a plan's monthly price, to the
// nearest picodollar.
func prorate(monthly ledger.Picodollars, s share) ledger.Picodollars {
	whole, part := int64(monthly)/s.den, int64(monthly)%s.den
	return ledger.Picodollars(whole*s.num + (part*s.num+s.den/2)/s.den)
}

// askedShare is the share of a month the days from the one with the date
// from to today's, each as civil gives it, are: as Accounts prices the
// period they make, where they make one, today, this week from the day
// weeks start on, the last 30 days or the last 12 weeks, each whole while
// it's still running; else a day each.
func askedShare(from, today time.Time, weekStarts time.Weekday) share {
	week := weekOf(today, weekStarts)
	switch {
	case from.Equal(today):
		return days(1)
	case from.Equal(week):
		return aWeek
	case from.Equal(today.AddDate(0, 0, -29)):
		return aMonth
	case from.Equal(week.AddDate(0, 0, -7*11)):
		return twelveWeeks
	}
	return days(int(today.Sub(from)/(24*time.Hour)) + 1)
}

// weekOf is the first day of the week the day falls in, as civil gives it,
// weeks starting on the day given.
func weekOf(day time.Time, starts time.Weekday) time.Time {
	return day.AddDate(0, 0, -int((7+day.Weekday()-starts)%7))
}

// Against is what plans cost over a period, Cost, and the worth of their
// accounts' requests against it, Ratio: each left out where no plan is
// known, and Ratio where they cost nothing.
type Against struct {
	Cost  *ledger.Picodollars `json:"cost,omitempty"`
	Ratio *float64            `json:"against,omitempty"`
}

// against is worth against cost.
func against(cost, worth ledger.Picodollars) Against {
	a := Against{Cost: &cost}
	if cost > 0 {
		ratio := float64(worth) / float64(cost)
		a.Ratio = &ratio
	}
	return a
}

// plans are the plans the configured accounts are on, as the price table
// prices them.
type plans struct {
	accounts config.Accounts
	table    ledger.Table
}

// of returns the plan of the account with the given id, reporting false for
// one whose plan isn't known, or that isn't configured.
func (p plans) of(account string) (ledger.Plan, bool) {
	for _, a := range p.accounts {
		if a.ID == account && a.Plan != "" {
			return p.table.Plan(a.Plan)
		}
	}
	return ledger.Plan{}, false
}

// against returns what the accounts' plans cost over a period, its share s
// of a month, each at its price on the day with the date first, the
// period's first, and the worth against it of those accounts alone, as
// worths, by account, hold it.
func (p plans) against(worths map[string]Worth, first string, s share) Against {
	var cost, worth ledger.Picodollars
	known := false
	for _, a := range p.accounts {
		if plan, ok := p.of(a.ID); ok {
			cost += prorate(plan.Price(first), s)
			worth += worths[a.ID].amount()
			known = true
		}
	}
	if !known {
		return Against{}
	}
	return against(cost, worth)
}

// PlanCost is an account's plan over the days, or every account's: the plan,
// as the config names it, and its price a month; what it cost over the days
// and the worth against it; and the worth of the account's requests. Of an
// account whose plan isn't known, only the worth is given. Of every
// account's, All, the price, cost and worth are of the accounts whose plans
// are known, and only of theirs.
type PlanCost struct {
	Account string              `json:"account,omitempty"`
	All     bool                `json:"all,omitempty"`
	Plan    string              `json:"plan,omitempty"`
	Price   *ledger.Picodollars `json:"price,omitempty"`
	Against
	Worth
}

// plansOf are each configured account's plan over days, in the config's
// order, then every account's, each priced at its price on the day with the
// date first, the period's first, prorated as its share s of a month.
func plansOf(days []Day, p plans, first string, s share) []PlanCost {
	worths := accountWorths(days)
	costs := make([]PlanCost, 0, len(p.accounts)+1)
	all := PlanCost{All: true}
	var price ledger.Picodollars
	for _, a := range p.accounts {
		cost := PlanCost{Account: a.ID, Worth: worths[a.ID]}
		if plan, ok := p.of(a.ID); ok {
			monthly := plan.Price(first)
			cost.Plan, cost.Price = a.Plan, &monthly
			cost.Against = against(prorate(monthly, s), cost.amount())
			price += monthly
			all.add(cost.Worth)
		}
		costs = append(costs, cost)
	}
	if all.Against = p.against(worths, first, s); all.Cost != nil {
		all.Price = &price
	}
	return append(costs, all)
}

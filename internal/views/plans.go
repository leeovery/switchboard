package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
)

// monthPart is a part of a month, num over den: what a plan costs over a period,
// as a part of its monthly price.
type monthPart struct{ num, den int64 }

// The shares of a month Accounts and History price their periods at: a day
// 12/365 of a month, a week 12/52, 30 days or a calendar month a month, 12
// weeks twelve weeks', and a year, of 365 days or 366, 12 months.
var (
	aWeek       = monthPart{12, 52}
	aMonth      = monthPart{1, 1}
	twelveWeeks = monthPart{12 * 12, 52}
	aYear       = monthPart{12, 1}
)

// days is the share of a month n days are.
func days(n int) monthPart {
	return monthPart{12 * int64(n), 365}
}

// prorate returns the part s of monthly, a plan's monthly price, to the
// nearest picodollar.
func prorate(monthly ledger.Picodollars, s monthPart) ledger.Picodollars {
	whole, part := int64(monthly)/s.den, int64(monthly)%s.den
	return ledger.Picodollars(whole*s.num + (part*s.num+s.den/2)/s.den)
}

// askedPeriod is the period the days asked for from the day asked make,
// today being the day given, each as dayfile.Civil gives one, as its plans
// are priced: its last day, and its share of a month, whole. Where they make
// one of Accounts' periods, it's that period, to its end, as a period still
// running is priced whole, the longest where two begin on the one day, as
// this week does with today on the week's first, Accounts' own to begin
// with; else the days from asked to today, a day each.
func askedPeriod(asked, today time.Time, weekStarts time.Weekday) (last time.Time, whole monthPart) {
	for _, p := range slices.Backward(accountsPeriods) {
		if asked.Equal(p.first(today, weekStarts)) {
			return p.last(today, weekStarts), p.share()
		}
	}
	return today, days(int(today.Sub(asked)/(24*time.Hour)) + 1)
}

// priced is how a period's plans are priced: at its share of a month, at
// the prices of the day with the date at; or, where none of its days is
// counted, none, its plans' prices those of its first day.
type priced struct {
	share monthPart
	at    string
	none  bool
}

// pricedOver is how the plans over a period from the day with the date
// first to the one with the date last are priced, at uncut a month whole,
// the days counted beginning on the one with the date from: the later of
// the first of the days asked for and the first the ledger holds, as there's
// no worth for the days before the ledger began. Where they begin by its
// first day, it's whole, as of that day; where they begin part-way through
// it, its days from then to its end, a day each, as of then, so a period
// still running is priced to its end, and the one a young ledger began in
// from its first day; and where they begin after it, none.
func pricedOver(first, last, from string, uncut monthPart) priced {
	start, startOK := dayfile.Civil(from)
	end, endOK := dayfile.Civil(last)
	switch {
	case from <= first || !startOK || !endOK:
		return priced{share: uncut, at: first}
	case end.Before(start):
		return priced{share: days(0), at: first, none: true}
	}
	return priced{share: days(int(end.Sub(start)/(24*time.Hour)) + 1), at: from}
}

// weekOf is the first day of the week the day falls in, as dayfile.Civil
// gives one, weeks starting on the day given.
func weekOf(day time.Time, starts time.Weekday) time.Time {
	return day.AddDate(0, 0, -int((7+day.Weekday()-starts)%7))
}

// Against is what plans cost over a period, Cost, and the worth of their
// accounts' requests against it, Ratio: each left out where no plan is
// known, and Ratio where they cost nothing. Of several accounts' plans,
// Plans counts those known, the cost and the worth being of their accounts
// alone; of one account's own, it's none.
type Against struct {
	Cost  *ledger.Picodollars `json:"cost,omitempty"`
	Ratio *float64            `json:"against,omitempty"`
	Plans PlansKnown          `json:"plans,omitzero"`
}

// PlansKnown are how many accounts' plans are known, Known, of how many
// accounts there are, Of.
type PlansKnown struct {
	Known int `json:"known"`
	Of    int `json:"of"`
}

// Said says a ratio is over the plans known, where some accounts' plans
// aren't: "2 of 3 plans known"; "" where every one is, or none is, so there's
// no ratio to say it of.
func (k PlansKnown) Said() string {
	if short := k.SaidShort(); short != "" {
		return short + " known"
	}
	return ""
}

// SaidShort says it as Said does, short, for where that doesn't fit: "2 of
// 3 plans".
func (k PlansKnown) SaidShort() string {
	if k.Known == 0 || k.Known >= k.Of {
		return ""
	}
	return prose.Grouped(k.Known) + " of " + prose.Counted(k.Of, "plan")
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

// against returns what the accounts' plans cost over a period, as pr
// prices it, and the worth against it of the accounts whose plans are known
// alone, as worths, by account, hold it, counting those known: the count
// alone where pr prices none of the period.
func (p plans) against(worths map[string]Worth, pr priced) Against {
	var cost, worth ledger.Picodollars
	counted := PlansKnown{Of: len(p.accounts)}
	for _, a := range p.accounts {
		if plan, ok := p.of(a.ID); ok {
			cost += prorate(plan.Price(pr.at), pr.share)
			worth += worths[a.ID].amount()
			counted.Known++
		}
	}
	if counted.Known == 0 || pr.none {
		return Against{Plans: counted}
	}
	a := against(cost, worth)
	a.Plans = counted
	return a
}

// PlanCost is an account's plan over the days, or every account's: the plan,
// as the config names it, and its price a month; what it cost over the days
// and the worth against it; and the worth of the account's requests. Of an
// account whose plan isn't known, only the worth is given. Of every
// account's, All, the price, cost and worth are of the accounts whose plans
// are known, and only of theirs, as its Plans counts them.
type PlanCost struct {
	Account string              `json:"account,omitempty"`
	All     bool                `json:"all,omitempty"`
	Plan    string              `json:"plan,omitempty"`
	Price   *ledger.Picodollars `json:"price,omitempty"`
	Against
	Worth
}

// Monthly is what the plan costs a month, as PerMonth says a price: "" where
// its price isn't known.
func (p PlanCost) Monthly() string {
	if p.Price == nil {
		return ""
	}
	return PerMonth(*p.Price)
}

// PerMonth says a plan's price a month, to the dollar, its thousands set
// apart, as "$1,000/mo".
func PerMonth(price ledger.Picodollars) string {
	return price.GroupedDollars() + "/mo"
}

// plansOf are each configured account's plan over days, in the config's
// order, then every account's, each priced as pr prices the period: where
// it prices none of it, each plan and its price alone.
func plansOf(days []Day, p plans, pr priced) []PlanCost {
	worths := accountWorths(days)
	costs := make([]PlanCost, 0, len(p.accounts)+1)
	all := PlanCost{All: true, Against: p.against(worths, pr)}
	var price ledger.Picodollars
	known := false
	for _, a := range p.accounts {
		cost := PlanCost{Account: a.ID, Worth: worths[a.ID]}
		if plan, ok := p.of(a.ID); ok {
			monthly := plan.Price(pr.at)
			cost.Plan, cost.Price = a.Plan, &monthly
			if !pr.none {
				cost.Against = against(prorate(monthly, pr.share), cost.amount())
			}
			price, known = price+monthly, true
			all.add(cost.Worth)
		}
		costs = append(costs, cost)
	}
	if known {
		all.Price = &price
	}
	return append(costs, all)
}

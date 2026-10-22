package views

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// TokensAsk is what Tokens is asked to show: its rows' period.
type TokensAsk struct {
	Period TokensPeriod
}

// TokensPeriod is the period a row of Tokens is, as < and > step it: a day, a
// week, as it opens, or a month.
type TokensPeriod string

const (
	// TokensByWeek is a calendar week, from week_starts' day.
	TokensByWeek TokensPeriod = ""
	// TokensByDay is a local day.
	TokensByDay TokensPeriod = "day"
	// TokensByMonth is a calendar month.
	TokensByMonth TokensPeriod = "month"
)

// Name is what the heading and the key line call the period: "day", "week"
// or "month".
func (p TokensPeriod) Name() string {
	return cmp.Or(string(p), "week")
}

// TokensPage is Tokens as the dashboard draws it: its rows, a period each,
// newest first, those with nothing recorded in them left out; the date of
// the prices their worth is at; and what every plan configured costs over a
// whole period of the kind asked, at today's prices, the most a period's
// worth is set against, nil where no plan is known or they cost nothing.
type TokensPage struct {
	Periods    []TokenPeriod
	PricesAsOf string
	PeriodCost *ledger.Picodollars
}

// TokenPeriod is a row of Tokens: a period, by its first day's date, or by
// month, its month, as Month names it; how many model versions it used; its
// tokens and their worth, what the plans cost for it and the worth against
// it; then by account, in the config's order, each by model version too, and
// by model version, with the accounts it ran on. SoFar is set on the period
// today is in, still running.
type TokenPeriod struct {
	First    string
	Versions int
	Tokens
	Worth
	Against
	ByAccount []AccountTokens
	ByModel   []ModelTokens
	SoFar     bool
}

// tokens is Tokens' page, its rows the periods asked for over the last 12
// months' days, as the Year's are, the days its grid draws before them left
// out, the oldest priced from their first day, or the later day the days
// were asked for from, or the ledger began on, as tokenPeriodsOf says,
// newest first.
func (p Past) tokens(ask TokensAsk) TokensPage {
	by := ask.Period.shape(p.in.WeekStarts)
	today, _ := by.of(dayfile.DateOf(p.in.Now))
	span := yearSpanOf(0, dayfile.CivilOf(p.in.Now), p.in.WeekStarts)
	from := p.counted(max(span.first, dayfile.DateOf(p.in.From)))
	known := plans{accounts: p.in.Accounts, table: p.in.Prices}
	all := tokenPeriodsOf(between(p.days, from, span.last), known, versions{table: p.in.Prices, family: p.in.Family}, from, by)
	var periods []TokenPeriod
	for _, t := range slices.Backward(all) {
		if len(t.ByAccount) > 0 {
			t.SoFar = t.First == today
			periods = append(periods, t)
		}
	}
	whole := known.against(nil, priced{share: by.share, at: dayfile.DateOf(p.in.Now)})
	page := TokensPage{Periods: periods, PricesAsOf: p.in.Prices.AsOf}
	if whole.Ratio != nil {
		page.PeriodCost = whole.Cost
	}
	return page
}

// periodShape is how Tokens' rows split days into periods: of gives the key
// of the period the day with the given date falls in, as TokenPeriod's First
// names it, reporting false for a date that isn't one; dates the dates of a
// period's first and last days, by its key; and share is the share of a
// month a whole period costs its plans.
type periodShape struct {
	of    func(date string) (string, bool)
	dates func(key string) (first, last string)
	share monthPart
}

// key is the key of the period the day falls in, as spansOf takes it.
func (by periodShape) key(d Day) (string, bool) {
	return by.of(d.Day)
}

// shape is the period's shape, weeks starting on the day given: a day costs
// 12/365 of a month, a week 12/52, and a calendar month a month.
func (p TokensPeriod) shape(weekStarts time.Weekday) periodShape {
	switch p {
	case TokensByDay:
		return periodShape{
			of:    func(date string) (string, bool) { _, ok := dayfile.Civil(date); return date, ok },
			dates: func(key string) (string, string) { return key, key },
			share: days(1),
		}
	case TokensByMonth:
		return periodShape{
			of: monthOf,
			dates: func(key string) (string, string) {
				first, _ := dayfile.MonthStart(key)
				return first.Format(time.DateOnly), first.AddDate(0, 1, -1).Format(time.DateOnly)
			},
			share: aMonth,
		}
	}
	return periodShape{
		of: weekDate(weekStarts),
		dates: func(key string) (string, string) {
			first, _ := dayfile.Civil(key)
			return key, first.AddDate(0, 0, 6).Format(time.DateOnly)
		},
		share: aWeek,
	}
}

package views

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

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
// newest first, those with nothing recorded in them left out; of one of the
// dial's periods, as Accounts' Tokens shows it, the whole period's row,
// Total, nil where nothing is recorded in it; the date of the prices their
// worth is at; and what every plan configured costs over a whole period of
// the kind its rows are, at today's prices, the most a row's worth is set
// against, nil where no plan is known or they cost nothing.
type TokensPage struct {
	Periods    []TokenPeriod
	Total      *TokenPeriod
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

// TokensOver is Accounts' Tokens' page over the dial's period given: its
// rows the period's, as its Rows has them, counted from its first day, or
// the later day the ledger began on, as tokensFrom counts them; and its
// Total, the whole period's over the days its rows hold, so its tokens and
// worth are theirs summed, its versions those they used, each once, and
// against its price as Accounts' all accounts has it. Over today, one row
// would only repeat the total, so it has none.
func (p Past) TokensOver(period Period) TokensPage {
	today := dayfile.CivilOf(p.in.Now)
	span := period.spanAt(today, p.in.WeekStarts)
	from := p.counted(span.first).Format(time.DateOnly)
	page, held := p.tokensFrom(from, span.last.Format(time.DateOnly), period.length.Rows())
	known := plans{accounts: p.in.Accounts, table: p.in.Prices}
	whole := tokenPeriodsOf(held, known, p.in.names(), p.versions(), from, period.shape(today, p.in.WeekStarts))
	if len(whole) == 1 && len(whole[0].ByAccount) > 0 {
		whole[0].SoFar = true
		page.Total = &whole[0]
	}
	if period.length == LengthToday {
		page.Periods = nil
	}
	return page
}

// tokensFrom is a page of Tokens, its rows the periods given over the days
// from the one with the date from to the one with the date last, counted
// from the first of them, as tokenPeriodsOf counts them, newest first, those
// with nothing recorded in them left out, the one today is in so far; and
// the days its rows hold, oldest first, those of the rows left out gone.
func (p Past) tokensFrom(from, last string, rows TokensPeriod) (TokensPage, []Day) {
	by := rows.shape(p.in.WeekStarts)
	today, _ := by.of(dayfile.DateOf(p.in.Now))
	known := plans{accounts: p.in.Accounts, table: p.in.Prices}
	days := between(p.days, from, last)
	all := tokenPeriodsOf(days, known, p.in.names(), p.versions(), from, by)
	var periods []TokenPeriod
	shown := make(map[string]bool, len(all))
	for _, t := range slices.Backward(all) {
		if len(t.ByAccount) > 0 {
			t.SoFar = t.First == today
			periods = append(periods, t)
			shown[t.First] = true
		}
	}
	held := slices.DeleteFunc(slices.Clone(days), func(d Day) bool {
		key, ok := by.key(d)
		return !ok || !shown[key]
	})
	whole := known.against(nil, priced{share: by.share, at: dayfile.DateOf(p.in.Now)})
	page := TokensPage{Periods: periods, PricesAsOf: p.in.Prices.AsOf}
	if whole.Ratio != nil {
		page.PeriodCost = whole.Cost
	}
	return page, held
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

package views

import "cmp"

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
// newest first, and the date of the prices their worth is at.
type TokensPage struct {
	Periods    []TokenPeriod
	PricesAsOf string
}

// TokenPeriod is a row of Tokens: a period, by its first day's date, or by
// month, its month, as Month names it; how many model versions it used; its
// tokens and their worth, what the plans cost for it and the worth against
// it; then by account, in the config's order, each by model version too, and
// by model version, with the accounts it ran on.
type TokenPeriod struct {
	First    string
	Versions int
	Tokens
	Worth
	Against
	ByAccount []AccountTokens
	ByModel   []ModelTokens
}

package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Past is History's days, priced once as they're given, which each of its
// views, the Year, Weeks, Days and Tokens, is built of as a page.
type Past struct {
	in   HistoryInput
	days []Day
}

// NewPast is the Past of the days given, the summaries the ledger holds of a
// year and today, oldest first, priced at today's prices. The input gives the
// time now, the prices, the accounts, their weeks and the families: its
// Ledger and From go unread, as the days are given.
func NewPast(in HistoryInput, days []ledger.Summary) Past {
	today := dateOf(in.Now).Format(time.DateOnly)
	v := versions{table: in.Prices, family: in.Family}
	return Past{in: in, days: pricedDays(days, in.Prices, today, v)}
}

// Year is the Year's page, as asked for.
func (p Past) Year(ask YearAsk) YearPage {
	return p.year(ask)
}

// Weeks is Weeks' page: the eight weeks to this one.
func (p Past) Weeks() WeeksPage {
	return p.weeksPage()
}

// Days is Days' page: the eight weeks to this one, a day at a time.
func (p Past) Days() DaysPage {
	return DaysPage{}
}

// Tokens is Tokens' page, as asked for.
func (p Past) Tokens(TokensAsk) TokensPage {
	return TokensPage{}
}

package views

import (
	"slices"
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
// time now, the prices, the accounts, their weeks and the families, and From,
// the day the days were asked for from, for Tokens to price a period they
// begin part-way through by its days alone: its Ledger goes unread, as the
// days are given.
func NewPast(in HistoryInput, days []ledger.Summary) Past {
	today := dateOf(in.Now).Format(time.DateOnly)
	v := versions{table: in.Prices, family: in.Family}
	return Past{in: in, days: pricedDays(days, in.Prices, today, v)}
}

// Since is the Past at now, the days given, the summaries the ledger holds of
// the days from the first of them to today, oldest first, in place of those
// it held from that day on. Only they are priced, at now's day's prices, as
// they're given: the days held before them stand as they were priced, so a
// look that reads today alone prices today alone.
func (p Past) Since(days []ledger.Summary, now time.Time) Past {
	p.in.Now = now
	if len(days) == 0 {
		return p
	}
	today := dateOf(now).Format(time.DateOnly)
	v := versions{table: p.in.Prices, family: p.in.Family}
	before, _ := splitAt(p.days, days[0].Day)
	p.days = slices.Concat(before, pricedDays(days, p.in.Prices, today, v))
	return p
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
	return daysPageOf(p.in, p.days)
}

// Tokens is Tokens' page, as asked for.
func (p Past) Tokens(ask TokensAsk) TokensPage {
	return p.tokens(ask)
}

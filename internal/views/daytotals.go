package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// DayTotals are a day's figures, as the Log's day rule says them, as History
// reads the day's summary: its every account's totals over the day alone.
type DayTotals struct {
	// Date is the local day's date.
	Date string
	// Requests are its requests, as the ledger counts them; Sessions its
	// sessions, each once, however many accounts it ran on; and Limits the
	// limits its accounts reached.
	Requests, Sessions, Limits int
	// Worth is what its requests were worth, at today's prices.
	Worth
}

// TotalsByDay are the figures of each local day from from's to now's, oldest
// first, of the ledger's days as days reads them: a day's summary where the
// ledger holds one that stands, else, as of today, and of a day not
// summarised since lines came to be filed under it, the day summarised from
// its lines as it's read. Each is worked out as History's totals are, priced
// at the prices of now's day.
func TotalsByDay(days LedgerDays, from, now time.Time, prices ledger.Table) []DayTotals {
	return pricedTotals(days.Days(from), now, prices, func(string) bool { return true })
}

// TotalsOn are the figures of the local days with the dates given, oldest
// first, as TotalsByDay has them, the ledger's days read from the earliest,
// the days given alone priced: none of a date it holds no day of.
func TotalsOn(days LedgerDays, dates []string, now time.Time, prices ledger.Table) []DayTotals {
	if len(dates) == 0 {
		return nil
	}
	return pricedTotals(days.Days(dayOf(slices.Min(dates)).Start), now, prices, func(date string) bool { return slices.Contains(dates, date) })
}

// pricedTotals are the figures of the days summaries are of that given says,
// oldest first, as TotalsByDay has them.
func pricedTotals(summaries []ledger.Summary, now time.Time, prices ledger.Table, given func(date string) bool) []DayTotals {
	today, v := Today(now).Date, versions{table: prices}
	var totals []DayTotals
	for _, s := range summaries {
		if !given(s.Day) {
			continue
		}
		// With no account configured, every account's totals are the only ones.
		all := accountTotalsOf([]Day{{Priced: prices.Priced(s, today)}}, nil, v).Accounts[0]
		t := DayTotals{Date: s.Day, Requests: all.Requests, Sessions: all.Sessions, Worth: all.Worth}
		for _, n := range all.Limits {
			t.Limits += n
		}
		totals = append(totals, t)
	}
	return totals
}

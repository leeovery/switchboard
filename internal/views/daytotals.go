package views

import (
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

// LedgerDates reads the summaries of the request ledger's days by their
// dates, as a ledger.Follower does.
type LedgerDates interface {
	// DaysOn gives the summaries of the local days with the dates given,
	// oldest first, each once, as LedgerDays' Days gives them from the
	// earliest: none of a date before the first day the ledger holds or
	// after today's.
	DaysOn(dates []string) []ledger.Summary
}

// TotalsOn are the figures of the local days with the dates given, oldest
// first, of those days alone as days reads them: a day's summary where the
// ledger holds one that stands, else, as of today, and of a day not
// summarised since lines came to be filed under it, the day summarised from
// its lines as it's read; none of a date it holds no day of. Each is worked
// out as History's totals are, priced at the prices of now's day.
func TotalsOn(days LedgerDates, dates []string, now time.Time, prices ledger.Table) []DayTotals {
	if len(dates) == 0 {
		return nil
	}
	today, v := Today(now).Date, versions{table: prices}
	var totals []DayTotals
	for _, s := range days.DaysOn(dates) {
		// With no account configured, every account's totals are the only ones.
		all := accountTotalsOf(nil, []Day{{Priced: prices.Priced(s, today)}}, nil, v).Accounts[0]
		totals = append(totals, DayTotals{Date: s.Day, Requests: all.Requests, Sessions: all.Sessions, Limits: all.LimitCount(), Worth: all.Worth})
	}
	return totals
}

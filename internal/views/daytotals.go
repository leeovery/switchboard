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

// TotalsOn are the figures of the local days with the dates given, oldest
// first, of the ledger's days as days reads them from the earliest given: a
// day's summary where the ledger holds one that stands, else, as of today,
// and of a day not summarised since lines came to be filed under it, the day
// summarised from its lines as it's read; none of a date it holds no day
// of. Each is worked out as History's totals are, priced at the prices of
// now's day, the days given alone.
func TotalsOn(days LedgerDays, dates []string, now time.Time, prices ledger.Table) []DayTotals {
	if len(dates) == 0 {
		return nil
	}
	today, v := Today(now).Date, versions{table: prices}
	var totals []DayTotals
	for _, s := range days.Days(dayOf(slices.Min(dates)).Start) {
		if !slices.Contains(dates, s.Day) {
			continue
		}
		// With no account configured, every account's totals are the only ones.
		all := accountTotalsOf(nil, []Day{{Priced: prices.Priced(s, today)}}, nil, v).Accounts[0]
		totals = append(totals, DayTotals{Date: s.Day, Requests: all.Requests, Sessions: all.Sessions, Limits: all.LimitCount(), Worth: all.Worth})
	}
	return totals
}

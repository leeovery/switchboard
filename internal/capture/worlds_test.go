package capture

import (
	"cmp"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// The accounts' worlds at the frames' moment, and the ledger's days of the
// auto world's three accounts and of the ten, each built once for the tests,
// as building the year takes a while.
var (
	worldsNow = sync.OnceValue(func() accountWorlds { return newAccountWorlds(moment(time.Local)) })
	threeDays = sync.OnceValue(func() ledgerDays { return daysOf(worldsNow().auto()) })
	tenDays   = sync.OnceValue(func() ledgerDays { return daysOf(worldsNow().many(10)) })
)

// daysOf are the ledger's days of the world given.
func daysOf(w accountWorld) ledgerDays {
	return ledgerDays(worldsNow().year.summaries(w.history, w.accounts))
}

// ledgerDays reads a world's ledger days as a ledger.Reader does: the local
// days from the day asked for to yesterday, those it holds.
type ledgerDays []ledger.Summary

// Days gives the summaries of the local days from from's to today's.
func (l ledgerDays) Days(from time.Time) []ledger.Summary {
	date := from.Local().Format(time.DateOnly)
	i, _ := slices.BinarySearchFunc(l, date, func(s ledger.Summary, date string) int { return cmp.Compare(s.Day, date) })
	return slices.Clone(l[i:])
}

// historyFrom is the History the views build of the days given from day d of
// the year to yesterday, of the world's accounts, at the frames' prices, as
// History and Accounts read it: weeks from Monday, the accounts' weeks
// Claude's.
func historyFrom(days ledgerDays, w accountWorld, d int) views.History {
	now := worldsNow().now
	return views.NewHistory(views.HistoryInput{
		Ledger: days, From: dayStart(now, d), Now: now, Prices: drawnPrices, Accounts: w.accounts,
		WeekStarts: time.Monday, WeekWindow: claude.WeekWindow, Family: claude.Provider{}.Family,
	})
}

// dollars is a sum of money, as the ledger counts it, in whole dollars, as
// the frames draw worth.
func dollars(t *testing.T, w views.Worth) int {
	t.Helper()
	if w.Amount == nil || len(w.Unpriced) > 0 {
		t.Fatalf("worth %v isn't priced whole", w)
	}
	return int(math.Round(float64(*w.Amount) / 1e12))
}

// percent is a share in whole percent, as the frames draw it.
func percent(share float64) int {
	return int(math.Round(share * 100))
}

// tenths is a ratio to a tenth, as the frames draw against its price.
func tenths(ratio *float64) float64 {
	if ratio == nil {
		return math.NaN()
	}
	return math.Round(*ratio*10) / 10
}

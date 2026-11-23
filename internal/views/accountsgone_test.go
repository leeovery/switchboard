package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Seen from Default, an Accounts page counts the accounts no longer here
// that the period's days name, whose work its all accounts' totals count
// though no row is theirs: work, moved to Acme, and team, removed, each
// once, however many days name it; an account that did no work in the
// period, as one moved out before it began, none; one whose day of the
// period holds its limits alone, as LIMITS counts them; a past period, by
// its own days; and the requests none answered, which are no account's,
// none.
func TestAnAccountsPageCountsTheAccountsNoLongerHereItsDaysName(t *testing.T) {
	work := func(date string) ledger.Summary {
		return day(date, account("personal", sent("claude-opus-5-5", 2, inputUsage)), account("work", sent("claude-opus-5-5", 3, inputUsage)))
	}
	team := func(date string) ledger.Summary {
		return day(date, account("h7c2", sent("claude-opus-5-5", 4, inputUsage)), account("personal", sent("claude-opus-5-5", 2, inputUsage)))
	}
	both := day("2026-10-06", account("h7c2", sent("claude-opus-5-5", 4, inputUsage)), account("work", sent("claude-opus-5-5", 3, inputUsage)))
	teamLimited := day("2026-10-06", windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-06", "11:20")}}.of("h7c2"))
	unanswered := day("2026-10-06", account("", sent("claude-opus-5-5", 1, inputUsage)), account("personal", sent("claude-opus-5-5", 2, inputUsage)))
	tests := []struct {
		name   string
		days   []ledger.Summary
		length views.Length
		keys   string
		want   int
	}{
		{"none", []ledger.Summary{day("2026-10-06", account("personal", sent("claude-opus-5-5", 2, inputUsage)))}, views.LengthThisWeek, "", 0},
		{"one moved", []ledger.Summary{work("2026-10-06")}, views.LengthThisWeek, "", 1},
		{"one removed", []ledger.Summary{team("2026-10-06")}, views.LengthThisWeek, "", 1},
		{"one of each", []ledger.Summary{both}, views.LengthThisWeek, "", 2},
		{"each counted once over its days", []ledger.Summary{work("2026-10-05"), both, team("2026-10-07")}, views.LengthThisWeek, "", 2},
		{"moved out before the period began", []ledger.Summary{work("2026-09-20")}, views.LengthThisWeek, "", 0},
		{"moved out within a longer period", []ledger.Summary{work("2026-09-20")}, views.LengthThirtyDays, "", 1},
		{"a past period, by its own days", []ledger.Summary{work("2026-09-30"), team("2026-10-06")}, views.LengthThisWeek, "<", 1},
		{"a past period before their work", []ledger.Summary{work("2026-10-06")}, views.LengthThisWeek, "<", 0},
		{"a day of limits alone", []ledger.Summary{teamLimited}, views.LengthThisWeek, "", 1},
		{"the requests none answered", []ledger.Summary{unanswered}, views.LengthThisWeek, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysFrom("2026-08-01", tt.days...)
			in := seenFrom("b2n8", days)
			in.Removed = status.RemovedAccounts{removedTeam}
			period := pressed(t, tt.length, tt.keys, now, time.Monday, time.Time{})
			if got := views.NewPast(in, days).Accounts(period).NoLongerHere; got != tt.want {
				t.Errorf("the page counts %d accounts no longer here, want %d", got, tt.want)
			}
		})
	}
}

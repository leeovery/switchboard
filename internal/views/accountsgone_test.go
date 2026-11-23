package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
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
// its own days; the requests none answered, which are no account's, none;
// and an account moved back into the pool, or removed and configured
// again, none, as each has a row of its own again. The accounts removed are
// those the accounts record keeps that the config doesn't configure, as
// status.RemovedOf gives them.
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
	movedBack := config.Accounts{{ID: "acme1", Label: "acme-1", Pool: "q7mt"}, {ID: "work", Label: "work", Pool: "b2n8"}, {ID: "personal", Label: "personal", Pool: "b2n8"}}
	configuredAgain := append(slices.Clone(poolAccounts), config.Account{ID: "h7c2", Label: "team", Pool: "b2n8"})
	record := identity.Record{Accounts: []identity.Entry{{ID: "h7c2", Label: removedTeam.Label, Removed: removedTeam.Removed}}}
	tests := []struct {
		name       string
		configured config.Accounts
		days       []ledger.Summary
		length     views.Length
		keys       string
		want       int
	}{
		{name: "none", days: []ledger.Summary{day("2026-10-06", account("personal", sent("claude-opus-5-5", 2, inputUsage)))}},
		{name: "one moved", days: []ledger.Summary{work("2026-10-06")}, want: 1},
		{name: "one removed", days: []ledger.Summary{team("2026-10-06")}, want: 1},
		{name: "one of each", days: []ledger.Summary{both}, want: 2},
		{name: "each counted once over its days", days: []ledger.Summary{work("2026-10-05"), both, team("2026-10-07")}, want: 2},
		{name: "moved out before the period began", days: []ledger.Summary{work("2026-09-20")}},
		{name: "moved out within a longer period", days: []ledger.Summary{work("2026-09-20")}, length: views.LengthThirtyDays, want: 1},
		{name: "a past period, by its own days", days: []ledger.Summary{work("2026-09-30"), team("2026-10-06")}, keys: "<", want: 1},
		{name: "a past period before their work", days: []ledger.Summary{work("2026-10-06")}, keys: "<"},
		{name: "a day of limits alone", days: []ledger.Summary{teamLimited}, want: 1},
		{name: "the requests none answered", days: []ledger.Summary{unanswered}},
		{name: "moved back into the pool", configured: movedBack, days: []ledger.Summary{work("2026-10-06")}},
		{name: "removed and configured again", configured: configuredAgain, days: []ledger.Summary{team("2026-10-06")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysFrom("2026-08-01", tt.days...)
			configured := tt.configured
			if configured == nil {
				configured = poolAccounts
			}
			in := input(days)
			in.Accounts = configured.InPool("b2n8")
			in.Moved = status.MovedOf(movedPools, configured, "b2n8")
			in.Removed = status.RemovedOf(record, configured)
			period := pressed(t, tt.length, tt.keys, now, time.Monday, time.Time{})
			if got := views.NewPast(in, days).Accounts(period).NoLongerHere; got != tt.want {
				t.Errorf("the page counts %d accounts no longer here, want %d", got, tt.want)
			}
		})
	}
}

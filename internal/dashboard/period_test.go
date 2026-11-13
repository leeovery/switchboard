package dashboard

import (
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The dial's period reads back as it was kept, by its name, this week by
// none, 12 months by a name of its own; a period a build from before the
// dial was shared kept under Accounts reads back as the same period, where
// none is kept in its own place; and one that isn't a period, as a later
// build's, reads as this week.
func TestThePeriodKeptReadsBack(t *testing.T) {
	tests := []struct {
		name string
		kept theme.Prefs
		want views.Period
	}{
		{name: "none kept", want: views.PeriodThisWeek},
		{name: "today", kept: theme.Prefs{Period: "today"}, want: views.PeriodToday},
		{name: "the last 30 days", kept: theme.Prefs{Period: "30-days"}, want: views.PeriodThirtyDays},
		{name: "the last 12 weeks", kept: theme.Prefs{Period: "12-weeks"}, want: views.PeriodTwelveWeeks},
		{name: "the last 12 months", kept: theme.Prefs{Period: "12-months"}, want: views.PeriodTwelveMonths},
		{name: "today, as a build from before kept it", kept: theme.Prefs{Accounts: theme.AccountsPrefs{View: "detail", Period: "today"}}, want: views.PeriodToday},
		{name: "30 days, as a build from before kept it", kept: theme.Prefs{Accounts: theme.AccountsPrefs{Period: "30-days"}}, want: views.PeriodThirtyDays},
		{name: "12 weeks, as a build from before kept it", kept: theme.Prefs{Accounts: theme.AccountsPrefs{Period: "12-weeks"}}, want: views.PeriodTwelveWeeks},
		{name: "kept in its own place over a build's from before", kept: theme.Prefs{Period: "12-months", Accounts: theme.AccountsPrefs{Period: "today"}}, want: views.PeriodTwelveMonths},
		{name: "a later build's", kept: theme.Prefs{Period: "fortnight"}, want: views.PeriodThisWeek},
		{name: "a later build's under Accounts", kept: theme.Prefs{Accounts: theme.AccountsPrefs{Period: "30 days"}}, want: views.PeriodThisWeek},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PeriodKept(tt.kept); got != tt.want {
				t.Errorf("PeriodKept(%+v) = %q, want %q", tt.kept, got, tt.want)
			}
		})
	}
}

// Every period kept reads back as itself, 12 months among them, and this
// week too where a build from before kept another under Accounts, which the
// keeping drops; Accounts' sub-tab, kept beside it, stays.
func TestEveryPeriodKeptRoundTrips(t *testing.T) {
	for _, period := range views.Periods() {
		t.Run(period.Name(), func(t *testing.T) {
			prefs := theme.Prefs{Accounts: theme.AccountsPrefs{View: "detail", Period: "12-weeks"}}
			KeepPeriod(&prefs, period)
			if got := PeriodKept(prefs); got != period {
				t.Errorf("%q kept reads back as %q", period, got)
			}
			if prefs.Accounts != (theme.AccountsPrefs{View: "detail"}) {
				t.Errorf("keeping %q leaves Accounts' %+v, want its sub-tab alone", period, prefs.Accounts)
			}
		})
	}
}

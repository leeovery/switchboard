package dashboard

import (
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The dial's length reads back as it was kept, by its name, this week by
// none, 12 months by a name of its own; a length a build from before the
// dial was shared kept under Accounts reads back as the same length, where
// none is kept in its own place; and one that isn't a length, as a later
// build's, reads as this week.
func TestTheLengthKeptReadsBack(t *testing.T) {
	tests := []struct {
		name string
		kept theme.Prefs
		want views.Length
	}{
		{name: "none kept", want: views.LengthThisWeek},
		{name: "today", kept: theme.Prefs{Period: "today"}, want: views.LengthToday},
		{name: "the last 30 days", kept: theme.Prefs{Period: "30-days"}, want: views.LengthThirtyDays},
		{name: "the last 12 weeks", kept: theme.Prefs{Period: "12-weeks"}, want: views.LengthTwelveWeeks},
		{name: "the last 12 months", kept: theme.Prefs{Period: "12-months"}, want: views.LengthTwelveMonths},
		{name: "today, as a build from before kept it", kept: theme.Prefs{Accounts: theme.AccountsPrefs{View: "detail", Period: "today"}}, want: views.LengthToday},
		{name: "30 days, as a build from before kept it", kept: theme.Prefs{Accounts: theme.AccountsPrefs{Period: "30-days"}}, want: views.LengthThirtyDays},
		{name: "12 weeks, as a build from before kept it", kept: theme.Prefs{Accounts: theme.AccountsPrefs{Period: "12-weeks"}}, want: views.LengthTwelveWeeks},
		{name: "kept in its own place over a build's from before", kept: theme.Prefs{Period: "12-months", Accounts: theme.AccountsPrefs{Period: "today"}}, want: views.LengthTwelveMonths},
		{name: "a later build's", kept: theme.Prefs{Period: "fortnight"}, want: views.LengthThisWeek},
		{name: "a later build's under Accounts", kept: theme.Prefs{Accounts: theme.AccountsPrefs{Period: "30 days"}}, want: views.LengthThisWeek},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LengthKept(tt.kept); got != tt.want {
				t.Errorf("LengthKept(%+v) = %q, want %q", tt.kept, got, tt.want)
			}
		})
	}
}

// Every length kept reads back as itself, 12 months among them, and this
// week too where a build from before kept another under Accounts, which the
// keeping drops; Accounts' sub-tab, kept beside it, stays.
func TestEveryLengthKeptRoundTrips(t *testing.T) {
	for _, length := range views.Lengths() {
		t.Run(length.Name(), func(t *testing.T) {
			prefs := theme.Prefs{Accounts: theme.AccountsPrefs{View: "detail", Period: "12-weeks"}}
			KeepLength(&prefs, length)
			if got := LengthKept(prefs); got != length {
				t.Errorf("%q kept reads back as %q", length, got)
			}
			if prefs.Accounts != (theme.AccountsPrefs{View: "detail"}) {
				t.Errorf("keeping %q leaves Accounts' %+v, want its sub-tab alone", length, prefs.Accounts)
			}
		})
	}
}

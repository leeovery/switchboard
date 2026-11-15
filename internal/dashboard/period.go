package dashboard

import (
	"cmp"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// PeriodKept is the period the preferences kept, by its name: where they
// keep none, the one a build from before the dial was shared kept under
// Accounts. One that isn't a period, as a later build's, reads as this week.
func PeriodKept(kept theme.Prefs) views.Period {
	if period := views.Period(cmp.Or(kept.Period, kept.Accounts.Period)); period.Known() {
		return period
	}
	return views.PeriodThisWeek
}

// KeepPeriod has prefs keep period, as PeriodKept reads it back, in place of
// the one a build from before the dial was shared kept under Accounts.
func KeepPeriod(prefs *theme.Prefs, period views.Period) {
	prefs.Period, prefs.Accounts.Period = string(period), ""
}

// periodDial is the dial's period at a heading's right, by its name, and the
// widest name it can take, which the heading keeps room for as it steps.
func periodDial(period views.Period) (name, widest string) {
	return period.Name(), widestOf(views.PeriodNames())
}

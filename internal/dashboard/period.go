package dashboard

import (
	"cmp"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// LengthKept is the period's length the preferences kept: where they keep
// none, the one a build from before the dial was shared kept under Accounts.
// One that isn't a length, as a later build's, reads as this week.
func LengthKept(kept theme.Prefs) views.Length {
	if length := views.Length(cmp.Or(kept.Period, kept.Accounts.Period)); length.Known() {
		return length
	}
	return views.LengthThisWeek
}

// KeepLength has prefs keep the period's length given, as LengthKept reads
// it back, in place of the one a build from before the dial was shared kept
// under Accounts.
func KeepLength(prefs *theme.Prefs, length views.Length) {
	prefs.Period, prefs.Accounts.Period = string(length), ""
}

// periodDial is the dial's period at a heading's right, by its length's
// name, and the widest name it can take, which the heading keeps room for as
// it steps.
func periodDial(period views.Period) (name, widest string) {
	return period.Length().Name(), widestOf(views.LengthNames())
}

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

// dialed is the dial's period as a heading draws it at its right: its name;
// the widest name it keeps room for, so nothing moves the heading at now;
// whether it's gone, its name then in accent.attention, never read as now;
// and whether < or > does nothing, at the ledger's first day or at now,
// each then faint.
type dialed struct {
	name, widest   string
	gone           bool
	backOff, onOff bool
}

// periodDial is the dial's period given as a heading draws it, as dialed
// has it.
func periodDial(d views.Dial) dialed {
	return dialed{name: d.Name(), widest: d.Widest(), gone: d.Gone(), backOff: d.Earliest(), onOff: d.AtNow()}
}

// shown reports whether the heading has a dial to draw.
func (d dialed) shown() bool {
	return d.name != ""
}

// atWidest is the dial at the widest it takes, as the heading keeps room for
// it: its widest name, or its own where that's wider, as a period gone's may
// be.
func (d dialed) atWidest() dialed {
	d.name = widestOf([]string{d.widest, d.name})
	return d
}

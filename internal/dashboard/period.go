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

// dialed is the dial's period as a heading's readout draws it at its right:
// its range, which - and + change, and when it is, which < and > change;
// the range and when of the widest readout it keeps room for, so no step
// moves the heading; whether it's gone, when then in accent.attention,
// never read as now; and whether - or + does nothing, at the shortest
// range or the longest, and < or >, at the ledger's first day or at now,
// each then faint.
type dialed struct {
	ranged, when             string
	widestRanged, widestWhen string
	gone                     bool
	shorterOff, longerOff    bool
	backOff, onOff           bool
}

// periodDial is the dial's period given as a heading draws it, as dialed
// has it.
func periodDial(d views.Dial) dialed {
	widestRanged, widestWhen := d.Widest()
	return dialed{
		ranged: d.Range(), when: d.When(), widestRanged: widestRanged, widestWhen: widestWhen, gone: d.Gone(),
		shorterOff: d.Shortest(), longerOff: d.Longest(), backOff: d.Earliest(), onOff: d.AtNow(),
	}
}

// shown reports whether the heading has a dial to draw.
func (d dialed) shown() bool {
	return d.when != ""
}

// atWidest is the readout at the widest it keeps room for, as the heading
// keeps room for it.
func (d dialed) atWidest() dialed {
	d.ranged, d.when = d.widestRanged, d.widestWhen
	return d
}

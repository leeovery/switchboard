package dashboard

import "github.com/leeovery/switchboard/internal/theme"

// LogTicks are which of the Log's Requests' columns show, as the f panel
// ticks them. The zero value shows those shown at first: every column but
// the extras, worth and those after it before stopped.
type LogTicks struct {
	// toggled are the columns shown otherwise than at first, a bit each, so
	// the zero value shows those shown at first.
	toggled uint32
}

// logShownAtFirst are the columns shown at first, a bit each.
const logShownAtFirst = (1<<LogWorth - 1) | 1<<LogStopped | 1<<LogRouting

// Shows reports whether the column shows: at always does.
func (t LogTicks) Shows(c LogColumn) bool {
	return c == LogAt || (logShownAtFirst^t.toggled)&(1<<c) != 0
}

// Toggled is t with the column hidden where it shows, and shown where it's
// hidden: but at, which always shows.
func (t LogTicks) Toggled(c LogColumn) LogTicks {
	if c == LogAt {
		return t
	}
	return LogTicks{toggled: t.toggled ^ 1<<c}
}

// Count is how many columns show, at aside, as the panel's "in 11 columns"
// counts them, whatever the width lets show.
func (t LogTicks) Count() int {
	return len(t.shown()) - 1
}

// shown are the columns that show, in the table's order, at first.
func (t LogTicks) shown() []LogColumn {
	var shown []LogColumn
	for _, c := range EveryLogColumn() {
		if t.Shows(c) {
			shown = append(shown, c)
		}
	}
	return shown
}

// LogKept are the columns the preferences keep shown, by name: those shown
// at first, given none, as a file that keeps none has it; else those named,
// and at, a name of none of them left out, as a later switchboard's.
func LogKept(kept theme.LogPrefs) LogTicks {
	if kept.Columns == nil {
		return LogTicks{}
	}
	var shown uint32
	for _, c := range EveryLogColumn() {
		for _, name := range kept.Columns {
			if name == c.Name() {
				shown |= 1 << c
			}
		}
	}
	return LogTicks{toggled: (shown ^ logShownAtFirst) &^ (1 << LogAt)}
}

// Kept is what the preferences keep of the columns that show, as LogKept
// reads it: the names of those shown, at aside, in the table's order, never
// nil, so none kept as none; or nothing for those shown at first, so a later
// switchboard's own stand.
func (t LogTicks) Kept() theme.LogPrefs {
	if t == (LogTicks{}) {
		return theme.LogPrefs{}
	}
	names := []string{}
	for _, c := range t.shown()[1:] {
		names = append(names, c.Name())
	}
	return theme.LogPrefs{Columns: names}
}

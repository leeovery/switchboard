package dashboard

import (
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
)

// monthName is the month, as views names it, as History names it: Oct, or
// with its year, Oct 2025; as it is where it isn't a month.
func monthName(month string, withYear bool) string {
	first, ok := dayfile.MonthStart(month)
	if !ok {
		return month
	}
	return prose.Month(first, withYear)
}

// weekHead heads a week, as Weeks and Days do, by its first day's date, as
// 10 Aug: in text.faint, this week's bold text.secondary; as it is, cleaned,
// where it isn't a date.
func weekHead(date string, this bool) line {
	k := faintInk
	if this {
		k = strongInk
	}
	if day, ok := dayfile.Civil(date); ok {
		return line{{prose.DayMonth(day), k}}
	}
	return line{{prose.Clean(date), k}}
}

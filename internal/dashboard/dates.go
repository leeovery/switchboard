package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The dashboard's times and dates: a time to the second, as the title row's
// clock and the Log's lines give it, as 14:40:26; a month as views names it,
// as 2026-10; a week by its first day, as 24 Aug; and a day, as Wed 30 Sep.
const (
	secondsLayout = time.TimeOnly
	monthLayout   = "2006-01"
	weekLayout    = "2 Jan"
	dayLayout     = "Mon 2 Jan"
)

// civilDay is the day with the given date, reporting false for one that
// isn't.
func civilDay(date string) (time.Time, bool) {
	day, err := time.Parse(time.DateOnly, date)
	return day, err == nil
}

// monthName is the month, as views names it, as History names it: Oct, or
// with its year, Oct 2025; as it is where it isn't a month.
func monthName(month string, withYear bool) string {
	m, err := time.Parse(monthLayout, month)
	if err != nil {
		return month
	}
	if withYear {
		return m.Format("Jan 2006")
	}
	return m.Format("Jan")
}

// weekHead heads a week, as Weeks and Days do, by its first day's date, as
// 10 Aug: in text.faint, this week's bold text.secondary; as it is, cleaned,
// where it isn't a date.
func weekHead(date string, this bool) line {
	k := faintInk
	if this {
		k = strongInk
	}
	if day, ok := civilDay(date); ok {
		return line{{day.Format(weekLayout), k}}
	}
	return line{{status.Clean(date), k}}
}

// dayAbbreviation and dayInitial name a weekday as History does, wide and
// on a phone or over Days' week: Mon, M.
func dayAbbreviation(d time.Weekday) string { return d.String()[:3] }
func dayInitial(d time.Weekday) string      { return d.String()[:1] }

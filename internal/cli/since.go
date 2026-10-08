package cli

import (
	"fmt"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/redact"
)

// sinceForms says what --since takes.
const sinceForms = "a day, as 2026-10-01, a time today, as 14:00, or how long ago, as 3h or 2d"

// since returns when --since's value, given at now, starts, as startOf reads
// it. It fails for a value startOf can't read, and for one still to come.
func since(given string, now time.Time) (time.Time, error) {
	from, err := startOf(given, now)
	if err == nil && from.After(now) {
		return time.Time{}, fmt.Errorf("--since %s is still to come", given)
	}
	return from, err
}

// sinceOr returns when --since's value, given at now, starts, as since says:
// fallback where it's given none.
func sinceOr(given string, now, fallback time.Time) (time.Time, error) {
	if given == "" {
		return fallback, nil
	}
	return since(given, now)
}

// startOf returns when given starts at now: a local day, as 2026-10-01, at
// its start; a time of the day now falls on, in now's time zone, as 14:00,
// when the clocks first read it that day, or went forward over it, as
// dayfile.TimeOfDay says, so 00:00 is the day's first instant; or how long
// before now, more than none, a count of days, as 2d, a day being 24 hours,
// or a duration, as 3h or 90m. A count of more days than a duration holds,
// config.MostDays, is read as that many: a read of the ledger starts at its
// first day anyway. It fails for any other.
func startOf(given string, now time.Time) (time.Time, error) {
	if start, _, ok := dayfile.Day(given); ok {
		return start, nil
	}
	if at, err := time.Parse("15:04", given); err == nil {
		return dayfile.TimeOfDay(now, at.Hour(), at.Minute()), nil
	}
	if days, ok := config.ParseDays(given); ok && days > 0 {
		return now.Add(-time.Duration(min(days, config.MostDays)) * 24 * time.Hour), nil
	}
	if ago, err := time.ParseDuration(given); err == nil && ago > 0 {
		return now.Add(-ago), nil
	}
	return time.Time{}, fmt.Errorf("--since %q isn't %s", redact.Text(given), sinceForms)
}

// startOfDay returns when the local day days before the one now falls on
// began, as dayfile.DayStart says.
func startOfDay(now time.Time, days int) time.Time {
	return dayfile.DayStart(now.Local(), -days)
}

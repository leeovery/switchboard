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
// its start; a time of the local day now falls on, as 14:00; or how long
// before now, more than none, a count of days, as 2d, a day being 24 hours,
// config.MostDays at most, or a duration, as 3h or 90m. It fails for any
// other, saying plainly of a count of days past config.MostDays.
func startOf(given string, now time.Time) (time.Time, error) {
	if start, _, ok := dayfile.Day(given); ok {
		return start, nil
	}
	if at, err := time.Parse("15:04", given); err == nil {
		y, m, d := now.Local().Date()
		return time.Date(y, m, d, at.Hour(), at.Minute(), 0, 0, time.Local), nil
	}
	if days, ok := config.ParseDays(given); ok && days > 0 {
		if days > config.MostDays {
			return time.Time{}, fmt.Errorf("--since %s is more than %dd: give a day, as 2026-10-01, to start further back", given, config.MostDays)
		}
		return now.Add(-time.Duration(days) * 24 * time.Hour), nil
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

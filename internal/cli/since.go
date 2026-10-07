package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/redact"
)

// sinceForms says what --since takes.
const sinceForms = "a day, as 2026-10-01, a time today, as 14:00, or how long ago, as 3h or 2d"

// since returns when --since's value, given at now, starts: a local day, as
// 2026-10-01, at its start; a time of the local day now falls on, as 14:00;
// or how long before now, as 3h or 2d, a day being 24 hours. It fails for any
// other, and for one still to come.
func since(given string, now time.Time) (time.Time, error) {
	from, ok := startOf(given, now)
	switch {
	case !ok:
		return time.Time{}, fmt.Errorf("--since %q isn't %s", redact.Text(given), sinceForms)
	case from.After(now):
		return time.Time{}, fmt.Errorf("--since %s is still to come", given)
	}
	return from, nil
}

// sinceOr returns when --since's value, given at now, starts, as since says:
// fallback where it's given none.
func sinceOr(given string, now, fallback time.Time) (time.Time, error) {
	if given == "" {
		return fallback, nil
	}
	return since(given, now)
}

// startOf returns when given starts at now, as since reads it, reporting false
// for a value it doesn't read.
func startOf(given string, now time.Time) (time.Time, bool) {
	if start, _, ok := dayfile.Day(given); ok {
		return start, true
	}
	if at, err := time.Parse("15:04", given); err == nil {
		y, m, d := now.Local().Date()
		return time.Date(y, m, d, at.Hour(), at.Minute(), 0, 0, time.Local), true
	}
	if ago, ok := agoOf(given); ok {
		return now.Add(-ago), true
	}
	return time.Time{}, false
}

// agoOf returns how long ago given says, more than none: a whole number of
// days, as 2d, or a duration, as 3h or 90m.
func agoOf(given string) (time.Duration, bool) {
	if days, ok := strings.CutSuffix(given, "d"); ok {
		n, err := strconv.Atoi(days)
		return time.Duration(n) * 24 * time.Hour, err == nil && n > 0
	}
	ago, err := time.ParseDuration(given)
	return ago, err == nil && ago > 0
}

// startOfDay returns when the local day days before the one now falls on
// began, as dayfile.DayStart says.
func startOfDay(now time.Time, days int) time.Time {
	return dayfile.DayStart(now.Local(), -days)
}

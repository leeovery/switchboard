package capture

import (
	"math"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// The year's days, counted as the frames' generator counts them: day 0 is
// Monday 29 September 2025, the first day History's Year draws; the eight
// weeks Weeks and Days draw start on day 315, Monday 10 August 2026; this week
// on day 364, Monday 28 September; and today is day 367, Thursday 1 October.
const (
	eightWeeks = 315
	thisWeek   = 364
	today      = 367
)

// civil is day d of the year's date, wherever it's read: its midnight in UTC,
// which has every date's.
func civil(d int) time.Time {
	return time.Date(2025, time.September, 29+d, 0, 0, 0, 0, time.UTC)
}

// dayOf is the day of the year t falls on, in its time zone, counted by the
// calendar: a day the clocks change in is a day, however long.
func dayOf(t time.Time) int {
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Sub(civil(0)) / (24 * time.Hour))
}

// dayStart is when day d of the year starts in now's time zone: at its first
// instant, as clockOn says.
func dayStart(now time.Time, d int) time.Time {
	return dayAt(now, d, 0, 0, 0)
}

// dayAt is when the clocks in now's time zone first read the time of day
// given on day d of the year, as clockOn says.
func dayAt(now time.Time, d, hour, minute, second int) time.Time {
	return clockOn(civil(d), hour, minute, second, now.Location())
}

// clockOn is when the clocks in loc first read the time of day given on
// date's date, as dayfile.TimeOfDay says, or went forward over it: always on
// that date, where time.Date puts a midnight they go forward over on the day
// before.
func clockOn(date time.Time, hour, minute, second int, loc *time.Location) time.Time {
	y, m, d := date.Date()
	noon := time.Date(y, m, d, 12, 0, 0, 0, loc)
	return dayfile.TimeOfDay(noon, hour, minute).Add(time.Duration(second) * time.Second)
}

// weekdayWeights weigh a day's use by its day of the week, from Monday, as
// the frames' generator weighs them: weekdays busy, weekends light.
var weekdayWeights = [7]float64{1.0, 1.15, 1.1, 1.05, .8, .12, .06}

// empty reports whether nothing was used on day d of the year, as on a
// Saturday in five and a Sunday in three (acct12.worth_by_day).
func empty(d int) bool {
	return d%7 == 6 && d%3 == 0 || d%7 == 5 && d%5 == 0
}

// busy is how busy every account together was on day d of the year, as
// acct10.day_use shapes the days: growing through the year, weekdays busy and
// weekends light, and none on an empty day.
func busy(d int) float64 {
	if empty(d) {
		return 0
	}
	growth := .45 + .55*float64(d)/today
	return max(growth*weekdayWeights[d%7]*(1+.35*math.Sin(float64(d)*1.7)*math.Sin(float64(d)*.23)), 0)
}

// accountBusy is how busy the account with the given id was on day d of the
// year, against the rest of its week, as acct3.day_use shapes an account's
// days, its phase its id's: none on an empty day.
func accountBusy(id string, d int) float64 {
	if empty(d) {
		return 0
	}
	phase := 0
	for _, r := range id {
		phase += int(r)
	}
	return max(weekdayWeights[d%7]*(1+.35*math.Sin(float64(d-eightWeeks)*1.7+float64(phase%11))), 0)
}

// weekClock is a time of the week: a day of it and a time of that day.
type weekClock struct {
	day          time.Weekday
	hour, minute int
}

// weekResets are when each account's week resets, as the frames have it.
var weekResets = map[string]weekClock{
	"work": {time.Sunday, 21, 0}, "personal": {time.Sunday, 2, 0}, "side": {time.Monday, 10, 0},
	"client": {time.Sunday, 23, 0}, "spare": {time.Thursday, 18, 0}, "lab": {time.Monday, 8, 0},
	"team": {time.Saturday, 22, 0}, "extra": {time.Wednesday, 7, 0}, "ops": {time.Monday, 14, 0},
	"studio": {time.Sunday, 6, 0},
}

// weekReset is when the account with the given id's week resets the weeks
// given after it next does after now, or before, for weeks less than none: at
// its time of the week on that week's date, as clockOn says, each week's
// read afresh, so a time the clocks skip one week moves no other's.
func weekReset(id string, now time.Time, weeks int) time.Time {
	c := weekResets[id]
	y, m, d := now.Date()
	on := func(days int) time.Time {
		return clockOn(time.Date(y, m, d+days, 0, 0, 0, 0, time.UTC), c.hour, c.minute, 0, now.Location())
	}
	ahead := (int(c.day) - int(now.Weekday()) + 7) % 7
	if !on(ahead).After(now) {
		ahead += 7
	}
	return on(ahead + 7*weeks)
}

// peaks are how full each account's week got, the eight weeks Weeks draws,
// each the account's own week placed under the calendar week from 10 August
// to 28 September, the last still running, so far (acct3.PEAK, and acct4's
// and acct9's for the accounts of the frames of six and ten).
var peaks = map[string][8]float64{
	"work":     {72, 88, 100, 94, 100, 81, 100, 34},
	"personal": {64, 70, 58, 77, 69, 90, 85, 66},
	"side":     {22, 35, 41, 30, 52, 47, 60, 78},
	"client":   {40, 52, 48, 61, 55, 58, 63, 47},
	"spare":    {5, 8, 3, 10, 6, 4, 9, 5},
	"lab":      {60, 72, 70, 66, 81, 75, 79, 58},
	"team":     {80, 92, 100, 88, 100, 95, 100, 83},
	"extra":    {15, 22, 18, 30, 25, 19, 28, 12},
	"ops":      {30, 35, 28, 41, 33, 38, 36, 29},
	"studio":   {45, 50, 62, 48, 55, 58, 52, 41},
}

// fiveHourLimits are how many of its 5-hour limits each account reached in
// each of the eight weeks (acct3.LIMITS5, and acct4's and acct9's).
var fiveHourLimits = map[string][8]int{
	"work":     {3, 4, 6, 5, 7, 4, 6, 2},
	"personal": {1, 2, 0, 2, 1, 3, 2, 0},
	"side":     {0, 0, 1, 0, 1, 0, 1, 1},
	"client":   {0, 0, 0, 1, 0, 0, 1, 0},
	"lab":      {1, 2, 1, 1, 3, 2, 2, 1},
	"team":     {2, 3, 5, 3, 6, 4, 5, 2},
	"studio":   {0, 1, 0, 0, 1, 0, 0, 0},
}

// limitDay reports whether the account with the given id reached a 5-hour
// limit on day d of the year, of the eight weeks or the week before them, as
// acct3.limit_day spreads each week's limits over its weekdays: the week
// before taking the first's count.
func limitDay(id string, d int) bool {
	week, weekday := max(d-eightWeeks, 0)/7, d%7
	return weekday < 5 && (weekday*3+week)%5 < fiveHourLimits[id][week]
}

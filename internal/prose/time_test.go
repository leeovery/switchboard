package prose_test

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
)

func TestDatedIsNeverWiderThanItsWidthAndAnotherDaysFillsIt(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 5, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name string
		t    time.Time
		want int
	}{
		{name: "now's day", t: now.Add(-time.Hour), want: 5},
		{name: "the day before", t: now.Add(-10 * time.Hour), want: prose.DatedWidth()},
		{name: "a month before", t: now.AddDate(0, -1, 0), want: prose.DatedWidth()},
		{name: "the day after", t: now.Add(15 * time.Hour), want: prose.DatedWidth()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ansi.StringWidth(prose.Dated(now, tt.t)); got != tt.want {
				t.Errorf("Dated(%v, %v) = %q, %d characters, want %d", now, tt.t, prose.Dated(now, tt.t), got, tt.want)
			}
		})
	}
}

func TestNoDayIsWiderThanDayWidestAndADayOfTwoFiguresFillsIt(t *testing.T) {
	widest := 0
	for d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); d.Year() == 2026; d = d.AddDate(0, 0, 1) {
		widest = max(widest, utf8.RuneCountInString(prose.Day(d)))
	}
	if got := utf8.RuneCountInString(prose.DayWidest()); got != widest {
		t.Errorf("DayWidest() = %q, %d characters, want %d, the widest day of a year", prose.DayWidest(), got, widest)
	}
}

// Every time of day is said as wide as TimeOfDayWidest.
func TestATimeOfDayIsAsWideAsItsWidest(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 5, 0, 0, time.FixedZone("UTC+1", 60*60))
	widest := prose.TimeOfDayWidest()
	if widest != "23:59" {
		t.Errorf("TimeOfDayWidest = %q, want %q", widest, "23:59")
	}
	for minute := range 24 * 60 {
		if said := prose.TimeOfDay(now, now.Add(time.Duration(minute)*time.Minute)); len(said) != len(widest) {
			t.Errorf("TimeOfDay says %q, not as wide as %q", said, widest)
		}
	}
}

// No stay Stint says is wider than StintWidest, from starts across more than
// a year back to ends on their own day, the next, or any day after, dated
// or not; and the widest fills it.
func TestAStintIsNeverWiderThanItsWidest(t *testing.T) {
	now := time.Date(2027, 1, 3, 9, 5, 0, 0, time.FixedZone("UTC+1", 60*60))
	widest, filled := ansi.StringWidth(prose.StintWidest()), 0
	for back := range 400 {
		from := now.AddDate(0, 0, -back).Add(-time.Hour)
		for _, on := range []int{0, 1, 2, 7, back} {
			to := from.AddDate(0, 0, on).Add(30 * time.Minute)
			if to.After(now) {
				to = time.Time{}
			}
			for _, dated := range []bool{false, true} {
				said := prose.Stint(now, from, to, dated)
				n := ansi.StringWidth(said)
				filled = max(filled, n)
				if n > widest {
					t.Errorf("%q is %d cells, wider than StintWidest's %d", said, n, widest)
				}
			}
		}
	}
	if filled != widest {
		t.Errorf("the widest stay said is %d cells, want StintWidest's %d", filled, widest)
	}
}

func TestAStintGivesItsEndsTheirDaysWhereTheSessionSpansDays(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 31, 0, 0, time.FixedZone("UTC+1", 60*60))
	at := func(days, hour, minute int) time.Time {
		return time.Date(2026, 9, 28+days, hour, minute, 0, 0, now.Location())
	}
	tests := []struct {
		name     string
		from, to time.Time
		dated    bool
		want     string
	}{
		{name: "on a day", from: at(0, 12, 2), to: at(0, 14, 31), want: "12:02 – 14:31"},
		{name: "to now", from: at(0, 14, 31), want: "14:31 – now"},
		{name: "dated, both ends on one day", from: at(-1, 16, 40), to: at(-1, 18, 5), dated: true, want: "yesterday 16:40 – 18:05"},
		{name: "dated, today to now", from: at(0, 9, 12), dated: true, want: "today 09:12 – now"},
		{name: "dated, across days", from: at(-1, 23, 10), to: at(0, 0, 40), dated: true, want: "yesterday 23:10 – today 00:40"},
		{name: "dated, within the week", from: at(-3, 9, 0), to: at(-3, 10, 0), dated: true, want: "Fri 09:00 – 10:00"},
		{name: "in UTC, shown in now's time zone", from: at(0, 12, 2).UTC(), to: at(0, 14, 31).UTC(), want: "12:02 – 14:31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Stint(now, tt.from, tt.to, tt.dated); got != tt.want {
				t.Errorf("Stint() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A time after a start that names its own day is its time of day on the
// start's day, whichever day that is, and with its day on another.
func TestALaterTimeNamesItsDayOnlyOffItsStartsDay(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 31, 0, 0, time.FixedZone("UTC+1", 60*60))
	at := func(days, hour, minute int) time.Time {
		return time.Date(2026, 9, 28+days, hour, minute, 0, 0, now.Location())
	}
	tests := []struct {
		name         string
		start, later time.Time
		want         string
	}{
		{name: "both today", start: at(0, 12, 2), later: at(0, 14, 0), want: "14:00"},
		{name: "both yesterday", start: at(-1, 13, 20), later: at(-1, 16, 2), want: "16:02"},
		{name: "both within the week", start: at(-3, 9, 0), later: at(-3, 10, 0), want: "10:00"},
		{name: "started yesterday, today", start: at(-1, 23, 30), later: at(0, 0, 40), want: "today 00:40"},
		{name: "started before yesterday, yesterday", start: at(-3, 22, 0), later: at(-1, 1, 10), want: "yesterday 01:10"},
		{name: "a day in UTC that's another in now's time zone", start: at(0, 0, 30).UTC(), later: at(0, 1, 0).UTC(), want: "01:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Later(now, tt.start, tt.later); got != tt.want {
				t.Errorf("Later() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSameDayIsInNowsTimeZone(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 31, 0, 0, time.FixedZone("UTC+1", 60*60))
	late := time.Date(2026, 9, 28, 23, 30, 0, 0, time.UTC)
	if prose.SameDay(now, now, late) {
		t.Error("SameDay() of 23:30 UTC and now, both the 28th in UTC, = true, want false: in now's zone it's the 29th")
	}
	if !prose.SameDay(now, late, late.Add(time.Hour)) {
		t.Error("SameDay() of 23:30 and 00:30 UTC = false, want true: both the 29th in now's zone")
	}
}

func TestClock(t *testing.T) {
	utc := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name          string
		now, time     time.Time
		want, wantDay string
	}{
		{name: "afternoon", now: utc, time: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), want: "Mon 18:10", wantDay: "18:10"},
		{name: "just after midnight", now: utc, time: time.Date(2026, 10, 4, 0, 5, 0, 0, time.UTC), want: "Sun 00:05", wantDay: "00:05"},
		{
			name:    "in now's time zone",
			now:     utc.In(time.FixedZone("UTC-7", -7*60*60)),
			time:    time.Date(2026, 9, 29, 6, 30, 0, 0, time.UTC),
			want:    "Mon 23:30",
			wantDay: "23:30",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Clock(tt.now, tt.time); got != tt.want {
				t.Errorf("Clock(%v, %v) = %q, want %q", tt.now, tt.time, got, tt.want)
			}
			if got := prose.TimeOfDay(tt.now, tt.time); got != tt.wantDay {
				t.Errorf("TimeOfDay(%v, %v) = %q, want %q", tt.now, tt.time, got, tt.wantDay)
			}
		})
	}
}

// An hour of the day is named in two figures, as an axis of hours names it.
func TestHourNamed(t *testing.T) {
	for hour, want := range map[int]string{0: "00", 3: "03", 9: "09", 12: "12", 21: "21", 23: "23"} {
		if got := prose.HourNamed(hour); got != want {
			t.Errorf("HourNamed(%d) = %q, want %q", hour, got, want)
		}
	}
}

// A time to the second, and a reading's time, are in now's time zone.
func TestATimeToTheSecondAndAReadingsAreInNowsTimeZone(t *testing.T) {
	now := time.Date(2026, 10, 6, 22, 30, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name            string
		t               time.Time
		second, reading string
	}{
		{name: "in now's zone", t: time.Date(2026, 10, 6, 14, 40, 26, 0, now.Location()), second: "14:40:26", reading: "Tue 6 Oct 14:40"},
		{name: "in UTC", t: time.Date(2026, 10, 6, 21, 14, 5, 0, time.UTC), second: "22:14:05", reading: "Tue 6 Oct 22:14"},
		{name: "past midnight in now's zone", t: time.Date(2026, 10, 6, 23, 10, 0, 0, time.UTC), second: "00:10:00", reading: "Wed 7 Oct 00:10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.ToTheSecond(now, tt.t); got != tt.second {
				t.Errorf("ToTheSecond() = %q, want %q", got, tt.second)
			}
			if got := prose.Reading(now, tt.t); got != tt.reading {
				t.Errorf("Reading() = %q, want %q", got, tt.reading)
			}
		})
	}
}

// Brief is a time of day on now's day, in now's time zone, else its weekday
// alone.
func TestBriefIsATimeOfDayOnNowsDayElseItsWeekday(t *testing.T) {
	now := time.Date(2026, 10, 2, 14, 31, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "on now's day", t: time.Date(2026, 10, 2, 15, 54, 0, 0, now.Location()), want: "15:54"},
		{name: "on another", t: time.Date(2026, 10, 4, 15, 54, 0, 0, now.Location()), want: "Sun"},
		{name: "the day before in UTC, now's day in now's zone", t: time.Date(2026, 10, 1, 23, 30, 0, 0, time.UTC), want: "00:30"},
		{name: "now's day in UTC, the next in now's zone", t: time.Date(2026, 10, 2, 23, 30, 0, 0, time.UTC), want: "Sat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Brief(now, tt.t); got != tt.want {
				t.Errorf("Brief() = %q, want %q", got, tt.want)
			}
		})
	}
}

// BriefDated reads as Brief has it, but more than a day ahead on now's
// weekday, its date alone, as RunOut dates it there; a week back on now's
// weekday is Brief's weekday still.
func TestBriefDatedIsBriefButAWeekOnItsDate(t *testing.T) {
	zone := time.FixedZone("UTC+1", 60*60)
	now := time.Date(2026, 10, 1, 14, 42, 0, 0, zone)
	monthEnd := time.Date(2026, 10, 29, 9, 0, 0, 0, zone)
	tests := []struct {
		name   string
		now, t time.Time
		want   string
	}{
		{name: "later today", now: now, t: now.Add(81 * time.Minute), want: "16:03"},
		{name: "tomorrow, within a day", now: now, t: now.Add(20 * time.Hour), want: "Fri"},
		{name: "five days ahead", now: now, t: now.Add(5 * 24 * time.Hour), want: "Tue"},
		{name: "six days ahead, the day before now's weekday", now: now, t: now.Add(6 * 24 * time.Hour), want: "Wed"},
		{name: "six and a half days ahead, on now's weekday", now: now, t: now.Add(6*24*time.Hour + 12*time.Hour), want: "8 Oct"},
		{name: "a week back, on now's weekday", now: now, t: now.Add(-7 * 24 * time.Hour), want: "Thu"},
		{name: "across a month's turn, on now's weekday", now: monthEnd, t: monthEnd.Add(6*24*time.Hour + 18*time.Hour), want: "5 Nov"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.BriefDated(tt.now, tt.t); got != tt.want {
				t.Errorf("BriefDated(%v, %v) = %q, want %q", tt.now, tt.t, got, tt.want)
			}
		})
	}
}

// Befell shows when an event befell as Dated does, but a week back, on now's
// weekday, by its date, as its weekday alone would read as today's; never
// wider than BefellWidth, which a date of two figures fills.
func TestBefellIsDatedButAWeekBackItsDate(t *testing.T) {
	zone := time.FixedZone("UTC+1", 60*60)
	now := time.Date(2026, 11, 19, 16, 0, 0, 0, zone)
	earlier := time.Date(2026, 11, 12, 10, 0, 0, 0, zone)
	tests := []struct {
		name   string
		now, t time.Time
		want   string
	}{
		{name: "earlier today", now: now, t: now.Add(-time.Hour), want: "15:00"},
		{name: "yesterday", now: now, t: now.Add(-20 * time.Hour), want: "Wed 20:00"},
		{name: "six days back", now: now, t: now.Add(-6 * 24 * time.Hour), want: "Fri 16:00"},
		{name: "a week back, on now's weekday", now: now, t: now.Add(-6*24*time.Hour - 23*time.Hour), want: "12 Nov 17:00"},
		{name: "a week back, at now's time", now: now, t: now.Add(-7 * 24 * time.Hour), want: "12 Nov 16:00"},
		{name: "a week back, the day before now's weekday", now: now, t: now.Add(-7*24*time.Hour - 17*time.Hour), want: "Wed 23:00"},
		{name: "a week back, a date of one figure", now: earlier, t: earlier.Add(-7 * 24 * time.Hour), want: "5 Nov 10:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Befell(tt.now, tt.t); got != tt.want {
				t.Errorf("Befell(%v, %v) = %q, want %q", tt.now, tt.t, got, tt.want)
			}
		})
	}
	if got := utf8.RuneCountInString(prose.Befell(now, now.Add(-7*24*time.Hour))); got != prose.BefellWidth() {
		t.Errorf("a week back, a date of two figures takes %d characters, want BefellWidth, %d", got, prose.BefellWidth())
	}
}

// A day is named by the date it has in its own time zone, as a date parsed
// in UTC has it, however far west of UTC now is.
func TestADayIsNamedByTheDateItHasInItsOwnTimeZone(t *testing.T) {
	tests := []struct {
		name                                     string
		d                                        time.Time
		day, inYear, dayMonth, dayMonthYear      string
		month, monthYear, weekday, weekdayLetter string
		weekdayPair                              string
	}{
		{
			name: "a date parsed in UTC", d: time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC),
			day: "Tue 22 Sep", inYear: "Tue 22 Sep 2026", dayMonth: "22 Sep", dayMonthYear: "22 Sep 2026",
			month: "Sep", monthYear: "Sep 2026", weekday: "Tue", weekdayLetter: "T", weekdayPair: "Tu",
		},
		{
			name: "late in a zone west of UTC", d: time.Date(2026, 10, 7, 23, 30, 0, 0, time.FixedZone("UTC-7", -7*60*60)),
			day: "Wed 7 Oct", inYear: "Wed 7 Oct 2026", dayMonth: "7 Oct", dayMonthYear: "7 Oct 2026",
			month: "Oct", monthYear: "Oct 2026", weekday: "Wed", weekdayLetter: "W", weekdayPair: "We",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, got := range []struct{ form, got, want string }{
				{"Day", prose.Day(tt.d), tt.day},
				{"DayInYear", prose.DayInYear(tt.d), tt.inYear},
				{"DayMonth", prose.DayMonth(tt.d), tt.dayMonth},
				{"DayMonthYear", prose.DayMonthYear(tt.d), tt.dayMonthYear},
				{"Month", prose.Month(tt.d, false), tt.month},
				{"Month with its year", prose.Month(tt.d, true), tt.monthYear},
				{"Weekday", prose.Weekday(tt.d.Weekday()), tt.weekday},
				{"WeekdayInitial", prose.WeekdayInitial(tt.d.Weekday()), tt.weekdayLetter},
				{"WeekdayPair", prose.WeekdayPair(tt.d.Weekday()), tt.weekdayPair},
			} {
				if got.got != got.want {
					t.Errorf("%s() = %q, want %q", got.form, got.got, got.want)
				}
			}
		})
	}
}

// The weeks to a day are counted, then the day named.
func TestWeeksToCountsTheWeeksThenNamesTheDay(t *testing.T) {
	last := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		weeks int
		want  string
	}{
		{weeks: 4, want: "the 4 weeks to Wed 30 Sep"},
		{weeks: 2, want: "the 2 weeks to Wed 30 Sep"},
		{weeks: 1, want: "the 1 week to Wed 30 Sep"},
	}
	for _, tt := range tests {
		if got := prose.WeeksTo(tt.weeks, last); got != tt.want {
			t.Errorf("WeeksTo(%d) = %q, want %q", tt.weeks, got, tt.want)
		}
	}
}

func TestPastShowsMoreOfATimeTheFurtherBackItsDayIs(t *testing.T) {
	// Wednesday 7 October, 01:30, an hour ahead of UTC.
	now := time.Date(2026, 10, 7, 1, 30, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "today", t: now.Add(-time.Hour), want: "00:30"},
		{name: "yesterday, within a day", t: now.Add(-2 * time.Hour), want: "yesterday 23:30"},
		{name: "yesterday, in now's time zone", t: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), want: "yesterday 01:00"},
		{name: "within the week", t: time.Date(2026, 10, 1, 14, 8, 0, 0, time.UTC), want: "Thu 15:08"},
		{name: "a week back", t: time.Date(2026, 9, 30, 14, 8, 0, 0, time.UTC), want: "30 Sep 15:08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Past(now, tt.t); got != tt.want {
				t.Errorf("Past() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A run-out within a day of now reads as Dated has it; further ahead, its
// weekday alone, in now's time zone, but on now's weekday, which would read
// as today, its day as Day names it.
func TestARunOutReadsAsDatedWithinADayElseItsWeekdayButAWeekOnItsDay(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 42, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "later today", t: now.Add(81 * time.Minute), want: "16:03"},
		{name: "across midnight", t: now.Add(10 * time.Hour), want: "Fri 00:42"},
		{name: "exactly a day ahead", t: now.Add(24 * time.Hour), want: "Fri 14:42"},
		{name: "a minute over a day", t: now.Add(24*time.Hour + time.Minute), want: "Fri"},
		{name: "days ahead", t: now.Add(4 * 24 * time.Hour), want: "Mon"},
		{name: "five days ahead", t: now.Add(5 * 24 * time.Hour), want: "Tue"},
		{name: "six days ahead, the day before now's weekday", t: now.Add(6 * 24 * time.Hour), want: "Wed"},
		{name: "six and a half days ahead, on now's weekday, across the week's turn", t: now.Add(6*24*time.Hour + 12*time.Hour), want: "Thu 8 Oct"},
		{name: "a week ahead, on now's weekday", t: now.Add(7 * 24 * time.Hour), want: "Thu 8 Oct"},
		{name: "in UTC, on now's weekday in now's time zone alone", t: time.Date(2026, 10, 7, 23, 30, 0, 0, time.UTC), want: "Thu 8 Oct"},
		{name: "in UTC, within a day, tomorrow in now's time zone", t: time.Date(2026, 10, 1, 23, 30, 0, 0, time.UTC), want: "Fri 00:30"},
		{name: "in UTC, more than a day ahead, its day in now's time zone", t: time.Date(2026, 10, 2, 23, 30, 0, 0, time.UTC), want: "Sat"},
		{name: "before now, as Dated has it", t: now.Add(-15 * time.Hour), want: "Wed 23:42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.RunOut(now, tt.t); got != tt.want {
				t.Errorf("RunOut(%v, %v) = %q, want %q", now, tt.t, got, tt.want)
			}
		})
	}
}

// A run-out on now's weekday six days on, across a month's turn, is dated
// in the month it falls in.
func TestARunOutOnTodaysWeekdayAcrossAMonthsTurnReadsDated(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.FixedZone("UTC+1", 60*60))
	if got, want := prose.RunOut(now, now.Add(6*24*time.Hour+18*time.Hour)), "Thu 5 Nov"; got != want {
		t.Errorf("RunOut six and three quarter days on = %q, want %q", got, want)
	}
}

// No run-out RunOut says is wider than RunOutWidth, from a day back to a
// week on, and a dated one of two figures fills it.
func TestNoRunOutIsWiderThanRunOutWidthAndADatedOneFillsIt(t *testing.T) {
	now := time.Date(2026, 10, 8, 14, 42, 0, 0, time.FixedZone("UTC+1", 60*60))
	widest := 0
	for ahead := -24 * time.Hour; ahead <= 7*24*time.Hour; ahead += 10 * time.Minute {
		widest = max(widest, utf8.RuneCountInString(prose.RunOut(now, now.Add(ahead))))
	}
	if widest != prose.RunOutWidth() {
		t.Errorf("the widest run-out is %d characters, want RunOutWidth's %d", widest, prose.RunOutWidth())
	}
}

// A day is 24 hours gone, not the wall clock's: across London's fall back,
// 24h30m on reads as its day alone, though the clock reads 23h30m on.
func TestARunOutsDayIsTheHoursGoneAcrossAClockChange(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load Europe/London: %v", err)
	}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, london)
	tests := []struct {
		name  string
		ahead time.Duration
		want  string
	}{
		{name: "just inside a day gone", ahead: 23*time.Hour + 59*time.Minute, want: "Sun 10:59"},
		{name: "past a day gone, inside a day by the clock", ahead: 24*time.Hour + 30*time.Minute, want: "Sun"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.RunOut(now, now.Add(tt.ahead)); got != tt.want {
				t.Errorf("RunOut(%v, %v on) = %q, want %q", now, tt.ahead, got, tt.want)
			}
		})
	}
}

func TestDatedShowsTheDayOnAnyOtherDay(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 42, 7, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "later today", t: now.Add(81 * time.Minute), want: "16:03"},
		{name: "earlier today", t: now.Add(-30 * time.Minute), want: "14:12"},
		{name: "tomorrow, within a day", t: now.Add(13*time.Hour + 24*time.Minute), want: "Fri 04:06"},
		{name: "yesterday, within a day", t: now.Add(-15 * time.Hour), want: "Wed 23:42"},
		{name: "in UTC, today in now's time zone", t: time.Date(2026, 10, 1, 22, 30, 0, 0, time.UTC), want: "23:30"},
		{name: "in UTC, tomorrow in now's time zone", t: time.Date(2026, 10, 1, 23, 30, 0, 0, time.UTC), want: "Fri 00:30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Dated(now, tt.t); got != tt.want {
				t.Errorf("Dated() = %q, want %q", got, tt.want)
			}
		})
	}
}

// No time Past says is wider than PastWidest says it, nor Later than
// LaterWidest, and the day before's fills PastWidest, from now's day to more
// than a year back, across a turn of the year; on its start's day, Later is
// as wide as LaterWidest at any clock.
func TestPastWidestIsAsWideAsPastOrLaterEverSays(t *testing.T) {
	now := time.Date(2027, 1, 3, 9, 5, 0, 0, time.FixedZone("UTC+1", 60*60))
	widest := ansi.StringWidth(prose.PastWidest(now, now))
	width := ansi.StringWidth
	for back := range 400 {
		at := now.AddDate(0, 0, -back).Add(-time.Minute)
		if said := prose.Past(now, at); width(said) > widest {
			t.Errorf("%d days back, %q is %d characters, wider than PastWidest's %d", back, said, width(said), widest)
		}
		for _, start := range []time.Time{at.AddDate(0, 0, -1), at.Add(-time.Minute)} {
			said, most := prose.Later(now, start, at), prose.LaterWidest(now, start, at)
			if width(said) > width(most) {
				t.Errorf("%d days back, from %v, Later says %q, wider than LaterWidest's %q", back, start, said, most)
			}
			if prose.SameDay(now, start, at) && width(said) != width(most) {
				t.Errorf("%d days back, on its start's day, Later says %q, want it as wide as LaterWidest's %q", back, said, most)
			}
		}
	}
	if said := prose.Past(now, now.AddDate(0, 0, -1)); ansi.StringWidth(said) != widest {
		t.Errorf("the day before is said %q, want it as wide as PastWidest's %d", said, widest)
	}
}

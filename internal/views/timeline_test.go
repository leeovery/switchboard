package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// sameZoneNow is when the timelines are drawn in a zone an hour east of UTC:
// 13:12 on Monday 28 September 2026.
var sameZoneNow = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

// clockOn is a time by sameZoneNow's clock, on its day.
func clockOn(hour, minute int) time.Time {
	return time.Date(2026, 9, 28, hour, minute, 0, 0, sameZoneNow.Location())
}

// loadZone is the time zone with the given name, as the tests' clocks change in.
func loadZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

// civil is the day with the given date, as dayfile.Civil gives one.
func civil(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestTheLiveTimelineOfEachWindow(t *testing.T) {
	tests := []struct {
		name    string
		window  views.RunwayWindow
		columns int
		start   time.Time
		step    time.Duration
		// wantNow is the column now falls in.
		wantNow int
	}{
		{name: "the 5-hour at 160 columns: from the hour before now, on its ten minutes, a column every 10", columns: 136, start: clockOn(12, 10), step: 10 * time.Minute, wantNow: 6},
		{name: "the 5-hour narrower: more minutes a column, spanning as long", columns: 68, start: clockOn(12, 10), step: 20 * time.Minute, wantNow: 3},
		{name: "the 5-hour wider: fewer", columns: 170, start: clockOn(12, 10), step: 8 * time.Minute, wantNow: 7},
		{name: "the week: from a day before now, on its ten minutes, its 10,080 minutes shared", window: views.RunwayWeek, columns: 136, start: clockOn(13, 10).AddDate(0, 0, -1), step: 7 * 24 * time.Hour / 136, wantNow: 19},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := views.LiveTimeline(tt.window, sameZoneNow, tt.columns)
			if !tl.Start.Equal(tt.start) || tl.Step() != tt.step || tl.Columns != tt.columns || !tl.Now.Equal(sameZoneNow) {
				t.Errorf("LiveTimeline(%q) = from %v, a column every %v across %d, now %v; want from %v, every %v, now %v", tt.window, tl.Start, tl.Step(), tl.Columns, tl.Now, tt.start, tt.step, sameZoneNow)
			}
			if got := tl.NowColumn(); got != tt.wantNow {
				t.Errorf("now falls in column %d, want %d", got, tt.wantNow)
			}
		})
	}
}

func TestTheFramesNowFallsInItsColumn(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 42, 7, 0, time.Local)
	tests := []struct {
		name    string
		window  views.RunwayWindow
		columns int
		want    int
	}{
		{name: "runway-5-hour-160, its lanes from column 24", columns: 136, want: 30 - 24},
		{name: "runway-week-160", window: views.RunwayWeek, columns: 136, want: 43 - 24},
		{name: "runway-phone-52, its lanes from column 14", columns: 38, want: 15 - 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.LiveTimeline(tt.window, now, tt.columns).NowColumn(); got != tt.want {
				t.Errorf("now falls in column %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTheLiveTimelineStartsOnTheTenMinutesOfItsClock(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			// Gaza's clocks went back from 01:00 to 00:00 on 24 October 2020:
			// now is 00:25 the second time, so the hour before it is 00:25 the
			// first.
			name: "where midnight came twice", now: time.Date(2020, 10, 23, 22, 25, 0, 0, time.UTC).In(loadZone(t, "Asia/Gaza")),
			want: time.Date(2020, 10, 23, 21, 20, 0, 0, time.UTC),
		},
		{
			name: "a quarter of an hour off UTC's", now: time.Date(2026, 10, 1, 14, 42, 7, 0, loadZone(t, "Asia/Kathmandu")),
			want: time.Date(2026, 10, 1, 13, 40, 0, 0, loadZone(t, "Asia/Kathmandu")),
		},
		{
			name: "half an hour off UTC's", now: time.Date(2026, 10, 1, 14, 42, 7, 0, loadZone(t, "Asia/Kolkata")),
			want: time.Date(2026, 10, 1, 13, 40, 0, 0, loadZone(t, "Asia/Kolkata")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.LiveTimeline(views.RunwayFiveHour, tt.now, 136).Start; !got.Equal(tt.want) {
				t.Errorf("the 5-hour's timeline starts at %v, want %v: the hour before now, taken back to its clock's ten minutes", got, tt.want)
			}
		})
	}
}

func TestATimelinesColumnsStandForTheirMiddlesButNowsForNow(t *testing.T) {
	tl := views.LiveTimeline(views.RunwayFiveHour, sameZoneNow, 136)
	for _, tt := range []struct {
		col  int
		want time.Time
	}{
		{col: 0, want: clockOn(12, 15)},
		{col: 5, want: clockOn(13, 5)},
		{col: 6, want: sameZoneNow},
		{col: 7, want: clockOn(13, 25)},
	} {
		if got := tl.Moment(tt.col); !got.Equal(tt.want) {
			t.Errorf("Moment(%d) = %v, want %v", tt.col, got, tt.want)
		}
	}
	for _, tt := range []struct {
		at   time.Time
		want int
	}{
		{at: clockOn(12, 10), want: 0},
		{at: clockOn(12, 9), want: -1},
		{at: clockOn(14, 42), want: 15},
		{at: clockOn(12, 10).Add(fiveHourSpan - time.Nanosecond), want: 135},
		{at: clockOn(12, 10).Add(fiveHourSpan), want: 136},
	} {
		if got := tl.Column(tt.at); got != tt.want {
			t.Errorf("Column(%v) = %d, want %d", tt.at, got, tt.want)
		}
	}
}

// fiveHourSpan is how long the 5-hour view spans.
const fiveHourSpan = 22*time.Hour + 40*time.Minute

// The columns cut the span whole, each where the last ends, the first from
// its start and the last to its end, none more than a nanosecond longer
// than another; and each instant falls in the column whose bounds hold it.
func TestATimelinesColumnsCutItsSpanEvenlyToTheNanosecond(t *testing.T) {
	for _, tt := range []struct {
		name    string
		window  views.RunwayWindow
		columns int
	}{
		{name: "the 5-hour at 136", columns: 136},
		{name: "the 5-hour at 98", columns: 98},
		{name: "the week at 136", window: views.RunwayWeek, columns: 136},
		{name: "the week at 76", window: views.RunwayWeek, columns: 76},
		{name: "the week at 38", window: views.RunwayWeek, columns: 38},
		{name: "the week at 7", window: views.RunwayWeek, columns: 7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tl := views.LiveTimeline(tt.window, sameZoneNow, tt.columns)
			step := tl.Step()
			end := tl.Start
			for i := range tl.Columns {
				from, to := tl.Bounds(i)
				if !from.Equal(end) {
					t.Fatalf("column %d begins at %v, want %v, where the one before ends", i, from, end)
				}
				if d := to.Sub(from); d < step || d > step+time.Nanosecond {
					t.Errorf("column %d is %v long, want %v, or a nanosecond more", i, d, step)
				}
				if got := [3]int{tl.Column(from), tl.Column(from.Add(to.Sub(from) / 2)), tl.Column(to.Add(-time.Nanosecond))}; got != [3]int{i, i, i} {
					t.Errorf("column %d's first instant, middle and last fall in columns %v", i, got)
				}
				end = to
			}
			if !end.Equal(tl.End) {
				t.Errorf("the last column ends at %v, want %v, the span's end", end, tl.End)
			}
		})
	}
}

func TestADaySteppedBackToRunsFromItsMidnightToTheNext(t *testing.T) {
	london := loadZone(t, "Europe/London")
	tests := []struct {
		name       string
		day        time.Time
		now        time.Time
		start, end time.Time
		step       time.Duration
		// holdsNow is whether the timeline holds now, picked out.
		holdsNow bool
	}{
		{
			name: "a day gone: 24 hours, no now", day: civil(2026, time.September, 30), now: time.Date(2026, 10, 1, 14, 42, 7, 0, london),
			start: time.Date(2026, 9, 30, 0, 0, 0, 0, london), end: time.Date(2026, 10, 1, 0, 0, 0, 0, london), step: 24 * time.Hour / 136,
		},
		{
			name: "today, now picked out", day: civil(2026, time.October, 1), now: time.Date(2026, 10, 1, 14, 42, 7, 0, london),
			start: time.Date(2026, 10, 1, 0, 0, 0, 0, london), end: time.Date(2026, 10, 2, 0, 0, 0, 0, london), step: 24 * time.Hour / 136, holdsNow: true,
		},
		{
			name: "today at a second to midnight", day: civil(2026, time.September, 30), now: time.Date(2026, 9, 30, 23, 59, 59, 0, london),
			start: time.Date(2026, 9, 30, 0, 0, 0, 0, london), end: time.Date(2026, 10, 1, 0, 0, 0, 0, london), step: 24 * time.Hour / 136, holdsNow: true,
		},
		{
			name: "the same day a second after midnight, now a day gone", day: civil(2026, time.September, 30), now: time.Date(2026, 10, 1, 0, 0, 1, 0, london),
			start: time.Date(2026, 9, 30, 0, 0, 0, 0, london), end: time.Date(2026, 10, 1, 0, 0, 0, 0, london), step: 24 * time.Hour / 136,
		},
		{
			name: "a 23-hour day, the clocks going forward", day: civil(2026, time.March, 29), now: time.Date(2026, 4, 1, 12, 0, 0, 0, london),
			start: time.Date(2026, 3, 29, 0, 0, 0, 0, london), end: time.Date(2026, 3, 30, 0, 0, 0, 0, london), step: 23 * time.Hour / 136,
		},
		{
			name: "a 25-hour day, the clocks going back", day: civil(2026, time.October, 25), now: time.Date(2026, 10, 28, 12, 0, 0, 0, london),
			start: time.Date(2026, 10, 25, 0, 0, 0, 0, london), end: time.Date(2026, 10, 26, 0, 0, 0, 0, london), step: 25 * time.Hour / 136,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := views.Stepped(views.RunwayFiveHour, tt.day, tt.now, time.Monday, 136)
			if !tl.Start.Equal(tt.start) || !tl.End.Equal(tt.end) || tl.Step() != tt.step {
				t.Errorf("Stepped(%v) = from %v to %v, a column every %v; want from %v to %v, every %v", tt.day, tl.Start, tl.End, tl.Step(), tt.start, tt.end, tt.step)
			}
			if got := !tl.Now.IsZero(); got != tt.holdsNow || got && !tl.Now.Equal(tt.now) {
				t.Errorf("it holds now %v, want it held: %v", tl.Now, tt.holdsNow)
			}
			if first, last, ok := tl.Named(); !ok || !first.Equal(tt.day) || !last.Equal(tt.day) {
				t.Errorf("it's named %v to %v, %v, want %v alone", first, last, ok, tt.day)
			}
		})
	}
}

func TestAWeekSteppedBackToRunsFromItsFirstDayForSeven(t *testing.T) {
	london := loadZone(t, "Europe/London")
	thursday := time.Date(2026, 10, 1, 14, 42, 7, 0, london)
	tests := []struct {
		name        string
		day         time.Time
		starts      time.Weekday
		now         time.Time
		first, last time.Time
		end         time.Time
		holdsNow    bool
	}{
		{
			name: "this week, from Monday, now picked out", day: civil(2026, time.October, 1), starts: time.Monday, now: thursday,
			first: civil(2026, time.September, 28), last: civil(2026, time.October, 4), end: time.Date(2026, 10, 5, 0, 0, 0, 0, london), holdsNow: true,
		},
		{
			name: "this week, from Sunday", day: civil(2026, time.October, 1), starts: time.Sunday, now: thursday,
			first: civil(2026, time.September, 27), last: civil(2026, time.October, 3), end: time.Date(2026, 10, 4, 0, 0, 0, 0, london), holdsNow: true,
		},
		{
			name: "a week gone, by any of its days", day: civil(2026, time.September, 26), starts: time.Monday, now: thursday,
			first: civil(2026, time.September, 21), last: civil(2026, time.September, 27), end: time.Date(2026, 9, 28, 0, 0, 0, 0, london),
		},
		{
			name: "a week the clocks went back in: 169 hours", day: civil(2026, time.October, 21), starts: time.Monday, now: time.Date(2026, 11, 2, 9, 0, 0, 0, london),
			first: civil(2026, time.October, 19), last: civil(2026, time.October, 25), end: time.Date(2026, 10, 26, 0, 0, 0, 0, london),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := views.Stepped(views.RunwayWeek, tt.day, tt.now, tt.starts, 136)
			start := time.Date(tt.first.Year(), tt.first.Month(), tt.first.Day(), 0, 0, 0, 0, london)
			if !tl.Start.Equal(start) || !tl.End.Equal(tt.end) {
				t.Errorf("Stepped(week, %v) = from %v to %v, want from %v to %v", tt.day, tl.Start, tl.End, start, tt.end)
			}
			if first, last, ok := tl.Named(); !ok || !first.Equal(tt.first) || !last.Equal(tt.last) {
				t.Errorf("it's named %v to %v, %v, want %v to %v", first, last, ok, tt.first, tt.last)
			}
			if got := !tl.Now.IsZero(); got != tt.holdsNow {
				t.Errorf("it holds now %v, want it held: %v", tl.Now, tt.holdsNow)
			}
		})
	}
}

func TestALiveTimelineIsNamedByItsWindowAlone(t *testing.T) {
	for _, tl := range []views.Timeline{
		views.LiveTimeline(views.RunwayWeek, sameZoneNow, 136),
		views.Stepped(views.RunwayFiveHour, time.Time{}, sameZoneNow, time.Monday, 136),
	} {
		if first, last, ok := tl.Named(); ok {
			t.Errorf("a live timeline from %v is named %v to %v, want it unnamed", tl.Start, first, last)
		}
	}
	if got, want := views.Stepped(views.RunwayFiveHour, time.Time{}, sameZoneNow, time.Monday, 136), views.LiveTimeline(views.RunwayFiveHour, sameZoneNow, 136); got != want {
		t.Errorf("stepped back to no day, the timeline is %+v, want the live view's, %+v", got, want)
	}
}

func TestTheAxisTicksTheDaysWhereTheClocksPutTheirMidnights(t *testing.T) {
	tests := []struct {
		name string
		zone string
		day  time.Time
		// want are the starts of the days ticked, by the clock in UTC.
		want []time.Time
	}{
		{
			name: "a week of 24-hour days", zone: "Europe/London", day: civil(2026, time.September, 30),
			want: []time.Time{
				time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC), time.Date(2026, 9, 28, 23, 0, 0, 0, time.UTC), time.Date(2026, 9, 29, 23, 0, 0, 0, time.UTC),
				time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 23, 0, 0, 0, time.UTC), time.Date(2026, 10, 2, 23, 0, 0, 0, time.UTC),
				time.Date(2026, 10, 3, 23, 0, 0, 0, time.UTC),
			},
		},
		{
			// Chile's clocks went from 00:00 to 01:00 on Sunday 6 September
			// 2026: that day starts at 01:00.
			name: "the clocks going forward at midnight", zone: "America/Santiago", day: civil(2026, time.September, 2),
			want: []time.Time{
				time.Date(2026, 8, 31, 4, 0, 0, 0, time.UTC), time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC), time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC),
				time.Date(2026, 9, 3, 4, 0, 0, 0, time.UTC), time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC), time.Date(2026, 9, 5, 4, 0, 0, 0, time.UTC),
				time.Date(2026, 9, 6, 4, 0, 0, 0, time.UTC),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 11, 2, 9, 0, 0, 0, loadZone(t, tt.zone))
			got := views.Stepped(views.RunwayWeek, tt.day, now, time.Monday, 136).Midnights()
			if !instantsEqual(got, tt.want) {
				t.Errorf("Midnights() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTheLiveWeeksAxisTicksTheDaysThatBeginInIt(t *testing.T) {
	london := loadZone(t, "Europe/London")
	tl := views.LiveTimeline(views.RunwayWeek, time.Date(2026, 10, 1, 14, 42, 7, 0, london), 136)
	var want []time.Time
	for day := 1; day <= 7; day++ {
		want = append(want, time.Date(2026, 10, day, 0, 0, 0, 0, london))
	}
	if got := tl.Midnights(); !instantsEqual(got, want) {
		t.Errorf("from %v, Midnights() = %v, want Thursday's to Wednesday's, %v: none before it starts", tl.Start, got, want)
	}
}

func TestTheAxisTicksTheHoursTheClocksRead(t *testing.T) {
	london := loadZone(t, "Europe/London")
	tests := []struct {
		name  string
		day   time.Time
		every int
		// want are the hours ticked, by the clock in London, and wantCount
		// how many.
		want      []int
		wantCount int
	}{
		{name: "every other hour of a day", day: civil(2026, time.September, 30), every: 2, want: []int{2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22}, wantCount: 11},
		{name: "every sixth", day: civil(2026, time.September, 30), every: 6, want: []int{6, 12, 18}, wantCount: 3},
		{name: "a 23-hour day, never 01:00, which the clocks went forward over", day: civil(2026, time.March, 29), every: 1, wantCount: 22},
		{name: "a 25-hour day, 01:00 once, where the clocks first read it", day: civil(2026, time.October, 25), every: 1, wantCount: 23},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hours := views.Stepped(views.RunwayFiveHour, tt.day, time.Date(2026, 11, 2, 9, 0, 0, 0, london), time.Monday, 136).Hours(tt.every)
			if len(hours) != tt.wantCount {
				t.Errorf("Hours(%d) = %v, want %d of them", tt.every, hours, tt.wantCount)
			}
			for i, h := range tt.want {
				if i < len(hours) && hours[i].In(london).Hour() != h {
					t.Errorf("Hours(%d)[%d] = %v, want %02d:00", tt.every, i, hours[i].In(london), h)
				}
			}
			for _, at := range hours {
				if at.In(london).Minute() != 0 || at.In(london).Hour() == 0 {
					t.Errorf("Hours(%d) ticks %v, want whole hours, no midnight", tt.every, at.In(london))
				}
			}
		})
	}
}

func TestTheHourTheClocksGoForwardToAtMidnightIsTheDaysStartNotAnHour(t *testing.T) {
	santiago := loadZone(t, "America/Santiago")
	tl := views.Stepped(views.RunwayFiveHour, civil(2026, time.September, 6), time.Date(2026, 9, 8, 12, 0, 0, 0, santiago), time.Monday, 136)
	start := time.Date(2026, 9, 6, 1, 0, 0, 0, santiago)
	if !tl.Start.Equal(start) || !instantsEqual(tl.Midnights(), []time.Time{start}) {
		t.Errorf("the day starts at %v, its midnights %v, want both at %v, where the clocks went forward to", tl.Start, tl.Midnights(), start)
	}
	if hours := tl.Hours(1); len(hours) != 22 || hours[0].In(santiago).Hour() != 2 {
		t.Errorf("Hours(1) = %v, want the 22 from 02:00", hours)
	}
}

// instantsEqual reports whether got and want are the same instants, in
// order.
func instantsEqual(got, want []time.Time) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !got[i].Equal(want[i]) {
			return false
		}
	}
	return true
}

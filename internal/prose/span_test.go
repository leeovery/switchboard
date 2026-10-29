package prose_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

func TestCountdown(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "past", until: -time.Minute, want: "now"},
		{name: "due", until: 0, want: "now"},
		{name: "under a minute", until: 30 * time.Second, want: "0m"},
		{name: "minutes, rounded down", until: 7*time.Minute + 59*time.Second, want: "7m"},
		{name: "just under an hour", until: time.Hour - time.Second, want: "59m"},
		{name: "an hour", until: time.Hour, want: "1h"},
		{name: "an hour, and seconds too few for a minute", until: time.Hour + 59*time.Second, want: "1h"},
		{name: "just past an hour", until: time.Hour + time.Minute, want: "1h 1m"},
		{name: "whole hours", until: 4 * time.Hour, want: "4h"},
		{name: "hours and minutes", until: 4*time.Hour + 57*time.Minute + 30*time.Second, want: "4h 57m"},
		{name: "just under a day", until: 24*time.Hour - time.Second, want: "23h 59m"},
		{name: "a day", until: 24 * time.Hour, want: "1d"},
		{name: "a day, and minutes too few for an hour", until: 24*time.Hour + 59*time.Minute, want: "1d"},
		{name: "just past a day", until: 25 * time.Hour, want: "1d 1h"},
		{name: "whole days", until: 6 * 24 * time.Hour, want: "6d"},
		{name: "days and hours, rounded down", until: 5*24*time.Hour + 12*time.Hour + 59*time.Minute, want: "5d 12h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Countdown(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("Countdown(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}

// A duration is said as Countdown counts down one that long, but none is
// none, not now.
func TestADurationIsInWholeUnitsAsCountdownSaysIt(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{d: -time.Minute, want: "0m"},
		{d: 0, want: "0m"},
		{d: 7*time.Minute + 59*time.Second, want: "7m"},
		{d: 3*time.Hour + 10*time.Minute, want: "3h 10m"},
		{d: 4 * time.Hour, want: "4h"},
		{d: 5*24*time.Hour + 12*time.Hour + 59*time.Minute, want: "5d 12h"},
	}
	for _, tt := range tests {
		if got := prose.Duration(tt.d); got != tt.want {
			t.Errorf("Duration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// A column of Runway's timeline is said in whole minutes where it's whole,
// else about as long: to a tenth of a minute under 20 minutes, a tenth that
// rounds whole said whole, and to the minute from 20.
func TestAColumnsLengthIsInWholeMinutesWhereItsWholeElseAbout(t *testing.T) {
	const fiveHourSpan, day = 22*time.Hour + 40*time.Minute, 24 * time.Hour
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "the 5-hour at 136 columns", d: fiveHourSpan / 136, want: "10 minutes"},
		{name: "the week at 136", d: 7 * day / 136, want: "≈ 74 minutes"},
		{name: "a day stepped back to, at 136", d: day / 136, want: "≈ 10.6 minutes"},
		{name: "the 5-hour at 38", d: fiveHourSpan / 38, want: "≈ 36 minutes"},
		{name: "a 25-hour day, its tenth rounding whole", d: 25 * time.Hour / 136, want: "≈ 11 minutes"},
		{name: "the 5-hour at 176", d: fiveHourSpan / 176, want: "≈ 7.7 minutes"},
		{name: "a 23-hour day", d: 23 * time.Hour / 136, want: "≈ 10.1 minutes"},
		{name: "a second under 20 minutes", d: 20*time.Minute - time.Second, want: "≈ 20 minutes"},
		{name: "20 minutes and 40 seconds", d: 20*time.Minute + 40*time.Second, want: "≈ 21 minutes"},
		{name: "one minute", d: time.Minute, want: "1 minute"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.ColumnLength(tt.d); got != tt.want {
				t.Errorf("ColumnLength(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// Minutes count the minutes from the one one time falls in to the one
// another does, both counted, as a turn's took counts them: 14:29 to 14:37
// is 9 minutes, and two times within one minute are 1.
func TestMinutesCountEachMinuteFromTheOneOneTimeFallsInToAnothers(t *testing.T) {
	at := func(minute, second int) time.Time { return time.Date(2026, 10, 7, 14, minute, second, 0, time.UTC) }
	tests := []struct {
		from, to time.Time
		want     string
	}{
		{from: at(2, 0), to: at(2, 59), want: "1m"},
		{from: at(2, 59), to: at(5, 0), want: "4m"},
		{from: at(29, 4), to: at(37, 30), want: "9m"},
		{from: at(40, 0), to: at(42, 7), want: "3m"},
		{from: at(2, 30), to: at(42, 10), want: "41m"},
	}
	for _, tt := range tests {
		if got := prose.Minutes(tt.from, tt.to); got != tt.want {
			t.Errorf("Minutes(%v, %v) = %q, want %q", tt.from, tt.to, got, tt.want)
		}
	}
}

// Minutes within four characters count as Minutes does to 999, then whole
// hours to 99, then whole days, never rounding up.
func TestMinutesWithinHoldToFourCharacters(t *testing.T) {
	from := time.Date(2026, 10, 7, 14, 2, 30, 0, time.UTC)
	tests := []struct {
		minutes int
		want    string
	}{
		{minutes: 1, want: "1m"},
		{minutes: 999, want: "999m"},
		{minutes: 1000, want: "16h"},
		{minutes: 1019, want: "16h"},
		{minutes: 5999, want: "99h"},
		{minutes: 6000, want: "4d"},
		{minutes: 99_999, want: "69d"},
	}
	for _, tt := range tests {
		to := from.Add(time.Duration(tt.minutes-1) * time.Minute)
		if got := prose.MinutesWithin(from, to); got != tt.want {
			t.Errorf("MinutesWithin over %d minutes = %q, want %q", tt.minutes, got, tt.want)
		}
	}
}

func TestHoursAreWholeWhatsLeftOverCut(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{d: time.Hour, want: "1h"},
		{d: 15*time.Hour + 7*time.Minute, want: "15h"},
		{d: 2*time.Hour - time.Second, want: "1h"},
		{d: 30 * time.Hour, want: "30h"},
	}
	for _, tt := range tests {
		if got := prose.Hours(tt.d); got != tt.want {
			t.Errorf("Hours(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestASpanIsInSecondsUnderAMinuteThenAsTheDashboardWritesSpans(t *testing.T) {
	tests := []struct {
		name string
		span time.Duration
		want string
	}{
		{name: "none", span: 0, want: "0s"},
		{name: "less than none", span: -time.Second, want: "0s"},
		{name: "seconds, cut", span: 34*time.Second + 900*time.Millisecond, want: "34s"},
		{name: "just under a minute", span: time.Minute - time.Millisecond, want: "59s"},
		{name: "a minute", span: time.Minute, want: "1m"},
		{name: "minutes, cut", span: 9*time.Minute + 59*time.Second, want: "9m"},
		{name: "an hour", span: time.Hour, want: "1h"},
		{name: "an hour and minutes, the minutes two digits", span: time.Hour + 5*time.Minute, want: "1h 05m"},
		{name: "hours and minutes", span: 5*time.Hour + 27*time.Minute, want: "5h 27m"},
		{name: "a day and hours", span: 27 * time.Hour, want: "1d 3h"},
		{name: "whole days", span: 48*time.Hour + 30*time.Minute, want: "2d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Span(tt.span); got != tt.want {
				t.Errorf("Span(%v) = %q, want %q", tt.span, got, tt.want)
			}
		})
	}
}

func TestSeenAgoRoundsToTheMinuteThenTheHour(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{ago: 0, want: "now"}, {ago: 29 * time.Second, want: "now"}, {ago: 30 * time.Second, want: "1m"},
		{ago: 50 * time.Second, want: "1m"}, {ago: 9*time.Minute + 20*time.Second, want: "9m"},
		{ago: 59*time.Minute + 29*time.Second, want: "59m"}, {ago: 59*time.Minute + 30*time.Second, want: "1h"},
		{ago: 2*time.Hour + 29*time.Minute, want: "2h"},
	}
	for _, tt := range tests {
		if got := prose.SeenAgo(now, now.Add(-tt.ago)); got != tt.want {
			t.Errorf("seen %v ago reads %q, want %q", tt.ago, got, tt.want)
		}
	}
}

func TestSecondsAreToATenthUnderAMinuteThenMinutesAndSeconds(t *testing.T) {
	tests := []struct {
		took time.Duration
		want string
	}{
		{took: 0, want: "0.0s"},
		{took: 812 * time.Millisecond, want: "0.8s"},
		{took: 11849 * time.Millisecond, want: "11.8s"},
		{took: 11850 * time.Millisecond, want: "11.9s"},
		{took: 59949 * time.Millisecond, want: "59.9s"},
		{took: 59950 * time.Millisecond, want: "1m 00s"},
		{took: 65 * time.Second, want: "1m 05s"},
		{took: 123*time.Second + 400*time.Millisecond, want: "2m 03s"},
		{took: 123*time.Second + 500*time.Millisecond, want: "2m 04s"},
		{took: 75 * time.Minute, want: "75m 00s"},
	}
	for _, tt := range tests {
		if got := prose.Seconds(tt.took); got != tt.want {
			t.Errorf("Seconds(%v) = %q, want %q", tt.took, got, tt.want)
		}
	}
}

func TestAStopwatchShowsMinutesAndSecondsThenHoursFirst(t *testing.T) {
	tests := []struct {
		waited time.Duration
		want   string
	}{
		{waited: -time.Second, want: "0:00"},
		{waited: 0, want: "0:00"},
		{waited: 21*time.Second + 900*time.Millisecond, want: "0:21"},
		{waited: 59 * time.Second, want: "0:59"},
		{waited: time.Minute, want: "1:00"},
		{waited: 12*time.Minute + 5*time.Second, want: "12:05"},
		{waited: time.Hour - time.Second, want: "59:59"},
		{waited: time.Hour, want: "1:00:00"},
		{waited: time.Hour + 2*time.Minute + 5*time.Second, want: "1:02:05"},
	}
	for _, tt := range tests {
		if got := prose.Stopwatch(tt.waited); got != tt.want {
			t.Errorf("Stopwatch(%v) = %q, want %q", tt.waited, got, tt.want)
		}
	}
}

func TestResets(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "past", until: -time.Minute, want: "resets now"},
		{name: "due", until: 0, want: "resets now"},
		{name: "minutes", until: 7 * time.Minute, want: "resets in 7m"},
		{name: "days and hours", until: 5*24*time.Hour + 12*time.Hour, want: "resets in 5d 12h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Resets(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("Resets(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}

func TestUntilRoundsUpToTheMinuteAndPadsTheMinutesAfterHours(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 7, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "exactly on a minute", until: 4*time.Hour + 8*time.Minute, want: "in 4h 08m"},
		{name: "a second past a minute, rounded up", until: 4*time.Hour + 8*time.Minute + time.Second, want: "in 4h 09m"},
		{name: "a second short of a minute, rounded up", until: 4*time.Hour + 7*time.Minute + 59*time.Second, want: "in 4h 08m"},
		{name: "the frames' 16:30 from 14:42:07", until: time.Hour + 47*time.Minute + 53*time.Second, want: "in 1h 48m"},
		{name: "under a minute left", until: 30 * time.Second, want: "in 1m"},
		{name: "a moment left", until: time.Nanosecond, want: "in 1m"},
		{name: "rounded up into the next hour", until: 59*time.Minute + 30*time.Second, want: "in 1h"},
		{name: "rounded up into the next day", until: 23*time.Hour + 59*time.Minute + 30*time.Second, want: "in 1d"},
		{name: "whole hours, no minutes to pad", until: 4 * time.Hour, want: "in 4h"},
		{name: "minutes alone, unpadded", until: 7 * time.Minute, want: "in 7m"},
		{name: "hours after days, unpadded", until: 3*24*time.Hour + 6*time.Hour, want: "in 3d 6h"},
		{name: "due", until: 0, want: "now"},
		{name: "past", until: -3 * time.Minute, want: "now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Until(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("Until(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}

func TestOverSaysTheSpanARecentRateIsMeasuredOver(t *testing.T) {
	tests := []struct {
		over time.Duration
		want string
	}{
		{over: 30 * time.Minute, want: "last 30 min"},
		{over: 18*time.Minute + 59*time.Second, want: "last 18 min"},
		{over: time.Hour - time.Second, want: "last 59 min"},
		{over: time.Hour, want: "last 1h"},
		{over: time.Hour + 30*time.Minute, want: "last 1h 30m"},
		{over: 2 * time.Hour, want: "last 2h"},
		{over: 26 * time.Hour, want: "last 1d 2h"},
	}
	for _, tt := range tests {
		if got := prose.Over(tt.over); got != tt.want {
			t.Errorf("Over(%v) = %q, want %q", tt.over, got, tt.want)
		}
	}
}

func TestClockSpanShowsAPartOfADayAsItsClocksAnEnDashBetween(t *testing.T) {
	tests := []struct {
		start, end time.Duration
		want       string
	}{
		{start: 8 * time.Hour, end: 23 * time.Hour, want: "08:00–23:00"},
		{start: 22*time.Hour + 30*time.Minute, end: 6 * time.Hour, want: "22:30–06:00"},
	}
	for _, tt := range tests {
		if got := prose.ClockSpan(tt.start, tt.end); got != tt.want {
			t.Errorf("ClockSpan(%v, %v) = %q, want %q", tt.start, tt.end, got, tt.want)
		}
	}
}

func TestClockOfShowsATimeSinceMidnightAsHoursAndMinutes(t *testing.T) {
	tests := []struct {
		since time.Duration
		want  string
	}{
		{since: 0, want: "00:00"},
		{since: 3*time.Hour + 50*time.Minute, want: "03:50"},
		{since: 8 * time.Hour, want: "08:00"},
		{since: 23*time.Hour + 59*time.Minute + 59*time.Second, want: "23:59"},
	}
	for _, tt := range tests {
		if got := prose.ClockOf(tt.since); got != tt.want {
			t.Errorf("ClockOf(%v) = %q, want %q", tt.since, got, tt.want)
		}
	}
}

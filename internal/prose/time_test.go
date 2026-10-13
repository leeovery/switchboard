package prose_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

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

func TestWhenShowsTheDayOnlyBeyondADay(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	day := 24 * time.Hour
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "later today", t: now.Add(2 * time.Hour), want: "15:12"},
		{name: "tomorrow, within a day", t: now.Add(14 * time.Hour), want: "03:12"},
		{name: "earlier, within a day", t: now.Add(-20 * time.Hour), want: "17:12"},
		{name: "a day on", t: now.Add(day), want: "Tue 13:12"},
		{name: "days ago", t: now.Add(-3 * day), want: "Fri 13:12"},
		{name: "in UTC, shown in now's time zone", t: now.Add(time.Hour).UTC(), want: "14:12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.When(now, tt.t); got != tt.want {
				t.Errorf("When() = %q, want %q", got, tt.want)
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

package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// A week begins at the start of its first day, the day weeks start on, as
// the clocks put that day's midnight, in now's time zone.
func TestAWeekBeginsAtItsFirstDaysStart(t *testing.T) {
	london := loadZone(t, "Europe/London")
	santiago := loadZone(t, "America/Santiago")
	tests := []struct {
		name   string
		now    time.Time
		starts time.Weekday
		want   time.Time
	}{
		{name: "from Monday", now: time.Date(2026, 10, 1, 14, 42, 7, 0, london), starts: time.Monday, want: time.Date(2026, 9, 28, 0, 0, 0, 0, london)},
		{name: "from Sunday", now: time.Date(2026, 10, 1, 14, 42, 7, 0, london), starts: time.Sunday, want: time.Date(2026, 9, 27, 0, 0, 0, 0, london)},
		{name: "on its first day", now: time.Date(2026, 9, 28, 9, 0, 0, 0, london), starts: time.Monday, want: time.Date(2026, 9, 28, 0, 0, 0, 0, london)},
		{name: "at its first midnight", now: time.Date(2026, 10, 5, 0, 0, 0, 0, london), starts: time.Monday, want: time.Date(2026, 10, 5, 0, 0, 0, 0, london)},
		{name: "a second before it", now: time.Date(2026, 10, 4, 23, 59, 59, 0, london), starts: time.Monday, want: time.Date(2026, 9, 28, 0, 0, 0, 0, london)},
		{name: "the clocks gone back since it began", now: time.Date(2026, 10, 25, 12, 0, 0, 0, london), starts: time.Monday, want: time.Date(2026, 10, 19, 0, 0, 0, 0, london)},
		{
			// Chile's clocks went from 00:00 to 01:00 on Sunday 6 September
			// 2026, so a week from Sunday starts at 01:00.
			name: "its first midnight the clocks went forward over", now: time.Date(2026, 9, 9, 12, 0, 0, 0, santiago), starts: time.Sunday,
			want: time.Date(2026, 9, 6, 1, 0, 0, 0, santiago),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.WeekStart(tt.now, tt.starts); !got.Equal(tt.want) {
				t.Errorf("WeekStart(%v, %v) = %v, want %v", tt.now, tt.starts, got, tt.want)
			}
		})
	}
}

// Accounts' this week and Runway's week stepped back to begin where a week
// does.
func TestThisWeekBeginsWhereAWeekDoes(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 42, 7, 0, time.Local)
	for _, starts := range []time.Weekday{time.Monday, time.Sunday, time.Saturday} {
		want := views.WeekStart(now, starts)
		if got := views.AccountsThisWeek.From(now, starts); !got.Equal(want) {
			t.Errorf("this week from %v begins at %v, want %v, where WeekStart has it", starts, got, want)
		}
		if got := views.Stepped(views.RunwayWeek, civil(2026, time.October, 1), now, starts, 136).Start; !got.Equal(want) {
			t.Errorf("this week stepped back to from %v begins at %v, want %v, where WeekStart has it", starts, got, want)
		}
	}
}

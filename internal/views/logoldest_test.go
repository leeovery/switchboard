package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

func TestTheOldestDayIsTheLaterOfTheLedgersFirstAndTheOldestItKeeps(t *testing.T) {
	const day = 24 * time.Hour
	tests := []struct {
		name  string
		first time.Time
		keep  time.Duration
		want  string
	}{
		{name: "kept for good: the ledger's first day", first: time.Date(2024, 1, 10, 9, 0, 0, 0, time.Local), keep: dayfile.Forever, want: "2024-01-10"},
		{name: "kept 400 days, the ledger older: 400 days before today", first: time.Date(2024, 1, 10, 9, 0, 0, 0, time.Local), keep: 400 * day, want: "2025-09-02"},
		{name: "kept 400 days, the ledger younger: its first day", first: october(1, 23, 59), keep: 400 * day, want: "2026-10-01"},
		{name: "kept a day: yesterday", first: october(1, 9, 0), keep: day, want: "2026-10-06"},
		{name: "kept part of a day, counted whole", first: october(1, 9, 0), keep: day + time.Hour, want: "2026-10-05"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := views.OldestKept(tt.first, tt.keep, now)
			if got.Day.Date != tt.want || got.Keep != tt.keep || !got.Day.Start.Equal(views.Today(got.Day.Start).Start) {
				t.Errorf("the oldest day is %+v, want %s, kept %v", got, tt.want, tt.keep)
			}
			// Where the keep, not the ledger's first day, makes it the oldest,
			// it's the oldest day the ledger's files are kept on till today.
			if tt.keep == dayfile.Forever || got.Day.Date == views.Today(tt.first).Date {
				return
			}
			before, _ := views.OldestDay{}.DayBefore(got.Day)
			if until, _ := dayfile.KeptUntil(got.Day.Date, tt.keep); until != views.Today(now).Date {
				t.Errorf("the oldest day's lines are kept until %s, want today", until)
			}
			if until, _ := dayfile.KeptUntil(before.Date, tt.keep); until >= views.Today(now).Date {
				t.Errorf("the day before the oldest's lines are kept until %s, want before today", until)
			}
		})
	}
}

func TestTheLogReadsBackADayAtATimeToTheOldestDayAndNoFurther(t *testing.T) {
	oldest := views.OldestDay{Day: views.Today(october(5, 0, 0)), Keep: 400 * 24 * time.Hour}
	tests := []struct {
		name   string
		oldest views.OldestDay
		first  time.Time
		want   string
	}{
		{name: "after the oldest day: the day before", oldest: oldest, first: october(7, 0, 0), want: "2026-10-06"},
		{name: "the day after the oldest: the oldest", oldest: oldest, first: october(6, 13, 0), want: "2026-10-05"},
		{name: "the oldest day: none", oldest: oldest, first: october(5, 23, 0)},
		{name: "before the oldest day: none", oldest: oldest, first: october(3, 9, 0)},
		{name: "the oldest day not known: the day before", first: october(5, 9, 0), want: "2026-10-04"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.oldest.DayBefore(views.Today(tt.first))
			if got.Date != tt.want || ok != (tt.want != "") || ok && !got.Start.Equal(views.Today(got.Start).Start) {
				t.Errorf("the day read back to is %+v, %t, want %q", got, ok, tt.want)
			}
		})
	}
}

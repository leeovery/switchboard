package views

import (
	"maps"
	"testing"
	"time"
)

// dayCopy is a copy of the day with the given date, read whole at at, owed
// where owed says, holding one session whose id says whose copy it is.
func dayCopy(date, whose string, at time.Time, owed bool) sessionDay {
	return sessionDay{date: date, sessions: []DaySession{{ID: whose}}, at: at, owed: owed}
}

// whoseDays are whose copy each day of days is, by its date.
func whoseDays(days []sessionDay) map[string]string {
	whose := make(map[string]string, len(days))
	for _, d := range days {
		whose[d.date] = d.sessions[0].ID
	}
	return whose
}

// Of each day the tail's days and a read both hold, the copy read later
// stands, an owed one read whole at no time: Joined takes it, and Without
// lets go of the tail's where the read's is later, and of each day before
// the first the read read, so the tail's days hold only the days the read
// doesn't stand for.
func TestTheLaterReadOfADayStands(t *testing.T) {
	early := time.Date(2026, 10, 8, 0, 30, 0, 0, time.UTC)
	ten, eleven := early.Add(9*time.Hour+30*time.Minute), early.Add(10*time.Hour+30*time.Minute)
	read := SessionDays{first: "2026-10-05", days: []sessionDay{
		dayCopy("2026-10-05", "read", ten, false), dayCopy("2026-10-06", "read", ten, false), dayCopy("2026-10-07", "read", ten, false),
	}}
	tailed := SessionDays{days: []sessionDay{
		dayCopy("2026-10-04", "tail", early, false),
		dayCopy("2026-10-05", "tail", eleven, false),
		dayCopy("2026-10-06", "tail", early, false),
		dayCopy("2026-10-07", "tail", time.Time{}, true),
		dayCopy("2026-10-08", "tail", eleven, false),
	}}
	joined := map[string]string{"2026-10-04": "tail", "2026-10-05": "tail", "2026-10-06": "read", "2026-10-07": "read", "2026-10-08": "tail"}
	if got := whoseDays(tailed.Joined(read).days); !maps.Equal(got, joined) {
		t.Errorf("joined, the days are of %v, want %v: the copy read later of each, an owed one's never", got, joined)
	}
	kept := map[string]string{"2026-10-05": "tail", "2026-10-08": "tail"}
	if got := whoseDays(tailed.Without(read).days); !maps.Equal(got, kept) {
		t.Errorf("without the read, the tail's days are of %v, want %v: those read later alone, none before the read's first", got, kept)
	}
}

// Days tell otherwise than those they were fed from where today is another
// day, its fold took lines in or began afresh, or a day gone is of another
// read, or another day is held, or one let go of; never where they hold the
// same reads.
func TestDaysToldOtherwiseAsTheirDayFoldOrReadsChange(t *testing.T) {
	at := time.Date(2026, 10, 8, 0, 30, 0, 0, time.UTC)
	was := SessionDays{today: LedgerDay{Date: "2026-10-08"}, folds: 3, days: []sessionDay{dayCopy("2026-10-07", "tail", at, false)}}
	tests := []struct {
		name   string
		change func(d SessionDays) SessionDays
		told   bool
	}{
		{"the same reads", func(d SessionDays) SessionDays { return d }, false},
		{"today another day", func(d SessionDays) SessionDays { d.today.Date = "2026-10-09"; return d }, true},
		{"today's fold taking lines in", func(d SessionDays) SessionDays { d.folds++; return d }, true},
		{"a day of another read", func(d SessionDays) SessionDays {
			d.days = []sessionDay{dayCopy("2026-10-07", "tail", at.Add(time.Minute), false)}
			return d
		}, true},
		{"another day held", func(d SessionDays) SessionDays {
			d.days = withDay(d.days, dayCopy("2026-10-06", "tail", at, false))
			return d
		}, true},
		{"a day let go of", func(d SessionDays) SessionDays { d.days = nil; return d }, true},
	}
	for _, tt := range tests {
		if got := tt.change(was).Told(was); got != tt.told {
			t.Errorf("%s, told %t, want %t", tt.name, got, tt.told)
		}
	}
}

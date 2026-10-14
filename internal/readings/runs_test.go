package readings_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
)

// Runway's tail carries in each window's run from before its start: its
// readings back to the first of the unbroken run that ends with its last, no
// reset any of them gives passed before the next was read, each window's
// apart, those filed late in their places, ordered by when they were read;
// then those from the start, as they were read.
func TestRunwaysTailCarriesInEachWindowsRun(t *testing.T) {
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	from, end := start.AddDate(0, 0, -7), start.AddDate(0, 0, 1)
	of := func(key string, hours int, used float64, resets time.Time) readings.Reading {
		r := workRead(start.Add(time.Duration(hours)*time.Hour), used)
		r.Key, r.ResetsAt = key, resets
		return r
	}
	periodEnd, weekEnd := start.Add(-48*time.Hour), start.AddDate(0, 0, 3)
	tooOld := of("7d", -24*8, 0.1, weekEnd)
	periodBefore := of("7d", -50, 1, periodEnd)
	first, middle, last := of("7d", -40, 0.2, weekEnd), of("7d", -20, 0.6, weekEnd), of("7d", -1, 1, weekEnd)
	fiveHourEnded, fiveHour := of("5h", -10, 0.9, start.Add(-8*time.Hour)), of("5h", -2, 0.4, start.Add(2*time.Hour))
	today := of("7d", 1, 1, weekEnd)
	tests := []struct {
		name       string
		read, want []readings.Reading
	}{
		{name: "a run whole", read: []readings.Reading{first, middle, last, today}, want: []readings.Reading{first, middle, last, today}},
		{
			name: "a run after one whose reset passed, the reading before it left out",
			read: []readings.Reading{periodBefore, first, middle, last, today}, want: []readings.Reading{first, middle, last, today},
		},
		{
			name: "a reading of the run filed late, in its place",
			read: []readings.Reading{periodBefore, first, last, middle, today}, want: []readings.Reading{first, middle, last, today},
		},
		{
			name: "a reading of the period before filed late, left out",
			read: []readings.Reading{first, middle, last, periodBefore, today}, want: []readings.Reading{first, middle, last, today},
		},
		{name: "a reading from before the carry left out", read: []readings.Reading{tooOld, first, last}, want: []readings.Reading{first, last}},
		{
			name: "each window's run apart, the 5-hour's from after its last reset",
			read: []readings.Reading{periodBefore, first, fiveHourEnded, middle, fiveHour, last, today},
			want: []readings.Reading{first, middle, fiveHour, last, today},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readings.CarriedRuns(slices.Values(tt.read), from, start, end); !slices.EqualFunc(got, tt.want, sameReading) {
				t.Errorf("CarriedRuns() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

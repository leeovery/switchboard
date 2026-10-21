package views

import (
	"testing"
	"time"
)

// Every span the Year shows holds MostYearWeeks at most, and some reach it:
// the last 12 months, on every day of a year, and every calendar year from
// 2024 to 2035, leap years among them, with weeks starting on each day.
func TestEverySpanOfTheYearHoldsMostYearWeeksAtMost(t *testing.T) {
	today := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	most := 0
	for start := time.Sunday; start <= time.Saturday; start++ {
		for year := 2024; year <= 2035; year++ {
			s := yearSpanOf(year, today, start)
			if s.weeks > MostYearWeeks {
				t.Errorf("%d, weeks from %s, holds %d weeks, past %d", year, start, s.weeks, MostYearWeeks)
			}
			most = max(most, s.weeks)
		}
		for day := range 366 {
			s := yearSpanOf(0, today.AddDate(0, 0, day), start)
			if s.weeks > MostYearWeeks {
				t.Errorf("the 12 months to %s, weeks from %s, hold %d weeks, past %d", today.AddDate(0, 0, day).Format(time.DateOnly), start, s.weeks, MostYearWeeks)
			}
		}
	}
	if most != MostYearWeeks {
		t.Errorf("no span reaches %d weeks, the most any holds %d", MostYearWeeks, most)
	}
	if got := yearSpanOf(2028, today, time.Sunday).weeks; got != MostYearWeeks {
		t.Errorf("2028, weeks from Sunday, holds %d weeks, want %d", got, MostYearWeeks)
	}
}
